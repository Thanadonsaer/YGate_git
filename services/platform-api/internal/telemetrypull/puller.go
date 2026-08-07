// Package telemetrypull drives the backend side of the telemetry.drain /
// telemetry.ack handshake described in
// docs/superpowers/specs/2026-08-06-telemetry-backend-pull-design.md: one
// ticker per connected gateway that pulls its buffered outbox rows over the
// existing gatewayhub-managed WebSocket instead of waiting for the
// middleware to push them over REST.
package telemetrypull

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
)

const (
	drainBatchSize      = 20
	commandTimeout      = 15 * time.Second
	defaultPullInterval = 60 * time.Second
)

// Ingester is the subset of *ingestion.Service that Run/pullOnce need -- kept
// as an interface so tests can stub it out without a real database.
type Ingester interface {
	IngestRaw(ctx context.Context, client ingestion.Client, idempotencyKey string, raw []byte, batch ingestion.RawBatch, now time.Time) (ingestion.Result, error)
	MiddlewareClientPullConfig(ctx context.Context, clientID pgtype.UUID) (pollIntervalSeconds int32, apiPollingEnabled bool, err error)
	RecordMiddlewarePullEvent(ctx context.Context, client ingestion.Client, action string, details map[string]any) error
}

// Run loops until ctx is done. Each cycle it re-reads gatewayID's current
// poll_interval_seconds/api_polling_enabled from the database (NOT just
// once at startup) so an admin toggling API Polling from the web UI takes
// effect within one cycle -- no WebSocket reconnect required -- and so a
// freshly (re)connected gateway gets its first drain immediately instead
// of waiting a full interval. Call as `go telemetrypull.Run(...)` right
// after the gateway's WebSocket registers with hub -- ctx should be that
// connection's own request context, so this goroutine exits when the
// connection does.
func Run(ctx context.Context, hub *gatewayhub.Hub, ingest Ingester, client ingestion.Client, gatewayID string) {
	for {
		interval := defaultPullInterval
		pollIntervalSeconds, apiPollingEnabled, err := ingest.MiddlewareClientPullConfig(ctx, client.ID)
		switch {
		case err != nil:
			log.Printf("telemetry pull: read pull config for %s: %v", gatewayID, err)
		case !apiPollingEnabled:
			log.Printf("telemetry pull: disabled for %s (api_polling_enabled=false)", gatewayID)
		default:
			if pollIntervalSeconds > 0 {
				interval = time.Duration(pollIntervalSeconds) * time.Second
			}
			pullOnce(ctx, hub, ingest, client, gatewayID)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

type commandResult struct {
	Ok    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

type drainedBatch struct {
	IDs      []int64           `json:"ids"`
	Readings []json.RawMessage `json:"readings"`
}

func pullOnce(ctx context.Context, hub *gatewayhub.Hub, ingest Ingester, client ingestion.Client, gatewayID string) {
	started := time.Now()
	audit := func(action string, details map[string]any) {
		details["gatewayId"] = gatewayID
		details["durationMs"] = time.Since(started).Milliseconds()
		if err := ingest.RecordMiddlewarePullEvent(ctx, client, action, details); err != nil {
			log.Printf("telemetry pull: audit %s for %s: %v", action, gatewayID, err)
		}
	}
	audit("middleware.pull.started", map[string]any{"batchSize": drainBatchSize})

	drainCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	commandID := newCommandID()
	data, _ := json.Marshal(map[string]any{"batchSize": drainBatchSize})
	payload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": commandID, "kind": "telemetry.drain", "data": data})
	raw, err := hub.RunCommand(drainCtx, gatewayID, commandID, payload)
	if err != nil {
		log.Printf("telemetry pull: drain %s failed: %v", gatewayID, err)
		audit("middleware.pull.failed", map[string]any{"stage": "drain", "error": err.Error()})
		return
	}
	var result commandResult
	if err = json.Unmarshal(raw, &result); err != nil {
		log.Printf("telemetry pull: decode drain result for %s failed: %v", gatewayID, err)
		audit("middleware.pull.failed", map[string]any{"stage": "drain_decode", "error": err.Error()})
		return
	}
	if !result.Ok {
		log.Printf("telemetry pull: drain %s rejected: %s", gatewayID, result.Error)
		audit("middleware.pull.failed", map[string]any{"stage": "drain", "error": result.Error})
		return
	}
	var drained drainedBatch
	if err = json.Unmarshal(result.Data, &drained); err != nil {
		log.Printf("telemetry pull: decode drained batch for %s failed: %v", gatewayID, err)
		audit("middleware.pull.failed", map[string]any{"stage": "batch_decode", "error": err.Error()})
		return
	}
	if len(drained.IDs) == 0 {
		audit("middleware.pull.empty", map[string]any{"count": 0})
		return
	}

	body, _ := json.Marshal(map[string]any{"schemaVersion": ingestion.RawSchemaVersion, "data": drained.Readings})
	var batch ingestion.RawBatch
	if err = json.Unmarshal(body, &batch); err != nil {
		log.Printf("telemetry pull: decode drained batch for %s failed: %v", gatewayID, err)
		audit("middleware.pull.failed", map[string]any{"stage": "batch_decode", "error": err.Error(), "count": len(drained.IDs)})
		return
	}
	resultIngest, err := ingest.IngestRaw(ctx, client, "", body, batch, time.Now())
	if err != nil {
		log.Printf("telemetry pull: ingest for %s failed: %v", gatewayID, err)
		audit("middleware.pull.failed", map[string]any{"stage": "ingest", "count": len(drained.IDs), "error": err.Error()})
		return
	}
	resultDetails := map[string]any{"count": len(drained.IDs), "accepted": resultIngest.AcceptedCount, "duplicate": resultIngest.DuplicateCount, "rejected": resultIngest.RejectedCount}
	if len(resultIngest.Errors) > 0 {
		resultDetails["errors"] = resultIngest.Errors
	}
	if resultIngest.AcceptedCount == 0 && resultIngest.DuplicateCount == 0 {
		log.Printf("telemetry pull: ingest rejected all %d readings for %s: %+v", len(drained.IDs), gatewayID, resultIngest.Errors)
		resultDetails["stage"] = "ingest_result"
		audit("middleware.pull.failed", resultDetails)
		return // keep rows pending so the operator can fix the rejection and retry
	}
	audit("middleware.pull.succeeded", resultDetails)

	ackCtx, cancelAck := context.WithTimeout(ctx, commandTimeout)
	defer cancelAck()
	ackCommandID := newCommandID()
	ackData, _ := json.Marshal(map[string]any{"ids": drained.IDs})
	ackPayload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": ackCommandID, "kind": "telemetry.ack", "data": ackData})
	if _, err = hub.RunCommand(ackCtx, gatewayID, ackCommandID, ackPayload); err != nil {
		log.Printf("telemetry pull: ack for %s failed: %v (rows will redeliver next tick)", gatewayID, err)
		audit("middleware.pull.ack_failed", map[string]any{"stage": "ack", "count": len(drained.IDs), "error": err.Error()})
	}
}
func newCommandID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

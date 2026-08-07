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
	drainBatchSize      = 500
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
		if pollIntervalSeconds > 0 {
			interval = time.Duration(pollIntervalSeconds) * time.Second
		}
		if err != nil {
			log.Printf("telemetry pull: read pull config for %s: %v", gatewayID, err)
			if !waitForPullSchedule(ctx, time.Now().UTC().Add(interval)) {
				return
			}
			continue
		}
		if !apiPollingEnabled {
			log.Printf("telemetry pull: disabled for %s (api_polling_enabled=false)", gatewayID)
			if !waitForPullSchedule(ctx, time.Now().UTC().Add(interval)) {
				return
			}
			continue
		}
		// Align to wall-clock slots: a five-minute interval runs at :00, :05,
		// :10, ... instead of five minutes after the WebSocket connected.
		if !waitForPullSchedule(ctx, nextScheduledPull(time.Now(), interval)) {
			return
		}
		pullOnce(ctx, hub, ingest, client, gatewayID)
	}
}

func nextScheduledPull(now time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		interval = defaultPullInterval
	}
	epoch := time.Unix(0, 0).In(now.Location())
	elapsed := now.Sub(epoch)
	return epoch.Add(elapsed.Truncate(interval) + interval)
}

func waitForPullSchedule(ctx context.Context, at time.Time) bool {
	delay := time.Until(at)
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type commandResult struct {
	Ok    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

type plantLog struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func plantLogs(readings []json.RawMessage) []plantLog {
	seen := make(map[string]bool)
	plants := make([]plantLog, 0)
	for _, raw := range readings {
		var reading struct {
			PlantCode string `json:"plantCode"`
			PlantName string `json:"plantName"`
		}
		if json.Unmarshal(raw, &reading) != nil || reading.PlantCode == "" || seen[reading.PlantCode] {
			continue
		}
		seen[reading.PlantCode] = true
		plants = append(plants, plantLog{Code: reading.PlantCode, Name: reading.PlantName})
	}
	return plants
}

func observedWindow(readings []json.RawMessage) (string, string) {
	var from, to time.Time
	for _, raw := range readings {
		var reading struct {
			CollectTime int64 `json:"collectTime"`
		}
		if json.Unmarshal(raw, &reading) != nil || reading.CollectTime <= 0 {
			continue
		}
		observed := time.UnixMilli(reading.CollectTime).UTC()
		if from.IsZero() || observed.Before(from) {
			from = observed
		}
		if to.IsZero() || observed.After(to) {
			to = observed
		}
	}
	if from.IsZero() {
		return "", ""
	}
	return from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano)
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
	// No "started" audit row: at the default 60s interval that was ~1,440
	// rows per gateway per day in audit_log (which has no retention) carrying
	// nothing a later succeeded/failed/empty row doesn't already say.
	drainCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	commandID := newCommandID()
	raw, err := hub.RunCommand(drainCtx, gatewayID, commandID,
		commandPayload(commandID, "telemetry.drain", map[string]any{"batchSize": drainBatchSize}))
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
	resultDetails := map[string]any{"count": len(drained.IDs), "accepted": resultIngest.AcceptedCount, "duplicate": resultIngest.DuplicateCount, "rejected": resultIngest.RejectedCount, "receivedAt": time.Now().UTC().Format(time.RFC3339Nano)}
	if observedFrom, observedTo := observedWindow(drained.Readings); observedFrom != "" {
		resultDetails["observedFrom"] = observedFrom
		resultDetails["observedTo"] = observedTo
	}
	if plants := plantLogs(drained.Readings); len(plants) > 0 {
		resultDetails["plants"] = plants
	}
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
	ackPayload := commandPayload(ackCommandID, "telemetry.ack", map[string]any{"ids": drained.IDs})
	ackRaw, err := hub.RunCommand(ackCtx, gatewayID, ackCommandID, ackPayload)
	ackErr := ""
	switch {
	case err != nil:
		ackErr = err.Error()
	default:
		// A command that reached the Gateway and came back `ok:false` is still
		// a failed ack: the rows stay PENDING and redeliver forever, so every
		// later pull is pure duplicates. Only the transport error was being
		// checked, which made that failure mode completely silent.
		var ackResult commandResult
		if decodeErr := json.Unmarshal(ackRaw, &ackResult); decodeErr != nil {
			ackErr = "decode ack result: " + decodeErr.Error()
		} else if !ackResult.Ok {
			ackErr = ackResult.Error
			if ackErr == "" {
				ackErr = "gateway rejected the acknowledgement"
			}
		}
	}
	if ackErr != "" {
		log.Printf("telemetry pull: ack for %s failed: %s (rows will redeliver next tick)", gatewayID, ackErr)
		audit("middleware.pull.ack_failed", map[string]any{"stage": "ack", "count": len(drained.IDs), "error": ackErr})
	}
}

// commandPayload builds a command.request frame with data nested as a JSON
// object.
//
// The nesting is the whole point: the Middleware decodes `data` into a
// json.RawMessage and unmarshals it into the command's own request struct.
// Marshalling an already-encoded []byte into map[string]any does NOT nest it
// -- encoding/json base64-encodes []byte into a JSON *string* -- so the
// Middleware got `"data":"eyJiYXRjaFNpemUiOjUwMH0="`, failed to decode it, and
// silently fell back to its defaults: telemetry.drain ignored batchSize and
// used 20 (which is why every pull moved exactly 20 readings no matter what
// drainBatchSize said), and telemetry.ack saw an empty id list. Wrapping in
// json.RawMessage is what keeps it an object on the wire.
func commandPayload(commandID, kind string, data any) []byte {
	encoded, _ := json.Marshal(data)
	payload, _ := json.Marshal(map[string]any{
		"type": "command.request", "commandId": commandID, "kind": kind,
		"data": json.RawMessage(encoded),
	})
	return payload
}

func newCommandID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

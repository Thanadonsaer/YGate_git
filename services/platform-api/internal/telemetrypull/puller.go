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

	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
)

const (
	drainBatchSize = 20
	commandTimeout = 15 * time.Second
)

// Ingester is the subset of *ingestion.Service that pullOnce needs -- kept
// as an interface so tests can stub it out without a real database.
type Ingester interface {
	IngestRaw(ctx context.Context, client ingestion.Client, idempotencyKey string, raw []byte, batch ingestion.RawBatch, now time.Time) (ingestion.Result, error)
}

// Run ticks at interval until ctx is done, draining and ingesting gatewayID's
// buffered telemetry each tick. Call as `go telemetrypull.Run(...)` right
// after the gateway's WebSocket registers with hub -- ctx should be that
// connection's own request context, so this goroutine exits when the
// connection does.
func Run(ctx context.Context, hub *gatewayhub.Hub, ingest Ingester, client ingestion.Client, gatewayID string, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pullOnce(ctx, hub, ingest, client, gatewayID)
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
	drainCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	commandID := newCommandID()
	data, _ := json.Marshal(map[string]any{"batchSize": drainBatchSize})
	payload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": commandID, "kind": "telemetry.drain", "data": data})
	raw, err := hub.RunCommand(drainCtx, gatewayID, commandID, payload)
	if err != nil {
		return // offline or timed out -- try again next tick, nothing was mutated
	}
	var result commandResult
	if err = json.Unmarshal(raw, &result); err != nil || !result.Ok {
		log.Printf("telemetry pull: drain %s failed: ok=%v err=%v msg=%q", gatewayID, result.Ok, err, result.Error)
		return
	}
	var drained drainedBatch
	if err = json.Unmarshal(result.Data, &drained); err != nil || len(drained.IDs) == 0 {
		return
	}

	body, _ := json.Marshal(map[string]any{"schemaVersion": ingestion.RawSchemaVersion, "data": drained.Readings})
	var batch ingestion.RawBatch
	if err = json.Unmarshal(body, &batch); err != nil {
		log.Printf("telemetry pull: decode drained batch for %s: %v", gatewayID, err)
		return
	}
	if _, err = ingest.IngestRaw(ctx, client, "", body, batch, time.Now()); err != nil {
		log.Printf("telemetry pull: ingest for %s: %v", gatewayID, err)
		return // no ack sent -- rows stay PENDING and are redrained next tick
	}

	ackCtx, cancelAck := context.WithTimeout(ctx, commandTimeout)
	defer cancelAck()
	ackCommandID := newCommandID()
	ackData, _ := json.Marshal(map[string]any{"ids": drained.IDs})
	ackPayload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": ackCommandID, "kind": "telemetry.ack", "data": ackData})
	if _, err = hub.RunCommand(ackCtx, gatewayID, ackCommandID, ackPayload); err != nil {
		log.Printf("telemetry pull: ack for %s: %v (rows will redeliver next tick)", gatewayID, err)
	}
}

func newCommandID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

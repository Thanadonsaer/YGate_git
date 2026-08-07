package telemetrypull

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
)

func TestNextScheduledPullAlignsToWallClock(t *testing.T) {
	location := time.FixedZone("ICT", 7*60*60)
	cases := []struct {
		name     string
		now      time.Time
		interval time.Duration
		want     time.Time
	}{
		{"five minute slot", time.Date(2026, 8, 7, 12, 47, 13, 0, location), 5 * time.Minute, time.Date(2026, 8, 7, 12, 50, 0, 0, location)},
		{"exact boundary advances", time.Date(2026, 8, 7, 12, 50, 0, 0, location), 5 * time.Minute, time.Date(2026, 8, 7, 12, 55, 0, 0, location)},
		{"one minute slot", time.Date(2026, 8, 7, 12, 59, 59, 0, location), time.Minute, time.Date(2026, 8, 7, 13, 0, 0, 0, location)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextScheduledPull(tc.now, tc.interval); !got.Equal(tc.want) {
				t.Fatalf("nextScheduledPull() = %s, want %s", got, tc.want)
			}
		})
	}
}

type stubIngester struct {
	calls      []ingestion.RawBatch
	err        error
	auditCalls []string
	result     ingestion.Result

	pollIntervalSeconds int32
	apiPollingEnabled   bool
	pullConfigErr       error
}

func (s *stubIngester) RecordMiddlewarePullEvent(ctx context.Context, client ingestion.Client, action string, details map[string]any) error {
	s.auditCalls = append(s.auditCalls, action)
	return nil
}

func (s *stubIngester) IngestRaw(ctx context.Context, client ingestion.Client, idempotencyKey string, raw []byte, batch ingestion.RawBatch, now time.Time) (ingestion.Result, error) {
	s.calls = append(s.calls, batch)
	if s.err != nil {
		return ingestion.Result{}, s.err
	}
	if s.result.Status != "" {
		return s.result, nil
	}
	return ingestion.Result{Status: "accepted", AcceptedCount: int32(len(batch.Data))}, nil
}

func (s *stubIngester) MiddlewareClientPullConfig(ctx context.Context, clientID pgtype.UUID) (int32, bool, error) {
	return s.pollIntervalSeconds, s.apiPollingEnabled, s.pullConfigErr
}

func TestPullOnceDrainsIngestsThenAcks(t *testing.T) {
	hub := gatewayhub.New()
	out, resolve, unregister := hub.Register("gw-1")
	defer unregister()

	reading, _ := json.Marshal(map[string]any{
		"gatewayId": "gw-1", "devDn": "dev-1", "plantCode": "P1", "devTypeId": 1,
		"collectTime": time.Now().UnixMilli(), "registerAddressMap": map[string]float64{"40001": 1},
	})

	go func() {
		drainPayload := <-out
		var drainReq struct {
			CommandID string `json:"commandId"`
			Kind      string `json:"kind"`
		}
		_ = json.Unmarshal(drainPayload, &drainReq)
		if drainReq.Kind != "telemetry.drain" {
			t.Errorf("first command kind = %q, want telemetry.drain", drainReq.Kind)
		}
		drainResultData, _ := json.Marshal(map[string]any{"ids": []int64{1}, "readings": []json.RawMessage{reading}})
		resolve(drainReq.CommandID, mustMarshal(map[string]any{"ok": true, "data": json.RawMessage(drainResultData)}))

		ackPayload := <-out
		var ackReq struct {
			CommandID string `json:"commandId"`
			Kind      string `json:"kind"`
		}
		_ = json.Unmarshal(ackPayload, &ackReq)
		if ackReq.Kind != "telemetry.ack" {
			t.Errorf("second command kind = %q, want telemetry.ack", ackReq.Kind)
		}
		resolve(ackReq.CommandID, mustMarshal(map[string]any{"ok": true}))
	}()

	ingest := &stubIngester{}
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pullOnce(context.Background(), hub, ingest, client, "gw-1")

	if len(ingest.calls) != 1 || len(ingest.calls[0].Data) != 1 {
		t.Fatalf("ingest.calls = %+v, want exactly 1 call with 1 reading", ingest.calls)
	}
	want := []string{"middleware.pull.succeeded"}
	if !slices.Equal(ingest.auditCalls, want) {
		t.Fatalf("auditCalls = %v, want %v", ingest.auditCalls, want)
	}
}

func TestPullOnceDoesNotAckWhenAllReadingsRejected(t *testing.T) {
	hub := gatewayhub.New()
	out, resolve, unregister := hub.Register("gw-1")
	defer unregister()

	reading, _ := json.Marshal(map[string]any{
		"gatewayId": "gw-1", "devDn": "dev-1", "plantCode": "P1", "devTypeId": 1,
		"collectTime": time.Now().UnixMilli(), "registerAddressMap": map[string]float64{"40001": 1},
	})
	go func() {
		drainPayload := <-out
		var req struct {
			CommandID string `json:"commandId"`
		}
		_ = json.Unmarshal(drainPayload, &req)
		data, _ := json.Marshal(map[string]any{"ids": []int64{1}, "readings": []json.RawMessage{reading}})
		resolve(req.CommandID, mustMarshal(map[string]any{"ok": true, "data": json.RawMessage(data)}))
		select {
		case <-out:
			t.Error("unexpected ack after all readings were rejected")
		case <-time.After(100 * time.Millisecond):
		}
	}()

	ingest := &stubIngester{result: ingestion.Result{Status: "accepted", RejectedCount: 1, Errors: []ingestion.RecordError{{Code: "UNKNOWN_DEVICE", Message: "device is not registered"}}}}
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pullOnce(context.Background(), hub, ingest, client, "gw-1")
	if !slices.Equal(ingest.auditCalls, []string{"middleware.pull.failed"}) {
		t.Fatalf("auditCalls = %v, want failed", ingest.auditCalls)
	}
}
func TestPullOnceSkipsSilentlyWhenGatewayOffline(t *testing.T) {
	hub := gatewayhub.New()
	ingest := &stubIngester{}
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pullOnce(context.Background(), hub, ingest, client, "gw-offline") // must not panic or block
	if len(ingest.calls) != 0 {
		t.Fatalf("ingest.calls = %+v, want none for an offline gateway", ingest.calls)
	}
}

func TestPullOnceSkipsAckWhenIngestErrors(t *testing.T) {
	hub := gatewayhub.New()
	out, resolve, unregister := hub.Register("gw-1")
	defer unregister()

	reading, _ := json.Marshal(map[string]any{
		"gatewayId": "gw-1", "devDn": "dev-1", "plantCode": "P1", "devTypeId": 1,
		"collectTime": time.Now().UnixMilli(), "registerAddressMap": map[string]float64{"40001": 1},
	})

	go func() {
		drainPayload := <-out
		var drainReq struct {
			CommandID string `json:"commandId"`
			Kind      string `json:"kind"`
		}
		_ = json.Unmarshal(drainPayload, &drainReq)
		if drainReq.Kind != "telemetry.drain" {
			t.Errorf("first command kind = %q, want telemetry.drain", drainReq.Kind)
		}
		drainResultData, _ := json.Marshal(map[string]any{"ids": []int64{1}, "readings": []json.RawMessage{reading}})
		resolve(drainReq.CommandID, mustMarshal(map[string]any{"ok": true, "data": json.RawMessage(drainResultData)}))

		// Wait briefly to verify no second command (telemetry.ack) is sent.
		// Use a timeout to avoid blocking forever if there's a bug.
		select {
		case <-out:
			t.Error("unexpected second command sent after ingest error (should skip ack)")
		case <-time.After(100 * time.Millisecond):
			// Expected: no second command
		}
	}()

	ingest := &stubIngester{err: errors.New("simulated ingest failure")}
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pullOnce(context.Background(), hub, ingest, client, "gw-1")

	if len(ingest.calls) != 1 {
		t.Fatalf("ingest.calls = %+v, want exactly 1 call", ingest.calls)
	}
}

func TestRunPullsOnNextAlignedScheduleOnceEnabled(t *testing.T) {
	hub := gatewayhub.New()
	out, resolve, unregister := hub.Register("gw-1")
	defer unregister()

	reading, _ := json.Marshal(map[string]any{
		"gatewayId": "gw-1", "devDn": "dev-1", "plantCode": "P1", "devTypeId": 1,
		"collectTime": time.Now().UnixMilli(), "registerAddressMap": map[string]float64{"40001": 1},
	})

	ingest := &stubIngester{apiPollingEnabled: true, pollIntervalSeconds: 1} // one-second schedule keeps the aligned-slot test fast
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go Run(ctx, hub, ingest, client, "gw-1")

	drainPayload := <-out
	var drainReq struct {
		CommandID string `json:"commandId"`
		Kind      string `json:"kind"`
	}
	_ = json.Unmarshal(drainPayload, &drainReq)
	if drainReq.Kind != "telemetry.drain" {
		t.Fatalf("first command kind = %q, want telemetry.drain", drainReq.Kind)
	}
	drainResultData, _ := json.Marshal(map[string]any{"ids": []int64{1}, "readings": []json.RawMessage{reading}})
	resolve(drainReq.CommandID, mustMarshal(map[string]any{"ok": true, "data": json.RawMessage(drainResultData)}))

	ackPayload := <-out
	var ackReq struct {
		CommandID string `json:"commandId"`
	}
	_ = json.Unmarshal(ackPayload, &ackReq)
	resolve(ackReq.CommandID, mustMarshal(map[string]any{"ok": true}))

	if len(ingest.calls) != 1 {
		t.Fatalf("ingest.calls = %d, want exactly 1 after the next aligned schedule", len(ingest.calls))
	}
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// Decodes a command frame the way modbus-api-middleware's realtimeclient does
// (`Data json.RawMessage`, then Unmarshal into the command's request struct).
// Before commandPayload existed, `data` went out base64-encoded as a JSON
// string, both decodes below failed, and the Middleware silently used its
// fallbacks -- batchSize 20 for every drain, and an empty id list for every ack.
func TestCommandPayloadNestsDataAsObjectForMiddleware(t *testing.T) {
	decodeData := func(t *testing.T, payload []byte) json.RawMessage {
		t.Helper()
		var msg struct {
			Type      string          `json:"type"`
			CommandID string          `json:"commandId"`
			Kind      string          `json:"kind"`
			Data      json.RawMessage `json:"data,omitempty"`
		}
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("decode command frame: %v", err)
		}
		if msg.Type != "command.request" || msg.CommandID == "" || msg.Kind == "" {
			t.Fatalf("command frame = %+v", msg)
		}
		return msg.Data
	}

	var drain struct {
		BatchSize int `json:"batchSize"`
	}
	data := decodeData(t, commandPayload("cmd-1", "telemetry.drain", map[string]any{"batchSize": drainBatchSize}))
	if err := json.Unmarshal(data, &drain); err != nil {
		t.Fatalf("Middleware cannot decode drain data %s: %v", data, err)
	}
	if drain.BatchSize != drainBatchSize {
		t.Fatalf("batchSize = %d, want %d (Middleware falls back to 20 when this does not arrive)", drain.BatchSize, drainBatchSize)
	}

	var ack struct {
		IDs []int64 `json:"ids"`
	}
	ackData := decodeData(t, commandPayload("cmd-2", "telemetry.ack", map[string]any{"ids": []int64{7, 8, 9}}))
	if err := json.Unmarshal(ackData, &ack); err != nil {
		t.Fatalf("Middleware cannot decode ack data %s: %v", ackData, err)
	}
	if !slices.Equal(ack.IDs, []int64{7, 8, 9}) {
		t.Fatalf("ack ids = %v, want [7 8 9] (an empty list acks nothing and the rows redeliver forever)", ack.IDs)
	}
}

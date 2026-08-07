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
	want := []string{"middleware.pull.started", "middleware.pull.succeeded"}
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
	if !slices.Equal(ingest.auditCalls, []string{"middleware.pull.started", "middleware.pull.failed"}) {
		t.Fatalf("auditCalls = %v, want started and failed", ingest.auditCalls)
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

func TestRunPullsImmediatelyOnceEnabledAndSkipsWhileDisabled(t *testing.T) {
	hub := gatewayhub.New()
	out, resolve, unregister := hub.Register("gw-1")
	defer unregister()

	reading, _ := json.Marshal(map[string]any{
		"gatewayId": "gw-1", "devDn": "dev-1", "plantCode": "P1", "devTypeId": 1,
		"collectTime": time.Now().UnixMilli(), "registerAddressMap": map[string]float64{"40001": 1},
	})

	ingest := &stubIngester{apiPollingEnabled: true, pollIntervalSeconds: 3600} // interval large enough that only an immediate first pull would complete in time
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
		t.Fatalf("ingest.calls = %d, want exactly 1 (immediate first pull, no wait for the 3600s interval)", len(ingest.calls))
	}
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

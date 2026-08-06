package telemetrypull

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
)

type stubIngester struct {
	calls []ingestion.RawBatch
	err   error
}

func (s *stubIngester) IngestRaw(ctx context.Context, client ingestion.Client, idempotencyKey string, raw []byte, batch ingestion.RawBatch, now time.Time) (ingestion.Result, error) {
	s.calls = append(s.calls, batch)
	if s.err != nil {
		return ingestion.Result{}, s.err
	}
	return ingestion.Result{Status: "accepted", AcceptedCount: int32(len(batch.Data))}, nil
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

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

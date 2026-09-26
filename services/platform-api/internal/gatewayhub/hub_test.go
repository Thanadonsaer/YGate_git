package gatewayhub

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCallOfflineReturnsErrOffline(t *testing.T) {
	h := New()
	_, err := h.Call(context.Background(), "unknown-gateway", "readNow", nil, nil)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("Call() err=%v want ErrOffline", err)
	}
}

func TestCallTimesOutWhenNobodyResolves(t *testing.T) {
	h := New()
	out, _, unregister := h.Register("gw-1")
	defer unregister()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { <-out }() // drain the payload so Call's send doesn't block

	_, err := h.Call(ctx, "gw-1", "readNow", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() err=%v want DeadlineExceeded", err)
	}
}

// fakeGateway answers the next command.request on gw-1 with reply, and
// hands back the request frame it saw.
func fakeGateway(t *testing.T, h *Hub, reply string) (<-chan map[string]any, func()) {
	t.Helper()
	out, resolve, unregister := h.Register("gw-1")
	seen := make(chan map[string]any, 1)
	go func() {
		var frame map[string]any
		if err := json.Unmarshal(<-out, &frame); err != nil {
			t.Error(err)
		}
		seen <- frame
		resolve(frame["commandId"].(string), json.RawMessage(reply))
	}()
	return seen, unregister
}

func TestCallBuildsEnvelopeAndReturnsData(t *testing.T) {
	h := New()
	seen, unregister := fakeGateway(t, h, `{"ok":true,"data":{"value":7}}`)
	defer unregister()

	data, err := h.Call(context.Background(), "gw-1", "readNow", map[string]any{"connectionId": 3}, nil)
	if err != nil {
		t.Fatalf("Call() unexpected err=%v", err)
	}
	if string(data) != `{"value":7}` {
		t.Fatalf("Call() data=%s", data)
	}
	frame := <-seen
	if frame["type"] != "command.request" || frame["kind"] != "readNow" || frame["connectionId"] != float64(3) || frame["commandId"] == "" {
		t.Fatalf("request frame=%v", frame)
	}
}

func TestCallRejectedCarriesGatewayReason(t *testing.T) {
	h := New()
	_, unregister := fakeGateway(t, h, `{"ok":false,"error":"modbus timeout"}`)
	defer unregister()

	_, err := h.Call(context.Background(), "gw-1", "readNow", nil, nil)
	var rejected *RejectedError
	if !errors.As(err, &rejected) || rejected.Reason != "modbus timeout" || rejected.Kind != "readNow" {
		t.Fatalf("Call() err=%v want RejectedError with reason", err)
	}
}

func TestPushConfigOfflineReturnsFalse(t *testing.T) {
	h := New()
	if h.PushConfig("unknown-gateway", []byte(`{}`)) {
		t.Fatal("PushConfig() = true for an unregistered gateway, want false")
	}
}

func TestIsOnlineReflectsRegistration(t *testing.T) {
	h := New()
	if h.IsOnline("gw-1") {
		t.Fatal("IsOnline() = true before Register")
	}
	_, _, unregister := h.Register("gw-1")
	if !h.IsOnline("gw-1") {
		t.Fatal("IsOnline() = false after Register")
	}
	unregister()
	if h.IsOnline("gw-1") {
		t.Fatal("IsOnline() = true after unregister")
	}
}

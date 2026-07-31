package gatewayhub

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestRunCommandOfflineReturnsErrOffline(t *testing.T) {
	h := New()
	_, err := h.RunCommand(context.Background(), "unknown-gateway", "cmd-1", []byte(`{}`))
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("RunCommand() err=%v want ErrOffline", err)
	}
}

func TestRunCommandTimesOutWhenNobodyResolves(t *testing.T) {
	h := New()
	out, resolve, unregister := h.Register("gw-1")
	defer unregister()
	_ = resolve

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go func() { <-out }() // drain the payload so RunCommand's send doesn't block

	_, err := h.RunCommand(ctx, "gw-1", "cmd-1", []byte(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunCommand() err=%v want DeadlineExceeded", err)
	}
}

func TestRunCommandResolvesWhenReaderRepliesWithMatchingCommandID(t *testing.T) {
	h := New()
	out, resolve, unregister := h.Register("gw-1")
	defer unregister()

	go func() {
		<-out // pretend to be the gateway receiving the command
		resolve("cmd-1", json.RawMessage(`{"ok":true}`))
	}()

	result, err := h.RunCommand(context.Background(), "gw-1", "cmd-1", []byte(`{}`))
	if err != nil {
		t.Fatalf("RunCommand() unexpected err=%v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Fatalf("RunCommand() result=%s want {\"ok\":true}", result)
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

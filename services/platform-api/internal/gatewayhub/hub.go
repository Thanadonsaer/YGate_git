// Package gatewayhub is the in-process pub/sub that lets an admin HTTP
// request (save config, run a connection test) hand a message to whichever
// goroutine is holding a middleware gateway's live outbound WebSocket
// connection, and get a correlated reply back for on-demand commands.
package gatewayhub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ErrOffline is returned when the target gateway has no live connection.
var ErrOffline = errors.New("gateway is offline")

// RejectedError is a command.result that came back ok:false.
type RejectedError struct {
	Kind   string
	Reason string // the gateway's own error text
}

func (e *RejectedError) Error() string {
	return "gateway rejected " + e.Kind + ": " + e.Reason
}

type conn struct {
	out chan []byte

	mu      sync.Mutex
	pending map[string]pendingCommand
}

type pendingCommand struct {
	result   chan json.RawMessage
	progress func(json.RawMessage)
}

// Hub tracks one live connection per gateway ID (middleware_client.id).
type Hub struct {
	mu    sync.Mutex
	conns map[string]*conn
}

func New() *Hub {
	return &Hub{conns: map[string]*conn{}}
}

// Register attaches gatewayID's live connection to the hub. out yields
// messages to write to the socket; resolve must be called by the read loop
// whenever a command.result arrives; unregister must be called (typically
// deferred) when the connection closes. The out channel is never closed by
// the hub, so callers must select on their own context/connection lifetime
// rather than ranging over it.
func (h *Hub) Register(gatewayID string) (out <-chan []byte, resolve func(commandID string, result json.RawMessage), unregister func()) {
	c := &conn{out: make(chan []byte, 16), pending: map[string]pendingCommand{}}
	h.mu.Lock()
	h.conns[gatewayID] = c
	h.mu.Unlock()

	resolve = func(commandID string, result json.RawMessage) {
		c.mu.Lock()
		pending := c.pending[commandID]
		delete(c.pending, commandID)
		c.mu.Unlock()
		if pending.result != nil {
			pending.result <- result
		}
	}
	unregister = func() {
		h.mu.Lock()
		if h.conns[gatewayID] == c {
			delete(h.conns, gatewayID)
		}
		h.mu.Unlock()
	}
	return c.out, resolve, unregister
}

// Progress forwards an in-flight command update without resolving it.
func (h *Hub) Progress(gatewayID, commandID string, payload json.RawMessage) {
	h.mu.Lock()
	c := h.conns[gatewayID]
	h.mu.Unlock()
	if c == nil {
		return
	}
	c.mu.Lock()
	pending := c.pending[commandID]
	c.mu.Unlock()
	if pending.progress != nil {
		pending.progress(payload)
	}
}

// IsOnline reports whether gatewayID currently has a live connection.
func (h *Hub) IsOnline(gatewayID string) bool {
	h.mu.Lock()
	_, ok := h.conns[gatewayID]
	h.mu.Unlock()
	return ok
}

// PushConfig delivers payload if the gateway is currently connected.
// Returns false when offline or the outbound buffer is full -- the caller
// is responsible for redelivery (e.g. on the gateway's next hello).
func (h *Hub) PushConfig(gatewayID string, payload []byte) bool {
	h.mu.Lock()
	c := h.conns[gatewayID]
	h.mu.Unlock()
	if c == nil {
		return false
	}
	select {
	case c.out <- payload:
		return true
	default:
		return false
	}
}

// Call sends a command.request of kind to gatewayID and waits for its
// command.result, returning the result's data.
//
// fields are merged into the request frame next to type/commandId/kind (for
// example "connectionId", or "data" for commands that nest their arguments);
// progress, when non-nil, receives command.progress frames. It owns the whole
// ADR-0005 envelope so callers never build or parse it:
//   - ErrOffline: no live connection
//   - ctx.Err(): nothing came back in time
//   - *RejectedError: ok:false
func (h *Hub) Call(ctx context.Context, gatewayID, kind string, fields map[string]any, progress func(json.RawMessage)) (json.RawMessage, error) {
	var id [16]byte
	_, _ = rand.Read(id[:])
	commandID := hex.EncodeToString(id[:])
	frame := map[string]any{}
	for k, v := range fields {
		frame[k] = v
	}
	frame["type"], frame["commandId"], frame["kind"] = "command.request", commandID, kind
	payload, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("encode %s command: %w", kind, err)
	}
	raw, err := h.run(ctx, gatewayID, commandID, payload, progress)
	if err != nil {
		return nil, err
	}
	var result struct {
		Ok    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", kind, err)
	}
	if !result.Ok {
		if result.Error == "" {
			result.Error = "no reason given"
		}
		return nil, &RejectedError{Kind: kind, Reason: result.Error}
	}
	return result.Data, nil
}

func (h *Hub) run(ctx context.Context, gatewayID, commandID string, payload []byte, progress func(json.RawMessage)) (json.RawMessage, error) {
	h.mu.Lock()
	c := h.conns[gatewayID]
	h.mu.Unlock()
	if c == nil {
		return nil, ErrOffline
	}
	result := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[commandID] = pendingCommand{result: result, progress: progress}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, commandID)
		c.mu.Unlock()
	}()
	select {
	case c.out <- payload:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r := <-result:
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

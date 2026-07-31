// Package gatewayhub is the in-process pub/sub that lets an admin HTTP
// request (save config, run a connection test) hand a message to whichever
// goroutine is holding a middleware gateway's live outbound WebSocket
// connection, and get a correlated reply back for on-demand commands.
package gatewayhub

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// ErrOffline is returned when the target gateway has no live connection.
var ErrOffline = errors.New("gateway is offline")

type conn struct {
	out chan []byte

	mu      sync.Mutex
	pending map[string]chan json.RawMessage
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
	c := &conn{out: make(chan []byte, 16), pending: map[string]chan json.RawMessage{}}
	h.mu.Lock()
	h.conns[gatewayID] = c
	h.mu.Unlock()

	resolve = func(commandID string, result json.RawMessage) {
		c.mu.Lock()
		ch := c.pending[commandID]
		delete(c.pending, commandID)
		c.mu.Unlock()
		if ch != nil {
			ch <- result
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

// RunCommand sends payload to gatewayID and blocks until a matching
// command.result is resolved, ctx is done, or the gateway is offline.
func (h *Hub) RunCommand(ctx context.Context, gatewayID, commandID string, payload []byte) (json.RawMessage, error) {
	h.mu.Lock()
	c := h.conns[gatewayID]
	h.mu.Unlock()
	if c == nil {
		return nil, ErrOffline
	}
	result := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[commandID] = result
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

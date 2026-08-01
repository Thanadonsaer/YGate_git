// Package realtimeclient is the middleware's outbound, always-initiated-
// from-here connection to the Central Platform's realtime config channel.
// No inbound port is ever opened for this: the middleware dials out, sends
// hello, and then reacts to whatever the platform sends back -- a config
// push to apply, or a connect-test/read-now command to run against a live
// device. This mirrors internal/delivery/worker.go's outbound-auth
// conventions (X-Api-Key header, endpoint/key sourced from GatewayConfig).
package realtimeclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"chpp/modbus-api-middleware/internal/app"
	"chpp/modbus-api-middleware/internal/configcache"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
	"chpp/modbus-api-middleware/internal/updater"
)

type Client struct {
	Store          *store.Store
	Cache          *configcache.Cache
	App            *app.Service
	GatewayID      string
	Endpoint       string
	APIKey         string
	Version        string
	CanApplyUpdate bool
}

func (c *Client) updater() *updater.Manager {
	return &updater.Manager{Version: c.Version, CanApply: c.CanApplyUpdate}
}

// envelope is the generic shape of every message on the channel; the
// embedded ConfigSnapshot flattens brands/deviceSets/connections/plants/
// version directly into the JSON object for config.snapshot messages,
// matching the wire contract exactly.
type envelope struct {
	Type            string          `json:"type"`
	GatewayID       string          `json:"gatewayId,omitempty"`
	AppliedVersion  int64           `json:"appliedVersion,omitempty"`
	SoftwareVersion string          `json:"softwareVersion,omitempty"`
	Status          string          `json:"status,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	CommandID       string          `json:"commandId,omitempty"`
	Kind            string          `json:"kind,omitempty"`
	ConnectionID    int64           `json:"connectionId,omitempty"`
	Ok              bool            `json:"ok,omitempty"`
	Data            json.RawMessage `json:"data,omitempty"`
	Error           string          `json:"error,omitempty"`
	// update.stage command fields -- "patchVersion" not "version", which
	// domain.ConfigSnapshot below already claims for the config version.
	DownloadURL  string `json:"downloadUrl,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	PatchVersion string `json:"patchVersion,omitempty"`
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	Binary       string `json:"binary,omitempty"`
	domain.ConfigSnapshot
}

// Run dials, handles messages until the connection drops or ctx is
// cancelled, then reconnects with exponential backoff (same 1<<min(n,8)
// second curve internal/store's delivery outbox retry already uses).
// Non-blocking: call as `go client.Run(ctx)`.
func (c *Client) Run(ctx context.Context) {
	attempts := 0
	for ctx.Err() == nil {
		connected, err := c.runOnce(ctx)
		if err != nil {
			log.Printf("realtime client: %v", err)
		}
		if connected {
			attempts = 0
		}
		if ctx.Err() != nil {
			return
		}
		attempts++
		wait := time.Duration(1<<min(attempts, 8)) * time.Second
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) runOnce(ctx context.Context) (connected bool, err error) {
	wsURL, err := wsURLFromEndpoint(c.Endpoint)
	if err != nil {
		return false, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-Api-Key": {strings.TrimSpace(c.APIKey)}},
	})
	cancel()
	if err != nil {
		return false, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	conn.SetReadLimit(8 << 20) // 8MiB -- matches platform-api's gateway_realtime.go limit
	defer conn.CloseNow()

	version, err := c.Store.CurrentConfigVersion()
	if err != nil {
		return true, fmt.Errorf("read local config version: %w", err)
	}
	if err = wsjson.Write(ctx, conn, envelope{Type: "hello", GatewayID: c.GatewayID, AppliedVersion: version, SoftwareVersion: c.Version}); err != nil {
		return true, fmt.Errorf("send hello: %w", err)
	}

	for {
		readCtx, readCancel := context.WithTimeout(ctx, 45*time.Second)
		var msg envelope
		readErr := wsjson.Read(readCtx, conn, &msg)
		readCancel()
		if readErr != nil {
			if ctx.Err() != nil {
				return true, nil
			}
			return true, fmt.Errorf("read: %w", readErr)
		}
		switch msg.Type {
		case "config.snapshot":
			c.handleConfigSnapshot(ctx, conn, msg)
		case "command.request":
			c.handleCommand(ctx, conn, msg)
		case "heartbeat":
			// server keepalive; no reply needed, receiving it just resets
			// the per-read deadline above.
		}
	}
}

// handleConfigSnapshot validates+persists via Store.ApplyConfigSnapshot
// (single transaction, reuses the same validators local edits go through)
// and only hot-swaps the cache when that succeeds. The ack sent back
// reflects exactly what config_history recorded -- a cache-rebuild hiccup
// after a successful apply is logged but does not flip the ack to FAILED,
// since the data is already correctly persisted.
func (c *Client) handleConfigSnapshot(ctx context.Context, conn *websocket.Conn, msg envelope) {
	snapshot := msg.ConfigSnapshot
	ack := envelope{Type: "config.ack", AppliedVersion: snapshot.Version}
	if err := c.Store.ApplyConfigSnapshot(snapshot.Version, snapshot); err != nil {
		ack.Status, ack.Reason = "FAILED", err.Error()
	} else {
		ack.Status = "APPLIED"
		if cfg, rebuildErr := configcache.RebuildFromStore(c.Store); rebuildErr != nil {
			log.Printf("realtime client: rebuild cache after applied config v%d: %v", snapshot.Version, rebuildErr)
		} else {
			c.Cache.Swap(cfg)
		}
	}
	if err := wsjson.Write(ctx, conn, ack); err != nil {
		log.Printf("realtime client: send config.ack: %v", err)
	}
}

func (c *Client) handleCommand(ctx context.Context, conn *websocket.Conn, msg envelope) {
	result := envelope{Type: "command.result", CommandID: msg.CommandID}
	switch msg.Kind {
	case "readNow":
		reading, measurements, err := c.App.PollConnection(fmt.Sprint(msg.ConnectionID))
		if err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			data, _ := json.Marshal(map[string]any{"reading": reading, "measurements": measurements})
			result.Ok, result.Data = true, data
		}
	case "connectTest":
		info, latency, connection, err := c.App.ProbeConnection(msg.ConnectionID)
		if err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			data, _ := json.Marshal(map[string]any{"connection": connection, "probe": info, "latencyMs": latency.Milliseconds()})
			result.Ok, result.Data = true, data
		}
	case "config-export":
		cfg := c.Cache.Load()
		snapshot := domain.ConfigSnapshot{Version: cfg.Version, Brands: cfg.Brands, Plants: cfg.Plants}
		for _, ds := range cfg.DeviceSets {
			snapshot.DeviceSets = append(snapshot.DeviceSets, ds)
		}
		for _, conn := range cfg.Connections {
			snapshot.Connections = append(snapshot.Connections, conn)
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			result.Ok, result.Data = true, data
		}
	case "update.stage":
		if err := c.stageUpdate(ctx, msg); err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			result.Ok = true
		}
	case "update.apply":
		if backup, err := c.updater().Apply(); err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			data, _ := json.Marshal(map[string]any{"backup": backup})
			result.Ok, result.Data = true, data
		}
	case "update.rollback":
		if backup, err := c.updater().Rollback(); err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			data, _ := json.Marshal(map[string]any{"backup": backup})
			result.Ok, result.Data = true, data
		}
	case "service.restart":
		if err := c.updater().RestartOnly(); err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			result.Ok = true
		}
	default:
		result.Ok, result.Error = false, "unknown command kind"
	}
	if err := wsjson.Write(ctx, conn, result); err != nil {
		log.Printf("realtime client: send command.result: %v", err)
	}
}

// stageUpdate downloads the patch zip msg.DownloadURL points at (the same
// platform-api host this WS connection is already talking to, authenticated
// the same X-Api-Key way) and hands it to the updater for validation+staging.
func (c *Client) stageUpdate(ctx context.Context, msg envelope) error {
	if msg.DownloadURL == "" {
		return fmt.Errorf("update.stage: downloadUrl is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, msg.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", strings.TrimSpace(c.APIKey))
	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download patch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download patch: unexpected status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return fmt.Errorf("download patch: %w", err)
	}
	_, err = c.updater().StageZip(data)
	return err
}

func wsURLFromEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid gateway endpoint %q", endpoint)
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	return (&url.URL{Scheme: scheme, Host: u.Host, Path: "/api/v1/gateway/realtime"}).String(), nil
}

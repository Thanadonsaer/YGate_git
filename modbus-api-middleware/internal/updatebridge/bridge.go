// Package updatebridge is the deliberately small recovery client used to
// replace an old Middleware binary whose patch downloader is limited to 60s.
// It reads the existing SQLite configuration but never writes to it.
//go:build !mips && !mipsle

package updatebridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	_ "modernc.org/sqlite"

	"chpp/modbus-api-middleware/internal/updater"
)

const StageTimeout = 30 * time.Minute

type GatewayConfig struct {
	GatewayID string
	Endpoint  string
	APIKey    string
	Version   int64
}

type Bridge struct {
	DBPath  string
	Version string
	busy    atomic.Bool
}

type envelope struct {
	Type            string `json:"type"`
	GatewayID       string `json:"gatewayId,omitempty"`
	AppliedVersion  int64  `json:"appliedVersion,omitempty"`
	SoftwareVersion string `json:"softwareVersion,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	Status          string `json:"status,omitempty"`
	Reason          string `json:"reason,omitempty"`
	CommandID       string `json:"commandId,omitempty"`
	Kind            string `json:"kind,omitempty"`
	Ok              bool   `json:"ok,omitempty"`
	Error           string `json:"error,omitempty"`
	Phase           string `json:"phase,omitempty"`
	DownloadedBytes int64  `json:"downloadedBytes,omitempty"`
	TotalBytes      int64  `json:"totalBytes,omitempty"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	json.RawMessage `json:"-"`
}

type DownloadProgress struct {
	DownloadedBytes int64
	TotalBytes      int64
}

func ReadGatewayConfig(path string) (GatewayConfig, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return GatewayConfig{}, err
	}
	defer db.Close()
	var cfg GatewayConfig
	if err = db.QueryRow(`SELECT gateway_id, endpoint, api_key FROM gateway_config WHERE id=1`).Scan(&cfg.GatewayID, &cfg.Endpoint, &cfg.APIKey); err != nil {
		return GatewayConfig{}, fmt.Errorf("read gateway config: %w", err)
	}
	if err = db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM config_history WHERE status='APPLIED'`).Scan(&cfg.Version); err != nil && !strings.Contains(err.Error(), "no such table: config_history") {
		return GatewayConfig{}, fmt.Errorf("read config version: %w", err)
	}
	cfg.GatewayID = strings.TrimSpace(cfg.GatewayID)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg, nil
}

func openReadOnly(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(abs) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func NewStageContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), StageTimeout)
}

func WSURLFromEndpoint(endpoint string) (string, error) {
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

func (b *Bridge) Run(ctx context.Context) error {
	attempts := 0
	for ctx.Err() == nil {
		cfg, err := ReadGatewayConfig(b.DBPath)
		if err == nil && cfg.Endpoint != "" && cfg.APIKey != "" {
			connected, runErr := b.runOnce(ctx, cfg)
			if runErr != nil {
				log.Printf("update bridge: %v", runErr)
			}
			if connected {
				attempts = 0
			}
		} else if err != nil {
			log.Printf("update bridge: %v", err)
		}
		attempts++
		wait := time.Duration(1<<min(attempts, 6)) * time.Second
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func (b *Bridge) runOnce(ctx context.Context, cfg GatewayConfig) (bool, error) {
	wsURL, err := WSURLFromEndpoint(cfg.Endpoint)
	if err != nil {
		return false, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"X-Api-Key": {cfg.APIKey}}})
	cancel()
	if err != nil {
		return false, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	writer := &socketWriter{conn: conn}
	if err = writer.write(ctx, envelope{Type: "hello", GatewayID: cfg.GatewayID, AppliedVersion: cfg.Version, SoftwareVersion: b.Version, Capabilities: []string{"update-bridge"}}); err != nil {
		return true, fmt.Errorf("send hello: %w", err)
	}

	for {
		readCtx, readCancel := context.WithTimeout(ctx, 45*time.Second)
		var msg envelope
		err = wsjson.Read(readCtx, conn, &msg)
		readCancel()
		if err != nil {
			if ctx.Err() != nil {
				return true, nil
			}
			return true, fmt.Errorf("read: %w", err)
		}
		switch msg.Type {
		case "heartbeat":
		case "command.request":
			b.handleCommand(ctx, writer, cfg.APIKey, msg)
		case "config.snapshot":
			// The bridge intentionally never writes the user's configuration.
		}
	}
}

func (b *Bridge) handleCommand(ctx context.Context, writer *socketWriter, apiKey string, msg envelope) {
	if msg.Kind == "update.stage" {
		if !b.busy.CompareAndSwap(false, true) {
			b.writeResult(writer, envelope{Type: "command.result", CommandID: msg.CommandID, Error: "update bridge is already staging another patch"})
			return
		}
		go func() {
			defer b.busy.Store(false)
			stageCtx, cancel := NewStageContext(ctx)
			defer cancel()
			err := b.stage(stageCtx, apiKey, msg, func(p DownloadProgress) {
				b.writeResult(writer, envelope{Type: "command.progress", CommandID: msg.CommandID, Phase: "download", DownloadedBytes: p.DownloadedBytes, TotalBytes: p.TotalBytes})
			})
			result := envelope{Type: "command.result", CommandID: msg.CommandID, Ok: err == nil}
			if err != nil {
				result.Error = err.Error()
			}
			b.writeResult(writer, result)
		}()
		return
	}
	if msg.Kind == "update.apply" {
		if b.busy.Load() {
			b.writeResult(writer, envelope{Type: "command.result", CommandID: msg.CommandID, Error: "update bridge is still staging a patch"})
			return
		}
		_, err := (&updater.Manager{Version: b.Version, CanApply: true}).Apply()
		result := envelope{Type: "command.result", CommandID: msg.CommandID, Ok: err == nil}
		if err != nil {
			result.Error = err.Error()
		}
		b.writeResult(writer, result)
		return
	}
	b.writeResult(writer, envelope{Type: "command.result", CommandID: msg.CommandID, Error: "update bridge supports only update.stage and update.apply"})
}

func (b *Bridge) stage(ctx context.Context, apiKey string, msg envelope, report func(DownloadProgress)) error {
	data, err := downloadPatch(ctx, msg.DownloadURL, apiKey, report)
	if err != nil {
		return err
	}
	_, err = (&updater.Manager{Version: b.Version, CanApply: true}).StageZip(data)
	return err
}

func downloadPatch(ctx context.Context, downloadURL, apiKey string, report func(DownloadProgress)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", strings.TrimSpace(apiKey))
	resp, err := (&http.Client{Timeout: StageTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("download patch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download patch: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	reader := &progressReader{reader: io.LimitReader(resp.Body, 128<<20), total: resp.ContentLength, report: report}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("download patch: %w", err)
	}
	return data, nil
}

type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	last   int64
	lastAt time.Time
	report func(DownloadProgress)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.report != nil && (r.last == 0 || r.read-r.last >= 256<<10 || time.Since(r.lastAt) >= 250*time.Millisecond || err == io.EOF) {
			r.last, r.lastAt = r.read, time.Now()
			r.report(DownloadProgress{DownloadedBytes: r.read, TotalBytes: r.total})
		}
	}
	return n, err
}

type socketWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *socketWriter) write(parent context.Context, value envelope) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	w.mu.Lock()
	defer w.mu.Unlock()
	return wsjson.Write(ctx, w.conn, value)
}

func (b *Bridge) writeResult(writer *socketWriter, result envelope) {
	if err := writer.write(context.Background(), result); err != nil {
		log.Printf("update bridge: send %s: %v", result.Type, err)
	}
}

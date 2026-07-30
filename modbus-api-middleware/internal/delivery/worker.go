package delivery

import (
	"bytes"
	"chpp/modbus-api-middleware/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Worker struct {
	Store            *store.Store
	Endpoint, APIKey string
	BatchSize        int
	Client           *http.Client
	BeforeSend       func() error
}

func (w *Worker) SendOnce() (int, error) {
	endpoint, apiKey, timeout, enabled, err := w.config()
	if err != nil || endpoint == "" || apiKey == "" {
		return 0, err
	}
	if !enabled {
		return 0, nil
	}
	if w.BeforeSend != nil {
		if err := w.BeforeSend(); err != nil {
			return 0, err
		}
	}
	events, err := w.Store.Ready(w.BatchSize)
	if err != nil || len(events) == 0 {
		return 0, err
	}
	data := make([]json.RawMessage, len(events))
	ids := make([]int64, len(events))
	for i, e := range events {
		data[i] = json.RawMessage(e.Payload)
		ids[i] = e.ID
	}
	body, _ := json.Marshal(map[string]any{"schemaVersion": "2.0", "data": data})
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	client := w.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		_ = w.Store.Failed(ids, 0, err.Error(), true)
		return 0, err
	}
	defer resp.Body.Close()
	message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return len(ids), w.Store.DeliveredWithResponse(ids, resp.StatusCode, string(message))
	}
	retry := resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500
	_ = w.Store.Failed(ids, resp.StatusCode, string(message), retry)
	return 0, fmt.Errorf("API returned %d", resp.StatusCode)
}

func (w *Worker) Interval() time.Duration {
	seconds := 5
	if w.Store != nil {
		cfg, err := w.Store.GatewayConfig()
		if err == nil && cfg.SendIntervalSeconds > 0 {
			seconds = cfg.SendIntervalSeconds
		}
	}
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

func (w *Worker) config() (string, string, time.Duration, bool, error) {
	endpoint := strings.TrimSpace(w.Endpoint)
	apiKey := strings.TrimSpace(w.APIKey)
	timeoutSeconds := 10
	enabled := endpoint != "" || apiKey != ""
	if (endpoint == "" || apiKey == "") && w.Store != nil {
		cfg, err := w.Store.GatewayConfig()
		if err != nil {
			return "", "", 0, false, err
		}
		if endpoint == "" {
			endpoint = cfg.Endpoint
		}
		if apiKey == "" {
			apiKey = cfg.APIKey
		}
		timeoutSeconds = cfg.SendTimeoutSeconds
		enabled = cfg.APIPollingEnabled
	}
	if timeoutSeconds < 1 {
		timeoutSeconds = 10
	}
	return strings.TrimSpace(endpoint), strings.TrimSpace(apiKey), time.Duration(timeoutSeconds) * time.Second, enabled, nil
}

package web

import (
	"bytes"
	"chpp/modbus-api-middleware/internal/app"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/modbus"
	"chpp/modbus-api-middleware/internal/store"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed static/index.html
var files embed.FS

type Server struct {
	Store            *store.Store
	App              *app.Service
	GatewayID        string
	Version          string
	CanApplyUpdate   bool
	UpdateRoot       string
	LicensePublicKey string
	LicenseFile      string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/connect-test/", s.connectTest)
	mux.HandleFunc("/api/read-now/", s.readNow)
	mux.HandleFunc("/api/send-api-once/", s.sendAPIOnce)
	static, _ := fs.Sub(files, "static")
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) connectTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	connection, err := s.connectionFromPath(r.URL.Path, "/api/connect-test/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	client := s.App.Client
	if client == nil {
		client = &modbus.Client{Timeout: 3 * time.Second}
	}
	info, latency, err := client.ProbeDetails(connection.Host, connection.Port, connection.UnitID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": connection, "probe": info, "latencyMs": latency.Milliseconds()})
}

func (s *Server) readNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idText := strings.TrimPrefix(r.URL.Path, "/api/read-now/")
	reading, measurements, err := s.App.PollConnection(idText)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reading": reading, "measurements": measurements, "queued": false, "sent": false})
}

func (s *Server) sendAPIOnce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	connection, err := s.connectionFromPath(r.URL.Path, "/api/send-api-once/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	reading, measurements, err := s.App.PollConnection(fmt.Sprint(connection.ConnectionID))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	cfg, err := s.Store.GatewayConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("API endpoint and API key are required"))
		return
	}
	gatewayID := strings.TrimSpace(s.GatewayID)
	if strings.TrimSpace(cfg.GatewayID) != "" {
		gatewayID = strings.TrimSpace(cfg.GatewayID)
	}
	reading, _, err = app.PrepareReadingForAPI(reading, gatewayID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, response, err := sendReadingOnce(cfg.Endpoint, cfg.APIKey, cfg.SendTimeoutSeconds, reading)
	detail, _ := json.Marshal(map[string]any{"reading": reading, "measurements": measurements, "httpStatus": status, "response": response})
	if err != nil {
		s.App.LogPoll(connection.ConnectionID, connection.ConnectionName, "ERROR", err.Error(), string(detail))
		writeError(w, http.StatusBadGateway, err)
		return
	}
	s.App.LogPoll(connection.ConnectionID, connection.ConnectionName, "OK", fmt.Sprintf("sent API once: HTTP %d", status), string(detail))
	writeJSON(w, http.StatusOK, map[string]any{"reading": reading, "measurements": measurements, "sent": true, "httpStatus": status, "response": response})
}

func sendReadingOnce(endpoint, apiKey string, timeoutSeconds int, reading domain.Reading) (int, string, error) {
	if timeoutSeconds < 1 {
		timeoutSeconds = 10
	}
	body, _ := json.Marshal(map[string]any{"schemaVersion": "2.0", "data": []domain.Reading{reading}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(endpoint), bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", strings.TrimSpace(apiKey))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, string(message), fmt.Errorf("API returned %d", resp.StatusCode)
	}
	return resp.StatusCode, string(message), nil
}

func (s *Server) connectionFromPath(path, prefix string) (domain.ConnectionConfig, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(path, prefix)), 10, 64)
	if err != nil || id <= 0 {
		return domain.ConnectionConfig{}, fmt.Errorf("invalid connection id")
	}
	return s.Store.Connection(id)
}

package app

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
)

func (s *Service) PollEnabledConnections(gatewayID string, logf func(string, ...any)) error {
	if logf == nil {
		logf = log.Printf
	}
	connections := []domain.ConnectionConfig{}
	for _, c := range s.Cache.Load().Connections {
		if c.Enabled {
			connections = append(connections, c)
		}
	}
	// GatewayConfig (endpoint/API key/interval) intentionally stays
	// SQLite-sourced, not cached: it is how the middleware finds and
	// authenticates to the platform in the first place, so pushing it
	// centrally would be circular.
	cfg, err := s.Store.GatewayConfig()
	if err != nil {
		return err
	}
	gw := strings.TrimSpace(gatewayID)
	if strings.TrimSpace(cfg.GatewayID) != "" {
		gw = strings.TrimSpace(cfg.GatewayID)
	}
	readOK, failed, queued := 0, 0, 0
	for _, c := range connections {
		reading, measurements, err := s.PollConnection(fmt.Sprint(c.ConnectionID))
		if err != nil {
			failed++
			detail, _ := json.Marshal(map[string]any{"pollError": err.Error()})
			s.logPoll(c.ConnectionID, c.ConnectionName, "ERROR", err.Error(), string(detail))
			logf("poll %s: %v", c.ConnectionName, err)
			continue
		}
		readOK++
		created, err := s.Enqueue(reading, gw)
		if err != nil {
			failed++
			detail, _ := json.Marshal(map[string]any{"reading": reading, "measurements": measurements})
			s.logPoll(c.ConnectionID, c.ConnectionName, "ERROR", err.Error(), string(detail))
			logf("enqueue %s: %v", c.ConnectionName, err)
			continue
		}
		if created {
			queued++
		}
		// Success detail is deliberately empty: the reading itself is already
		// in outbox_events.payload_json, and copying it into poll_logs too
		// doubled the SQLite footprint of every poll for a row nobody reads
		// unless something failed.
		s.logPoll(c.ConnectionID, c.ConnectionName, "OK", "poll queued", "")
	}
	logf("poll sweep: %d connection(s), %d read OK, %d failed, %d queued", len(connections), readOK, failed, queued)
	return nil
}

func (s *Service) LogPoll(connectionID int64, name, status, message, detail string) {
	s.logPoll(connectionID, name, status, message, detail)
}
func (s *Service) logPoll(connectionID int64, name, status, message, detail string) {
	_ = s.Store.SavePollLog(domain.PollLog{ConnectionID: connectionID, ConnectionName: name, Status: status, Message: message, Detail: detail})
}

func (s *Service) PollInterval() time.Duration {
	seconds := 5
	if s.Store != nil {
		if cfg, err := s.Store.GatewayConfig(); err == nil && cfg.SendIntervalSeconds > 0 {
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

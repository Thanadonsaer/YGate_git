//go:build !mips && !mipsle

package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
)

const gatewayConfigSchema = `
CREATE TABLE IF NOT EXISTS gateway_config (
 id INTEGER PRIMARY KEY CHECK(id=1),
 gateway_id TEXT NOT NULL DEFAULT '',
 endpoint TEXT NOT NULL DEFAULT '',
 api_key TEXT NOT NULL DEFAULT '',
 api_polling_enabled INTEGER NOT NULL DEFAULT 0,
 send_interval_seconds INTEGER NOT NULL DEFAULT 5,
 send_timeout_seconds INTEGER NOT NULL DEFAULT 10,
 idle_heartbeat_seconds INTEGER NOT NULL DEFAULT 1800
);`

func (s *Store) ensureGatewayConfig() error {
	if _, err := s.DB.Exec(gatewayConfigSchema); err != nil {
		return err
	}
	if err := s.ensureColumn("gateway_config", "send_interval_seconds", "INTEGER NOT NULL DEFAULT 5"); err != nil {
		return err
	}
	if err := s.ensureColumn("gateway_config", "api_polling_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("gateway_config", "send_timeout_seconds", "INTEGER NOT NULL DEFAULT 10"); err != nil {
		return err
	}
	return s.ensureColumn("gateway_config", "idle_heartbeat_seconds", "INTEGER NOT NULL DEFAULT 1800")
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	rows, err := s.DB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err = rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) ensureColumn(table, column, definition string) error {
	ok, err := s.hasColumn(table, column)
	if err != nil || ok {
		return err
	}
	_, err = s.DB.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

func (s *Store) GatewayConfig() (domain.GatewayConfig, error) {
	var v domain.GatewayConfig
	var apiPollingEnabled int
	if err := s.ensureGatewayConfig(); err != nil {
		return v, err
	}
	err := s.DB.QueryRow("SELECT gateway_id,endpoint,api_key,api_polling_enabled,send_interval_seconds,send_timeout_seconds,idle_heartbeat_seconds FROM gateway_config WHERE id=1").Scan(&v.GatewayID, &v.Endpoint, &v.APIKey, &apiPollingEnabled, &v.SendIntervalSeconds, &v.SendTimeoutSeconds, &v.IdleHeartbeatSeconds)
	v.APIPollingEnabled = apiPollingEnabled != 0
	if err == sql.ErrNoRows {
		return normalizeGatewayConfig(v), nil
	}
	return normalizeGatewayConfig(v), err
}

func (s *Store) SaveGatewayConfig(v domain.GatewayConfig) (domain.GatewayConfig, error) {
	v = normalizeGatewayConfig(v)
	if v.Endpoint != "" {
		u, err := url.ParseRequestURI(v.Endpoint)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return v, fmt.Errorf("endpoint must be http(s) URL")
		}
	}
	if err := s.ensureGatewayConfig(); err != nil {
		return v, err
	}
	apiPollingEnabled := 0
	if v.APIPollingEnabled {
		apiPollingEnabled = 1
	}
	_, err := s.DB.Exec(`INSERT INTO gateway_config(id,gateway_id,endpoint,api_key,api_polling_enabled,send_interval_seconds,send_timeout_seconds,idle_heartbeat_seconds) VALUES(1,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET gateway_id=excluded.gateway_id, endpoint=excluded.endpoint, api_key=excluded.api_key, api_polling_enabled=excluded.api_polling_enabled, send_interval_seconds=excluded.send_interval_seconds, send_timeout_seconds=excluded.send_timeout_seconds, idle_heartbeat_seconds=excluded.idle_heartbeat_seconds`, v.GatewayID, v.Endpoint, v.APIKey, apiPollingEnabled, v.SendIntervalSeconds, v.SendTimeoutSeconds, v.IdleHeartbeatSeconds)
	return v, err
}

func normalizeGatewayConfig(v domain.GatewayConfig) domain.GatewayConfig {
	v.GatewayID = strings.TrimSpace(v.GatewayID)
	v.Endpoint = strings.TrimSpace(v.Endpoint)
	v.APIKey = strings.TrimSpace(v.APIKey)
	if v.SendIntervalSeconds < 1 {
		v.SendIntervalSeconds = 5
	}
	if v.SendIntervalSeconds > 3600 {
		v.SendIntervalSeconds = 3600
	}
	if v.SendTimeoutSeconds < 1 {
		v.SendTimeoutSeconds = 10
	}
	if v.SendTimeoutSeconds > 300 {
		v.SendTimeoutSeconds = 300
	}
	// Floor of a minute: anything shorter defeats the point of skipping
	// unchanged readings at all. Ceiling of a day so a typo can never leave a
	// dead device looking alive for a week.
	if v.IdleHeartbeatSeconds < 60 {
		v.IdleHeartbeatSeconds = 1800
	}
	if v.IdleHeartbeatSeconds > 86400 {
		v.IdleHeartbeatSeconds = 86400
	}
	return v
}

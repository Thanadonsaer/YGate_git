package store

import (
	"chpp/modbus-api-middleware/internal/domain"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
)

type Store struct{ DB *sql.DB }
type OutboxEvent struct {
	ID       int64
	Payload  string
	Attempts int
}

const schema = `
CREATE TABLE IF NOT EXISTS profiles (id TEXT PRIMARY KEY, config TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS devices (id TEXT PRIMARY KEY, config TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS outbox_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, idempotency_key TEXT NOT NULL UNIQUE, payload_hash TEXT NOT NULL,
 payload_json TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING', attempt_count INTEGER NOT NULL DEFAULT 0,
 next_retry_at INTEGER NOT NULL DEFAULT 0, last_http_status INTEGER, last_error TEXT, last_response TEXT, created_at INTEGER NOT NULL, delivered_at INTEGER
);
CREATE INDEX IF NOT EXISTS outbox_ready ON outbox_events(status,next_retry_at);`

// Open connects to the SQLite file at path.
//
// The DSN pragmas and the single-connection pool are what stop the
// intermittent "database is locked" (SQLITE_BUSY) errors in the delivery logs.
// Three separate writers share this file -- the poll loop enqueueing readings,
// the delivery loop marking them DELIVERED/RETRYING, and the local web UI
// editing configuration -- while database/sql was free to open a connection
// per concurrent caller. SQLite allows only one writer at a time, so any
// overlap failed immediately instead of waiting:
//
//   - journal_mode=WAL lets readers run while a write is in progress, rather
//     than taking the rollback journal's whole-file lock. Readers stop
//     blocking writers entirely.
//   - busy_timeout=5000 makes a blocked writer wait up to 5s for the write
//     lock instead of returning SQLITE_BUSY on the spot. Without it WAL alone
//     still surfaces every collision as an error.
//   - _txlock=immediate takes the write lock when a transaction begins rather
//     than when it first writes. Under WAL, a deferred transaction that reads
//     and then upgrades to a write fails with SQLITE_BUSY_SNAPSHOT, and that
//     one is NOT retried by busy_timeout -- it has to be retried by the
//     application, which none of the s.DB.Begin() callers do.
//
// Deliberately NOT SetMaxOpenConns(1): several readers (Store.Brands,
// Store.DeviceSets) issue a nested query while their outer rows are still
// open, so a one-connection pool deadlocks the moment the inner query waits
// for the connection the outer loop is holding.
//
// modernc.org/sqlite strips a `?...` query from a non-"file:" DSN and applies
// these settings, so this stays correct for the plain Windows paths main.go
// passes (a file: URI would mangle the backslashes).
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db}
	if err = s.ensureColumn("outbox_events", "last_response", "TEXT"); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func saveJSON(db *sql.DB, table, id string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("INSERT INTO %s(id,config) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET config=excluded.config", table), id, string(b))
	return err
}
func (s *Store) SaveProfile(v domain.RegisterProfile) error {
	return saveJSON(s.DB, "profiles", v.ProfileID, v)
}
func (s *Store) SaveDevice(v domain.Device) error { return saveJSON(s.DB, "devices", v.DeviceID, v) }
func loadJSON[T any](db *sql.DB, table, id string) (T, error) {
	var z T
	var b string
	err := db.QueryRow(fmt.Sprintf("SELECT config FROM %s WHERE id=?", table), id).Scan(&b)
	if err != nil {
		return z, err
	}
	err = json.Unmarshal([]byte(b), &z)
	return z, err
}
func (s *Store) Profile(id string) (domain.RegisterProfile, error) {
	return loadJSON[domain.RegisterProfile](s.DB, "profiles", id)
}
func (s *Store) Device(id string) (domain.Device, error) {
	return loadJSON[domain.Device](s.DB, "devices", id)
}
func (s *Store) Enqueue(key, hash string, reading domain.Reading) (bool, error) {
	b, err := json.Marshal(reading)
	if err != nil {
		return false, err
	}
	r, err := s.DB.Exec("INSERT OR IGNORE INTO outbox_events(idempotency_key,payload_hash,payload_json,created_at) VALUES(?,?,?,?)", key, hash, string(b), time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n == 1, nil
}
func (s *Store) Ready(limit int) ([]OutboxEvent, error) {
	rows, err := s.DB.Query("SELECT id,payload_json,attempt_count FROM outbox_events WHERE status IN ('PENDING','RETRYING') AND next_retry_at<=? ORDER BY id LIMIT ?", time.Now().UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		if err = rows.Scan(&e.ID, &e.Payload, &e.Attempts); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) Delivered(ids []int64) error {
	return s.DeliveredWithResponse(ids, 0, "")
}

func (s *Store) DeliveredWithResponse(ids []int64, status int, response string) error {
	for _, id := range ids {
		if _, err := s.DB.Exec("UPDATE outbox_events SET status='DELIVERED',delivered_at=?,last_http_status=?,last_error='',last_response=? WHERE id=?", time.Now().UnixMilli(), status, response, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) Failed(ids []int64, status int, message string, retry bool) error {
	for _, id := range ids {
		state := "DEAD_LETTER"
		next := int64(0)
		if retry {
			state = "RETRYING"
			var attempts int
			_ = s.DB.QueryRow("SELECT attempt_count FROM outbox_events WHERE id=?", id).Scan(&attempts)
			delay := time.Second * time.Duration(1<<min(attempts, 8))
			next = time.Now().Add(delay).UnixMilli()
		}
		if _, err := s.DB.Exec("UPDATE outbox_events SET status=?,attempt_count=attempt_count+1,next_retry_at=?,last_http_status=?,last_error=?,last_response=? WHERE id=?", state, next, status, message, message, id); err != nil {
			return err
		}
	}
	return nil
}

//go:build !mips && !mipsle

package store

import (
	"time"

	"chpp/modbus-api-middleware/internal/domain"
)

func (s *Store) ensurePollLogs() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS poll_logs(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 connection_id INTEGER NOT NULL,
 connection_name TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL,
 message TEXT NOT NULL DEFAULT '',
 detail TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS poll_logs_connection ON poll_logs(connection_id, created_at DESC);`)
	return err
}

func (s *Store) SavePollLog(v domain.PollLog) error {
	if err := s.ensurePollLogs(); err != nil {
		return err
	}
	if v.CreatedAt == 0 {
		v.CreatedAt = time.Now().UnixMilli()
	}
	_, err := s.DB.Exec(`INSERT INTO poll_logs(connection_id,connection_name,status,message,detail,created_at) VALUES(?,?,?,?,?,?)`, v.ConnectionID, v.ConnectionName, v.Status, v.Message, v.Detail, v.CreatedAt)
	return err
}

func (s *Store) PollLogs(connectionID int64, limit int) ([]domain.PollLog, error) {
	if err := s.ensurePollLogs(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT id,connection_id,connection_name,status,message,detail,created_at FROM poll_logs WHERE connection_id=? ORDER BY created_at DESC LIMIT ?`, connectionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PollLog{}
	for rows.Next() {
		var v domain.PollLog
		if err = rows.Scan(&v.ID, &v.ConnectionID, &v.ConnectionName, &v.Status, &v.Message, &v.Detail, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

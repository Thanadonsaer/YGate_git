//go:build !mips && !mipsle

package store

import (
	"database/sql"

	"chpp/modbus-api-middleware/internal/domain"
)

func (s *Store) DeliveryLogs(limit int) ([]domain.DeliveryLog, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(`SELECT id,idempotency_key,status,attempt_count,last_http_status,last_error,last_response,created_at,delivered_at
FROM outbox_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.DeliveryLog{}
	for rows.Next() {
		var v domain.DeliveryLog
		var status, delivered sql.NullInt64
		var lastError, lastResponse sql.NullString
		if err = rows.Scan(&v.ID, &v.IdempotencyKey, &v.Status, &v.Attempts, &status, &lastError, &lastResponse, &v.CreatedAt, &delivered); err != nil {
			return nil, err
		}
		if status.Valid {
			v.LastHTTPStatus = int(status.Int64)
		}
		if lastError.Valid {
			v.LastError = lastError.String
		}
		if lastResponse.Valid {
			v.LastResponse = lastResponse.String
		}
		if delivered.Valid {
			v.DeliveredAt = delivered.Int64
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) ClearDeliveryLogs() (int64, error) {
	r, err := s.DB.Exec("DELETE FROM outbox_events WHERE status IN ('DELIVERED','DEAD_LETTER')")
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

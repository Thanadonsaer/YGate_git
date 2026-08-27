//go:build !mips && !mipsle

package store

import "time"

func (s *Store) CleanupOld(retention time.Duration) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention).UnixMilli()
	var total int64
	for _, q := range []string{
		"DELETE FROM outbox_events WHERE status='DELIVERED' AND COALESCE(delivered_at,created_at) < ?",
		"DELETE FROM outbox_events WHERE status='DEAD_LETTER' AND created_at < ?",
	} {
		r, err := s.DB.Exec(q, cutoff)
		if err != nil {
			return total, err
		}
		n, _ := r.RowsAffected()
		total += n
	}
	if err := s.ensurePollLogs(); err != nil {
		return total, err
	}
	r, err := s.DB.Exec("DELETE FROM poll_logs WHERE created_at < ?", cutoff)
	if err != nil {
		return total, err
	}
	n, _ := r.RowsAffected()
	return total + n, nil
}

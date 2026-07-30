package store

import (
	"path/filepath"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
)

func TestCleanupOldKeepsPendingQueue(t *testing.T) {
	s, err := OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old := time.Now().Add(-48 * time.Hour).UnixMilli()
	newer := time.Now().UnixMilli()
	for _, row := range []struct {
		key, status string
		created     int64
		delivered   any
	}{
		{"delivered-old", "DELIVERED", old, old},
		{"dead-old", "DEAD_LETTER", old, nil},
		{"pending-old", "PENDING", old, nil},
		{"delivered-new", "DELIVERED", newer, newer},
	} {
		_, err = s.DB.Exec(`INSERT INTO outbox_events(idempotency_key,payload_hash,payload_json,status,created_at,delivered_at) VALUES(?,?,?,?,?,?)`, row.key, "h", `{}`, row.status, row.created, row.delivered)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SavePollLog(domain.PollLog{ConnectionID: 1, Status: "OK", CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.CleanupOld(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 3 {
		t.Fatalf("deleted=%d, want 3", deleted)
	}
	var count int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM outbox_events WHERE idempotency_key='pending-old'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("pending removed: count=%d err=%v", count, err)
	}
}

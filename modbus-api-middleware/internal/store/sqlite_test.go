package store

import (
	"chpp/modbus-api-middleware/internal/domain"
	"path/filepath"
	"testing"
)

func TestOutboxIdempotencySurvivesOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r := rawTestReading()
	created, err := s.Enqueue("same", "hash", r)
	if err != nil || !created {
		t.Fatalf("first=%v,%v", created, err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err = s.Enqueue("same", "hash", r)
	if err != nil || created {
		t.Fatalf("duplicate=%v,%v", created, err)
	}
}

func TestClearDeliveryLogsKeepsPending(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := rawTestReading()
	_, _ = s.Enqueue("pending", "hash-pending", r)
	_, _ = s.Enqueue("done", "hash-done", r)
	if err = s.DeliveredWithResponse([]int64{2}, 200, "ok"); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.ClearDeliveryLogs()
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	ready, err := s.Ready(10)
	if err != nil || len(ready) != 1 || ready[0].ID != 1 {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
}

func rawTestReading() domain.Reading {
	return domain.Reading{GatewayID: "G1", DevDn: "D1", PlantCode: "P1", DevTypeID: 1, CollectTime: 1, RegisterAddressMap: map[string]float64{"1": 1}}
}

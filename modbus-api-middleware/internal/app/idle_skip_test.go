package app

import (
	"path/filepath"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

const testGateway = "GW-01"

func idleTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "idle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Service{Store: st}
}

func reading(at time.Time, values map[string]float64) domain.Reading {
	return domain.Reading{
		DevDn: "dev-1", DevName: "Inverter 1", PlantCode: "P1", PlantName: "Plant 1",
		DevTypeID: 1, Model: "M1", CollectTime: at.UnixMilli(), RegisterAddressMap: values,
	}
}

// The whole point of the change: a device sitting at a frozen value stops
// producing rows, but not forever -- one heartbeat still gets through so the
// platform can tell "idle" from "gone".
func TestEnqueueSkipsUnchangedReadingsUntilHeartbeatIsDue(t *testing.T) {
	svc := idleTestService(t)
	start := time.Now().Truncate(time.Second)
	frozen := map[string]float64{"40001": 0, "40002": 0}

	created, err := svc.Enqueue(reading(start, frozen), testGateway)
	if err != nil || !created {
		t.Fatalf("first reading: created=%v err=%v, want stored", created, err)
	}

	// Half an hour of identical polls: nothing new is worth storing.
	for minute := 5; minute < 30; minute += 5 {
		created, err = svc.Enqueue(reading(start.Add(time.Duration(minute)*time.Minute), frozen), testGateway)
		if err != nil {
			t.Fatal(err)
		}
		if created {
			t.Fatalf("reading at +%dm was stored, want skipped as unchanged", minute)
		}
	}

	// At the heartbeat boundary one identical reading goes through anyway.
	created, err = svc.Enqueue(reading(start.Add(DefaultIdleHeartbeat), frozen), testGateway)
	if err != nil || !created {
		t.Fatalf("heartbeat reading: created=%v err=%v, want stored", created, err)
	}

	// ...and the clock restarts from the heartbeat, not from the first reading.
	created, err = svc.Enqueue(reading(start.Add(DefaultIdleHeartbeat+5*time.Minute), frozen), testGateway)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("reading 5m after the heartbeat was stored, want skipped as unchanged")
	}
}

func TestEnqueueStoresAsSoonAsAValueMoves(t *testing.T) {
	svc := idleTestService(t)
	start := time.Now().Truncate(time.Second)

	if created, err := svc.Enqueue(reading(start, map[string]float64{"40001": 0}), testGateway); err != nil || !created {
		t.Fatalf("first reading: created=%v err=%v", created, err)
	}
	if created, err := svc.Enqueue(reading(start.Add(5*time.Minute), map[string]float64{"40001": 0}), testGateway); err != nil || created {
		t.Fatalf("unchanged reading: created=%v err=%v, want skipped", created, err)
	}
	// A single register moving is enough, well inside the heartbeat window.
	if created, err := svc.Enqueue(reading(start.Add(10*time.Minute), map[string]float64{"40001": 1.5}), testGateway); err != nil || !created {
		t.Fatalf("changed reading: created=%v err=%v, want stored", created, err)
	}
	// Back to the value it held 15 minutes ago: still a change from the last
	// reading stored, so it must not be mistaken for the old baseline.
	if created, err := svc.Enqueue(reading(start.Add(15*time.Minute), map[string]float64{"40001": 0}), testGateway); err != nil || !created {
		t.Fatalf("reverted reading: created=%v err=%v, want stored", created, err)
	}
}

func TestEnqueueTracksEachDeviceSeparately(t *testing.T) {
	svc := idleTestService(t)
	start := time.Now().Truncate(time.Second)
	frozen := map[string]float64{"40001": 0}

	first := reading(start, frozen)
	second := reading(start, frozen)
	second.DevDn = "dev-2"

	for _, r := range []domain.Reading{first, second} {
		if created, err := svc.Enqueue(r, testGateway); err != nil || !created {
			t.Fatalf("%s first reading: created=%v err=%v", r.DevDn, created, err)
		}
	}
	// dev-2 having identical values must not suppress dev-1's own baseline,
	// and vice versa.
	for _, r := range []domain.Reading{first, second} {
		later := r
		later.CollectTime = start.Add(5 * time.Minute).UnixMilli()
		if created, err := svc.Enqueue(later, testGateway); err != nil || created {
			t.Fatalf("%s unchanged reading: created=%v err=%v, want skipped", r.DevDn, created, err)
		}
	}
}

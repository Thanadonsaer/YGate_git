package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

func TestMaintenancePreventsNewPollSweepUntilReleased(t *testing.T) {
	svc := &Service{}
	release, err := svc.BeginMaintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if svc.tryBeginPoll() {
		t.Fatal("poll started while middleware is in maintenance mode")
	}
	release()
	if !svc.tryBeginPoll() {
		t.Fatal("poll did not resume after maintenance mode ended")
	}
	svc.endPoll()
}

func TestPollIntervalUsesSavedGatewayConfigClampedTo1And3600Seconds(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.SaveGatewayConfig(domain.GatewayConfig{SendIntervalSeconds: 45}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st}
	if got := svc.PollInterval(); got != 45*time.Second {
		t.Fatalf("PollInterval() = %v, want 45s", got)
	}
}

func TestPollIntervalDefaultsTo5SecondsWhenUnset(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &Service{Store: st}
	if got := svc.PollInterval(); got != 5*time.Second {
		t.Fatalf("PollInterval() = %v, want 5s", got)
	}
}

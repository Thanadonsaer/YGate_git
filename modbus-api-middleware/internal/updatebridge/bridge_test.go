package updatebridge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

func TestReadGatewayConfigIsReadOnlyAndKeepsExistingValues(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "middleware.db")
	st, err := store.OpenNormalized(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.GatewayConfig{GatewayID: "plant-01", Endpoint: "https://ygate-api.example.com/api/v2/ingestion/register-readings", APIKey: "secret-key"}
	if _, err = st.SaveGatewayConfig(want); err != nil {
		st.Close()
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	before := fileHash(t, dbPath)

	got, err := ReadGatewayConfig(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	after := fileHash(t, dbPath)
	if got.GatewayID != want.GatewayID || got.Endpoint != want.Endpoint || got.APIKey != want.APIKey {
		t.Fatalf("config = %+v, want gateway/endpoint/key from existing database", got)
	}
	if before != after {
		t.Fatal("read-only bridge changed middleware.db")
	}
}

func TestStageContextSurvivesRealtimeDisconnect(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	stageCtx, stop := NewStageContext(parent)
	defer stop()
	cancel()
	if err := stageCtx.Err(); err != nil {
		t.Fatalf("stage context canceled with realtime context: %v", err)
	}
}

func TestWSURLUsesGatewayHostAndRealtimePath(t *testing.T) {
	got, err := WSURLFromEndpoint("https://ygate-api.example.com/api/v2/ingestion/register-readings")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://ygate-api.example.com/api/v1/gateway/realtime" {
		t.Fatalf("websocket URL = %q", got)
	}
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

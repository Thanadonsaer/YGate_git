package realtimeclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"chpp/modbus-api-middleware/internal/app"
	"chpp/modbus-api-middleware/internal/configcache"
	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

func TestTelemetryDrainThenAckDeliversAndMarksRows(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.Enqueue("key-1", "hash-1", domain.Reading{GatewayID: "gw-1", DevDn: "dev-1", PlantCode: "P1", DevTypeID: 1, CollectTime: 1, RegisterAddressMap: map[string]float64{"40001": 1}}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	drainedIDs := make(chan []int64, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()

		var hello envelope
		if err = wsjson.Read(ctx, conn, &hello); err != nil {
			done <- fmt.Errorf("read hello: %w", err)
			return
		}
		if hello.Type != "hello" {
			done <- fmt.Errorf("first message type = %q, want hello", hello.Type)
			return
		}

		drainData, _ := json.Marshal(map[string]any{"batchSize": 20})
		if err = wsjson.Write(ctx, conn, envelope{Type: "command.request", CommandID: "cmd-drain", Kind: "telemetry.drain", Data: drainData}); err != nil {
			done <- err
			return
		}
		var drainResult envelope
		if err = wsjson.Read(ctx, conn, &drainResult); err != nil {
			done <- err
			return
		}
		var drained struct {
			IDs      []int64           `json:"ids"`
			Readings []json.RawMessage `json:"readings"`
		}
		if err = json.Unmarshal(drainResult.Data, &drained); err != nil {
			done <- fmt.Errorf("unmarshal drain result data: %w", err)
			return
		}
		if !drainResult.Ok || len(drained.IDs) != 1 || len(drained.Readings) != 1 {
			done <- fmt.Errorf("drainResult = %+v, drained = %+v, want ok=true with 1 id and 1 reading", drainResult, drained)
			return
		}
		drainedIDs <- drained.IDs

		ackData, _ := json.Marshal(map[string]any{"ids": drained.IDs})
		if err = wsjson.Write(ctx, conn, envelope{Type: "command.request", CommandID: "cmd-ack", Kind: "telemetry.ack", Data: ackData}); err != nil {
			done <- err
			return
		}
		var ackResult envelope
		if err = wsjson.Read(ctx, conn, &ackResult); err != nil {
			done <- err
			return
		}
		if !ackResult.Ok {
			done <- fmt.Errorf("ackResult = %+v, want ok=true", ackResult)
			return
		}
		done <- nil
	}))
	defer server.Close()

	// wsURLFromEndpoint (client.go) only cares about scheme=="https" -> wss;
	// any other scheme, including httptest's plain "http", becomes "ws", so
	// server.URL can be passed straight through unmodified.
	if _, err = st.SaveGatewayConfig(domain.GatewayConfig{Endpoint: server.URL, APIKey: "test-key", APIPollingEnabled: true}); err != nil {
		t.Fatal(err)
	}

	cache := configcache.New()
	client := &Client{Store: st, Cache: cache, App: &app.Service{Store: st, Cache: cache}, Version: "test"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go client.Run(ctx)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fake server: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for drain/ack exchange")
	}

	ids := <-drainedIDs
	remaining, err := st.Ready(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no PENDING/RETRYING rows after ack, got %d (acked ids=%v)", len(remaining), ids)
	}
}

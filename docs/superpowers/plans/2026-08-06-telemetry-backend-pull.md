# Backend-Triggered Telemetry Pull Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the middleware's self-initiated REST push of telemetry with platform-api pulling each connected gateway's buffered readings over the existing outbound gateway WebSocket channel, using a two-phase `telemetry.drain` / `telemetry.ack` command handshake.

**Architecture:** Middleware keeps scanning Modbus devices into its local SQLite outbox on its own ticker (unchanged). platform-api starts one ticker per connected gateway (spawned when its WebSocket registers with `gatewayhub.Hub`) that sends `telemetry.drain`, ingests the returned batch via the existing `ingestion.Service.IngestRaw` path, and only then sends `telemetry.ack` so the middleware marks those rows delivered.

**Tech Stack:** Go (both services), `github.com/coder/websocket` + `wsjson`, `pgx/v5`, existing `gatewayhub.Hub` request/response correlation, existing SQLite outbox (`modernc.org/sqlite`).

**Design doc:** `docs/superpowers/specs/2026-08-06-telemetry-backend-pull-design.md`

## Global Constraints

- Do not change `app.PollEnabledConnections`, the outbox schema, or the REST route `POST /api/v2/ingestion/register-readings` (left in place, just unused by the new path).
- Single platform-api instance assumed — no cross-instance gateway ownership coordination.
- Drain batch size: 20 (matches the REST worker's prior `BatchSize: 20`).
- Command timeout: 15s per `hub.RunCommand` call (matches existing `RunMiddlewareCommand` precedent in `internal/core/middleware_config.go:657`).
- The `api_polling_enabled` flag is kept as the admin on/off switch, but now gates whether platform-api *starts pulling* from a gateway at all (checked once when the gateway's WebSocket registers), not a locally-silent no-op deep in the middleware.

---

### Task 1: Middleware — handle `telemetry.drain` / `telemetry.ack` commands

**Files:**
- Modify: `modbus-api-middleware/internal/realtimeclient/client.go:216-282` (`handleCommand`)
- Test: `modbus-api-middleware/internal/realtimeclient/client_test.go` (new)

**Interfaces:**
- Consumes: `c.Store.Ready(limit int) ([]store.OutboxEvent, error)`, `c.Store.Delivered(ids []int64) error` (both already exist, `modbus-api-middleware/internal/store/sqlite.go:86,102`).
- Produces: two new `msg.Kind` values middleware understands: `"telemetry.drain"` (request `{"batchSize": N}` in `msg.Data`, replies `{"ids": [...], "readings": [...]}` in `result.Data`) and `"telemetry.ack"` (request `{"ids": [...]}` in `msg.Data`, replies `{}` with `result.Ok`). Later tasks (platform-api side) rely on exactly these field names.

- [ ] **Step 1: Write the failing integration test**

Create `modbus-api-middleware/internal/realtimeclient/client_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modbus-api-middleware && go test ./internal/realtimeclient/... -run TestTelemetryDrainThenAckDeliversAndMarksRows -v`
Expected: FAIL — the fake server hangs / times out because `handleCommand` doesn't recognize `"telemetry.drain"` or `"telemetry.ack"` yet (falls into the `default` case, `result.Ok=false, result.Error="unknown command kind"`), so `st.Ready(20)` still returns 1 row after ack, or the assertions on `drainResult`/`ackResult` fail.

- [ ] **Step 3: Implement `telemetry.drain` and `telemetry.ack` in `handleCommand`**

In `modbus-api-middleware/internal/realtimeclient/client.go`, add two cases to the `switch msg.Kind` block in `handleCommand` (right after the existing `case "config-export":` block, before `case "update.stage":`):

```go
	case "telemetry.drain":
		var req struct {
			BatchSize int `json:"batchSize"`
		}
		_ = json.Unmarshal(msg.Data, &req)
		if req.BatchSize <= 0 || req.BatchSize > 500 {
			req.BatchSize = 20
		}
		events, err := c.Store.Ready(req.BatchSize)
		if err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			ids := make([]int64, len(events))
			readings := make([]json.RawMessage, len(events))
			for i, e := range events {
				ids[i] = e.ID
				readings[i] = json.RawMessage(e.Payload)
			}
			data, _ := json.Marshal(map[string]any{"ids": ids, "readings": readings})
			result.Ok, result.Data = true, data
		}
	case "telemetry.ack":
		var req struct {
			IDs []int64 `json:"ids"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			result.Ok, result.Error = false, err.Error()
		} else if err := c.Store.Delivered(req.IDs); err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			result.Ok = true
		}
```

No new imports needed — `encoding/json` is already imported in this file.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modbus-api-middleware && go test ./internal/realtimeclient/... -run TestTelemetryDrainThenAckDeliversAndMarksRows -v`
Expected: PASS

- [ ] **Step 5: Run the full middleware test suite to check for regressions**

Run: `cd modbus-api-middleware && go build ./... && go test ./...`
Expected: PASS (all packages)

- [ ] **Step 6: Commit**

```bash
git add modbus-api-middleware/internal/realtimeclient/client.go modbus-api-middleware/internal/realtimeclient/client_test.go
git commit -m "$(cat <<'EOF'
feat(middleware): handle telemetry.drain and telemetry.ack over realtime WS

Lets platform-api pull buffered outbox rows on demand instead of the
middleware pushing them itself, per docs/superpowers/specs/2026-08-06-telemetry-backend-pull-design.md.
EOF
)"
```

---

### Task 2: Middleware — decouple Modbus scan loop from REST delivery

**Files:**
- Modify: `modbus-api-middleware/internal/app/poller.go` (add `PollInterval`)
- Modify: `modbus-api-middleware/cmd/middleware/main.go` (replace `runDelivery`/`delivery.Worker` wiring with a scan-only loop)
- Test: `modbus-api-middleware/internal/app/poller_test.go` (new, for `PollInterval` only — `PollEnabledConnections` already has its own coverage)

**Interfaces:**
- Consumes: `s.Store.GatewayConfig() (domain.GatewayConfig, error)` (existing).
- Produces: `func (s *Service) PollInterval() time.Duration` — later steps in this task call it from `cmd/middleware/main.go`.

- [ ] **Step 1: Write the failing test for `PollInterval`**

Create `modbus-api-middleware/internal/app/poller_test.go`:

```go
package app

import (
	"path/filepath"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modbus-api-middleware && go test ./internal/app/... -run TestPollInterval -v`
Expected: FAIL with `svc.PollInterval undefined (type *Service has no field or method PollInterval)`

- [ ] **Step 3: Implement `PollInterval`**

Add to `modbus-api-middleware/internal/app/poller.go` (needs `"time"` added to its import block):

```go
func (s *Service) PollInterval() time.Duration {
	seconds := 5
	if s.Store != nil {
		if cfg, err := s.Store.GatewayConfig(); err == nil && cfg.SendIntervalSeconds > 0 {
			seconds = cfg.SendIntervalSeconds
		}
	}
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modbus-api-middleware && go test ./internal/app/... -run TestPollInterval -v`
Expected: PASS

- [ ] **Step 5: Replace the REST delivery loop in `cmd/middleware/main.go` with a scan-only loop**

In `modbus-api-middleware/cmd/middleware/main.go`:

Remove the `"chpp/modbus-api-middleware/internal/delivery"` import (line 6).

Replace (around line 167-170):

```go
	worker := &delivery.Worker{Store: st, BatchSize: 20, Client: &http.Client{Timeout: 10 * time.Second}, BeforeSend: func() error {
		return svc.PollEnabledConnections(cfg.GatewayID, log.Printf)
	}}
	go runDelivery(ctx, worker)
```

with:

```go
	go runPollScan(ctx, svc, cfg.GatewayID)
```

Replace the `runDelivery` function (lines 211-224) with:

```go
func runPollScan(ctx context.Context, svc *app.Service, gatewayID string) {
	for {
		if err := svc.PollEnabledConnections(gatewayID, log.Printf); err != nil {
			log.Printf("poll scan: %v", err)
		}
		timer := time.NewTimer(svc.PollInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
```

Check whether `net/http` is still used elsewhere in this file (it is — `httpServer := &http.Server{...}` further down) so its import stays.

- [ ] **Step 6: Build and run the full middleware test suite**

Run: `cd modbus-api-middleware && go build ./... && go test ./...`
Expected: PASS, no unused-import errors

- [ ] **Step 7: Commit**

```bash
git add modbus-api-middleware/internal/app/poller.go modbus-api-middleware/internal/app/poller_test.go modbus-api-middleware/cmd/middleware/main.go
git commit -m "$(cat <<'EOF'
refactor(middleware): decouple Modbus scan loop from REST delivery

The scan loop (PollEnabledConnections) now runs on its own ticker
instead of firing as a side effect of the REST push worker, which
telemetry.drain (client.go) makes obsolete as the delivery path.
EOF
)"
```

---

### Task 3: platform-api — add a poll-config lookup query for a middleware client

**Files:**
- Modify: `services/platform-api/internal/database/queries/ingestion.sql` (add query)
- Modify: `services/platform-api/internal/database/dbgen/ingestion.sql.go` (hand-add the matching generated code — no `sqlc` CLI available in this environment; if the developer has `sqlc` installed, running `sqlc generate` from `services/platform-api` after the `.sql` edit should reproduce this exactly instead)
- Modify: `services/platform-api/internal/ingestion/service.go` (expose a service method)
- Test: `services/platform-api/internal/ingestion/service_integration_test.go` (add a test alongside existing integration tests — this package's tests already run against a real Postgres; follow its existing setup helper)

**Interfaces:**
- Consumes: nothing new.
- Produces: `func (s *Service) MiddlewareClientPullConfig(ctx context.Context, clientID pgtype.UUID) (pollIntervalSeconds int32, apiPollingEnabled bool, err error)` — Task 5 (telemetrypull wiring) calls this by exactly this name and return shape.

- [ ] **Step 1: Add the SQL query**

Append to `services/platform-api/internal/database/queries/ingestion.sql`:

```sql
-- name: MiddlewareClientPullConfig :one
SELECT poll_interval_seconds, api_polling_enabled
FROM auth.middleware_client
WHERE id = sqlc.arg(id);
```

- [ ] **Step 2: Hand-add the generated Go code**

Append to `services/platform-api/internal/database/dbgen/ingestion.sql.go`, after the `InsertTelemetryReading` function (before `const onboardDevice`), matching this file's existing generated style exactly:

```go
const middlewareClientPullConfig = `-- name: MiddlewareClientPullConfig :one
SELECT poll_interval_seconds, api_polling_enabled
FROM auth.middleware_client
WHERE id = $1
`

type MiddlewareClientPullConfigRow struct {
	PollIntervalSeconds int32
	ApiPollingEnabled   bool
}

func (q *Queries) MiddlewareClientPullConfig(ctx context.Context, id pgtype.UUID) (MiddlewareClientPullConfigRow, error) {
	row := q.db.QueryRow(ctx, middlewareClientPullConfig, id)
	var i MiddlewareClientPullConfigRow
	err := row.Scan(&i.PollIntervalSeconds, &i.ApiPollingEnabled)
	return i, err
}
```

- [ ] **Step 3: Add the service method**

In `services/platform-api/internal/ingestion/service.go`, add right after `Authenticate` (after line 96):

```go
func (s *Service) MiddlewareClientPullConfig(ctx context.Context, clientID pgtype.UUID) (pollIntervalSeconds int32, apiPollingEnabled bool, err error) {
	row, err := s.queries.MiddlewareClientPullConfig(ctx, clientID)
	if err != nil {
		return 0, false, fmt.Errorf("load middleware client pull config: %w", err)
	}
	return row.PollIntervalSeconds, row.ApiPollingEnabled, nil
}
```

- [ ] **Step 4: Write an integration test**

Check the existing setup helper in `services/platform-api/internal/ingestion/service_integration_test.go` (or `raw_service_integration_test.go`) for how a `*Service` and a seeded `middleware_client` row are constructed in this package's other integration tests, and add:

```go
func TestMiddlewareClientPullConfigReadsPollIntervalAndApiPolling(t *testing.T) {
	service, client := newTestServiceAndClient(t) // reuse whatever helper the existing tests in this file use
	pollIntervalSeconds, apiPollingEnabled, err := service.MiddlewareClientPullConfig(context.Background(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pollIntervalSeconds < 5 || pollIntervalSeconds > 3600 {
		t.Fatalf("pollIntervalSeconds=%d out of the DB CHECK constraint range", pollIntervalSeconds)
	}
	_ = apiPollingEnabled
}
```

If this package's existing integration tests build their fixture differently than `newTestServiceAndClient`, use that pattern instead — match whatever `TestAuthenticate...`-style test in `service_integration_test.go` already does to get a `Service` and a `Client` with a valid `ID`.

- [ ] **Step 5: Run the test**

Run: `cd services/platform-api && go test ./internal/ingestion/... -run TestMiddlewareClientPullConfig -v`
Expected: PASS (requires the integration Postgres this package's other tests already require — see this package's existing `_integration_test.go` files for how the DB connection is set up, e.g. env var or build tag)

- [ ] **Step 6: Build and run the package test suite**

Run: `cd services/platform-api && go build ./... && go test ./internal/ingestion/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add services/platform-api/internal/database/queries/ingestion.sql services/platform-api/internal/database/dbgen/ingestion.sql.go services/platform-api/internal/ingestion/service.go services/platform-api/internal/ingestion/service_integration_test.go
git commit -m "$(cat <<'EOF'
feat(platform-api): add MiddlewareClientPullConfig query

Exposes poll_interval_seconds and api_polling_enabled for a
middleware_client so the telemetry pull scheduler (next commit) knows
how often, and whether, to pull from a given gateway.
EOF
)"
```

---

### Task 4: platform-api — `telemetrypull` scheduler package

**Files:**
- Create: `services/platform-api/internal/telemetrypull/puller.go`
- Test: `services/platform-api/internal/telemetrypull/puller_test.go`

**Interfaces:**
- Consumes: `*gatewayhub.Hub` (`Register`, `RunCommand`, `IsOnline` — `services/platform-api/internal/gatewayhub/hub.go`), `ingestion.Client`, `ingestion.RawBatch`, `ingestion.RawSchemaVersion` (`services/platform-api/internal/ingestion/service.go`, `raw_service.go`).
- Produces: `func Run(ctx context.Context, hub *gatewayhub.Hub, ingest Ingester, client ingestion.Client, gatewayID string, interval time.Duration)` and the `Ingester` interface — Task 5 (`gateway_realtime.go` wiring) calls `telemetrypull.Run` by this exact signature.

- [ ] **Step 1: Write the failing test**

Create `services/platform-api/internal/telemetrypull/puller_test.go`:

```go
package telemetrypull

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
)

type stubIngester struct {
	calls []ingestion.RawBatch
}

func (s *stubIngester) IngestRaw(ctx context.Context, client ingestion.Client, idempotencyKey string, raw []byte, batch ingestion.RawBatch, now time.Time) (ingestion.Result, error) {
	s.calls = append(s.calls, batch)
	return ingestion.Result{Status: "accepted", AcceptedCount: int32(len(batch.Data))}, nil
}

func TestPullOnceDrainsIngestsThenAcks(t *testing.T) {
	hub := gatewayhub.New()
	out, resolve, unregister := hub.Register("gw-1")
	defer unregister()

	reading, _ := json.Marshal(map[string]any{
		"gatewayId": "gw-1", "devDn": "dev-1", "plantCode": "P1", "devTypeId": 1,
		"collectTime": time.Now().UnixMilli(), "registerAddressMap": map[string]float64{"40001": 1},
	})

	go func() {
		drainPayload := <-out
		var drainReq struct {
			CommandID string `json:"commandId"`
			Kind      string `json:"kind"`
		}
		_ = json.Unmarshal(drainPayload, &drainReq)
		if drainReq.Kind != "telemetry.drain" {
			t.Errorf("first command kind = %q, want telemetry.drain", drainReq.Kind)
		}
		drainResultData, _ := json.Marshal(map[string]any{"ids": []int64{1}, "readings": []json.RawMessage{reading}})
		resolve(drainReq.CommandID, mustMarshal(map[string]any{"ok": true, "data": json.RawMessage(drainResultData)}))

		ackPayload := <-out
		var ackReq struct {
			CommandID string `json:"commandId"`
			Kind      string `json:"kind"`
		}
		_ = json.Unmarshal(ackPayload, &ackReq)
		if ackReq.Kind != "telemetry.ack" {
			t.Errorf("second command kind = %q, want telemetry.ack", ackReq.Kind)
		}
		resolve(ackReq.CommandID, mustMarshal(map[string]any{"ok": true}))
	}()

	ingest := &stubIngester{}
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pullOnce(context.Background(), hub, ingest, client, "gw-1")

	if len(ingest.calls) != 1 || len(ingest.calls[0].Data) != 1 {
		t.Fatalf("ingest.calls = %+v, want exactly 1 call with 1 reading", ingest.calls)
	}
}

func TestPullOnceSkipsSilentlyWhenGatewayOffline(t *testing.T) {
	hub := gatewayhub.New()
	ingest := &stubIngester{}
	client := ingestion.Client{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, OrganizationID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pullOnce(context.Background(), hub, ingest, client, "gw-offline") // must not panic or block
	if len(ingest.calls) != 0 {
		t.Fatalf("ingest.calls = %+v, want none for an offline gateway", ingest.calls)
	}
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/platform-api && go test ./internal/telemetrypull/... -v`
Expected: FAIL to compile — package `telemetrypull` and `pullOnce`/`Ingester` don't exist yet.

- [ ] **Step 3: Implement the package**

Create `services/platform-api/internal/telemetrypull/puller.go`:

```go
// Package telemetrypull drives the backend side of the telemetry.drain /
// telemetry.ack handshake described in
// docs/superpowers/specs/2026-08-06-telemetry-backend-pull-design.md: one
// ticker per connected gateway that pulls its buffered outbox rows over the
// existing gatewayhub-managed WebSocket instead of waiting for the
// middleware to push them over REST.
package telemetrypull

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"

	"ygate/platform-api/internal/gatewayhub"
	"ygate/platform-api/internal/ingestion"
)

const (
	drainBatchSize = 20
	commandTimeout = 15 * time.Second
)

// Ingester is the subset of *ingestion.Service that pullOnce needs -- kept
// as an interface so tests can stub it out without a real database.
type Ingester interface {
	IngestRaw(ctx context.Context, client ingestion.Client, idempotencyKey string, raw []byte, batch ingestion.RawBatch, now time.Time) (ingestion.Result, error)
}

// Run ticks at interval until ctx is done, draining and ingesting gatewayID's
// buffered telemetry each tick. Call as `go telemetrypull.Run(...)` right
// after the gateway's WebSocket registers with hub -- ctx should be that
// connection's own request context, so this goroutine exits when the
// connection does.
func Run(ctx context.Context, hub *gatewayhub.Hub, ingest Ingester, client ingestion.Client, gatewayID string, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pullOnce(ctx, hub, ingest, client, gatewayID)
		}
	}
}

type commandResult struct {
	Ok    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

type drainedBatch struct {
	IDs      []int64           `json:"ids"`
	Readings []json.RawMessage `json:"readings"`
}

func pullOnce(ctx context.Context, hub *gatewayhub.Hub, ingest Ingester, client ingestion.Client, gatewayID string) {
	drainCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	commandID := newCommandID()
	data, _ := json.Marshal(map[string]any{"batchSize": drainBatchSize})
	payload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": commandID, "kind": "telemetry.drain", "data": data})
	raw, err := hub.RunCommand(drainCtx, gatewayID, commandID, payload)
	if err != nil {
		return // offline or timed out -- try again next tick, nothing was mutated
	}
	var result commandResult
	if err = json.Unmarshal(raw, &result); err != nil || !result.Ok {
		log.Printf("telemetry pull: drain %s failed: ok=%v err=%v msg=%q", gatewayID, result.Ok, err, result.Error)
		return
	}
	var drained drainedBatch
	if err = json.Unmarshal(result.Data, &drained); err != nil || len(drained.IDs) == 0 {
		return
	}

	body, _ := json.Marshal(map[string]any{"schemaVersion": ingestion.RawSchemaVersion, "data": drained.Readings})
	var batch ingestion.RawBatch
	if err = json.Unmarshal(body, &batch); err != nil {
		log.Printf("telemetry pull: decode drained batch for %s: %v", gatewayID, err)
		return
	}
	if _, err = ingest.IngestRaw(ctx, client, "", body, batch, time.Now()); err != nil {
		log.Printf("telemetry pull: ingest for %s: %v", gatewayID, err)
		return // no ack sent -- rows stay PENDING and are redrained next tick
	}

	ackCtx, cancelAck := context.WithTimeout(ctx, commandTimeout)
	defer cancelAck()
	ackCommandID := newCommandID()
	ackData, _ := json.Marshal(map[string]any{"ids": drained.IDs})
	ackPayload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": ackCommandID, "kind": "telemetry.ack", "data": ackData})
	if _, err = hub.RunCommand(ackCtx, gatewayID, ackCommandID, ackPayload); err != nil {
		log.Printf("telemetry pull: ack for %s: %v (rows will redeliver next tick)", gatewayID, err)
	}
}

func newCommandID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/platform-api && go test ./internal/telemetrypull/... -v`
Expected: PASS

- [ ] **Step 5: Build the whole module**

Run: `cd services/platform-api && go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add services/platform-api/internal/telemetrypull
git commit -m "$(cat <<'EOF'
feat(platform-api): add telemetrypull scheduler

Per-gateway ticker that drains buffered telemetry over the gateway
WebSocket and ingests it via the existing raw ingestion path, per
docs/superpowers/specs/2026-08-06-telemetry-backend-pull-design.md.
EOF
)"
```

---

### Task 5: platform-api — wire the puller into the gateway WebSocket handler

**Files:**
- Modify: `services/platform-api/internal/httpapi/gateway_realtime.go`
- Test: `services/platform-api/internal/httpapi/gateway_realtime_test.go` (new)

**Interfaces:**
- Consumes: `telemetrypull.Run` (Task 4), `ingestionService.MiddlewareClientPullConfig` (Task 3), `hub.Register` (existing).
- Produces: nothing further consumed by other tasks — this is the last wiring step.

- [ ] **Step 1: Write the failing test**

Create `services/platform-api/internal/httpapi/gateway_realtime_test.go`. This test only checks that a gateway with `api_polling_enabled = false` does **not** get sent a `telemetry.drain` command even after waiting past its poll interval — it does not require a real Postgres because it stubs the two `ingestionService` calls the handler makes (`Authenticate`, `MiddlewareClientPullConfig`) at the `httpapi` package's existing seams. First, check how this package's other tests (e.g. `raw_ingestion_test.go` if one exists, or `ingestion.go`'s callers) construct a fake `*ingestion.Service` — if `ingestion.Service` has no interface seam at the `httpapi` layer today, add the minimal one needed:

Check first: `Grep -n "ingestionService \*ingestion.Service" services/platform-api/internal/httpapi/server.go` — `gatewayRealtimeHandler` currently takes a concrete `*ingestion.Service`. Introducing an interface here is a bigger change than this task's scope; instead, test this at the `telemetrypull` level (already done in Task 4) and skip a dedicated `gateway_realtime_test.go`. Confirm this by re-reading `gateway_realtime.go`'s existing test coverage:

Run: `ls services/platform-api/internal/httpapi/gateway_realtime_test.go 2>/dev/null || echo "no existing test file"`

If none exists (expected), skip writing a new websocket-level test for this task — Task 4's `puller_test.go` already covers the drain/ack/offline logic in isolation, and Task 3's integration test covers `MiddlewareClientPullConfig`. This task is a thin wiring change; verify it manually per Step 4 below instead of adding test infrastructure disproportionate to a few lines of glue code.

- [ ] **Step 2: Wire `telemetrypull.Run` into `gatewayRealtimeHandler`**

In `services/platform-api/internal/httpapi/gateway_realtime.go`:

Add to the import block:

```go
	"ygate/platform-api/internal/telemetrypull"
```

Change:

```go
		out, resolve, unregister := hub.Register(gatewayID)
		defer unregister()
```

to:

```go
		out, resolve, unregister := hub.Register(gatewayID)
		defer unregister()

		if pollIntervalSeconds, apiPollingEnabled, err := ingestionService.MiddlewareClientPullConfig(ctx, client.ID); err != nil {
			log.Printf("telemetry pull: read pull config for %s: %v", gatewayID, err)
		} else if apiPollingEnabled {
			go telemetrypull.Run(ctx, hub, ingestionService, client, gatewayID, time.Duration(pollIntervalSeconds)*time.Second)
		}
```

`client` here is the `ingestion.Client` already returned by `service.Authenticate` earlier in this function (line 44) — it already carries `ID`, `OrganizationID`, `Name`, `AutoOnboard`, exactly what `ingestion.Client` requires. `ingestionService` (the `*ingestion.Service` parameter) satisfies `telemetrypull.Ingester` automatically since it already has an `IngestRaw` method with a matching signature.

- [ ] **Step 3: Build**

Run: `cd services/platform-api && go build ./...`
Expected: PASS

- [ ] **Step 4: Manual smoke test**

Run: `cd d:/PROJECT_2026/YGate_git && ./run-all.bat` is Windows-interactive (opens windows) — instead run platform-api and the middleware directly for this check:

```bash
cd services/platform-api && go run ./cmd/platform-api &
cd modbus-api-middleware && go run ./cmd/middleware -gateway test-gw -endpoint http://127.0.0.1:<platform-api-port> -apiKey <a-valid-key-from-the-DB>
```

Watch platform-api's logs: within one `poll_interval_seconds` window after the middleware connects (visible via a `hello` log line from `HandleGatewayHello`), there should be no `telemetry pull:` error lines, and a row should appear in `telemetry.telemetry_reading` for any enabled connection the middleware has configured. If `api_polling_enabled` is `false` for that middleware_client row, confirm no `telemetry.drain` traffic occurs (no `outbox_events` rows on the middleware's SQLite ever transition to `DELIVERED`).

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/httpapi/gateway_realtime.go
git commit -m "$(cat <<'EOF'
feat(platform-api): start telemetry pull ticker when a gateway connects

Wires telemetrypull.Run into gatewayRealtimeHandler, gated on the
middleware_client's api_polling_enabled flag, completing the
backend-triggered pull design.
EOF
)"
```

---

### Task 6: Web UI — update the API Polling checkbox copy

**Files:**
- Modify: `apps/web/app/features/middlewares/middlewares-page.tsx:171`

**Interfaces:**
- Consumes: nothing new (same `apiPollingEnabled` state/field already wired).
- Produces: nothing consumed elsewhere.

- [ ] **Step 1: Update the label**

In `apps/web/app/features/middlewares/middlewares-page.tsx`, change line 171 from:

```tsx
          <label className="toggle-field full-field"><input type="checkbox" checked={apiPollingEnabled} onChange={(event) => setApiPollingEnabled(event.target.checked)} /> เปิดใช้งาน API Polling (ส่ง telemetry ผ่าน REST API)</label>
```

to:

```tsx
          <label className="toggle-field full-field"><input type="checkbox" checked={apiPollingEnabled} onChange={(event) => setApiPollingEnabled(event.target.checked)} /> เปิดใช้งาน Telemetry Pull (platform ดึงข้อมูลผ่าน WebSocket)</label>
```

This is copy-only — the underlying field (`apiPollingEnabled`, `PATCH`/`POST` payload key `apiPollingEnabled`) and its DB column are unchanged; Task 5 already made platform-api read this same flag to decide whether to start pulling.

- [ ] **Step 2: Verify in the browser**

Run: `cd apps/web && npm run dev`, open the Middlewares page, open a gateway's edit form, confirm the new label text renders correctly in Thai and the checkbox still toggles.

- [ ] **Step 3: Commit**

```bash
git add apps/web/app/features/middlewares/middlewares-page.tsx
git commit -m "$(cat <<'EOF'
docs(web): reword API Polling checkbox for the WebSocket pull design

REST push is no longer what this flag controls -- it now gates
platform-api's telemetry pull ticker (see telemetrypull package).
EOF
)"
```

---

## Post-plan check

After all tasks: `git log --oneline -8` should show 6 commits (one per task); `cd modbus-api-middleware && go build ./... && go test ./...` and `cd services/platform-api && go build ./...` should both be clean.

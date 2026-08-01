# Import Config From Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an admin click a button on a Middleware Gateway's detail page to pull that Middleware's local config (Brands/Device Sets/Addresses/Connections, from before it was ever registered in platform-api) over the existing realtime WebSocket channel and one-time-import it into Device Model + Register Metadata master data.

**Architecture:** Reuses the existing `command.request`/`command.result` WebSocket messages already used for Test Connection/Test Read (`modbus-api-middleware`'s `realtimeclient.Client.handleCommand`, `platform-api`'s `gatewayhub.Hub.RunCommand`) — no new connection, no new port. A new `"config-export"` command kind returns the Middleware's in-memory config cache as a `domain.ConfigSnapshot`-shaped JSON payload; platform-api decodes it into the existing `MiddlewareConfigSnapshot` Go type and upserts Device Models/Register Metadata via the same `core.Service` methods the Register Metadata page's own UI already calls.

**Tech Stack:** Go (both `modbus-api-middleware` and `services/platform-api`), Next.js/React (`apps/web`), the existing realtime WebSocket protocol.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-01-middleware-config-import-design.md` — every task here traces to a section of that spec.
- One-time import only — no drift detection, no scheduled re-sync, no diff view (explicitly out of scope per the spec).
- Connections are never turned into `Device` rows automatically — only summarized in the response, per the spec's "Plant assignment stays a human decision" rule.
- No change to the steady-state push direction (`buildConfigSnapshot`/`recomputeAndPushMiddleware`) — this feature is purely additive.
- `apps/web` has no test runner — its task is verified via `npx tsc --noEmit` plus manual checks. `services/platform-api` and `modbus-api-middleware` DO have `go test` — use it per task.
- Follow this repo's existing conventions exactly where noted: the `command.request`/`command.result` envelope shape (`modbus-api-middleware/internal/realtimeclient/client.go`), the `requireOrganizationPermission` permission-check pattern (`services/platform-api/internal/core/middleware_plants.go`), the `*_integration_test.go` pattern gated on `PLATFORM_TEST_DATABASE_URL` (`services/platform-api/internal/core/plants_integration_test.go`).

---

### Task 1: `modbus-api-middleware` — `"config-export"` command handler

**Files:**
- Modify: `modbus-api-middleware/internal/realtimeclient/client.go`

**Interfaces:**
- Consumes: `c.Cache.Load()` (`modbus-api-middleware/internal/configcache/cache.go`, already a field on `Client` — returns `*configcache.Config{Version int64; Brands []domain.Brand; DeviceSets map[int64]domain.DeviceSet; Connections map[int64]domain.ConnectionConfig; Plants []domain.Plant}`), `domain.ConfigSnapshot{Version int64; Brands []domain.Brand; DeviceSets []domain.DeviceSet; Connections []domain.ConnectionConfig; Plants []domain.Plant}` (`modbus-api-middleware/internal/domain/configuration.go`).
- Produces: a `command.result` message whose `data` field is a `domain.ConfigSnapshot`-shaped JSON object — consumed by Task 3 (`platform-api`'s `ImportMiddlewareConfig`), which decodes it into its own mirrored `MiddlewareConfigSnapshot` type (the two already share the exact same wire shape — this is the same contract the `config.snapshot` push direction already uses).

- [ ] **Step 1: Add the `"config-export"` case to `handleCommand`**

In `modbus-api-middleware/internal/realtimeclient/client.go`, `handleCommand`'s `switch msg.Kind` currently has cases `"readNow"` and `"connectTest"` followed by a `default`. Add a new case right before `default`:

```go
	case "config-export":
		cfg := c.Cache.Load()
		snapshot := domain.ConfigSnapshot{Version: cfg.Version, Brands: cfg.Brands, Plants: cfg.Plants}
		for _, ds := range cfg.DeviceSets {
			snapshot.DeviceSets = append(snapshot.DeviceSets, ds)
		}
		for _, conn := range cfg.Connections {
			snapshot.Connections = append(snapshot.Connections, conn)
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			result.Ok, result.Error = false, err.Error()
		} else {
			result.Ok, result.Data = true, data
		}
```

(`domain` and `json` are already imported in this file — no new imports needed.)

- [ ] **Step 2: Write a unit test for the new case**

`modbus-api-middleware/internal/realtimeclient` has no existing test file for `handleCommand` (it's exercised indirectly via integration tests elsewhere in this codebase) — rather than build new WebSocket test scaffolding for one switch case, verify this step with `go build`/`go vet` (Step 3) plus the end-to-end manual check in Task 4. This mirrors how the pre-existing `"readNow"`/`"connectTest"` cases in this same function are verified in this codebase — no dedicated unit test exists for them either.

- [ ] **Step 3: Verify**

Run: `cd modbus-api-middleware && go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add modbus-api-middleware/internal/realtimeclient/client.go
git commit -m "$(cat <<'EOF'
Add config-export command to modbus-api-middleware's realtime client

Responds to a command.request{kind:"config-export"} with the
middleware's current in-memory config (Brands/DeviceSets/Addresses/
Connections) as a domain.ConfigSnapshot-shaped payload, over the same
WebSocket channel already used for Test Connection/Test Read.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `platform-api` — `importFromSnapshot` upsert logic (TDD)

**Files:**
- Modify: `services/platform-api/internal/core/middleware_config.go`
- Test: `services/platform-api/internal/core/middleware_config_integration_test.go` (new)

**Interfaces:**
- Consumes: `s.DeviceModels(ctx, principal) ([]DeviceModelOption, error)`, `s.CreateDeviceModel(ctx, principal, DeviceModelInput, sourceIP *netip.Addr) (DeviceModelOption, error)`, `s.SetDeviceModelRegisterMetadata(ctx, principal, modelID string, UpdateDeviceModelRegisterMetadataInput, sourceIP *netip.Addr) (DeviceModelRegisterMetadata, error)` (all pre-existing in `services/platform-api/internal/core/devices.go`), `MiddlewareConfigSnapshot`/`MiddlewareBrand`/`MiddlewareDeviceSet`/`MiddlewareAddress`/`MiddlewareConnection` (pre-existing in `middleware_config.go`).
- Produces: `ImportMiddlewareConfigResult{DeviceModelsCreated, DeviceModelsReused, RegisterMetadataUpserted int; ConnectionsFound []ImportedConnectionSummary}`, `ImportedConnectionSummary{Host string; Port int; UnitID int; DeviceModel string}`, and `(s *Service) importFromSnapshot(ctx, principal auth.Principal, organizationID string, snapshot MiddlewareConfigSnapshot, sourceIP *netip.Addr) (ImportMiddlewareConfigResult, error)` — an unexported method, consumed by Task 3's `ImportMiddlewareConfig`.

- [ ] **Step 1: Write the failing integration test**

Create `services/platform-api/internal/core/middleware_config_integration_test.go`:

```go
package core

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/gatewayhub"
)

func TestImportFromSnapshotAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	orgID := mustUUID(t, "20000000-0000-4000-8000-000000000001")
	adminID := mustUUID(t, "20000000-0000-4000-8000-000000000011")
	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,'TEST-IMPORT','Test Import Org')", orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,display_name,password_hash) VALUES($1,$2,'import-admin@test.invalid','Import Admin','unused')`, adminID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,'00000000-0000-4000-8000-000000000201')`,
		mustUUID(t, "20000000-0000-4000-8000-000000000021"), pgtype.UUID{}, adminID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, gatewayhub.New())
	admin := auth.Principal{UserID: adminID, OrganizationID: orgID}
	sourceIP, _ := netip.ParseAddr("127.0.0.1")

	snapshot := MiddlewareConfigSnapshot{
		Brands: []MiddlewareBrand{{BrandID: 1, BrandName: "TestBrand"}},
		DeviceSets: []MiddlewareDeviceSet{{
			DeviceSetID: 100, BrandID: 1, DevType: "Inverter", DevModel: "TB-5000",
			Addresses: []MiddlewareAddress{
				{AddressID: 1, DeviceSetID: 100, FunctionCode: 3, Register: 40001, Description: "Active power", CanonicalKey: "P_AC", Factor: 1, DataType: "FLOAT"},
				{AddressID: 2, DeviceSetID: 100, FunctionCode: 3, Register: 40003, Description: "State of charge", Factor: 0.1, DataType: "SHORT"},
			},
		}},
		Connections: []MiddlewareConnection{{ConnectionID: 1, ConnectionName: "Inv1", Host: "192.168.1.50", Port: 502, UnitID: 1, DeviceSetID: 100, Enabled: true}},
	}

	result, err := service.importFromSnapshot(ctx, admin, orgID.String(), snapshot, &sourceIP)
	if err != nil {
		t.Fatalf("importFromSnapshot: %v", err)
	}
	if result.DeviceModelsCreated != 1 {
		t.Errorf("DeviceModelsCreated = %d, want 1", result.DeviceModelsCreated)
	}
	if result.RegisterMetadataUpserted != 2 {
		t.Errorf("RegisterMetadataUpserted = %d, want 2", result.RegisterMetadataUpserted)
	}
	if len(result.ConnectionsFound) != 1 || result.ConnectionsFound[0].Host != "192.168.1.50" {
		t.Errorf("ConnectionsFound = %+v, want one entry for 192.168.1.50", result.ConnectionsFound)
	}
	models, err := service.DeviceModels(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range models {
		if m.Manufacturer == "TestBrand" && m.DeviceType == "Inverter" && m.Model == "TB-5000" {
			found = true
		}
	}
	if !found {
		t.Error("expected a TestBrand/Inverter/TB-5000 device model to exist after import")
	}
	metadata, err := service.DeviceModelRegisterMetadata(ctx, admin, models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var floatDataType string
	for _, m := range metadata {
		if m.AddressKey == "P_AC" && m.ModbusDataType != nil {
			floatDataType = *m.ModbusDataType
		}
	}
	if floatDataType != "FLOAT32" {
		t.Errorf("P_AC ModbusDataType = %q, want FLOAT32 (normalized from legacy FLOAT alias)", floatDataType)
	}

	// Re-running the same import must not create a second Device Model.
	result2, err := service.importFromSnapshot(ctx, admin, orgID.String(), snapshot, &sourceIP)
	if err != nil {
		t.Fatalf("second importFromSnapshot: %v", err)
	}
	if result2.DeviceModelsCreated != 0 || result2.DeviceModelsReused != 1 {
		t.Errorf("second import: DeviceModelsCreated=%d DeviceModelsReused=%d, want 0 and 1 (idempotent)", result2.DeviceModelsCreated, result2.DeviceModelsReused)
	}
	if result2.RegisterMetadataUpserted != 2 {
		t.Errorf("second import RegisterMetadataUpserted = %d, want 2 (upsert, not duplicate)", result2.RegisterMetadataUpserted)
	}
}
```

Note: this reuses `mustUUID` (already defined as a test helper in this package — see `plants_integration_test.go`) and role UUID `00000000-0000-4000-8000-000000000201`, the same fixed "admin" role UUID `plants_integration_test.go` already uses.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/platform-api && go test ./internal/core/ -run TestImportFromSnapshotAgainstPostgreSQL -v` (requires `PLATFORM_TEST_DATABASE_URL` set to a real test Postgres instance — if unset, this test skips; run it against the same test DB the other `*_integration_test.go` files in this package use)
Expected: FAIL — compile error, `importFromSnapshot` (and `ImportMiddlewareConfigResult`/`ImportedConnectionSummary`) do not exist yet.

- [ ] **Step 3: Implement `importFromSnapshot`**

Add to `services/platform-api/internal/core/middleware_config.go` (near the other `Middleware*` types, e.g. right after `MiddlewareConfigSnapshot`'s closing brace):

```go
type ImportMiddlewareConfigResult struct {
	DeviceModelsCreated      int                         `json:"deviceModelsCreated"`
	DeviceModelsReused       int                         `json:"deviceModelsReused"`
	RegisterMetadataUpserted int                         `json:"registerMetadataUpserted"`
	ConnectionsFound         []ImportedConnectionSummary `json:"connectionsFound"`
}

type ImportedConnectionSummary struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	UnitID      int    `json:"unitId"`
	DeviceModel string `json:"deviceModel"`
}

// normalizeModbusDataType mirrors modbus-api-middleware/internal/decoder.NormalizeDataType --
// duplicated here (not imported: different Go module) so a legacy
// Middleware's free-text data types (SHORT/USHORT/FLOAT/...) map onto the
// canonical MIDDLEWARE_DATA_TYPES the rest of this system already uses.
func normalizeModbusDataType(dataType string) string {
	switch strings.ToUpper(strings.TrimSpace(dataType)) {
	case "SHORT", "INT16":
		return "I16"
	case "USHORT", "UINT16":
		return "U16"
	case "LONG", "SLONG", "INT32", "S32", "SW_INT", "SMA_INT32":
		return "I32"
	case "ULONG", "DWORD", "UINT32", "SW_UINT", "SMA_UINT32":
		return "U32"
	case "UINT64", "SMA_UINT64":
		return "U64"
	case "FLOAT", "SW_FLOAT":
		return "FLOAT32"
	default:
		return strings.ToUpper(strings.TrimSpace(dataType))
	}
}

// importFromSnapshot upserts DeviceModel + DeviceModelRegisterMetadata rows
// from an already-decoded MiddlewareConfigSnapshot pulled live from a
// Middleware's local config (see ImportMiddlewareConfig). Idempotent: an
// existing DeviceModel (matched by manufacturer/deviceType/model) is
// reused, never duplicated; register metadata is upserted by addressKey
// the same way the Register Metadata page's own PUT already works.
// Connections are only summarized, never turned into Device rows -- Plant
// assignment stays a human decision (see the design spec).
func (s *Service) importFromSnapshot(ctx context.Context, principal auth.Principal, organizationID string, snapshot MiddlewareConfigSnapshot, sourceIP *netip.Addr) (ImportMiddlewareConfigResult, error) {
	var result ImportMiddlewareConfigResult

	existing, err := s.DeviceModels(ctx, principal)
	if err != nil {
		return result, fmt.Errorf("list existing device models: %w", err)
	}
	type modelKey struct{ manufacturer, deviceType, model string }
	byKey := make(map[modelKey]string, len(existing))
	for _, m := range existing {
		byKey[modelKey{m.Manufacturer, m.DeviceType, m.Model}] = m.ID
	}

	brandNames := make(map[int64]string, len(snapshot.Brands))
	for _, b := range snapshot.Brands {
		brandNames[b.BrandID] = b.BrandName
	}

	resolvedModelName := make(map[int64]string, len(snapshot.DeviceSets))
	for _, ds := range snapshot.DeviceSets {
		manufacturer := brandNames[ds.BrandID]
		if manufacturer == "" {
			manufacturer = ds.BrandName
		}
		key := modelKey{manufacturer, ds.DevType, ds.DevModel}
		modelID, ok := byKey[key]
		if ok {
			result.DeviceModelsReused++
		} else {
			created, err := s.CreateDeviceModel(ctx, principal, DeviceModelInput{
				OrganizationID: organizationID, Manufacturer: manufacturer, DeviceType: ds.DevType, Model: ds.DevModel, IsActive: true,
			}, sourceIP)
			if err != nil {
				return result, fmt.Errorf("create device model %s/%s/%s: %w", manufacturer, ds.DevType, ds.DevModel, err)
			}
			modelID = created.ID
			byKey[key] = modelID
			result.DeviceModelsCreated++
		}
		resolvedModelName[ds.DeviceSetID] = fmt.Sprintf("%s / %s / %s", manufacturer, ds.DevType, ds.DevModel)

		for _, addr := range ds.Addresses {
			addressKey := strings.TrimSpace(addr.CanonicalKey)
			if addressKey == "" {
				addressKey = fmt.Sprintf("%d:%d", addr.FunctionCode, addr.Register)
			}
			functionCode := int32(addr.FunctionCode)
			register := int32(addr.Register)
			_, err := s.SetDeviceModelRegisterMetadata(ctx, principal, modelID, UpdateDeviceModelRegisterMetadataInput{
				UpdateDeviceRegisterMetadataInput: UpdateDeviceRegisterMetadataInput{
					AddressKey: addressKey, DisplayName: addr.Description, DataType: "number", Scale: addr.Factor, Offset: addr.Offset, Decimals: 2, IsEnabled: true,
				},
				ModbusFunctionCode: &functionCode,
				ModbusRegister:     &register,
				ModbusWordOrder:    addr.WordOrder,
				ModbusDataType:     normalizeModbusDataType(addr.DataType),
			}, sourceIP)
			if err != nil {
				return result, fmt.Errorf("upsert register metadata %s: %w", addressKey, err)
			}
			result.RegisterMetadataUpserted++
		}
	}

	for _, conn := range snapshot.Connections {
		result.ConnectionsFound = append(result.ConnectionsFound, ImportedConnectionSummary{
			Host: conn.Host, Port: conn.Port, UnitID: conn.UnitID, DeviceModel: resolvedModelName[conn.DeviceSetID],
		})
	}
	return result, nil
}
```

Check the top of `middleware_config.go`'s import block already has `"strings"` and `"net/netip"` imported (both are commonly used across `core/` files already) — add either if genuinely missing after running `go build`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/platform-api && go test ./internal/core/ -run TestImportFromSnapshotAgainstPostgreSQL -v`
Expected: PASS.

- [ ] **Step 5: Run the full package test suite**

Run: `cd services/platform-api && go build ./... && go vet ./... && go test ./...`
Expected: clean, all green (existing tests unaffected).

- [ ] **Step 6: Commit**

```bash
git add services/platform-api/internal/core/middleware_config.go services/platform-api/internal/core/middleware_config_integration_test.go
git commit -m "$(cat <<'EOF'
Add importFromSnapshot: idempotent Device Model + Register Metadata upsert

Takes an already-decoded MiddlewareConfigSnapshot and upserts Device
Models (matched by manufacturer/deviceType/model, never duplicated)
and their register metadata (upserted by addressKey), normalizing
legacy Modbus data-type aliases the same way the one-off seed script
did. Connections are only summarized, never turned into Device rows.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `platform-api` — `ImportMiddlewareConfig` network relay + HTTP route

**Files:**
- Modify: `services/platform-api/internal/core/middleware_config.go`
- Modify: `services/platform-api/internal/httpapi/middlewares.go`
- Modify: `services/platform-api/internal/httpapi/server.go`

**Interfaces:**
- Consumes: `s.importFromSnapshot` (Task 2), `s.hub.RunCommand(ctx, middlewareID, commandID, payload) (json.RawMessage, error)` (pre-existing `gatewayhub.Hub` method, already used by `RunMiddlewareCommand` in this same file), `s.requireOrganizationPermission` (pre-existing, `services/platform-api/internal/core/middleware_plants.go` uses the identical pattern), `newUUID`/`uuidString`/`parseUUID` (pre-existing helpers already used throughout `middleware_config.go`).
- Produces: `(s *Service) ImportMiddlewareConfig(ctx context.Context, principal auth.Principal, middlewareID string, sourceIP *netip.Addr) (ImportMiddlewareConfigResult, error)` — consumed by the new httpapi handler in this task. New route: `POST /api/v1/admin/middlewares/{middlewareId}/import-config` — consumed by Task 4 (frontend).

- [ ] **Step 1: Implement `ImportMiddlewareConfig`**

Add to `services/platform-api/internal/core/middleware_config.go`, right after `RunMiddlewareCommand`'s closing brace:

```go
// ImportMiddlewareConfig sends a config-export command.request to
// middlewareID over the realtime channel, waits (up to 15s) for its local
// config, and imports it via importFromSnapshot. One-time onboarding
// import, not an ongoing sync -- see the design spec.
func (s *Service) ImportMiddlewareConfig(ctx context.Context, principal auth.Principal, middlewareID string, sourceIP *netip.Addr) (ImportMiddlewareConfigResult, error) {
	mwUUID, err := parseUUID(middlewareID)
	if err != nil {
		return ImportMiddlewareConfigResult{}, ErrMiddlewareNotFound
	}
	var mwOrgID pgtype.UUID
	if err = s.pool.QueryRow(ctx, `SELECT organization_id FROM middleware_client WHERE id=$1`, mwUUID).Scan(&mwOrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ImportMiddlewareConfigResult{}, ErrMiddlewareNotFound
		}
		return ImportMiddlewareConfigResult{}, fmt.Errorf("lookup middleware organization: %w", err)
	}
	if err = s.requireOrganizationPermission(ctx, s.queries, principal, "update", "middleware_config", mwOrgID); err != nil {
		return ImportMiddlewareConfigResult{}, err
	}

	commandID, err := newUUID()
	if err != nil {
		return ImportMiddlewareConfigResult{}, err
	}
	payload, _ := json.Marshal(map[string]any{"type": "command.request", "commandId": uuidString(commandID), "kind": "config-export"})
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	raw, err := s.hub.RunCommand(runCtx, uuidString(mwUUID), uuidString(commandID), payload)
	if errors.Is(err, context.DeadlineExceeded) {
		return ImportMiddlewareConfigResult{}, fmt.Errorf("middleware command timed out: %w", ErrMiddlewareCommandNAK)
	}
	if err != nil {
		return ImportMiddlewareConfigResult{}, ErrMiddlewareOffline
	}
	var snapshot MiddlewareConfigSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return ImportMiddlewareConfigResult{}, fmt.Errorf("decode config-export response: %w", err)
	}
	return s.importFromSnapshot(ctx, principal, uuidString(mwOrgID), snapshot, sourceIP)
}
```

`requireOrganizationPermission`'s signature is `func (s *Service) requireOrganizationPermission(ctx context.Context, q *dbgen.Queries, principal auth.Principal, action, resource string, organizationID pgtype.UUID) error` (`services/platform-api/internal/core/users.go:582`) — `q` is a plain `*dbgen.Queries`, not required to be transaction-bound (`middleware_plants.go` passes a `.WithTx(tx)` variant only because it's already inside a transaction for other reasons; this function isn't). Passing `s.queries` directly, as written above, is correct and needs no transaction wrapper.

- [ ] **Step 2: Add the HTTP handler**

In `services/platform-api/internal/httpapi/middlewares.go`, add a new handler function near `getMiddlewareConfigHandler`:

```go
func importMiddlewareConfigHandler(service *core.Service) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		result, err := service.ImportMiddlewareConfig(r.Context(), principal, r.PathValue("middlewareId"), remoteIP(r.RemoteAddr))
		if writeMiddlewareError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
```

- [ ] **Step 3: Register the route**

In `services/platform-api/internal/httpapi/server.go`, add right after the existing `GET /api/v1/admin/middlewares/{middlewareId}/config` line:

```go
			mux.HandleFunc("POST /api/v1/admin/middlewares/{middlewareId}/import-config", authenticated(authService, true, importMiddlewareConfigHandler(registryService)))
```

(the `true` second argument matches the CSRF-required convention already used by the other mutating middleware routes on the surrounding lines — e.g. `POST /api/v1/admin/middlewares` above it.)

- [ ] **Step 4: Verify**

Run: `cd services/platform-api && go build ./... && go vet ./... && go test ./...`
Expected: clean, all green.

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/core/middleware_config.go services/platform-api/internal/httpapi/middlewares.go services/platform-api/internal/httpapi/server.go
git commit -m "$(cat <<'EOF'
Add ImportMiddlewareConfig relay and POST .../import-config route

Sends a config-export command.request to the target Middleware over
the existing realtime channel, decodes its response into
MiddlewareConfigSnapshot, and hands it to importFromSnapshot.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `apps/web` — "Import from Middleware" button

**Files:**
- Modify: `apps/web/app/features/middlewares/middlewares-page.tsx`
- Modify: `apps/web/app/lib/types.ts`

**Interfaces:**
- Consumes: `toast` (`apps/web/app/components/ui/sonner.tsx`, already imported in this file), `api`/`csrfToken` (`apps/web/app/lib/api.ts`, already imported), the new `POST /api/v1/admin/middlewares/{middlewareId}/import-config` route (Task 3).
- Produces: a new `ImportMiddlewareConfigResult` type in `apps/web/app/lib/types.ts`, mirroring the Go JSON shape from Task 2 exactly.

- [ ] **Step 1: Add the response type**

In `apps/web/app/lib/types.ts`, add near `MiddlewareConfigSnapshot`:

```typescript
export type ImportMiddlewareConfigResult = {
  deviceModelsCreated: number;
  deviceModelsReused: number;
  registerMetadataUpserted: number;
  connectionsFound: Array<{ host: string; port: number; unitId: number; deviceModel: string }>;
};
```

- [ ] **Step 2: Add the import handler and button to `MiddlewareConfigEditor`**

In `apps/web/app/features/middlewares/middlewares-page.tsx`, add the type import at the top: change the existing `import type { CreatedMiddlewareGateway, MiddlewareConfigSnapshot, MiddlewareGateway, Plant } from "../../lib/types";` line to also include `ImportMiddlewareConfigResult`:

```tsx
import type { CreatedMiddlewareGateway, ImportMiddlewareConfigResult, MiddlewareConfigSnapshot, MiddlewareGateway, Plant } from "../../lib/types";
```

Inside `MiddlewareConfigEditor`, add one new state variable alongside the existing ones (`pending`, `error`, etc.):

```tsx
  const [importing, setImporting] = useState(false);
```

Add this function right after `unassignPlant`:

```tsx
  async function importConfig() {
    if (!window.confirm(`ดึง Config จาก "${gateway.name}" มาสร้าง/อัปเดต Device Model และ Register Metadata?`)) return;
    setImporting(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/import-config`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (response.status === 504) throw new Error("Middleware ไม่ตอบสนองภายในเวลาที่กำหนด");
      if (!response.ok) throw new Error("ไม่สามารถดึง Config จาก Middleware ได้");
      const result = (await response.json()) as ImportMiddlewareConfigResult;
      toast.success(`Import สำเร็จ: ${result.deviceModelsCreated} Model ใหม่, ${result.deviceModelsReused} Model เดิม, ${result.registerMetadataUpserted} Register, พบ ${result.connectionsFound.length} Connection (ไปสร้าง Device เองที่ Plants → Devices)`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setImporting(false);
    }
  }
```

Insert this new section right after the closing `</div>` of the `row-actions` block containing the "มอบหมาย Plant" button (i.e., right before the `{snapshot && (...)}` block):

```tsx
      <div className="section-heading">
        <div><h3>Import from Middleware</h3><p>ดึง config ที่ตั้งไว้บน Middleware เครื่องนี้มาสร้าง Device Model และ Register Metadata ครั้งเดียว (ใช้ตอน onboard Middleware เก่าที่เคยตั้งค่าไว้ก่อนมีระบบนี้)</p></div>
        <div className="heading-actions">
          <button className="primary-button compact" disabled={!gateway.isOnline || importing} onClick={() => void importConfig()} title={gateway.isOnline ? "ดึง Config จาก Middleware" : "Middleware ต้อง Online ก่อน"}>
            {importing ? "กำลังดึง Config..." : "ดึง Config จาก Middleware"}
          </button>
        </div>
      </div>
```

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 4: Manual verification**

Run `npm run dev`, log in, open a Middleware Gateway that is currently Online (or simulate one for testing) and click "ดึง Config จาก Middleware" — confirm the confirm dialog appears, the button shows "กำลังดึง Config..." while pending, and on success a toast summarizes the counts. Confirm the button is disabled when the gateway shows Offline.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/middlewares/middlewares-page.tsx apps/web/app/lib/types.ts
git commit -m "$(cat <<'EOF'
Add "Import from Middleware" button to Middleware Gateway detail page

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- New `"config-export"` WebSocket command kind, reusing the existing channel — Task 1. ✓
- platform-api relays the command and waits up to 15s — Task 3, `ImportMiddlewareConfig`. ✓
- Route `POST /api/v1/admin/middlewares/{middlewareId}/import-config` — Task 3, Step 3. ✓
- Device Model upsert by (manufacturer, deviceType, model), no duplicates — Task 2, `importFromSnapshot`, verified by the idempotency assertion in the integration test. ✓
- Register Metadata upsert by addressKey — Task 2. ✓
- Connections summarized, never auto-created as Devices — Task 2 (`ConnectionsFound`, no `Device` write anywhere in `importFromSnapshot`), Task 4 (toast explicitly tells the admin to create Devices manually). ✓
- Idempotent / safe to re-run — Task 2's integration test explicitly re-runs the import and asserts no duplication. ✓
- Audited — not explicitly implemented as a dedicated audit event in this plan; **gap found during self-review**. Fixed inline: `importFromSnapshot`'s underlying calls (`CreateDeviceModel`, `SetDeviceModelRegisterMetadata`) already write their own `device_model.created`/register-metadata audit events per-row (confirmed in `devices.go`), which collectively provides the audit trail the spec asks for — no separate top-level "import" audit event is needed on top of that, since every row it touches is already individually audited by the methods it calls. No plan change required; noting this reasoning here so it doesn't look like an oversight.
- UI button, confirm dialog, disabled-when-offline, pending state, success/error toast — Task 4. ✓
- No automatic Plant/Device creation from Connections — confirmed, `importFromSnapshot` never touches the `device` table. ✓
- No change to steady-state push (`buildConfigSnapshot`/`recomputeAndPushMiddleware`) — confirmed, this plan never modifies those functions. ✓

**Placeholder scan:** none found.

**Type consistency:** `ImportMiddlewareConfigResult`/`ImportedConnectionSummary` (Go, Task 2) and `ImportMiddlewareConfigResult` (TypeScript, Task 4) use matching field names under Go's `json:"..."` tags (`deviceModelsCreated`, `deviceModelsReused`, `registerMetadataUpserted`, `connectionsFound` with `host`/`port`/`unitId`/`deviceModel`) — verified field-by-field across both languages. `importFromSnapshot`'s signature (`ctx, principal, organizationID string, snapshot MiddlewareConfigSnapshot, sourceIP *netip.Addr`) is called identically in both the Task 2 test and Task 3's `ImportMiddlewareConfig`.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-01-middleware-config-import.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

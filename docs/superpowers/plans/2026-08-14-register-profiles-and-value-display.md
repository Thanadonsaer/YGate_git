# Register Profiles and Interpreted Telemetry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let Device Models share Register Profiles, decode exact and bitmask values centrally, show interpreted values throughout the UI, and export raw/numeric/display values together.

**Architecture:** Profiles own reusable register definitions and mappings; Device Models reference one Profile. Raw telemetry remains unchanged. A central resolver returns numeric `dataItemMap` plus additive `displayItemMap`, and is reused by latest/history/alarm evaluation. Existing model metadata is migrated into one profile per model without changing current behavior.

**Tech Stack:** Go, PostgreSQL/pgx, SQL JSONB functions, net/http, React/TypeScript, CSV, OpenAPI.

**Spec:** `docs/superpowers/specs/2026-08-14-register-profiles-alarm-decoding-login-status-design.md`

## Global Constraints

- Never overwrite or stringify numeric telemetry.
- Device override remains the highest-precedence presentation override.
- Exact mappings may be used by any register; Normal/Alarm/Severity semantics are accepted only for alarm-enabled mappings.
- Bitmask matching emits all active bits in deterministic bit order.
- Unknown values fall back to the formatted numeric value.
- No firmware-version layer and no second interpreted telemetry store.

---

### Task 1: Create and migrate Register Profile storage

**Files:**
- Create: `services/platform-api/internal/database/migrations/000050_register_profiles.sql`
- Modify: `services/platform-api/internal/database/database_test.go`
- Test: `services/platform-api/internal/database/register_profiles_integration_test.go`

**Schema interfaces:**

```text
registry.register_profile(id, organization_id, name, manufacturer, description, created_at, updated_at)
registry.register_profile_address(id, profile_id, address_key, presentation/modbus fields, is_alarm, mapping_mode)
registry.register_value_mapping(id, profile_address_id, match_value, bit_index, display_value, alarm_state, severity, sort_order)
registry.device_model.register_profile_id
```

- [ ] Add failing migration catalog and database integration tests for tenant uniqueness, address uniqueness, exact-vs-bitmask checks, alarm semantic checks, and model/profile organization consistency.
- [ ] Run `go test ./internal/database -run 'Migration|RegisterProfile' -count=1`; expect failure.
- [ ] Add migration 50. For every model containing metadata, create a same-organization profile, copy its addresses, assign the model, and retain compatibility data until application cutover is complete.
- [ ] Add indexes for profile listing, model lookup, and address/mapping resolution.
- [ ] Run database tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal/database && git commit -m "feat: add reusable register profiles"`.

### Task 2: Implement profile CRUD and model assignment

**Files:**
- Create: `services/platform-api/internal/core/register_profiles.go`
- Create: `services/platform-api/internal/core/register_profiles_test.go`
- Create: `services/platform-api/internal/httpapi/register_profiles.go`
- Modify: `services/platform-api/internal/httpapi/server.go`
- Test: `services/platform-api/internal/httpapi/register_profiles_test.go`
- Modify: `packages/api-contracts/platform-api.yaml`

**HTTP interfaces:**

```text
GET/POST       /api/v1/register-profiles
GET/PUT/DELETE /api/v1/register-profiles/{profileId}
GET/PUT/DELETE /api/v1/register-profiles/{profileId}/addresses/{addressKey}
PUT            /api/v1/device-models/{modelId}/register-profile
```

```go
type RegisterValueMapping struct {
    ID, DisplayValue, AlarmState, Severity string
    MatchValue *int64
    BitIndex *int32
}
```

- [ ] Write core tests for RBAC, organization scoping, normalized names/address keys, exact mapping uniqueness, bit range, deterministic order, and forbidden alarm fields on normal mappings.
- [ ] Write HTTP tests for CRUD, assignment, validation responses, and deletion conflict while a model uses the profile.
- [ ] Run focused tests; expect failure.
- [ ] Implement minimal transactional CRUD, audit events, and routes; reuse existing device-model permissions rather than adding a speculative permission family.
- [ ] Update OpenAPI schemas and paths, then run core, HTTP, and OpenAPI contract tests.
- [ ] Commit: `git add services/platform-api/internal/core/register_profiles* services/platform-api/internal/httpapi packages/api-contracts/platform-api.yaml && git commit -m "feat: manage register profiles"`.

### Task 3: Build the shared value resolver

**Files:**
- Create: `services/platform-api/internal/core/register_resolver.go`
- Create: `services/platform-api/internal/core/register_resolver_test.go`

**Interfaces:**

```go
type ResolvedRegister struct {
    AddressKey string
    NumericValue float64
    DisplayValue string
    Matches []ResolvedValueMatch
}

func ResolveRegister(def RegisterDefinition, raw float64) ResolvedRegister
```

- [ ] Write table tests for scale/offset, exact hit, exact miss, multiple bit hits, zero bitmask, signed/invalid inputs, display fallback, decimal/unit formatting, and deterministic match ordering.
- [ ] Run `go test ./internal/core -run ResolveRegister -count=1`; expect compile failure.
- [ ] Implement a pure resolver with no database calls. Apply scale/offset once, match exact mappings on the configured integral value, and match bitmasks by configured bit indexes.
- [ ] Keep alarm state in match results for the next plan, but do not create events here.
- [ ] Run focused tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal/core/register_resolver* && git commit -m "feat: resolve register display mappings"`.

### Task 4: Cut existing metadata and middleware config over to Profiles

**Files:**
- Modify: `services/platform-api/internal/core/devices.go`
- Modify: `services/platform-api/internal/core/devices_test.go`
- Modify: `services/platform-api/internal/httpapi/devices.go`
- Modify: `services/platform-api/internal/core/middleware_config.go`
- Modify: `services/platform-api/internal/core/middleware_config_test.go`
- Modify: `apps/web/app/lib/telemetry-history.ts`

- [ ] Add compatibility tests proving `DeviceModelRegisterMetadata`, `DeviceRegisterMetadata`, and middleware snapshots return the assigned Profile addresses and retain device override precedence.
- [ ] Add a model-without-profile test returning an empty metadata set rather than guessing a profile.
- [ ] Run focused tests; expect failure against legacy model-owned rows.
- [ ] Change the existing list/update entry points to resolve through `register_profile_id`; keep their response shape while adding profile/mapping fields so existing callers do not break abruptly.
- [ ] Build middleware Modbus address lists from profile addresses and ensure one profile edit affects every assigned model.
- [ ] Run core tests and frontend telemetry-history tests.
- [ ] Commit: `git add services/platform-api/internal/core services/platform-api/internal/httpapi/devices.go apps/web/app/lib/telemetry-history.ts && git commit -m "refactor: source register metadata from profiles"`.

### Task 5: Add interpreted values to latest and history telemetry

**Files:**
- Create migration: `services/platform-api/internal/database/migrations/000051_telemetry_display_values.sql`
- Modify: `services/platform-api/internal/database/queries/telemetry.sql`
- Regenerate: `services/platform-api/internal/database/dbgen/telemetry.sql.go`
- Modify: `services/platform-api/internal/core/telemetry.go`
- Test: `services/platform-api/internal/core/telemetry_test.go`
- Test: `services/platform-api/internal/core/telemetry_integration_test.go`
- Modify: `apps/web/app/lib/types.ts`
- Modify: `packages/api-contracts/platform-api.yaml`

**API interface:**

```json
{
  "dataItemMap":{"30070":145},
  "displayItemMap":{"30070":"Model A"}
}
```

- [ ] Add integration tests proving latest and history share identical numeric/display resolution, unknown values fall back, and raw rows stay unchanged.
- [ ] Run telemetry tests; expect missing `displayItemMap` failures.
- [ ] Add a database mapping function/view returning numeric and display JSON maps from the assigned Profile. Keep `telemetry.mapped_data_items()` compatible for existing Alarm Rules during this slice.
- [ ] Add `DisplayItemMap map[string]string` to `LatestTelemetry`, scan it for latest/history, update TypeScript and OpenAPI types, and regenerate sqlc with v1.30.0.
- [ ] Run telemetry and contract tests; expect pass.
- [ ] Commit: `git add services/platform-api apps/web/app/lib/types.ts packages/api-contracts/platform-api.yaml && git commit -m "feat: return interpreted telemetry values"`.

### Task 6: Update display surfaces without breaking calculations

**Files:**
- Modify: `apps/web/app/features/plants/plants-page.tsx`
- Modify: `apps/web/app/lib/telemetry-history.ts`
- Modify: `apps/web/app/features/scada/scada-page.tsx`
- Modify: relevant dashboard/report consumers found by `rg -n "dataItemMap" apps/web/app`
- Test: `apps/web/app/lib/telemetry-history.test.ts`
- Test/Create: `apps/web/app/lib/display-value.test.ts`

**Display rule:**

```ts
displayItemMap[addressKey] ?? formatNumeric(dataItemMap[addressKey], metadata)
```

- [ ] Add tests proving Device Detail and shared display helpers prefer interpreted text while charting, aggregation, thresholds, and SCADA numeric transforms still consume `dataItemMap`.
- [ ] Run relevant Node tests; expect failure.
- [ ] Introduce one small shared display helper and replace ad-hoc display formatting at Device Detail, history table, SCADA labels, and status cards. Do not pass strings into numeric chart/report functions.
- [ ] Run `npm test` and `npm run typecheck`; expect pass.
- [ ] Commit: `git add apps/web/app && git commit -m "feat: show interpreted register values"`.

### Task 7: Export and import Profiles with both numeric and display values

**Files:**
- Modify: `services/platform-api/internal/core/csv_transfer.go`
- Test: `services/platform-api/internal/core/csv_transfer_test.go`
- Modify: `services/platform-api/internal/httpapi/csv_transfer.go`
- Modify: `apps/web/app/features/register-metadata/register-metadata-page.tsx`
- Modify: report CSV generator files returned by `rg -l "dataItemMap" apps/web/app/lib apps/web/app/features`

- [ ] Add round-trip tests for profile name, model assignment, mapping mode, exact value/bit, display text, alarm state, severity, and Unicode labels.
- [ ] Add report-export tests requiring adjacent `<point>_numeric` and `<point>_display` columns.
- [ ] Run focused CSV tests; expect header/assertion failures.
- [ ] Extend configuration CSV without removing existing columns. Group repeated mapping rows by profile/address during import and reject mixed exact/bitmask definitions atomically.
- [ ] Extend telemetry/report export to write both numeric and display values.
- [ ] Run Go CSV tests and web report tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal apps/web/app && git commit -m "feat: import and export register value mappings"`.

### Task 8: Replace the metadata editor with a Profile editor

**Files:**
- Modify: `apps/web/app/features/register-metadata/register-metadata-page.tsx`
- Modify: `apps/web/app/lib/types.ts`
- Test/Create: `apps/web/app/features/register-metadata/register-metadata-page.test.tsx`

- [ ] Add UI tests for profile CRUD, assigning multiple models, exact mapping rows, bitmask rows, alarm-only fields, validation, and CSV actions.
- [ ] Run the component test; expect failure.
- [ ] Add Profile selection/assignment above the existing address table; adapt the dialog to edit mappings and conditionally show Normal/Alarm/Severity only when `isAlarm` is enabled.
- [ ] Prevent mixed exact/bit rows client-side while treating server validation as authoritative.
- [ ] Run component tests and `npm run typecheck`; expect pass.
- [ ] Commit: `git add apps/web/app/features/register-metadata apps/web/app/lib/types.ts && git commit -m "feat: edit reusable register profiles"`.

### Task 9: Verify the Register Profile slice

- [ ] Run `go test ./...` in platform-api.
- [ ] Run `npm test`, `npm run typecheck`, and `npm run build` in apps/web.
- [ ] Manually verify one Profile assigned to two Huawei models produces the same middleware addresses and `145 -> Model A` display.
- [ ] Inspect `git diff --check` and confirm raw telemetry schema/data were not duplicated.


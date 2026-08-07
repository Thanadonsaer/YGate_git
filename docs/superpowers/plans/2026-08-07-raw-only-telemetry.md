# Raw-only Telemetry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task with verification checkpoints.

**Goal:** Make `telemetry.raw_register_reading` the only telemetry history source while preserving current APIs and deriving display values from Register Metadata.

**Architecture:** Add shared Raw latest/history read SQL in Platform API. Plant, Dashboard, SCADA, Energy Analysis, and alarm evaluation use this read model. Keep `telemetry_latest` and `telemetry_reading` during the first migration for rollback; stop writing/reading them as authoritative before a later removal migration.

**Tech Stack:** Go, PostgreSQL migrations, pgx/sqlc-generated queries, existing Platform API tests.

## Global Constraints

- Raw register values must remain unchanged in `telemetry.raw_register_reading`.
- Factor/Offset are applied only at read/calculation time from Register Metadata.
- Latest ordering is `observed_at DESC, received_at DESC, id DESC` per device.
- Device metadata overrides model metadata; disabled metadata is excluded from calculated output.
- The first migration is additive/reversible and must not drop telemetry tables.

---

### Task 1: Add shared Raw read model

**Files:**
- Create: `services/platform-api/internal/core/raw_telemetry.go`
- Test: `services/platform-api/internal/core/raw_telemetry_test.go`

- [ ] Add `rawLatestSQL` and `rawHistorySQL` helpers that select Raw rows and calculate values with device metadata priority over model metadata.
- [ ] Add unit tests for metadata precedence, disabled metadata, default scale/offset, and latest ordering.
- [ ] Run `go test ./internal/core`.

### Task 2: Move Plant latest and History to Raw

**Files:**
- Modify: `services/platform-api/internal/core/telemetry.go`
- Test: `services/platform-api/internal/ingestion/raw_service_integration_test.go`

- [ ] Make `LatestTelemetry` use the shared Raw latest read model as its primary result.
- [ ] Make device history use Raw rows and apply metadata at query time.
- [ ] Keep response field names unchanged for Web compatibility.
- [ ] Verify a Raw-only ingest returns calculated values and updated `observedAt` without relying on `telemetry_reading`.
- [ ] Run `go test ./internal/core ./internal/ingestion`.

### Task 3: Move Dashboard status to Raw

**Files:**
- Modify: `services/platform-api/internal/database/queries/dashboard.sql`
- Modify: `services/platform-api/internal/core/dashboard.go`
- Modify: `services/platform-api/internal/database/dbgen/dashboard.sql.go` if query regeneration is unavailable.
- Test: `services/platform-api/internal/core/dashboard_test.go`

- [ ] Replace `telemetry_latest` joins with a per-device Raw latest subquery.
- [ ] Count reporting/stale/offline devices from Raw latest timestamps.
- [ ] Preserve dashboard response shape and stale threshold behavior.
- [ ] Run dashboard unit/integration tests.

### Task 4: Move alarm evaluation and cleanup references

**Files:**
- Modify: `services/platform-api/internal/ingestion/raw_service.go`
- Modify: `services/platform-api/internal/core/alarms.go`
- Modify: `services/platform-api/internal/core/hard_delete.go`
- Test: existing alarm and hard-delete integration tests.

- [ ] Evaluate alarms from the same metadata-mapped Raw values used by Plant/SCADA.
- [ ] Remove new dependencies on `telemetry_reading` and `telemetry_latest` from cleanup paths while preserving deletion scope.
- [ ] Run alarm, hard-delete, and ingestion tests.

### Task 5: Stop treating legacy telemetry tables as authoritative

**Files:**
- Modify: `services/platform-api/internal/ingestion/service.go`
- Modify: `services/platform-api/internal/database/queries/ingestion.sql`
- Modify: `services/platform-api/internal/database/database_test.go`
- Create: `services/platform-api/internal/database/migrations/000037_raw_only_read_model.sql`

- [ ] Stop writing calculated telemetry into `telemetry_reading` for new Raw ingestion.
- [ ] Keep the compatibility projection only if required by rollback/legacy APIs, clearly marked non-authoritative.
- [ ] Add a reversible migration/view/read-model definition and a verification query for Raw latest coverage.
- [ ] Do not drop existing telemetry tables in this phase.
- [ ] Run migration/database tests.

### Task 6: Full verification and handoff

**Files:**
- Modify: `docs/superpowers/specs/2026-08-07-raw-only-telemetry-design.md` if implementation decisions change.

- [ ] Run `go test ./...` in Platform API.
- [ ] Run `go test ./...` in Middleware.
- [ ] Run Web typecheck/build.
- [ ] Verify no production code path requires `telemetry_reading` for new Raw data.
- [ ] Report deployment order and the later table-removal prerequisites.
# Register-Decoded Alarms and Plant Email Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Convert alarm-enabled Register Profile mappings into normal Alarm Log events with realtime/ack support and optional Plant-scoped Role email notifications.

**Architecture:** Ingestion resolves alarm mappings inside the telemetry transaction. Exact mode has one active code: changing code clears the old event and opens a new event. Bitmask mode tracks one open event per active mapping bit. Events snapshot mapping identity/text/raw/display/severity. Existing threshold rules remain unchanged. Email reuses the current post-commit asynchronous mailer and is gated by Plant settings.

**Tech Stack:** Go, PostgreSQL/pgx, existing ingestion and mailer, net/http realtime polling, React/TypeScript, OpenAPI.

**Spec:** `docs/superpowers/specs/2026-08-14-register-profiles-alarm-decoding-login-status-design.md`

## Global Constraints

- Register alarms are Alarm Log events, not Event Logbook entries.
- A repeated identical alarm state never creates a duplicate event or email.
- Exact code transition closes the previous mapping event and opens the new one in the same transaction.
- Bitmask transitions independently open/close each mapped bit.
- Historical event wording must not change after Profile edits/deletes.
- Email defaults off and is sent only for newly opened events after commit; no clear email and no outbox in this scope.

---

### Task 1: Extend Alarm Event provenance and Plant notification settings

**Files:**
- Create: `services/platform-api/internal/database/migrations/000052_register_alarm_events.sql`
- Modify: `services/platform-api/internal/database/database_test.go`
- Test/Create: `services/platform-api/internal/database/register_alarm_events_integration_test.go`

**Schema interface:**

```text
alarm.alarm_event.source_type = RULE | REGISTER
alarm_rule_id nullable
register_mapping_source_id uuid nullable (copied identity, no FK to mutable profile rows)
register_snapshot jsonb nullable
plant.alarm_email_enabled boolean NOT NULL DEFAULT false
plant.alarm_notify_role_id uuid nullable
```

- [ ] Add failing tests for source-specific checks, immutable snapshots, partial open-event uniqueness, and Plant notify-role organization consistency.
- [ ] Run database tests; expect failure because migration 52 is missing.
- [ ] Add migration that preserves every rule event as `RULE`, makes `alarm_rule_id` nullable, replaces the old uniqueness index with source-specific partial indexes, and adds Plant settings.
- [ ] Use one open REGISTER event key per `(device_id, register_mapping_source_id)`.
- [ ] Run database tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal/database && git commit -m "feat: store decoded register alarm events"`.

### Task 2: Model decoded alarm signals as a pure transition

**Files:**
- Create: `services/platform-api/internal/ingestion/register_alarms.go`
- Create: `services/platform-api/internal/ingestion/register_alarms_test.go`
- Reuse: `services/platform-api/internal/core/register_resolver.go`

**Interfaces:**

```go
type registerAlarmSignal struct {
    MappingSourceID string
    AddressKey string
    RawValue float64
    DisplayValue string
    Severity string
}

func registerAlarmTransitions(open, current []registerAlarmSignal) (toClose, toOpen []registerAlarmSignal)
```

- [ ] Write table tests for normal exact value, first alarm, repeated alarm, exact A-to-B transition, return to normal, multiple bit activation, one-bit clear, unknown code, and unmapped bits.
- [ ] Run `go test ./internal/ingestion -run RegisterAlarmTransitions -count=1`; expect compile failure.
- [ ] Implement deterministic set-difference transition logic. Treat mappings marked `NORMAL` as no alarm signal; accept only configured alarm severities.
- [ ] Run focused tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal/ingestion/register_alarms* && git commit -m "feat: compute register alarm transitions"`.

### Task 3: Evaluate Register alarms during ingestion

**Files:**
- Modify: `services/platform-api/internal/ingestion/service.go`
- Modify: `services/platform-api/internal/ingestion/alarms.go`
- Modify/Create: `services/platform-api/internal/ingestion/alarms_integration_test.go`

- [ ] Add integration tests that ingest exact and bitmask readings and assert open/clear timestamps, exact A-to-B close/open ordering, snapshot contents, duplicate suppression, coexistence with Alarm Rules, and transaction rollback safety.
- [ ] Run `go test ./internal/ingestion -run 'RegisterAlarm|Alarm' -count=1`; expect failures.
- [ ] Load alarm-enabled definitions for the device's assigned Profile in the ingestion transaction, resolve current signals, lock open REGISTER events for the device, close stale signals, and insert newly active signals.
- [ ] Snapshot profile/address/mapping IDs, address, raw value, numeric value, display text, severity, Profile name, and Device Model identity into `register_snapshot`.
- [ ] Append newly opened Register events to the same post-commit breach collection used by rule email; never send inside the transaction.
- [ ] Run ingestion tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal/ingestion && git commit -m "feat: open alarms from register metadata"`.

### Task 4: Make Alarm APIs source-aware without breaking ack/realtime

**Files:**
- Modify: `services/platform-api/internal/core/alarms.go`
- Test: `services/platform-api/internal/core/alarms_test.go`
- Test: `services/platform-api/internal/core/alarms_integration_test.go`
- Modify: `services/platform-api/internal/httpapi/alarms.go`
- Test: `services/platform-api/internal/httpapi/alarms_test.go`
- Modify: `services/platform-api/internal/httpapi/realtime.go`
- Test: `services/platform-api/internal/httpapi/realtime_test.go`
- Modify: `packages/api-contracts/platform-api.yaml`

**API interface:**

```go
type AlarmEvent struct {
    SourceType string `json:"sourceType"`
    AlarmRuleID *string `json:"alarmRuleId,omitempty"`
    RegisterSnapshot *RegisterAlarmSnapshot `json:"registerSnapshot,omitempty"`
    // existing fields unchanged
}
```

- [ ] Add tests for mixed RULE/REGISTER listing, source-specific JSON, chronological ordering, ack on both sources, and realtime delivery of Register events.
- [ ] Run focused core/HTTP tests; expect scan/schema failures.
- [ ] Update every event SELECT/RETURNING and `scanAlarmEvent`; preserve existing condition snapshots for RULE events and use register snapshots only for REGISTER events.
- [ ] Keep `LatestAlarmEventID`/`AlarmEventsSince` source-agnostic so realtime works without a second channel.
- [ ] Update OpenAPI and run contract tests.
- [ ] Commit: `git add services/platform-api/internal/core/alarms.go services/platform-api/internal/httpapi packages/api-contracts/platform-api.yaml && git commit -m "feat: expose register alarms in alarm log"`.

### Task 5: Add Plant-scoped email settings

**Files:**
- Modify: `services/platform-api/internal/core/plants.go`
- Modify: `services/platform-api/internal/database/queries/core.sql`
- Regenerate: `services/platform-api/internal/database/dbgen/core.sql.go`
- Test: `services/platform-api/internal/core/plants_test.go`
- Modify: `services/platform-api/internal/httpapi/plants.go`
- Test: `services/platform-api/internal/httpapi/plants_test.go`
- Modify: `apps/web/app/lib/types.ts`
- Modify: `packages/api-contracts/platform-api.yaml`

**Interfaces:**

```json
{"alarmEmailEnabled":false,"alarmNotifyRoleId":null}
```

- [ ] Add tests for default-off, enabling with a valid organization Role, rejecting cross-organization Role IDs, disabling while retaining/clearing the role according to request, RBAC, and audit output.
- [ ] Run Plant tests; expect missing fields.
- [ ] Add settings to Plant read/create/update inputs and SQL, validate role scope transactionally, regenerate sqlc v1.30.0, and update API/types.
- [ ] Keep existing per-rule notify role behavior for RULE events; Plant settings govern decoded REGISTER events.
- [ ] Run core/HTTP/OpenAPI tests; expect pass.
- [ ] Commit: `git add services/platform-api apps/web/app/lib/types.ts packages/api-contracts/platform-api.yaml && git commit -m "feat: configure plant alarm email"`.

### Task 6: Send decoded-alarm email only for newly opened events

**Files:**
- Modify: `services/platform-api/internal/ingestion/alarms.go`
- Test: `services/platform-api/internal/ingestion/alarms_integration_test.go`
- Test/Create: `services/platform-api/internal/ingestion/alarm_email_test.go`
- Reuse: `services/platform-api/internal/notify/*`

- [ ] Add fake-mailer tests for disabled Plant, no Role, no recipients, first open, repeated same state, exact code transition, bit activation, clear, SMTP failure, and active scoped/unscoped Role recipients.
- [ ] Run email tests; expect failures for Register events.
- [ ] Extend `alarmBreach` with source/snapshot display data. Before sending REGISTER mail, load Plant `alarm_email_enabled` and `alarm_notify_role_id`; reuse `alarmNotifyRecipients` and the existing 20-second detached context.
- [ ] Render Plant, Device, register/address, raw/numeric value, decoded text, and severity. Log mail failures without failing ingestion.
- [ ] Run ingestion tests; expect pass.
- [ ] Commit: `git add services/platform-api/internal/ingestion && git commit -m "feat: email newly opened register alarms"`.

### Task 7: Update Alarm and Plant UI

**Files:**
- Modify: `apps/web/app/features/alarms/alarms-page.tsx`
- Modify: `apps/web/app/features/plants/plants-page.tsx`
- Modify: `apps/web/app/lib/types.ts`
- Test/Create: `apps/web/app/features/alarms/alarms-page.test.tsx`
- Test/Create: `apps/web/app/features/plants/plant-alarm-settings.test.tsx`

- [ ] Add Alarm page tests for source label, decoded register text/raw value, severity, open/cleared state, and acknowledgement.
- [ ] Add Plant settings tests for toggle default-off, Role selector, save validation, and permissions.
- [ ] Run component tests; expect failure.
- [ ] Render RULE rows as today and REGISTER rows from immutable snapshot. Add Plant “Alarm email” toggle and notify Role selector using the existing notify-role endpoint/options.
- [ ] Do not add a second Alarm Log or Event Logbook row.
- [ ] Run `npm test` and `npm run typecheck`; expect pass.
- [ ] Commit: `git add apps/web/app && git commit -m "feat: manage decoded alarm notifications"`.

### Task 8: End-to-end verification

- [ ] Run `go test ./...` in platform-api.
- [ ] Run `npm test`, `npm run typecheck`, and `npm run build` in apps/web.
- [ ] With a test Profile, ingest exact values `0 -> 145 -> 145 -> 146 -> 0`; verify two events, first cleared on transition, second cleared on normal, and exactly two emails when enabled.
- [ ] Ingest bitmask `0 -> 3 -> 2 -> 0`; verify two events, bit 0 clears while bit 1 remains open, then bit 1 clears.
- [ ] Edit mapping display text after events exist; verify historical Alarm Log retains its original snapshot.
- [ ] Inspect `git diff --check` and confirm threshold Alarm Rules still pass unchanged.

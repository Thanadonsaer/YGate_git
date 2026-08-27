# Middleware RUT906 Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a pure-Go file storage backend for the RUT906/MIPS build while retaining SQLite on Windows and preserving middleware behavior/configuration.

**Architecture:** Keep the existing store-facing methods used by the application. Split platform implementations with build tags: SQLite remains the host/default backend; the MIPS backend uses a versioned in-memory state document persisted by atomic temp-file rename under a mutex. The MIPS build excludes SQLite and the update bridge until the same backend is ported.

**Tech Stack:** Go 1.23, standard library JSON, file locking/mutex, existing domain types, existing SQLite backend.

**Spec:** `docs/superpowers/specs/2026-08-19-middleware-rut906-storage-design.md`

## Global Constraints

- Windows continues to use `middleware.db` through SQLite.
- RUT906 uses a pure-Go file backend with no CGO or external database.
- Existing routes, domain types, configuration fields, retry semantics, and callers remain unchanged.
- File writes are atomic and recoverable after interruption.
- MIPS builds must not import `modernc.org/sqlite`.

### Task 1: Define the backend boundary and MIPS build target

**Files:**
- Modify: `modbus-api-middleware/cmd/middleware/main.go`
- Modify: `modbus-api-middleware/internal/store/open.go`
- Modify: `modbus-api-middleware/internal/store/sqlite.go`
- Create: `modbus-api-middleware/internal/store/backend_mips.go`
- Test: `modbus-api-middleware/internal/store/backend_test.go`

**Interfaces:**
- `OpenNormalized(path string) (*Store, error)` remains the caller-facing constructor.
- MIPS `Store` exposes the same methods consumed by existing packages.
- The `-db` flag keeps its meaning and defaults to a MIPS file-store path only on MIPS.

- [ ] Write a compile/test probe proving the MIPS build does not select `modernc.org/sqlite` and that `OpenNormalized` can initialize a file store.
- [ ] Run the probe and confirm it fails before the backend implementation exists.
- [ ] Add build-tagged backend selection and the MIPS state/document types without changing Windows SQLite behavior.
- [ ] Run host tests and a `GOOS=linux GOARCH=mips CGO_ENABLED=0 go test`/build probe.

### Task 2: Port configuration and domain persistence

**Files:**
- Create: `modbus-api-middleware/internal/store/file_configuration_mips.go`
- Create: `modbus-api-middleware/internal/store/file_domain_mips.go`
- Test: `modbus-api-middleware/internal/store/file_configuration_mips_test.go`

- [ ] Add failing round-trip tests for gateway config, brands, device sets, addresses, plants, connections, profiles, and devices.
- [ ] Implement the minimal state operations using the existing domain structs and ID allocation rules.
- [ ] Verify web/config cache callers can rebuild the same configuration shape as SQLite.

### Task 3: Port outbox and logs

**Files:**
- Create: `modbus-api-middleware/internal/store/file_outbox_mips.go`
- Create: `modbus-api-middleware/internal/store/file_logs_mips.go`
- Test: `modbus-api-middleware/internal/store/file_outbox_mips_test.go`

- [ ] Add failing tests for idempotent enqueue, ready ordering, retry backoff state, delivered state, delivery logs, poll logs, and cleanup retention.
- [ ] Implement each mutation through the atomic state writer and preserve existing retry/status semantics.
- [ ] Verify restart persistence by closing and reopening the file store in tests.

### Task 4: Update build, deployment, and documentation

**Files:**
- Modify: `modbus-api-middleware/build-all.bat`
- Modify: `modbus-api-middleware/cmd/middleware/main.go`
- Modify: `modbus-api-middleware/deploy/manage-service.sh`
- Modify: `modbus-api-middleware/deploy/install-systemd.sh`
- Modify: `modbus-api-middleware/README.md`

- [ ] Add a MIPS middleware binary target while leaving Windows SQLite build output unchanged.
- [ ] Make Linux deploy select the MIPS file-store path and avoid the SQLite-only update bridge.
- [ ] Document RUT906 transfer, permissions, service startup, storage location, backup, and settings transfer.
- [ ] Add a clear startup error if a Windows SQLite database is passed to the MIPS file backend.

### Task 5: Full verification

- [ ] Run `go test ./...` for the host/SQLite build.
- [ ] Run `GOOS=linux GOARCH=mips CGO_ENABLED=0 go test ./...` or the narrow MIPS package tests and `go build`.
- [ ] Run `bash -n deploy/manage-service.sh deploy/install-systemd.sh`.
- [ ] Verify no MIPS package imports `modernc.org/sqlite`.
- [ ] Perform the RUT906 smoke test: start, open UI, save config, poll, queue delivery, restart, and confirm data remains.

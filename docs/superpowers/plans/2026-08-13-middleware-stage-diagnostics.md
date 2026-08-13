# Middleware Stage Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Safely stage Middleware patches with realtime byte progress and actionable diagnostics.

**Architecture:** The Middleware uses an in-memory maintenance gate around stage work. `command.progress` travels from Middleware through the Platform hub into the in-memory update job queried by the web UI. Final failures are structured and rendered as a copyable support payload.

**Tech Stack:** Go, coder/websocket, React/TypeScript, Node test runner.

**Spec:** `docs/superpowers/specs/2026-08-13-middleware-stage-diagnostics-design.md`

## Global Constraints

- Never stop the OS service before stage finishes.
- Never discard SQLite telemetry or interrupt active Modbus I/O.
- Resume polling in every stage exit path.
- Use actual downloaded bytes and known patch size for progress.

---

### Task 1: Middleware maintenance gate and progress payload

**Files:**
- Modify: `modbus-api-middleware/internal/app/service.go`, `internal/app/poller.go`
- Modify: `modbus-api-middleware/internal/realtimeclient/client.go`
- Test: `modbus-api-middleware/internal/app/poller_test.go`, `internal/realtimeclient/client_test.go`

**Interfaces:**
- Produces `Service.BeginMaintenance(context.Context) (func(), error)` and `StageProgress` command messages.

- [ ] Write failing tests proving a poll sweep waits/skips during maintenance and that a staged HTTP download emits byte progress before its result.
- [ ] Run `go test ./internal/app ./internal/realtimeclient` and confirm those tests fail for the missing gate/progress behavior.
- [ ] Add the gate, defer release in `stageUpdate`, stream the response body with a counting reader, and send `command.progress` through a serialized websocket writer.
- [ ] Run `go test ./internal/app ./internal/realtimeclient` and confirm it passes.

### Task 2: Platform progress and structured stage errors

**Files:**
- Modify: `services/platform-api/internal/httpapi/gateway_realtime.go`
- Modify: `services/platform-api/internal/gatewayhub/hub.go`
- Modify: `services/platform-api/internal/core/middleware_update_jobs.go`, `middleware_patch.go`, `middleware_patches.go`
- Test: `services/platform-api/internal/gatewayhub/hub_test.go`, `internal/core/middleware_update_jobs_test.go`

**Interfaces:**
- Consumes `command.progress`.
- Produces `MiddlewareUpdateJobItem` phase/byte/ETA/error fields and a structured JSON error response.

- [ ] Write failing tests proving progress updates only the matching command/job item and structured errors retain code, phase, and retryability.
- [ ] Run `go test ./internal/gatewayhub ./internal/core` and confirm expected failures.
- [ ] Add progress subscription/routing, job item updates, typed stage errors, and JSON error serialization.
- [ ] Run `go test ./internal/gatewayhub ./internal/core` and confirm it passes.

### Task 3: Progress and diagnostic UI

**Files:**
- Modify: `apps/web/app/features/middlewares/middlewares-page.tsx`, `app/lib/types.ts`, `app/lib/middleware-progress.ts`
- Test: `apps/web/app/lib/middleware-progress.test.ts`

**Interfaces:**
- Consumes job item phase/bytes/ETA/error diagnostic fields.
- Produces a byte progress label and copyable support payload.

- [ ] Write failing tests for percentage, remaining MB, ETA suppression without speed, and diagnostic formatting.
- [ ] Run `npm test` from `apps/web` and confirm failures.
- [ ] Render per-item byte progress and a copy support-details action for failed stages.
- [ ] Run `npm test`, `npm run typecheck`, and `npm run build` from `apps/web` and confirm all pass.

# Backend Microservices Split — Target Architecture & Phase 1

Status: Approved (design), pending implementation plan for Phase 1 only
Date: 2026-08-01

## Context

The backend today is 3 deployables: `services/api-gateway` (a thin reverse proxy — CORS, security headers, health check, single upstream), `services/platform-api` (a monolith containing everything else: auth, users, roles, api-keys, profile, plants, devices, telemetry ingestion + query, middleware gateway realtime hub + config computation, SCADA, alarms, dashboard, audit, hard-delete cascades, notifications), and `modbus-api-middleware` (on-site, unrelated to this split). The user wants `platform-api` split into multiple independently-deployable services along business-domain boundaries, plus a local dev script to run everything at once for testing.

This document has two parts: **the target architecture** (all services, DB strategy, gateway routing — design now, build incrementally) and **Phase 1** (the only part with an implementation plan right now — extracting `auth-service`). Every later phase gets its own spec before implementation, per this project's decomposition practice — this document does not plan Phase 2 onward in task-level detail.

## Target service map

`platform-api`'s `internal/core/*.go` files map to target services as follows:

| Target service | Source (`internal/core/`) |
|---|---|
| `auth-service` | `internal/auth` package, `users.go`, `roles.go`, `api_keys.go`, `profile.go` |
| `plant-service` (master data) | `plants.go`, `devices.go` |
| `telemetry-service` | `internal/ingestion` package, `telemetry.go` |
| `middleware-gateway-service` | `internal/gatewayhub` package, `middleware_config.go`, `middleware_plants.go` |
| `scada-service` | `scada.go` |
| `alarm-service` | `alarms.go` |
| `dashboard-service` | `dashboard.go`, `dashboard_layout.go` |
| `notification-service` | `internal/notification` package (already isolated) |

**Not split into their own service:**
- `audit.go` — stays a shared Go library imported by every service; each service writes audit events into the same shared `audit_event` table directly. Making audit its own service would add a network hop to every write path for a purely cross-cutting concern, with no isolation benefit.
- `hard_delete.go` — cascades across Plant/Device/User/RegisterMetadata/telemetry in one transaction today. This is the single biggest risk in the whole split (see Database strategy) and is the reason the split uses a shared database rather than one database per service. Each domain's hard-delete endpoint moves to the service that owns that domain's primary table (e.g. Plant/Device hard-delete moves to `plant-service`), and continues to delete across other domains' tables directly via SQL in the same transaction — this still works because the database instance is shared, even though process ownership is split.

## Database strategy

**One shared PostgreSQL instance, one shared migration set — not database-per-service.**

- Every service connects to the same Postgres instance (per-service connection pool, same target database).
- Each service is the sole *writer* of its own domain's tables (e.g. `plant-service` writes `plant`/`device`/`device_model`/`device_model_register_metadata`), but any service may *read* another domain's tables directly via SQL when needed, rather than calling another service's HTTP API.
- `internal/database/migrations/` stays one shared migration directory — no per-service schema split.

**Why not database-per-service:** `hard_delete.go`'s cross-domain cascades would require a distributed transaction or saga pattern under true database-per-service — significant complexity for a system at this scale, with real risk of partial-delete inconsistency. A shared database gets the deployment/process/fault-isolation benefits the user wants (Section: why) without that risk. This is explicitly a "modular monolith split into separately-deployable processes" rather than textbook microservices with owned databases — schema changes still require coordinating across services, which is an accepted trade-off at this scale.

## API Gateway routing

`services/api-gateway/internal/gateway/gateway.go` changes from a single-upstream reverse proxy to a path-prefix router:

```go
routes := map[string]*url.URL{
  "/api/v1/auth/":                 authServiceURL,
  "/api/v1/admin/users":           authServiceURL,
  "/api/v1/admin/roles":           authServiceURL,
  "/api/v1/admin/permissions":     authServiceURL,
  "/api/v1/admin/api-keys":        authServiceURL,
  "/api/v1/plants":                plantServiceURL,
  "/api/v1/device-models":         plantServiceURL,
  "/api/v1/dashboard":             dashboardServiceURL,
  "/api/v1/scada":                 scadaServiceURL,
  "/api/v1/admin/middlewares":     middlewareGatewayServiceURL,
  "/api/v1/realtime":              middlewareGatewayServiceURL,
  "/api/v1/plants/{id}/telemetry": telemetryServiceURL,
  "/api/v1/plants/{id}/alarms":    alarmServiceURL,
}
```

Longest-matching-prefix wins; existing CORS/security-header/health-check behavior in `gateway.go` is unchanged, only the single-`platformURL` proxy target becomes a routing table.

**Session/auth validation is not a network call.** Every service imports the existing `internal/auth` package as a shared Go module (compiled in, not called over HTTP) and validates the session cookie directly against the shared `sessions`/`users` tables. This avoids making `auth-service` a synchronous bottleneck and single point of failure for every request across every other service.

**The browser-facing realtime WebSocket** (`/api/v1/realtime` — `telemetry.snapshot`/`alarm.event`/`connection.heartbeat` broadcast to the web UI, distinct from the Middleware's own realtime WS) moves to `middleware-gateway-service` (it already owns the hub/broadcast infrastructure via `gatewayhub`) rather than becoming a 9th service. It sources telemetry/alarm data by reading the shared database directly (poll or Postgres `LISTEN`/`NOTIFY`), not by calling `telemetry-service`/`alarm-service` over HTTP.

## Local dev: `run-all.bat`

New file at repo root, `run-all.bat`, matching this repo's existing `.bat` conventions (see `modbus-api-middleware/build-all.bat`):

```bat
@echo off
setlocal EnableExtensions
cd /d "%~dp0"

start "api-gateway"           cmd /k "cd services\api-gateway && go run ./cmd/api-gateway"
start "platform-api"          cmd /k "cd services\platform-api && go run ./cmd/platform-api"
start "modbus-api-middleware" cmd /k "cd modbus-api-middleware && go run ./cmd/middleware"
start "web"                   cmd /k "cd apps\web && npm run dev"

echo All services starting in separate windows.
```

This targets *today's* process list (3 backend processes + the web dev server) — it does not manage Postgres (assumed already running separately). As each phase extracts a new service, that phase's implementation adds one more `start "<service>" cmd /k "..."` line; the script's structure doesn't need to change shape to accommodate growth.

## Phase rollout order

1. **`auth-service`** (this document's Phase 1, planned now) — most foundational (every other service depends on the session-validation pattern working across a process boundary) and the most self-contained domain (users/roles/api-keys/profile don't need to read Plant/Device/Telemetry data), making it the lowest-risk service to prove the pattern with.
2. **`plant-service`** — proves the hard-delete-across-shared-database pattern (Plant → Device → RegisterMetadata cascade); needs to be stable early since Telemetry/SCADA/Alarm all reference Device/Plant IDs.
3. **`telemetry-service`**, **`middleware-gateway-service`** — the two services motivating the split in the first place (highest write volume, clearest fault-isolation value).
4. **`scada-service`**, **`alarm-service`**, **`dashboard-service`**, **`notification-service`** — remaining domains, extracted as needed, lowest urgency.

Each phase after Phase 1 gets its own brainstormed spec before any implementation plan is written — this document only fixes the target shape and the order, not the task-level detail for phases 2+.

## Phase 1 scope: extract `auth-service`

- New deployable: `services/auth-service` (new Go module, `cmd/auth-service/main.go`), following the same structure as `services/platform-api` (`internal/core`, `internal/httpapi`, `internal/database` connection setup, `internal/config`, `internal/envfile`).
- Moves (not copies) from `services/platform-api`: the `internal/auth` package, `internal/core/users.go`, `roles.go`, `api_keys.go`, `profile.go`, and their corresponding `internal/httpapi` route handlers and route registrations for `/api/v1/auth/*`, `/api/v1/admin/users`, `/api/v1/admin/roles`, `/api/v1/admin/permissions`, `/api/v1/admin/api-keys`, `/api/v1/auth/profile`, `/api/v1/auth/sessions`.
- `services/platform-api` keeps everything else, and keeps using `internal/auth` as a shared package for its own session validation (per the "no network call" rule above) — `platform-api` does NOT call `auth-service` over HTTP for session checks; it imports the same package and reads the same shared `sessions` table directly.
- `services/api-gateway` gains the routing-table change (Section: API Gateway routing) scoped to just the `auth-service` routes for this phase — other routes stay pointed at `platform-api` until their own phase extracts them.
- `run-all.bat` gains one more `start` line for `auth-service`.
- No database schema change — `auth-service` and `platform-api` read/write the same existing `users`/`roles`/`permissions`/`sessions`/`api_key_client` tables via the same migrations.

## Explicitly out of scope (this document)

- Task-level implementation detail for Phases 2 onward (plant-service, telemetry-service, middleware-gateway-service, scada-service, alarm-service, dashboard-service, notification-service) — each gets its own spec when its phase begins.
- Any change to `modbus-api-middleware` (unrelated to this split).
- Any change to the actual business logic inside the moved packages — Phase 1 is a process-boundary move, not a rewrite.
- Container orchestration (Docker Compose/Kubernetes) — out of scope unless separately requested; `run-all.bat` covers local testing only.

## Verification (Phase 1)

- `go build ./... && go vet ./... && go test ./...` green in both `services/auth-service` (new) and `services/platform-api` (with the moved packages removed).
- `services/api-gateway`'s routing table correctly forwards `/api/v1/auth/*` and the admin users/roles/api-keys/profile routes to `auth-service`, everything else still to `platform-api` — verified by hitting both through the gateway.
- Manual: log in, manage users/roles/API keys, view/edit profile, all through the web UI with `auth-service` running as its own process — confirm `apps/web` needs zero changes (it only ever talks to the gateway's stable URL, never a service directly).
- `run-all.bat` starts `auth-service` alongside the existing 4 processes in its own window.

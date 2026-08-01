# Import Config From Middleware — Design

Status: Approved (design), pending implementation plan
Date: 2026-08-01

## Context

Before the realtime config-sync feature (ADR 0005, Phase 1) existed, `modbus-api-middleware` was configured standalone via its own local web UI (`modbus-api-middleware/internal/web/configuration.go` and related files) against its own local SQLite store (`modbus-api-middleware/internal/store/configuration.go` — Brands, Device Sets, Addresses, Connections). Some already-deployed Middlewares still carry this locally-authored config, never entered into platform-api's Device Model / Register Metadata master data. The one-off seed script written earlier this session (`docs/middleware_db/middleware.db` → `POST /api/v1/device-models` + register-metadata calls) solved this once, offline, for a single exported SQLite file — this feature makes the same import repeatable, in-product, and driven by a live connected Middleware instead of a manually-exported file.

## Decision

Goal is **one-time onboarding import**, not ongoing bidirectional sync. Config still flows one direction in steady state (platform-api computes → pushes to Middleware, per ADR 0005) — this feature adds a second, explicit, admin-triggered direction (Middleware's local config → platform-api master data) used only when bringing an already-configured Middleware into the system for the first time. Re-running it later is safe (idempotent upserts) but is not something the system does automatically or repeatedly.

## Wire protocol

Reuses the existing WebSocket `command.request`/response channel already used for Test Connection/Test Read (`services/platform-api/internal/core/middleware_config.go`'s `RunMiddlewareCommand`, `modbus-api-middleware`'s existing command handler) — no new connection, no new port.

- New command kind: `"config-export"`.
- Middleware handler: on receiving `command.request{kind:"config-export"}`, reads its full local config from its own store (all Brands, Device Sets with Addresses, Connections — the same data its pre-realtime-sync local web UI edited) and returns it as the command response payload, shaped like the existing `MiddlewareConfigSnapshot` type (`brands`, `deviceSets` with nested `addresses`, `connections`) — reuse that struct, do not invent a new wire type.
- platform-api: new `ImportMiddlewareConfig(ctx, principal, middlewareID)` in `services/platform-api/internal/core/middleware_config.go`, sending the command through `gatewayhub` and waiting up to 15s for the response — same timeout/wait pattern `RunMiddlewareCommand` already uses.
- New route: `POST /api/v1/admin/middlewares/{middlewareId}/import-config`.

## Import logic

- **Brand + Device Set → `DeviceModel`**: upsert by `(manufacturer, deviceType, model)` — if a matching Device Model already exists, reuse it (no duplicate); otherwise create it. Same matching rule the one-off seed script used.
- **Address → `DeviceModelRegisterMetadata`**: upsert by `addressKey` under the resolved Device Model.
- **Connection → nothing automatic.** A Connection implies a Plant assignment, which is an organizational decision a human should make, not something this import should guess. Connections are not turned into `Device` rows. Instead, the import result reports what was found (host, port, unit ID, resolved Device Model name) so an admin can create the matching `Device` manually on the Plants → Devices page using that information.
- **Idempotent**: safe to run the same import more than once against the same Middleware — nothing gets duplicated, existing rows are reused/updated in place.
- **Audited**: the import action writes an audit event (matching the existing audit-event pattern used elsewhere in `core/`) recording who triggered it, against which Middleware, and when.

## UI

On the Middleware Gateway detail view (`apps/web/app/features/middlewares/middlewares-page.tsx`, `MiddlewareConfigEditor`), add an "Import from Middleware" section below the existing "Plants ที่ Middleware นี้ดูแล" section:

- A single button, "ดึง Config จาก Middleware" — disabled whenever the gateway is not `isOnline` (same disabled condition already used for Test Connection/Test Read elsewhere in this codebase).
- Clicking it shows a confirm dialog first (`window.confirm`, matching the existing confirm pattern used for decommission/delete actions elsewhere in this app) warning that this will create/update Device Model and Register Metadata entries, before sending the request.
- While pending: a toast indicates the import is in progress (mirrors the existing Test Connection/Test Read toast pattern from the design-system redesign).
- On success: a toast/banner summarizing the result — counts of Device Models and Register Metadata rows created/updated, plus the count of Connections found (with a note to create their Devices manually on Plants → Devices).
- On error/timeout: an error toast, same pattern as Test Connection/Test Read failures.

## Explicitly out of scope

- No automatic/scheduled re-import, no drift detection, no diff view between platform-api's computed config and the Middleware's local config.
- No automatic `Device`/`Plant` creation from Connections.
- No change to the steady-state push direction (`buildConfigSnapshot`/`recomputeAndPushMiddleware`) — this is purely an additive, admin-triggered, one-way read path.
- No change to `modbus-api-middleware`'s own local web UI or local store — only a new command-response handler reading from what's already there.

## Verification

- Backend: unit/integration test for `ImportMiddlewareConfig` covering upsert-on-existing-model (no duplicate), create-new-model, register metadata upsert, and the Connection-summary-only (no Device created) behavior.
- Manual: with a real or test Middleware carrying local Brands/DeviceSets/Addresses/Connections and connected Online, click "Import from Middleware" from a fresh org with no matching Device Models, confirm Device Models + Register Metadata appear in the Register Metadata page and the result summary lists the Connections found; run the import a second time, confirm no duplicates are created.
- Manual: attempt the import while the gateway shows Offline, confirm the button is disabled.

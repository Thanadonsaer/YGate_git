# Middleware Remote Management (Software Update + Restart) — Design

Status: Approved (design), pending implementation plan
Date: 2026-08-02

## Context

`modbus-api-middleware` already has a full self-update mechanism (`internal/web/update.go`): upload a patch zip (`update-manifest.json` + a signed binary), stage it, apply it (backs up the current binary, stops the OS service, copies the new binary in, starts the service again — via `startUpdater`'s Windows PowerShell / Linux shell script), or roll back to the last backup. It also already knows whether it's safe to do any of this (`CanApplyUpdate`, true only when running as a Windows Service or under systemd). All of this is reachable **only from the Middleware's own local web UI**, at the Middleware's own IP — there is no way to trigger it from ygate's central web.

That's a real operational gap: `modbus-api-middleware` is an outbound-only WebSocket client (it dials out to platform-api, never accepts inbound connections — ADR 0005), so it's frequently deployed behind NAT/firewalls where an admin can't just open its local IP. The only channel platform-api has to reach a connected Middleware at all is the existing `command.request`/`command.result` WS channel (already used for `readNow`, `connectTest`, `config-export`). This design reuses that channel to expose Update and Restart to ygate's central web, instead of building a second communication path.

Separately: `ImportMiddlewareConfig`'s `importFromSnapshot` never mapped the wire snapshot's `SourceUnit` field into `RegisterMetadata.Unit` — that's a one-line bug fix, already shipped (commit `c28eecd`), not part of this design.

## Decision

Reuse the existing WS command channel for all remote-management operations. Add a small, filesystem-backed patch repository to platform-api (global, not organization-scoped — a binary isn't tied to a tenant) that Platform Admins upload to and push from. Restrict every operation in this design to Platform Admin — these affect the Middleware's running software and process lifecycle for the whole deployment, a materially bigger blast radius than the existing per-organization `middleware_config` permission.

## Wire protocol: 4 new command kinds

Same `command.request`/`command.result` envelope and `RunCommand`/`hub.RunCommand` plumbing `ImportMiddlewareConfig`/`RunMiddlewareCommand` already use — no new connection, no new port, no protocol version bump.

- **`update.stage`** — payload `{downloadUrl, sha256, patchVersion, os, arch, binary}` (`patchVersion`, not `version` -- the shared envelope already uses the `version` JSON key for the unrelated int64 config version). The middleware does an HTTPS GET against `downloadUrl`, authenticating with the same `X-Api-Key` header it already sends on the WS handshake (`realtimeclient.Client.APIKey`), then runs the exact validation + staging logic `stageUpdateZip` already has (manifest checks, sha256 verification, write to `updates/staged/`) — refactor `stageUpdateZip` to accept an `io.Reader` instead of assuming a `multipart.File`, so both the local HTTP upload handler and this new WS command path call the same core function.
- **`update.apply`** — no payload needed (operates on whatever is currently staged). Calls the same `backupCurrentBinary` + `startUpdater(staged binary)` the local "Apply" button already calls.
- **`update.rollback`** — no payload. Calls the same `latestBackup` + `startUpdater(backup path)` the local "Rollback" button already calls.
- **`service.restart`** (new capability, doesn't exist locally today either) — stop+start the service via the same OS-specific script `startUpdater` uses, but skip the binary-copy step entirely (`Source == Destination`, i.e. copy the running exe over itself — reuses the script unchanged rather than forking a second script).

All four are gated by the existing `CanApplyUpdate` flag: if false, the command handler replies `{ok:false, error:"middleware not running as a managed service — use the local update UI on the middleware host instead"}` immediately, rather than attempting and hanging. `update.apply`/`update.rollback`/`service.restart` all reply `{ok:true}` immediately after starting the OS script (which itself sleeps 2s before stopping the service) — the actual restart happens a few seconds after the command result is sent, and shows up on ygate web as the gateway going Offline then Online again (existing hub connect/disconnect tracking, no change needed there).

## Software version visibility

Today's `hello` envelope only carries `appliedVersion` (the *config* version, `domain.ConfigSnapshot.Version` — unrelated to the binary). Add a `version` field to `hello` carrying the middleware binary's own version string (`s.Version`, the same value already shown in the local web UI and burned into the binary via `-ldflags -X main.version=...`). `HandleGatewayHello` stores it on `middleware_client.software_version` (new column, migration below). This is what ygate web shows as "Current Version" and is the only way an admin can confirm an Apply/Rollback actually took effect (the gateway reconnecting with a new `version` in its next `hello`).

## services/platform-api

### Migration

- `middleware_client` gains `software_version text` (nullable — unset until a `hello` with the new field arrives; older middleware binaries that don't send `version` yet just leave it null, no migration required on their side).
- New table `middleware_patch`: `id uuid pk, version text, os text, arch text, binary_filename text, sha256 text, file_size_bytes bigint, storage_path text, uploaded_by uuid references app_user(id), created_at timestamptz` — global, no `organization_id`. Unique on `(version, os, arch)` (re-uploading the same target twice is almost certainly a mistake).
- New `resource_type='middleware_patch'` permission rows (`create`, `read`, `delete`), granted **only** to the Platform Admin role (`organization_id IS NULL` scope) — no Organization Admin grant, matching the confirmed decision.
- The 5 new middleware-lifecycle routes below are permission-checked against `middleware_patch` (not `middleware_client`) using `hasGlobalPermissionQuery` (`internal/core/permission.go`, already used for organization-agnostic checks) instead of `requireOrganizationPermission` — since the grant above only exists at `organization_id IS NULL` scope, this naturally restricts every one of these routes to Platform Admin regardless of which organization the target Middleware belongs to.

### Patch storage

Local filesystem on the platform-api host, path from a new config var `PLATFORM_MIDDLEWARE_PATCH_DIR` (default `./data/middleware-patches`, mirroring how other file-ish local state is configured in this codebase) — matches the current single-node PM2/systemd deploy model; no object storage service exists in this stack today and none is warranted for patch files in the tens-of-MB range.

### New endpoints (all Platform Admin only)

- `POST /api/v1/admin/middleware-patches` — multipart upload. Validates the zip with the same manifest-check logic `modbus-api-middleware/internal/web/update.go`'s `validateManifest` already has (App/Version/OS/Arch/Binary/SHA256 shape) — this is server-side duplication of that check by necessity (platform-api and modbus-api-middleware are separate Go modules per ADR from the microservices-split work; no shared-code path exists between them), not a design choice to revisit lightly. Stores the file under the patch dir, inserts the `middleware_patch` row, returns it.
- `GET /api/v1/admin/middleware-patches` — list, newest first. Powers the version picker on the Middleware detail page.
- `DELETE /api/v1/admin/middleware-patches/{id}` — removes the file + row (simple cleanup; no in-use tracking needed since `update.stage` copies the file to the middleware immediately, it doesn't keep referencing the platform-api-side file after that).
- `GET /api/v1/admin/middleware-patches/{id}/download` — the only one of these five NOT behind cookie/CSRF auth: authenticated the same way the WS handshake is, by `X-Api-Key` header matched against `middleware_client.key_hash` (reuse the existing key-hash lookup gatewayhub's WS upgrade handler already has). This is what `update.stage`'s `downloadUrl` points at.
- `POST /api/v1/admin/middlewares/{middlewareId}/update/stage` — body `{patchId}`. Looks up the patch row, builds a `downloadUrl` pointing at the download endpoint above (same host the WS connection already terminates at), sends `update.stage` via `s.hub.RunCommand`, same 15s-wait pattern `ImportMiddlewareConfig` uses.
- `POST /api/v1/admin/middlewares/{middlewareId}/update/apply` — sends `update.apply`.
- `POST /api/v1/admin/middlewares/{middlewareId}/update/rollback` — sends `update.rollback`.
- `POST /api/v1/admin/middlewares/{middlewareId}/restart` — sends `service.restart`.

All four `RunCommand`-based endpoints follow the exact error-mapping `ImportMiddlewareConfig`/`writeMiddlewareError` already established: offline → 503, command timeout/NAK → 504. Each writes an audit event (`middleware_patch.uploaded`, `middleware_client.update_staged`, `.update_applied`, `.update_rolled_back`, `.restarted`) via the same `CreateAuditEventFull` pattern used everywhere else in `core/`.

## modbus-api-middleware

- `internal/realtimeclient/client.go`'s `handleCommand` switch gains 4 new cases (`update.stage`, `update.apply`, `update.rollback`, `service.restart`), each a thin call into refactored functions extracted from `internal/web/update.go` (`stageFromReader`, `applyStaged`, `rollbackToBackup`, `restartServiceOnly` — names illustrative, exact naming left to implementation) so the HTTP handlers (`updateUpload`/`updateApply`/`updateRollback`) and the new WS command handlers share one implementation each, not two.
- `internal/realtimeclient/client.go`'s hello-sending code adds `Version: c.Version` (new field on `envelope`, `omitempty` like the others) — `Client` needs a `Version string` field threaded in from `cmd/middleware/main.go` (it already has `version` in scope there, currently only passed to `webui.Server`).

## apps/web

### Middleware detail page (`middlewares-page.tsx`, `MiddlewareConfigEditor`)
New "Software Update" section, rendered only when the logged-in user is a Platform Admin (reuse whatever existing principal/role check this app already has for other Platform-Admin-only UI, e.g. the Middleware create button):

- "Current Version: {gateway.softwareVersion ?? '-'}"
- A patch picker (`Select`, populated from `GET /api/v1/admin/middleware-patches`, filtered client-side to patches whose `os`/`arch` look plausible — informational filter only, not a hard block, since the admin may know better than the picker)
- Buttons: "Stage" (enabled once a patch is picked), "Apply", "Rollback", "Restart" — Apply/Rollback are always-enabled (no "is something staged" precondition tracked client-side); the confirm dialog text itself says "operates on whatever is currently staged" / "rolls back to the last backup", and a bad request (nothing staged / no backup) surfaces as a normal error toast from the command's `{ok:false}` response. Each of the four is disabled while any of the other three is pending (only one lifecycle op in flight at a time), each with a `window.confirm` warning that the service will bounce, each following the exact pending-state/toast/error pattern just built for Push/Pull Config.

### New "Middleware Patches" page

New top-level nav entry (Platform-Admin-only, hidden entirely otherwise — same visibility rule as the Software Update section): upload form (file input + submit, matching this app's existing file-upload patterns if any exist, otherwise a plain `<input type="file">` + `FormData` POST) and a list table (version/os/arch/size/uploaded by/date/delete button).

## Explicitly out of scope

- No automatic/scheduled updates, no update-check polling, no "notify when a new version is available" — every step (upload, stage, apply, rollback, restart) is a manual admin action, matching the manual Push/Pull Config precedent from the previous fix.
- No multi-middleware batch operations (stage-to-all, fleet-wide rollout) — one Middleware at a time, selected from its own detail page.
- No progress streaming during Apply/Rollback beyond the immediate `{ok:true}` ack + the gateway's Offline→Online transition already visible on the Middlewares list. If Apply fails after the ack (e.g. the new binary crashes on boot), the existing `windowsUpdaterScript`'s automatic `Start-Service` retry on failure is the only safety net — no additional health-check/auto-rollback-on-boot-failure is added by this design.
- No changes to the local Middleware web UI's own Update page — it keeps working exactly as today, independently of the new WS path.

## Verification

- Backend: unit tests for the 5 new `core/` functions covering permission checks (Platform Admin required, Organization Admin rejected), offline-gateway 503, and the patch-upload manifest validation (reject wrong `app`/`os`/`arch`, reject sha256 mismatch) mirroring `stageUpdateZip`'s existing test coverage style.
- Backend: `modbus-api-middleware` unit tests for the 4 new `handleCommand` cases, especially the `CanApplyUpdate=false` short-circuit.
- Manual (requires a real or test Middleware runnable as a Windows Service/systemd unit, connected Online): upload a patch built with `build-all.bat`'s existing `make-update-zip.ps1` output, Stage it from ygate web, confirm it appears staged; Apply, confirm the gateway goes Offline then Online with the new `Current Version` shown; Rollback, confirm it goes back; Restart, confirm a clean bounce with the same version.
- Manual: attempt any of the 4 lifecycle actions while `CanApplyUpdate` is false (e.g. running the middleware as a plain foreground process), confirm the clear "not a managed service" error surfaces on ygate web instead of a timeout.

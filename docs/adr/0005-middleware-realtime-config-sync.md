# ADR-0005: Middleware Realtime Config Sync (No Inbound Port)

- Status: Accepted
- Date: 2026-07-31

## Context

Each `modbus-api-middleware` instance is only configurable through its own local embedded web UI, reachable on that site's LAN. Managing Brands / Device Sets / Register Addresses / Modbus Connections per site does not scale as an operations model. The user wants central management from `apps/web` ("ygate"), with two hard constraints:

- No inbound port and no Cloudflare Tunnel at any site — every connection must be initiated outbound, from the middleware toward the platform.
- Realtime, not interval polling.

## Decision

- **Transport**: the middleware dials an outbound WebSocket to `GET /api/v1/gateway/realtime` on `services/platform-api`, authenticated via the same `X-Api-Key` header (and `middleware_client` lookup) the v1/v2 ingestion endpoints already use. This is the only viable "realtime + no inbound port" combination.
- **One channel, two directions of intent**: config pushes (platform→middleware, ack'd) and on-demand command relay (platform→middleware "run this test", middleware→platform "here's the result", correlated by a `commandId`) both ride the same socket.
- **Full-snapshot sync, not diffs.** Every config push replaces the middleware's entire local Brands/DeviceSets/Addresses/Connections/Plants set inside one SQLite transaction. Per-site config is small; a diff/patch protocol would be materially more code for no measurable benefit at this size.
- **IDs are cross-reference keys only, never literal primary keys.** `middleware_client.config_snapshot` (the platform's stored, admin-edited copy) and the wire payload use the same JSON shape as the middleware's local domain types. On every apply, the middleware deletes and re-inserts every row, remapping IDs through a within-snapshot map — this sidesteps reconciling two independently-assigned ID spaces (ygate-web-assigned temp IDs for new entities vs. the middleware's own SQLite autoincrement).
- **Validation reuses the middleware's own local-edit validators** (`normalizeDeviceSet`/`normalizeAddress`/`validateAddress`/`normalizeConnection` in `internal/store`), applied inside one transaction. A failure rolls the whole transaction back — SQLite is left exactly as it was — and is recorded as a `FAILED` row in a new local `config_history` table instead. Only a `nil` error from `ApplyConfigSnapshot` may trigger a hot-swap of the in-memory config cache (`internal/configcache`).
- **Storage on platform-api is a single jsonb blob** (`middleware_client.config_snapshot` + a `middleware_config_history` audit/version table), not normalized relational tables per brand/device-set/address/connection. The middleware is the authority on what's a valid Modbus config (it already has that logic); platform-api's job is to store, version, and relay, not re-implement Modbus config validation. The one deliberate exception: a connection's `plantCode` is validated against platform-api's real `plant` registry at save time, since that registry already exists and is more authoritative than the middleware's local flat `plants` table.
- **Test Connection / Test Read results return as a plain synchronous HTTP response** to the admin's POST call — the server already blocks internally on `gatewayhub.RunCommand` waiting for the middleware's reply, so there is no need to also thread the result through the browser's separate `/api/v1/realtime` WebSocket.
- **The middleware's in-memory config cache uses `atomic.Pointer[Config]`** (`internal/configcache`), swapped as a whole snapshot. Every request handler and the poller read from this cache, never SQLite directly. SQLite remains the durable source of truth: on startup the middleware loads SQLite into the cache synchronously before serving, then the realtime client sends `hello{gatewayId, appliedVersion}` and the server decides whether to push.

## Wire contract

Endpoint: `GET /api/v1/gateway/realtime` (platform-api). JSON messages over the WebSocket, one `type` field each:

- `hello` (middleware→platform, first message): `{type, gatewayId, appliedVersion}`
- `config.snapshot` (platform→middleware): `{type, version, brands[], deviceSets[], connections[], plants[]}`
- `config.ack` (middleware→platform): `{type, status: "APPLIED"|"FAILED", reason?}`
- `command.request` (platform→middleware): `{type, commandId, kind: "connectTest"|"readNow", connectionId}`
- `command.result` (middleware→platform): `{type, commandId, ok, data?, error?}`
- `heartbeat`: server→middleware every 20s; the middleware does not send its own heartbeats, it just uses receipt of any message (including heartbeats) to reset a 45s per-read liveness deadline and reconnects with exponential backoff (`1<<min(attempts,8)` seconds, matching the existing delivery-outbox retry curve) if that deadline lapses.

## Consequences

- The middleware's local embedded web UI (`static/index.html`, `/api/connect-test/`, `/api/read-now/`, `/api/send-api-once/`) is untouched — this feature is purely additive, for on-site debugging to keep working exactly as before.
- `services/api-gateway` needed no changes: it is a transparent wildcard `httputil.ReverseProxy`, which passes both the new admin REST routes and the WebSocket upgrade through untouched.
- `GatewayConfig` (endpoint/API key/interval — how the middleware finds and authenticates to the platform in the first place) is explicitly excluded from the synced config; pushing it centrally would be circular.
- If a future need arises to manage Brands/DeviceSets/Addresses/Connections as fully independent, deletable-with-history resources on the platform side (rather than "the middleware's current full snapshot"), the jsonb-blob storage decision here would need to be revisited in favor of normalized tables.

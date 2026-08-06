# Backend-Triggered Telemetry Pull — Design

## Goal

Replace the middleware's self-initiated REST push of telemetry
(`modbus-api-middleware/internal/delivery/worker.go` → `POST
/api/v2/ingestion/register-readings`) with a design where platform-api
triggers each connected gateway to hand over its buffered readings over the
existing outbound WebSocket channel (`internal/realtimeclient`,
`gatewayhub`). Motivation: current push model is gated by a per-gateway
`api_polling_enabled` flag that silently no-ops when off (see
`worker.go:29`), which is what caused telemetry to stop flowing without any
error surfaced anywhere. A backend-driven pull removes that failure mode and
lets the backend request fresher data on demand (e.g. on SCADA page open)
instead of only ever seeing whatever cadence the middleware happened to be
configured with locally.

## Non-goals

- Changing how the middleware polls its own Modbus devices
  (`app.PollEnabledConnections`) — that scan loop, its cadence
  (`middleware_client.poll_interval_seconds`), and its local SQLite outbox
  (`outbox_events`) are unchanged.
- Multi-instance platform-api / distributed gateway ownership — confirmed
  single-instance deployment (`run-all.bat`), so `gatewayhub.Hub`'s
  in-process connection map is sufficient; no cross-instance coordination.
- Removing `POST /api/v2/ingestion/register-readings` — left in place as a
  dead/legacy route rather than deleted, in case of manual/offline gateway
  use; not exercised by the new path.

## Architecture

Two round trips over the gateway's existing realtime WebSocket connection,
mirroring the `config.snapshot` / `config.ack` handshake already used for
config sync:

1. **platform-api → middleware**: `telemetry.drain` command, sent by a new
   per-gateway ticker in platform-api (interval = that gateway's
   `poll_interval_seconds`, same column already synced today). Only ticks
   for gateways currently registered in `gatewayhub.Hub` (i.e. online).
2. **middleware → platform-api**: `command.result` containing the batch
   read via `Store.Ready(batchSize)` — read-only, does **not** mark rows
   delivered.
3. **platform-api**: ingests the batch through the existing
   `ingestion.Service` validation/write path (same logic
   `rawIngestionHandler` uses today, called directly instead of over HTTP).
4. **platform-api → middleware**: on successful DB write, a second command
   `telemetry.ack` with the delivered ids.
5. **middleware**: on receiving `telemetry.ack`, calls `Store.Delivered(ids)`.

The two-phase handoff exists because a successful WebSocket write only means
the bytes were flushed to the socket, not that platform-api finished writing
to its database — unlike the REST push, where an HTTP 2xx response already
implies the DB write committed. Marking rows delivered before the DB write
is confirmed would risk silently losing data if platform-api crashed
in between.

## Components

- **modbus-api-middleware** (`internal/realtimeclient/client.go`):
  `handleCommand` gains two new `msg.Kind` cases:
  - `"telemetry.drain"`: calls `Store.Ready(batchSize)`, returns
    `{events, ids}` as `command.result` data. Does not mutate outbox state.
  - `"telemetry.ack"`: takes `{ids}`, calls `Store.Delivered(ids)`, replies
    `{ok: true}`.
- **modbus-api-middleware** (`internal/delivery/worker.go`): unchanged.
  `runDelivery`'s call to `worker.SendOnce()` (which both scans via
  `BeforeSend` and POSTs) is removed from `cmd/middleware/main.go`'s startup
  wiring; the Modbus scan itself (`PollEnabledConnections`) moves to its own
  ticker so it keeps running independent of delivery.
- **platform-api** (new file, e.g. `internal/gatewayhub` or a new
  `internal/telemetrypull` package): scheduler that, for each gateway
  registered in `Hub`, ticks at that gateway's `poll_interval_seconds` and
  drives the two-phase drain/ack sequence via `Hub.RunCommand`.
- **platform-api** (`internal/ingestion`): expose the validation/write logic
  `rawIngestionHandler` calls as a plain function callable from the new
  scheduler, not only from the HTTP handler.

## Data flow

```
middleware ticker (poll_interval_seconds)
  -> PollEnabledConnections -> outbox_events (PENDING)

platform-api ticker (poll_interval_seconds, per online gateway)
  -> hub.RunCommand("telemetry.drain")
  -> middleware: Store.Ready(batchSize) -> command.result{events,ids}
  -> platform-api: ingestion.Service write (DB)
       success -> hub.RunCommand("telemetry.ack", ids)
                  -> middleware: Store.Delivered(ids)
       failure -> no ack sent; rows stay PENDING, retried next tick
```

## Error handling

- **Gateway offline** (`Hub.RunCommand` → `gatewayhub.ErrOffline`): skip
  this tick for that gateway, no error surfaced; data waits in the
  middleware's local outbox until the gateway reconnects and the next tick
  fires.
- **`telemetry.drain` timeout / ctx cancelled**: same as offline — nothing
  was mutated middleware-side (drain is read-only), safe to retry next tick.
- **platform-api DB write fails after drain**: no `telemetry.ack` is sent;
  rows remain `PENDING` in the middleware outbox and are included in the
  next drain automatically. No separate retry/backoff path needed on the
  backend side beyond "try again next tick."
- **`telemetry.ack` lost in transit** (drain succeeded, DB write succeeded,
  ack never arrives/processed): middleware will re-offer the same rows on
  the next drain since they're still `PENDING`. platform-api ingestion must
  stay idempotent on redelivery — it already is, via the existing
  `idempotency_key` used by the current REST path.

## Testing

- **middleware**: unit tests for `handleCommand` kind `telemetry.drain` and
  `telemetry.ack` against a real (in-memory/temp-file) `store.Store`,
  following the existing pattern in `worker_config_test.go` /
  `gateway_config_test.go`.
- **platform-api**: unit tests for the new scheduler against a fake/mock
  `gatewayhub.Hub` covering offline, timeout, and success paths.
- **platform-api**: integration test for the direct-call ingestion path,
  reusing the fixture setup in `raw_service_integration_test.go`.

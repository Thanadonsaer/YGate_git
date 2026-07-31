# ADR-0004: Raw Register Ingestion v2

- Status: Accepted
- Date: 2026-07-29
- Updated: 2026-07-30 — slimmed `registerAddressMap` (see Amendment below)
- Updated: 2026-07-31 — middleware now scales values before sending (see Amendment below)

## Context

The Middleware currently maps vendor Address Configuration to canonical keys and
sends normalized `dataItemMap` batches. Register metadata is now owned by the
Central Platform so the Middleware must send decoded raw register values without
canonical keys, scaling, unit conversion, or duplicated normalized values.

Replacing the existing endpoint would break deployed Middleware clients and the
accepted v1 compatibility contract.

## Decision

- Keep the v1 `dataItemMap` endpoints unchanged for compatibility.
- Add `POST /api/v2/ingestion/register-readings`.
- Require `schemaVersion: "2.0"` and a `registerAddressMap` keyed by
  `<registerAddress>`, value is the decoded raw numeric value
  (see Amendment: originally keyed `<functionCode>:<registerAddress>` with a
  per-entry object; slimmed on 2026-07-30 before any real deployment).
- The Middleware continues to own Modbus polling, address-mode resolution, byte
  order, word order, and source data-type decoding.
- The Central Platform owns scaling, offset, units, display metadata, and future
  canonical point mapping.
- Persist v2 records separately from normalized v1 telemetry. Do not populate
  `telemetry_latest` until a platform metadata mapping has produced canonical
  values.
- When an authenticated v1 or v2 payload contains an address key that is not in
  its Device Model metadata, create a safe default metadata row in the same
  ingestion transaction. Preserve existing operator-edited metadata and audit
  only newly discovered keys. Both v1 and v2 keys default to numeric; an
  authorized user refines the type (e.g. to boolean) in Register Metadata.

## Consequences

- Existing Middleware clients can migrate independently without a flag day.
- v2 ingestion can auto-onboard Plants and Devices using the existing policy.
- The latest-telemetry read model applies Device metadata over Device Model
  metadata, scales enabled v2 values, and exposes stable address keys without
  copying raw records into `telemetry_latest`.
- Telemetry history, Dashboard SQL aggregates, and SCADA persistence continue
  reading normalized v1 telemetry until a full canonical-point pipeline exists.
- Missing register reads are represented by absence from the map.
- Automatically discovered rows use neutral defaults (empty unit, scale 1,
  offset 0) until an authorized user enriches them in Register Metadata.
- No new service or infrastructure dependency is introduced.

## Amendment (2026-07-30): slim `registerAddressMap`

The original shape keyed each entry `<functionCode>:<registerAddress>` with a
per-entry object (`functionCode`, `registerAddress`, `dataType`, `rawValue`,
`quality`). Before any real deployment, this was slimmed to
`<registerAddress>: <rawValue>` to cut storage and bandwidth:

- `functionCode` and `registerAddress` were fully redundant with the map key.
- `dataType` was only ever consulted on the *first* reading of a
  never-before-seen address key (to seed a new Register Metadata row); every
  subsequent reading recomputed and discarded it. New keys now default to
  `"number"`, matching the v1 `dataItemMap` auto-onboard convention exactly —
  an operator corrects the type (e.g. to boolean) in Register Metadata.
- `quality` was never observed as anything but `"GOOD"` in practice — the
  Middleware only ever adds a map entry on a successful decode (see
  `decodeEntry` in `modbus-api-middleware/internal/app/service.go`), so
  quality carried no information. If per-register quality signalling is ever
  needed, reintroduce it as a documented, explicit change, not an unused
  passthrough field.
- The function-code prefix in the key was also the collision guard for a
  device set that mixes FC03 and FC04 on the same numeric address. No
  configured device set does that today (all use `address_mode: VENDOR_RAW`
  with FC03 uniformly). If that ever changes, the key format needs revisiting.

## Amendment (2026-07-31): middleware scales before sending

By product decision, this deployment no longer follows "Central Platform owns
scaling" for v2. `registerAddressMap` values are now `raw * Factor + Offset`,
computed per-address in the Middleware (`decodeEntry` in
`modbus-api-middleware/internal/app/service.go`) using each Address's
`factor`/`offset` from its own Device Set configuration, not Central Platform
Register Metadata.

- **The Central Platform must not re-apply scale to v2 data from this
  Middleware version** — doing so double-scales every value.
- A configured Factor of exactly `0` is treated as `1` (unset), so a blank/
  default Factor cannot silently zero out a reading.
- `measurements[].rawValue` (used only for local debugging in Monitor Live,
  never sent to the Platform) is intentionally left unscaled.
- If the Central Platform's own scaling ever ships, this Amendment must be
  reconciled first — a v2 ingestion source cannot be "raw" per the original
  Decision and "pre-scaled" per this Amendment at the same time.

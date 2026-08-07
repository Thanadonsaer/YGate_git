# Raw-only Telemetry Design

## Goal

Make `telemetry.raw_register_reading` the only telemetry source of truth. Raw register values remain unchanged and display/calculation values are derived from Register Metadata at read time.

## Target architecture

- `telemetry.raw_register_reading`: the only persisted telemetry history.
- `telemetry_latest`: a compatibility read model backed by the newest Raw row per device, not an independent source of data.
- Plant, Dashboard, SCADA, Energy Analysis, and History read Raw values through shared SQL/read-model logic and apply Register Metadata (`scale`, `value_offset`, `is_enabled`) at query time.
- Alarm evaluation reads the same mapped Raw read model so alarms and screens use identical values.

## Migration phases

1. Add shared Raw latest/history SQL and make all read APIs use it.
2. Keep the current physical `telemetry_latest` table temporarily for rollback and compatibility, but stop treating it as authoritative.
3. Update alarm evaluation and hard-delete/restore paths to use Raw.
4. Add a verification/backfill command for existing Raw rows and compare results against current screens.
5. After production verification, migrate `telemetry_latest` from table to view (or remove it where no compatibility is required).
6. Remove `telemetry_reading` only after all code, tests, migrations, and operational queries no longer reference it. Preserve an explicit backup/archive before removal.

## Data and ordering rules

- Latest is selected by `(observed_at DESC, received_at DESC, id DESC)` per device.
- A late-arriving reading is retained in Raw history but cannot replace a newer latest value.
- Metadata is resolved with device-level priority over model-level metadata.
- Disabled metadata points are excluded from calculated output; Raw still retains the register value.
- No Factor/Offset is applied in Middleware.

## Compatibility and rollback

The first implementation is additive and reversible. It does not drop tables or delete historical data. Existing physical projections remain available while the Raw read path is verified.

## Verification

- Unit tests for Raw latest selection, metadata precedence, disabled points, and late arrivals.
- Integration tests that ingest Raw and verify Plant latest, History, Dashboard status, SCADA values, and alarms.
- Full Platform API and Middleware test suites plus Web typecheck/build.
- Production acceptance requires checking that a new Raw row changes Plant/Dashboard without an insert into `telemetry_reading`.
# Telemetry Table Partitioning — Design

Sub-project 2 of 4 (database optimization request). Time-boxed: must land
within this session. Scope kept deliberately minimal as a result — see
Out of scope.

## Context

Dev DB is currently near-empty (`telemetry_reading`: 0 rows,
`raw_register_reading`: 1 row, `telemetry_ingest_batch`: 52 rows) — no
backfill/data-migration risk right now. This is prep infrastructure:
convert the append-only, unboundedly-growing telemetry tables to native
Postgres RANGE partitioning by time, so future growth doesn't degrade
query/vacuum performance and future retention (dropping old partitions)
becomes a cheap `DROP TABLE`, not a slow `DELETE`. No retention policy is
enabled yet — partitioning only, per explicit decision.

## Scope: which tables

- **`telemetry.telemetry_reading`** — partition by `observed_at`, monthly.
- **`telemetry.raw_register_reading`** — partition by `observed_at`, monthly.
- **`telemetry.telemetry_ingest_batch`** — partition by `received_at`, monthly.
- **`telemetry.telemetry_latest` is excluded.** It's a keyed snapshot table
  (`PRIMARY KEY (organization_id, device_id)`, one row per device,
  `UPDATE ... ON CONFLICT` in place) — not append-only, doesn't grow with
  time. Its `observed_at` changes on every update, which would force
  constant cross-partition row movement under time-based RANGE
  partitioning. Partitioning it would add real risk (row-movement UPDATE
  semantics, `ON CONFLICT DO UPDATE` interacting with partition key
  changes) for zero benefit (the table's size is bounded by device count,
  not time). Technical correction to the original request's "all 4
  tables" — flagged, not re-litigated given the time-box.

## Foreign-key fallout (the real complexity here)

Postgres requires a partitioned table's PRIMARY KEY/UNIQUE constraints to
include the partition key column. Two of the three tables being
partitioned are FK targets today:

- `telemetry_latest.telemetry_reading_id REFERENCES telemetry_reading(id)`
- `telemetry_reading.ingest_batch_id` and
  `raw_register_reading.ingest_batch_id` both
  `REFERENCES telemetry_ingest_batch(organization_id, id)`

Once `telemetry_reading`'s PK becomes `(id, observed_at)` and
`telemetry_ingest_batch`'s becomes `(id, received_at)`, the old
`(id)`/`(organization_id, id)`-only FK targets no longer exist.

**Decision (explicit, from the human partner): drop these FKs rather than
widen them with duplicated timestamp columns.** This is the standard,
widely-used pattern for Postgres time-series partitioning — referential
integrity between these tables becomes an application-level convention
instead of a DB-enforced constraint. Accepted trade-off given the time-box;
these are internal telemetry pipeline tables (not user-facing entities
like Plant/Device), and `hard_delete.go`'s existing cross-table cascades
already delete in the correct dependency order by convention, not by
relying on `ON DELETE RESTRICT` to catch mistakes.

`raw_register_reading` has no incoming FKs (confirmed — leaf table), so
it needs no FK changes, only the PK widening.

## Migration approach

One new migration:
`services/platform-api/internal/database/migrations/000034_telemetry_partitioning.sql`.
Postgres has no `ALTER TABLE ... PARTITION BY` — converting an existing
table requires: create a new partitioned table with the same columns/
constraints (PK widened to include the partition key, the FKs above
dropped), create its partitions, copy any existing rows across, drop the
old table, rename the new one into place. Given near-zero current data,
the copy step is trivial and fast — this is not a large-scale backfill
migration.

**Partitions created:** monthly partitions from 12 months before this
migration's date through 12 months after, plus a `DEFAULT` partition on
each table to catch any row outside that window (Postgres errors on
insert with no matching partition and no default — the default partition
is the safety net, not an invitation to leave rows there long-term).

**Indexes:** each table's pre-existing secondary indexes (if any beyond
the PK) are recreated the same way, scoped per-partition automatically by
Postgres (a `CREATE INDEX` on the parent table creates it on every
partition and any partition created later).

**Operational gap, disclosed not solved:** nothing in this sub-project
automates creating next month's partition ahead of time. Once the
pre-created window (12 months out) is exhausted, new partitions must be
created manually (a documented `ALTER TABLE ... ATTACH PARTITION` /
`CREATE TABLE ... PARTITION OF` pair) or rows fall into the `DEFAULT`
partition. Automating this (e.g. a scheduled job, or the `pg_partman`
extension) is out of scope for this time-boxed pass — noted as follow-up
work, not silently ignored.

## Application code impact

Check whether any Go code assumes `telemetry_reading.id` (or
`telemetry_ingest_batch.id`) alone is sufficient to `UPDATE`/`DELETE` a
single row, or relies on the now-dropped FKs for behavior (not just
integrity) — e.g. `hard_delete.go`'s cascades already delete
child-then-parent in explicit order, so dropping the FK shouldn't change
its behavior, but this needs verification during implementation, not
assumption.

## Out of scope (explicit, due to the time-box)

- Retention policy / automatic old-partition dropping — partitioning
  only, per explicit decision.
- Automated future-partition creation (cron/pg_partman) — manual/follow-up.
- `telemetry_latest` — excluded, see above.
- Backfilling/migrating any real historical data — none exists yet.
- Index audit, connection-pool/query tuning — separate sub-projects.

## Verification

- Fresh-DB migration replay succeeds (same pattern as the schema-
  namespacing migration's own verification).
- `go build/vet/test` green in `services/platform-api` (same pre-existing
  unrelated blockers as before — `plant_image.go`/`plants.go`'s ImageUrl
  WIP — apply here too, not this sub-project's problem).
- Manual: insert a telemetry reading via the existing ingestion path,
  confirm it lands in the correct monthly partition
  (`SELECT tableoid::regclass, observed_at FROM telemetry.telemetry_reading`).
- Confirm `hard_delete.go`'s cascades still work end-to-end despite the
  dropped FKs (its own explicit-order deletes, not FK-driven, so should
  be unaffected — verify, don't assume).

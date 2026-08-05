# Telemetry Table Partitioning — Design

Sub-project 2 of 4 (database optimization request). Time-boxed: must land
within this session. Implemented and verified directly against the real
local database as part of writing this design — see Verification for
actual results, not just a plan.

## Context

Dev DB is currently near-empty (`telemetry_reading`: 0 rows,
`raw_register_reading`: 1 row, `telemetry_ingest_batch`: 52 rows) — no
backfill/data-migration risk right now. This is prep infrastructure:
convert the append-only, unboundedly-growing telemetry tables to native
Postgres RANGE partitioning by time, so future growth doesn't degrade
query/vacuum performance and future retention (dropping old partitions)
becomes a cheap `DROP TABLE`, not a slow `DELETE`. No retention policy is
enabled yet — partitioning only, per explicit decision.

## Scope: which tables (corrected twice during implementation)

- **`telemetry.telemetry_reading`** — partitioned by `observed_at`, monthly.
- **`telemetry.raw_register_reading`** — partitioned by `observed_at`, monthly.
- **`telemetry.telemetry_latest` excluded.** It's a keyed snapshot table
  (`PRIMARY KEY (organization_id, device_id)`, one row per device,
  `UPDATE ... ON CONFLICT` in place) — not append-only, doesn't grow with
  time. Its `observed_at` changes on every update, which would force
  constant cross-partition row movement under time-based RANGE
  partitioning, for zero benefit (the table's size is bounded by device
  count, not time).
- **`telemetry.telemetry_ingest_batch` excluded** — found mid-implementation,
  not anticipated in the original scope. Its idempotency dedup
  (`ON CONFLICT (middleware_client_id, idempotency_key) DO UPDATE`) has no
  caller-supplied event-time column: `received_at` is a DB-generated
  `now()` value, different on every insert attempt including immediate
  retries (confirmed by reading the Go call site —
  `CreateOrGetIngestBatchParams` has no caller-supplied timestamp field).
  Postgres requires a partitioned table's UNIQUE constraints to include
  the partition key column; widening this one to
  `(middleware_client_id, idempotency_key, received_at)` would make the
  idempotency check fire almost never, silently defeating the exact
  guarantee it exists for. `telemetry_reading`/`raw_register_reading`
  don't have this problem — their dedup key includes `observed_at`, which
  IS a caller-supplied event-time value, stable across retries of the same
  physical reading. This distinction was verified directly against the
  real database (see Verification), not just reasoned about.

Both exclusions are technical corrections to the original request's "all
4 tables" — flagged here, not re-litigated given the time-box.

## Foreign-key fallout

Postgres requires a partitioned table's PRIMARY KEY/UNIQUE constraints to
include the partition key column. Only one incoming FK is affected, since
`telemetry_ingest_batch` staying unpartitioned means its existing
`(organization_id, id)` unique constraint — the FK target for
`telemetry_reading.ingest_batch_id` and `raw_register_reading.ingest_batch_id`
— never changes:

- `telemetry_latest.telemetry_reading_id REFERENCES telemetry_reading(id)`
  — dropped. `telemetry_reading`'s PK becomes `(id, observed_at)`, so the
  old `(id)`-only FK target no longer exists.

**Decision (explicit, from the human partner): drop this FK rather than
widen it with a duplicated timestamp column.** Standard pattern for
Postgres time-series partitioning — referential integrity here becomes an
application-level convention instead of a DB-enforced constraint.
Accepted trade-off given the time-box; `hard_delete.go`'s existing
cross-table cascades already delete in explicit dependency order, not by
relying on `ON DELETE RESTRICT` to catch mistakes, so this doesn't change
actual delete-cascade behavior.

`telemetry_reading_batch_fk` and `raw_register_reading_batch_fk` (both
referencing `telemetry_ingest_batch`) are untouched.

## Dedup constraint widening

`telemetry_reading_client_external_unique` and
`raw_register_reading_client_external_unique` both widen from
`(middleware_client_id, external_key)` to
`(middleware_client_id, external_key, observed_at)`, and the corresponding
`ON CONFLICT` targets in `queries/ingestion.sql` and
`queries/raw_ingestion.sql` were updated to match. Verified directly
against the real database (see Verification) that this correctly still
dedupes a retry of the same physical reading (same `external_key` +
`observed_at`) while correctly accepting a genuinely different reading.

## Migration approach

One new migration:
`services/platform-api/internal/database/migrations/000034_telemetry_partitioning.sql`,
applied after `000033_schema_namespacing.sql` — every table reference in
it is schema-qualified (`telemetry.telemetry_reading`, `plant.plant`,
`auth.middleware_client`, etc.) for the same reason `000033` required
qualifying application code: bare names resolve via `search_path`, and
these tables no longer live in `public`.

Postgres has no `ALTER TABLE ... PARTITION BY`. Converting an existing
table requires: rename it aside, create a new partitioned table with the
same columns/constraints (PK widened to include the partition key, the
one FK above dropped), create its partitions, copy any existing rows
across, recreate secondary indexes, drop the renamed-aside original.
Given near-zero current data, the copy step is trivial and fast — not a
real backfill migration.

**Naming collision, found by actually running the migration:** `RENAME
TABLE` does not rename the table's PRIMARY KEY/UNIQUE constraints or
plain indexes — those keep their original names, which then collide with
the new table's same-named constraints/indexes (PK/UNIQUE-backed indexes
and plain indexes both occupy schema-wide-unique names in Postgres; CHECK
and FOREIGN KEY constraint names only need to be unique per-table, so
those don't collide). Fixed by explicitly renaming the old table's
PK/UNIQUE constraints and secondary indexes (e.g. `telemetry_reading_pkey`
→ `telemetry_reading_old_pkey`) immediately after the table rename, before
creating the new table.

**Partitions created:** monthly partitions from 2026-06 through 2027-02
(3 months of runway before, 6 after, relative to this migration's
2026-08-05 authoring date), plus a `DEFAULT` partition on each table to
catch any row outside that window (Postgres errors on insert with no
matching partition and no default — the default partition is the safety
net, not an invitation to leave rows there long-term). Narrower than a
full year given this is dev-only data right now; expanding the window is
a cheap follow-up migration, not a redesign.

**Operational gap, disclosed not solved:** nothing in this sub-project
automates creating future partitions ahead of the pre-created window.
Once exhausted, new partitions must be created manually or rows fall into
`DEFAULT`. Automating this (scheduled job, or the `pg_partman` extension)
is out of scope for this time-boxed pass.

## Verification (actually run, not just planned)

- **Migration applied to the real local dev database** (`platform-admin
  migrate` against `127.0.0.1:5432`, credentials from
  `deploy/local/.env`). First attempt failed twice during development —
  once on unqualified table names (fixed by schema-qualifying the whole
  migration), once on the constraint/index naming collision above (fixed
  by explicit renames) — both caught by actually running it, not by
  review alone. Confirmed via `pg_partitioned_table`/`pg_inherits` that
  both tables are correctly partitioned into the expected 8 partitions
  each (7 monthly + default), the pre-existing 1 row in
  `raw_register_reading` survived the copy, `telemetry_ingest_batch`'s 52
  rows and non-partitioned status are untouched, and no leftover `_old`
  tables remain.
- **Transactional-rollback safety confirmed directly**: the first failed
  attempt was verified (via direct query) to have left zero trace in
  `schema_migrations` and zero schema change — the migration runner's
  per-file transaction wrapping worked exactly as relied upon.
- **Dedup semantics verified against the real database**, not just
  reasoned about: in a rolled-back transaction (no permanent data), a
  reading was inserted, an exact retry (same `external_key` +
  `observed_at`) correctly returned zero rows (deduped), and a reading
  with a different `observed_at` correctly inserted as a new row and
  landed in the correct `telemetry_reading_2026_08` partition.
- `sqlc generate` clean after the `ON CONFLICT` query edits.
- `go build ./...` / `go vet ./...` in `services/platform-api`: no new
  errors — only the same 7 pre-existing, unrelated `ImageUrl` WIP errors
  in `plant_image.go`/`plants.go` documented in the schema-namespacing
  sub-project, confirming this change introduces no new breakage.
- **Not run**: `go test ./...` for `internal/core`/`internal/ingestion`
  (blocked by the same pre-existing `ImageUrl` compile failure as before
  — not this sub-project's problem, same documented limitation carried
  over from sub-project 1).

## Out of scope (explicit, due to the time-box)

- Retention policy / automatic old-partition dropping — partitioning
  only, per explicit decision.
- Automated future-partition creation (cron/pg_partman) — manual/follow-up.
- `telemetry_latest`, `telemetry_ingest_batch` — excluded, see above.
- Backfilling/migrating any real historical data — none exists yet.
- Index audit, connection-pool/query tuning — separate sub-projects.

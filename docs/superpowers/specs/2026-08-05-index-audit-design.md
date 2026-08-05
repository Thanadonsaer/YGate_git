# Index Audit — Design

Sub-project 3 of 4 (database optimization request). Time-boxed. Implemented
and verified directly against the real local database, not just planned.

## Method

Queried `pg_constraint`/`pg_index` directly against the real dev database
to find every FK column across all 7 schemas + `public` with no covering
index (Postgres never auto-indexes FK columns, unlike PK/UNIQUE). ~25
gaps found initially. Two turned out to be false positives once checked
against actual index definitions (not just an FK-column-order-exact-match
heuristic): `auth.user_role_user_scope_idx` already covers
`(user_id, organization_id, ...)` — reversed order from the FK
declaration, but Postgres uses both as equality conditions regardless of
order — and `telemetry.telemetry_latest_plant_time_idx` already covers
`(organization_id, plant_id, ...)`. Neither needed a new index.

Of the remaining real gaps, only the ones with a verified real query
(grepped against actual `WHERE` clauses in the codebase, not assumed)
were added — every index has a write-cost tradeoff, so this is a
targeted pass, not an exhaustive one. Low-traffic admin/audit columns on
small tables (`scada_screen.updated_by`/`created_by`,
`middleware_patch.uploaded_by`, `site_setting.updated_by`,
`alarm_rule.notify_role_id`, `email_verification_token.user_id`) were
left out — candidates for a future pass if they show up in real
slow-query logs, not worth the write cost blind.

## What was added

New migration `services/platform-api/internal/database/migrations/000035_index_audit.sql`,
three indexes, all matched against `hard_delete.go`'s cascade-by-
middleware-client-id deletes (lines 187-189), which had no covering index
on any of the three tables it filters:

- `telemetry.telemetry_reading (organization_id, middleware_client_id)`
- `telemetry.raw_register_reading (organization_id, middleware_client_id)`
- `telemetry.telemetry_ingest_batch (organization_id, middleware_client_id)`

Created on each partitioned table's parent relation — Postgres
auto-propagates to every existing and future partition, no per-partition
`CREATE INDEX` needed.

## Verification (actually run)

- Applied to the real local dev database (`platform-admin migrate`) —
  clean.
- `EXPLAIN` on the exact `hard_delete.go` query pattern
  (`WHERE organization_id=$1 AND middleware_client_id=$2`) against
  `telemetry_reading` confirms the planner uses the new index (`Index
  Only Scan`) across every partition, not a sequential scan.
- `go build ./...` / `go vet ./...`: no new errors (pure DDL, no
  application code or query file changes needed — `middleware_client_id`
  filtering isn't part of any sqlc-managed query, only `hard_delete.go`'s
  raw SQL, which was already schema-qualified in sub-project 1).

## Out of scope (time-boxed)

- The ~20 lower-traffic FK gaps not matched against a real hot-path query.
- Query-plan tuning beyond adding missing indexes (e.g. rewriting
  existing queries) — that's sub-project 4 (connection-pool/query
  tuning)'s territory.
- `EXPLAIN ANALYZE` against realistic data volumes — current dev DB is
  near-empty, so cost estimates are illustrative, not load-tested.

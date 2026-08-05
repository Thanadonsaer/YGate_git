# Connection Pool Tuning — Design

Sub-project 4 of 4 (database optimization request, final one). Time-boxed.
Verified directly, not just planned.

## Finding

Neither `services/platform-api` nor `services/auth-service` configures
`pgxpool` at all — both call `pgxpool.New(ctx, databaseURL)` with zero
custom settings. Checked pgx's actual applied defaults directly (not
assumed): `MaxConns=32`, `MinConns=0`, `MaxConnLifetime=1h`,
`MaxConnIdleTime=30m`.

`MaxConns=32` was a reasonable default when `platform-api` was the only
process talking to Postgres. Now that `auth-service` runs as a separate
process against the same instance (sub-project 1's schema-namespacing
work is explicitly prep for splitting out more services per
`docs/superpowers/specs/2026-08-01-backend-microservices-split-design.md`),
every service defaulting to 32 connections risks exceeding Postgres's own
`max_connections` (default 100) as more services come online — 4 services
at the current default alone would request up to 128 connections.

`MinConns=0`/`MaxConnLifetime`/`MaxConnIdleTime` are left at pgx's
defaults — reasonable, not worth changing given the time-box.

## Fix

`normalizeDatabaseURL` in both services'
`internal/config/config.go` now defaults `pool_max_conns=10` in the
connection URL when the operator hasn't already set it — pgx's
`pgxpool.ParseConfig` (used internally by `pgxpool.New`) already
recognizes `pool_max_conns` as a standard connection-string parameter, so
this needed no `pgxpool.Config`/`database.Open` changes, only a URL
default, matching the file's existing pattern for the `schema`→
`search_path` translation. An operator can still override via
`?pool_max_conns=N` in `PLATFORM_DATABASE_URL`/`AUTH_DATABASE_URL` —
this only fills the gap when unset.

`10` per service leaves headroom for the 8-service target architecture
(8 × 10 = 80, under the default 100) while still being generous for
current single-instance load.

## Verification

- `go build`/`go vet ./...` both services: clean (platform-api's only
  errors are the same 7 pre-existing, unrelated `ImageUrl` WIP errors).
- `go test ./internal/config/...` both services: 2 existing tests
  asserted an exact `DatabaseURL` string and needed updating to include
  the new `?pool_max_conns=10` — legitimate, expected consequence of this
  change, not a regression. Both green after the update.
- `go test ./...` in auth-service: fully green (unaffected — this
  service doesn't import `internal/core`, no `ImageUrl` cascade).
- Confirmed via direct connection that `pool.Config().MaxConns` actually
  reflects the new value when the URL param is present (checked pgx's own
  parsed config, not just that the string looks right).

## Out of scope (time-boxed)

- Query-level tuning (rewriting slow queries, `EXPLAIN ANALYZE` against
  realistic data volumes) — current dev DB is near-empty, nothing slow to
  find yet; revisit once real traffic exists.
- `MinConns`/`MaxConnLifetime`/`MaxConnIdleTime` tuning — left at pgx
  defaults, no evidence they're wrong for this workload yet.
- Coordinating pool sizing across all 8 target services' eventual
  `max_connections` budget — `10` per service is a reasonable starting
  point, not a load-tested final answer.

# Database Schema Namespacing — Design

Sub-project 1 of 4 (database optimization request: schema namespacing,
telemetry partition/retention, index audit, connection-pool/query tuning).
Others are separate specs, not covered here.

## Context

`docs/superpowers/specs/2026-08-01-backend-microservices-split-design.md`
already decided: one shared PostgreSQL instance, not database-per-service,
because `hard_delete.go` cascades across Plant/Device/RegisterMetadata/
telemetry in one transaction and true database-per-service would need a
distributed transaction/saga to keep that safe. That decision stands.

This sub-project is narrower: group the 33 existing tables (all currently
in the default `public` schema) into 7 named Postgres **schemas** inside
that same single database/instance, matching the target service map's
domain boundaries. Cross-schema queries and single-transaction cascades
inside one Postgres database are unaffected by this — only
database-per-service would have broken them. This is prep work ahead of
future service-extraction phases (only `auth-service` has actually been
extracted as its own process so far); it does not extract any new service
itself.

## Table → schema mapping

Verified against actual code (not just the old doc's Go-file list — one
correction found: `middleware_client` is written exclusively by
`auth-service`'s `internal/core/api_keys.go`, so it belongs to `auth`, not
`middleware_gateway`, despite the "middleware" name).

| Schema | Tables |
|---|---|
| `auth` | app_user, role, permission, role_permission, user_role, user_session, password_reset_token, auth_attempt, password_recovery_attempt, email_verification_token, middleware_client |
| `plant` | plant, asset_group, device_model, device, device_register_metadata, device_model_register_metadata |
| `telemetry` | telemetry_ingest_batch, telemetry_reading, telemetry_latest, raw_register_reading |
| `middleware_gateway` | middleware_config_history, middleware_plant, middleware_patch |
| `scada` | scada_screen, scada_screen_publication |
| `alarm` | alarm_rule, alarm_event, alarm_rule_condition |
| `dashboard` | user_dashboard |

**Stays in `public`** (no single domain owner):
- `organization` — tenant FK referenced by nearly every domain
- `audit_log` — shared write target for every service's audit trail (per
  the existing architecture doc: "each service writes audit events into
  the same shared table directly")
- `site_setting` — platform-wide singleton config
- `schema_migrations` — the migration ledger table itself; infra, not
  domain data, and it's created by `database.Migrate()` at runtime, not
  by a numbered migration file

**Not created:** a `notification` schema. The notification domain
(`services/auth-service/internal/notification`) is stateless SMTP sending
with no table of its own. Add the schema when it first owns one — an
empty schema with nothing to move into it is speculative.

## Migration

One new file: `services/platform-api/internal/database/migrations/000033_schema_namespacing.sql`.
`CREATE SCHEMA IF NOT EXISTS <name>;` for each of the 7 schemas, then
`ALTER TABLE <table> SET SCHEMA <schema>;` for each of the 30 tables being
moved. Forward-only, matching every other migration in this directory — no
down-migration exists anywhere in this codebase, this one doesn't add one
either. `ALTER TABLE ... SET SCHEMA` does not rewrite table data or drop
existing indexes/constraints/sequences/triggers — those move with the
table, no separate handling needed.

This migration ships to production exactly like every other one: it's
picked up by `//go:embed migrations/*.sql` in
`services/platform-api/internal/database/database.go`, and applied by the
same `database.Migrate()` — invoked identically by `platform-api`'s normal
startup and by `platform-admin.exe migrate`
(`services/platform-api/cmd/platform-admin/main.go`), which is the step
`deploy/manual/build-release.ps1:210` runs during a production deploy.
There is no separate production migration path to account for.

## Query qualification

Every table reference in every `.sql` file under
`services/platform-api/internal/database/queries/` and
`services/auth-service/internal/database/queries/` changes from bare
(`FROM device`) to schema-qualified (`FROM plant.device`) — including
cross-domain reads that already exist today, e.g. `telemetry.go`'s query
joining `device`/`device_register_metadata` becomes
`plant.device`/`plant.device_register_metadata`. `hard_delete.go`'s
cross-domain cascade SQL gets the same treatment.

This is a hard requirement, not optional cleanup: once tables move out of
`public`, an unqualified reference only resolves via `search_path`, and
this codebase's connection `search_path` is normally just `public`
(`schema=public` in `DATABASE_URL`, see Production config below) — so an
unqualified `FROM device` would fail with "relation does not exist" the
moment `device` leaves `public`. Qualifying every reference removes any
dependency on `search_path` contents or ordering.

**Required step, easy to miss:** after editing the `.sql` query files,
`sqlc generate` must be re-run in **both** `services/platform-api` and
`services/auth-service`, and the regenerated `internal/database/dbgen/*.go`
files must be committed. Verified: `deploy/manual/build-release.ps1` does
**not** run `sqlc generate` — it only compiles the Go binaries from
whatever `dbgen` code is already checked into git (`dbgen/*.go` is
git-tracked, not gitignored). If the query files change but `dbgen` isn't
regenerated and committed, the production binary keeps shipping the old
unqualified queries and breaks as soon as this migration runs. No
`sqlc.yaml` config change is needed in either service — both already point
their schema-replay source at `services/platform-api/internal/database/migrations`
(auth-service via a relative path), so both pick up the new schema
automatically once `sqlc generate` runs after the migration file exists.

## Production config

`DATABASE_URL`/`PLATFORM_DATABASE_URL` in `deploy/local/.env` (and
`.env.example`) carry `?schema=public`, which `normalizeDatabaseURL` in
`services/*/internal/config/config.go` turns into `search_path=public` on
the connection. This does **not** need to change: with every query
schema-qualified, the connection's `search_path` no longer determines
which table a query hits — `public` remains correct for the objects that
actually stay there (`organization`, `audit_log`, `site_setting`,
`schema_migrations`).

## Test harness fix

Three integration test files under
`services/platform-api/internal/ingestion/` (`service_integration_test.go`,
`alarms_integration_test.go`, `raw_service_integration_test.go`) each spin
up one ephemeral Postgres **schema** per test run
(`CREATE SCHEMA ingestion_test_<timestamp>`), point `search_path` at just
that schema, and replay every migration into it from scratch — relying on
every migration creating unqualified (`public`-resolved-via-search_path)
objects that all land in that one ephemeral schema.

This breaks once migration `000033` starts doing
`CREATE SCHEMA auth` / `ALTER TABLE ... SET SCHEMA auth` (etc.) with fixed,
literal schema names: those always target the literally-named schema
regardless of `search_path`, so two tests running in parallel would both
try to `CREATE SCHEMA auth` and collide (unlike today's random-suffixed
ephemeral schema name, which never collides).

Fix: switch these three helpers from "ephemeral schema, `search_path`
override" to "ephemeral **database** per test run" —
`CREATE DATABASE ingestion_test_<timestamp>`, connect to it, run the normal
migration replay (which now creates `auth`/`plant`/`telemetry`/etc. inside
that dedicated database, no collision possible), and `DROP DATABASE
ingestion_test_<timestamp>` in cleanup instead of `DROP SCHEMA ... CASCADE`.

## Out of scope

- Extracting any service beyond the already-completed `auth-service` —
  this only prepares schema layout ahead of future phases.
- Telemetry partitioning/retention, index audit, connection-pool/query
  tuning — separate sub-projects, specced later.
- A down-migration/rollback path — this codebase has none anywhere,
  consistent with existing practice.
- Renaming any table, column, or changing any constraint — this is a
  schema-location move only.

## Verification

- `go build ./... && go vet ./... && go test ./...` green in both
  `services/platform-api` and `services/auth-service` after the migration,
  query-qualification, and `sqlc generate` steps.
- Fresh local database (`DROP DATABASE` + recreate, or a clean container)
  boots `platform-api` successfully — confirms migration 000033 applies
  cleanly from empty, not just as an incremental step on an
  already-migrated database.
- Manual: log in, manage plants/devices, view telemetry, SCADA, alarms,
  dashboard — full walkthrough through the web UI with schema-qualified
  queries live, confirming no regression from the qualification pass.
- The three updated integration test files pass with the new
  throwaway-database helper, including when run with `go test -run
  Integration ./... -count=1` back to back (confirms no collision from
  fixed schema names, the exact failure mode this fix prevents).

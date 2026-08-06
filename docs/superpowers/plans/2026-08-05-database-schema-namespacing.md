# Database Schema Namespacing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move 30 of 33 existing PostgreSQL tables out of `public` into 7
domain schemas (`auth`, `plant`, `telemetry`, `middleware_gateway`,
`scada`, `alarm`, `dashboard`), matching the microservices target service
map, and qualify every SQL reference to those tables so the app keeps
working identically — no behavior change, no new dependency, no process
split.

**Architecture:** One new forward-only migration creates the schemas and
moves the tables (`ALTER TABLE ... SET SCHEMA ...` — data, indexes,
constraints, sequences all move with it, no rewrite). Every table
reference becomes schema-qualified in both services' sqlc query files
(`.sql`) and in raw inline SQL in Go source — nothing may rely on
`search_path` to find a moved table. One test-isolation helper that
creates ephemeral Postgres objects per test run has to switch from
"ephemeral schema" to "ephemeral database", because the new schemas have
fixed, literal names that would collide across parallel/repeated test
runs the old scheme never risked.

**Tech Stack:** PostgreSQL (schemas, `ALTER TABLE ... SET SCHEMA`), Go
(`pgx/v5`), `sqlc` (query codegen for both `services/platform-api` and
`services/auth-service`).

## Global Constraints

- Table → schema mapping (authoritative, from
  `docs/superpowers/specs/2026-08-05-database-schema-namespacing-design.md`):
  - `auth`: app_user, role, permission, role_permission, user_role, user_session, password_reset_token, auth_attempt, password_recovery_attempt, email_verification_token, middleware_client
  - `plant`: plant, asset_group, device_model, device, device_register_metadata, device_model_register_metadata
  - `telemetry`: telemetry_ingest_batch, telemetry_reading, telemetry_latest, raw_register_reading
  - `middleware_gateway`: middleware_config_history, middleware_plant, middleware_patch
  - `scada`: scada_screen, scada_screen_publication
  - `alarm`: alarm_rule, alarm_event, alarm_rule_condition
  - `dashboard`: user_dashboard
- `organization`, `audit_log`, `site_setting`, and the `schema_migrations`
  ledger table stay in `public`, **unqualified, no changes anywhere** —
  do not add a `public.` prefix to these; they're correct as-is since
  `public` stays the default `search_path`.
- Do not create a `notification` schema — that domain owns no table yet.
- No down-migration — this codebase has none anywhere in
  `services/platform-api/internal/database/migrations/`; this migration
  doesn't add the first one.
- No renaming of any table, column, constraint, or index — schema
  location only.
- After editing any `.sql` query file in either service, `sqlc generate`
  must be re-run in that service's directory and the regenerated
  `internal/database/dbgen/*.go` files committed. Verified:
  `deploy/manual/build-release.ps1` does **not** run `sqlc generate` — it
  only compiles from whatever `dbgen` is already checked into git. A
  missed regeneration means the production binary ships stale,
  unqualified queries that fail the moment this migration runs there.

---

### Task 1: Migration — create schemas, move tables

**Files:**
- Create: `services/platform-api/internal/database/migrations/000033_schema_namespacing.sql`

**Interfaces:**
- Produces: 7 new Postgres schemas and relocates 30 tables into them,
  applied automatically by `database.Migrate()`
  (`services/platform-api/internal/database/database.go`) the same way
  every other migration is — on `platform-api` startup and via
  `platform-admin.exe migrate`. Nothing downstream in this plan can be
  verified until this migration exists and has been applied to the
  database the implementer tests against.

- [ ] **Step 1: Write the migration**

Create `services/platform-api/internal/database/migrations/000033_schema_namespacing.sql`:

```sql
-- Group existing tables into domain schemas ahead of future
-- service-extraction phases (see docs/superpowers/specs/
-- 2026-08-01-backend-microservices-split-design.md and
-- 2026-08-05-database-schema-namespacing-design.md).
-- organization, audit_log, and site_setting stay in public: no single
-- domain owns them.

CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS plant;
CREATE SCHEMA IF NOT EXISTS telemetry;
CREATE SCHEMA IF NOT EXISTS middleware_gateway;
CREATE SCHEMA IF NOT EXISTS scada;
CREATE SCHEMA IF NOT EXISTS alarm;
CREATE SCHEMA IF NOT EXISTS dashboard;

ALTER TABLE app_user SET SCHEMA auth;
ALTER TABLE role SET SCHEMA auth;
ALTER TABLE permission SET SCHEMA auth;
ALTER TABLE role_permission SET SCHEMA auth;
ALTER TABLE user_role SET SCHEMA auth;
ALTER TABLE user_session SET SCHEMA auth;
ALTER TABLE password_reset_token SET SCHEMA auth;
ALTER TABLE auth_attempt SET SCHEMA auth;
ALTER TABLE password_recovery_attempt SET SCHEMA auth;
ALTER TABLE email_verification_token SET SCHEMA auth;
ALTER TABLE middleware_client SET SCHEMA auth;

ALTER TABLE plant SET SCHEMA plant;
ALTER TABLE asset_group SET SCHEMA plant;
ALTER TABLE device_model SET SCHEMA plant;
ALTER TABLE device SET SCHEMA plant;
ALTER TABLE device_register_metadata SET SCHEMA plant;
ALTER TABLE device_model_register_metadata SET SCHEMA plant;

ALTER TABLE telemetry_ingest_batch SET SCHEMA telemetry;
ALTER TABLE telemetry_reading SET SCHEMA telemetry;
ALTER TABLE telemetry_latest SET SCHEMA telemetry;
ALTER TABLE raw_register_reading SET SCHEMA telemetry;

ALTER TABLE middleware_config_history SET SCHEMA middleware_gateway;
ALTER TABLE middleware_plant SET SCHEMA middleware_gateway;
ALTER TABLE middleware_patch SET SCHEMA middleware_gateway;

ALTER TABLE scada_screen SET SCHEMA scada;
ALTER TABLE scada_screen_publication SET SCHEMA scada;

ALTER TABLE alarm_rule SET SCHEMA alarm;
ALTER TABLE alarm_event SET SCHEMA alarm;
ALTER TABLE alarm_rule_condition SET SCHEMA alarm;

ALTER TABLE user_dashboard SET SCHEMA dashboard;
```

Note: `ALTER TABLE plant SET SCHEMA plant;` (schema named `plant`, table
named `plant`) is intentional and valid — Postgres schema names and table
names are independent namespaces; the table becomes `plant.plant`. `ALTER
TABLE ... SET SCHEMA` never requires FK-dependency ordering — cross-schema
foreign keys are fully supported — so the statement order above (grouped
by target schema) is for readability only, not correctness.

- [ ] **Step 2: Apply it to a real database and confirm it lands clean**

Point `PLATFORM_DATABASE_URL` (or `DATABASE_URL`) at a real local Postgres
instance with the existing 32 migrations already applied (or empty — the
runner replays everything needed either way), then run:

```
cd services/platform-api
go run ./cmd/platform-admin migrate
```

Expected: exits 0, and `psql` (or any client) against that database shows
`\dn` listing `auth`, `plant`, `telemetry`, `middleware_gateway`, `scada`,
`alarm`, `dashboard`, `public` — and `\dt auth.*` / `\dt plant.*` / etc.
show the tables from the Global Constraints mapping in their new homes,
`\dt public.*` showing only `organization`, `audit_log`, `site_setting`,
`schema_migrations` remaining.

- [ ] **Step 3: Commit**

```bash
git add services/platform-api/internal/database/migrations/000033_schema_namespacing.sql
git commit -m "feat(db): move tables into 7 domain schemas ahead of service extraction"
```

---

### Task 2: Fix ephemeral test-database helper (collision-safety)

**Files:**
- Modify: `services/platform-api/internal/ingestion/service_integration_test.go`

**Interfaces:**
- Consumes: Task 1's migration must exist (this task's own verification
  needs it, even though the fix itself only touches Go code).
- Produces: `disposablePool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool`
  — same name and signature as today, called unchanged by
  `raw_service_integration_test.go` and `alarms_integration_test.go` (no
  changes needed at either call site).

- [ ] **Step 1: Understand why the current helper breaks**

`disposablePool` today (`service_integration_test.go:215`) creates one
ephemeral **schema** with a random timestamped name, points
`search_path` at just that schema, then replays every migration into it
— every migration's unqualified `CREATE TABLE` lands inside that one
random schema via `search_path` resolution, so two parallel test runs
never collide (different random schema names).

Migration 000033 breaks this: `CREATE SCHEMA auth` and
`ALTER TABLE app_user SET SCHEMA auth` target the **literal** name `auth`
regardless of `search_path` — two test runs sharing one Postgres instance
would both try to create/populate the same literal `auth` schema and
collide (and worse, a table `ALTER`ed out of the ephemeral wrapper schema
into a shared literal schema never gets cleaned up by
`DROP SCHEMA <wrapper> CASCADE`, since it no longer lives under that
wrapper).

- [ ] **Step 2: Replace the helper body**

In `services/platform-api/internal/ingestion/service_integration_test.go`,
replace the full body of `disposablePool` (currently lines 215–246):

```go
func disposablePool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Del("schema")
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	admin, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("ingestion_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE")
		admin.Close()
	})
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	pool, err := database.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	return pool
}
```

with:

```go
func disposablePool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Del("schema")
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	admin, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("ingestion_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	dbURL := *parsed
	dbURL.Path = "/" + name
	pool, err := database.Open(ctx, dbURL.String())
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+identifier)
		admin.Close()
	})
	return pool
}
```

This keeps the same collision-free guarantee (a fresh, uniquely-named
Postgres object per test run) at the database level instead of the
schema level, which survives migration 000033's fixed schema names.
`CREATE DATABASE`/`DROP DATABASE` run as single statements via `Exec`
(same mechanism the old code used for `CREATE SCHEMA`/`DROP SCHEMA` —
neither can run inside an explicit multi-statement transaction block, and
neither needs to).

- [ ] **Step 3: Verify collision-safety**

With Task 1's migration already applied to whatever database
`PLATFORM_TEST_DATABASE_URL` (or the URL these tests otherwise use) points
at, run the affected package's tests twice in a row:

```
cd services/platform-api
go test ./internal/ingestion/... -run Integration -v -count=1
go test ./internal/ingestion/... -run Integration -v -count=1
```

Expected: both runs fail the same way — with errors from unqualified
table references inside the tests themselves (e.g. "relation \"device\"
does not exist"), **not** with a "database already exists" or schema
collision error. The relation-does-not-exist failures are expected and
correct at this point in the plan (Tasks 4–5 haven't qualified those
queries yet); what this step confirms is that the harness itself no
longer collides. If you see any `database "..." already exists` or
similar collision error, the fix is wrong — stop and reconsider.

- [ ] **Step 4: Commit**

```bash
git add services/platform-api/internal/ingestion/service_integration_test.go
git commit -m "fix(platform-api): make disposablePool collision-safe for named schemas"
```

---

### Task 3: Qualify auth-service (queries + raw SQL)

**Files:**
- Modify: `services/auth-service/internal/database/queries/auth.sql`
- Modify: `services/auth-service/internal/database/queries/core.sql`
- Modify: `services/auth-service/internal/database/dbgen/*.go` (regenerated, not hand-edited)
- Modify: `services/auth-service/internal/core/users.go`
- Modify: `services/auth-service/internal/core/roles.go`
- Modify: `services/auth-service/internal/core/helpers.go`
- Modify: `services/auth-service/internal/core/api_keys.go`
- Modify: `services/auth-service/internal/core/profile.go`

**Interfaces:**
- Consumes: Task 1's migration (schemas must exist for `sqlc generate` to
  resolve qualified names against the replayed schema).
- Produces: nothing new — every existing function signature in this
  service stays the same; only the SQL text inside them changes.

- [ ] **Step 1: Qualify `auth-service`'s sqlc query files**

In `services/auth-service/internal/database/queries/auth.sql` and
`core.sql`, add the `auth.` prefix to every bare reference to:
`app_user`, `role`, `permission`, `role_permission`, `user_role`,
`user_session`, `password_reset_token`, `auth_attempt`,
`password_recovery_attempt`, `email_verification_token`,
`middleware_client` — in `FROM`, `JOIN`, `INTO`, `UPDATE`, and
`REFERENCES` position. Leave `organization` unqualified (stays `public`).

Worked example (the permission-check pattern that recurs throughout both
services — auth-service's files use it too):

```sql
-- before
SELECT 1 FROM user_role ur
JOIN role r ON r.id = ur.role_id
JOIN role_permission rp ON rp.role_id = ur.role_id
JOIN permission pm ON pm.id = rp.permission_id

-- after
SELECT 1 FROM auth.user_role ur
JOIN auth.role r ON r.id = ur.role_id
JOIN auth.role_permission rp ON rp.role_id = ur.role_id
JOIN auth.permission pm ON pm.id = rp.permission_id
```

Simple single-table worked example:

```sql
-- before
INSERT INTO app_user(id, organization_id, email, username, display_name, password_hash, email_verified_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)

-- after
INSERT INTO auth.app_user(id, organization_id, email, username, display_name, password_hash, email_verified_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
```

Apply the same substitution to every remaining reference in both files —
`sqlc` itself is the completion check here (next step).

- [ ] **Step 2: Regenerate and let `sqlc` catch anything missed**

```
cd services/auth-service
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```

(bare `sqlc` is not installed/on PATH in this environment — use the
versioned `go run` invocation documented in
`services/platform-api/README.md`'s "Generate queries" section; it's the
same tool, same version, no separate install needed.)

Expected on the first run: one error per still-unqualified reference,
naming the file and line (e.g. "relation \"user_session\" does not
exist"). Fix each one the same way as Step 1 and re-run `sqlc generate`
until it exits clean.

- [ ] **Step 3: Qualify raw inline SQL in `internal/core`**

These files build SQL as Go string literals passed directly to
`tx.Exec`/`tx.QueryRow` — not through sqlc, so nothing catches a miss
except `go vet`/`go test` at best (a raw SQL string with a bad table name
is just a string to the Go compiler; only a failing query at runtime
reveals it). The full checklist below was produced by systematically
searching every non-test `.go` file in this package — nothing here is
guessed.

**`services/auth-service/internal/core/users.go`** — add `auth.` to
`app_user`, `user_role`, `role`, `role_permission`, `permission`,
`user_session`, `password_reset_token` at (line numbers as of this
plan's writing; if the file has drifted, search for these table names
instead of trusting exact numbers):
L76–98, L134–139, L211, L218, L266, L309, L312, L323, L327, L376, L379,
L382, L448, L477, L486, L508, L512, L560, L614, L642–647, L671–677, L696.

Worked example (L211):
```go
// before
"INSERT INTO app_user(id, organization_id, email, username, display_name, password_hash, email_verified_at) VALUES(...)"
// after
"INSERT INTO auth.app_user(id, organization_id, email, username, display_name, password_hash, email_verified_at) VALUES(...)"
```

**`services/auth-service/internal/core/roles.go`** — add `auth.` to
`permission`, `role`, `user_role`, `role_permission` at: L75, L107, L177,
L222, L244, L287, L302, L317, L355, L374, L379.

**`services/auth-service/internal/core/helpers.go`** — add `auth.` to
`user_role`, `role`, `role_permission`, `permission` at: L83–95 (the same
permission-check JOIN pattern as the worked example in Step 1).

**`services/auth-service/internal/core/api_keys.go`** — add `auth.` to
`middleware_client`, `user_role`, `role`, `role_permission`, `permission`
at: L78–96, L148, L190, L241, L264–270.

**`services/auth-service/internal/core/profile.go`** — add `auth.` to
`app_user` at: L63, L95.

Leave every reference to `organization` in all 5 files unqualified.

- [ ] **Step 4: Verify**

```
cd services/auth-service
go build ./...
go vet ./...
go test ./...
```

Expected: all green. If `PLATFORM_TEST_DATABASE_URL`-gated integration
tests exist in this service and are skipped without that env var set,
set it to a real database with Task 1's migration applied and re-run to
get real coverage, not just skips.

- [ ] **Step 5: Commit**

```bash
git add services/auth-service/internal/database/queries services/auth-service/internal/database/dbgen services/auth-service/internal/core
git commit -m "feat(auth-service): schema-qualify all queries for the auth schema move"
```

---

### Task 4: Qualify platform-api's sqlc query files

**Files:**
- Modify: `services/platform-api/internal/database/queries/core.sql`
- Modify: `services/platform-api/internal/database/queries/devices.sql`
- Modify: `services/platform-api/internal/database/queries/dashboard.sql`
- Modify: `services/platform-api/internal/database/queries/ingestion.sql`
- Modify: `services/platform-api/internal/database/queries/raw_ingestion.sql`
- Modify: `services/platform-api/internal/database/queries/telemetry.sql`
- Modify: `services/platform-api/internal/database/dbgen/*.go` (regenerated, not hand-edited)

**Interfaces:**
- Consumes: Task 1's migration.
- Produces: nothing new — same as Task 3, SQL text only.

- [ ] **Step 1: Qualify every bare reference to a moved table**

Across all 6 files, add the schema prefix from the Global Constraints
mapping to every bare reference to any of these 25 tables (the 4 tables
with no raw-SQL or sqlc usage found in `internal/core` —
`asset_group`, `auth_attempt`,
`password_recovery_attempt`, `email_verification_token` — may still
appear here; qualify them too if they do): app_user→`auth.`,
role→`auth.`, permission→`auth.`, role_permission→`auth.`,
user_role→`auth.`, user_session→`auth.`, middleware_client→`auth.`,
plant→`plant.`, device→`plant.`, device_model→`plant.`,
device_register_metadata→`plant.`,
device_model_register_metadata→`plant.`,
telemetry_ingest_batch→`telemetry.`, telemetry_reading→`telemetry.`,
telemetry_latest→`telemetry.`, raw_register_reading→`telemetry.`,
middleware_config_history→`middleware_gateway.`,
middleware_plant→`middleware_gateway.`,
middleware_patch→`middleware_gateway.`, scada_screen→`scada.`,
scada_screen_publication→`scada.`, alarm_rule→`alarm.`,
alarm_event→`alarm.`, alarm_rule_condition→`alarm.`,
user_dashboard→`dashboard.`. Leave `organization` unqualified.

Worked examples, taken directly from `core.sql` (same substitution
pattern applies everywhere in all 6 files):

```sql
-- before (core.sql, permission-check subquery — recurs throughout)
SELECT 1 FROM user_role ur
JOIN role r ON r.id = ur.role_id
JOIN role_permission rp ON rp.role_id = ur.role_id
JOIN permission pm ON pm.id = rp.permission_id

-- after
SELECT 1 FROM auth.user_role ur
JOIN auth.role r ON r.id = ur.role_id
JOIN auth.role_permission rp ON rp.role_id = ur.role_id
JOIN auth.permission pm ON pm.id = rp.permission_id
```

```sql
-- before (core.sql, plant list query)
FROM plant p
...
UPDATE plant
...
INSERT INTO plant (

-- after
FROM plant.plant p
...
UPDATE plant.plant
...
INSERT INTO plant.plant (
```

Note the `plant.plant` (schema `plant`, table `plant`) — this is correct
and matches migration 000033's `ALTER TABLE plant SET SCHEMA plant;`.

- [ ] **Step 2: Regenerate and let `sqlc` catch anything missed**

```
cd services/platform-api
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```

Fix every reported error the same way, re-run until clean. This is the
authoritative completion check for these 6 files — trust it over any
manual line count.

- [ ] **Step 3: Narrow build check (full test suite is not green yet)**

```
cd services/platform-api
go build ./...
go vet ./...
```

Expected: both clean. Do **not** run `go test ./...` yet and treat
failures as a problem — Task 5 hasn't qualified the raw-SQL code paths
in `internal/core` yet, so any test exercising `hard_delete.go`,
`middleware_config.go`, `scada.go`, `alarms.go`, `devices.go`,
`telemetry.go`, `permission.go`, `audit.go`, `middleware_plants.go`,
`middleware_patch.go`, or `plant_image.go` will still fail with
relation-does-not-exist errors until Task 5 lands. That's expected here,
not a regression to chase.

- [ ] **Step 4: Commit**

```bash
git add services/platform-api/internal/database/queries services/platform-api/internal/database/dbgen
git commit -m "feat(platform-api): schema-qualify sqlc queries for the schema move"
```

---

### Task 5: Qualify platform-api's raw SQL (final DB-layer task)

**Files:**
- Modify: `services/platform-api/internal/core/middleware_config.go`
- Modify: `services/platform-api/internal/core/devices.go`
- Modify: `services/platform-api/internal/core/plant_image.go`
- Modify: `services/platform-api/internal/core/middleware_plants.go`
- Modify: `services/platform-api/internal/core/middleware_patch.go`
- Modify: `services/platform-api/internal/core/alarms.go`
- Modify: `services/platform-api/internal/core/permission.go`
- Modify: `services/platform-api/internal/core/hard_delete.go`
- Modify: `services/platform-api/internal/core/telemetry.go`
- Modify: `services/platform-api/internal/core/scada.go`
- Modify: `services/platform-api/internal/core/audit.go`
- Modify: `services/platform-api/internal/sessioncheck/sessioncheck.go`
- Modify: `services/platform-api/internal/ingestion/service.go`
- Modify: `services/platform-api/internal/ingestion/alarms.go`
- Modify: `services/platform-api/internal/ingestion/service_integration_test.go`
- Modify: `services/platform-api/internal/ingestion/raw_service_integration_test.go`
- Modify: `services/platform-api/internal/ingestion/alarms_integration_test.go`
- Modify: `services/platform-api/cmd/platform-admin/main.go`

**Interfaces:**
- Consumes: Task 1's migration, Task 2's fixed test harness, Task 4's
  qualified sqlc queries (so the full test suite can finally go green
  here).
- Produces: nothing new — SQL text only. This is the last task touching
  the database layer; its own verification is the plan's closing
  full-suite check.

Same caveat as Task 3 Step 3: none of this is sqlc-managed, so nothing
catches a miss except a failing test at runtime. The `internal/core`
checklist below was produced by systematically searching every non-test
`.go` file in that one package (raw `Exec`/`Query`/`QueryRow` calls only —
sqlc-generated calls like `q.SomeMethod(...)` are already covered by
Task 4). Table names not listed for a file below genuinely don't appear
as raw SQL there — trust the list, don't re-derive it, but do search each
file for any of the 25 table names as a final sanity pass in Step 2.

**Scope correction, found during Task 3's review:** the original version
of this task was scoped only to `internal/core`, the same blind spot that
Task 3 hit in auth-service (raw SQL exists in other packages too, and in
test files, not just `internal/core`). A follow-up search covering the
rest of `services/platform-api` (every package except `internal/core` and
the sqlc-generated `internal/database/dbgen`, `_test.go` files included)
found 7 more files with genuine raw-SQL table references — these are
folded into this task's file list and checklist below rather than becoming
a separate task, since it's the same kind of mechanical work.

- [ ] **Step 1: Qualify each file**

**`middleware_config.go`** — `auth.` for `middleware_client`, `user_role`,
`role`, `role_permission`, `permission`; `middleware_gateway.` for
`middleware_plant`, `middleware_config_history`; `plant.` for `device`,
`plant`, `device_model`, `device_model_register_metadata`. Locations:
L241, L432–450, L508, L563, L606, L639–643, L679, L730, L735, L767–770,
L805–807, L835–838, L917, L952, L985, L994, L1003, L1036, L1048, L1053,
L1057, L1079–1085.

Worked example (L508):
```go
// before
"INSERT INTO middleware_client(...)"
// after
"INSERT INTO auth.middleware_client(...)"
```

**`devices.go`** — `plant.` for `device`, `device_model`,
`device_model_register_metadata`, `device_register_metadata`; `auth.` for
`user_role`, `role`, `role_permission`, `permission`. Locations: L164–173,
L197–214, L333–340, L391–406, L458, L497–500, L578–588, L654–665,
L701–705, L758–768, L807–822, L844–859, L954–957, L970–974, L983–985,
L1005–1009, L1032–1037, L1094–1115.

**`plant_image.go`** — `plant.` for `plant`. Locations: L103, L146.

**`middleware_plants.go`** — `middleware_gateway.` for `middleware_plant`;
`plant.` for `plant`; `auth.` for `middleware_client`. Locations: L31–38,
L82, L92, L101–103, L140, L149.

**`middleware_patch.go`** — `middleware_gateway.` for `middleware_patch`;
`auth.` for `app_user`, `middleware_client`. Locations: L87–89, L171–176,
L207, L235, L258, L314.

**`alarms.go`** — `alarm.` for `alarm_rule`, `alarm_event`,
`alarm_rule_condition`; `auth.` for `role`. Locations: L116–120,
L184–188, L243–247, L297, L320–326, L349–355, L379, L409–412, L429–434,
L477, L506, L526, L582, L605, L614–616.

**`permission.go`** — `auth.` for `user_role`, `role`, `role_permission`,
`permission`. Locations: L33–45 (same JOIN pattern as every other
permission check in this codebase).

**`hard_delete.go`** — `plant.` for `plant`, `device`, `device_model`;
`telemetry.` for `telemetry_latest`, `raw_register_reading`,
`telemetry_reading`, `telemetry_ingest_batch`; `auth.` for
`middleware_client`. Locations: L32, L43–49, L76, L87–92, L118, L129–134,
L164, L183, L186–190, L196–204. This file's cross-domain cascades (e.g.
deleting a Plant also deletes its Devices' telemetry) keep working
exactly as today — single Postgres instance, single transaction, schema
location doesn't affect transactional guarantees.

**`telemetry.go`** — `telemetry.` for `raw_register_reading`,
`telemetry_latest`; `plant.` for `device`, `device_register_metadata`,
`device_model_register_metadata`. Location: L105–143 (one large query;
qualify every bare table name inside it, including inside the `WITH`/CTE
and `LEFT JOIN LATERAL` subquery).

**`scada.go`** — `scada.` for `scada_screen`, `scada_screen_publication`;
`plant.` for `plant`, `device`; `auth.` for `user_role`, `role`,
`role_permission`, `permission`. Locations: L286–294, L332–335,
L392–396, L432, L440–443, L446, L484–489, L529, L571, L577–587 (the
`scadaPermissionExistsSQL` const, embedded into several queries above and
below — qualify it once where it's defined), L592–607, L618–627,
L630–640, L693–704, L720, L752.

**`audit.go`** — `auth.` for `app_user`, `user_role`, `role`,
`role_permission`, `permission`. Location: L51–74. Leave `audit_log`
unqualified (stays `public`) — do not touch L107–114, it only references
`audit_log`.

**The 7 files outside `internal/core` (scope correction, see note above):**

**`internal/sessioncheck/sessioncheck.go`** — `auth.` for `user_session`,
`app_user`. Locations: L52–53 (`activeSessionQuery`), L66
(`touchSessionQuery`). This file is a deliberate duplicate of
auth-service's session-validation logic per its own doc comment — qualify
it the same way, independently of auth-service's copy.

**`internal/ingestion/service.go`** — `plant.` for
`device_model_register_metadata`. Location: L357
(`ensureModelRegisterMetadata`).

**`internal/ingestion/alarms.go`** — `alarm.` for `alarm_rule`,
`alarm_rule_condition`, `alarm_event`; `auth.` for `user_role`,
`app_user`. Locations: L51, L77, L133, L149, L211–212.

**`internal/ingestion/service_integration_test.go`** — `auth.` for
`middleware_client`, `app_user`, `user_role`; `plant.` for `plant`,
`device`; `telemetry.` for `telemetry_reading`. Locations: L44, L89, L97,
L108, L111, L114, L117, L121. This is the same file Task 2 already
modified (the `disposablePool` helper) — qualify these separate lines
without touching `disposablePool` again.

**`internal/ingestion/raw_service_integration_test.go`** — `auth.` for
`middleware_client`, `app_user`, `user_role`; `telemetry.` for
`raw_register_reading`; `plant.` for `plant`, `device`,
`device_model_register_metadata`. Locations: L36–37, L56, L63, L66, L71,
L74.

**`internal/ingestion/alarms_integration_test.go`** — `auth.` for
`middleware_client`; `plant.` for `plant`, `device`; `alarm.` for
`alarm_rule`, `alarm_rule_condition`, `alarm_event`. Locations: L32, L60,
L63, L68–69, L74–75, L82, L90, L117.

**`cmd/platform-admin/main.go`** — `auth.` for `middleware_client`,
`app_user`, `role`, `user_role`. Locations: L177–178, L232–234, L241,
L248–249. Leave the `'middleware_client'`/`'app_user'` string literals at
L186/L255 alone — those are `audit_log.target_type` *values*, not table
references.

- [ ] **Step 1b: Fix type-shape breakage from Task 4's sqlc regeneration
  (distinct from raw-SQL qualification above)**

Task 4 discovered and root-caused a second, unrelated category of
breakage: qualifying table names in the `.sql` query files changed what
`sqlc generate` emits, in two ways that don't involve any table name text
at all:

1. Pairs of queries whose result/param shapes used to be byte-identical
   (e.g. `GetUserDashboard` / `GetUserDashboardForUpdate`) used to get a
   single shared Go type via a `type X = Y` alias. Once the underlying
   tables carry an explicit schema qualifier, sqlc's struct-reuse
   heuristic no longer fires, so each query gets its own distinct,
   non-interchangeable struct — code written assuming interchangeability
   now fails to compile.
2. `dashboard.go`'s `ListDashboardPlantStatus` query aggregates
   `max(tl.observed_at)` over a JOIN that now spans 3 different schemas
   (`plant.plant`/`plant.device`/`telemetry.telemetry_latest`) — sqlc's
   nullability inference degrades to `interface{}` for this field once
   the join crosses schema boundaries (a known sqlc analyzer limitation
   with cross-schema joins), where it previously inferred a concrete
   `pgtype.Timestamptz`.

This is caught entirely by the Go compiler (unlike the raw-SQL
qualification work, which has no compiler safety net) — `go build` is the
authoritative check here, the same way `sqlc generate` was for Step 1.
As of this plan's writing, `go build ./...` in `services/platform-api`
reports these errors (re-run it yourself first — this list may have
shifted if other tasks landed since, use it as a starting reference, not
gospel):

```
internal\core\dashboard.go:66:32: cannot use row.LastObservedAt (variable of type interface{}) as pgtype.Timestamptz value in argument to timePointer: need type assertion
internal\core\dashboard_layout.go:296:51: cannot use dbgen.GetUserDashboardParams{…} as dbgen.GetUserDashboardForUpdateParams value in argument to q.GetUserDashboardForUpdate
internal\core\dashboard_layout.go:308:14: cannot use q.CreateUserDashboard(...) (dbgen.CreateUserDashboardRow) as dbgen.GetUserDashboardRow value in assignment
internal\core\dashboard_layout.go:317:14: cannot use q.UpdateUserDashboard(...) (dbgen.UpdateUserDashboardRow) as dbgen.GetUserDashboardRow value in assignment
internal\core\dashboard_layout.go:370:51: cannot use dbgen.GetUserDashboardParams{…} as dbgen.GetUserDashboardForUpdateParams value in argument to q.GetUserDashboardForUpdate
internal\core\dashboard_layout.go:377:49: cannot use current (dbgen.GetUserDashboardForUpdateRow) as dbgen.GetUserDashboardRow value in argument to publishedDashboardLayoutFromRow
internal\core\dashboard_layout.go:385:49: cannot use row (dbgen.PublishUserDashboardRow) as dbgen.GetUserDashboardRow value in argument to publishedDashboardLayoutFromRow
internal\core\telemetry.go:215:21: cannot use cursorID (pgtype.UUID) as pgtype.Timestamptz value in struct literal
internal\core\telemetry.go:226:42: cannot use row (dbgen.ListDeviceTelemetryHistoryRow) as dbgen.ListLatestPlantTelemetryRow value in argument to telemetryFromRow
```

Fix by reading each error and the actual (now-distinct) `dbgen` type
definitions it names, then adjusting the calling code in `dashboard.go`,
`dashboard_layout.go`, and `telemetry.go` to use the correct type — e.g.
call the right accessor for a `*ForUpdate`-shaped result instead of
assuming it matches the base query's shape, convert a positional struct
literal to named fields where a field's position shifted, add an
explicit type assertion (`row.LastObservedAt.(pgtype.Timestamptz)`,
checking the `ok` or handling the zero-value case) where nullability
inference degraded to `interface{}`. Re-run `go build ./...` until these
9 errors are gone. These 3 files are already in this task's checklist
above for their raw-SQL qualification — this is additional, distinct work
in the same files, not a new file to add.

**Do not touch `plant_image.go` or `plants.go`'s `ImageUrl`-related build
errors** (`go build` will also show ~7 errors there) — Task 4 confirmed
these are pre-existing, unrelated WIP (an in-progress plant-image-upload
feature; `plant_image.go` is untracked and was already uncompilable
before this plan touched anything, `dbgen` has never had an `ImageUrl`
field). Leave them exactly as found, same as every other pre-existing
pending-work file in this plan.

- [ ] **Step 2: Final sanity sweep**

After Step 1, search each of the 18 files above for any remaining bare
occurrence of the 25 table names (the ones in Task 4 Step 1's list) that
Step 1's checklist might have missed — the checklist was produced by a
careful but manual search and could have a gap. Any hit that isn't
already schema-qualified needs the same fix.

Given this task's file list already grew once from an initially-too-narrow
search (see the scope-correction note above — Task 3 hit the identical
problem in auth-service), don't stop at just these 18 files: grep the
*whole* `services/platform-api` tree (excluding
`internal/database/dbgen`, which is sqlc-generated) for the 25 table
names in `FROM`/`JOIN`/`INTO`/`UPDATE`/`DELETE FROM` position, the same
way the scope-correction search was done. If that finds a genuine hit in
a file not already listed above, qualify it too and add it to this task's
file list before moving on — don't let a third undiscovered file reach
the final whole-branch review.

- [ ] **Step 3: Full verification — both services, both directions**

```
cd services/platform-api
go build ./... && go vet ./... && go test ./...
```
Expected: fully green now, including the `internal/ingestion` integration
tests (Task 2's fix) and every `internal/core` test.

```
cd services/auth-service
go build ./... && go vet ./... && go test ./...
```
Expected: still green (unaffected by this task, re-confirming Task 3
didn't regress).

Fresh-database boot check (confirms migration 000033 applies cleanly from
empty, not just incrementally on an already-migrated database):
```
cd services/platform-api
go run ./cmd/platform-api
```
against a brand-new, empty Postgres database — expect clean startup, no
errors, `schema_migrations` ends up with all 33 versions recorded.

Manual: start `platform-api` (and `auth-service`) against a
fully-migrated database and walk through the web UI — log in, view/manage
plants and devices, view telemetry and SCADA screens, check alarms and
the dashboard. No behavior change is expected anywhere; this only proves
the schema move + qualification didn't silently break a code path the
automated tests don't cover.

- [ ] **Step 4: Commit**

```bash
git add services/platform-api/internal/core
git commit -m "feat(platform-api): schema-qualify raw SQL in internal/core for the schema move"
```

# Platform API

Central Solar SCADA API. This deployable is separate from `modbus-api-middleware`, which remains the Site/Edge component and compatibility-contract reference.

## Configuration

- `PLATFORM_DATABASE_URL` or `DATABASE_URL` (required): PostgreSQL connection URL; `schema=public` is normalized to PostgreSQL `search_path=public`
- `PLATFORM_HTTP_ADDR` (optional): listen address, default `127.0.0.1:44441`
- `PLATFORM_SESSION_IDLE_TIMEOUT` (optional): default `30m`
- `PLATFORM_SESSION_ABSOLUTE_TIMEOUT` (optional): default `24h`
- `PLATFORM_COOKIE_SECURE` (optional): default `true`; set `false` only for local HTTP development
- `PLATFORM_PASSWORD_RESET_TTL` (optional): one-time reset token lifetime, default `30m`
- `PLATFORM_SMTP_ADDR`, `PLATFORM_SMTP_FROM`, `PLATFORM_PASSWORD_RESET_URL` (optional as a group): existing organizational SMTP endpoint, sender, and web reset page URL
- `PLATFORM_SMTP_USERNAME`, `PLATFORM_SMTP_PASSWORD` (optional): SMTP credentials
- AUTH_SMTP_ADDR, AUTH_SMTP_FROM, AUTH_SMTP_USERNAME, AUTH_SMTP_PASSWORD and AUTH_PASSWORD_RESET_URL: SMTP and frontend URL used for registration email verification and password reset links
- `PLATFORM_WEBSOCKET_ORIGINS` (optional): comma-separated origin host patterns, default `localhost:8080,127.0.0.1:8080`
- PLATFORM_PLANT_IMAGE_DIR (optional): Plant image storage directory, default ./data/plant-images; PNG/JPEG/WebP uploads are limited to 2 MiB
- `PLATFORM_PUBLIC_BASE_URL` (required for Middleware Gateways -> Software Update -> Stage): the externally-reachable Gateway URL (e.g. `https://ygate-api.example.com` or `http://127.0.0.1:44440` locally) that a Middleware client downloads staged patch binaries from. Left unset, Stage fails immediately with an "invalid middleware gateway data" error even while the gateway shows Online, since Online only reflects the realtime websocket, not this separate download step.

## Run

```powershell
$env:PLATFORM_DATABASE_URL = "postgres://platform:<password>@127.0.0.1:5432/platform"
go run ./cmd/platform-api
```

The service applies embedded forward-only migrations before listening.

- Liveness: `GET http://127.0.0.1:44441/healthz`
- Readiness: `GET http://127.0.0.1:44441/readyz`
- Login: `POST http://127.0.0.1:44441/api/v1/auth/login`
- Current user: `GET http://127.0.0.1:44441/api/v1/auth/me`
- Logout: `POST http://127.0.0.1:44441/api/v1/auth/logout`
- Logout all: `POST http://127.0.0.1:44441/api/v1/auth/logout-all`
- Change password: `POST http://127.0.0.1:44441/api/v1/auth/change-password`
- Forgot password: `POST http://127.0.0.1:44441/api/v1/auth/forgot-password`
- Reset password: `POST http://127.0.0.1:44441/api/v1/auth/reset-password`
- Sessions: `GET http://127.0.0.1:44441/api/v1/auth/sessions`
- Self profile: `GET|PUT http://127.0.0.1:44441/api/v1/auth/profile`
- Clear owned session records: `DELETE http://127.0.0.1:44441/api/v1/auth/sessions` (also logs out the current session)
- Revoke session: `DELETE http://127.0.0.1:44441/api/v1/auth/sessions/{sessionId}`
- OpenAPI contract: `GET http://127.0.0.1:44441/api/v1/admin/openapi`
- Realtime WebSocket: `GET ws://127.0.0.1:44441/api/v1/realtime`
- List authorized plants: `GET http://127.0.0.1:44441/api/v1/plants`
- Create a plant: `POST http://127.0.0.1:44441/api/v1/plants`
- Read a plant: `GET http://127.0.0.1:44441/api/v1/plants/{plantId}`
- Update or disable a plant: `PUT http://127.0.0.1:44441/api/v1/plants/{plantId}`
- Upload/replace Plant image: POST /api/v1/plants/{plantId}/image
- Remove Plant image: DELETE /api/v1/plants/{plantId}/image
- List devices in a plant: `GET http://127.0.0.1:44441/api/v1/plants/{plantId}/devices`
- Update or disable a device: `PUT http://127.0.0.1:44441/api/v1/plants/{plantId}/devices/{deviceId}`
- System Admin hard delete: `DELETE` on a Plant, Device, Device Model, API Key or managed User path with the operation-specific `X-Hard-Delete-Confirm` value from OpenAPI
- Clear Audit view: `DELETE http://127.0.0.1:44441/api/v1/admin/audit`; immutable source rows remain and the global `audit.cleared` marker hides prior rows from subsequent list calls
- Ingest Middleware telemetry: `POST http://127.0.0.1:44441/api/v1/ingestion/telemetry`
- Middleware compatibility alias: `POST http://127.0.0.1:44441/api/middleware/readings`
- Ingest raw register readings v2: `POST http://127.0.0.1:44441/api/v2/ingestion/register-readings`

## Middleware contract

The authoritative contract is `packages/api-contracts/platform-api.yaml`; the
authenticated `/openapi` web page renders the same file and provides Try it
request forms populated from its examples.

- v1 accepts the deployed normalized `dataItemMap` payload without requiring a
  schema field. Do not add required v1 fields.
- v2 requires `schemaVersion: "2.0"` and decoded values keyed by
  `<registerAddress>` (value is the raw decoded number; see
  `docs/adr/0004-raw-register-ingestion-v2.md`).
- Send `X-Api-Key: <secret>` and reuse one `Idempotency-Key` for every retry of
  the same decompressed body. A reused key with a different body returns `409`.
- `Content-Encoding` may be omitted/`identity` or set to `gzip`; the
  decompressed body limit is 1 MiB and each batch contains at most 500 readings.
- Send an optional `X-Correlation-ID` using letters, digits, `.`, `_`, `:` or
  `-`. Every ingestion response returns the accepted or generated value in the
  same header.
- HTTP `202` can contain record-level rejections. Inspect `acceptedCount`,
  `duplicateCount`, `rejectedCount` and `errors`; do not treat HTTP status alone
  as proof that every record was stored.

CI tests compare every registered HTTP route with the OpenAPI operations and
fail when route or ingestion response field names drift.

SMTP is optional for local development. When it is not configured, forgot-password still returns the same generic response but does not issue a token; authorized administrators can use the audited reset-password endpoint from User Management.

State-changing authenticated requests send the `platform_csrf` cookie value in `X-CSRF-Token`. Migration `000003_session_security.sql` invalidates existing sessions because older rows have no CSRF secret.

## Migrate the database

`cmd/platform-api` already applies every embedded forward-only migration
before it starts listening, so this step is optional for local development.
For a deploy pipeline, run it explicitly as its own step — before starting or
restarting the service — so a schema change is applied and verified
independently of any particular instance's boot:

```powershell
$env:PLATFORM_DATABASE_URL = "postgres://platform:<password>@127.0.0.1:5432/platform"
go run ./cmd/platform-admin migrate
```

The target database must already exist; this only brings its schema up to
date. Migrations are tracked in `schema_migrations` and applied under a
Postgres advisory lock, so running this concurrently from multiple deploy
instances is safe — only one applies each pending migration.

## Bootstrap the first user

No default account or password is created. Set secrets only in the current process environment:

```powershell
$env:DATABASE_URL = "postgresql://postgres:<password>@localhost:5432/ygate_db?schema=public"
$env:PLATFORM_BOOTSTRAP_EMAIL = "admin@example.com"
$env:PLATFORM_BOOTSTRAP_USERNAME = "admin"
$env:PLATFORM_BOOTSTRAP_DISPLAY_NAME = "System Admin"
$env:PLATFORM_BOOTSTRAP_PASSWORD = "<12-to-72-byte-password>"
$env:PLATFORM_BOOTSTRAP_ORGANIZATION_CODE = "ORG-001"
$env:PLATFORM_BOOTSTRAP_ORGANIZATION_NAME = "Organization Name"
$env:PLATFORM_BOOTSTRAP_ROLE = "System Admin"
go run ./cmd/platform-admin bootstrap-user
```

The command hashes the password with bcrypt, creates the organization only when needed, assigns one seeded baseline role, rejects duplicate users, and writes an append-only audit event. `PLATFORM_BOOTSTRAP_ROLE` defaults to `System Admin`; supported values are `System Admin`, `Organization Admin`, `Plant Manager`, `Engineer`, `Operator`, `Viewer`, and `Auditor`.

## Bootstrap a Middleware client

```powershell
$env:PLATFORM_MIDDLEWARE_NAME = "Site Gateway"
$env:PLATFORM_MIDDLEWARE_ORGANIZATION_CODE = "YGATE"
$env:PLATFORM_MIDDLEWARE_AUTO_ONBOARD = "true"
go run ./cmd/platform-admin bootstrap-middleware
```

The API key is shown once. Only its SHA-256 hash and short prefix are stored. Set `auto_onboard` only for trusted Middleware clients; it permits unknown Plant and Device identities in valid readings to be registered automatically.

## Generate queries

```powershell
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```

## Verify

```powershell
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build ./...
```

Set `PLATFORM_TEST_DATABASE_URL` to an explicitly provided disposable PostgreSQL database to run migration, authentication, registry, and ingestion integration coverage. The Next.js web shell and Go API Gateway are separate deployables in this repository.

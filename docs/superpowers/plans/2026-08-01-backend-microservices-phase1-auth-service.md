# Backend Microservices Phase 1: Extract auth-service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract login/session/password-reset/users/roles/permissions/api-keys/profile into a new, independently-deployable `services/auth-service`, with `platform-api` reduced to its remaining domains and `api-gateway` routing the split traffic — proving the multi-service pattern (gateway routing, shared-database session validation, no cross-service network calls for auth checks) before any later phase extracts another domain.

**Architecture:** `auth-service` becomes a new, separate Go module (its own `go.mod`, own generated DB-query layer, own HTTP server) sharing the same PostgreSQL instance and migrations as `platform-api`. Because Go's `internal/` package visibility rule forbids importing `platform-api/internal/auth` from a different module, this plan does **not** try to share that package across the module boundary — it duplicates the small, stable, read-only "validate this session cookie" logic into a new lightweight package inside `platform-api` (`internal/sessioncheck`), while the full login/logout/password-reset/session-management business logic moves to `auth-service` as its new canonical home. This is a refinement of the design spec's "shared Go package" wording, discovered while writing this plan (see the design spec's own Global Constraints — this note documents the deviation and why).

**Tech Stack:** Go (both services), `sqlc` (already used by `platform-api`, same tool for `auth-service`'s own generated query layer), `pgx/v5`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-01-backend-microservices-split-design.md`, Phase 1 section — every task here traces to it, with one documented deviation (see Architecture above): "shared Go package" becomes "duplicated small read-path package + relocated full package," not a cross-module import, because Go's `internal/` visibility rules make the literal cross-module import impossible without moving `internal/auth` out of `internal/` entirely — duplicating the small piece is lower-risk for a Phase 1 proof-of-pattern than restructuring package visibility.
- One shared PostgreSQL instance — confirmed no new database, no schema split. `auth-service` gets its own generated `dbgen` package (via its own `sqlc.yaml`) pointed at the SAME shared `services/platform-api/internal/database/migrations` schema, not a copy of the schema.
- `platform-api`'s remaining routes (`/api/v1/plants`, `/api/v1/scada`, etc.) are completely unaffected in behavior — only their internal session-validation dependency changes from the full `auth.Service` to the new small `sessioncheck` package. `apps/web` needs zero changes (it only ever talks to the gateway's stable URL).
- `HasUserPermission`/`HasOrganizationPermission` (in `services/platform-api/internal/database/queries/core.sql`) are used by every domain, not just auth — this plan duplicates those two queries into `auth-service`'s own query set rather than trying to share `platform-api`'s generated `dbgen` package (same cross-module visibility problem as `internal/auth`). `platform-api` keeps its own copy unchanged.
- Both services must independently `go build ./... && go vet ./... && go test ./...` clean at the end of this plan.

---

### Task 1: Scaffold `auth-service` — module, config/envfile/database bootstrap, its own generated query layer

**Files:**
- Create: `services/auth-service/go.mod`
- Create: `services/auth-service/internal/auth/*.go` (copied from `services/platform-api/internal/auth/*.go`, unchanged)
- Create: `services/auth-service/internal/config/*.go` (copied from `services/platform-api/internal/config/*.go`)
- Create: `services/auth-service/internal/envfile/*.go` (copied from `services/platform-api/internal/envfile/*.go`)
- Create: `services/auth-service/internal/database/database.go` (copied from `services/platform-api/internal/database/database.go` — the connection-pool bootstrap only, not `dbgen`)
- Create: `services/auth-service/internal/database/queries/auth.sql` (moved from `services/platform-api/internal/database/queries/auth.sql` — deleted from platform-api in Task 2)
- Create: `services/auth-service/internal/database/queries/core.sql` (a duplicate containing only the `HasUserPermission`/`HasOrganizationPermission` queries from `services/platform-api/internal/database/queries/core.sql` — platform-api keeps its own full `core.sql` unchanged)
- Create: `services/auth-service/sqlc.yaml`
- Create: `services/platform-api/internal/sessioncheck/sessioncheck.go` (new, hand-written)

**Interfaces:**
- Produces: `auth-service`'s full `auth.Service` (all of `New`, `Authenticate`, `Login`, `Logout`, `ForgotPassword`, `ResetPassword`, `ChangePassword`, session list/revoke, `ConfigurePasswordRecovery`, `Principal`, `ErrUnauthenticated` — whatever the copied package already exports, unchanged) — consumed by Task 3's `auth-service` httpapi handlers.
- Produces: `sessioncheck.Principal{UserID, OrganizationID pgtype.UUID; ...}` and `sessioncheck.Authenticate(ctx context.Context, pool *pgxpool.Pool, sessionCookieValue string) (Principal, error)` and `sessioncheck.ErrUnauthenticated` and a `ValidCSRF` method matching `auth.Principal`'s — consumed by Task 3's rewiring of `platform-api`'s `authenticated()` middleware.

- [ ] **Step 1: Read the source files this task copies/duplicates, in full, before writing anything**

Read `services/platform-api/internal/auth/*.go` (all files in that directory), `services/platform-api/internal/config/config.go`, `services/platform-api/internal/envfile/envfile.go`, `services/platform-api/internal/database/database.go`, and `services/platform-api/internal/database/queries/auth.sql` + the `HasUserPermission`/`HasOrganizationPermission` queries in `core.sql`. These have not been read in this planning session — this step is required before Step 2, not optional context-gathering.

- [ ] **Step 2: Create the new Go module**

```bash
mkdir -p services/auth-service/cmd/auth-service
cd services/auth-service
go mod init ygate/auth-service
```

Copy the `require` block for whatever dependencies `internal/auth`, `internal/config`, `internal/envfile`, `internal/database` actually import from `services/platform-api/go.mod` (at minimum `github.com/jackc/pgx/v5` and its transitive requirements — read `services/platform-api/go.mod` to get exact versions, keep them identical to avoid two different pgx versions talking to the same database in ways that haven't been tested together).

- [ ] **Step 3: Copy `internal/auth`, `internal/config`, `internal/envfile`, `internal/database/database.go` verbatim**

Copy each file's content unchanged except the package's own internal import paths that reference `ygate/platform-api/internal/...` — update those to `ygate/auth-service/internal/...` (there should be few or none, since `internal/auth`/`internal/config`/`internal/envfile`/`internal/database` are already fairly self-contained utility packages in the source; confirm by grepping the copied files for `ygate/platform-api` after pasting, fixing any hits).

- [ ] **Step 4: Set up `auth-service`'s own generated query layer**

Create `services/auth-service/internal/database/queries/auth.sql` with the exact content of `services/platform-api/internal/database/queries/auth.sql` (this will be deleted from platform-api in Task 2 — for now both copies exist simultaneously so nothing breaks mid-refactor).

Create `services/auth-service/internal/database/queries/core.sql` containing only the two query blocks (`-- name: HasOrganizationPermission :one` and `-- name: HasUserPermission :one`, including their full SQL bodies) copied from `services/platform-api/internal/database/queries/core.sql` — platform-api's `core.sql` is untouched.

Create `services/auth-service/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "../platform-api/internal/database/migrations"
    queries: "internal/database/queries"
    gen:
      go:
        package: "dbgen"
        out: "internal/database/dbgen"
        sql_package: "pgx/v5"
```

(the `schema` path points at `platform-api`'s migrations directory via a relative path — the schema is shared, not duplicated, matching the design spec's "one shared PostgreSQL instance, one shared migration set" rule.)

Run:

```bash
cd services/auth-service
sqlc generate
```

Expected: `services/auth-service/internal/database/dbgen/` is created with generated Go code for the `auth.sql` and `core.sql` queries.

- [ ] **Step 5: Write the small `sessioncheck` package for `platform-api`**

Based on what Step 1 revealed about `auth.Principal`'s exact fields and `auth.Service.Authenticate`'s exact session-lookup query, write `services/platform-api/internal/sessioncheck/sessioncheck.go` as a minimal, read-only mirror: a `Principal` struct with the same fields `auth.Principal` has (at minimum `UserID`, `OrganizationID`, and whatever `ValidCSRF` compares against), a `var ErrUnauthenticated = errors.New(...)` matching `auth.ErrUnauthenticated`'s message, a `ValidCSRF(token string) bool` method with the same logic `auth.Principal.ValidCSRF` uses, and a package-level `Authenticate(ctx context.Context, pool *pgxpool.Pool, sessionCookieValue string) (Principal, error)` function that runs the same session-lookup SQL `auth.Service.Authenticate` runs (read-only: look up the session row by cookie value, check it hasn't expired/been revoked, load the owning user, build and return a `Principal`) directly via `pool.QueryRow` (no `dbgen` dependency needed for one query — plain SQL is fine here and avoids adding sessioncheck to platform-api's sqlc config for a single read).

- [ ] **Step 6: Verify**

Run: `cd services/auth-service && go build ./... && go vet ./...`
Expected: clean (nothing calls `sessioncheck` or wires up `auth-service`'s HTTP server yet — this task only proves the packages compile standalone).

Run: `cd services/platform-api && go build ./... && go vet ./...`
Expected: clean (the new `sessioncheck` package isn't imported by anything yet — this only proves it compiles in isolation).

- [ ] **Step 7: Commit**

```bash
git add services/auth-service services/platform-api/internal/sessioncheck
git commit -m "$(cat <<'EOF'
Scaffold auth-service module and platform-api's session-check package

auth-service gets its own copy of internal/auth (unchanged) plus its
own sqlc-generated query layer (auth.sql + a duplicated
HasUserPermission/HasOrganizationPermission from core.sql), schema-shared
with platform-api's migrations via a relative path -- not a copied
schema. platform-api gets a new small internal/sessioncheck package
covering just the read-only "validate this session cookie" path its
own authenticated() middleware needs once the full auth.Service moves
out; this is a documented deviation from the design spec's "shared Go
package" wording, made necessary by Go's internal/ package visibility
rules blocking a literal cross-module import.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Move `users`/`roles`/`api_keys`/`profile` domain logic into `auth-service`

**Files:**
- Move: `services/platform-api/internal/core/users.go` → `services/auth-service/internal/core/users.go`
- Move: `services/platform-api/internal/core/roles.go` → `services/auth-service/internal/core/roles.go`
- Move: `services/platform-api/internal/core/api_keys.go` → `services/auth-service/internal/core/api_keys.go`
- Move: `services/platform-api/internal/core/profile.go` → `services/auth-service/internal/core/profile.go`
- Create: `services/auth-service/internal/core/service.go`
- Delete: `services/platform-api/internal/database/queries/auth.sql` (now lives only in `auth-service`, per Task 1)
- Modify: whichever `services/platform-api/internal/core/*.go` files Step 1 finds still referencing the moved files' types/functions (most likely `hard_delete.go`, since it cascades user deletes)

**Interfaces:**
- Consumes: `auth-service`'s `internal/auth` (Task 1), `internal/database/dbgen` (Task 1).
- Produces: `auth-service`'s own `core.Service` type and `core.New(pool *pgxpool.Pool) *Service` — consumed by Task 3's `auth-service` httpapi handlers and Task 4's `auth-service` `main.go`.

- [ ] **Step 1: Read the four files being moved, and `hard_delete.go`, in full**

Read `services/platform-api/internal/core/users.go`, `roles.go`, `api_keys.go`, `profile.go`, and `services/platform-api/internal/core/hard_delete.go` in full. Identify: (a) every exported function/type these four files define that any OTHER file in `services/platform-api/internal/core/` or `internal/httpapi/` references, and (b) whether `hard_delete.go`'s cascading delete (which per its name and the design spec's own risk analysis almost certainly touches users) calls directly into `users.go`'s functions or just issues its own SQL against the `app_user` table. If it calls functions being moved, that cascade logic must be inlined into `hard_delete.go` itself (as direct SQL against the shared `app_user`/`user_role`/`session` tables platform-api can still read/write, since the database is shared) rather than calling a function that no longer exists in this module — report back with what you find before proceeding to Step 2 if the cascade is non-trivial (more than a straightforward `DELETE ... WHERE`).

- [ ] **Step 2: Move the four files**

```bash
git mv services/platform-api/internal/core/users.go services/auth-service/internal/core/users.go
git mv services/platform-api/internal/core/roles.go services/auth-service/internal/core/roles.go
git mv services/platform-api/internal/core/api_keys.go services/auth-service/internal/core/api_keys.go
git mv services/platform-api/internal/core/profile.go services/auth-service/internal/core/profile.go
git mv services/platform-api/internal/database/queries/auth.sql /dev/null 2>/dev/null || rm services/platform-api/internal/database/queries/auth.sql
```

(the `auth.sql` deletion has no `git mv` destination since it was already copied to `auth-service` in Task 1 — this step removes platform-api's now-unused original.)

- [ ] **Step 3: Give `auth-service`'s `core` package its own `Service` type**

Create `services/auth-service/internal/core/service.go`:

```go
package core

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"ygate/auth-service/internal/database/dbgen"
)

type Service struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, queries: dbgen.New(pool)}
}
```

(mirrors `services/platform-api/internal/core/plants.go`'s existing `Service`/`New` shape, minus the `hub *gatewayhub.Hub` field — `auth-service` never talks to the Middleware realtime hub.)

- [ ] **Step 4: Fix imports in the four moved files**

In each of `services/auth-service/internal/core/{users,roles,api_keys,profile}.go`, change every occurrence of:
- `"ygate/platform-api/internal/auth"` → `"ygate/auth-service/internal/auth"`
- `"ygate/platform-api/internal/database/dbgen"` → `"ygate/auth-service/internal/database/dbgen"`

If any of the four files reference helper functions defined elsewhere in platform-api's `core` package (e.g. `parseUUID`, `newUUID`, `uuidString`, error variables like `ErrForbidden`/`ErrNotFound`/`ErrInvalid`/`ErrConflict`) that are NOT specific to plants/devices/etc. but are generic utilities, copy those specific small helper functions into a new `services/auth-service/internal/core/helpers.go` (do not copy the whole file they came from) — check `services/platform-api/internal/core/plants.go`'s top (where `ErrForbidden`/`ErrNotFound`/`ErrInvalid`/`ErrConflict` and `plantCodeRE` are declared) and wherever `parseUUID`/`newUUID`/`uuidString` are defined, and copy only the ones the moved files actually call.

- [ ] **Step 5: Apply whatever `hard_delete.go` fix Step 1 identified**

Make the change Step 1's investigation determined is needed (inline SQL cascade, or confirm no change needed if the cascade was already independent of the moved functions).

- [ ] **Step 6: Verify both services build**

Run: `cd services/auth-service && go build ./... && go vet ./...`
Expected: clean.

Run: `cd services/platform-api && go build ./... && go vet ./... && go test ./...`
Expected: clean — this is the step that proves nothing still-in-platform-api references the moved code.

- [ ] **Step 7: Commit**

```bash
git add services/auth-service/internal/core services/platform-api/internal/core services/platform-api/internal/database/queries/auth.sql
git commit -m "$(cat <<'EOF'
Move users/roles/api-keys/profile domain logic to auth-service

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Move HTTP handlers and routes; rewire `platform-api`'s session check

**Files:**
- Move: `services/platform-api/internal/httpapi/users.go` → `services/auth-service/internal/httpapi/users.go`
- Move: `services/platform-api/internal/httpapi/roles.go` → `services/auth-service/internal/httpapi/roles.go`
- Move: `services/platform-api/internal/httpapi/admin_integrations.go` → `services/auth-service/internal/httpapi/admin_integrations.go`
- Move: `services/platform-api/internal/httpapi/profile.go` → `services/auth-service/internal/httpapi/profile.go`
- Create: `services/auth-service/internal/httpapi/auth.go` (login/logout/forgot-password/reset-password/change-password/session-list/session-revoke handlers, extracted from `services/platform-api/internal/httpapi/server.go`)
- Create: `services/auth-service/internal/httpapi/server.go` (new, minimal — mirrors `services/platform-api/internal/httpapi/server.go`'s shape: constructs the mux, registers this service's routes, wraps with the same CORS/security-header middleware if `server.go` defines its own rather than relying only on the gateway)
- Modify: `services/platform-api/internal/httpapi/server.go` (remove the moved route registrations and inline handlers; rewire `authenticated()` to use `sessioncheck` instead of the full `auth.Service`)

**Interfaces:**
- Consumes: `auth-service`'s `core.Service` (Task 2), `auth.Service` (Task 1), `sessioncheck.Authenticate`/`sessioncheck.Principal`/`sessioncheck.ErrUnauthenticated` (Task 1).
- Produces: `auth-service`'s `httpapi.New(...)` HTTP handler constructor — consumed by Task 4's `auth-service` `main.go`.

- [ ] **Step 1: Read `services/platform-api/internal/httpapi/server.go` in full**

This file has not been read in this planning session. Identify every route registration and inline handler function related to: login, logout, forgot-password, reset-password, change-password, session listing, session revocation, and the `authenticated()` middleware function itself (already partially known from this plan's Architecture section: it calls `service.Authenticate(ctx, cookie.Value)` and `principal.ValidCSRF(...)`). Note the exact line ranges to extract.

- [ ] **Step 2: Move the four handler files**

```bash
git mv services/platform-api/internal/httpapi/users.go services/auth-service/internal/httpapi/users.go
git mv services/platform-api/internal/httpapi/roles.go services/auth-service/internal/httpapi/roles.go
git mv services/platform-api/internal/httpapi/admin_integrations.go services/auth-service/internal/httpapi/admin_integrations.go
git mv services/platform-api/internal/httpapi/profile.go services/auth-service/internal/httpapi/profile.go
```

Fix their import paths the same way Task 2 Step 4 did (`ygate/platform-api/internal/auth` → `ygate/auth-service/internal/auth`, `ygate/platform-api/internal/core` → `ygate/auth-service/internal/core`), and copy any small shared httpapi helper functions they call (`decodeJSON`, `writeJSON`, `remoteIP`, error-mapping helpers like whatever `writeMiddlewareError` is patterned after for these domains) into a new `services/auth-service/internal/httpapi/helpers.go`, mirroring only what's actually used — check `services/platform-api/internal/httpapi/server.go` or a shared helpers file for their definitions first.

- [ ] **Step 3: Extract auth routes into `auth-service`**

Based on Step 1's findings, write `services/auth-service/internal/httpapi/auth.go` containing the login/logout/forgot-password/reset-password/change-password/session-list/session-revoke handler functions, adapted to call `auth-service`'s own `auth.Service` (Task 1) instead of the shared one platform-api used to have. Preserve their exact request/response JSON shapes and status codes byte-for-byte — `apps/web`'s `auth-screen.tsx`/`sessions-page.tsx`/`profile-page.tsx` call these endpoints unchanged and must keep working without any frontend edit.

- [ ] **Step 4: Write `auth-service`'s `server.go`**

Create `services/auth-service/internal/httpapi/server.go` mirroring `services/platform-api/internal/httpapi/server.go`'s overall shape (its `New(...)` constructor signature, its CORS/security-header wrapping if any is done at this layer rather than only in `api-gateway`, its own `authenticated()` middleware using the FULL `auth-service`-local `auth.Service.Authenticate` — this is a second, independent copy of the `authenticated()` function, not shared with platform-api's, since `auth-service` also has protected admin routes like `/admin/users`) with route registrations for everything moved in this task: `/api/v1/auth/login`, `/api/v1/auth/logout`, `/api/v1/auth/forgot-password`, `/api/v1/auth/reset-password`, `/api/v1/auth/change-password`, `/api/v1/auth/profile`, `/api/v1/auth/sessions`, `/api/v1/admin/users`, `/api/v1/admin/roles`, `/api/v1/admin/permissions`, `/api/v1/admin/api-keys` (exact path list confirmed against what Step 1 found in the original `server.go`).

- [ ] **Step 5: Rewire `platform-api`'s `authenticated()` to use `sessioncheck`**

In `services/platform-api/internal/httpapi/server.go`, change `authenticated`'s signature from `func authenticated(service *auth.Service, csrfRequired bool, next ...)` to `func authenticated(pool *pgxpool.Pool, csrfRequired bool, next ...)`, and inside it, replace `service.Authenticate(r.Context(), cookie.Value)` with `sessioncheck.Authenticate(r.Context(), pool, cookie.Value)`, and `errors.Is(err, auth.ErrUnauthenticated)` with `errors.Is(err, sessioncheck.ErrUnauthenticated)`. Every `authenticated(authService, ...)` call site in this file (all the remaining plants/devices/scada/alarms/dashboard/audit/middleware/telemetry/hard-delete routes) changes to `authenticated(pool, ...)`. Remove the `internal/auth` import from this file if nothing else in it still needs it; add `internal/sessioncheck`.

- [ ] **Step 6: Remove the moved route registrations from `platform-api`'s `server.go`**

Delete the route-registration lines and inline handler functions Step 1 identified as moved (login/logout/forgot-password/reset-password/change-password/session listing/revocation/users/roles/permissions/api-keys). Remove `httpapi.New(...)`'s now-unused `authService *auth.Service` parameter if the constructor took it directly (check `cmd/platform-api/main.go`'s call site — Task 4 will need the matching update there too, tracked in Task 4's own steps for that file).

- [ ] **Step 7: Verify**

Run: `cd services/auth-service && go build ./... && go vet ./...`
Expected: clean (still no `main.go` calling this yet — Task 4 adds that).

Run: `cd services/platform-api && go build ./... && go vet ./... && go test ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add services/auth-service/internal/httpapi services/platform-api/internal/httpapi
git commit -m "$(cat <<'EOF'
Move auth/users/roles/api-keys/profile HTTP handlers to auth-service

platform-api's authenticated() middleware now validates sessions via
the new internal/sessioncheck package instead of the full auth.Service
that moved out -- every other route's behavior is unchanged.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `auth-service` `main.go`, `platform-api` `main.go` update, `api-gateway` routing

**Files:**
- Create: `services/auth-service/cmd/auth-service/main.go`
- Modify: `services/platform-api/cmd/platform-api/main.go`
- Modify: `services/api-gateway/internal/gateway/gateway.go`
- Modify: `services/api-gateway/internal/config/config.go`
- Modify: `services/api-gateway/cmd/api-gateway/main.go`

**Interfaces:**
- Consumes: `auth-service`'s `auth.Service`/`core.Service`/`httpapi.New` (Tasks 1-3), `platform-api`'s updated `httpapi.New` signature (Task 3 Step 6).

- [ ] **Step 1: Read `services/platform-api/cmd/platform-api/main.go` in full (already shown in this plan's investigation, re-read to confirm still current) and `services/api-gateway/internal/gateway/gateway.go` + `internal/config/config.go` in full**

- [ ] **Step 2: Write `auth-service`'s `main.go`**

Create `services/auth-service/cmd/auth-service/main.go`, following `platform-api/cmd/platform-api/main.go`'s exact structure (`envfile.LoadDefault()`, `config.Load()`, signal-context setup, `database.Open`, service construction, HTTP server with graceful shutdown) but scoped to only what `auth-service` needs:

```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ygate/auth-service/internal/auth"
	"ygate/auth-service/internal/config"
	"ygate/auth-service/internal/core"
	"ygate/auth-service/internal/database"
	"ygate/auth-service/internal/envfile"
	"ygate/auth-service/internal/httpapi"
	"ygate/auth-service/internal/notification"
)

var version = "dev"

func main() {
	if err := envfile.LoadDefault(); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(ctx, 30*time.Second)
	pool, err := database.Open(startupCtx, cfg.DatabaseURL)
	cancelStartup()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	authService := auth.New(pool, cfg.SessionIdleTimeout, cfg.SessionAbsoluteTimeout)
	var resetNotifier auth.ResetNotifier
	if cfg.SMTPAddr != "" {
		notifier, notifierErr := notification.NewSMTPResetNotifier(
			cfg.SMTPAddr, cfg.SMTPFrom, cfg.SMTPUsername, cfg.SMTPPassword, cfg.PasswordResetURL,
		)
		if notifierErr != nil {
			log.Fatal(notifierErr)
		}
		resetNotifier = notifier.Notify
	}
	authService.ConfigurePasswordRecovery(cfg.PasswordResetTTL, resetNotifier)
	registryService := core.New(pool)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.New(version, pool.Ping, authService, registryService, cfg.CookieSecure, cfg.AllowedOrigins...),
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("auth-service %s listening on %s", version, cfg.ListenAddr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
```

This needs its own `internal/notification` package too (for the SMTP password-reset email, same as platform-api's) — copy `services/platform-api/internal/notification/*.go` to `services/auth-service/internal/notification/*.go` unchanged (fix any `ygate/platform-api` import path references, matching the Task 1/2 pattern). Add this as an amendment to Task 1's file list if not already copied there — check whether Task 1 needs revisiting before this step, since `ResetNotifier`/`NewSMTPResetNotifier` are referenced here.

Reconcile `httpapi.New(...)`'s exact parameter list here against whatever Task 3 Step 4 actually wrote for `auth-service/internal/httpapi/server.go`'s `New` constructor — adjust either this call site or that constructor so they match exactly (this plan specifies both should exist; whichever is written second in execution order reconciles with the first).

- [ ] **Step 3: Update `platform-api`'s `main.go`**

In `services/platform-api/cmd/platform-api/main.go`: remove the `authService := auth.New(...)`, `resetNotifier`/SMTP wiring, and `authService.ConfigurePasswordRecovery(...)` block entirely (that's all moved to `auth-service` now) — `platform-api` no longer needs `internal/auth` or `internal/notification`'s SMTP reset-notifier wiring in `main.go`. Update the `httpapi.New(...)` call to match Task 3 Step 6's updated signature (passing `pool` where it used to pass `authService`, per the `authenticated()` rewiring). Remove the now-unused `"ygate/platform-api/internal/auth"` and `"ygate/platform-api/internal/notification"` imports if nothing else in this file needs them (check whether `notification` is used for anything besides the password-reset SMTP notifier before removing its import — if platform-api sends any OTHER kind of notification, keep the import and package).

- [ ] **Step 4: Add `AuthServiceURL` to `api-gateway`'s config**

In `services/api-gateway/internal/config/config.go`, add a new field (mirroring however `platform-api`'s upstream URL is currently configured there) for the auth-service upstream, e.g. `AuthServiceURL *url.URL`, loaded from a new environment variable (e.g. `AUTH_SERVICE_URL`), following the exact pattern the existing platform-api URL config field already uses in this file.

- [ ] **Step 5: Make `gateway.New` route by path prefix**

In `services/api-gateway/internal/gateway/gateway.go`, change `New(platformURL *url.URL, allowedOrigins []string) http.Handler` to also accept `authServiceURL *url.URL`. Replace the single `proxy := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) { request.SetURL(platformURL); ... }}` with two reverse proxies (one per upstream) and a small routing `http.HandlerFunc` that picks between them by path prefix before delegating:

```go
func New(platformURL, authServiceURL *url.URL, allowedOrigins []string) http.Handler {
	platformProxy := newProxy(platformURL)
	authProxy := newProxy(authServiceURL)
	authPrefixes := []string{"/api/v1/auth/", "/api/v1/admin/users", "/api/v1/admin/roles", "/api/v1/admin/permissions", "/api/v1/admin/api-keys"}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /gateway/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range authPrefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				authProxy.ServeHTTP(w, r)
				return
			}
		}
		platformProxy.ServeHTTP(w, r)
	}))
	return middleware(mux, allowedOrigins)
}

func newProxy(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("proxy to %s failed: %v", target, err)
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
	}
}
```

(add `"strings"` to the import block.) `/api/v1/admin/permissions` is included in `authPrefixes` because permission listing is part of the roles/users admin surface that moved.

- [ ] **Step 6: Update `api-gateway`'s `main.go` call site**

In `services/api-gateway/cmd/api-gateway/main.go`, update the `gateway.New(...)` call to pass both URLs, reading `AuthServiceURL` from the config the same way the existing platform URL is read.

- [ ] **Step 7: Verify**

Run for each of `services/auth-service`, `services/platform-api`, `services/api-gateway`:
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: clean in all three.

- [ ] **Step 8: Manual end-to-end check**

Start Postgres, then (in separate terminals) `go run ./cmd/auth-service` (from `services/auth-service`), `go run ./cmd/platform-api` (from `services/platform-api`), `go run ./cmd/api-gateway` (from `services/api-gateway`, with `AUTH_SERVICE_URL` pointed at the auth-service port), then `npm run dev` (from `apps/web`). Confirm: log in through the web UI (routes to auth-service via the gateway), then load Plants (routes to platform-api via the gateway) — both work through the same gateway URL the frontend always talks to, with zero frontend changes.

- [ ] **Step 9: Commit**

```bash
git add services/auth-service/cmd services/platform-api/cmd services/api-gateway
git commit -m "$(cat <<'EOF'
Wire up auth-service's main.go and api-gateway's multi-target routing

api-gateway now routes /api/v1/auth/*, /api/v1/admin/users,
/api/v1/admin/roles, /api/v1/admin/permissions, and
/api/v1/admin/api-keys to auth-service; everything else still goes to
platform-api, unchanged.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: `run-all.bat`

**Files:**
- Create: `run-all.bat` (repo root)

**Interfaces:** none — this is a standalone dev-convenience script.

- [ ] **Step 1: Create the script**

```bat
@echo off
setlocal EnableExtensions
cd /d "%~dp0"

start "auth-service"          cmd /k "cd services\auth-service && go run ./cmd/auth-service"
start "api-gateway"           cmd /k "cd services\api-gateway && go run ./cmd/api-gateway"
start "platform-api"          cmd /k "cd services\platform-api && go run ./cmd/platform-api"
start "modbus-api-middleware" cmd /k "cd modbus-api-middleware && go run ./cmd/middleware"
start "web"                   cmd /k "cd apps\web && npm run dev"

echo All services starting in separate windows.
```

- [ ] **Step 2: Verify**

Run `run-all.bat` from the repo root — confirm 5 separate terminal windows open, each showing that service's own log output, with no window immediately crashing (a crash means a config/env var problem, not a script problem — check that each service's expected env vars, e.g. `DATABASE_URL`/`AUTH_SERVICE_URL`, are set in whatever `.env`/`envfile` each service loads via `envfile.LoadDefault()`).

- [ ] **Step 3: Commit**

```bash
git add run-all.bat
git commit -m "$(cat <<'EOF'
Add run-all.bat for local multi-service testing

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- `auth-service` as a new Go module, own domain (auth/users/roles/api-keys/profile) — Tasks 1-3. ✓
- Shared PostgreSQL, shared migrations, no schema split — Task 1 Step 4 (`sqlc.yaml`'s relative `schema` path), confirmed throughout. ✓
- Session validation without a network round-trip to auth-service — Task 1 Step 5 / Task 3 Step 5 (`sessioncheck`), **with a documented deviation from the spec's literal "shared Go package" wording**, explained in this plan's Architecture section and Global Constraints, because Go's `internal/` visibility rules block a literal cross-module import — duplication of the small read-path achieves the same request-time behavior (no network call) without that blocker.
- `api-gateway` multi-target routing scoped to auth-service's routes only for Phase 1 — Task 4 Steps 4-6. ✓
- `run-all.bat` gains the new service — Task 5. ✓
- `apps/web` needs zero changes — confirmed nowhere in this plan touches `apps/web`; Task 3 Step 3 explicitly calls out preserving exact request/response shapes so this holds.
- No change to any other domain's behavior — confirmed via the "go build/vet/test clean in platform-api" verification step present in every task that touches it.

**Placeholder scan:** the phrase "report back with what you find" in Task 2 Step 1 and "Reconcile ... adjust either this call site or that constructor" in Task 4 Step 2 are the two places this plan asks an implementer to resolve something based on investigation rather than handing over pre-written code. These are flagged deliberately, not vague filler: both concern code this planning session did not read in full (`hard_delete.go`'s exact cascade logic; `auth-service/internal/httpapi/server.go`'s exact `New` signature, which Task 3 itself defines earlier in execution — the two tasks' write order makes one of them the source of truth the other must match, an ordering constraint stated explicitly, not an unresolved question). Every other step in this plan contains literal code or an exact command.

**Type consistency:** `sessioncheck.Principal`/`sessioncheck.Authenticate`/`sessioncheck.ErrUnauthenticated` (Task 1 Step 5) are referenced with the same names in Task 3 Step 5's rewiring of `authenticated()`. `core.New(pool *pgxpool.Pool) *Service` (Task 2 Step 3) is called identically in Task 4 Step 2's `main.go`. `auth.New(pool, cfg.SessionIdleTimeout, cfg.SessionAbsoluteTimeout)` and `authService.ConfigurePasswordRecovery(cfg.PasswordResetTTL, resetNotifier)` in Task 4 Step 2 match the exact signatures already proven to exist (they're copied unchanged from `platform-api/cmd/platform-api/main.go`, read in this planning session).

**Scope check:** this plan is Phase 1 only (auth-service). Phases 2+ (plant-service, telemetry-service, middleware-gateway-service, scada-service, alarm-service, dashboard-service, notification-service) are explicitly out of scope, per the design spec's own decomposition — each needs its own spec before its own plan, not folded into this one.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

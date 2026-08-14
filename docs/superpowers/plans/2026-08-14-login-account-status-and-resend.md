# Login Account Status, Verification Resend, and User Hard-Delete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Return safe, actionable login states after a correct password, support generic resend-by-email-or-username, and regression-protect System Admin hard deletion of another account.

**Architecture:** Add `PENDING_ACCESS` as a durable account state. Authentication verifies the password before revealing `EMAIL_UNVERIFIED`, `ACCESS_PENDING`, or `ACCOUNT_DISABLED`; unknown users and wrong passwords remain indistinguishable. Resend keeps a generic `202`. Existing user hard-delete is retained and tested rather than duplicated.

**Tech Stack:** Go 1.24, PostgreSQL/pgx/sqlc, net/http, Next.js/React/TypeScript, Node test runner, OpenAPI YAML.

**Spec:** `docs/superpowers/specs/2026-08-14-register-profiles-alarm-decoding-login-status-design.md`

## Global Constraints

- Never reveal account state until the supplied password has verified.
- Keep unknown identifier and wrong password on the current generic `401` path.
- Registration and resend responses remain enumeration-safe.
- Do not weaken the existing hard-delete safeguards: no self-delete, no deleting the last active System Admin, global `user:hard_delete` permission, explicit confirmation, audit before deletion.

---

### Task 1: Persist the pending-access state

**Files:**
- Create: `services/platform-api/internal/database/migrations/000049_pending_access_status.sql`
- Modify: `services/platform-api/internal/database/database_test.go`
- Modify: `services/auth-service/internal/auth/registration.go`
- Test: `services/auth-service/internal/auth/registration_test.go`

**Interfaces:**

```sql
-- New registration state
status IN ('ACTIVE', 'PENDING_ACCESS', 'DISABLED')
```

- [ ] Add a failing migration catalog test expecting `000049_pending_access_status.sql` and the fragment `PENDING_ACCESS`.
- [ ] Run `go test ./internal/database -run TestMigrationFiles -count=1` from `services/platform-api`; expect failure because migration 49 is absent.
- [ ] Add a forward migration that replaces the status constraint, changes unassigned `DISABLED` users to `PENDING_ACCESS`, and leaves provisioned disabled users unchanged.
- [ ] Change `Register` to insert `PENDING_ACCESS`; add a focused registration test asserting the insert state.
- [ ] Run `go test ./internal/database -count=1` in platform-api and `go test ./internal/auth -run Registration -count=1` in auth-service; expect pass.
- [ ] Commit: `git add services/platform-api/internal/database services/auth-service/internal/auth && git commit -m "feat: distinguish pending access accounts"`.

### Task 2: Classify login only after password verification

**Files:**
- Modify: `services/auth-service/internal/database/queries/auth.sql`
- Regenerate: `services/auth-service/internal/database/dbgen/auth.sql.go`
- Modify: `services/auth-service/internal/auth/service.go`
- Test: `services/auth-service/internal/auth/service_integration_test.go`

**Interfaces:**

```go
var (
    ErrEmailUnverified = errors.New("email verification required")
    ErrAccessPending   = errors.New("access approval pending")
    ErrAccountDisabled = errors.New("account disabled")
)

type LoginUser struct {
    // existing fields
    EmailVerifiedAt *time.Time
}
```

- [ ] Add integration cases for: correct password + missing verification, correct password + `PENDING_ACCESS`, correct password + `DISABLED`, and wrong password for each state.
- [ ] Assert the first three return their specific sentinel and every wrong-password case returns only `ErrInvalidCredentials`.
- [ ] Run `go test ./internal/auth -run AuthenticationLifecycle -count=1`; expect failures for missing state classification.
- [ ] Select `email_verified_at` in `GetLoginUser`, regenerate with `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate`, and move status/verification checks after `VerifyPassword` succeeds.
- [ ] Preserve lockout/rate-limit behavior without exposing whether an identifier exists.
- [ ] Run `go test ./internal/auth -count=1`; expect pass.
- [ ] Commit: `git add services/auth-service/internal && git commit -m "feat: return safe login account states"`.

### Task 3: Return structured login errors and generic resend

**Files:**
- Modify: `services/auth-service/internal/httpapi/auth.go`
- Test: `services/auth-service/internal/httpapi/auth_test.go`
- Modify: `services/auth-service/internal/auth/registration.go`
- Modify: `services/auth-service/internal/database/queries/auth.sql`
- Regenerate: `services/auth-service/internal/database/dbgen/auth.sql.go`
- Test: `services/auth-service/internal/auth/service_integration_test.go`

**Interfaces:**

```json
{"code":"EMAIL_UNVERIFIED","message":"Please verify your email before signing in."}
{"code":"ACCESS_PENDING","message":"Your account is waiting for administrator approval."}
{"code":"ACCOUNT_DISABLED","message":"Your account has been disabled by an administrator."}
```

```json
POST /api/v1/auth/resend-verification
{"identifier":"email@example.com-or-username"}
```

- [ ] Add handler tests asserting JSON content type, stable codes, and `403` for the three correct-password account states; retain generic `401` for invalid credentials.
- [ ] Add resend tests asserting both email and username are accepted and known/unknown/already-verified identifiers all return the same empty `202` response.
- [ ] Run `go test ./internal/httpapi -run 'Login|Resend' -count=1`; expect failure.
- [ ] Add a shared JSON auth-error writer, change resend request from `email` to `identifier`, resolve identifier by normalized email or username, and reuse the existing operation limiter for resend attempts.
- [ ] Do not send a token for unknown, already verified, or non-pending accounts.
- [ ] Regenerate sqlc, then run `go test ./internal/auth ./internal/httpapi -count=1`; expect pass.
- [ ] Commit: `git add services/auth-service/internal && git commit -m "feat: expose login guidance and safe verification resend"`.

### Task 4: Complete the approval transition

**Files:**
- Modify: `services/auth-service/internal/core/users.go`
- Test: `services/auth-service/internal/core/users_test.go`
- Test: `services/auth-service/internal/core/users_integration_test.go` if introduced for database-backed transitions
- Modify: `apps/web/app/lib/types.ts`
- Modify: `packages/api-contracts/platform-api.yaml`

**Interfaces:**

```ts
type ManagedUserStatus = "ACTIVE" | "PENDING_ACCESS" | "DISABLED";
```

- [ ] Add tests proving assignment of organization/role plus activation changes `PENDING_ACCESS` to `ACTIVE`, disabling an approved user sets `DISABLED`, and verifying email alone does not activate access.
- [ ] Run the focused core test; expect enum/transition failures.
- [ ] Update validation and managed-user serialization to accept `PENDING_ACCESS`; keep activation as an explicit Admin action.
- [ ] Update OpenAPI status enums and descriptions.
- [ ] Run `go test ./internal/core -count=1` in auth-service and the repository OpenAPI contract test; expect pass.
- [ ] Commit: `git add services/auth-service apps/web/app/lib/types.ts packages/api-contracts/platform-api.yaml && git commit -m "feat: complete pending user approval flow"`.

### Task 5: Add actionable Login UI

**Files:**
- Modify: `apps/web/app/components/auth-screen.tsx`
- Test: `apps/web/app/components/auth-screen.test.tsx`
- Modify: `apps/web/app/lib/types.ts`

**Behavior:**

- `EMAIL_UNVERIFIED`: explanation plus “Resend verification email”.
- `ACCESS_PENDING`: explanation that Admin approval is required.
- `ACCOUNT_DISABLED`: explanation to contact the administrator.
- `401`: existing generic credential message.

- [ ] Add component tests for all four responses and for resend using the entered username or email.
- [ ] Run `npm test -- auth-screen.test.tsx`; expect failure.
- [ ] Parse the structured error body, render only the matching call to action, disable resend while pending, and show the same neutral success notice after `202`.
- [ ] Run `npm test -- auth-screen.test.tsx` and `npm run typecheck`; expect pass.
- [ ] Commit: `git add apps/web/app/components apps/web/app/lib/types.ts && git commit -m "feat: guide pending users from login"`.

### Task 6: Regression-protect existing System Admin user hard-delete

**Files:**
- Modify: `services/auth-service/internal/core/users_test.go`
- Test/Create if DB harness is needed: `services/auth-service/internal/core/users_integration_test.go`
- Modify: `services/auth-service/internal/httpapi/users_test.go`
- Verify only: `services/auth-service/internal/core/users.go`
- Verify only: `services/auth-service/internal/httpapi/users.go`
- Verify only: `apps/web/app/features/users/users-page.tsx`

- [ ] Add tests proving a System Admin can hard-delete another non-final account with confirmation and audit, while self-delete, insufficient permission, and final-active-System-Admin deletion are rejected.
- [ ] Add a handler test for `DELETE /api/v1/admin/users/{userId}` and `X-Hard-Delete-Confirm`.
- [ ] Run focused tests; if they already pass, make no production changes. If a guard is missing, add only the minimal correction in the existing `HardDeleteUser` path.
- [ ] Run `go test ./internal/core ./internal/httpapi -count=1` in auth-service.
- [ ] Commit only if tests or fixes changed: `git commit -m "test: protect system admin user hard delete"`.

### Task 7: Verify the login slice

- [ ] Run `go test ./...` in `services/auth-service`.
- [ ] Run `go test ./...` in `services/platform-api`.
- [ ] Run `npm test` and `npm run typecheck` in `apps/web`.
- [ ] Inspect `git diff --check` and confirm no login response leaks state before password verification.


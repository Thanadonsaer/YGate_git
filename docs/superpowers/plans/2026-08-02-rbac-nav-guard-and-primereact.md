# RBAC Nav/Route Guard and PrimeReact Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose each user's RBAC permission set from auth-service, use it in `apps/web` to hide sidebar items and block direct navigation the user isn't allowed to reach, and replace every shadcn/Radix UI primitive (plus hand-rolled buttons) with PrimeReact so the whole app — including the already-working Site Branding page — looks and behaves consistently.

**Architecture:** Backend: one new read-only SQL query (`ListUserPermissions`) in `services/auth-service`, surfaced as a `Permissions []string` field on the existing `LoginUser` type returned by `/api/v1/auth/login` and `/api/v1/auth/me`. No existing authorization check changes. Frontend: a `requires?: string` field added to the existing nav table in `platform-shell.tsx` drives both list filtering and a redirect-on-mismatch guard; separately, every `app/components/ui/*.tsx` wrapper is rewritten to render PrimeReact internally while keeping its current exported component names and props, so far fewer files (only the ~13 that use hand-rolled button classes, plus a couple of true naming inconsistencies) need call-site edits.

**Tech Stack:** Go 1.x (`ygate/auth-service` module), pgx/v5, sqlc v1.29.0, PostgreSQL. Next.js 16.2.11, React 19.2.8, Tailwind CSS v4, PrimeReact ^11.0.0 (requires React >=19, confirmed compatible), primeicons ^8.0.0.

## Global Constraints

- No change to any existing `requireGlobalPermission` / `requireOrganizationPermission` / `HasUserPermission` / `HasOrganizationPermission` call anywhere in `platform-api` or `auth-service` — those remain the real authorization boundary. This plan only adds a read-only permission-list projection.
- Nav/route gating in `apps/web` is page-level only (does the user have `resource_type:action` anywhere in their scope), not plant-level, and is advisory UX, not a security boundary.
- The 7 existing system roles' default permission grants are unchanged.
- `apps/web` has no component test framework today — frontend verification is manual (dev server + browser), consistent with existing project practice.
- `go build ./...`, `go vet ./...`, `gofmt -l` (no output), and `go test ./...` must stay clean in both `services/auth-service` and `services/platform-api` after every backend task.
- `npx tsc --noEmit` (run from `apps/web`) must stay clean after every frontend task.
- Regenerate sqlc code with `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate`, run from `services/auth-service`.
- PrimeReact components run in unstyled mode (`unstyled` set globally on `PrimeReactProvider`) — the existing navy/teal CSS custom properties in `app/globals.css` (`--brand`, `--ink`, `--surface`, `--line`, `--radius-sm/md/lg`, `--shadow-sm/lg`, etc.) remain the single source of visual truth. No PrimeReact prebuilt theme CSS is imported.
- Existing table/row/section/form-message CSS (`.plant-row`, `.device-row`, `.section-heading`, `.form-message`, etc.) is untouched by this plan.

---

## Part 1 — Backend: expose the caller's permissions

### Task 1: Add `ListUserPermissions` query and `Service.Permissions` method

**Files:**
- Modify: `services/auth-service/internal/database/queries/core.sql`
- Modify (generated): `services/auth-service/internal/database/dbgen/core.sql.go` (via sqlc, not hand-edited)
- Modify: `services/auth-service/internal/auth/service.go`
- Test: `services/auth-service/internal/auth/service_integration_test.go`

**Interfaces:**
- Produces: `func (s *Service) Permissions(ctx context.Context, userID pgtype.UUID) ([]string, error)` on `auth.Service`, returning strings formatted `"resource_type:action"`, e.g. `"plant:read"`.

- [ ] **Step 1: Add the SQL query**

Append to `services/auth-service/internal/database/queries/core.sql` (same file as `HasOrganizationPermission`/`HasUserPermission`, same join shape):

```sql
-- name: ListUserPermissions :many
SELECT DISTINCT pm.resource_type, pm.action
FROM user_role ur
JOIN role_permission rp ON rp.role_id = ur.role_id
    AND (rp.organization_id IS NULL OR rp.organization_id = ur.organization_id)
JOIN permission pm ON pm.id = rp.permission_id
WHERE ur.user_id = sqlc.arg(user_id);
```

- [ ] **Step 2: Regenerate sqlc code**

Run from `services/auth-service`:

```powershell
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate
```

Confirm `internal/database/dbgen/core.sql.go` now has `ListUserPermissions(ctx context.Context, userID pgtype.UUID) ([]ListUserPermissionsRow, error)` and `type ListUserPermissionsRow struct { ResourceType string; Action string }`.

- [ ] **Step 3: Write the failing integration test**

Append to `services/auth-service/internal/auth/service_integration_test.go`:

```go
func TestPermissionsReflectsRoleGrantsAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	organizationID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	userID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	suffix := uuidString(userID)
	if _, err = pool.Exec(ctx, "INSERT INTO organization(id,code,name) VALUES($1,$2,$3)", organizationID, "PERM-"+suffix, "Permissions Integration"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO app_user(id,organization_id,email,username,display_name,password_hash)
		VALUES($1,$2,$3,$4,$5,'unused')`, userID, organizationID, "perm-"+suffix+"@example.com", "perm-"+suffix, "Permissions Viewer"); err != nil {
		t.Fatal(err)
	}
	// 00000000-0000-4000-8000-000000000206 is the seeded "Viewer" system role
	// (read-only across organization/plant/asset_group/device_model/device),
	// assigned scoped to this test's organization.
	viewerRoleID, err := parseUUID("00000000-0000-4000-8000-000000000206")
	if err != nil {
		t.Fatal(err)
	}
	assignmentID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_role(id,organization_id,user_id,role_id) VALUES($1,$2,$3,$4)`,
		assignmentID, organizationID, userID, viewerRoleID); err != nil {
		t.Fatal(err)
	}

	service := New(pool, 30*time.Minute, 24*time.Hour)
	permissions, err := service.Permissions(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	byValue := map[string]bool{}
	for _, permission := range permissions {
		byValue[permission] = true
	}
	if !byValue["plant:read"] || !byValue["device:read"] {
		t.Fatalf("expected read grants missing, got %v", permissions)
	}
	if byValue["user:create"] || byValue["plant:create"] {
		t.Fatalf("viewer role must not carry write grants, got %v", permissions)
	}
}
```

- [ ] **Step 4: Run it to confirm it fails**

```powershell
$env:PLATFORM_TEST_DATABASE_URL = "<your disposable test database URL>"
go test ./internal/auth/... -run TestPermissionsReflectsRoleGrantsAgainstPostgreSQL -v
```

Expected: FAIL — `service.Permissions undefined`.

- [ ] **Step 5: Implement `Service.Permissions`**

In `services/auth-service/internal/auth/service.go`, add (near `Authenticate`, after the `Principal`/`LoginUser` type block):

```go
func (s *Service) Permissions(ctx context.Context, userID pgtype.UUID) ([]string, error) {
	rows, err := s.queries.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user permissions: %w", err)
	}
	permissions := make([]string, len(rows))
	for i, row := range rows {
		permissions[i] = row.ResourceType + ":" + row.Action
	}
	return permissions, nil
}
```

- [ ] **Step 6: Run it to confirm it passes**

```powershell
go test ./internal/auth/... -run TestPermissionsReflectsRoleGrantsAgainstPostgreSQL -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add services/auth-service/internal/database/queries/core.sql services/auth-service/internal/database/dbgen/core.sql.go services/auth-service/internal/auth/service.go services/auth-service/internal/auth/service_integration_test.go
git commit -m "feat(auth-service): add Service.Permissions for a user's RBAC grant set"
```

---

### Task 2: Surface `Permissions` on `LoginUser` via login and `/me`

**Files:**
- Modify: `services/auth-service/internal/auth/service.go`
- Modify: `services/auth-service/internal/httpapi/auth.go`
- Modify: `services/auth-service/internal/httpapi/server.go:56` (the `/api/v1/auth/me` route registration)
- Test: `services/auth-service/internal/httpapi/auth_test.go`
- Test: `services/auth-service/internal/auth/service_integration_test.go` (extend `TestAuthenticationLifecycleAgainstPostgreSQL`)

**Interfaces:**
- Consumes: `Service.Permissions(ctx, userID) ([]string, error)` from Task 1.
- Produces: `LoginUser.Permissions []string` (`json:"permissions"`); `meHandler(permissions PermissionsFunc)` where `type PermissionsFunc func(context.Context, pgtype.UUID) ([]string, error)`.

- [ ] **Step 1: Write the failing unit test for `meHandler`**

Append to `services/auth-service/internal/httpapi/auth_test.go` (add `"github.com/jackc/pgx/v5/pgtype"` to imports):

```go
func TestMeHandlerAttachesPermissions(t *testing.T) {
	var userID pgtype.UUID
	_ = userID.Scan("10000000-0000-4000-8000-000000000099")
	principal := auth.Principal{UserID: userID, Email: "operator@example.com", DisplayName: "Operator"}
	permissions := func(_ context.Context, id pgtype.UUID) ([]string, error) {
		if id != userID {
			t.Fatalf("userID = %v", id)
		}
		return []string{"plant:read", "alarm:read"}, nil
	}
	res := httptest.NewRecorder()
	meHandler(permissions)(res, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), principal)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"permissions":["plant:read","alarm:read"]`) || !strings.Contains(body, `"email":"operator@example.com"`) {
		t.Fatalf("body=%s", body)
	}
}
```

`meHandler(permissions)` returns `func(http.ResponseWriter, *http.Request, auth.Principal)` — call that returned function directly with `(res, request, principal)`, as shown above.

- [ ] **Step 2: Run it to confirm it fails**

```powershell
go test ./internal/httpapi/... -run TestMeHandlerAttachesPermissions -v
```

Expected: FAIL — `meHandler` takes no arguments / `LoginUser` has no field `Permissions`.

- [ ] **Step 3: Add `Permissions` to `LoginUser` and populate it at login**

In `services/auth-service/internal/auth/service.go`:

```go
type LoginUser struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId,omitempty"`
	Email          string   `json:"email"`
	DisplayName    string   `json:"displayName"`
	Permissions    []string `json:"permissions"`
}
```

In `recordSuccess`, before building the returned `LoginResult` (after the `tx.Commit(ctx)` call succeeds), fetch permissions and attach them:

```go
	if err = tx.Commit(ctx); err != nil {
		return LoginResult{}, fmt.Errorf("commit login: %w", err)
	}
	permissions, err := s.Permissions(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt,
		User: LoginUser{
			ID: uuidString(user.ID), OrganizationID: uuidString(user.OrganizationID), Email: user.Email, DisplayName: user.DisplayName,
			Permissions: permissions,
		},
	}, nil
```

- [ ] **Step 4: Update `meHandler` to attach permissions**

In `services/auth-service/internal/httpapi/auth.go`, add the import `"github.com/jackc/pgx/v5/pgtype"`, then replace:

```go
func meHandler() func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, _ *http.Request, principal auth.Principal) {
		writeJSON(w, http.StatusOK, principal.User())
	}
}
```

with:

```go
type PermissionsFunc func(context.Context, pgtype.UUID) ([]string, error)

func meHandler(permissions PermissionsFunc) func(http.ResponseWriter, *http.Request, auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
		grants, err := permissions(r.Context(), principal.UserID)
		if err != nil {
			log.Printf("load permissions failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		user := principal.User()
		user.Permissions = grants
		writeJSON(w, http.StatusOK, user)
	}
}
```

- [ ] **Step 5: Wire the real service method at the route**

In `services/auth-service/internal/httpapi/server.go:56`, change:

```go
		mux.HandleFunc("GET /api/v1/auth/me", authenticated(authService, false, meHandler()))
```

to:

```go
		mux.HandleFunc("GET /api/v1/auth/me", authenticated(authService, false, meHandler(authService.Permissions)))
```

- [ ] **Step 6: Run the unit test to confirm it passes**

```powershell
go test ./internal/httpapi/... -run TestMeHandlerAttachesPermissions -v
```

Expected: PASS.

- [ ] **Step 7: Extend the login integration test**

In `services/auth-service/internal/auth/service_integration_test.go`, in `TestAuthenticationLifecycleAgainstPostgreSQL`, after the existing:

```go
	result, err := service.Login(ctx, LoginInput{Identifier: username, Password: "correct-password"})
	if err != nil || result.Token == "" || result.CSRFToken == "" || result.User.Email != email {
		t.Fatalf("result=%+v err=%v", result, err)
	}
```

add:

```go
	if result.User.Permissions == nil {
		t.Fatalf("login result has no permissions field: %+v", result.User)
	}
```

(This user has no role assignment in this test, so an empty-but-non-nil slice is the correct assertion — `make([]string, 0)` from `Service.Permissions` when `ListUserPermissions` returns zero rows.)

- [ ] **Step 8: Run the full auth-service test suite**

```powershell
go build ./...
go vet ./...
go test ./...
```

Expected: all pass (the two `_integration_test.go` files skip without `PLATFORM_TEST_DATABASE_URL`; set it to run them for real).

- [ ] **Step 9: Commit**

```bash
git add services/auth-service/internal/auth/service.go services/auth-service/internal/httpapi/auth.go services/auth-service/internal/httpapi/server.go services/auth-service/internal/httpapi/auth_test.go services/auth-service/internal/auth/service_integration_test.go
git commit -m "feat(auth-service): return the caller's RBAC permissions from login and /me"
```

---

## Part 2 — Frontend: nav filtering and route guard

### Task 3: Add `permissions` to `User` and a `hasPermission` helper

**Files:**
- Modify: `apps/web/app/lib/types.ts`
- Create: `apps/web/app/lib/permissions.ts`

**Interfaces:**
- Produces: `User.permissions: string[]`; `hasPermission(user: User, requirement: string): boolean`.

- [ ] **Step 1: Add the field to `User`**

In `apps/web/app/lib/types.ts:1-6`, change:

```ts
export type User = {
  id: string;
  organizationId?: string;
  email: string;
  displayName: string;
};
```

to:

```ts
export type User = {
  id: string;
  organizationId?: string;
  email: string;
  displayName: string;
  permissions: string[];
};
```

- [ ] **Step 2: Add the helper**

Create `apps/web/app/lib/permissions.ts`:

```ts
import type { User } from "./types";

export function hasPermission(user: User, requirement: string): boolean {
  return user.permissions.includes(requirement);
}
```

- [ ] **Step 3: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Expected: no new errors (existing call sites constructing a `User` — e.g. after login — will need `permissions` in Task 4/5's changes; if `tsc` reports a missing-property error on a `User` literal elsewhere first, note the file and fix it as part of Task 4).

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/lib/types.ts apps/web/app/lib/permissions.ts
git commit -m "feat(web): add permissions field to User and a hasPermission helper"
```

---

### Task 4: Nav filtering and route guard in `PlatformShell`

**Files:**
- Modify: `apps/web/app/components/platform-shell.tsx`

**Interfaces:**
- Consumes: `hasPermission(user, requirement)` from Task 3; `User.permissions` from Task 3.

- [ ] **Step 1: Add `requires` to the nav table**

In `apps/web/app/components/platform-shell.tsx:53-83`, replace the `navigation` array with:

```ts
const navigation = [
  {
    group: "Monitoring",
    items: [
      { href: "/", label: "Overview", icon: Activity },
      { href: "/site-map", label: "Site Map", icon: MapPinned, requires: "plant:read" },
      { href: "/scada/live", label: "SCADA Viewer", icon: Radio, requires: "scada_screen:view" },
      { href: "/alarms", label: "Alarms", icon: BellRing, requires: "alarm:read" },
    ],
  },
  {
    group: "Assets & Config",
    items: [
      { href: "/plants", label: "Plants", icon: Building2, requires: "plant:read" },
      { href: "/register-metadata", label: "Register Metadata", icon: Settings2, requires: "device_model:read" },
      { href: "/scada", label: "SCADA Builder", icon: Workflow, requires: "scada_screen:edit" },
    ],
  },
  {
    group: "Administration",
    items: [
      { href: "/users", label: "Users", icon: Users, requires: "user:read" },
      { href: "/roles", label: "Roles & Permissions", icon: ShieldEllipsis, requires: "role:read" },
      { href: "/middlewares", label: "Middleware Gateways", icon: Server, requires: "middleware_client:read" },
      { href: "/openapi", label: "OpenAPI", icon: FileText, requires: "api_contract:read" },
      { href: "/audit", label: "Audit Log", icon: ShieldCheck, requires: "audit:read" },
      { href: "/sessions", label: "Sessions", icon: ShieldCheck },
      { href: "/settings", label: "Site Branding", icon: Palette, requires: "site_setting:update" },
    ],
  },
] as const satisfies ReadonlyArray<{ group: string; items: ReadonlyArray<{ href: string; label: string; icon: typeof Activity; requires?: string }> }>;
```

(Items with no `requires` — `/`, `/sessions` — stay visible to every authenticated user; `/profile` isn't in the nav table today and needs no entry.)

- [ ] **Step 2: Import the helper**

In `apps/web/app/components/platform-shell.tsx`, add to the existing import block:

```ts
import { hasPermission } from "../lib/permissions";
```

- [ ] **Step 3: Filter the rendered nav**

In `apps/web/app/components/platform-shell.tsx:169-179` (the `<nav>` block), replace:

```tsx
          <nav aria-label="เมนูหลัก">
            {navigation.map(({ group, items }) => (
              <div className="nav-group" key={group}>
                <p className="nav-group-label">{group}</p>
                {items.map(({ href, label, icon: Icon }) => (
                  <Link key={href} href={href} className={pathname === href ? "nav-item active" : "nav-item"} onClick={() => setNavOpen(false)}>
                    <Icon size={18} /> {label}
                  </Link>
                ))}
              </div>
            ))}
          </nav>
```

with:

```tsx
          <nav aria-label="เมนูหลัก">
            {navigation.map(({ group, items }) => {
              const visible = items.filter((item) => !item.requires || hasPermission(user, item.requires));
              if (visible.length === 0) return null;
              return (
                <div className="nav-group" key={group}>
                  <p className="nav-group-label">{group}</p>
                  {visible.map(({ href, label, icon: Icon }) => (
                    <Link key={href} href={href} className={pathname === href ? "nav-item active" : "nav-item"} onClick={() => setNavOpen(false)}>
                      <Icon size={18} /> {label}
                    </Link>
                  ))}
                </div>
              );
            })}
          </nav>
```

- [ ] **Step 4: Add the route guard**

`platform-shell.tsx` needs `next/navigation`'s `useRouter` and a flattened lookup from pathname to `requires`. Add near the top of the file (after the `navigation` const):

```ts
const navRequirements: Record<string, string> = Object.fromEntries(
  navigation.flatMap(({ items }) => items.filter((item) => item.requires).map((item) => [item.href, item.requires as string])),
);
```

Add `useRouter` to the `next/navigation` import (currently `import { usePathname } from "next/navigation";`):

```ts
import { usePathname, useRouter } from "next/navigation";
```

Inside `PlatformShell`, after the existing `const pathname = usePathname();` line, add:

```ts
  const router = useRouter();
```

After the `user` is confirmed non-null (i.e., right after the `if (!user) { ... }` early return, before the `return (...)` that renders the shell), add a guard effect. Since that early return happens before any hooks would normally run for the no-user case, and hooks must run unconditionally, place this `useEffect` earlier, alongside the other `useEffect`s (near line 139, after `useEffect(() => { applyAccentColor(...) }, ...)`):

```ts
  useEffect(() => {
    if (!user) return;
    const required = navRequirements[pathname];
    if (required && !hasPermission(user, required)) router.replace("/");
  }, [user, pathname, router]);
```

- [ ] **Step 5: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Expected: clean.

- [ ] **Step 6: Manual verification**

1. Start the stack (auth-service, platform-api, api-gateway, `apps/web` dev server) per `README.md`.
2. Log in as an existing Platform Admin account — confirm every sidebar item still shows (Platform Admin has every permission).
3. Create a new user via `/users`, assign only the **Viewer** system role. Per `000005_rbac_registry.sql`, Viewer's only grants are `read` on `organization`, `plant`, `asset_group`, `device_model`, `device` — no `role:read`, `user:read`, `audit:read`, `scada_screen:*`, `alarm:read`, `middleware_client:read`, `api_contract:read`, or `site_setting:update`.
4. Log in as that user in a private/incognito window. Confirm the sidebar shows only **Overview, Site Map, Plants, Register Metadata, Sessions** (the items with no `requires` plus `plant:read`/`device_model:read`) — not Users, Roles, Middleware Gateways, OpenAPI, Audit Log, Site Branding, SCADA Builder, SCADA Viewer, Alarms.
5. Manually navigate the Viewer session to `/users` by typing the URL. Confirm it redirects to `/`.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/components/platform-shell.tsx
git commit -m "feat(web): filter sidebar nav and guard routes by user permissions"
```

---

## Part 3 — PrimeReact migration

### Task 5: Swap dependencies and register `PrimeReactProvider`

**Files:**
- Modify: `apps/web/package.json`
- Modify: `apps/web/app/layout.tsx`

**Interfaces:**
- Produces: every descendant of the root layout renders inside `PrimeReactProvider` with `unstyled: true` as the default for all PrimeReact components used in later tasks.

- [ ] **Step 1: Update dependencies**

In `apps/web/package.json`, remove these lines from `dependencies`:

```
"@radix-ui/react-dialog": "^1.1.23",
"@radix-ui/react-popover": "^1.1.23",
"@radix-ui/react-select": "^2.3.7",
"@radix-ui/react-tabs": "^1.1.21",
"@radix-ui/react-tooltip": "^1.2.16",
```

and remove `"sonner"` (find its exact version line via `grep sonner apps/web/package.json` first). Add:

```
"primereact": "^11.0.0",
"primeicons": "^8.0.0",
```

Then run:

```powershell
cd apps/web
npm install
```

- [ ] **Step 2: Register the provider**

Read `apps/web/app/layout.tsx` first to see its current root structure, then wrap the existing body content in `PrimeReactProvider`. Add the import:

```tsx
import { PrimeReactProvider } from "primereact/api";
```

Wrap whatever is currently rendered inside `<body>` with:

```tsx
<PrimeReactProvider value={{ unstyled: true }}>
  {/* existing children */}
</PrimeReactProvider>
```

- [ ] **Step 3: Verify the build**

```powershell
cd apps/web
npx tsc --noEmit
npm run build
```

Expected: this will fail at this point, because `app/components/ui/*.tsx` still import `@radix-ui/*` packages that were just removed, and `sonner.tsx` still imports `"sonner"`. That's expected — Tasks 6-11 fix each file. Confirm the *only* errors are "Cannot find module '@radix-ui/...'" / "Cannot find module 'sonner'" in the `app/components/ui/` files, not anything else.

- [ ] **Step 4: Commit**

```bash
git add apps/web/package.json apps/web/package-lock.json apps/web/app/layout.tsx
git commit -m "chore(web): replace Radix/sonner dependencies with PrimeReact"
```

---

### Task 6: Shared children-collection helper

**Files:**
- Create: `apps/web/app/components/ui/collect-children.ts`

**Interfaces:**
- Produces: `collectByType<P>(children: ReactNode, type: unknown): ReactElement<P>[]` — recursively walks a React children tree and returns every element whose `.type` matches `type`, descending into non-matching elements' own `children` prop. Used by Tasks 8 (Select) and 9 (Tabs) to read compound-component "props-only" children (e.g. `SelectItem`s nested inside a `SelectContent`) without ever mounting those marker components.

- [ ] **Step 1: Write it**

```ts
import { Children, isValidElement, type ReactElement, type ReactNode } from "react";

export function collectByType<P>(children: ReactNode, type: unknown): ReactElement<P>[] {
  const found: ReactElement<P>[] = [];
  Children.forEach(children, (child) => {
    if (!isValidElement(child)) return;
    if (child.type === type) {
      found.push(child as ReactElement<P>);
      return;
    }
    const nested = (child.props as { children?: ReactNode } | null)?.children;
    if (nested) found.push(...collectByType<P>(nested, type));
  });
  return found;
}
```

- [ ] **Step 2: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Expected: no new errors (this file isn't imported by anything yet).

- [ ] **Step 3: Commit**

```bash
git add apps/web/app/components/ui/collect-children.ts
git commit -m "feat(web): add collectByType helper for compound UI wrappers"
```

---

### Task 7: `Button` primitive

**Files:**
- Create: `apps/web/app/components/ui/button.tsx`

**Interfaces:**
- Produces: `Button` component, props = `React.ComponentProps<typeof import("primereact/button").Button> & { variant?: "primary" | "secondary" | "icon" | "text"; compact?: boolean; danger?: boolean; iconOnly?: boolean }`, default `variant="primary"`.

- [ ] **Step 1: Write it**

```tsx
"use client";

import { Button as PrimeButton, type ButtonProps as PrimeButtonProps } from "primereact/button";
import { cn } from "../../lib/cn";

type Variant = "primary" | "secondary" | "icon" | "text";

const base =
  "inline-flex items-center justify-center gap-1.5 font-bold transition disabled:cursor-not-allowed disabled:opacity-48 focus:outline-none focus-visible:ring-2 focus-visible:ring-focus";

const variantClass: Record<Variant, string> = {
  primary: "rounded-[var(--radius-md)] bg-[var(--brand)] px-4 py-2 text-sm text-white shadow-[var(--shadow-sm)] hover:brightness-110",
  secondary: "rounded-[var(--radius-md)] border border-line bg-surface px-4 py-2 text-sm text-ink hover:bg-canvas",
  icon: "rounded-[var(--radius-sm)] p-2 text-ink-soft hover:bg-canvas",
  text: "text-sm text-[var(--brand)] hover:underline",
};

type Props = PrimeButtonProps & {
  variant?: Variant;
  compact?: boolean;
  danger?: boolean;
  iconOnly?: boolean;
};

export function Button({ variant = "primary", compact, danger, iconOnly, className, ...props }: Props) {
  return (
    <PrimeButton
      unstyled
      className={cn(
        base,
        variantClass[variant],
        compact && "px-2.5 py-1.5 text-xs",
        iconOnly && "px-2 py-2",
        danger && "text-[var(--danger)]",
        className,
      )}
      {...props}
    />
  );
}
```

- [ ] **Step 2: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Expected: clean (unused-file, no call sites yet).

- [ ] **Step 3: Commit**

```bash
git add apps/web/app/components/ui/button.tsx
git commit -m "feat(web): add PrimeReact-backed Button primitive"
```

---

### Task 8: Replace hand-rolled button classes with `<Button>`

**Files (all 13 modified in this one task — mechanical, same rule applied everywhere):**
- `apps/web/app/components/auth-screen.tsx`
- `apps/web/app/features/alarms/alarms-page.tsx`
- `apps/web/app/features/audit/audit-page.tsx`
- `apps/web/app/features/dashboard/overview-page.tsx`
- `apps/web/app/features/middlewares/middlewares-page.tsx`
- `apps/web/app/features/plants/plants-page.tsx`
- `apps/web/app/features/profile/profile-page.tsx`
- `apps/web/app/features/roles/roles-page.tsx`
- `apps/web/app/features/scada/scada-page.tsx`
- `apps/web/app/features/scada/scada-viewer-page.tsx`
- `apps/web/app/features/sessions/sessions-page.tsx`
- `apps/web/app/features/settings/site-settings-page.tsx`
- `apps/web/app/features/users/users-page.tsx`

**Interfaces:**
- Consumes: `Button` from Task 7 (`import { Button } from "../../components/ui/button";` or `"./button"` for `auth-screen.tsx`, matching each file's existing relative-import depth for `app/components/ui/*`).

**Transformation rule** (apply per `className` combination found in the codebase today):

| Old | New |
|---|---|
| `className="primary-button"` | `<Button>` |
| `className="primary-button compact"` | `<Button compact>` |
| `className="secondary-button"` | `<Button variant="secondary">` |
| `className="secondary-button compact"` | `<Button variant="secondary" compact>` |
| `className="secondary-button danger"` | `<Button variant="secondary" danger>` |
| `className="secondary-button compact danger-button"` | `<Button variant="secondary" compact danger>` |
| `className="secondary-button compact icon-only"` | `<Button variant="secondary" compact iconOnly>` |
| `className="icon-button"` | `<Button variant="icon">` |
| `className="icon-button danger"` | `<Button variant="icon" danger>` |
| `className="text-button"` | `<Button variant="text">` |
| `className="text-button compact"` | `<Button variant="text" compact>` |
| `className="text-button danger"` | `<Button variant="text" danger>` |
| `className="text-button danger compact"` | `<Button variant="text" danger compact>` |
| `className="text-button compact icon-only"` | `<Button variant="text" compact iconOnly>` |

All other props on the original `<button>` (`onClick`, `disabled`, `title`, `aria-label`, `type`, children — including bare lucide-react icon elements) carry over unchanged; only the tag name and `className` prop are replaced. Example, from `apps/web/app/features/roles/roles-page.tsx`:

Before:

```tsx
<button className="icon-button" onClick={() => void loadRoles()} title="รีเฟรช" aria-label="รีเฟรชรายการ Role"><RefreshCw size={18} /></button>
<button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Role</button>
```

After:

```tsx
<Button variant="icon" onClick={() => void loadRoles()} title="รีเฟรช" aria-label="รีเฟรชรายการ Role"><RefreshCw size={18} /></Button>
<Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Role</Button>
```

- [ ] **Step 1: Add the `Button` import to all 13 files**

For every file listed above, add an import line for `Button` alongside its existing `app/components/ui/*` imports (same relative path style already used in that file, e.g. `roles-page.tsx` already imports `../../components/ui/dialog`, so add `import { Button } from "../../components/ui/button";`).

- [ ] **Step 2: Replace every matching `<button className="...">` with `<Button ...>` per the transformation rule**

Go file by file. Every `<button>` tag with one of the 14 class combinations above becomes a `<Button>` with the matching props, per the rule table.

- [ ] **Step 3: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Expected: clean across all 13 files.

- [ ] **Step 4: Manual smoke test**

Start `apps/web` dev server. Visit Roles, Plants, Users, Site Branding, Sessions pages; click a primary button, a secondary button, an icon-only button, and a text-button on each you visit. Confirm every click still fires its original action (buttons aren't now dead/no-op), and specifically confirm every `<form onSubmit={...}>`'s submit button (the one with no explicit `type` before this change, now inside `<Button>` with no explicit `type`) still submits — e.g. Site Branding's "บันทึก" button, Roles' role-editor "บันทึก" button.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/components/auth-screen.tsx apps/web/app/features/alarms/alarms-page.tsx apps/web/app/features/audit/audit-page.tsx apps/web/app/features/dashboard/overview-page.tsx apps/web/app/features/middlewares/middlewares-page.tsx apps/web/app/features/plants/plants-page.tsx apps/web/app/features/profile/profile-page.tsx apps/web/app/features/roles/roles-page.tsx apps/web/app/features/scada/scada-page.tsx apps/web/app/features/scada/scada-viewer-page.tsx apps/web/app/features/sessions/sessions-page.tsx apps/web/app/features/settings/site-settings-page.tsx apps/web/app/features/users/users-page.tsx
git commit -m "refactor(web): replace hand-rolled button classes with the Button primitive"
```

---

### Task 9: Rewrite `dialog.tsx` on PrimeReact

**Files:**
- Modify: `apps/web/app/components/ui/dialog.tsx`

**Interfaces:**
- Produces (unchanged from today): `Dialog({ open, onOpenChange, children })`, `DialogContent({ className, showClose, children })`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogBody` — same names/props every call site (`alarms-page.tsx`, `dashboard/overview-page.tsx`, `middlewares-page.tsx`, `plants-page.tsx`, `profile-page.tsx`, `register-metadata-page.tsx`, `roles-page.tsx`, `scada-page.tsx`, `users-page.tsx`) already uses. No call sites change in this task.

- [ ] **Step 1: Rewrite the file**

```tsx
"use client";

import * as React from "react";
import { Dialog as PrimeDialog } from "primereact/dialog";
import { X } from "lucide-react";
import { cn } from "../../lib/cn";

type DialogContextValue = { onOpenChange?: (open: boolean) => void };
const DialogContext = React.createContext<DialogContextValue>({});

function Dialog({ open, onOpenChange, children }: { open: boolean; onOpenChange?: (open: boolean) => void; children: React.ReactNode }) {
  if (!open) return null;
  return <DialogContext.Provider value={{ onOpenChange }}>{children}</DialogContext.Provider>;
}

function DialogContent({
  className,
  children,
  showClose = true,
}: {
  className?: string;
  children: React.ReactNode;
  showClose?: boolean;
}) {
  const { onOpenChange } = React.useContext(DialogContext);
  return (
    <PrimeDialog
      visible
      modal
      unstyled
      closable={false}
      onHide={() => onOpenChange?.(false)}
      pt={{
        root: { className: cn("relative w-full max-w-lg max-h-[calc(100vh-2rem)] overflow-y-auto rounded-[var(--radius-lg)] border border-line bg-surface shadow-[var(--shadow-lg)]", className) },
        mask: { className: "fixed inset-0 z-50 flex items-center justify-center bg-ink/55 p-4" },
      }}
    >
      {children}
      {showClose && (
        <button
          type="button"
          onClick={() => onOpenChange?.(false)}
          className="absolute right-4 top-4 rounded-[var(--radius-sm)] p-1.5 text-ink-soft transition hover:bg-canvas focus:outline-none focus-visible:ring-2 focus-visible:ring-focus"
          aria-label="ปิด"
          title="ปิด"
        >
          <X size={18} />
        </button>
      )}
    </PrimeDialog>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("flex items-center justify-between gap-4 border-b border-line px-5 py-4", className)} {...props} />;
}

function DialogTitle({ className, ...props }: React.ComponentProps<"h2">) {
  return <h2 className={cn("font-display text-xl font-extrabold text-ink", className)} {...props} />;
}

function DialogDescription({ className, ...props }: React.ComponentProps<"p">) {
  return <p className={cn("text-xs font-extrabold uppercase text-ink-soft", className)} {...props} />;
}

function DialogBody({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("p-5", className)} {...props} />;
}

export { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody };
```

- [ ] **Step 2: Type-check and fix `pt` key names if needed**

```powershell
cd apps/web
npx tsc --noEmit
```

If TypeScript rejects the `pt.root` or `pt.mask` keys, open `node_modules/primereact/dialog/dialog.d.ts` (or the editor's hover-info on `PrimeDialog`'s `pt` prop) to find the exact `DialogPassThroughOptions` section names for this installed version and adjust the two keys in the `pt={{ ... }}` object accordingly — the rest of the file is unaffected.

- [ ] **Step 3: Manual smoke test**

Start the dev server, open the Role editor dialog (`/roles` → "เพิ่ม Role"), confirm it renders centered with a backdrop, the header/title/description/body look right, the × closes it, and clicking the backdrop does not close it (matches today's `onOpenChange` — only explicit close actions call it, since `closable={false}` and the mask has no click handler wired). Repeat for one dialog on `/plants` and `/users`.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/ui/dialog.tsx
git commit -m "refactor(web): back the Dialog primitive with PrimeReact"
```

---

### Task 10: Rewrite `select.tsx` on PrimeReact

**Files:**
- Modify: `apps/web/app/components/ui/select.tsx`

**Interfaces:**
- Consumes: `collectByType` from Task 6.
- Produces (unchanged from today): `Select({ value, onValueChange, disabled, children })`, `SelectValue({ placeholder })`, `SelectTrigger`, `SelectContent`, `SelectItem({ value, disabled, children })` — same names/props every call site (`alarms-page.tsx`, `dashboard/overview-page.tsx`, `middlewares-page.tsx`, `plants-page.tsx`, `register-metadata-page.tsx`, `scada-page.tsx`, `users-page.tsx`) already uses. No call sites change in this task.

- [ ] **Step 1: Rewrite the file**

```tsx
"use client";

import * as React from "react";
import { Dropdown } from "primereact/dropdown";
import { collectByType } from "./collect-children";

function SelectValue(_props: { placeholder?: string }) {
  return null;
}

function SelectTrigger({ children }: { children?: React.ReactNode }) {
  return <>{children}</>;
}

function SelectContent({ children }: { children?: React.ReactNode }) {
  return <>{children}</>;
}

function SelectItem({ children }: { value: string; disabled?: boolean; children?: React.ReactNode }) {
  return <>{children}</>;
}

function Select({
  value,
  onValueChange,
  disabled,
  children,
}: {
  value?: string;
  onValueChange?: (value: string) => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  const items = collectByType<{ value: string; disabled?: boolean; children?: React.ReactNode }>(children, SelectItem);
  const values = collectByType<{ placeholder?: string }>(children, SelectValue);
  const options = items.map((item) => ({ label: item.props.children, value: item.props.value, disabled: item.props.disabled }));
  return (
    <Dropdown
      value={value}
      onChange={(event) => onValueChange?.(event.value as string)}
      disabled={disabled}
      options={options}
      optionLabel="label"
      optionValue="value"
      optionDisabled="disabled"
      placeholder={values[0]?.props.placeholder}
      unstyled
      className="flex h-10 w-full items-center gap-2 rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-sm text-ink"
      panelClassName="rounded-[var(--radius-sm)] border border-line bg-surface shadow-[var(--shadow-lg)]"
    />
  );
}

export { Select, SelectValue, SelectTrigger, SelectContent, SelectItem };
```

- [ ] **Step 2: Type-check and fix option types if needed**

```powershell
cd apps/web
npx tsc --noEmit
```

`item.props.children` is typed `React.ReactNode`, but PrimeReact's default `optionLabel` rendering expects a `string` for its internal filter/accessibility text — if `tsc` or runtime warns, confirm every existing `SelectItem` call site passes plain text/interpolated-string children (checked during design: yes, e.g. `{m.manufacturer} / {m.deviceType} / {m.model}`), so this is safe; if a future call site needs a non-string label, that's a real gap to solve then, not now.

- [ ] **Step 3: Manual smoke test**

Start the dev server, open the Plants page's device-model `<Select>` (`plants-page.tsx:341-344`), confirm it opens, lists options, selecting one calls the same `setDeviceModelId` as before, and the disabled state (`models.length === 0`) still renders disabled.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/ui/select.tsx
git commit -m "refactor(web): back the Select primitive with PrimeReact Dropdown"
```

---

### Task 11: Rewrite `tabs.tsx` on PrimeReact

**Files:**
- Modify: `apps/web/app/components/ui/tabs.tsx`

**Interfaces:**
- Consumes: `collectByType` from Task 6.
- Produces (unchanged from today): `Tabs({ value, defaultValue, onValueChange, children })`, `TabsList`, `TabsTrigger({ value, children })`, `TabsContent({ value, children })` — same names/props every call site (`alarms-page.tsx`, `middlewares-page.tsx`, `scada-page.tsx`) already uses, including the "list-only, no content panels" usage in `alarms-page.tsx`/`scada-page.tsx` (pure segmented-switch UI) and the "list + content panels" usage in `middlewares-page.tsx`. No call sites change in this task.

- [ ] **Step 1: Rewrite the file**

```tsx
"use client";

import * as React from "react";
import { TabView, TabPanel } from "primereact/tabview";
import { collectByType } from "./collect-children";

function TabsList({ children }: { children?: React.ReactNode }) {
  return <>{children}</>;
}

function TabsTrigger({ children }: { value: string; children?: React.ReactNode }) {
  return <>{children}</>;
}

function TabsContent({ children }: { value: string; children?: React.ReactNode }) {
  return <>{children}</>;
}

function Tabs({
  value,
  defaultValue,
  onValueChange,
  children,
}: {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  children: React.ReactNode;
}) {
  const triggers = collectByType<{ value: string; children?: React.ReactNode }>(children, TabsTrigger);
  const contents = collectByType<{ value: string; children?: React.ReactNode }>(children, TabsContent);
  const values = triggers.map((trigger) => trigger.props.value);
  const [uncontrolled, setUncontrolled] = React.useState(defaultValue ?? values[0]);
  const active = value ?? uncontrolled;
  const activeIndex = Math.max(0, values.indexOf(active));

  function handleTabChange(index: number) {
    const next = values[index];
    if (value === undefined) setUncontrolled(next);
    onValueChange?.(next);
  }

  return (
    <TabView
      activeIndex={activeIndex}
      onTabChange={(event) => handleTabChange(event.index)}
      unstyled
      pt={{
        nav: { className: "inline-flex h-10 items-center gap-0.5 rounded-[var(--radius-sm)] border border-line bg-canvas p-1" },
        inkbar: { className: "hidden" },
      }}
    >
      {triggers.map((trigger) => (
        <TabPanel
          key={trigger.props.value}
          header={trigger.props.children}
          headerClassName="inline-flex h-[30px] items-center gap-1.5 rounded-[calc(var(--radius-sm)-2px)] px-3 text-xs font-bold text-ink-soft"
          pt={{
            headerAction: { className: "flex h-full items-center gap-1.5 px-1" },
          }}
        >
          {contents.find((content) => content.props.value === trigger.props.value)?.props.children}
        </TabPanel>
      ))}
    </TabView>
  );
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
```

The active-tab highlight (`data-[state=active]:bg-surface` etc. in the old Radix version) needs a PrimeReact-equivalent active-header selector; if `TabPanel`'s `headerClassName` doesn't apply an active-state variant on its own, add a small rule targeting `[aria-selected="true"]` inside the tab nav to `app/globals.css` in Task 14 rather than fighting it here — carry this forward as a concrete to-do into Task 14 Step 1, not a blocker for this task.

- [ ] **Step 2: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Fix any `pt` key-name mismatches the same way as Task 9 Step 2 (check `node_modules/primereact/tabview/tabview.d.ts`).

- [ ] **Step 3: Manual smoke test**

Start the dev server. On `/alarms`, confirm the Log/Rules segmented switch still toggles the list. On `/middlewares`, confirm the four tabs (Plants/Push-Pull/Software Update/Connections) switch their panel content. On `/scada`, confirm the Edit/Preview/Published switch still works (including that selecting "Published" calls `showPublished()` as before, not just a visual switch).

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/ui/tabs.tsx
git commit -m "refactor(web): back the Tabs primitive with PrimeReact TabView"
```

---

### Task 12: Reimplement `tooltip.tsx` and `popover.tsx` as small custom primitives

**Files:**
- Modify: `apps/web/app/components/ui/tooltip.tsx`
- Modify: `apps/web/app/components/ui/popover.tsx`

**Interfaces:**
- Produces (unchanged from today): `Tooltip`, `TooltipTrigger({ asChild, children })`, `TooltipContent({ className, children })`; `Popover`, `PopoverTrigger({ asChild, children })`, `PopoverContent({ className, align, children })`. No call sites change (`middlewares-page.tsx`, `scada-page.tsx` for Tooltip; Popover has zero call sites today).

PrimeReact's own `Tooltip`/`OverlayPanel` components are imperative and target-selector-based, not children-composable the way these call sites use them (`<Tooltip><TooltipTrigger asChild>{el}</TooltipTrigger><TooltipContent>...</TooltipContent></Tooltip>`), and today's visual styling on `TooltipContent`/`PopoverContent` is already 100% this codebase's own Tailwind/CSS-variable classes — Radix supplied only positioning/portal behavior here, not appearance. Rather than force a mismatched imperative API onto every call site, these become small self-contained components (no UI library dependency at all) that preserve the exact compound API. `ponytail:` this drops floating-ui-style viewport-collision repositioning (tooltips/popovers always open below-right of their trigger); add real collision detection if a tooltip/popover is ever placed near a screen edge in practice.

- [ ] **Step 1: Rewrite `tooltip.tsx`**

```tsx
"use client";

import * as React from "react";
import { cn } from "../../lib/cn";

type TooltipContextValue = { open: boolean; setOpen: (open: boolean) => void };
const TooltipContext = React.createContext<TooltipContextValue | null>(null);

function Tooltip({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false);
  return (
    <TooltipContext.Provider value={{ open, setOpen }}>
      <span className="relative inline-block">{children}</span>
    </TooltipContext.Provider>
  );
}

function TooltipTrigger({ children }: { asChild?: boolean; children: React.ReactElement }) {
  const ctx = React.useContext(TooltipContext);
  if (!ctx) throw new Error("TooltipTrigger must be used inside Tooltip");
  return React.cloneElement(children, {
    onMouseEnter: () => ctx.setOpen(true),
    onMouseLeave: () => ctx.setOpen(false),
    onFocus: () => ctx.setOpen(true),
    onBlur: () => ctx.setOpen(false),
  } as React.HTMLAttributes<HTMLElement>);
}

function TooltipContent({ className, children }: { className?: string; children: React.ReactNode }) {
  const ctx = React.useContext(TooltipContext);
  if (!ctx?.open) return null;
  return (
    <span
      role="tooltip"
      className={cn(
        "absolute left-0 top-full z-50 mt-1 max-w-64 rounded-[var(--radius-sm)] bg-ink px-2.5 py-1.5 text-xs font-semibold text-surface shadow-[var(--shadow-sm)]",
        className,
      )}
    >
      {children}
    </span>
  );
}

export { Tooltip, TooltipTrigger, TooltipContent };
```

- [ ] **Step 2: Rewrite `popover.tsx`**

```tsx
"use client";

import * as React from "react";
import { cn } from "../../lib/cn";

type PopoverContextValue = { open: boolean; setOpen: (open: boolean) => void };
const PopoverContext = React.createContext<PopoverContextValue | null>(null);

function Popover({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false);
  const rootRef = React.useRef<HTMLSpanElement>(null);
  React.useEffect(() => {
    if (!open) return;
    function handleClickOutside(event: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);
  return (
    <PopoverContext.Provider value={{ open, setOpen }}>
      <span ref={rootRef} className="relative inline-block">
        {children}
      </span>
    </PopoverContext.Provider>
  );
}

function PopoverTrigger({ children }: { asChild?: boolean; children: React.ReactElement }) {
  const ctx = React.useContext(PopoverContext);
  if (!ctx) throw new Error("PopoverTrigger must be used inside Popover");
  return React.cloneElement(children, {
    onClick: () => ctx.setOpen(!ctx.open),
  } as React.HTMLAttributes<HTMLElement>);
}

function PopoverContent({
  className,
  align = "center",
  children,
}: {
  className?: string;
  align?: "start" | "center" | "end";
  children: React.ReactNode;
}) {
  const ctx = React.useContext(PopoverContext);
  if (!ctx?.open) return null;
  const alignClass = align === "start" ? "left-0" : align === "end" ? "right-0" : "left-1/2 -translate-x-1/2";
  return (
    <div
      className={cn(
        "absolute top-full z-50 mt-2 w-72 rounded-[var(--radius-md)] border border-line bg-surface p-4 text-sm text-ink shadow-[var(--shadow-lg)]",
        alignClass,
        className,
      )}
    >
      {children}
    </div>
  );
}

export { Popover, PopoverTrigger, PopoverContent };
```

- [ ] **Step 3: Type-check**

```powershell
cd apps/web
npx tsc --noEmit
```

Expected: clean.

- [ ] **Step 4: Manual smoke test**

On `/middlewares`, hover each of the four action buttons wrapped in `Tooltip` (Push Config, Pull Config, Upload Patch, Restart) and confirm the tooltip text still appears. On `/scada`, select a node and hover the node-type hint text in the inspector header, confirm the tooltip shows type/ID.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/components/ui/tooltip.tsx apps/web/app/components/ui/popover.tsx
git commit -m "refactor(web): reimplement Tooltip/Popover without Radix"
```

---

### Task 13: Rewrite `sonner.tsx` on PrimeReact `Toast`

**Files:**
- Modify: `apps/web/app/components/ui/sonner.tsx`

**Interfaces:**
- Produces (unchanged from today): `Toaster` component (mounted once in `platform-shell.tsx`); `toast.success(message: string)`, `toast.error(message: string)` — same call-site API every one of the 11 files calling `toast.success`/`toast.error` already uses.

- [ ] **Step 1: Rewrite the file**

```tsx
"use client";

import * as React from "react";
import { Toast } from "primereact/toast";

let activeToast: Toast | null = null;

function Toaster() {
  const ref = React.useRef<Toast>(null);
  React.useEffect(() => {
    activeToast = ref.current;
    return () => {
      activeToast = null;
    };
  }, []);
  return (
    <Toast
      ref={ref}
      position="bottom-right"
      unstyled
      pt={{
        message: {
          className:
            "rounded-[var(--radius-md)] border border-line bg-surface px-4 py-3 text-sm text-ink shadow-[var(--shadow-lg)]",
        },
      }}
    />
  );
}

export const toast = {
  success(message: string) {
    activeToast?.show({ severity: "success", summary: message, life: 3500 });
  },
  error(message: string) {
    activeToast?.show({ severity: "error", summary: message, life: 5000 });
  },
};

export { Toaster };
```

- [ ] **Step 2: Type-check and fix `pt` key names if needed**

```powershell
cd apps/web
npx tsc --noEmit
```

If the `message` passthrough key doesn't exist on this installed version's `ToastPassThroughOptions`, check `node_modules/primereact/toast/toast.d.ts` for the correct section name and adjust.

- [ ] **Step 3: Manual smoke test**

Trigger any success toast (e.g. save the Site Branding form) and any error toast (e.g. submit an invalid Role name), confirm both render bottom-right and match the app's surface/border/shadow styling.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/ui/sonner.tsx
git commit -m "refactor(web): back the toast primitive with PrimeReact Toast"
```

---

### Task 14: Remove now-dead CSS and amend the design-system spec

**Files:**
- Modify: `apps/web/app/globals.css`
- Modify: `docs/superpowers/specs/2026-07-31-design-system-redesign.md`

- [ ] **Step 1: Remove superseded CSS**

In `apps/web/app/globals.css`, find and delete the rule blocks for `.primary-button`, `.secondary-button`, `.icon-button`, `.text-button`, and any `.modal-backdrop`/native-`<select>`-specific chrome rules that Tasks 8-13 made unreachable. Do not touch `.plant-row`, `.device-row`, `.user-row`, `.api-key-row`, `.audit-row`, `.session-row`, `.role-row`, `.metadata-row`, `.section-heading`, `.form-message`, or any other class not tied to the primitives just replaced.

- [ ] **Step 2: Verify nothing still references the removed classes**

```powershell
cd apps/web
grep -rn "primary-button\|secondary-button\|icon-button\|text-button" app --include=*.tsx
```

Expected: no output (Task 8 already replaced every call site).

- [ ] **Step 3: Amend the prior spec**

In `docs/superpowers/specs/2026-07-31-design-system-redesign.md`, in the "Component approach" section (the paragraph starting "Install `shadcn/ui`..."), add a note directly below it:

```markdown
**Superseded 2026-08-02:** the component library choice above (shadcn/ui + Radix) was replaced with PrimeReact — see `docs/superpowers/specs/2026-08-02-rbac-nav-guard-and-primereact-design.md`. The design tokens sections above (color, typography, spacing/radius/shadow) are unaffected and remain authoritative.
```

- [ ] **Step 4: Type-check and build**

```powershell
cd apps/web
npx tsc --noEmit
npm run build
```

Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/globals.css docs/superpowers/specs/2026-07-31-design-system-redesign.md
git commit -m "chore(web): remove CSS superseded by PrimeReact primitives, amend design-system spec"
```

---

### Task 15: Full manual verification pass

**Files:** none (verification only).

- [ ] **Step 1: Backend checks**

```powershell
cd services/auth-service
gofmt -l .
go vet ./...
go test ./...
cd ../platform-api
gofmt -l .
go vet ./...
go test ./...
```

Expected: `gofmt -l` prints nothing; both `go test ./...` pass (set `PLATFORM_TEST_DATABASE_URL` first to include the integration tests from Tasks 1-2).

- [ ] **Step 2: Frontend checks**

```powershell
cd apps/web
npx tsc --noEmit
npm run build
```

Expected: both clean.

- [ ] **Step 3: End-to-end walkthrough**

Start auth-service, platform-api, api-gateway, and `apps/web` dev server. As a Platform Admin:
1. Open Site Branding (`/settings`), change the site name and accent color, save, confirm the toast and the updated name/color reflect immediately in the sidebar brand mark and topbar.
2. Upload a logo, confirm it appears in the sidebar/topbar and on the (logged-out) login screen.
3. Open the Role editor dialog, the device-model Select on Plants, a Tabs view on Middlewares, and a Tooltip on Middlewares — confirm all look and behave consistently with the rest of the app (this is the original "ปุ่มเพี้ยนๆ" complaint — confirm it's resolved).

As the Viewer-role test user created in Task 4:
4. Confirm the sidebar only shows the permitted items and that typing a disallowed URL (e.g. `/users`) redirects to `/`.

- [ ] **Step 4: Report results**

No commit for this task — if any check fails, return to the relevant earlier task and fix it there (with its own commit), then re-run this task's checks.

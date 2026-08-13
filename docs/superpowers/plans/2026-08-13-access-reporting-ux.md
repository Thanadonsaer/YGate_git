# Access, Reporting, and Analytics UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce write permissions consistently in the UI, expose email verification and role scope, and add calculated multi-device CSV exports.

**Architecture:** Use one web permission helper driven by the authenticated session to gate mutation controls. Preserve backend authorization as the source of truth. Extend auth-service user/profile JSON with verification and role data; use existing device-history and telemetry-math processing for a client-side, one-file CSV export.

**Tech Stack:** Next.js/React/TypeScript, Go net/http and pgx, PostgreSQL, Node test runner.

**Spec:** `docs/superpowers/specs/2026-08-13-access-reporting-ux-design.md`

## Global Constraints

- Hide mutation controls without their matching `resource:action` permission; never weaken server-side authorization.
- System Admin alone may choose a different organization.
- Reports export CSV, not XLSX; output one file containing names rather than IDs.
- CSV calculated values must reuse Analytics scaling/offset and kWh integration rules.

---

### Task 1: Shared permission and organization-scope helpers

**Files:**
- Modify: `apps/web/app/lib/permissions.ts`
- Test: `apps/web/app/lib/permissions.test.ts`

**Interfaces:**
- Produces: `can(user, resourceType, action): boolean` and `isSystemAdmin(user): boolean`.

- [ ] **Step 1: Write the failing test**

```ts
test("can checks a resource action grant", () => {
  assert.equal(can({ permissions: ["plant:update"] } as User, "plant", "update"), true);
  assert.equal(can({ permissions: ["plant:read"] } as User, "plant", "update"), false);
});

test("isSystemAdmin detects the global role", () => {
  assert.equal(isSystemAdmin({ roles: ["System Admin"] } as User), true);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --test-name-pattern="resource action grant|global role"` from `apps/web`.

- [ ] **Step 3: Write minimal implementation**

```ts
export function can(user: User, resourceType: string, action: string) {
  return user.permissions.includes(`${resourceType}:${action}`);
}

export function isSystemAdmin(user: Pick<User, "roles">) {
  return user.roles.includes("System Admin");
}
```

- [ ] **Step 4: Run tests and typecheck**

Run: `npm test; npm run typecheck` from `apps/web`.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/lib/permissions.ts apps/web/app/lib/permissions.test.ts
git commit -m "feat(web): add shared permission helpers"
```

### Task 2: Verification and role data in user/profile APIs

**Files:**
- Modify: `services/auth-service/internal/core/users.go`
- Modify: `services/auth-service/internal/core/profile.go`
- Modify: `apps/web/app/lib/types.ts`
- Test: `services/auth-service/internal/core/users_test.go` or existing users integration test file

**Interfaces:**
- Produces: `ManagedUser.emailVerifiedAt?: time.Time`, `SelfProfile.roles: []string`, and matching JSON fields `emailVerifiedAt` and `roles`.

- [ ] **Step 1: Write a failing auth-service test**

Assert the Users query result serializes `emailVerifiedAt` for a verified user and omits/nulls it for an unverified registration user; assert OwnProfile returns the assigned role names.

- [ ] **Step 2: Run the focused Go test and verify it fails**

Run: `go test ./internal/core -run 'Test.*(Verified|OwnProfile)'` from `services/auth-service`.

- [ ] **Step 3: Add fields and query columns**

Add `u.email_verified_at` to the Users SELECT and scan it into `pgtype.Timestamptz`; add `EmailVerifiedAt *time.Time` to `ManagedUser`. Change the profile query to aggregate assigned roles and populate `SelfProfile.Roles`. Add `emailVerifiedAt?: string | null` to `ManagedUser` and `roles: string[]` to `SelfProfile` in `types.ts`.

- [ ] **Step 4: Run Go and web checks**

Run: `go test ./internal/core` from `services/auth-service`; then `npm run typecheck` from `apps/web`.

- [ ] **Step 5: Commit**

```bash
git add services/auth-service/internal/core/users.go services/auth-service/internal/core/profile.go apps/web/app/lib/types.ts
git commit -m "feat(users): expose verification and profile roles"
```

### Task 3: Permission-gated user, role, plant, and profile controls

**Files:**
- Modify: `apps/web/app/features/users/users-page.tsx`
- Modify: `apps/web/app/features/roles/roles-page.tsx`
- Modify: `apps/web/app/features/plants/plants-page.tsx`
- Modify: `apps/web/app/features/profile/profile-page.tsx`

**Interfaces:**
- Consumes: `can`, `isSystemAdmin`, expanded `SelfProfile` and `ManagedUser`.
- Produces: read-only views for callers without write permissions; organization selectors scoped by System Admin status.

- [ ] **Step 1: Write a failing render/helper test**

Extract a small pure `userActions`/`roleActions` predicate only if needed; assert an account with only `user:read` has no edit, reset, unlock, disable, or delete action, and an account with `role:read` has no Role create/update/delete action.

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `npm test -- --test-name-pattern="read-only.*action"` from `apps/web`.

- [ ] **Step 3: Gate User Management UI**

Use `usePlatformSession()` and `can` to render:
```tsx
{can(currentUser, "user", "create") && <Button onClick={() => setEditor("create")}>...</Button>}
{can(currentUser, "user", "update") && item.id !== currentUser.id && <Button ...><Pencil /></Button>}
{can(currentUser, "user", "disable") && item.id !== currentUser.id && <Button ... />}
```
Add a `Verified` table column using `StatusTag`, with `Verified {formatDate(emailVerifiedAt)}` or `ยังไม่ Verify`. For the current user, pass `roleReadOnly` to `UserEditor` and omit its role selector/update payload.

- [ ] **Step 4: Gate Roles and scope by organization**

Load organizations only for System Admin, add an Organization Select above the table, and filter roles by its ID. Non-System Admin uses `defaultOrganizationId` in a disabled Select. Hide Add/Edit/Delete based on `role:create`, `role:update`, `role:delete`; retain system-role immutability.

- [ ] **Step 5: Gate Plants and scope create organization**

Hide import/create/edit/delete/status actions by their existing Plant/Device permissions. In `PlantEditor`, render the organization as a Select: enabled for System Admin and populated with organizations; disabled and fixed to `defaultOrganizationId` otherwise. Ensure a non-System Admin create payload always sends that fixed ID.

- [ ] **Step 6: Show own roles read-only in My Profile**

Add a `Role` field in `profile-preview` from `profile.roles.join(", ") || "-"`; do not add role inputs to `EditProfileDialog`.

- [ ] **Step 7: Run tests, typecheck, and build**

Run: `npm test; npm run typecheck; npm run build` from `apps/web`.

- [ ] **Step 8: Commit**

```bash
git add apps/web/app/features/users/users-page.tsx apps/web/app/features/roles/roles-page.tsx apps/web/app/features/plants/plants-page.tsx apps/web/app/features/profile/profile-page.tsx
git commit -m "feat(web): gate management actions by permission"
```

### Task 4: Analytics loading overlay

**Files:**
- Modify: `apps/web/app/features/energy-analysis/energy-analysis-page.tsx`
- Modify: `apps/web/app/globals.css`

**Interfaces:**
- Consumes: existing `loading` state around history requests.
- Produces: visible chart-area loading mask when query-driving state changes.

- [ ] **Step 1: Write a failing render test or extract a pure loading predicate**

Assert `isQueryLoading(true, selectedDevices)` returns true while a replacement request is pending and false after it resolves.

- [ ] **Step 2: Run focused test and verify it fails**

Run: `npm test -- --test-name-pattern="query loading"` from `apps/web`.

- [ ] **Step 3: Implement the overlay**

Wrap chart panels in a positioned container and conditionally render:
```tsx
{loading && <div className="analytics-loading" role="status">กำลังโหลดข้อมูล…</div>}
```
The overlay must remain until the active request settles, and aborted obsolete requests must not clear it for a newer request.

- [ ] **Step 4: Run verification**

Run: `npm test; npm run typecheck` from `apps/web`.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/energy-analysis/energy-analysis-page.tsx apps/web/app/globals.css
git commit -m "fix(analytics): show loading while filters refresh charts"
```

### Task 5: Calculated multi-device CSV report export

**Files:**
- Modify: `apps/web/app/features/reports/reports-page.tsx`
- Create: `apps/web/app/lib/calculated-report-csv.ts`
- Test: `apps/web/app/lib/calculated-report-csv.test.ts`

**Interfaces:**
- Consumes: `Plant`, `Device`, history response types, `toSeries`, scaling metadata, and kWh integration from `telemetry-math.ts`.
- Produces: `calculatedReportCSV(rows): string`, with `Plant`, `Device`, `Observed at`, parameter columns, and calculated kWh columns.

- [ ] **Step 1: Write failing CSV tests**

```ts
test("calculated report uses names and scaled values", () => {
  const csv = calculatedReportCSV([{ plantName: "North", deviceName: "INV-01", observedAt: "2026-08-13T00:00:00Z", values: { power: 12.5 }, kWh: 0.25 }]);
  assert.match(csv, /Plant,Device,Observed at,power,kWh/);
  assert.match(csv, /North,INV-01/);
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `npm test -- --test-name-pattern="calculated report uses names"` from `apps/web`.

- [ ] **Step 3: Implement CSV transformer**

Use the existing CSV escaping convention in `seriesToCSV`. Build a union of parameter keys across selected devices, one output record per timestamp/device, and calculate kWh with the same `totalEnergyKWh`/bucket integration semantics already used in Analytics.

- [ ] **Step 4: Replace Reports form and export action**

Load Plants; when Plant selection changes load its Devices. Provide multi-select controls for Plants and Devices, default the date range as today’s existing range, fetch authorized history for each selected Device, process it through the helper, and call:
```ts
downloadBlob(new Blob([`﻿${csv}`], { type: "text/csv;charset=utf-8" }), `ygate-calculated-report-${stamp}.csv`);
```
Hide the export button if the user lacks the report’s existing read permission. Remove the XLSX report-type selector and `/api/v1/reports/export` call from this view; do not remove the backend endpoint in this change.

- [ ] **Step 5: Run verification**

Run: `npm test; npm run typecheck; npm run build` from `apps/web`.

- [ ] **Step 6: Commit**

```bash
git add apps/web/app/features/reports/reports-page.tsx apps/web/app/lib/calculated-report-csv.ts apps/web/app/lib/calculated-report-csv.test.ts
git commit -m "feat(reports): export calculated multi-device CSV"
```

### Task 6: Repository-wide permission-control audit

**Files:**
- Modify: every `apps/web/app/features/**` file with mutation controls not covered in Tasks 3 or 5.
- Test: extend relevant existing library/page tests.

**Interfaces:**
- Consumes: shared `can` helper from Task 1.
- Produces: no create/update/delete/assign/status mutation control is visible to read-only users on any page.

- [ ] **Step 1: Inventory mutation buttons**

Run: `rg -n 'method: "(POST|PUT|PATCH|DELETE)"|onClick=.*(setEditor|delete|save|update|create)' apps/web/app/features` and map each control to its server permission.

- [ ] **Step 2: Add focused tests for any new predicate**

For each resource action absent from prior tests, add a `can` assertion such as `assert.equal(can(viewer, "alarm", "delete"), false)`.

- [ ] **Step 3: Gate each discovered control**

Import `usePlatformSession` and `can` where needed. Render mutation actions only when the corresponding permission exists; retain read controls and disabled state for resource-specific rules.

- [ ] **Step 4: Run full verification**

Run: `npm test; npm run typecheck; npm run build` from `apps/web`, then `go test ./...` from `services/auth-service` and `services/platform-api`.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features
git commit -m "fix(web): hide unauthorized management controls"
```

## Plan Self-Review

- Spec coverage: Tasks 1, 3, and 6 cover all-page action visibility; Task 2 covers Verified and profile roles; Task 3 covers role and plant organization scope; Task 4 covers Analytics loading; Task 5 covers calculated single-file multi-selection CSV.
- No placeholders: commands, file targets, interfaces, and expected checks are specified for every task.
- Type consistency: permissions use `resource:action`; user verification uses `emailVerifiedAt`; profile roles use `roles`; CSV exports named calculated records.


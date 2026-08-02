# RBAC Nav/Route Guard, Site Branding Fix, PrimeReact Migration — apps/web + platform-api

Status: Approved (design), pending implementation plan
Date: 2026-08-02

## Context

`apps/web` (Next.js) + `services/platform-api` (Go) already have a full RBAC schema: `permission`, `role`, `role_permission`, `user_role` tables, 40+ permissions across `plant`, `device`, `device_model`, `user`, `role`, `scada_screen`, `alarm`, `dashboard`, `middleware_client`, `middleware_config`, `middleware_patch`, `audit`, `site_setting`, `api_contract`, and 7 system roles (Platform Admin, Organization Admin, Plant Manager, Engineer, Operator, Viewer, Auditor) with default grants (migration `000005_rbac_registry.sql` and later). Backend mutation endpoints already enforce these via `requireGlobalPermission` / `requireOrganizationPermission` / `HasUserPermission` — a user without a grant gets `403`.

The gap is entirely on the frontend: `/api/v1/auth/me` returns a `LoginUser` with no role/permission information, so `apps/web/app/components/platform-shell.tsx` renders every sidebar item to every logged-in user regardless of what they're actually allowed to do, and there is no guard stopping direct navigation to a URL the user has no permission for.

Site Branding (`/settings`, backend `internal/httpapi/site_settings.go` + `internal/core/site_settings.go` + migration `000024_site_branding.sql`) was previously reported broken; the user has since confirmed saving works, but the page's buttons/inputs look visually broken relative to the rest of the app.

Separately, the user wants the whole app's interactive primitives (buttons, dialogs, selects, tooltips, popovers, tabs, toasts) rebuilt on PrimeReact. This directly supersedes the component-library decision in `docs/superpowers/specs/2026-07-31-design-system-redesign.md`, which chose shadcn/ui + Radix and already shipped `Dialog`, `Select`, `Tooltip`, `Popover`, `Tabs`, `Sonner` wrappers under `app/components/ui/`. This spec amends that decision; the design tokens section of the 2026-07-31 spec (colors, spacing, radius, shadow, typography) is unaffected and stays authoritative.

This is one combined spec/plan per explicit user instruction, covering three tracks that land together:
1. Backend: expose the caller's permission set.
2. Frontend: nav filtering + route guard driven by that permission set.
3. Frontend: replace shadcn/Radix primitives (and hand-rolled buttons) with PrimeReact, restyled to match the existing navy/teal token set — this is what fixes the Site Branding page's visual inconsistency (its buttons/inputs get the same primitives as everywhere else).

## Decisions

- **Permission exposure**: add `Permissions []string` (format `"resource_type:action"`, e.g. `"plant:read"`) to `auth.LoginUser`, computed via a new query joining `user_role` → `role_permission` (global or matching org) → `permission`, distinct on `(resource_type, action)`. Returned by both `/api/v1/auth/login` and `/api/v1/auth/me` (same struct, no new endpoint).
- **Nav/route gating granularity**: page-level only (does the user have the resource:action anywhere in their scope), not plant-level. Existing per-plant/per-org enforcement in backend read/write handlers is unchanged and remains the real authority; the frontend check is advisory (hide + redirect), not a security boundary.
- **Gating enforcement**: both nav-item visibility and a route guard that redirects away from a disallowed pathname (covers direct URL entry), implemented once in `PlatformShell`, not per-page.
- **Role defaults**: unchanged — use the 7 existing system roles' existing grants as-is.
- **Site Branding**: no backend logic changes; the routes, handlers, service methods, and migration are already correct and confirmed working. The fix is purely visual, delivered by the PrimeReact primitive swap in track 3.
- **UI library**: PrimeReact, unstyled mode + Tailwind passthrough (not a prebuilt PrimeReact theme) so the existing navy/teal token set (`--brand`, `--ink`, `--surface`, `--radius-md`, etc. from `globals.css`) stays the single source of visual truth. This avoids running two competing theme systems.
- **Migration shape**: keep the existing wrapper file names/paths under `app/components/ui/` and their existing exported API where the API is a plain component (`Dialog`, `Select`, `Tooltip`, `Popover`, `Tabs`) — swap only the implementation inside each wrapper from Radix to PrimeReact, so call sites across ~15 feature pages don't need to change. For `sonner.tsx`, keep the imperative `toast.success(...)` / `toast.error(...)` module-level API call sites already use, backed internally by a PrimeReact `Toast` + module-level ref. Add one new primitive, `Button`, which does not exist today (buttons are currently hand-rolled CSS classes).
- **Dependencies**: remove `@radix-ui/react-dialog`, `@radix-ui/react-popover`, `@radix-ui/react-select`, `@radix-ui/react-tabs`, `@radix-ui/react-tooltip`, `sonner`; add `primereact`, `primeicons`.
- **Scope of the Button rollout**: replace `.primary-button` / `.secondary-button` / `.icon-button` / `.text-button` usages across all ~13 files that use them with the new `Button` primitive. Table/row/section CSS (`.plant-row`, `.device-row`, etc.) is explicitly out of scope, per the untouched list in the 2026-07-31 spec.

## Backend changes (platform-api)

- New query (raw SQL or a `sqlc` addition alongside existing `HasUserPermission`/`HasOrganizationPermission` in `internal/database/dbgen`): distinct `(resource_type, action)` pairs reachable by a user through any `user_role` row, joining `role_permission` where `role_permission.organization_id IS NULL OR role_permission.organization_id = user_role.organization_id`.
- `auth.Principal` gains a method (or the login/me handlers call the service directly) to produce `Permissions []string` formatted `"resource_type:action"`, attached to `LoginUser`.
- No change to any existing `requireGlobalPermission` / `requireOrganizationPermission` / `HasUserPermission` call — those remain the actual authorization boundary.

## Frontend changes (apps/web)

### Types

`apps/web/app/lib/types.ts`: `User` gains `permissions: string[]`.

### Nav/route gating

New tiny helper, `apps/web/app/lib/permissions.ts`: `hasPermission(user: User, requirement: string): boolean` — `requirement` is `"resource_type:action"`; checks `user.permissions.includes(requirement)`.

`platform-shell.tsx` nav item table gets an optional `requires?: string` per item:

| Route | Requires | Notes |
|---|---|---|
| `/`, `/profile`, `/sessions` | — | always visible |
| `/site-map`, `/plants` | `plant:read` | |
| `/scada/live` | `scada_screen:view` | |
| `/alarms` | `alarm:read` | |
| `/register-metadata` | `device_model:read` | |
| `/scada` | `scada_screen:edit` | SCADA Builder |
| `/users` | `user:read` | |
| `/roles` | `role:read` | |
| `/middlewares` | `middleware_client:read` | |
| `/openapi` | `api_contract:read` | permission already exists in the table, previously unused by any route |
| `/audit` | `audit:read` | |
| `/settings` | `site_setting:update` | edit-only page, no separate read permission exists |

Nav groups whose items are all filtered out are omitted entirely (no empty group headers).

Route guard: in `PlatformShell`, once `user` is loaded, look up the current `pathname` (exact match, same as the existing `titles` lookup a few lines below it) against the table above; if it has a `requires` entry the user doesn't satisfy, redirect to `/` via `next/navigation`'s `useRouter().replace("/")`. No dedicated "403" page — redirect is silent, consistent with hiding the nav item in the first place.

### PrimeReact migration

- `package.json`: remove the five `@radix-ui/*` packages listed above and `sonner`; add `primereact`, `primeicons`.
- `app/lib/primereact-pt.ts` (new): PrimeReact passthrough config mapping component parts to Tailwind classes built from the existing CSS custom properties (e.g. button root → `bg-[var(--brand)] text-white rounded-[var(--radius-md)] shadow-[var(--shadow-sm)]`), imported once and passed via `PrimeReactProvider` at the app root (`app/layout.tsx`).
- Rewrite in place (same exported component names/props shape used by call sites today):
  - `app/components/ui/dialog.tsx` → PrimeReact `Dialog`
  - `app/components/ui/select.tsx` → PrimeReact `Dropdown`
  - `app/components/ui/tooltip.tsx` → PrimeReact `Tooltip`
  - `app/components/ui/popover.tsx` → PrimeReact `OverlayPanel`
  - `app/components/ui/tabs.tsx` → PrimeReact `TabView`
  - `app/components/ui/sonner.tsx` → PrimeReact `Toast`, keeping the `toast.success()/toast.error()` call-site API via a module-level `Toast` ref
- New `app/components/ui/button.tsx` wrapping PrimeReact `Button`; replace `.primary-button` / `.secondary-button` / `.icon-button` / `.text-button` JSX across the ~13 files that use them.
- `globals.css`: remove the CSS rules for the classes fully superseded by the above (`.primary-button`, `.secondary-button`, `.icon-button`, `.text-button`, `.modal-backdrop` and related dialog chrome, native `<select>` styling superseded by `Dropdown`). Table/row/section/form-message CSS is untouched.
- Amend `docs/superpowers/specs/2026-07-31-design-system-redesign.md`'s "Component approach" section to point at this spec for the current library choice (PrimeReact, not shadcn/Radix); its design tokens section stays authoritative and unchanged.

## Testing / validation

- Backend: extend existing auth/session integration tests (`internal/httpapi/server_test.go` or equivalent) with a case asserting `/api/v1/auth/me` includes the expected `permissions` entries for a known seeded role.
- Frontend: manual verification (no existing component test harness in `apps/web`) — log in as a low-privilege role (e.g. Viewer) and confirm the sidebar only shows permitted items, and that pasting a disallowed URL redirects to `/`. Re-verify Site Branding page save/upload/remove-logo flows visually after the PrimeReact swap.
- `go build ./...` and `npx tsc --noEmit` must stay clean (both already verified clean on the pre-change codebase during design).

## Out of scope

- Changing any system role's default permission grants.
- Any backend authorization logic change — this spec only adds a read-only permission-list projection.
- SCADA feature/capability expansion (already called out as separate in the 2026-07-31 spec).
- A dedicated "access denied" page — redirect-to-home is the whole UX for a disallowed route.

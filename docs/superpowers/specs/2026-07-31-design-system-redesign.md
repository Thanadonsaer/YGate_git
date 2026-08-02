# Design System Redesign — apps/web

Status: Approved (design), pending implementation plan
Date: 2026-07-31

## Context

ygate's `apps/web` is a Next.js admin/SCADA product for solar plant monitoring: auth, dashboard, plants/devices, middleware gateways, device model register metadata, users/roles/API keys, audit log, sessions, site map (Leaflet), and a SCADA screen editor/viewer (`@xyflow/react`). Styling today is hand-rolled Tailwind + a single `apps/web/app/globals.css` with per-feature classes (`.plant-row`, `.device-row`, `.api-key-table`, `.modal-backdrop`, `.section-heading`, etc.), one typeface (Inter), a navy/teal palette (`--nav: #122735`, `--action: #174f68`), no component library.

The user wants the whole product restyled to feel like a polished commercial SaaS product, with SCADA specifically called out as needing more capability later. This spec covers **only the design system / visual redesign**. SCADA functional/capability expansion (new node types, new interactions) is an explicitly separate follow-up sub-project — out of scope here. This spec restyles the SCADA *editor/viewer* visually (dark canvas) but does not add SCADA features.

This is a whole-project visual redesign, decomposed per `superpowers:brainstorming` into two independent tracks:
1. **This spec** — design system (tokens, component primitives, layout refresh) applied across all existing pages.
2. **Future spec** — SCADA feature/capability expansion (not started).

## Decisions (confirmed with user during brainstorming)

- **Component library**: adopt shadcn/ui + Radix primitives, targeted — not a full rebuild. Replace only the hand-rolled interactive primitives that repeat across pages and currently lack real accessibility (focus trap, ESC, ARIA): `Dialog` (replaces `.modal-backdrop` pattern, used in ~14 places), `Select` (replaces native `<select>` styling in editors), `Tooltip`/`Popover` (currently absent anywhere in the app), `Tabs` (replaces the custom `.mode-switch` in the SCADA titlebar), `Sonner`/`Toast` (new — for fire-and-forget actions like Test Connection/Test Read/Save config that currently have no feedback).
- **Not replaced**: all existing table/row layouts (`plant-row`, `device-row`, `user-row`, `api-key-row`, `audit-row`, `session-row`, `role-row`, `metadata-row`) keep their current grid-column structure — only their tokens (color/spacing/radius/shadow) are refreshed. Inline `.form-message` success/error banners stay as-is.
- **Theme**: hybrid — light theme for the admin shell (everything except the SCADA workbench), dark theme scoped only to the SCADA screen editor/viewer (`.scada-workbench`, `.scada-palette`, `.scada-inspector`, `.scada-stage-shell`), via a `.scada-dark` scoping class, not a global dark-mode toggle.
- **Rollout scope**: apply to all ~15 existing page/feature areas in this one spec (auth, sidebar/topbar shell, dashboard, plants/devices, middlewares, register-metadata, users, roles, api-keys, audit, sessions, site-map, SCADA shell chrome).
- **Brand base**: evolve the existing navy/teal identity rather than replace it — it is already distinctive (not a generic AI-default look), just needs refinement (softer radius, real shadows, secondary typeface, a signature motif).

## Design tokens

### Color — light shell (admin pages)

| Token | Hex | Usage |
|---|---|---|
| `--ink` | `#0F1B26` | primary text |
| `--ink-soft` | `#4A5A66` | secondary/muted text |
| `--surface` | `#FFFFFF` | card/table/modal background |
| `--canvas` | `#F1F4F6` | page background |
| `--line` | `#DCE3E7` | borders/dividers |
| `--brand` | `#0E5C73` | primary action color (evolved from current `#174f68`) |
| `--accent` | `#E8A33D` | signature/energy accent — used sparingly (Live Pulse motif, not general buttons) |
| `--success` | `#16825C` | positive status |
| `--warning` | `#B9770E` | warning status |
| `--danger` | `#C0392B` | error/destructive status |
| `--focus` | `#1C8FB0` | focus ring |

### Color — dark (SCADA canvas only, scoped under `.scada-dark`)

| Token | Hex | Usage |
|---|---|---|
| `--scada-bg` | `#0B141C` | canvas/stage background |
| `--scada-surface` | `#121F2B` | node cards, palette, inspector panels |
| `--scada-line` | `#223140` | borders |
| `--scada-ink` | `#E7EDF1` | text on dark |
| `--scada-muted` | `#7C93A3` | secondary text on dark |
| `--scada-accent` | `#2FB8D9` | selected node, edges/wires, live indicator |

Status colors (success/warning/danger) reuse the light-theme hues at increased luminance for legibility on dark backgrounds — no separate named tokens, computed via `color-mix` at build/CSS time same as the current codebase's existing pattern (see `globals.css` usage of `color-mix(in srgb, ...)`).

### Typography — 3 roles

- **Display** — `Manrope`. Headings, section titles, sidebar brand mark, KPI figures' label. Used with restraint (headings only, never body text).
- **Body/UI** — `Inter` (kept from current implementation — already proven in dense tables/forms). Body text, form labels, buttons, table cell text.
- **Data/mono** — `JetBrains Mono`. Numeric readouts, IP addresses, Modbus register keys/addresses, timestamps, log/JSON viewers (`.audit-json pre`, `.openapi-viewer`), SCADA node metric values. Replaces the current ad hoc `Consolas, monospace` fallback used in a few spots (`.activity-band p`, `.secret-panel code`) with a single deliberate, loaded webfont.

Fonts are loaded via `next/font` (already how Next.js in this repo would load webfonts — no separate font-loading dependency needed).

### Spacing, radius, shadow

- Spacing: unchanged, keep existing 4px-based Tailwind scale.
- Radius: bump from current 5–7px to a 3-step scale — `--radius-sm: 8px` (inputs, chips), `--radius-md: 10px` (cards, buttons), `--radius-lg: 12px` (modals/dialogs).
- Shadow: introduce 2 elevation levels not present today — `--shadow-sm` (row hover, card rest state), `--shadow-lg` (modal/dropdown/popover). SCADA dark canvas uses glow instead of shadow for selected nodes (`box-shadow: 0 0 0 1px var(--scada-line), 0 0 12px rgb(47 184 217 / 8%)`).

## Component approach

Install `shadcn/ui` (Radix-based, already compatible with the existing Tailwind v4 setup — no conflicting UI dependency in `package.json` today). Generate only these primitives: `dialog`, `select`, `tooltip`, `popover`, `tabs`, `sonner`. Each replaces one existing hand-rolled pattern in place — same call sites, no structural page rewrites:

- `.modal-backdrop` + editor panels (`plant-editor`, `middleware-editor`, `role-editor`, `user-editor`, `api-key-editor`, `metadata-editor`) → shadcn `Dialog`, keeping each editor's existing form JSX/fields, only the backdrop/panel/close-button chrome changes.
- Native `<select>` in editors → shadcn `Select`.
- SCADA inspector's binding pickers and any table action needing an explanatory hint → shadcn `Tooltip`/`Popover` (net-new capability, not a replacement).
- `.mode-switch` (SCADA titlebar view/edit toggle) → shadcn `Tabs`.
- Fire-and-forget actions with no current feedback (Test Connection, Test Read, Save Middleware Config) → shadcn `Sonner` toast.

Everything else (tables, rows, dashboard widgets, sidebar, topbar, forms) keeps its current DOM/class structure, restyled via the new tokens above.

**Superseded 2026-08-02:** the component library choice above (shadcn/ui + Radix) was replaced with PrimeReact — see `docs/superpowers/specs/2026-08-02-rbac-nav-guard-and-primereact-design.md`. The design tokens sections above (color, typography, spacing/radius/shadow) are unaffected and remain authoritative.

## Signature element — "Live Pulse"

A small animated waveform (inline SVG + CSS animation, no new dependency), directly derived from the product's subject matter (electrical/telemetry waveforms read over Modbus), not decorative:

- **Topbar live chip**: replaces the current static dot (`.live-chip`/`.dot`) with a dot + waveform line that visually reflects the real `useRealtimeSocket` connection state — smooth continuous motion when `connected`, a jittering/irregular line when `connecting`, a flat line when `offline`. Driven by existing connection-state data, not a fake decorative loop.
- **Section heading underline**: replaces the current plain bottom border under `.section-heading` with a subtle pulse-line accent.
- **Auth screen**: replaces the current static dark overlay (`.auth-overlay`) with a faint pulse line animated across the background.
- **Loading screen**: replaces the current opacity-pulse spinner animation with the waveform motif, consistent with the rest of the motif's usage.

## Key screens (wireframe intent)

- **Sidebar/topbar (light)**: unchanged structure (248px sidebar + topbar), Manrope for `h1`/nav labels, pulse motif in the live chip, refined spacing/radius per tokens above.
- **Dashboard KPI card**: unchanged grid position, refined to `--radius-md` + `--shadow-sm`, headline figure in Manrope, secondary figure in JetBrains Mono, thin accent progress bar.
- **SCADA workbench**: `.scada-workbench`, `.scada-palette`, `.scada-inspector`, `.scada-stage-shell` restyled under `.scada-dark` scope — dark background, `--scada-accent` for selected nodes and edges (`react-flow__edge-path`), palette/inspector inputs restyled as dark-variant shadcn components.

## Explicitly out of scope

- New SCADA node types, new SCADA interactivity/capability (separate future sub-project).
- Any backend/API change — this is `apps/web` styling and component-primitive work only.
- Global dark mode toggle for the admin shell (dark is SCADA-canvas-only, scoped, not user-toggleable elsewhere).
- Rewriting table/row DOM structure or grid-column layouts — tokens and primitives only.

## Verification

- `npx tsc --noEmit` and `npx next build` clean after each page area is migrated.
- Manual pass per page: dialog/select/tooltip/tabs interactions keyboard-accessible (Tab, Esc, Enter), visible focus ring on all interactive elements, responsive breakpoints (`max-width: 1100px`, `max-width: 520px`) still work after token/component swap.
- SCADA dark canvas: confirm edges/selected-node styling legible against `--scada-bg`, confirm palette/inspector remain usable (contrast) in dark mode.
- Live Pulse chip: confirm it reflects real connection state transitions (connected/connecting/offline) via `useRealtimeSocket`, not a static loop.

# Design System Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle every page in `apps/web` with a refreshed token system (color/typography/spacing/radius/shadow), targeted shadcn/Radix primitives (Dialog, Select, Tooltip, Popover, Tabs, Sonner) replacing hand-rolled equivalents, a dark-scoped SCADA canvas, and a "Live Pulse" signature motif — without rewriting table/row layouts or touching any backend.

**Architecture:** Tailwind v4 CSS-first tokens (`:root` custom properties + `@theme inline` mapping) in `apps/web/app/globals.css` are the single source of truth for color/type/radius/shadow, consumed both by existing hand-rolled CSS classes (`.plant-row`, `.device-row`, etc. — via `var(--token)`) and by new Tailwind utility classes (`bg-surface`, `text-ink`, `border-line` — via the `@theme inline` mapping) so both styling systems already present in the codebase converge on one palette. New shadcn-style Radix primitives are hand-written (not CLI-generated, to keep execution scriptable/non-interactive) under `apps/web/app/components/ui/`.

**Tech Stack:** Next.js 16 (App Router), React 19, Tailwind v4 (CSS-first, no `tailwind.config.js`), Radix UI primitives, `sonner` for toasts, `next/font` for Manrope/Inter/JetBrains Mono, `class-variance-authority` + `clsx` + `tailwind-merge` for the `cn()` helper. No new backend/API work.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-31-design-system-redesign.md` — every task here traces to a section of that spec.
- `apps/web` has **no test runner** (no jest/vitest/playwright in `package.json`) and this is a styling/component-primitive migration, not new business logic — so the skill's default "write failing test → implement → pass" cycle does not apply. Every task instead ends with: `npx tsc --noEmit` (from `apps/web`) clean, `npx next build` clean (only re-run build at the end of each numbered phase, not every single task, to keep iteration fast — `tsc --noEmit` alone is enough per-task signal), and a concrete manual-check description. This substitution applies to every task below; it is not repeated per task.
- Do not rename existing CSS custom properties still referenced by hand-rolled classes (`--ink`, `--muted`, `--line`, `--surface`, `--canvas`, `--nav`, `--nav-active`, `--action`, `--success`, `--warning`, `--danger`, `--focus`) — only change their hex values and add new ones (`--energy`, `--radius-*`, `--shadow-*`, `--scada-*`). This keeps every page that isn't explicitly migrated in this plan visually consistent for free, with zero code changes to that page.
- New Tailwind utility color classes (`bg-surface`, `text-ink`, etc.) are defined via `@theme inline` referencing the existing `:root` custom properties — never hardcode a hex value in a new Tailwind class or a new component; always go through a token.
- Do not touch any file under `services/`, `modbus-api-middleware/`, or `packages/api-contracts/` — this plan is `apps/web` only.
- All new npm dependencies are installed directly in `apps/web` (`cd apps/web && npm install ...`) — there is no root `package.json`/workspace, confirmed during planning.
- Every task that adds a dependency lists the exact `npm install` command; every task that touches `globals.css` or a shared primitive must keep every previously-migrated page visually working (check by re-reading, not by guessing).
- From Task 13 onward, a task may say "same `Dialog`/`DialogContent`/`DialogHeader`/`DialogBody` wrapper pattern as Task 12" — this refers ONLY to the literal, byte-identical 6-line JSX skeleton (open `Dialog`, `DialogContent`, `DialogHeader` with a `DialogDescription`+`DialogTitle` pair, `DialogBody` wrapping the existing `<form>`). Every task still spells out, in full, every value that differs: the exact `DialogDescription`/`DialogTitle` text, which `<select>`s become `Select` and their exact option lists, and any other content change. Nothing about business logic or unique markup is ever elided this way — only the repeated wrapper shell.

---

## Phase 1 — Foundation (tokens, primitives, signature element)

### Task 1: Path alias + dependencies

**Files:**
- Modify: `apps/web/tsconfig.json`
- Modify: `apps/web/package.json`

**Interfaces:**
- Produces: `@/*` import alias resolving to `apps/web/app/*`, used by every task after this one for `app/components/ui/*` and `app/lib/cn`.

- [ ] **Step 1: Add the path alias**

In `apps/web/tsconfig.json`, add `baseUrl` and `paths` to `compilerOptions` (after `"moduleResolution": "bundler",`):

```json
    "moduleResolution": "bundler",
    "baseUrl": ".",
    "paths": {
      "@/*": ["./app/*"]
    },
```

- [ ] **Step 2: Install dependencies**

```bash
cd apps/web
npm install @radix-ui/react-dialog @radix-ui/react-select @radix-ui/react-tooltip @radix-ui/react-popover @radix-ui/react-tabs sonner clsx tailwind-merge class-variance-authority tw-animate-css
```

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: no errors (alias unused so far, but must resolve — create a throwaway `app/lib/cn.ts` in Task 2 to prove it).

- [ ] **Step 4: Commit**

```bash
git add apps/web/tsconfig.json apps/web/package.json apps/web/package-lock.json
git commit -m "$(cat <<'EOF'
Add @/* path alias and shadcn/Radix dependencies

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Design tokens — color, type, radius, shadow

**Files:**
- Modify: `apps/web/app/globals.css`
- Modify: `apps/web/app/layout.tsx`
- Create: `apps/web/app/lib/cn.ts`

**Interfaces:**
- Produces: `cn(...classes: ClassValue[]) => string` from `app/lib/cn.ts` — consumed by every primitive in Phase 1.
- Produces: CSS custom properties (`--ink`, `--muted`, `--line`, `--surface`, `--canvas`, `--nav`, `--nav-active`, `--action`, `--success`, `--warning`, `--danger`, `--focus`, `--energy`, `--radius-sm`, `--radius-md`, `--radius-lg`, `--shadow-sm`, `--shadow-lg`, `--scada-bg`, `--scada-surface`, `--scada-line`, `--scada-ink`, `--scada-muted`, `--scada-accent`) and Tailwind utility classes (`bg-surface`, `text-ink`, `text-ink-soft`, `border-line`, `bg-canvas`, `bg-brand`, `text-brand`, `bg-energy`, `text-energy`, `bg-success`/`warning`/`danger`, `ring-focus`, `font-display`, `font-body`, `font-mono`, `rounded-sm|md|lg` (mapped to the new radius scale), `shadow-sm|lg` (mapped to the new shadow scale)) — consumed by every task after this one.

- [ ] **Step 1: Create the `cn` helper**

```typescript
// apps/web/app/lib/cn.ts
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

- [ ] **Step 2: Load Manrope, Inter, JetBrains Mono via `next/font`**

Replace `apps/web/app/layout.tsx` in full:

```tsx
import type { Metadata } from "next";
import { Inter, JetBrains_Mono, Manrope } from "next/font/google";
import "react-grid-layout/css/styles.css";
import "react-resizable/css/styles.css";
import "./globals.css";
import "@xyflow/react/dist/style.css";
import "leaflet/dist/leaflet.css";

const manrope = Manrope({ subsets: ["latin"], weight: ["600", "700", "800"], variable: "--font-manrope" });
const inter = Inter({ subsets: ["latin"], weight: ["400", "500", "600", "700"], variable: "--font-inter" });
const jetbrainsMono = JetBrains_Mono({ subsets: ["latin"], weight: ["400", "500", "700"], variable: "--font-jetbrains-mono" });

export const metadata: Metadata = {
  title: "YGATE Solar SCADA",
  description: "Solar plant monitoring platform",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="th" className={`${manrope.variable} ${inter.variable} ${jetbrainsMono.variable}`}>
      <body>{children}</body>
    </html>
  );
}
```

- [ ] **Step 3: Rewrite the token block in `globals.css`**

Replace lines 1–30 of `apps/web/app/globals.css` (the `@import` + `:root` block + base element rules) with:

```css
@import "tailwindcss";
@import "tw-animate-css";

:root {
  --ink: #0f1b26;
  --muted: #4a5a66;
  --line: #dce3e7;
  --surface: #ffffff;
  --canvas: #f1f4f6;
  --nav: #10202c;
  --nav-active: #e8edf0;
  --action: #0e5c73;
  --success: #16825c;
  --warning: #b9770e;
  --danger: #c0392b;
  --focus: #1c8fb0;
  --energy: #e8a33d;

  --radius-sm: 8px;
  --radius-md: 10px;
  --radius-lg: 12px;
  --shadow-sm: 0 1px 2px rgb(15 27 38 / 6%), 0 1px 1px rgb(15 27 38 / 4%);
  --shadow-lg: 0 24px 48px rgb(15 27 38 / 20%), 0 4px 12px rgb(15 27 38 / 8%);

  --scada-bg: #0b141c;
  --scada-surface: #121f2b;
  --scada-line: #223140;
  --scada-ink: #e7edf1;
  --scada-muted: #7c93a3;
  --scada-accent: #2fb8d9;
}

@theme inline {
  --color-ink: var(--ink);
  --color-ink-soft: var(--muted);
  --color-line: var(--line);
  --color-surface: var(--surface);
  --color-canvas: var(--canvas);
  --color-brand: var(--action);
  --color-success: var(--success);
  --color-warning: var(--warning);
  --color-danger: var(--danger);
  --color-focus: var(--focus);
  --color-energy: var(--energy);

  --color-scada-bg: var(--scada-bg);
  --color-scada-surface: var(--scada-surface);
  --color-scada-line: var(--scada-line);
  --color-scada-ink: var(--scada-ink);
  --color-scada-muted: var(--scada-muted);
  --color-scada-accent: var(--scada-accent);

  --radius-sm: var(--radius-sm);
  --radius-md: var(--radius-md);
  --radius-lg: var(--radius-lg);
  --shadow-sm: var(--shadow-sm);
  --shadow-lg: var(--shadow-lg);

  --font-display: var(--font-manrope), sans-serif;
  --font-body: var(--font-inter), "Segoe UI", Tahoma, sans-serif;
  --font-mono: var(--font-jetbrains-mono), Consolas, monospace;
}

* { box-sizing: border-box; }
html, body { min-height: 100%; }
body {
  margin: 0;
  color: var(--ink);
  background: var(--canvas);
  font-family: var(--font-body);
  letter-spacing: 0;
}
h1, h2, h3, .section-heading h2, .plant-editor h2, .scada-titlebar .registry-title input { font-family: var(--font-display); }
button, input, select, textarea { font: inherit; letter-spacing: 0; }
button { cursor: pointer; }
button:disabled { cursor: not-allowed; opacity: .48; }
button:focus-visible, a:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible { outline: 3px solid color-mix(in srgb, var(--focus) 35%, transparent); outline-offset: 2px; }
```

Do not touch anything from the old line 31 (`.loading-screen {`) onward yet — later tasks edit those sections individually.

- [ ] **Step 4: Update existing hardcoded-radius rules to use the new scale**

In `apps/web/app/globals.css`, run a plain find/replace across the whole file (not just the block above):
- `border-radius: 6px;` → `border-radius: var(--radius-sm);` (appears on `.auth-panel input`, `.primary-button`/`.secondary-button`/`.back-button`/`.text-button`, `.form-message`, `.icon-button`, `.dashboard-widget`)
- `border-radius: 7px;` → `border-radius: var(--radius-md);` (appears on `.plant-editor`)
- `border-radius: 5px;` → `border-radius: var(--radius-sm);` (appears on numerous input/select/row rules)

Use the Edit tool with `replace_all: true` for each of the three literal strings.

- [ ] **Step 5: Verify**

Run: `cd apps/web && npx tsc --noEmit && npx next build`
Expected: both clean. Load `/` in dev (`npm run dev`) and confirm the login screen and (after login) every existing page still render without console errors — colors will look nearly identical since token values are close to the originals, this step is a no-visual-regression check, not a new-look check.

- [ ] **Step 6: Commit**

```bash
git add apps/web/app/globals.css apps/web/app/layout.tsx apps/web/app/lib/cn.ts
git commit -m "$(cat <<'EOF'
Refresh design tokens: palette, Manrope/Inter/JetBrains Mono, radius/shadow scale

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Dialog primitive

**Files:**
- Create: `apps/web/app/components/ui/dialog.tsx`

**Interfaces:**
- Consumes: `cn` from `app/lib/cn.ts` (Task 2).
- Produces: `Dialog`, `DialogTrigger`, `DialogPortal`, `DialogClose`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogBody` — consumed by every page-migration task with a modal in Phase 2/3.

- [ ] **Step 1: Write the primitive**

```tsx
// apps/web/app/components/ui/dialog.tsx
"use client";

import * as React from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "../../lib/cn";

function Dialog(props: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />;
}

function DialogPortal(props: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />;
}

function DialogClose(props: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />;
}

function DialogOverlay({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-ink/55 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
        className,
      )}
      {...props}
    />
  );
}

function DialogContent({
  className,
  children,
  showClose = true,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & { showClose?: boolean }) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        className={cn(
          "fixed left-1/2 top-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 -translate-y-1/2 gap-0 overflow-y-auto rounded-[var(--radius-lg)] border border-line bg-surface shadow-[var(--shadow-lg)] duration-200 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 max-h-[calc(100vh-2rem)]",
          className,
        )}
        {...props}
      >
        {children}
        {showClose && (
          <DialogPrimitive.Close
            className="absolute right-4 top-4 rounded-[var(--radius-sm)] p-1.5 text-ink-soft transition hover:bg-canvas focus:outline-none focus-visible:ring-2 focus-visible:ring-focus"
            aria-label="ปิด"
            title="ปิด"
          >
            <X size={18} />
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="dialog-header" className={cn("flex items-center justify-between gap-4 border-b border-line px-5 py-4", className)} {...props} />;
}

function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return <DialogPrimitive.Title data-slot="dialog-title" className={cn("font-display text-xl font-extrabold text-ink", className)} {...props} />;
}

function DialogDescription({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return <DialogPrimitive.Description data-slot="dialog-description" className={cn("text-xs font-extrabold uppercase text-ink-soft", className)} {...props} />;
}

function DialogBody({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="dialog-body" className={cn("p-5", className)} {...props} />;
}

export { Dialog, DialogPortal, DialogClose, DialogOverlay, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody };
```

- [ ] **Step 2: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean (nothing imports this yet, but it must typecheck standalone).

- [ ] **Step 3: Commit**

```bash
git add apps/web/app/components/ui/dialog.tsx
git commit -m "$(cat <<'EOF'
Add Dialog primitive (Radix) for the design system

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Select primitive

**Files:**
- Create: `apps/web/app/components/ui/select.tsx`

**Interfaces:**
- Consumes: `cn` from `app/lib/cn.ts`.
- Produces: `Select`, `SelectTrigger`, `SelectValue`, `SelectContent`, `SelectItem` — consumed by every page-migration task that has a native `<select>`.

- [ ] **Step 1: Write the primitive**

```tsx
// apps/web/app/components/ui/select.tsx
"use client";

import * as React from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown } from "lucide-react";
import { cn } from "../../lib/cn";

function Select(props: React.ComponentProps<typeof SelectPrimitive.Root>) {
  return <SelectPrimitive.Root data-slot="select" {...props} />;
}

function SelectValue(props: React.ComponentProps<typeof SelectPrimitive.Value>) {
  return <SelectPrimitive.Value data-slot="select-value" {...props} />;
}

function SelectTrigger({ className, children, ...props }: React.ComponentProps<typeof SelectPrimitive.Trigger>) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      className={cn(
        "flex h-10 w-full items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-sm text-ink outline-none transition focus:border-focus focus:ring-4 focus:ring-focus/15 disabled:cursor-not-allowed disabled:opacity-48 data-[placeholder]:text-ink-soft",
        className,
      )}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        <ChevronDown size={16} className="shrink-0 text-ink-soft" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  );
}

function SelectContent({ className, children, position = "popper", ...props }: React.ComponentProps<typeof SelectPrimitive.Content>) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        data-slot="select-content"
        position={position}
        className={cn(
          "relative z-50 max-h-96 min-w-[8rem] overflow-y-auto rounded-[var(--radius-sm)] border border-line bg-surface shadow-[var(--shadow-lg)] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
          position === "popper" && "w-[var(--radix-select-trigger-width)]",
          className,
        )}
        {...props}
      >
        <SelectPrimitive.Viewport className="p-1">{children}</SelectPrimitive.Viewport>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  );
}

function SelectItem({ className, children, ...props }: React.ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      className={cn(
        "relative flex w-full cursor-pointer select-none items-center gap-2 rounded-[var(--radius-sm)] py-2 pl-8 pr-2 text-sm text-ink outline-none data-[highlighted]:bg-canvas data-[disabled]:pointer-events-none data-[disabled]:opacity-48",
        className,
      )}
      {...props}
    >
      <span className="absolute left-2 flex size-3.5 items-center justify-center">
        <SelectPrimitive.ItemIndicator>
          <Check size={14} className="text-brand" />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  );
}

export { Select, SelectValue, SelectTrigger, SelectContent, SelectItem };
```

- [ ] **Step 2: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add apps/web/app/components/ui/select.tsx
git commit -m "$(cat <<'EOF'
Add Select primitive (Radix) for the design system

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Tooltip + Popover primitives

**Files:**
- Create: `apps/web/app/components/ui/tooltip.tsx`
- Create: `apps/web/app/components/ui/popover.tsx`

**Interfaces:**
- Produces: `TooltipProvider`, `Tooltip`, `TooltipTrigger`, `TooltipContent` — net-new capability, first consumed in the SCADA inspector task (Phase 3).
- Produces: `Popover`, `PopoverTrigger`, `PopoverContent` — net-new capability, first consumed in the SCADA inspector task (Phase 3).

- [ ] **Step 1: Write the Tooltip primitive**

```tsx
// apps/web/app/components/ui/tooltip.tsx
"use client";

import * as React from "react";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { cn } from "../../lib/cn";

function TooltipProvider({ delayDuration = 200, ...props }: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return <TooltipPrimitive.Provider data-slot="tooltip-provider" delayDuration={delayDuration} {...props} />;
}

function Tooltip(props: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return (
    <TooltipProvider>
      <TooltipPrimitive.Root data-slot="tooltip" {...props} />
    </TooltipProvider>
  );
}

function TooltipTrigger(props: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

function TooltipContent({ className, sideOffset = 6, children, ...props }: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          "z-50 max-w-64 rounded-[var(--radius-sm)] bg-ink px-2.5 py-1.5 text-xs font-semibold text-surface shadow-[var(--shadow-sm)] data-[state=delayed-open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:fade-in-0",
          className,
        )}
        {...props}
      >
        {children}
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  );
}

export { TooltipProvider, Tooltip, TooltipTrigger, TooltipContent };
```

- [ ] **Step 2: Write the Popover primitive**

```tsx
// apps/web/app/components/ui/popover.tsx
"use client";

import * as React from "react";
import * as PopoverPrimitive from "@radix-ui/react-popover";
import { cn } from "../../lib/cn";

function Popover(props: React.ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />;
}

function PopoverTrigger(props: React.ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />;
}

function PopoverContent({ className, align = "center", sideOffset = 6, ...props }: React.ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        className={cn(
          "z-50 w-72 rounded-[var(--radius-md)] border border-line bg-surface p-4 text-sm text-ink shadow-[var(--shadow-lg)] outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  );
}

export { Popover, PopoverTrigger, PopoverContent };
```

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/ui/tooltip.tsx apps/web/app/components/ui/popover.tsx
git commit -m "$(cat <<'EOF'
Add Tooltip and Popover primitives (Radix) for the design system

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Tabs primitive

**Files:**
- Create: `apps/web/app/components/ui/tabs.tsx`

**Interfaces:**
- Produces: `Tabs`, `TabsList`, `TabsTrigger`, `TabsContent` — consumed by the SCADA mode-switch migration (Phase 3) and the Alarms log/rules mode-switch (Phase 2).

- [ ] **Step 1: Write the primitive**

```tsx
// apps/web/app/components/ui/tabs.tsx
"use client";

import * as React from "react";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { cn } from "../../lib/cn";

function Tabs({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Root>) {
  return <TabsPrimitive.Root data-slot="tabs" className={cn("flex flex-col gap-2", className)} {...props} />;
}

function TabsList({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.List>) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      className={cn("inline-flex h-10 items-center gap-0.5 rounded-[var(--radius-sm)] border border-line bg-canvas p-1", className)}
      {...props}
    />
  );
}

function TabsTrigger({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        "inline-flex h-[30px] items-center gap-1.5 rounded-[calc(var(--radius-sm)-2px)] px-3 text-xs font-bold text-ink-soft transition data-[state=active]:bg-surface data-[state=active]:text-nav data-[state=active]:shadow-[var(--shadow-sm)]",
        className,
      )}
      {...props}
    />
  );
}

function TabsContent(props: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return <TabsPrimitive.Content data-slot="tabs-content" {...props} />;
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
```

- [ ] **Step 2: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add apps/web/app/components/ui/tabs.tsx
git commit -m "$(cat <<'EOF'
Add Tabs primitive (Radix) for the design system

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Sonner (toast) primitive

**Files:**
- Create: `apps/web/app/components/ui/sonner.tsx`
- Modify: `apps/web/app/components/platform-shell.tsx`

**Interfaces:**
- Produces: `Toaster` component (mounted once in `PlatformShell`) and re-exports `toast` from `"sonner"` — consumed by the Test Connection/Test Read migration in the Plants task (Phase 2) and mutation-success toasts across CRUD pages.

- [ ] **Step 1: Write the primitive**

```tsx
// apps/web/app/components/ui/sonner.tsx
"use client";

import { Toaster as Sonner, type ToasterProps } from "sonner";

function Toaster(props: ToasterProps) {
  return (
    <Sonner
      theme="light"
      position="bottom-right"
      toastOptions={{
        classNames: {
          toast: "rounded-[var(--radius-md)] border border-line bg-surface text-ink shadow-[var(--shadow-lg)] font-body",
          title: "font-bold",
          description: "text-ink-soft",
          success: "border-success/30",
          error: "border-danger/30",
        },
      }}
      {...props}
    />
  );
}

export { Toaster };
export { toast } from "sonner";
```

- [ ] **Step 2: Mount `<Toaster />` once, in `PlatformShell`**

In `apps/web/app/components/platform-shell.tsx`, add the import near the top (after the `AuthScreen` import):

```tsx
import { AuthScreen } from "./auth-screen";
import { Toaster } from "./ui/sonner";
```

Then wrap the existing return so `<Toaster />` renders alongside the shell regardless of loading/auth state — replace the final `return` block (currently starting `return (\n    <PlatformSessionContext.Provider ...` at the end of the file) so it becomes:

```tsx
  return (
    <PlatformSessionContext.Provider value={{ user, liveState, lastLiveAt, updateCurrentUser: setUser }}>
      <Toaster />
      <main className="app-shell">
```

(only the two new lines — `<Toaster />` right after the `<PlatformSessionContext.Provider ...>` opening tag — everything else in that return block is unchanged; the closing tags already close `</main>` then `</PlatformSessionContext.Provider>`, which still balances since `<Toaster />` is self-closing).

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean. In dev, confirm the app still loads and logs in (Toaster renders nothing visible until a toast fires).

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/ui/sonner.tsx apps/web/app/components/platform-shell.tsx
git commit -m "$(cat <<'EOF'
Add Sonner toast primitive and mount Toaster in PlatformShell

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Live Pulse signature component

**Files:**
- Create: `apps/web/app/components/live-pulse.tsx`

**Interfaces:**
- Produces: `LivePulse({ state: "connected" | "connecting" | "offline", className?: string })` — an inline SVG waveform, animated via CSS only (no JS animation loop). Consumed by the platform-shell topbar task, the auth-screen task, the loading-screen task, and the section-heading underline (Phase 2).
- Consumes: nothing new — `state` is meant to be fed directly from the existing `ConnectionState` type (`app/lib/types.ts`) already returned by `useRealtimeSocket`.

- [ ] **Step 1: Write the component**

```tsx
// apps/web/app/components/live-pulse.tsx
"use client";

import type { ConnectionState } from "../lib/types";
import { cn } from "../lib/cn";

const PATHS: Record<ConnectionState, string> = {
  connected: "M0 12 L10 12 L15 3 L20 21 L25 6 L30 18 L35 12 L44 12 L49 3 L54 21 L59 6 L64 18 L69 12 L78 12 L83 3 L88 21 L93 6 L98 18 L103 12 L112 12",
  connecting: "M0 12 L10 12 L14 8 L18 17 L22 5 L26 19 L30 12 L34 15 L38 9 L44 12 L52 12 L56 6 L60 18 L64 12 L70 14 L76 12 L82 12 L86 4 L90 20 L94 12 L100 12 L112 12",
  offline: "M0 12 L112 12",
};

export function LivePulse({ state, className }: { state: ConnectionState; className?: string }) {
  return (
    <svg
      className={cn("live-pulse", `live-pulse-${state}`, className)}
      viewBox="0 0 112 24"
      width="56"
      height="12"
      role="img"
      aria-label={state === "connected" ? "เชื่อมต่อ live" : state === "connecting" ? "กำลังเชื่อมต่อ" : "ออฟไลน์"}
    >
      <path d={PATHS[state]} fill="none" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
```

- [ ] **Step 2: Add its CSS to `globals.css`**

Append to the end of `apps/web/app/globals.css`:

```css
.live-pulse path { stroke: currentColor; }
.live-pulse-connected path { stroke-dasharray: 300; stroke-dashoffset: 0; animation: live-pulse-scroll 2.2s linear infinite; }
.live-pulse-connecting path { stroke-dasharray: 300; stroke-dashoffset: 0; animation: live-pulse-scroll 0.9s linear infinite; opacity: .75; }
.live-pulse-offline path { opacity: .35; }
@keyframes live-pulse-scroll {
  from { stroke-dashoffset: 0; }
  to { stroke-dashoffset: -224; }
}
@media (prefers-reduced-motion: reduce) {
  .live-pulse-connected path, .live-pulse-connecting path { animation: none; }
}
```

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`
Expected: clean. This component has no consumers yet (wired in Task 9/10) so nothing renders it — this step only proves it compiles standalone.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/live-pulse.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Add Live Pulse signature waveform component

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Phase 2 — Shell, auth, dashboard, and CRUD pages (light shell)

### Task 9: Platform shell (sidebar/topbar)

**Files:**
- Modify: `apps/web/app/components/platform-shell.tsx`
- Modify: `apps/web/app/globals.css`

**Interfaces:**
- Consumes: `LivePulse` (Task 8).

- [ ] **Step 1: Replace the topbar live chip with Live Pulse**

In `apps/web/app/components/platform-shell.tsx`, add the import:

```tsx
import { LivePulse } from "./live-pulse";
```

Replace this block (currently):

```tsx
            <div className={`live-chip ${liveState}`}>
              {liveState === "connected" ? <Wifi size={15} /> : <WifiOff size={15} />}
              <span>{liveState === "connected" ? "Live" : liveState === "connecting" ? "Connecting" : "Offline"}</span>
            </div>
```

with:

```tsx
            <div className={`live-chip ${liveState}`}>
              <LivePulse state={liveState} />
              <span>{liveState === "connected" ? "Live" : liveState === "connecting" ? "Connecting" : "Offline"}</span>
            </div>
```

Remove the now-unused `Wifi, WifiOff` entries from the `lucide-react` import list at the top of the file (keep every other icon in that list unchanged).

- [ ] **Step 2: Loading screen uses Live Pulse instead of the pulsing sun icon**

Replace:

```tsx
    return <main className="loading-screen" aria-label="กำลังโหลด"><SunMedium size={30} /><span>YGATE Solar SCADA</span></main>;
```

with:

```tsx
    return <main className="loading-screen" aria-label="กำลังโหลด"><LivePulse state="connecting" className="loading-pulse" /><span>YGATE Solar SCADA</span></main>;
```

- [ ] **Step 3: Update `globals.css` — sidebar/topbar tokens, remove the old `.dot`/pulse spinner rules made obsolete by Live Pulse**

Replace the `.loading-screen` block:

```css
.loading-screen {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--nav);
  font-weight: 700;
  font-family: var(--font-display);
}
.loading-pulse { width: 84px; height: 18px; color: var(--action); }
```

(this replaces the old `.loading-screen svg { animation: pulse ... }` and `@keyframes pulse` rules — delete those two, Live Pulse now owns the animation).

Update `.sidebar` background and `.nav-item.active` to use the refreshed `--nav`/`--nav-active` tokens (values already updated in Task 2, no rule changes needed here — the classes already reference `var(--nav)`/`var(--nav-active)`). Update `.brand-lockup strong` to pick up the display font:

```css
.brand-lockup strong { font-size: 15px; font-family: var(--font-display); }
```

Update `.topbar h1` the same way:

```css
.topbar h1 { margin: 0; font-size: 20px; line-height: 1.2; font-family: var(--font-display); }
```

- [ ] **Step 4: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev, load the app, confirm the loading screen shows the pulse line, log in, confirm the topbar live chip shows a moving waveform when connected and a flat line briefly if you throttle the network to simulate `connecting`/`offline`.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/components/platform-shell.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Wire Live Pulse into topbar live chip and loading screen

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Auth screen + section-heading underline

**Files:**
- Modify: `apps/web/app/components/auth-screen.tsx`
- Modify: `apps/web/app/globals.css`

**Interfaces:**
- Consumes: `LivePulse` (Task 8).

- [ ] **Step 1: Replace the static overlay with a Live Pulse background accent**

In `apps/web/app/components/auth-screen.tsx`, add the import:

```tsx
import { LivePulse } from "./live-pulse";
```

Replace:

```tsx
      <div className="auth-overlay" />
```

with:

```tsx
      <div className="auth-overlay" />
      <LivePulse state="connected" className="auth-pulse" />
```

- [ ] **Step 2: Add CSS for the auth pulse and the section-heading underline accent**

Append to `apps/web/app/globals.css`:

```css
.auth-pulse { position: absolute; z-index: 1; bottom: 12vh; left: clamp(32px, 6vw, 96px); width: min(480px, 60%); height: 40px; color: rgb(255 255 255 / 22%); }
.section-heading { position: relative; padding-bottom: 14px; }
.section-heading::after { content: ""; position: absolute; left: 0; bottom: 0; width: 64px; height: 3px; border-radius: 2px; background: linear-gradient(90deg, var(--action), var(--energy)); }
```

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev, view the logged-out screen and confirm a faint moving waveform is visible over the background photo without harming text legibility; view any page with a `.section-heading` (e.g. Plants) and confirm the small gradient underline appears beneath the heading row.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/components/auth-screen.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Add Live Pulse to auth screen and gradient underline to section headings

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Dashboard (overview-page.tsx)

**Files:**
- Modify: `apps/web/app/features/dashboard/overview-page.tsx`
- Modify: `apps/web/app/globals.css`

**Interfaces:**
- Consumes: `Dialog`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogBody` (Task 3); `Select`, `SelectTrigger`, `SelectValue`, `SelectContent`, `SelectItem` (Task 4).

- [ ] **Step 1: KPI card typography — Manrope headline, JetBrains Mono not needed here (values are already `strong` text; give the KPI figure the display font)**

Append to `apps/web/app/globals.css`, in the dashboard section:

```css
.dashboard-kpi strong { font-size: 17px; font-family: var(--font-display); }
.dashboard-kpi { border-radius: var(--radius-md); }
.dashboard-widget { box-shadow: var(--shadow-sm); }
```

(`.dashboard-widget` already has `border-radius: 7px` covered by the Task 2 find/replace to `var(--radius-md)`.)

- [ ] **Step 2: Migrate `TimeseriesConfigEditor`'s modal to `Dialog`, and its two `<select>` to `Select`**

In `apps/web/app/features/dashboard/overview-page.tsx`, add imports at the top:

```tsx
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
```

Replace the entire `TimeseriesConfigEditor` function body's `return` statement (currently the `<div className="modal-backdrop" ...>` block) with:

```tsx
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <div><DialogDescription>Timeseries widget</DialogDescription><DialogTitle>ตั้งค่ากราฟ</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="grid grid-cols-2 gap-4" onSubmit={submit}>
            <label className="grid gap-1.5 text-xs font-bold text-ink">Plant
              <Select value={plantId} onValueChange={(value) => { setPlantId(value); setDeviceId(""); setPointKey(""); }}>
                <SelectTrigger><SelectValue placeholder="เลือก Plant" /></SelectTrigger>
                <SelectContent>{plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.name} ({plant.code})</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label className="grid gap-1.5 text-xs font-bold text-ink">Device
              <Select value={deviceId} onValueChange={(value) => { setDeviceId(value); setPointKey(""); }} disabled={!plantId}>
                <SelectTrigger><SelectValue placeholder="เลือก Device" /></SelectTrigger>
                <SelectContent>{devices.map((device) => <SelectItem key={device.id} value={device.id}>{device.name} ({device.externalId})</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label className="col-span-2 grid gap-1.5 text-xs font-bold text-ink">Point key
              <input className="h-10 rounded-[var(--radius-sm)] border border-line px-3 text-sm" list="timeseries-point-keys" value={pointKey} onChange={(event) => setPointKey(event.target.value)} maxLength={200} required />
              <datalist id="timeseries-point-keys">{pointOptions.map((key) => <option key={key} value={key} />)}</datalist>
            </label>
            <label className="grid gap-1.5 text-xs font-bold text-ink">ช่วงเวลา
              <Select value={String(timeRangeHours)} onValueChange={(value) => setTimeRangeHours(Number(value) as 1 | 6 | 24 | 168)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="1">1 ชั่วโมง</SelectItem>
                  <SelectItem value="6">6 ชั่วโมง</SelectItem>
                  <SelectItem value="24">24 ชั่วโมง</SelectItem>
                  <SelectItem value="168">7 วัน</SelectItem>
                </SelectContent>
              </Select>
            </label>
            <label className="grid gap-1.5 text-xs font-bold text-ink">Unit<input className="h-10 rounded-[var(--radius-sm)] border border-line px-3 text-sm" value={unit} onChange={(event) => setUnit(event.target.value)} maxLength={20} placeholder="kW" /></label>
            <label className="grid gap-1.5 text-xs font-bold text-ink">Decimals<input className="h-10 rounded-[var(--radius-sm)] border border-line px-3 text-sm" type="number" min="0" max="6" value={decimals} onChange={(event) => setDecimals(Number(event.target.value))} required /></label>
            {error && <p className="form-message error col-span-2">{error}</p>}
            <div className="col-span-2 flex justify-end gap-2"><button type="button" className="secondary-button" onClick={onClose}>ยกเลิก</button><button className="primary-button"><Save size={17} /> บันทึกการตั้งค่า</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
```

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev, open Dashboard → "ปรับ Layout" → "เพิ่มกราฟ", confirm the dialog opens centered with a visible backdrop, Plant/Device/ช่วงเวลา are Radix Select dropdowns (keyboard-navigable, closes on Escape), and saving still works end to end.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/features/dashboard/overview-page.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Migrate dashboard timeseries widget config dialog to Dialog/Select primitives

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: Plants & Devices (plants-page.tsx) — exemplar CRUD migration

**Files:**
- Modify: `apps/web/app/features/plants/plants-page.tsx`

**Interfaces:**
- Consumes: `Dialog`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogBody` (Task 3); `Select`, `SelectTrigger`, `SelectValue`, `SelectContent`, `SelectItem` (Task 4); `toast` (Task 7).
- Establishes the reusable pattern every remaining CRUD page in Phase 2 follows: native `<select>` → `Select`, `.modal-backdrop` div → `Dialog`, silent-success mutation → `toast.success(...)`.

- [ ] **Step 1: Add imports**

At the top of `apps/web/app/features/plants/plants-page.tsx`:

```tsx
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { toast } from "../../components/ui/sonner";
```

- [ ] **Step 2: `decommissionPlant`/`hardDeletePlant`/`decommissionDevice`/`hardDeleteDevice` get success toasts**

In each of the four functions, the line `if (response.ok) void loadPlants();` (or `loadDevices`/`await loadPlants()`/`await loadDevices()`) currently gives no success feedback. Change:

```tsx
    if (response.ok) void loadPlants();
    else setError("ไม่สามารถปิดใช้งานโรงไฟฟ้าได้");
```

to:

```tsx
    if (response.ok) { toast.success(`ปิดใช้งานโรงไฟฟ้า "${plant.name}" แล้ว`); void loadPlants(); }
    else setError("ไม่สามารถปิดใช้งานโรงไฟฟ้าได้");
```

Apply the same pattern (toast message naming the entity, then the existing refresh call) to:
- `hardDeletePlant`: `if (response.ok) { toast.success(\`ลบโรงไฟฟ้า "${plant.name}" ถาวรแล้ว\`); await loadPlants(); }`
- `decommissionDevice`: `if (response.ok) { toast.success(\`ปิดใช้งาน Device "${device.name}" แล้ว\`); void loadDevices(); }`
- `hardDeleteDevice`: `if (response.ok) { toast.success(\`ลบ Device "${device.name}" ถาวรแล้ว\`); await loadDevices(); }`

- [ ] **Step 3: Replace `runCommand`'s inline banner with a toast**

Replace the whole `runCommand` function body:

```tsx
  async function runCommand(kind: "test-connection" | "test-read", device: Device) {
    setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: true } }));
    try {
      const response = await api(`/api/v1/plants/${plant.id}/devices/${device.id}/${kind}`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (response.status === 503) throw new Error("ไม่มี Middleware ดูแล Plant นี้อยู่ หรือออฟไลน์อยู่");
      if (response.status === 504) throw new Error("Middleware ไม่ตอบสนองภายในเวลาที่กำหนด");
      if (!response.ok) throw new Error("ทดสอบไม่สำเร็จ");
      const data = (await response.json()) as { ok?: boolean; error?: string };
      const message = data.error || (kind === "test-connection" ? "เชื่อมต่อสำเร็จ" : "อ่านค่าสำเร็จ");
      setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: false } }));
      if (data.ok === false) toast.error(message); else toast.success(message);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด";
      setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: false } }));
      toast.error(message);
    }
  }
```

Update the `testOutcomes` state type declaration (currently `useState<Record<string, { pending: boolean; ok?: boolean; message?: string }>>({})`) to drop the now-unused `ok`/`message` fields:

```tsx
  const [testOutcomes, setTestOutcomes] = useState<Record<string, { pending: boolean }>>({});
```

Remove the now-dead inline banner JSX in the device row:

```tsx
              {outcome?.pending && <p className="form-message" style={{ gridColumn: "1 / -1" }}>กำลังทดสอบ...</p>}
              {outcome && !outcome.pending && <p className={outcome.ok ? "form-message" : "form-message error"} style={{ gridColumn: "1 / -1" }}>{outcome.message}</p>}
```

(delete both lines — the `outcome` variable is still read for `outcome?.pending` in the two button `disabled` props above it, which stay unchanged).

- [ ] **Step 4: `DeviceEditor`'s Model `<select>` → `Select`**

Replace:

```tsx
          <label className="full-field">Model<select value={deviceModelId} onChange={(event) => setDeviceModelId(event.target.value)} required>
            {models.length === 0 && <option value="">ยังไม่มี Device Model — สร้างที่หน้า Register Metadata ก่อน</option>}
            {models.map((m) => <option key={m.id} value={m.id}>{m.manufacturer} / {m.deviceType} / {m.model}</option>)}
          </select></label>
```

with:

```tsx
          <label className="full-field">Model
            <Select value={deviceModelId} onValueChange={setDeviceModelId} disabled={models.length === 0}>
              <SelectTrigger><SelectValue placeholder="ยังไม่มี Device Model — สร้างที่หน้า Register Metadata ก่อน" /></SelectTrigger>
              <SelectContent>{models.map((m) => <SelectItem key={m.id} value={m.id}>{m.manufacturer} / {m.deviceType} / {m.model}</SelectItem>)}</SelectContent>
            </Select>
          </label>
```

- [ ] **Step 5: `DeviceEditor` and `PlantEditor` modals → `Dialog`**

Replace `DeviceEditor`'s `return` (the `<div className="modal-backdrop" ...>...</div>` block) with:

```tsx
  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>{plant.code}</DialogDescription><DialogTitle>{device ? "แก้ไข Device" : "เพิ่ม Device"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label>Device ID<input autoFocus={!device} value={externalId} onChange={(event) => setExternalId(event.target.value)} maxLength={200} required disabled={Boolean(device)} /></label>
            <label>ชื่อ Device<input autoFocus={Boolean(device)} value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">Model
              <Select value={deviceModelId} onValueChange={setDeviceModelId} disabled={models.length === 0}>
                <SelectTrigger><SelectValue placeholder="ยังไม่มี Device Model — สร้างที่หน้า Register Metadata ก่อน" /></SelectTrigger>
                <SelectContent>{models.map((m) => <SelectItem key={m.id} value={m.id}>{m.manufacturer} / {m.deviceType} / {m.model}</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label>IP<input value={modbusHost} onChange={(event) => setModbusHost(event.target.value)} placeholder="192.168.1.100 (เว้นว่างถ้าไม่ใช่ Modbus device)" /></label>
            <label>Port<input type="number" min="1" max="65535" value={modbusPort} onChange={(event) => setModbusPort(event.target.value)} /></label>
            <label>Unit ID<input type="number" min="0" max="255" value={modbusUnitId} onChange={(event) => setModbusUnitId(event.target.value)} /></label>
            <label>Poll interval (s)<input type="number" min="1" max="3600" value={pollIntervalSeconds} onChange={(event) => setPollIntervalSeconds(event.target.value)} /></label>
            <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /><span>เปิดใช้งาน Device</span></label>
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}>{pending ? "กำลังบันทึก" : device ? "บันทึก" : "สร้าง Device"}</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
```

Note the `<form>` keeps its existing `<label>`/grid children exactly as before (unchanged), only the surrounding wrapper (`modal-backdrop` div + `plant-editor device-editor` section + manual header) is replaced by `Dialog`/`DialogContent`/`DialogHeader`. Because `plant-editor`'s CSS defined `form { padding: 20px; display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }` and related `label`/`input`/`.full-field`/`.toggle-field` rules, add one new class `.plant-editor-form` to `apps/web/app/globals.css` that copies those same rules so they still apply inside the new `DialogBody` wrapper (which no longer has the `.plant-editor` ancestor class):

```css
.plant-editor-form { padding: 20px; display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.plant-editor-form label { min-width: 0; display: grid; gap: 7px; color: var(--ink); font-size: 13px; font-weight: 700; }
.plant-editor-form input, .plant-editor-form select { width: 100%; height: 42px; padding: 0 11px; border: 1px solid #b8c3c9; border-radius: var(--radius-sm); color: var(--ink); background: white; }
.plant-editor-form input:disabled, .plant-editor-form select:disabled { color: var(--muted); background: #edf0f2; }
.plant-editor-form .full-field { grid-column: 1 / -1; }
.plant-editor-form .toggle-field { width: fit-content; grid-template-columns: 20px auto; align-items: center; gap: 9px; }
.plant-editor-form .toggle-field input { width: 18px; height: 18px; accent-color: var(--action); }
```

Apply the same `Dialog` wrapper pattern to `PlantEditor` (replace its `modal-backdrop`/`plant-editor` return block the same way, using `plant-editor-form` as the form's className, `DialogDescription`="Plant registry", `DialogTitle`={plant ? "แก้ไขโรงไฟฟ้า" : "เพิ่มโรงไฟฟ้า"}) and to `DeviceLatestDialog` (replace its `modal-backdrop`/`plant-editor device-editor` return block with `Dialog`/`DialogContent`/`DialogHeader`/`DialogBody`, keeping its existing inner `<div className="grid ...">`/reading list JSX unchanged inside `DialogBody`). `PlantEditor`'s existing `useEffect` that manually locks `document.body.style.overflow` and listens for Escape is now redundant (Radix `Dialog` already does both) — delete that whole `useEffect` block from `PlantEditor`.

- [ ] **Step 6: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: open Plants → add/edit a plant (dialog opens, Escape closes it, backdrop click closes it, save works); open a plant's Devices → add/edit a device (Model is now a Select, IP/Port/etc. unchanged); decommission or hard-delete a plant/device and confirm a success toast appears bottom-right; click Test Connection/Test Read on a Modbus-configured device and confirm a toast (not an inline row banner) reports the result.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/features/plants/plants-page.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Migrate Plants/Devices to Dialog/Select primitives with toast feedback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: Middlewares (middlewares-page.tsx)

**Files:**
- Modify: `apps/web/app/features/middlewares/middlewares-page.tsx`

**Interfaces:**
- Consumes: `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogBody`, `Select`/`SelectTrigger`/`SelectValue`/`SelectContent`/`SelectItem`, `toast` (same as Task 12).

- [ ] **Step 1: Add the same three imports as Task 12** (adjust the relative path is identical: `../../components/ui/dialog`, `../../components/ui/select`, `../../components/ui/sonner`).

- [ ] **Step 2: `setGatewayActive` gets a success toast**

Replace:

```tsx
    if (response.ok) await loadGateways();
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะ Middleware" : "ไม่สามารถเปลี่ยนสถานะ Middleware ได้");
```

with:

```tsx
    if (response.ok) { toast.success(isActive ? `เปิดใช้งาน "${gateway.name}" แล้ว` : `ปิดใช้งาน "${gateway.name}" แล้ว`); await loadGateways(); }
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะ Middleware" : "ไม่สามารถเปลี่ยนสถานะ Middleware ได้");
```

- [ ] **Step 3: `assignPlant`/`unassignPlant` get success toasts**

In `assignPlant`, replace `setAddPlantId(""); await load();` with `toast.success(\`มอบหมาย Plant ให้ ${gateway.name} แล้ว\`); setAddPlantId(""); await load();`.
In `unassignPlant`, replace `await load();` (the one inside the `if (!response.ok) throw ...` try block, after the DELETE call succeeds) with `toast.success(\`เอา "${plant.name}" ออกจาก ${gateway.name} แล้ว\`); await load();`.

- [ ] **Step 4: `MiddlewareEditor` modal → `Dialog`**

Replace the `return` block (the `modal-backdrop`/`plant-editor api-key-editor` wrapper) with the same `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogBody` pattern from Task 12, using `className="plant-editor-form"` on the inner `<form>` (the shared class added in Task 12 already covers this — no new CSS needed), `DialogDescription`="Middleware gateway", `DialogTitle`={gateway ? "แก้ไข Middleware" : "เพิ่ม Middleware"}.

- [ ] **Step 5: Assigned-Plant `<select>` → `Select`**

Replace:

```tsx
        <select value={addPlantId} onChange={(event) => setAddPlantId(event.target.value)}>
          <option value="">เลือก Plant ที่จะมอบหมาย...</option>
          {unassignedPlants.map((p) => <option key={p.id} value={p.id}>{p.code} - {p.name}</option>)}
        </select>
```

with:

```tsx
        <Select value={addPlantId} onValueChange={setAddPlantId} disabled={unassignedPlants.length === 0}>
          <SelectTrigger className="w-64"><SelectValue placeholder="เลือก Plant ที่จะมอบหมาย..." /></SelectTrigger>
          <SelectContent>{unassignedPlants.map((p) => <SelectItem key={p.id} value={p.id}>{p.code} - {p.name}</SelectItem>)}</SelectContent>
        </Select>
```

- [ ] **Step 6: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: create/edit a Middleware Gateway via the dialog; assign/unassign a Plant and confirm success toasts; toggle a gateway active/inactive and confirm a toast.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/features/middlewares/middlewares-page.tsx
git commit -m "$(cat <<'EOF'
Migrate Middleware Gateways to Dialog/Select primitives with toast feedback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 14: Register Metadata (register-metadata-page.tsx)

**Files:**
- Modify: `apps/web/app/features/register-metadata/register-metadata-page.tsx`
- Modify: `apps/web/app/components/ui.ts`

**Interfaces:**
- Consumes: `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogDescription`/`DialogBody` (Task 3), `Select`/`SelectTrigger`/`SelectValue`/`SelectContent`/`SelectItem` (Task 4), `toast` (Task 7).

This page already uses raw Tailwind utility classes with hardcoded `slate-*`/`rose-*`/`emerald-*`/`cyan-*` colors and its own local `Dialog` helper (defined at the bottom of the file) instead of the shared global CSS classes — bring it onto the shared tokens and the shared `Dialog` primitive.

- [ ] **Step 1: Update `apps/web/app/components/ui.ts` to reference tokens instead of hardcoded slate/cyan**

Replace the file in full:

```typescript
export const inputClass = "h-10 w-full rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-sm text-ink outline-none transition focus:border-focus focus:ring-4 focus:ring-focus/15";
export const labelClass = "grid min-w-0 gap-1.5 text-xs font-bold text-ink";
export const iconButtonClass = "inline-grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-line bg-surface text-ink-soft transition hover:border-ink-soft/40 hover:bg-canvas disabled:opacity-50";
export const primaryButtonClass = "inline-flex h-10 items-center justify-center gap-2 rounded-[var(--radius-sm)] bg-brand px-4 text-sm font-bold text-white transition hover:brightness-110 disabled:opacity-50";
export const secondaryButtonClass = "inline-flex h-10 items-center justify-center gap-2 rounded-[var(--radius-sm)] border border-line bg-surface px-4 text-sm font-bold text-ink transition hover:bg-canvas disabled:opacity-50";
```

- [ ] **Step 2: Replace hardcoded slate/rose/emerald text classes throughout `register-metadata-page.tsx`**

Run these literal find/replace passes across the file (via Edit tool, `replace_all: true` for each):
- `text-slate-900` → `text-ink`
- `text-slate-700` → `text-ink`
- `text-slate-600` → `text-ink-soft`
- `text-slate-500` → `text-ink-soft`
- `border-slate-200` → `border-line`
- `bg-slate-50` → `bg-canvas`
- `bg-rose-50 text-rose-700` → `bg-danger/10 text-danger`
- `text-rose-700` → `text-danger`
- `hover:border-rose-200 hover:bg-rose-50` → `hover:border-danger/30 hover:bg-danger/10`
- `bg-emerald-50 text-emerald-700` → `bg-success/10 text-success`
- `focus-visible:outline-cyan-700` → `focus-visible:outline-focus`
- `accent-[#174f68]` → `accent-brand`

- [ ] **Step 3: Replace the file-local `Dialog` helper with the shared primitive**

Delete the local `function Dialog({ title, eyebrow, onClose, pending, children }: ...) { ... }` function entirely (it duplicates what Task 3's primitive now provides).

Add the import at the top:

```tsx
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
```

Every call site currently does `<Dialog title={...} eyebrow={...} onClose={onClose} pending={pending}>{children}</Dialog>` (two call sites: inside `DeviceModelDialog` and `AddressMetadataDialog`). Replace both with the shared primitive's composition — for `DeviceModelDialog`:

```tsx
  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>Model configuration</DialogDescription><DialogTitle>{model ? "แก้ไข Device Model" : "เพิ่ม Device Model"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
            {/* ...unchanged form fields exactly as they are today... */}
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
```

and for `AddressMetadataDialog`, the same wrapper with `DialogDescription`={`${model.manufacturer} / ${model.model}`} and `DialogTitle`={item ? \`แก้ไข ${item.addressKey}\` : "เพิ่ม Address Metadata"}`. In both, keep every `<label>`/`<input>`/`<select>`/`<textarea>` inside the `<form>` byte-for-byte identical to what's there today except for Step 4 below.

- [ ] **Step 4: Replace the 5 native `<select>` elements with `Select`**

Add the import:

```tsx
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
```

`DeviceModelDialog` has no `<select>` (only inputs/checkbox) — skip it. `AddressMetadataDialog` has 4 selects (Data type, Function code, Word order, Modbus type). Replace each, e.g.:

```tsx
        <label className={labelClass}>Data type
          <Select value={dataType} onValueChange={(value) => setDataType(value as DeviceModelRegisterMetadata["dataType"])}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="number">number</SelectItem>
              <SelectItem value="boolean">boolean</SelectItem>
              <SelectItem value="text">text</SelectItem>
              <SelectItem value="enum">enum</SelectItem>
            </SelectContent>
          </Select>
        </label>
```

and the same pattern for the Modbus function code (`""`/`"3"`/`"4"` values, labels `-`/`FC03`/`FC04`), word order (`""`/`"HIGH_LOW"`/`"LOW_HIGH"`, labels `Word order (default)`/`HIGH_LOW`/`LOW_HIGH`), and Modbus data type (`""` plus `MIDDLEWARE_DATA_TYPES.map(...)`, exactly mirroring the existing `<option>` loop but as `SelectItem`).

- [ ] **Step 5: Success toasts for save/delete**

Add the import: `import { toast } from "../../components/ui/sonner";`

In `modelSaved`/`addressSaved` (called after a successful PUT/POST from inside the dialog components), and in `removeAddress`/`hardDeleteModel` after their `response.ok` branches, add a `toast.success(...)` call analogous to Task 12 Step 2/3 (e.g. `toast.success(\`บันทึก Model ${model.manufacturer} ${model.model} แล้ว\`)`, `toast.success(\`ลบ Address ${item.addressKey} แล้ว\`)`).

- [ ] **Step 6: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: open Register Metadata, add/edit a Device Model, add/edit an Address (all 4 selects work, dialog closes on save), delete an address and a model, confirm toasts.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/features/register-metadata/register-metadata-page.tsx apps/web/app/components/ui.ts
git commit -m "$(cat <<'EOF'
Migrate Register Metadata onto shared tokens, Dialog, and Select primitives

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 15: Users (users-page.tsx)

**Files:**
- Modify: `apps/web/app/features/users/users-page.tsx`

- [ ] **Step 1: Add the standard three imports** (dialog, select, sonner — same relative paths as Task 12).

- [ ] **Step 2: `setUserActive`/`unlockUser` get success toasts** — same pattern as Task 12 Step 2, e.g. `toast.success(isActive ? \`เปิดใช้งาน ${target.displayName} แล้ว\` : \`ปิดใช้งาน ${target.displayName} แล้ว\`)` before/alongside the existing `await loadUsers()`, and `toast.success(\`ปลดล็อก ${target.displayName} แล้ว\`)` in `unlockUser`.

- [ ] **Step 3: `UserEditor` and `PasswordResetDialog` modals → `Dialog`**, `Role` `<select>` in `UserEditor` → `Select` — identical pattern to Task 12 Step 4/5, using `className="plant-editor-form"` on both forms (the shared class from Task 12), `DialogDescription`="User management" / `DialogTitle`={user ? "แก้ไขผู้ใช้" : "เพิ่มผู้ใช้"}` for `UserEditor`, and `DialogDescription`={user.email} / `DialogTitle`="ตั้งรหัสผ่านใหม่"` for `PasswordResetDialog`. Replace:

```tsx
        <label>Role<select value={roleId} onChange={(event) => setRoleId(event.target.value)} required>{roles.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select></label>
```

with:

```tsx
        <label>Role
          <Select value={roleId} onValueChange={setRoleId}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>{roles.map((role) => <SelectItem key={role.id} value={role.id}>{role.name}</SelectItem>)}</SelectContent>
          </Select>
        </label>
```

- [ ] **Step 4: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: add/edit a user (Role select works), unlock a locked user, deactivate/reactivate a user, reset a password — confirm toasts and dialogs behave like Task 12.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/users/users-page.tsx
git commit -m "$(cat <<'EOF'
Migrate Users to Dialog/Select primitives with toast feedback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 16: Roles (roles-page.tsx)

**Files:**
- Modify: `apps/web/app/features/roles/roles-page.tsx`

- [ ] **Step 1: Add `Dialog`/`toast` imports** (no `<select>` on this page — Role editor's only "picker" is the `permission-picker` checkbox fieldset, which stays exactly as-is; no `Select` import needed here).

- [ ] **Step 2: `deleteRole` gets a success toast** on its `if (response.ok) { await loadRoles(); return; }` branch: `toast.success(\`ลบ Role "${role.name}" แล้ว\`); await loadRoles(); return;`.

- [ ] **Step 3: `RoleEditor` modal → `Dialog`**, `className="plant-editor-form"` on the form, keeping the `<fieldset className="full-field permission-picker">...</fieldset>` block byte-for-byte unchanged inside it (the `permission-picker`/`permission-group` CSS classes already reference tokens via `var(--muted)` etc., so they restyle for free from Task 2 — no changes needed there). Delete `RoleEditor`'s manual `useEffect` that sets `document.body.style.overflow`/listens for Escape (same reasoning as Task 12 Step 5 — Radix Dialog already does this), `DialogDescription`="Access management" / `DialogTitle`={role ? "แก้ไข Role" : "เพิ่ม Role"}`.

- [ ] **Step 4: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: add/edit/delete a Role, confirm the permission checkbox picker still works inside the new Dialog, confirm the delete toast.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/roles/roles-page.tsx
git commit -m "$(cat <<'EOF'
Migrate Roles to Dialog primitive with toast feedback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 17: API Keys (api-keys-page.tsx)

**Files:**
- Modify: `apps/web/app/features/api-keys/api-keys-page.tsx`

- [ ] **Step 1: Add `Dialog`/`toast` imports** (no `<select>` — skip `Select`).

- [ ] **Step 2: `setClientActive` gets a success toast**, same pattern as Task 13 Step 2 (`toast.success(isActive ? ... : ...)`).

- [ ] **Step 3: `APIKeyEditor` modal → `Dialog`**, `className="plant-editor-form"`, `DialogDescription`="Middleware API key" / `DialogTitle`={client ? "แก้ไข API Key" : "เพิ่ม API Key"}`.

- [ ] **Step 4: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: create/edit an API Key, toggle active state, confirm toasts and that the secret-reveal panel (`.secret-panel`, unrelated to this migration) still works.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/api-keys/api-keys-page.tsx
git commit -m "$(cat <<'EOF'
Migrate API Keys to Dialog primitive with toast feedback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 18: Alarms (alarms-page.tsx)

**Files:**
- Modify: `apps/web/app/features/alarms/alarms-page.tsx`

**Interfaces:**
- Consumes: `Dialog` (Task 3), `Select` (Task 4), `Tabs`/`TabsList`/`TabsTrigger` (Task 6), `toast` (Task 7).

- [ ] **Step 1: Add imports** (dialog, select, tabs, sonner).

- [ ] **Step 2: Plant `<select>` in the heading → `Select`**

Replace:

```tsx
          <select value={plantId} onChange={(event) => setPlantId(event.target.value)} aria-label="เลือกโรงไฟฟ้า">
            {plants.map((plant) => <option key={plant.id} value={plant.id}>{plant.name}</option>)}
          </select>
```

with:

```tsx
          <Select value={plantId} onValueChange={setPlantId}>
            <SelectTrigger className="w-48" aria-label="เลือกโรงไฟฟ้า"><SelectValue /></SelectTrigger>
            <SelectContent>{plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.name}</SelectItem>)}</SelectContent>
          </Select>
```

- [ ] **Step 3: `.mode-switch` log/rules toggle → `Tabs`**

Replace:

```tsx
          <div className="mode-switch" aria-label="มุมมอง Alarm">
            <button className={tab === "log" ? "active" : ""} onClick={() => setTab("log")}>Log</button>
            <button className={tab === "rules" ? "active" : ""} onClick={() => setTab("rules")}>Rules</button>
          </div>
```

with:

```tsx
          <Tabs value={tab} onValueChange={(value) => setTab(value as "log" | "rules")}>
            <TabsList aria-label="มุมมอง Alarm">
              <TabsTrigger value="log">Log</TabsTrigger>
              <TabsTrigger value="rules">Rules</TabsTrigger>
            </TabsList>
          </Tabs>
```

(this page renders the `log`/`rules` bodies itself via the existing `tab === "log" ? ... : ...` conditional further down — leave that conditional exactly as-is; `Tabs` here is used purely as the switch control, not as a `TabsContent` wrapper, since the two bodies already have distinct layouts/state that don't fit cleanly under one `Tabs` tree without a larger restructure that's out of scope for a token/primitive swap).

- [ ] **Step 4: `deleteRule`/`acknowledge` get success toasts** — `toast.success(\`ลบกฎ "${rule.label}" แล้ว\`)` and `toast.success("Acknowledge alarm แล้ว")` respectively, alongside their existing `await loadAlarms()`.

- [ ] **Step 5: `AlarmRuleEditor` modal → `Dialog`**, its Device `<select>` (only present when `!rule`, i.e. create mode) → `Select`, its Severity `<select>` → `Select`. Same wrapper pattern as Task 12, `className="plant-editor-form"`, `DialogDescription`="Alarm rule" / `DialogTitle`={rule ? "แก้ไขกฎแจ้งเตือน" : "เพิ่มกฎแจ้งเตือน"}`.

- [ ] **Step 6: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: switch Plant via the new Select, switch Log/Rules via the new Tabs, add/edit/delete an alarm rule, acknowledge an alarm event — confirm toasts.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/features/alarms/alarms-page.tsx
git commit -m "$(cat <<'EOF'
Migrate Alarms to Dialog/Select/Tabs primitives with toast feedback

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 19: Audit, Sessions, Site Map, Profile — token-only polish

**Files:**
- Modify: `apps/web/app/features/audit/audit-page.tsx`
- Modify: `apps/web/app/features/sessions/sessions-page.tsx`
- Modify: `apps/web/app/globals.css`

**Interfaces:**
- Consumes: `toast` (Task 7) for `clearAudit`/`revokeSession`/`clearSessions`. Site Map and Profile need **no code changes** — both already use only shared CSS classes (`.site-map-*`, `.profile-*`) that inherit new tokens automatically from Task 2; this task does not touch `site-map-page.tsx` or `profile-page.tsx`.

- [ ] **Step 1: `apps/web/app/features/audit/audit-page.tsx` — success toast on clear, mono font for the JSON viewer**

Add `import { toast } from "../../components/ui/sonner";`. Replace:

```tsx
    if (response.ok) await loadEvents();
    else setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ clear Audit view ได้" : "ไม่สามารถ clear Audit Log ได้");
```

with:

```tsx
    if (response.ok) { toast.success("Clear Audit Log แล้ว"); await loadEvents(); }
    else setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ clear Audit view ได้" : "ไม่สามารถ clear Audit Log ได้");
```

- [ ] **Step 2: `apps/web/app/features/sessions/sessions-page.tsx` — success toast on revoke**

Add the same import. Replace:

```tsx
    if (response.ok) await loadSessions();
    else setError("ไม่สามารถยกเลิกเซสชันได้");
```

with:

```tsx
    if (response.ok) { toast.success("ยกเลิกเซสชันแล้ว"); await loadSessions(); }
    else setError("ไม่สามารถยกเลิกเซสชันได้");
```

(leave `clearSessions` alone — it navigates away via `window.location.assign("/")` immediately on success, so a toast would never be seen).

- [ ] **Step 3: `globals.css` — JetBrains Mono for JSON/log/timestamp displays**

Update the two rules that currently hardcode `Consolas, monospace`:

```css
.activity-band p { margin: 3px 0 0; color: var(--muted); font-family: var(--font-mono); font-size: 11px; }
```

and

```css
.secret-panel code { min-width: 0; padding: 9px 10px; overflow: auto; color: #0d3f2d; background: white; border: 1px solid #bce6d4; border-radius: 5px; font-family: var(--font-mono); font-size: 12px; white-space: nowrap; }
```

and add mono font to the audit JSON viewer and site-map popup coordinates-style text (already `font-family: Consolas, "Courier New", monospace;` on `.audit-json pre` and `.openapi-viewer`):

```css
.audit-json pre { max-height: 220px; margin: 8px 0 0; padding: 10px; overflow: auto; border: 1px solid var(--line); border-radius: var(--radius-sm); background: #f7f9fa; color: var(--ink); font-family: var(--font-mono); font-size: 11px; line-height: 1.45; white-space: pre; }
```

- [ ] **Step 4: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: view Audit Log and expand a row's JSON details (now in JetBrains Mono), revoke a session and confirm the toast, view Site Map and Profile and confirm they already look correctly re-themed with zero code changes (colors/radius updated via Task 2 alone).

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/audit/audit-page.tsx apps/web/app/features/sessions/sessions-page.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Add toast feedback to Audit/Sessions and JetBrains Mono to log/JSON views

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 20: OpenAPI (openapi-page.tsx)

**Files:**
- Modify: `apps/web/app/features/openapi/openapi-page.tsx`

- [ ] **Step 1: Replace hardcoded slate/rose/emerald/sky/amber/violet/cyan Tailwind classes with token-based ones**

Run the same find/replace list as Task 14 Step 2, plus these OpenAPI-specific ones (Edit tool, `replace_all: true` each):
- `bg-sky-50 text-sky-700` → `bg-focus/10 text-focus` (GET method badge)
- `bg-emerald-50 text-emerald-700` → `bg-success/10 text-success` (POST method badge)
- `bg-amber-50 text-amber-800` → `bg-warning/10 text-warning` (PUT method badge)
- `bg-violet-50 text-violet-700` → `bg-energy/15 text-energy` (PATCH method badge — reuses the signature energy color for a badge that has no closer semantic match)
- `bg-rose-50 text-rose-700` → `bg-danger/10 text-danger` (DELETE method badge and error banners)
- `border-cyan-700 text-cyan-800` (tab underline) → `border-brand text-brand`
- `bg-slate-900 text-white` (method-filter active pill) → `bg-nav text-white`
- `bg-slate-950` (YAML source pane background, `Try it` JSON textarea) → leave as-is — this is a deliberate dark code-viewer treatment independent of the SCADA dark scope, matches the existing `.openapi-viewer` dark treatment already in `globals.css`; only recolor its text from `text-slate-200`/`text-slate-100`/`text-slate-300` to `text-scada-ink` (reusing the SCADA dark-surface text token, since both are "dark code panel on a light page" contexts) for a single consistent dark-panel foreground color instead of three slightly different Tailwind grays.

- [ ] **Step 2: JetBrains Mono for the YAML source and JSON response viewers**

Add `font-mono` to the two `<pre>` elements (`className="max-h-[65vh] overflow-auto p-4 text-xs leading-6 text-scada-ink font-mono"` and the result-body `<pre>`'s className gains `font-mono`), and to the `<code>` elements showing `operation.path`/`operationId`.

- [ ] **Step 3: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: open OpenAPI, confirm method badges use the new palette, switch to YAML Source tab and confirm it's still readable, expand an endpoint and use "Try it" to confirm the console still executes requests.

- [ ] **Step 4: Commit**

```bash
git add apps/web/app/features/openapi/openapi-page.tsx
git commit -m "$(cat <<'EOF'
Migrate OpenAPI page onto shared tokens and JetBrains Mono

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3 — SCADA dark canvas

### Task 21: SCADA shell — dark scope, Tabs mode-switch, Dialog create-screen, Tooltip in inspector

**Files:**
- Modify: `apps/web/app/features/scada/scada-page.tsx`
- Modify: `apps/web/app/globals.css`

**Interfaces:**
- Consumes: `Dialog` (Task 3), `Select` (Task 4), `Tooltip`/`TooltipTrigger`/`TooltipContent` (Task 5), `Tabs`/`TabsList`/`TabsTrigger` (Task 6).

- [ ] **Step 1: Scope the SCADA workbench to the dark tokens**

In `apps/web/app/globals.css`, wrap the existing SCADA rule block (`.scada-workbench` through `.react-flow__controls, .react-flow__minimap { ... }`, currently lines ~439–526) in a `.scada-dark` ancestor selector by prefixing every one of those selectors with `.scada-dark ` and swapping the token references inside them from the light tokens to the `--scada-*` tokens. Concretely, replace this line:

```css
.scada-palette, .scada-inspector, .scada-stage-shell { min-width: 0; border: 1px solid var(--line); background: var(--surface); }
```

with:

```css
.scada-dark .scada-palette, .scada-dark .scada-inspector, .scada-dark .scada-stage-shell { min-width: 0; border: 1px solid var(--scada-line); background: var(--scada-surface); }
```

and apply the same `.scada-dark ` prefix + `var(--scada-line)`/`var(--scada-surface)`/`var(--scada-bg)`/`var(--scada-ink)`/`var(--scada-muted)`/`var(--scada-accent)` substitution (in place of `var(--line)`/`var(--surface)`/`var(--canvas)`/`var(--ink)`/`var(--muted)`/`var(--action)`) to every remaining selector in that block: `.scada-palette header, .scada-inspector header`, `.scada-palette header strong, .scada-inspector header strong`, `.scada-palette header small, .scada-inspector header small`, `.scada-palette > button`, `.scada-palette > button:hover`, `.scada-palette > button strong`, `.scada-palette > button small`, `.palette-note`, `.scada-stage-shell`, `.scada-stage-toolbar`, `.scada-stage`, `.scada-inspector > label`, `.scada-inspector input, .scada-inspector select`, `.inspector-grid label`, `.binding-editor > label`, `.scada-item-row`, `.version-list strong`, `.version-list small`, `.scada-node` (background/border/box-shadow), `.scada-node.selected` (border-color/box-shadow → `var(--scada-accent)`), `.scada-node small`, `.led-lamp`, `.node-quality.normal`, `.react-flow__edge-path` (stroke → `var(--scada-accent)`), `.react-flow__controls, .react-flow__minimap`. Each substitution is mechanical: prefix the selector with `.scada-dark `, and inside the declaration swap any `var(--line|surface|canvas|ink|muted|action)` for the matching `var(--scada-*)` token. Do **not** touch the SCADA node *type-specific* color rules that reference `--success`/`--warning`/`--danger` (e.g. `.alarm-items b.alarm`, `.status.online`) — those stay as the shared status colors, unchanged, since alarm severity meaning should stay consistent between light and dark contexts.

Add the selected-node glow treatment called for in the spec, replacing:

```css
.scada-dark .scada-node.selected { border-color: var(--scada-accent); box-shadow: 0 0 0 2px color-mix(in srgb, var(--scada-accent) 16%, transparent); }
```

with:

```css
.scada-dark .scada-node.selected { border-color: var(--scada-accent); box-shadow: 0 0 0 1px var(--scada-line), 0 0 12px color-mix(in srgb, var(--scada-accent) 25%, transparent); }
```

- [ ] **Step 2: Apply the `.scada-dark` class**

In `apps/web/app/features/scada/scada-page.tsx`, `ScadaCanvas`'s root `return` currently starts:

```tsx
  return <div className={editable ? "scada-workbench editing" : "scada-workbench viewing"}>
```

Change to:

```tsx
  return <div className={editable ? "scada-workbench editing scada-dark" : "scada-workbench viewing scada-dark"}>
```

- [ ] **Step 3: `.mode-switch` (Edit/Preview/Published) → `Tabs`**

Add the import: `import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";`

Replace:

```tsx
        <div className="mode-switch" aria-label="โหมด SCADA">
          {screen.canEdit && <button className={mode === "edit" ? "active" : ""} onClick={() => setMode("edit")}><Pencil size={15} /> Edit</button>}
          {screen.canEdit && <button className={mode === "preview" ? "active" : ""} onClick={() => setMode("preview")}><Eye size={15} /> Preview</button>}
          <button className={mode === "published" ? "active" : ""} onClick={() => void showPublished()}><Radio size={15} /> Published</button>
        </div>
```

with:

```tsx
        <Tabs value={mode} onValueChange={(value) => { if (value === "published") void showPublished(); else setMode(value as BuilderMode); }}>
          <TabsList aria-label="โหมด SCADA">
            {screen.canEdit && <TabsTrigger value="edit"><Pencil size={15} /> Edit</TabsTrigger>}
            {screen.canEdit && <TabsTrigger value="preview"><Eye size={15} /> Preview</TabsTrigger>}
            <TabsTrigger value="published"><Radio size={15} /> Published</TabsTrigger>
          </TabsList>
        </Tabs>
```

(the `.mode-switch`/`.mode-switch button`/`.mode-switch button.active` CSS rules in `globals.css` become dead code after this — delete them; they were the only consumer of that class besides the Alarms page, which Task 18 already migrated to `Tabs`.)

- [ ] **Step 4: `ScadaLibrary`'s create-screen modal → `Dialog`, Plant `<select>` → `Select`**

Add the imports: `import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";` and `import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";`

Replace the inline `{createOpen && <div className="modal-backdrop" ...>...</div>}` block at the end of `ScadaLibrary` with:

```tsx
    <Dialog open={createOpen} onOpenChange={onCreateOpen}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div><DialogDescription>Phase 3B</DialogDescription><DialogTitle>สร้าง SCADA Screen</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={(event) => void onCreate(event)}>
            <label className="full-field">Plant
              <Select value={createPlantID} onValueChange={onCreatePlantID}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>{plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.code} · {plant.name}</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label className="full-field">ชื่อ Screen<input autoFocus value={createName} onChange={(event) => onCreateName(event.target.value)} maxLength={100} placeholder="Single Line Diagram" required /></label>
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={() => onCreateOpen(false)}>ยกเลิก</button><button className="primary-button"><FilePlus2 size={17} /> สร้าง Draft</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
```

- [ ] **Step 5: Tooltip on the SCADA inspector's node-type label**

Add the import: `import { Tooltip, TooltipTrigger, TooltipContent } from "../../components/ui/tooltip";`

In `ScadaInspector`, replace:

```tsx
    <header><Pencil size={17} /><div><strong>Node properties</strong><small>{selected.type} · {selected.id.slice(0, 12)}</small></div></header>
```

with:

```tsx
    <header><Pencil size={17} /><div><strong>Node properties</strong><Tooltip><TooltipTrigger asChild><small className="cursor-help underline decoration-dotted">{selected.type} · {selected.id.slice(0, 12)}</small></TooltipTrigger><TooltipContent>Node type: {selected.type}<br />Full ID: {selected.id}</TooltipContent></Tooltip></div></header>
```

- [ ] **Step 6: Verify**

Run: `cd apps/web && npx tsc --noEmit`. In dev: open a SCADA screen, confirm the workbench (palette/stage/inspector) renders on a dark background with the cyan accent on the selected node and edges; confirm Edit/Preview/Published switches via the new Tabs; create a new screen via the Dialog+Select; hover the node-type label in the inspector and confirm a tooltip with the full node ID appears.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/features/scada/scada-page.tsx apps/web/app/globals.css
git commit -m "$(cat <<'EOF'
Scope SCADA workbench to dark tokens; migrate mode-switch to Tabs, create-screen to Dialog/Select, add inspector Tooltip

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Phase 4 — Final verification

### Task 22: Full build, responsive, and accessibility pass

**Files:** none (verification only).

- [ ] **Step 1: Full typecheck and build**

```bash
cd apps/web
npx tsc --noEmit
npx next build
```

Expected: both clean, zero errors/warnings introduced by this plan.

- [ ] **Step 2: Manual pass — every migrated page**

In dev (`npm run dev`), walk every page touched by Tasks 9–21 (Dashboard, Plants/Devices, Middlewares, Register Metadata, Users, Roles, API Keys, Alarms, Audit, Sessions, OpenAPI, SCADA, plus Auth screen and the sidebar/topbar shell) and confirm for each: Tab reaches every interactive element in a sensible order, Escape closes any open Dialog, every Dialog/Select/Tooltip/Tabs has a visible focus ring (the global `:focus-visible` rule from Task 2 already applies to native elements — confirm it also renders on the new Radix triggers, which are real `<button>`/`<div role="...">` elements so the same CSS selector should already catch them; if a specific Radix trigger doesn't show the ring, add its data-slot to the `:focus-visible` selector list in `globals.css` as a fix, not a new rule shape).

- [ ] **Step 3: Manual pass — responsive breakpoints**

Resize the browser (or use devtools device toolbar) to below 1100px and below 520px on: Plants (table→stacked row layout), Users, API Keys, SCADA (workbench stacks to one column, palette becomes a wrapping grid) — confirm the existing `@media` rules in `globals.css` (untouched by this plan except for token substitutions) still produce the same stacked layouts as before, since no grid-template-columns rules were changed, only colors/radius/fonts.

- [ ] **Step 4: Manual pass — Live Pulse reflects real connection state**

With the dev server running, open the browser devtools Network tab, throttle to "Offline", confirm the topbar live chip's waveform goes flat and the label reads "Offline"; restore the network and confirm it returns to the moving "Live" waveform without a page reload.

- [ ] **Step 5: No commit for this task** — it is verification-only. If Step 2 or 3 finds a real regression, fix it in the file/task where it was introduced and amend that task's commit is not appropriate (per repo convention, never amend) — instead make a small new commit describing the fix, referencing which task it corrects.

# TOR Minimum Platform Modules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the minimum TOR feature gaps while reusing the existing YGate pages and real 17-site connectivity, with only one genuinely new top-level menu: Reports.

**Architecture:** Extend the existing Portfolio/Overview, Plant/Device, Site Map, Energy Analysis, SCADA and Alarms vertical slices. Keep SCADA as a fixed plant mimic and Dashboard Profiles as saved analytical layouts. Add new persistence/API only where the current model cannot represent the TOR requirement.

**Tech Stack:** Next.js 16, React 19, TypeScript, Go, PostgreSQL migrations/sqlc, Node built-in test runner, Go test.

## Global Constraints

- Preserve the current configurable plant/device and Middleware mapping; do not hard-code 17 sites.
- Preserve current RBAC and audit logging for every new mutating API.
- Do not duplicate existing pages as new top-level modules when a tab or extension satisfies the TOR.
- Every new production behavior starts with a failing test and is verified with the focused test followed by the full relevant suite.
- Keep deployment-persisted assets and Middleware version folders outside release directories; this plan does not reset those paths.

---

### Task 1: Align existing navigation names with the TOR/Farsight terminology

**Files:**
- Modify: `apps/web/app/lib/navigation.ts`
- Test: `apps/web/app/lib/navigation.test.ts`

**Interfaces:**
- Produces stable labels and page titles used by the sidebar and Roles & Permissions editor.

- [x] **Step 1: Write the failing test**

```ts
import test from "node:test";
import assert from "node:assert/strict";
import { navigation, titles } from "./navigation";

test("uses TOR-aligned monitoring menu names", () => {
  const labels = navigation.flatMap((group) => group.items.map((item) => item.label));
  assert.deepEqual(labels.slice(0, 6), [
    "Portfolio Monitoring",
    "Plant & Asset Manager",
    "GIS Asset Map",
    "Analytics",
    "SCADA",
    "Alarms & Event Logbook",
  ]);
  assert.equal(titles["/"], "Portfolio Monitoring");
  assert.equal(titles["/scada/live"], "SCADA");
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm test -- app/lib/navigation.test.ts` from `apps/web`.
Expected: FAIL because the current labels are Overview, Plants/Devices, Site Map, Energy Analysis, SCADA Viewer and Alarms.

- [x] **Step 3: Write minimal implementation**

Update only the six monitoring labels and matching titles/permission-group labels. Keep all hrefs and permission resource types unchanged.

- [x] **Step 4: Run test to verify it passes**

Run: `npm test -- app/lib/navigation.test.ts` from `apps/web`.
Expected: PASS.

### Task 2: Add the minimum Portfolio KPI calculations as pure, testable domain logic

**Files:**
- Create: `apps/web/app/lib/portfolio-kpi.ts`
- Create: `apps/web/app/lib/portfolio-kpi.test.ts`
- Modify: `apps/web/app/features/dashboard/overview-page.tsx`

**Interfaces:**
- Produces `calculateCapacityFactor(activePowerKw, installedAcKw): number | null`.
- Produces `calculateTargetAchievement(actual, target): number | null`.
- Produces `rankPortfolio(items, value): Array<{ id: string; rank: number; value: number }>`.

- [x] **Step 1: Write the failing test**

```ts
import test from "node:test";
import assert from "node:assert/strict";
import { calculateCapacityFactor, calculateTargetAchievement, rankPortfolio } from "./portfolio-kpi";

test("calculates capacity factor against installed AC capacity", () => {
  assert.equal(calculateCapacityFactor(500, 1000), 50);
  assert.equal(calculateCapacityFactor(500, 0), null);
});

test("calculates target achievement without dividing by zero", () => {
  assert.equal(calculateTargetAchievement(90, 100), 90);
  assert.equal(calculateTargetAchievement(90, 0), null);
});

test("ranks portfolio values from highest to lowest", () => {
  assert.deepEqual(rankPortfolio([{ id: "b", value: 5 }, { id: "a", value: 10 }], (item) => item.value), [
    { id: "a", rank: 1, value: 10 },
    { id: "b", rank: 2, value: 5 },
  ]);
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm test -- app/lib/portfolio-kpi.test.ts` from `apps/web`.
Expected: FAIL because `portfolio-kpi.ts` does not exist.

- [x] **Step 3: Write minimal implementation**

Implement finite-number guards, percentage calculations and a stable descending sort; do not add persistence or estimated values yet.

- [x] **Step 4: Run test to verify it passes**

Run: `npm test -- app/lib/portfolio-kpi.test.ts` from `apps/web`.
Expected: PASS.

- [x] **Step 5: Integrate the existing Overview page**

Rename its visible heading to Portfolio Monitoring and expose the pure KPI helpers only for values already available from the current dashboard response. Do not invent revenue/target data until the API has configurable KPI inputs.

### Task 3: Add plant lifecycle and asset-type fields without changing site connectivity

**Files:**
- Create: `services/platform-api/internal/database/migrations/000042_plant_lifecycle.sql`
- Modify: `services/platform-api/internal/core/plants.go`
- Modify: `services/platform-api/internal/httpapi/plants.go`
- Modify: `apps/web/app/lib/types.ts`
- Modify: `apps/web/app/features/plants/plants-page.tsx`
- Create: `services/platform-api/internal/core/plant_lifecycle_test.go`

**Interfaces:**
- Plant lifecycle values: `IN_CONSTRUCTION`, `OPERATIONAL`, `OFFLINE`, `RETIRED`.
- Plant API exposes `lifecycleStatus` and accepts it on create/update.
- Device API exposes its existing model/device type for asset-type filtering.

- [x] **Step 1: Write the failing domain test**

Test lifecycle validation accepts the four values and rejects unknown values; run `go test ./internal/core -run TestPlantLifecycle -v` from `services/platform-api` and confirm failure.

- [x] **Step 2: Add migration and API validation**

Add a non-null lifecycle column with default `OPERATIONAL`, update the Go DTO/request validation and generated database access required by the repository's sqlc workflow. Preserve existing records and Middleware/device links.

- [x] **Step 3: Add the UI filter and lifecycle display**

Add a lifecycle filter and status column to Plant & Asset Manager; keep the existing configurable plant list and device association flow unchanged.

- [x] **Step 4: Run focused and regression tests**

Run the focused Go test, the existing plant HTTP/core tests, and the web typecheck.

### Follow-up plans

After Tasks 1–3 are green, create one separate plan per remaining vertical slice so each can be reviewed and verified independently: Analytics (XY Scatter/Solar Power Curve), Event Logbook, Reports, and deployment persistence/17-site acceptance. This avoids coupling unrelated schema changes and keeps each migration independently testable.

Implementation note: the Analytics slice has now also been started in the existing Energy Analysis page. It includes tested timestamp-aligned scatter transformation plus Trend Viewer, XY Scatter and Solar Power Curve tabs; Event Logbook, Reports and deployment persistence remain separate follow-up slices.

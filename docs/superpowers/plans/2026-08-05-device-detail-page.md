# Device Detail Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `Eye` button's current-values dialog on the Plant/Device page with a full-page Device Detail view showing current values with units and a filterable multi-series telemetry history chart.

**Architecture:** All backend endpoints already exist (`device-register-metadata`, `telemetry/history`). This is a frontend-only change confined to `apps/web/app/features/plants/plants-page.tsx`: add a `DeviceHistoryChart` sub-component that extends the existing hand-rolled SVG polyline approach (already used in `overview-page.tsx`) to multiple series, then add a `DeviceDetailView` full-page component (same swap pattern already used for `selectedPlant`) that uses it, removing the now-dead `DeviceLatestDialog`.

**Tech Stack:** Next.js/React (client component), existing `api()`/`errorMessage()` helpers, existing `Select`/`Button` UI primitives, Tailwind utility classes (project uses CSS-variable-backed Tailwind v4 tokens: `--color-brand`, `--color-ink`), lucide-react icons. No new dependencies.

## Global Constraints

- No new npm dependency — reuse the existing hand-rolled SVG chart pattern from `overview-page.tsx`.
- No new route — reuse the existing full-page-swap-via-state pattern (`selectedPlant` → `DeviceManagement` in the same file).
- Chart Y-axis: single shared axis for all selected series (per approved design — no per-unit axis splitting).
- Units/display names come from `GET /api/v1/plants/{plantId}/device-register-metadata/{deviceId}`; fall back to the raw `addressKey` with no unit suffix when no metadata entry exists.
- Delete `DeviceLatestDialog` entirely once its only call site is replaced — don't leave dead code.
- This app has no test runner (`apps/web/package.json` has no `test` script, no jest/vitest/testing-library). Verification steps use `npm run typecheck` plus manual browser checks, per existing precedent (see `docs/superpowers/plans/2026-08-03-site-map-clusters-and-plant-image.md` Task 4).

---

### Task 1: Filterable multi-series telemetry history chart component

**Files:**
- Modify: `apps/web/app/features/plants/plants-page.tsx`
- Test: `apps/web` typecheck

**Interfaces:**
- Consumes: existing `Plant`, `Device`, `RegisterMetadata`, `TelemetryHistoryPage` types from `../../lib/types`; existing `api`, `errorMessage` from `../../lib/api`; existing `Select`/`SelectTrigger`/`SelectValue`/`SelectContent`/`SelectItem` from `../../components/ui/select`.
- Produces: `DeviceHistoryChart({ plant, device, metadataByKey, availableKeys }: { plant: Plant; device: Device; metadataByKey: Record<string, RegisterMetadata>; availableKeys: string[] })` — a self-contained panel with parameter checklist, time-range picker, and multi-series SVG chart. Consumed by `DeviceDetailView` in Task 2.

- [ ] **Step 1: Add `RegisterMetadata` and `TelemetryHistoryPage` to the type import**

In `apps/web/app/features/plants/plants-page.tsx`, change:

```ts
import type { Device, DeviceModelOption, LatestTelemetry, Plant } from "../../lib/types";
```

to:

```ts
import type { Device, DeviceModelOption, LatestTelemetry, Plant, RegisterMetadata, TelemetryHistoryPage } from "../../lib/types";
```

- [ ] **Step 2: Run typecheck to confirm the import resolves**

Run: `cd apps/web && npm run typecheck`
Expected: PASS — both types are already exported from `lib/types.ts`; this confirms the import path before they're used.

- [ ] **Step 3: Add the `DeviceHistoryChart` component**

Add this new function to `apps/web/app/features/plants/plants-page.tsx`, placed after the closing brace of `DeviceManagement` (before `DeviceLatestDialog`, which Task 2 removes):

```tsx
const CHART_COLORS = ["#2563eb", "#16a34a", "#d97706", "#dc2626", "#7c3aed", "#0891b2"];
const CHART_RANGES = [
  { label: "1 ชั่วโมง", hours: 1 as const },
  { label: "6 ชั่วโมง", hours: 6 as const },
  { label: "24 ชั่วโมง", hours: 24 as const },
  { label: "7 วัน", hours: 168 as const },
];

function DeviceHistoryChart({ plant, device, metadataByKey, availableKeys }: { plant: Plant; device: Device; metadataByKey: Record<string, RegisterMetadata>; availableKeys: string[] }) {
  const [selectedKeys, setSelectedKeys] = useState<string[]>(() => availableKeys.slice(0, 1));
  const [rangeHours, setRangeHours] = useState<1 | 6 | 24 | 168>(24);
  const [series, setSeries] = useState<Record<string, Array<{ at: string; value: number }>>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setSelectedKeys((current) => current.filter((key) => availableKeys.includes(key)));
  }, [availableKeys]);

  useEffect(() => {
    if (selectedKeys.length === 0) {
      setSeries({});
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void (async () => {
      const to = new Date();
      const from = new Date(to.getTime() - rangeHours * 60 * 60 * 1000);
      const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), limit: "500" });
      try {
        const response = await api(`/api/v1/plants/${plant.id}/devices/${device.id}/telemetry/history?${query}`, { signal: controller.signal });
        if (!response.ok) throw new Error("ไม่สามารถโหลดข้อมูลกราฟได้");
        const page = (await response.json()) as TelemetryHistoryPage;
        const next: Record<string, Array<{ at: string; value: number }>> = {};
        for (const key of selectedKeys) {
          next[key] = page.data.flatMap((entry) => {
            const value = entry.dataItemMap[key];
            return Number.isFinite(value) ? [{ at: entry.observedAt, value }] : [];
          }).reverse();
        }
        setSeries(next);
      } catch (cause) {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [plant.id, device.id, selectedKeys, rangeHours]);

  function toggleKey(key: string) {
    setSelectedKeys((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key]);
  }

  const allValues = Object.values(series).flat().map((point) => point.value);
  const minimum = allValues.length ? Math.min(...allValues) : 0;
  const maximum = allValues.length ? Math.max(...allValues) : 0;
  const range = maximum - minimum || 1;

  return (
    <section className="device-chart-panel">
      <div className="section-heading">
        <div><p>Telemetry history</p><h3>กราฟค่าย้อนหลัง</h3></div>
        <Select value={String(rangeHours)} onValueChange={(value) => setRangeHours(Number(value) as 1 | 6 | 24 | 168)}>
          <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
          <SelectContent>{CHART_RANGES.map((option) => <SelectItem key={option.hours} value={String(option.hours)}>{option.label}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <div className="flex flex-wrap gap-2 px-1 pb-3">
        {availableKeys.map((key) => {
          const meta = metadataByKey[key];
          const active = selectedKeys.includes(key);
          return (
            <button
              key={key}
              type="button"
              onClick={() => toggleKey(key)}
              className={active ? "rounded-full border border-brand bg-brand/10 px-3 py-1 text-xs font-bold text-brand" : "rounded-full border border-slate-200 px-3 py-1 text-xs text-slate-500"}
            >
              {meta?.displayName || key}{meta?.unit ? ` (${meta.unit})` : ""}
            </button>
          );
        })}
        {availableKeys.length === 0 && <span className="text-xs text-slate-500">ยังไม่มี Parameter ให้เลือก</span>}
      </div>
      {error && <p className="form-message error">{error}</p>}
      {!error && selectedKeys.length === 0 && <div className="table-state">เลือก Parameter อย่างน้อยหนึ่งตัวเพื่อดูกราฟ</div>}
      {!error && selectedKeys.length > 0 && !loading && allValues.length === 0 && <div className="table-state">ไม่มีข้อมูลในช่วงเวลานี้</div>}
      {!error && allValues.length > 0 && <>
        <svg className="h-64 w-full" viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label="กราฟ telemetry ย้อนหลัง">
          <line x1="0" y1="36" x2="100" y2="36" stroke="#e2e8f0" strokeWidth="0.3" />
          {selectedKeys.map((key, index) => {
            const points = series[key] ?? [];
            if (points.length === 0) return null;
            const polyline = points.map((point, pointIndex) => `${points.length === 1 ? 50 : pointIndex * 100 / (points.length - 1)},${36 - (point.value - minimum) * 32 / range}`).join(" ");
            return <polyline key={key} points={polyline} fill="none" stroke={CHART_COLORS[index % CHART_COLORS.length]} strokeWidth="0.6" />;
          })}
        </svg>
        <div className="flex flex-wrap gap-4 px-1 pt-2 text-xs text-slate-500">
          {selectedKeys.map((key, index) => {
            const meta = metadataByKey[key];
            const points = series[key] ?? [];
            const latestValue = points.length ? points[points.length - 1].value : undefined;
            return (
              <span key={key} className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full" style={{ background: CHART_COLORS[index % CHART_COLORS.length] }} />
                {meta?.displayName || key}: <strong className="text-slate-900">{latestValue == null ? "-" : latestValue.toLocaleString(undefined, meta ? { minimumFractionDigits: meta.decimals, maximumFractionDigits: meta.decimals } : undefined)}{meta?.unit ? ` ${meta.unit}` : ""}</strong>
              </span>
            );
          })}
        </div>
      </>}
    </section>
  );
}
```

- [ ] **Step 4: Run typecheck to confirm the whole file still passes**

Run: `cd apps/web && npm run typecheck`
Expected: PASS — `DeviceHistoryChart` is self-contained and not referenced anywhere yet, so it compiles standalone.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/features/plants/plants-page.tsx
git commit -m "$(cat <<'EOF'
feat(web): add multi-series telemetry history chart component

Parameter checklist + time-range picker (1h/6h/24h/7d) drive a
multi-series SVG chart reusing the single-shared-y-axis approach
already used by the Overview timeseries widget. Not wired into any
page yet — that's the next commit.
EOF
)"
```

---

### Task 2: Full-page Device Detail view wired to the chart, replacing the dialog

**Files:**
- Modify: `apps/web/app/features/plants/plants-page.tsx`
- Test: `apps/web` typecheck + manual browser check

**Interfaces:**
- Consumes: `DeviceHistoryChart` produced in Task 1; existing `Plant`, `Device`, `LatestTelemetry`, `RegisterMetadata` types; existing `api`, `errorMessage`.
- Produces: `DeviceDetailView({ plant, device, reading, onBack }: { plant: Plant; device: Device; reading?: LatestTelemetry; onBack: () => void })`, rendered by `DeviceManagement` in place of `DeviceLatestDialog` when `selectedDevice` is set.

- [ ] **Step 1: Add the `DeviceDetailView` component**

Add this new function to `apps/web/app/features/plants/plants-page.tsx`, placed right after `DeviceManagement`'s closing brace (before `DeviceHistoryChart`, or anywhere after it — order between the two doesn't matter since both are module-level functions):

```tsx
function DeviceDetailView({ plant, device, reading, onBack }: { plant: Plant; device: Device; reading?: LatestTelemetry; onBack: () => void }) {
  const [metadata, setMetadata] = useState<RegisterMetadata[]>([]);
  const [metadataError, setMetadataError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const response = await api(`/api/v1/plants/${plant.id}/device-register-metadata/${device.id}`);
      if (cancelled) return;
      if (!response.ok) {
        setMetadataError("ไม่สามารถโหลด Unit ของ Parameter ได้");
        return;
      }
      setMetadata((await response.json()) as RegisterMetadata[]);
    })();
    return () => { cancelled = true; };
  }, [plant.id, device.id]);

  const metadataByKey = Object.fromEntries(metadata.map((item) => [item.addressKey, item]));
  const values = Object.entries(reading?.dataItemMap ?? {}).sort(([a], [b]) => a.localeCompare(b));
  const availableKeys = values.map(([key]) => key);

  return (
    <div className="content device-detail-content">
      <div className="section-heading">
        <div className="registry-title">
          <Button variant="icon" onClick={onBack} title="กลับไป Device" aria-label="กลับไป Device"><ArrowLeft size={18} /></Button>
          <div><p>{plant.code} · {device.externalId}</p><h2>{device.name}</h2></div>
        </div>
      </div>
      {metadataError && <p className="form-message error">{metadataError}</p>}
      <section className="grid gap-px overflow-hidden rounded-md border border-slate-200 bg-slate-200 sm:grid-cols-2 xl:grid-cols-4" aria-label="ค่าปัจจุบัน">
        {values.length === 0 && <div className="table-state col-span-full">ยังไม่มี telemetry สำหรับ Device นี้</div>}
        {values.map(([key, value]) => {
          const meta = metadataByKey[key];
          return (
            <div className="bg-white p-4" key={key}>
              <small className="font-bold text-slate-500">{meta?.displayName || key}</small>
              <strong className="mt-1 block text-xl text-slate-900">
                {Number.isFinite(value) ? value.toLocaleString(undefined, meta ? { minimumFractionDigits: meta.decimals, maximumFractionDigits: meta.decimals } : undefined) : "-"}
                {meta?.unit ? <span className="ml-1 text-sm font-normal text-slate-500">{meta.unit}</span> : null}
              </strong>
              <span className="text-xs text-slate-500">{key}</span>
            </div>
          );
        })}
      </section>
      <DeviceHistoryChart plant={plant} device={device} metadataByKey={metadataByKey} availableKeys={availableKeys} />
    </div>
  );
}
```

- [ ] **Step 2: Run typecheck to confirm it compiles standalone**

Run: `cd apps/web && npm run typecheck`
Expected: PASS — `DeviceDetailView` isn't wired into `DeviceManagement` yet, but it's fully self-contained and type-correct on its own.

- [ ] **Step 3: Wire the full-page swap into `DeviceManagement`**

In `DeviceManagement`, find:

```tsx
  const latestReadings = Object.values(latestByDevice);
  const lastObservedAt = latestReadings.reduce<string | undefined>((latest, reading) => !latest || reading.observedAt > latest ? reading.observedAt : latest, undefined);

  return (
```

and change it to (mirrors the existing `if (selectedPlant) return <DeviceManagement .../>` pattern in `PlantsPage`):

```tsx
  const latestReadings = Object.values(latestByDevice);
  const lastObservedAt = latestReadings.reduce<string | undefined>((latest, reading) => !latest || reading.observedAt > latest ? reading.observedAt : latest, undefined);

  if (selectedDevice) {
    return (
      <DeviceDetailView
        plant={plant}
        device={selectedDevice}
        reading={latestByDevice[selectedDevice.id]}
        onBack={() => setSelectedDevice(null)}
      />
    );
  }

  return (
```

- [ ] **Step 4: Remove the dialog render and the dead `DeviceLatestDialog` component**

Delete this line from `DeviceManagement`'s JSX (now unreachable — Step 3's early return handles `selectedDevice`):

```tsx
      {selectedDevice && <DeviceLatestDialog device={selectedDevice} reading={latestByDevice[selectedDevice.id]} onClose={() => setSelectedDevice(null)} />}
```

Then delete the entire `DeviceLatestDialog` function:

```tsx
function DeviceLatestDialog({ device, reading, onClose }: { device: Device; reading?: LatestTelemetry; onClose: () => void }) {
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>{device.externalId}</DialogDescription><DialogTitle>ค่าล่าสุดของ {device.name}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          {!reading ? <div className="table-state">ยังไม่มี telemetry สำหรับ Device นี้</div> : <>
            <div className="grid grid-cols-2 gap-3 px-5 py-4 text-sm"><div><small className="block text-slate-500">Observed</small><strong>{formatDate(reading.observedAt)}</strong></div><div><small className="block text-slate-500">Received</small><strong>{formatDate(reading.receivedAt)}</strong></div></div>
            <div className="max-h-[55vh] overflow-auto border-t border-slate-200 px-5 py-3">
              {Object.entries(reading.dataItemMap).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => <div key={key} className="flex items-center justify-between gap-4 border-b border-slate-100 py-2 text-sm last:border-0"><code className="text-slate-600">{key}</code><strong className="text-slate-900">{Number.isFinite(value) ? value.toLocaleString() : "-"}</strong></div>)}
            </div>
          </>}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 5: Run typecheck to confirm the whole file passes clean**

Run: `cd apps/web && npm run typecheck`
Expected: PASS — no dangling references to `DeviceLatestDialog`, `DeviceDetailView` fully wired.

- [ ] **Step 6: Manual browser verification**

Run: `cd apps/web && npm run dev`, sign in, go to Plant/Device, open a Plant with at least one device that has telemetry.

Check:
1. Click the `Eye` button on a device with telemetry → full page opens (not a dialog), back button returns to the device list.
2. Current values grid shows unit next to values that have register metadata; devices/points without metadata show the raw key with no unit.
3. Chart panel: one parameter is pre-selected by default; toggling parameter chips adds/removes lines; changing the time range refetches and redraws.
4. Select 2+ parameters with different units (e.g. Voltage + Power) — both lines render on the same axis without crashing.
5. Open a device with **no** telemetry (`reading` undefined) — values grid and chart both show empty states, no crash.
6. Open a device whose model has no register metadata rows — values show raw keys with no unit, chart parameter chips show raw keys, no crash.

- [ ] **Step 7: Commit**

```bash
git add apps/web/app/features/plants/plants-page.tsx
git commit -m "$(cat <<'EOF'
feat(web): replace device latest-values dialog with full detail page

Eye button now navigates to a full Device Detail view: current values
decorated with unit/display-name from device-register-metadata, plus
the filterable multi-series history chart. Removes the now-dead
DeviceLatestDialog modal it replaces.
EOF
)"
```

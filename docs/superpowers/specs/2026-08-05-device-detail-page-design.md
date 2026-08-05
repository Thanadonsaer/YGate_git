# Device Detail Page — Design

Sub-project 1 of 3 (Plant/Device UX request). Others (Energy Analysis page,
Overview summary charts) are separate specs, not covered here.

## Goal

From `Plant/Device` management, clicking a device's `Eye` action opens a full
detail page (not a dialog) showing current values with units, and a
filterable multi-series time-series chart.

## Architecture

Extend `apps/web/app/features/plants/plants-page.tsx`. No new route, no new
dependency. `DeviceManagement`'s existing `selectedDevice` state now renders
a new `DeviceDetailView` component instead of `DeviceLatestDialog`, using the
same full-page swap pattern already used for `selectedPlant` →
`DeviceManagement`.

`DeviceLatestDialog` and `TestReadDialog` are unaffected — only the `Eye`
button's target changes.

## Data

- **Current values**: passed down from parent's existing
  `latestByDevice[device.id]` (`LatestTelemetry`). No new fetch.
- **Units / display names**: `GET /api/v1/plants/{plantId}/device-register-metadata/{deviceId}`
  → `RegisterMetadata[]`. Build `addressKey -> { displayName, unit, decimals }`
  map. If no metadata entry for a key, fall back to the raw addressKey and no
  unit suffix.
- **Chart history**: `GET /api/v1/plants/{plantId}/devices/{deviceId}/telemetry/history?from&to&limit=500`,
  called per selection change (time range or parameter toggle re-derives
  `from`/`to`, refetches). No cursor pagination — same precedent as the
  existing Overview `TimeseriesWidget`; 500 rows covers all offered ranges at
  typical reporting intervals.

## Components

1. **Header** — back button (returns to `DeviceManagement`), device name +
   model, plant code. Matches existing header pattern in the file.
2. **Current values grid** — one card per parameter present in
   `dataItemMap`: displayName (or key), value, unit. Reuses existing grid/card
   CSS classes from the plant-summary section above `DeviceManagement`.
3. **Chart panel**:
   - Parameter multi-select: checklist built from register metadata (or raw
     keys if no metadata), each togglable independently.
   - Time range select: 1h / 6h / 24h / 7d (same options as
     `TimeseriesConfigEditor` in `overview-page.tsx`).
   - Chart: extends the existing hand-rolled SVG polyline approach in
     `overview-page.tsx` to multiple series — fixed small color palette
     (~6 colors, cycled), single shared Y-axis (no per-unit axis splitting),
     legend line shows parameter name + unit per series.

## Error handling

Matches existing file conventions: try/catch around fetches, Thai error
strings in `<p className="form-message error">`, empty/loading states via
`.table-state`. No telemetry for the device → empty chart/grid state, not a
crash.

## Out of scope

- Per-device register metadata unit editing (already exists on Register
  Metadata page).
- Any change to the existing `LatestValues` inline table peek (unaffected —
  still shows key: value without unit, per current behavior); adding units
  there is a nice-to-have not required by this spec.
- Pagination beyond 500 rows / requests exceeding 31-day range.

## Testing

Manual, no automated test framework exists for this app currently:
1. Open a device with active telemetry: values show with correct units,
   toggle parameters and time ranges, confirm chart redraws correctly.
2. Open a device with no telemetry: confirm empty states render, no crash.
3. Open a device whose model has no register metadata: confirm raw keys show
   with no unit suffix, no crash.

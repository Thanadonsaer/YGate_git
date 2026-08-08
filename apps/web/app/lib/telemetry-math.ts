// Pure helpers behind the telemetry charts. Deliberately import-free so
// `node --experimental-strip-types --test` can run app/lib/*.test.ts directly
// without a bundler resolving extensionless imports.

export type Point = { t: number; v: number };

/** Points plotted per series. Above this the SVG path stops being readable. */
export const MAX_PLOT_POINTS = 600;

const POWER_TO_KW: Record<string, number> = { w: 0.001, kw: 1, mw: 1000 };
const ENERGY_TO_KWH: Record<string, number> = { wh: 0.001, kwh: 1, mwh: 1000, gwh: 1_000_000 };

export type UnitKind = "power" | "energy" | "other";

export function classifyUnit(unit: string): UnitKind {
  const normalized = unit.trim().toLowerCase();
  if (normalized in POWER_TO_KW) return "power";
  if (normalized in ENERGY_TO_KWH) return "energy";
  return "other";
}

export function toSeries(readings: Array<{ observedAt: string; dataItemMap: Record<string, number> }>) {
  const byKey: Record<string, Point[]> = {};
  for (const reading of readings) {
    const t = Date.parse(reading.observedAt);
    if (Number.isNaN(t)) continue;
    for (const [key, value] of Object.entries(reading.dataItemMap)) {
      if (!Number.isFinite(value)) continue;
      (byKey[key] ??= []).push({ t, v: value });
    }
  }
  for (const points of Object.values(byKey)) points.sort((a, b) => a.t - b.t);
  return byKey;
}

/**
 * Energy over the window, in kWh.
 *
 * A kW/W/MW series is instantaneous power, so integrate it with the trapezoid
 * rule. A kWh/Wh/MWh series is a counter, so sum only the positive deltas --
 * that covers a lifetime counter (sum == last - first) and a daily counter that
 * resets at midnight, without a reset being read as a huge negative. Returns
 * null when the unit is neither, because there is no meaningful energy for it.
 */
export function totalEnergyKWh(points: Point[], unit: string): number | null {
  const normalized = unit.trim().toLowerCase();
  const powerFactor = POWER_TO_KW[normalized];
  const energyFactor = ENERGY_TO_KWH[normalized];
  if (powerFactor === undefined && energyFactor === undefined) return null;
  let total = 0;
  for (let index = 1; index < points.length; index++) {
    total += segmentKWh(points[index - 1], points[index], powerFactor, energyFactor);
  }
  return total;
}

function segmentKWh(previous: Point, current: Point, powerFactor?: number, energyFactor?: number) {
  if (powerFactor !== undefined) {
    return ((current.v + previous.v) / 2) * powerFactor * ((current.t - previous.t) / 3_600_000);
  }
  const delta = current.v - previous.v;
  return delta > 0 ? delta * (energyFactor ?? 1) : 0;
}

export type Bucket = { key: string; start: number; kwh: number };

/**
 * Energy grouped into hour or day buckets, in the browser's local timezone --
 * the same timezone the `datetime-local` range inputs and formatDate already
 * use, so every time on the page agrees. Each segment's energy is attributed to
 * the bucket its earlier sample falls in.
 */
export function bucketEnergy(points: Point[], unit: string, step: "hour" | "day"): Bucket[] {
  const normalized = unit.trim().toLowerCase();
  const powerFactor = POWER_TO_KW[normalized];
  const energyFactor = ENERGY_TO_KWH[normalized];
  if (powerFactor === undefined && energyFactor === undefined) return [];
  const totals = new Map<string, Bucket>();
  for (let index = 1; index < points.length; index++) {
    const previous = points[index - 1];
    const kwh = segmentKWh(previous, points[index], powerFactor, energyFactor);
    const key = bucketKey(previous.t, step);
    const bucket = totals.get(key);
    if (bucket) bucket.kwh += kwh;
    else totals.set(key, { key, start: bucketStart(previous.t, step), kwh });
  }
  return [...totals.values()].sort((a, b) => a.start - b.start);
}

export function bucketKey(t: number, step: "hour" | "day") {
  const date = new Date(t);
  const pad = (value: number) => String(value).padStart(2, "0");
  const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
  return step === "day" ? day : `${day}T${pad(date.getHours())}`;
}

function bucketStart(t: number, step: "hour" | "day") {
  const date = new Date(t);
  date.setMinutes(0, 0, 0);
  if (step === "day") date.setHours(0);
  return date.getTime();
}

/**
 * Largest-Triangle-Three-Buckets. Averaging would flatten the midday peak and
 * the evening drop-off, which is exactly what someone reads a PV curve for, so
 * this keeps the extremes instead.
 */
export function downsample(points: Point[], threshold = MAX_PLOT_POINTS): Point[] {
  if (threshold < 3 || points.length <= threshold) return points;
  const bucketSize = (points.length - 2) / (threshold - 2);
  const sampled: Point[] = [points[0]];
  let anchorIndex = 0;
  for (let bucket = 0; bucket < threshold - 2; bucket++) {
    const avgStart = Math.floor((bucket + 1) * bucketSize) + 1;
    const avgEnd = Math.min(Math.floor((bucket + 2) * bucketSize) + 1, points.length);
    let avgT = 0;
    let avgV = 0;
    for (let index = avgStart; index < avgEnd; index++) {
      avgT += points[index].t;
      avgV += points[index].v;
    }
    const avgCount = Math.max(avgEnd - avgStart, 1);
    avgT /= avgCount;
    avgV /= avgCount;

    const rangeStart = Math.floor(bucket * bucketSize) + 1;
    const rangeEnd = Math.min(Math.floor((bucket + 1) * bucketSize) + 1, points.length - 1);
    const anchor = points[anchorIndex];
    let bestArea = -1;
    let bestIndex = rangeStart;
    for (let index = rangeStart; index < rangeEnd; index++) {
      const area = Math.abs(
        (anchor.t - avgT) * (points[index].v - anchor.v) - (anchor.t - points[index].t) * (avgV - anchor.v),
      );
      if (area > bestArea) {
        bestArea = area;
        bestIndex = index;
      }
    }
    sampled.push(points[bestIndex]);
    anchorIndex = bestIndex;
  }
  sampled.push(points[points.length - 1]);
  return sampled;
}

const TIME_STEPS = [
  60_000, 5 * 60_000, 15 * 60_000, 30 * 60_000,
  3_600_000, 3 * 3_600_000, 6 * 3_600_000, 12 * 3_600_000,
  86_400_000, 7 * 86_400_000,
];

/**
 * Tick timestamps landing on round local wall-clock times, so zooming re-labels
 * the axis at whatever resolution the new span deserves.
 */
export function timeTicks(from: number, to: number, target = 6): number[] {
  if (!(to > from)) return [];
  const span = to - from;
  const step = TIME_STEPS.find((candidate) => span / candidate <= target) ?? TIME_STEPS[TIME_STEPS.length - 1];
  const offset = new Date(from).getTimezoneOffset() * 60_000;
  const ticks: number[] = [];
  for (let tick = Math.ceil((from + offset) / step) * step - offset; tick <= to; tick += step) {
    ticks.push(tick);
    if (ticks.length >= 64) break;
  }
  return ticks;
}

export function valueTicks(min: number, max: number, target = 5): number[] {
  if (!Number.isFinite(min) || !Number.isFinite(max)) return [];
  if (min === max) return [min];
  const rough = (max - min) / target;
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const step = [1, 2, 2.5, 5, 10].map((multiple) => multiple * magnitude).find((candidate) => candidate >= rough)
    ?? 10 * magnitude;
  const ticks: number[] = [];
  for (let tick = Math.ceil(min / step) * step; tick <= max + step * 1e-9; tick += step) {
    ticks.push(Number(tick.toFixed(10)));
    if (ticks.length >= 24) break;
  }
  return ticks;
}

/** Index of the point nearest `t`; -1 when there are none. Used by the crosshair. */
export function nearestIndex(points: Point[], t: number) {
  if (points.length === 0) return -1;
  let low = 0;
  let high = points.length - 1;
  while (low < high) {
    const middle = (low + high) >> 1;
    if (points[middle].t < t) low = middle + 1;
    else high = middle;
  }
  if (low > 0 && Math.abs(points[low - 1].t - t) <= Math.abs(points[low].t - t)) return low - 1;
  return low;
}

export function seriesToCSV(columns: Array<{ key: string; header: string }>, seriesByKey: Record<string, Point[]>) {
  const stamps = [...new Set(columns.flatMap(({ key }) => (seriesByKey[key] ?? []).map((point) => point.t)))]
    .sort((a, b) => a - b);
  const lookups = columns.map(({ key }) => new Map((seriesByKey[key] ?? []).map((point) => [point.t, point.v])));
  const escape = (value: string) => (/[",\n\r]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value);
  const rows = [["observedAt", ...columns.map((column) => column.header)].map(escape).join(",")];
  for (const stamp of stamps) {
    const values = lookups.map((lookup) => lookup.get(stamp));
    rows.push([
      new Date(stamp).toISOString(),
      ...values.map((value) => (value === undefined ? "" : String(value))),
    ].join(","));
  }
  return rows.join("\r\n");
}

/** Same length as [from, to], ending where it starts -- the "previous period". */
export function previousPeriod(from: Date, to: Date) {
  const span = to.getTime() - from.getTime();
  return { from: new Date(from.getTime() - span), to: new Date(from.getTime()) };
}

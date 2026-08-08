import assert from "node:assert/strict";
import test from "node:test";
import {
  bucketEnergy,
  classifyUnit,
  downsample,
  nearestIndex,
  previousPeriod,
  seriesToCSV,
  timeTicks,
  toSeries,
  totalEnergyKWh,
  valueTicks,
  type Point,
} from "./telemetry-math.ts";

const HOUR = 3_600_000;

/** Local midnight on a fixed day, so the assertions hold in any timezone. */
function at(hoursFromMidnight: number) {
  return new Date(2026, 7, 8, 0, 0, 0, 0).getTime() + hoursFromMidnight * HOUR;
}

test("classifyUnit splits power from counters and leaves everything else alone", () => {
  assert.equal(classifyUnit("kW"), "power");
  assert.equal(classifyUnit(" W "), "power");
  assert.equal(classifyUnit("kWh"), "energy");
  assert.equal(classifyUnit("MWh"), "energy");
  assert.equal(classifyUnit("Hz"), "other");
  assert.equal(classifyUnit(""), "other");
});

test("totalEnergyKWh integrates a power series over real elapsed time", () => {
  // 10 kW held for 2 h is 20 kWh; a 0->20 kW ramp over 1 h averages to 10 kWh.
  assert.equal(totalEnergyKWh([{ t: at(0), v: 10 }, { t: at(2), v: 10 }], "kW"), 20);
  assert.equal(totalEnergyKWh([{ t: at(0), v: 0 }, { t: at(1), v: 20 }], "kW"), 10);
  // Unit scaling: 1000 W for 1 h is 1 kWh, 2 MW for 1 h is 2000 kWh.
  assert.equal(totalEnergyKWh([{ t: at(0), v: 1000 }, { t: at(1), v: 1000 }], "W"), 1);
  assert.equal(totalEnergyKWh([{ t: at(0), v: 2 }, { t: at(1), v: 2 }], "MW"), 2000);
});

test("totalEnergyKWh sums positive counter deltas and ignores a midnight reset", () => {
  // A daily-yield register that reaches 12, resets to 0, then climbs to 5.
  const points: Point[] = [
    { t: at(0), v: 10 },
    { t: at(1), v: 12 },
    { t: at(2), v: 0 },
    { t: at(3), v: 5 },
  ];
  assert.equal(totalEnergyKWh(points, "kWh"), 7);
  assert.ok(totalEnergyKWh(points, "kWh")! > 0, "without the reset guard this goes negative");
});

test("totalEnergyKWh refuses units that are not energy at all", () => {
  assert.equal(totalEnergyKWh([{ t: at(0), v: 50 }, { t: at(1), v: 50 }], "Hz"), null);
  assert.equal(totalEnergyKWh([{ t: at(0), v: 1 }], "kW"), 0, "a single sample spans no time");
});

test("bucketEnergy groups into local hours and keeps the totals consistent", () => {
  const points: Point[] = [
    { t: at(0), v: 10 },
    { t: at(1), v: 10 },
    { t: at(2), v: 30 },
    { t: at(3), v: 30 },
  ];
  const buckets = bucketEnergy(points, "kW", "hour");
  assert.deepEqual(buckets.map((bucket) => bucket.kwh), [10, 20, 30]);
  const summed = buckets.reduce((sum, bucket) => sum + bucket.kwh, 0);
  assert.equal(summed, totalEnergyKWh(points, "kW"), "bars must add up to the KPI above them");
  assert.ok(buckets[0].start < buckets[1].start, "buckets come back in time order");
  assert.equal(bucketEnergy(points, "Hz", "hour").length, 0);
});

test("bucketEnergy collapses a multi-hour series into one day bucket", () => {
  const points: Point[] = [{ t: at(0), v: 10 }, { t: at(1), v: 10 }, { t: at(2), v: 10 }];
  const buckets = bucketEnergy(points, "kW", "day");
  assert.equal(buckets.length, 1);
  assert.equal(buckets[0].kwh, 20);
});

test("downsample honours the budget and keeps the endpoints and the spike", () => {
  const points: Point[] = Array.from({ length: 1000 }, (_, index) => ({ t: at(0) + index * 60_000, v: 1 }));
  points[500] = { t: points[500].t, v: 999 };
  const sampled = downsample(points, 100);
  assert.equal(sampled.length, 100);
  assert.deepEqual(sampled[0], points[0]);
  assert.deepEqual(sampled[sampled.length - 1], points[points.length - 1]);
  assert.ok(sampled.some((point) => point.v === 999), "the peak survives; averaging would erase it");
  assert.ok(sampled.every((point, index) => index === 0 || point.t > sampled[index - 1].t), "stays ordered");
  const short = points.slice(0, 20);
  assert.equal(downsample(short, 100), short, "under budget the array passes straight through");
});

test("timeTicks lands on round local times and coarsens as the span grows", () => {
  const hourly = timeTicks(at(0), at(6));
  assert.ok(hourly.length >= 2 && hourly.length <= 8);
  for (const tick of hourly) {
    const date = new Date(tick);
    assert.equal(date.getMinutes(), 0, "an hour-scale tick sits on the hour");
    assert.equal(date.getSeconds(), 0);
  }
  const monthly = timeTicks(at(0), at(24 * 30));
  assert.ok(monthly.length >= 2 && monthly.length <= 10, "a month does not produce hundreds of ticks");
  assert.ok(monthly[1] - monthly[0] > hourly[1] - hourly[0], "zooming out picks a coarser step");
  assert.deepEqual(timeTicks(at(1), at(0)), [], "an inverted range has no ticks");
});

test("valueTicks produces evenly spaced round steps inside the data range", () => {
  const ticks = valueTicks(0, 97);
  assert.ok(ticks.length >= 3);
  assert.ok(ticks[0] >= 0 && ticks[ticks.length - 1] <= 97);
  const step = ticks[1] - ticks[0];
  assert.ok(ticks.every((tick, index) => index === 0 || Math.abs(tick - ticks[index - 1] - step) < 1e-9));
  assert.deepEqual(valueTicks(5, 5), [5], "a flat series still yields its own line");
});

test("nearestIndex finds the closest sample for the crosshair", () => {
  const points: Point[] = [{ t: 0, v: 1 }, { t: 100, v: 2 }, { t: 200, v: 3 }];
  assert.equal(nearestIndex(points, -50), 0);
  assert.equal(nearestIndex(points, 40), 0);
  assert.equal(nearestIndex(points, 60), 1);
  assert.equal(nearestIndex(points, 500), 2);
  assert.equal(nearestIndex([], 0), -1);
});

test("toSeries orders by time and drops unusable samples", () => {
  const series = toSeries([
    { observedAt: "2026-08-08T02:00:00.000Z", dataItemMap: { pv: 2, bad: Number.NaN } },
    { observedAt: "2026-08-08T01:00:00.000Z", dataItemMap: { pv: 1 } },
    { observedAt: "not-a-date", dataItemMap: { pv: 99 } },
  ]);
  assert.deepEqual(series.pv.map((point) => point.v), [1, 2]);
  assert.equal(series.bad, undefined);
});

test("seriesToCSV aligns columns on timestamps and quotes risky headers", () => {
  const csv = seriesToCSV(
    [{ key: "a", header: "PV Power (kW)" }, { key: "b", header: 'Grid, "AC"' }],
    { a: [{ t: 0, v: 1 }, { t: 1000, v: 2 }], b: [{ t: 1000, v: 9 }] },
  );
  const lines = csv.split("\r\n");
  assert.equal(lines[0], 'observedAt,PV Power (kW),"Grid, ""AC"""');
  assert.equal(lines[1], "1970-01-01T00:00:00.000Z,1,", "a gap in a column stays empty, not zero");
  assert.equal(lines[2], "1970-01-01T00:00:01.000Z,2,9");
});

test("previousPeriod mirrors the window immediately before it", () => {
  const from = new Date(at(24));
  const to = new Date(at(48));
  const previous = previousPeriod(from, to);
  assert.equal(previous.to.getTime(), from.getTime());
  assert.equal(previous.to.getTime() - previous.from.getTime(), to.getTime() - from.getTime());
});

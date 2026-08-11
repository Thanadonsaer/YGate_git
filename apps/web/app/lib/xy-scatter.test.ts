import test from "node:test";
import assert from "node:assert/strict";
import { buildScatterPoints, findSignalKey } from "./xy-scatter.ts";

test("builds timestamp-aligned finite scatter points", () => {
  assert.deepEqual(buildScatterPoints(
    [{ t: 2, v: 20 }, { t: 1, v: 10 }, { t: 3, v: Number.NaN }],
    [{ t: 1, v: 100 }, { t: 2, v: 200 }, { t: 3, v: 300 }],
  ), [
    { x: 10, y: 100, t: 1 },
    { x: 20, y: 200, t: 2 },
  ]);
});

test("finds a mapped signal by register key", () => {
  assert.equal(findSignalKey(["voltage", "irradiance_wm2", "active_power_kw"], /irradiance/i), "irradiance_wm2");
  assert.equal(findSignalKey(["voltage"], /irradiance/i), undefined);
});

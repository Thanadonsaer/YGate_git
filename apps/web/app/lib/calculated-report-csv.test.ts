import assert from "node:assert/strict";
import test from "node:test";
import { calculatedReportCSV } from "./calculated-report-csv.ts";

test("calculated report uses plant and device names", () => {
  const csv = calculatedReportCSV([{ plant: "North", device: "INV-01", observedAt: "2026-08-13T00:00:00Z", values: { power: 12.5 }, kWh: 0.25 }]);
  assert.match(csv, /^Plant,Device,Observed at,power,kWh\r\n/);
  assert.match(csv, /North,INV-01,2026-08-13T00:00:00Z,12.5,0.25/);
});

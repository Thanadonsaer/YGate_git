import test from "node:test";
import assert from "node:assert/strict";
import { navigation, titles } from "./navigation.ts";

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

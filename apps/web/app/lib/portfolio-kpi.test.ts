import test from "node:test";
import assert from "node:assert/strict";
import { calculateCapacityFactor, calculateTargetAchievement, rankPortfolio } from "./portfolio-kpi.ts";

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

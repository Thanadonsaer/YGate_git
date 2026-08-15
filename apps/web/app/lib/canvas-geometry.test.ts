import assert from "node:assert/strict";
import test from "node:test";

import { absolutePosition, lockAxis, relativePosition, reorder, resequenceZ, snapToGuides, type Rect } from "./canvas-geometry.ts";

const target: Rect = { x: 100, y: 100, width: 100, height: 50 };

test("snapToGuides pulls a near-miss onto the neighbour's edge", () => {
  // Dragged 3px short of a left-edge alignment -- inside the 6px threshold.
  const { position, guides } = snapToGuides({ x: 97, y: 300, width: 100, height: 50 }, [target]);
  assert.equal(position.x, 100);
  assert.equal(position.y, 300, "the untouched axis keeps the raw pointer position");
  assert.equal(guides.length, 1);
  assert.equal(guides[0].axis, "x");
  assert.equal(guides[0].at, 100);
  // Vertical gap between the two rects: 300 - (100 + 50).
  assert.equal(guides[0].gap, 150);
});

test("snapToGuides leaves a far drag alone", () => {
  const { position, guides } = snapToGuides({ x: 400, y: 400, width: 100, height: 50 }, [target]);
  assert.deepEqual(position, { x: 400, y: 400 });
  assert.deepEqual(guides, []);
});

test("snapToGuides aligns centres, not just edges", () => {
  // Centre of target is x=150; a 60-wide rect centres there at x=120.
  const { position, guides } = snapToGuides({ x: 122, y: 300, width: 60, height: 40 }, [target]);
  assert.equal(position.x, 120);
  assert.equal(guides[0].at, 150);
});

test("snapToGuides picks the closest candidate when several are in range", () => {
  const near: Rect = { x: 98, y: 400, width: 20, height: 20 };
  const { position } = snapToGuides({ x: 97, y: 300, width: 100, height: 50 }, [target, near]);
  assert.equal(position.x, 98, "1px away beats 3px away");
});

test("snapToGuides reports a zero gap for overlapping rects", () => {
  const { guides } = snapToGuides({ x: 97, y: 120, width: 100, height: 50 }, [target]);
  assert.equal(guides.find((guide) => guide.axis === "x")?.gap, 0);
});

test("reorder drops a layer below the named sibling", () => {
  assert.deepEqual(reorder(["a", "b", "c"], "c", "b"), ["a", "c", "b"]);
  assert.deepEqual(reorder(["a", "b", "c"], "a", null), ["b", "c", "a"], "null drops it on top");
  assert.deepEqual(reorder(["a", "b", "c"], "a", "a"), ["a", "b", "c"], "dropping onto itself is a no-op");
});

test("reorder ignores ids that are not in the list", () => {
  assert.deepEqual(reorder(["a", "b"], "zz", "a"), ["a", "b"]);
  assert.deepEqual(reorder(["a", "b"], "a", "zz"), ["a", "b"]);
});

test("resequenceZ hands out dense indices so repeated drags cannot drift", () => {
  assert.deepEqual(resequenceZ(["a", "b", "c"]), { a: 0, b: 1, c: 2 });
  // Re-running on a reordered list reuses the same range instead of growing it.
  assert.deepEqual(resequenceZ(reorder(["a", "b", "c"], "a", null)), { b: 0, c: 1, a: 2 });
});

test("absolutePosition and relativePosition round-trip through a parent chain", () => {
  const local = { x: 10, y: 20 };
  const absolute = absolutePosition(local, [{ x: 100, y: 200 }, { x: 1000, y: 2000 }]);
  assert.deepEqual(absolute, { x: 1110, y: 2220 });
  assert.deepEqual(relativePosition(absolute, { x: 1100, y: 2200 }), { x: 10, y: 20 });
});

test("lockAxis keeps the dominant axis of a shift-drag", () => {
  assert.deepEqual(lockAxis({ x: 0, y: 0 }, { x: 50, y: 8 }), { x: 50, y: 0 });
  assert.deepEqual(lockAxis({ x: 0, y: 0 }, { x: 8, y: -50 }), { x: 0, y: -50 });
});

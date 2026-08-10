import assert from "node:assert/strict";
import test from "node:test";

import { sameSelection } from "./selection.ts";

test("sameSelection matches equal contents so React can bail out", () => {
  // The empty-vs-empty case is the one that caused the loop: an idle canvas
  // re-reported "nothing selected" as a brand new [] on every store sync.
  assert.equal(sameSelection([], []), true);
  assert.equal(sameSelection(["a"], ["a"]), true);
  assert.equal(sameSelection(["a", "b"], ["a", "b"]), true);
});

test("sameSelection rejects real selection changes", () => {
  assert.equal(sameSelection([], ["a"]), false);
  assert.equal(sameSelection(["a"], []), false);
  assert.equal(sameSelection(["a", "b"], ["a"]), false);
  assert.equal(sameSelection(["a", "b"], ["b", "a"]), false);
});

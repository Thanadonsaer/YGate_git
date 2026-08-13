import test from "node:test";
import assert from "node:assert/strict";
import { middlewareProgressLabel } from "./middleware-progress.ts";

test("labels the active Middleware software update operation", () => {
  assert.equal(middlewareProgressLabel({ uploading: false, staging: false, applying: true, rollingBack: false, restarting: false }), "กำลัง Apply patch...");
});

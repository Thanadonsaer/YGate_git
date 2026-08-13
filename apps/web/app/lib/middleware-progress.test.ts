import test from "node:test";
import assert from "node:assert/strict";
import { estimateRemainingMs, middlewareProgressLabel } from "./middleware-progress.ts";

test("labels the active Middleware software update operation", () => {
  assert.equal(middlewareProgressLabel({ uploading: false, staging: true, applying: false, rollingBack: false, restarting: false }), "กำลังส่งไปยัง Middleware...");
});

test("estimates remaining middleware update time from completed gateways", () => {
  assert.equal(estimateRemainingMs([
    { status: "succeeded", durationMs: 1000 },
    { status: "failed", durationMs: 3000 },
    { status: "running" },
    { status: "queued" },
  ]), 4000);
});

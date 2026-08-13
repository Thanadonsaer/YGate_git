import test from "node:test";
import assert from "node:assert/strict";
import { estimateDownloadRemainingMs, estimateRemainingMs, middlewareProgressLabel } from "./middleware-progress.ts";

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

test("estimates download ETA from transferred bytes and elapsed time", () => {
  const now = Date.parse("2026-08-13T10:01:00.000Z");
  assert.equal(estimateDownloadRemainingMs([
    { status: "running", startedAt: "2026-08-13T10:00:00.000Z", downloadedBytes: 25, totalBytes: 100 },
  ], now), 180000);
});

test("does not show download ETA before any bytes arrive", () => {
  assert.equal(estimateDownloadRemainingMs([
    { status: "running", startedAt: "2026-08-13T10:00:00.000Z", downloadedBytes: 0, totalBytes: 100 },
  ], Date.parse("2026-08-13T10:01:00.000Z")), null);
});

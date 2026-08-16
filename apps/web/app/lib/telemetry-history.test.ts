import assert from "node:assert/strict";
import { test } from "node:test";
import { bareAddressKey, fetchRanges } from "./telemetry-history.ts";

// Register Metadata is keyed "reg40072" but telemetry.mapped_data_items()
// emits the bare "40072", so the catalog has to answer to both spellings or
// every parameter picker falls back to showing the raw address.
test("bareAddressKey aliases register keys, and only register keys", () => {
  assert.equal(bareAddressKey("reg40072"), "40072");
  assert.equal(bareAddressKey("reg2080_fc4"), "2080_fc4");

  // A curated named key must not alias itself to a truncated string.
  assert.equal(bareAddressKey("regulation_mode"), "");
  assert.equal(bareAddressKey("active_power"), "");
  assert.equal(bareAddressKey("40072"), "");
});

// Eight devices selected at once meant eight concurrent history walks, which
// exhausted the browser's per-origin connection budget and 502'd the API. The
// fan-out has to stay bounded no matter how many devices are picked -- and the
// results still have to line up with deviceIds, since the caller reads them by
// index.
test("fetchRanges bounds the fan-out and keeps device order", async (t) => {
  const original = globalThis.fetch;
  t.after(() => {
    globalThis.fetch = original;
  });

  let inFlight = 0;
  let peak = 0;
  const order: string[] = [];
  globalThis.fetch = async (input: Request | URL | string) => {
    const deviceId = /devices\/([^/]+)\//.exec(String(input))?.[1] ?? "";
    order.push(deviceId);
    inFlight += 1;
    peak = Math.max(peak, inFlight);
    await new Promise((resolve) => setTimeout(resolve, 5));
    inFlight -= 1;
    return new Response(JSON.stringify({ data: [{ deviceId }], nextCursor: null }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  const deviceIds = ["d1", "d2", "d3", "d4", "d5", "d6", "d7", "d8"];
  const pages = await fetchRanges("plant-1", deviceIds, new Date(0), new Date(1));

  assert.ok(peak <= 3, `at most 3 walks in flight, saw ${peak}`);
  assert.equal(order.length, deviceIds.length, "every device is still fetched");
  assert.deepEqual(
    pages.map((page) => page.readings[0]?.deviceId),
    deviceIds,
    "results stay in deviceIds order, since the caller zips them by index",
  );
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { bareAddressKey } from "./telemetry-history.ts";

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

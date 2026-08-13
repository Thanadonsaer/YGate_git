import assert from "node:assert/strict";
import test from "node:test";
import { can, isSystemAdmin } from "./permissions.ts";

test("can checks a resource action grant", () => {
  const user = { permissions: ["plant:update"] } as never;
  assert.equal(can(user, "plant", "update"), true);
  assert.equal(can(user, "plant", "delete"), false);
});

test("isSystemAdmin uses the global organization permission", () => {
  assert.equal(isSystemAdmin({ permissions: ["organization:create"] } as never), true);
  assert.equal(isSystemAdmin({ permissions: ["plant:create"] } as never), false);
});

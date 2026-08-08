import assert from "node:assert/strict";
import test from "node:test";
import { api, onSessionExpired } from "./api.ts";

test("api reports an expired session on 401, except for a rejected login", async (t) => {
  const original = globalThis.fetch;
  t.after(() => {
    globalThis.fetch = original;
  });

  let status = 401;
  globalThis.fetch = async () => new Response(null, { status });

  let expired = 0;
  const stop = onSessionExpired(() => {
    expired += 1;
  });

  await api("/api/v1/plants");
  assert.equal(expired, 1, "a 401 on a normal request means the session died");

  await api("/api/v1/auth/login", { method: "POST", body: "{}" });
  assert.equal(expired, 1, "a rejected login is wrong credentials, not an expired session");

  status = 200;
  await api("/api/v1/plants");
  assert.equal(expired, 1, "a successful request must not bounce anyone to login");

  status = 401;
  stop();
  await api("/api/v1/plants");
  assert.equal(expired, 1, "unsubscribing stops the notifications");
});

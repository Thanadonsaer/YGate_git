import assert from "node:assert/strict";
import test from "node:test";
import { api, ApiError, apiJson, onSessionExpired } from "./api.ts";

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

test("apiJson sends CSRF on writes only, surfaces server errors, and tolerates 204", async (t) => {
  const originalFetch = globalThis.fetch;
  const g = globalThis as { document?: unknown };
  const originalDocument = g.document;
  t.after(() => {
    globalThis.fetch = originalFetch;
    g.document = originalDocument;
  });
  g.document = { cookie: "platform_csrf=tok%3D1" };

  const seen: { method?: string; csrf?: string | null; body?: unknown }[] = [];
  let next = () => new Response(JSON.stringify([{ id: "p1" }]), { status: 200 });
  globalThis.fetch = async (_input, init) => {
    seen.push({ method: init?.method, csrf: new Headers(init?.headers).get("X-CSRF-Token"), body: init?.body });
    return next();
  };

  assert.deepEqual(await apiJson<{ id: string }[]>("/api/v1/plants"), [{ id: "p1" }]);
  assert.equal(seen[0].csrf, null, "GET must not carry the CSRF token");

  next = () => new Response(null, { status: 204 });
  assert.equal(await apiJson("/api/v1/plants", { method: "POST", body: { name: "x" } }), undefined);
  assert.equal(seen[1].csrf, "tok=1");
  assert.equal(seen[1].body, JSON.stringify({ name: "x" }));

  next = () => new Response(JSON.stringify({ message: "ชื่อซ้ำ" }), { status: 409 });
  const conflict = await apiJson("/api/v1/plants", { method: "POST", body: {} }).catch((cause: unknown) => cause);
  assert.ok(conflict instanceof ApiError);
  assert.equal(conflict.status, 409);
  assert.equal(conflict.serverMessage, "ชื่อซ้ำ");

  next = () => new Response("permission denied\n", { status: 403 });
  const denied = await apiJson("/api/v1/plants", { messages: { 403: "ห้าม" } }).catch((cause: unknown) => cause);
  assert.ok(denied instanceof ApiError);
  assert.equal(denied.serverMessage, "permission denied");
  assert.equal(denied.message, "ห้าม", "the page's own wording wins over the server's");
});

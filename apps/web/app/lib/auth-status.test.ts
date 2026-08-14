import assert from "node:assert/strict";
import test from "node:test";
import { loginStatusFromBody, resendVerificationPayload } from "./auth-status.ts";

test("maps a structured login response to a user-facing status", () => {
  assert.deepEqual(loginStatusFromBody({ code: "EMAIL_UNVERIFIED", message: "verify" }), { code: "EMAIL_UNVERIFIED", message: "verify" });
  assert.deepEqual(loginStatusFromBody({ code: "ACCESS_PENDING", message: "pending" }), { code: "ACCESS_PENDING", message: "pending" });
  assert.equal(loginStatusFromBody({ code: "OTHER", message: "no" }), null);
});

test("resend payload uses the entered identifier", () => {
  assert.deepEqual(resendVerificationPayload("  USER@Example.COM "), { identifier: "USER@Example.COM" });
});

export type LoginStatusCode = "EMAIL_UNVERIFIED" | "ACCESS_PENDING" | "ACCOUNT_DISABLED";

export type LoginStatus = { code: LoginStatusCode; message: string };

export function loginStatusFromBody(body: unknown): LoginStatus | null {
  if (!body || typeof body !== "object") return null;
  const value = body as { code?: unknown; message?: unknown };
  if (value.code !== "EMAIL_UNVERIFIED" && value.code !== "ACCESS_PENDING" && value.code !== "ACCOUNT_DISABLED") return null;
  return { code: value.code, message: typeof value.message === "string" ? value.message : "ไม่สามารถเข้าสู่ระบบได้" };
}

export function resendVerificationPayload(identifier: string) {
  return { identifier: identifier.trim() };
}

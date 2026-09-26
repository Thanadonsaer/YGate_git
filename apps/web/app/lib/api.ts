export const gatewayURL = process.env.NEXT_PUBLIC_GATEWAY_URL ?? "http://localhost:44440";

// Site-served assets (e.g. the uploaded logo) come back from the API as a
// relative path -- prefix with the gateway origin so <img src> resolves
// even though the Next.js app and the API gateway aren't the same origin.
export function assetURL(path: string) {
  return `${gatewayURL}${path}`;
}

export function csrfToken() {
  const prefix = "platform_csrf=";
  const cookie = document.cookie.split("; ").find((item) => item.startsWith(prefix));
  return cookie ? decodeURIComponent(cookie.slice(prefix.length)) : "";
}

export function errorMessage(cause: unknown) {
  return cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด";
}

let sessionExpiredHandler: (() => void) | undefined;

/**
 * Register the callback that runs when the server rejects a request because the
 * session is gone. Every authenticated request goes through `api`, so this is
 * the one place that can notice a session expiring mid-visit -- without it the
 * page keeps rendering as if signed in and only fills up with errors. Returns
 * an unsubscribe function so it drops straight into a `useEffect`.
 */
export function onSessionExpired(handler: () => void) {
  sessionExpiredHandler = handler;
  return () => {
    if (sessionExpiredHandler === handler) sessionExpiredHandler = undefined;
  };
}

export async function api(path: string, init?: RequestInit) {
  try {
    const response = await fetch(`${gatewayURL}${path}`, {
      ...init,
      credentials: "include",
      headers: {
        ...(init?.body && !(init.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
    });
    // A 401 from the login form means "wrong credentials", not an expired
    // session -- every other endpoint returns it only once the cookie is dead.
    if (response.status === 401 && !path.startsWith("/api/v1/auth/login")) sessionExpiredHandler?.();
    return response;
  } catch {
    // Gateway unreachable (down, network drop, CORS-origin rejected). Return a
    // non-ok Response instead of letting fetch's rejection propagate, so every
    // caller's existing `if (response.ok) ... else setError(...)` handles it
    // the same way it handles a 4xx/5xx from the server.
    return new Response(null, { status: 503, statusText: "Network error" });
  }
}

/** Per-call wording keyed by HTTP status, plus `default` for any other failure. */
export type ApiErrorMessages = Partial<Record<number | "default", string>>;

const defaultApiErrorMessages: Record<number, string> = {
  401: "เซสชันหมดอายุ กรุณาเข้าสู่ระบบใหม่",
  403: "บัญชีนี้ไม่มีสิทธิ์ทำรายการนี้",
  404: "ไม่พบข้อมูล หรือบัญชีนี้ไม่มีสิทธิ์เข้าถึง",
  409: "ข้อมูลซ้ำหรือถูกแก้ไขจาก session อื่น กรุณาโหลดใหม่",
  503: "ไม่สามารถเชื่อมต่อเซิร์ฟเวอร์ได้",
};

/**
 * The Thai text a page shows for a failed request: the page's own wording for
 * that status wins, then its catch-all, then a generic per-status message, and
 * only then whatever the server said (platform-api errors are mostly terse
 * English `http.Error` text, so it is the last resort, not the first).
 */
export function apiErrorMessage(status: number, serverMessage = "", messages: ApiErrorMessages = {}) {
  return messages[status] ?? messages.default ?? defaultApiErrorMessages[status] ?? (serverMessage || "เกิดข้อผิดพลาด");
}

export class ApiError extends Error {
  readonly status: number;
  /** `message`/`error` from a JSON error body, else the plain-text body, trimmed. */
  readonly serverMessage: string;

  constructor(status: number, serverMessage: string, messages?: ApiErrorMessages) {
    super(apiErrorMessage(status, serverMessage, messages));
    this.name = "ApiError";
    this.status = status;
    this.serverMessage = serverMessage;
  }
}

function serverErrorText(body: string) {
  try {
    const parsed = JSON.parse(body) as { message?: unknown; error?: unknown };
    const text = parsed.message ?? parsed.error;
    if (typeof text === "string") return text;
  } catch {}
  return body.trim();
}

/**
 * JSON request through `api`: sends the CSRF token on anything but GET,
 * JSON-encodes `body`, and throws an `ApiError` (whose `message` is already the
 * user-facing Thai text, so `errorMessage(cause)` just works) on a non-2xx.
 * Resolves to `undefined` for 204 / empty bodies.
 */
export async function apiJson<T = unknown>(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal; headers?: Record<string, string>; messages?: ApiErrorMessages } = {},
): Promise<T> {
  const method = options.method ?? "GET";
  const response = await api(path, {
    method,
    signal: options.signal,
    headers: { ...(method === "GET" ? {} : { "X-CSRF-Token": csrfToken() }), ...options.headers },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  const text = await response.text();
  if (!response.ok) throw new ApiError(response.status, serverErrorText(text), options.messages);
  return (text ? JSON.parse(text) : undefined) as T;
}

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

/** The `YYYY-MM-DDTHH:mm` shape `<input type="datetime-local">` requires, in browser-local time. */
export function toDatetimeLocal(date: Date) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function formatDate(value: string) {
  return new Intl.DateTimeFormat("th-TH", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

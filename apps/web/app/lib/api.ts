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

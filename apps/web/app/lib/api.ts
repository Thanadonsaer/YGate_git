export const gatewayURL = process.env.NEXT_PUBLIC_GATEWAY_URL ?? "http://localhost:44440";

export function csrfToken() {
  const prefix = "platform_csrf=";
  const cookie = document.cookie.split("; ").find((item) => item.startsWith(prefix));
  return cookie ? decodeURIComponent(cookie.slice(prefix.length)) : "";
}

export async function api(path: string, init?: RequestInit) {
  try {
    return await fetch(`${gatewayURL}${path}`, {
      ...init,
      credentials: "include",
      headers: {
        ...(init?.body && !(init.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
    });
  } catch {
    // Gateway unreachable (down, network drop, CORS-origin rejected). Return a
    // non-ok Response instead of letting fetch's rejection propagate, so every
    // caller's existing `if (response.ok) ... else setError(...)` handles it
    // the same way it handles a 4xx/5xx from the server.
    return new Response(null, { status: 503, statusText: "Network error" });
  }
}

export function formatDate(value: string) {
  return new Intl.DateTimeFormat("th-TH", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

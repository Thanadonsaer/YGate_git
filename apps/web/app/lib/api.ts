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

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

export function formatDate(value: string) {
  return new Intl.DateTimeFormat("th-TH", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

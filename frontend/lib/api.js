export const API_BASE = (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080").replace(
  /\/$/,
  "",
);
export class APIError extends Error {
  constructor(message, status, requestId) {
    super(message);
    this.status = status;
    this.requestId = requestId;
  }
}
export async function request(
  path,
  { token, csrfToken, method = "GET", body, key, signal, raw = false, download = false } = {},
) {
  const headers = { Accept: "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (csrfToken && !["GET", "HEAD"].includes(method)) headers["X-CSRF-Token"] = csrfToken;
  if (body !== undefined)
    headers["Content-Type"] = raw ? "application/octet-stream" : "application/json";
  if (key) headers["Idempotency-Key"] = key;
  let response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
      signal,
      cache: "no-store",
      credentials: token ? "omit" : "include",
    });
  } catch (error) {
    if (error.name === "AbortError") throw error;
    throw new APIError("Cannot reach your hosting service. Please try again.", 0);
  }
  if (response.ok && download) return response.blob();
  const data = response.status === 204 ? null : await response.json().catch(() => null);
  if (!response.ok)
    throw new APIError(
      data?.error || "Something went wrong. Please try again.",
      response.status,
      response.headers.get("X-Request-ID"),
    );
  return data;
}
export function consoleURL(id) {
  const url = new URL(`${API_BASE}/servers/${encodeURIComponent(id)}/console`);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}
export function formatBytes(bytes, digits = 1) {
  if (bytes == null) return "—";
  return bytes >= 1073741824
    ? `${(bytes / 1073741824).toFixed(digits)} GB`
    : `${(bytes / 1048576).toFixed(digits)} MB`;
}
export function duration(ms) {
  if (ms == null) return "—";
  const mins = Math.floor(ms / 60000);
  return mins >= 60 ? `${Math.floor(mins / 60)}h ${mins % 60}m` : `${mins}m`;
}
export function price(plan) {
  return new Intl.NumberFormat("en", { style: "currency", currency: plan.currency }).format(
    plan.price_cents / 100,
  );
}
export const STATE_LABELS = {
  PENDING_PAYMENT: "Awaiting activation",
  PAYMENT_CONFIRMED: "Queued",
  PROVISIONING: "Installing",
  RUNNING: "Ready",
  SUSPENDED: "Suspended",
  FAILED: "Needs attention",
  CANCELLED: "Cancelled",
  running: "Online",
  offline: "Ready to start",
  ACCEPTANCE_REQUIRED: "Review agreement",
  starting: "Starting",
  stopping: "Stopping",
};

export const API_BASE = (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080").replace(
  /\/$/,
  "",
);
export class APIError extends Error {
  constructor(message, status, requestId, code) {
    super(message);
    this.status = status;
    this.requestId = requestId;
    this.code = typeof code === "string" ? code : undefined;
  }
}
export async function request(
  path,
  {
    token,
    csrfToken,
    method = "GET",
    body,
    key,
    signal,
    raw = false,
    download = false,
    baseURL = API_BASE,
    credentials = token ? "omit" : "include",
    timeoutMs = raw || download ? 120000 : 30000,
  } = {},
) {
  const headers = { Accept: "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (csrfToken && !["GET", "HEAD"].includes(method)) headers["X-CSRF-Token"] = csrfToken;
  if (body !== undefined)
    headers["Content-Type"] = raw ? "application/octet-stream" : "application/json";
  if (key) headers["Idempotency-Key"] = key;
  if (signal?.aborted) throw new DOMException("Request cancelled", "AbortError");
  if (typeof navigator !== "undefined" && navigator.onLine === false)
    throw new APIError("You’re offline. Reconnect and try again.", 0, undefined, "OFFLINE");
  // Bound the full response, including body reads. Caller cancellation remains distinct.
  const controller = new AbortController();
  let timedOut = false;
  const cancel = () => controller.abort(signal.reason);
  if (signal?.aborted) cancel();
  else signal?.addEventListener("abort", cancel, { once: true });
  const timer = setTimeout(
    () => {
      timedOut = true;
      controller.abort();
    },
    Number.isFinite(timeoutMs) && timeoutMs > 0 ? timeoutMs : 30000,
  );
  let response;
  try {
    response = await fetch(`${baseURL}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
      signal: controller.signal,
      cache: "no-store",
      credentials,
    });
    if (response.ok && download) return await response.blob();
    let data = null;
    if (response.status !== 204 && method !== "HEAD") {
      try {
        data = await response.json();
      } catch (error) {
        if (controller.signal.aborted) throw error;
        if (response.ok)
          throw new APIError(
            ["GET", "HEAD"].includes(method)
              ? "Unexpected response. Please try again."
              : "Unexpected response. Check the result before retrying.",
            502,
            response.headers.get("X-Request-ID"),
            "INVALID_RESPONSE",
          );
      }
    }
    if (!response.ok) {
      const error = new APIError(
        data?.error || "Something went wrong. Please try again.",
        response.status,
        response.headers.get("X-Request-ID") || data?.request_id,
        data?.code,
      );
      const seconds = Number(response.headers.get("Retry-After"));
      if (response.status === 429)
        error.retryAfterMs =
          Number.isFinite(seconds) && seconds > 0 ? Math.min(seconds, 900) * 1000 : 60000;
      throw error;
    }
    return data;
  } catch (error) {
    if (signal?.aborted) throw new DOMException("Request cancelled", "AbortError");
    if (timedOut)
      throw new APIError(
        ["GET", "HEAD"].includes(method)
          ? "Connection timed out. Please try again."
          : "Request timed out. Check the result before retrying.",
        0,
        response?.headers.get("X-Request-ID"),
        "REQUEST_TIMEOUT",
      );
    if (error instanceof APIError) throw error;
    throw new APIError("Cannot reach your hosting service. Please try again.", 0);
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", cancel);
  }
}
export function consoleURL(id) {
  const url = new URL(`${API_BASE}/servers/${encodeURIComponent(id)}/console`);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}
export function formatBytes(bytes, digits = 1) {
  if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(digits)} KB`;
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
  CHECKING: "Checking…",
  UNKNOWN: "Status unavailable",
  PENDING_PAYMENT: "Awaiting activation",
  PAYMENT_CONFIRMED: "Queued",
  PROVISIONING: "Installing",
  RUNNING: "Ready",
  SUSPENDED: "Suspended",
  FAILED: "Needs attention",
  CANCELLED: "Cancelled",
  DELETING: "Deleting",
  running: "Online",
  offline: "Ready to start",
  ACCEPTANCE_REQUIRED: "Review agreement",
  starting: "Starting",
  stopping: "Stopping",
};

export function backupPreflightRejected(error) {
  return ["BACKUP_DECISION_REQUIRED", "BACKUP_CHECK_FAILED"].includes(error?.code);
}

"use client";
import { useCallback, useEffect, useState } from "react";
import { useAuth } from "./auth";

export function useResource(path, interval = 0) {
  const { session, api, serverRevision } = useAuth();
  const token = session?.csrf_token;
  const [result, setResult] = useState(null);
  const [reload, setReload] = useState(0);
  // A response belongs to one session, path and explicit refresh generation.
  // Derive loading from this key so navigation never renders a previous result.
  const resourceKey = JSON.stringify([
    token || null,
    path,
    reload,
    path === "/servers" || path?.startsWith("/servers/") ? serverRevision : 0,
  ]);
  const enabled = !!token && !!path;
  const refresh = useCallback(() => setReload((value) => value + 1), []);

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let timer;
    let alive = true;
    let inFlight = false;
    let limitedUntil = 0;
    let failed = false;
    let reconnectPending = false;
    const visibleAndOnline = () => document.visibilityState !== "hidden" && navigator.onLine;
    function schedule(delay, reconnect = false) {
      clearTimeout(timer);
      if (alive && (interval || reconnect) && visibleAndOnline()) timer = setTimeout(load, delay);
    }
    function resume() {
      if (!alive || !visibleAndOnline()) return;
      if (inFlight) {
        reconnectPending = true;
        return;
      }
      if (!interval && !failed) return;
      schedule(Math.max(0, limitedUntil - Date.now()), true);
    }
    async function load() {
      if (!alive || inFlight) return;
      inFlight = true;
      let retryDelay = interval;
      try {
        const data = await api(path, { signal: controller.signal });
        failed = false;
        if (alive) setResult({ key: resourceKey, data, error: null });
      } catch (error) {
        failed = error.name !== "AbortError";
        if (error.status === 429) {
          retryDelay = Math.max(interval, error.retryAfterMs || 60000);
          limitedUntil = Date.now() + retryDelay;
        }
        if (alive && error.name !== "AbortError") {
          setResult((previous) => ({
            key: resourceKey,
            data: previous?.key === resourceKey ? previous.data : null,
            error,
          }));
        }
      } finally {
        inFlight = false;
        const reconnect = reconnectPending && failed;
        reconnectPending = false;
        schedule(reconnect ? Math.max(0, limitedUntil - Date.now()) : retryDelay, reconnect);
      }
    }
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("online", resume);
    // A timer already queued before the browser goes offline must not send a request.
    function suspend() {
      clearTimeout(timer);
    }
    window.addEventListener("offline", suspend);
    const pauseHidden = () => {
      if (!visibleAndOnline()) suspend();
    };
    document.addEventListener("visibilitychange", pauseHidden);
    load();
    return () => {
      alive = false;
      controller.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", resume);
      document.removeEventListener("visibilitychange", pauseHidden);
      window.removeEventListener("online", resume);
      window.removeEventListener("offline", suspend);
    };
  }, [enabled, api, path, interval, resourceKey]);

  const matches = enabled && result?.key === resourceKey;
  return {
    data: matches ? result.data : null,
    error: matches ? result.error : null,
    loading: enabled && !matches,
    refresh,
  };
}

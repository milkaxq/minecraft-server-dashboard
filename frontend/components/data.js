"use client";
import { useCallback, useEffect, useState } from "react";
import { useAuth } from "./auth";

export function useResource(path, interval = 0) {
  const { session, api } = useAuth();
  const token = session?.token;
  const [result, setResult] = useState(null);
  const [reload, setReload] = useState(0);
  // A response belongs to one session, path and explicit refresh generation.
  // Derive loading from this key so navigation never renders a previous result.
  const resourceKey = JSON.stringify([token || null, path, reload]);
  const enabled = !!token && !!path;
  const refresh = useCallback(() => setReload((value) => value + 1), []);

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let timer;
    let alive = true;
    async function load() {
      try {
        const data = await api(path, { signal: controller.signal });
        if (alive) setResult({ key: resourceKey, data, error: null });
      } catch (error) {
        if (alive && error.name !== "AbortError") {
          setResult((previous) => ({
            key: resourceKey,
            data: previous?.key === resourceKey ? previous.data : null,
            error,
          }));
        }
      } finally {
        if (alive && interval) timer = setTimeout(load, interval);
      }
    }
    load();
    return () => {
      alive = false;
      controller.abort();
      clearTimeout(timer);
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

"use client";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { request } from "../lib/api";
const AuthContext = createContext(null);
export function AuthProvider({ children, marketing = false }) {
  const [session, setSession] = useState(null);
  const [serverRevision, setServerRevision] = useState(0);
  const refreshServers = useCallback(() => setServerRevision((value) => value + 1), []);
  const [restoring, setRestoring] = useState(true);
  const [restoreError, setRestoreError] = useState(null);
  const [authNotice, setAuthNotice] = useState("");
  const generation = useRef(0);
  const router = useRouter();
  const csrfToken = session?.csrf_token;
  const loadSession = useCallback(async (signal) => {
    const attempt = ++generation.current;
    try {
      const next = await request("/auth/session", {
        signal,
      });
      if (!signal?.aborted && attempt === generation.current) {
        setRestoreError(null);
        setSession(next);
      }
    } catch (error) {
      if (attempt !== generation.current || error.name === "AbortError") return;
      if (error.status === 401) setSession(null);
      else setRestoreError(error);
    } finally {
      if (!signal?.aborted && attempt === generation.current) setRestoring(false);
    }
  }, []);
  const restoreSession = useCallback(() => {
    setRestoring(true);
    setRestoreError(null);
    return loadSession();
  }, [loadSession]);
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      if (marketing) {
        setRestoring(false);
        return;
      }
      return loadSession(controller.signal);
    }, 0);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [loadSession, marketing]);
  const api = useCallback(
    async (path, options = {}) => {
      const attempt = generation.current;
      try {
        return await request(path, { ...options, csrfToken });
      } catch (error) {
        if (error.status === 401 && attempt === generation.current) {
          generation.current++;
          setSession(null);
          setRestoreError(null);
          router.replace("/login");
        }
        throw error;
      }
    },
    [csrfToken, router],
  );
  useEffect(() => {
    if (!session) return;
    const remaining = new Date(session.expires_at).getTime() - Date.now();
    const timer = setTimeout(
      () => {
        generation.current++;
        setSession(null);
        router.replace("/login");
      },
      Math.max(0, remaining),
    );
    return () => clearTimeout(timer);
  }, [session, router]);
  async function login(email, password) {
    generation.current++;
    setRestoring(false);
    setAuthNotice("");
    const next = await request("/auth/session-login", {
      method: "POST",
      body: { email, password },
    });
    generation.current++;
    setRestoreError(null);
    setSession(next);
    router.replace("/dashboard");
  }
  async function logout() {
    try {
      if (session) await api("/auth/logout", { method: "POST" });
      generation.current++;
      setSession(null);
      setRestoreError(null);
      router.replace("/login");
    } catch {
      setAuthNotice("Could not sign out. Check your connection and try again.");
    }
  }
  function forgetSession() {
    generation.current++;
    setSession(null);
    setRestoreError(null);
    setRestoring(false);
  }
  function passwordChanged() {
    forgetSession();
    setAuthNotice(
      "Password changed. All sessions were signed out. Sign in with your new password.",
    );
    router.replace("/login");
  }
  return (
    <AuthContext.Provider
      value={{
        session,
        serverRevision,
        refreshServers,
        api,
        login,
        logout,
        passwordChanged,
        forgetSession,
        authNotice,
        restoring,
        restoreError,
        restoreSession,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}
export function useAuth() {
  return useContext(AuthContext);
}

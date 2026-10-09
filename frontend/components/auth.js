"use client";
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { request } from "../lib/api";
const AuthContext = createContext(null);
export function AuthProvider({ children }) {
  const [session, setSession] = useState(null);
  const [authNotice, setAuthNotice] = useState("");
  const token = session?.token;
  const router = useRouter();
  // Sessions deliberately stay in memory. Reloading requires signing in again.
  const api = useCallback(
    async (path, options = {}) => {
      try {
        return await request(path, { ...options, token });
      } catch (error) {
        if (error.status === 401) {
          setSession(null);
          router.replace("/login");
        }
        throw error;
      }
    },
    [token, router],
  );
  useEffect(() => {
    if (!session) return;
    const remaining = new Date(session.expires_at).getTime() - Date.now();
    const timer = setTimeout(
      () => {
        setSession(null);
        router.replace("/login");
      },
      Math.max(0, remaining),
    );
    return () => clearTimeout(timer);
  }, [session, router]);
  async function login(email, password) {
    setAuthNotice("");
    const next = await request("/auth/login", { method: "POST", body: { email, password } });
    setSession(next);
    router.replace("/dashboard");
  }
  async function logout() {
    try {
      if (session) await api("/auth/logout", { method: "POST" });
    } finally {
      setSession(null);
      router.replace("/login");
    }
  }
  function passwordChanged() {
    setSession(null);
    setAuthNotice(
      "Password changed. All sessions were signed out. Sign in with your new password.",
    );
    router.replace("/login");
  }
  return (
    <AuthContext.Provider value={{ session, api, login, logout, passwordChanged, authNotice }}>
      {children}
    </AuthContext.Provider>
  );
}
export function useAuth() {
  return useContext(AuthContext);
}

"use client";
import { useSyncExternalStore } from "react";
function subscribe(notify) {
  window.addEventListener("online", notify);
  window.addEventListener("offline", notify);
  return () => {
    window.removeEventListener("online", notify);
    window.removeEventListener("offline", notify);
  };
}
const online = () => navigator.onLine;
const serverOnline = () => true;
export function ConnectionStatus() {
  const connected = useSyncExternalStore(subscribe, online, serverOnline);
  if (connected) return null;
  return (
    <p className="connection-status" role="status">
      You’re offline. Reconnect to continue.
    </p>
  );
}

"use client";
import { useState, useSyncExternalStore } from "react";

const eventName = "blockyard-player-setup-ready";
function subscribe(listener) {
  window.addEventListener("storage", listener);
  window.addEventListener(eventName, listener);
  return () => {
    window.removeEventListener("storage", listener);
    window.removeEventListener(eventName, listener);
  };
}
export function useReturningPlayer(setup) {
  // Store a browser preference for this public game address, never a login credential.
  const key = setup?.address ? `blockyard-player-setup:${setup.address}` : null;
  const revision = setup
    ? JSON.stringify([
        setup.published_at,
        setup.minecraft_version,
        setup.loader,
        setup.loader_version,
        setup.addons,
        setup.pack_ready,
        setup.pack_reason,
        setup.verified_login,
        setup.whitelist,
      ])
    : null;
  const saved = useSyncExternalStore(
    subscribe,
    () => {
      try {
        return key ? localStorage.getItem(key) : null;
      } catch {
        return null;
      }
    },
    () => null,
  );
  const [temporary, setTemporary] = useState(null);
  const remembered = temporary?.key === key ? temporary.revision : saved;
  function ready() {
    if (
      !key ||
      !revision ||
      setup.pack_reason ||
      (setup.addons?.some((addon) => addon.required) && !setup.pack_ready)
    )
      return;
    try {
      localStorage.setItem(key, revision);
      setTemporary(null);
      window.dispatchEvent(new Event(eventName));
    } catch {
      setTemporary({ key, revision });
    }
  }
  return {
    revision,
    returning: !!remembered && remembered === revision && !setup?.pack_reason,
    changed: !!remembered && remembered !== revision,
    ready,
  };
}

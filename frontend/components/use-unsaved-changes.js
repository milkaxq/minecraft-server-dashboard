"use client";
import { useEffect } from "react";

// Browsers supply their own warning text for refresh, tab close and full navigation.
export function useUnsavedChanges(dirty) {
  useEffect(() => {
    if (!dirty) return;
    const warn = (event) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
}

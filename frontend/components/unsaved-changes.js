"use client";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useRouter } from "next/navigation";

export function UnsavedChangesGuard({ dirty }) {
  return dirty ? <ActiveGuard /> : null;
}

function ActiveGuard() {
  const router = useRouter();
  const [destination, setDestination] = useState(null);
  const approvedUnload = useRef(false);
  useEffect(() => {
    const warn = (event) => {
      if (approvedUnload.current) return;
      event.preventDefault();
      event.returnValue = "";
    };
    const follow = (event) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const anchor = event.target instanceof Element ? event.target.closest("a[href]") : null;
      if (
        !anchor ||
        anchor.hasAttribute("download") ||
        (anchor.target && anchor.target !== "_self")
      )
        return;
      let url;
      try {
        url = new URL(anchor.href, window.location.href);
      } catch {
        return;
      }
      if (!["http:", "https:"].includes(url.protocol)) return;
      if (
        url.origin === window.location.origin &&
        url.pathname === window.location.pathname &&
        url.search === window.location.search
      )
        return;
      event.preventDefault();
      event.stopPropagation();
      setDestination(url.href);
    };
    window.addEventListener("beforeunload", warn);
    document.addEventListener("click", follow, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      document.removeEventListener("click", follow, true);
    };
  }, []);
  function leave() {
    const url = new URL(destination);
    setDestination(null);
    if (url.origin === window.location.origin) router.push(url.pathname + url.search + url.hash);
    else {
      approvedUnload.current = true;
      window.location.assign(url.href);
    }
  }
  return destination
    ? createPortal(
        <LeaveDialog onStay={() => setDestination(null)} onLeave={leave} />,
        document.querySelector(".app-shell") || document.body,
      )
    : null;
}

function LeaveDialog({ onStay, onLeave }) {
  const ref = useRef(null);
  useEffect(() => {
    const dialog = ref.current;
    dialog.showModal();
    return () => dialog.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className="confirm-dialog"
      aria-label="Leave without saving?"
      onCancel={(event) => {
        event.preventDefault();
        onStay();
      }}
    >
      <h2>Leave without saving?</h2>
      <div className="confirm-content">
        <p>
          You have unsaved edits on this page. Stay to save or copy them, or leave and discard them.
        </p>
      </div>
      <div className="dialog-actions">
        <button type="button" className="button secondary" onClick={onStay} autoFocus>
          Stay and keep editing
        </button>
        <button type="button" className="button danger" onClick={onLeave}>
          Leave without saving
        </button>
      </div>
    </dialog>
  );
}

"use client";
import { useEffect, useRef } from "react";
import { Icon } from "./ui";

export function Confirm({
  title,
  children,
  label = "Confirm",
  busy,
  confirmDisabled = false,
  confirmVariant = "danger",
  onCancel,
  onConfirm,
}) {
  const ref = useRef(null);
  useEffect(() => {
    const el = ref.current;
    el.showModal();
    return () => el.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className={`confirm-dialog ${confirmVariant === "primary" ? "install-dialog" : ""}`}
      aria-label={title}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <div className="confirm-icon">
        <Icon name="shield" size={25} />
      </div>
      <h2>{title}</h2>
      <div className="confirm-content">{children}</div>
      <div className="dialog-actions">
        <button className="button secondary" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <button
          className={`button ${confirmVariant}`}
          disabled={busy || confirmDisabled}
          onClick={onConfirm}
        >
          {busy ? "Working…" : label}
        </button>
      </div>
    </dialog>
  );
}

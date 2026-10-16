"use client";
import { useEffect, useRef, useState } from "react";
import { Icon } from "./ui";

export function Confirm({
  title,
  children,
  label = "Confirm",
  cancelLabel = "Cancel",
  busy,
  confirmDisabled = false,
  confirmVariant = "danger",
  onCancel,
  onConfirm,
}) {
  const ref = useRef(null);
  const cancelButton = useRef(null);
  const inFlight = useRef(false);
  const [working, setWorking] = useState(false);
  const locked = !!busy || working;
  function cancel() {
    if (!busy && !inFlight.current) onCancel();
  }
  async function confirm() {
    if (busy || confirmDisabled || inFlight.current || !onConfirm) return;
    inFlight.current = true;
    setWorking(true);
    try {
      await onConfirm();
    } finally {
      inFlight.current = false;
      setWorking(false);
    }
  }
  useEffect(() => {
    const el = ref.current;
    el.showModal();
    if (!el.querySelector("input, textarea, select, [autofocus]")) cancelButton.current?.focus();
    return () => el.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className={`confirm-dialog ${confirmVariant === "primary" ? "install-dialog" : ""}`}
      aria-label={title}
      onCancel={(e) => {
        e.preventDefault();
        cancel();
      }}
    >
      <div className="confirm-icon">
        <Icon name="shield" size={25} />
      </div>
      <h2>{title}</h2>
      <div className="confirm-content">{children}</div>
      <div className="dialog-actions">
        <button
          ref={cancelButton}
          type="button"
          className="button secondary"
          disabled={locked}
          onClick={cancel}
        >
          {cancelLabel}
        </button>
        {onConfirm && (
          <button
            className={`button ${confirmVariant}`}
            type="button"
            disabled={locked || confirmDisabled}
            onClick={confirm}
          >
            {locked ? "Working…" : label}
          </button>
        )}
      </div>
    </dialog>
  );
}

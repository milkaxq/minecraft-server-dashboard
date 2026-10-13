"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useAuth } from "./auth";
import { ErrorMessage } from "./ui";

export function PluginCheck({ id, mods, usage, files }) {
  const { api } = useAuth();
  const [check, setCheck] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const expiry = useRef(null);
  const request = useRef(null);
  useEffect(
    () => () => {
      clearTimeout(expiry.current);
      request.current?.abort();
    },
    [],
  );
  const running = usage?.power_state === "running";
  const current = check && running && usage.uptime_ms >= check.uptime_ms && check.files === files;
  async function inspect() {
    setBusy(true);
    setError(null);
    setCheck(null);
    const controller = new AbortController();
    request.current = controller;
    try {
      const result = await api(`/servers/${id}/plugins/check`, {
        signal: controller.signal,
        method: "POST",
        key: crypto.randomUUID(),
        body: {},
      });
      if (controller.signal.aborted) return;
      setCheck({ ...result, files });
      clearTimeout(expiry.current);
      expiry.current = setTimeout(() => setCheck(null), 60000);
    } catch (err) {
      if (err.name !== "AbortError") setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel addon-runtime-check" aria-labelledby="addon-runtime-title">
      <div className="section-heading">
        <div>
          <h2 id="addon-runtime-title">Is it running?</h2>
          <p>
            File installed means the .jar is on your server. It does not confirm the addon loaded
            successfully.
          </p>
        </div>
        {!mods && (
          <button className="button secondary" onClick={inspect} disabled={!running || busy}>
            {busy ? "Checking plugins…" : "Check loaded plugins"}
          </button>
        )}
      </div>
      <ErrorMessage error={error} />
      {mods ? (
        <p>
          Automatic per-mod load checks are not available for this loader. Start the server, then
          inspect Console for missing dependencies, incompatible versions, or startup errors. File
          presence alone is not a load check.
        </p>
      ) : !running ? (
        <p>Start the server and wait for Online, then check which plugins it reports as enabled.</p>
      ) : !current ? (
        <p>Runtime status has not been checked for this server session and installed file list.</p>
      ) : (
        <>
          <p className="field-help">
            Checked {new Date(check.checked_at).toLocaleTimeString()}. This is a snapshot of the
            server’s plugin response; check again after changes.
          </p>
          {!check.confirmed ? (
            <p className="notice">
              Load status could not be confirmed.{" "}
              {check.reason || "Open Console and check the startup messages."}
            </p>
          ) : !check.plugins.length ? (
            <p role="status">The running server reports no loaded plugins.</p>
          ) : (
            <ul className="runtime-plugin-list">
              {check.plugins.map((plugin) => (
                <li key={plugin.name}>
                  <strong>{plugin.name}</strong>
                  <span
                    className={`client-requirement-tag ${plugin.enabled ? "" : "client-requirement-required"}`}
                  >
                    {plugin.enabled ? "Loaded and enabled" : "Disabled or failed to load"}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
      <Link className="text-link" href={`/servers/${id}/console`}>
        Open Console →
      </Link>
    </section>
  );
}

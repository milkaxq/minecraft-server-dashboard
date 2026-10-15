"use client";
import Link from "next/link";
import { useState } from "react";
import { Select } from "./select";
import { diagnosticReport, LAG_SYMPTOMS } from "../lib/lag-diagnostics";

export function LagHelp({ server, ready, powerState, usage, usageError }) {
  const [symptom, setSymptom] = useState("blocks");
  const [report, setReport] = useState("");
  const [message, setMessage] = useState("");
  async function copyReport() {
    const snapshot = diagnosticReport({
      server,
      ready,
      powerState,
      usage,
      usageError,
      symptom,
      capturedAt: new Date().toISOString(),
    });
    setReport(snapshot);
    try {
      await navigator.clipboard.writeText(snapshot);
      setMessage("Report copied.");
    } catch {
      setMessage("Copy is unavailable. Select and copy the report below manually.");
    }
  }
  return (
    <div className="lag-help">
      <label className="field">
        What do players notice?
        <Select value={symptom} onChange={(event) => setSymptom(event.target.value)}>
          {Object.entries(LAG_SYMPTOMS).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
      </label>
      <p aria-live="polite">
        {symptom === "blocks" &&
          "Everyone lagging? Check CPU and lower view distance. Just one player? Check their connection."}
        {symptom === "chunks" &&
          "Try a lower view distance. Pause terrain generation while friends are playing."}
        {symptom === "fps" &&
          "Lower client render distance or turn off shaders. Keep required mods installed."}
      </p>
      {server.permissions?.includes("settings") && symptom !== "fps" && (
        <Link className="text-link" href={`/servers/${server.id}/settings`}>
          Open Minecraft settings →
        </Link>
      )}
      <p className="field-help">Tick rate and player ping aren’t measured here.</p>
      <button className="button secondary" type="button" onClick={copyReport}>
        Copy diagnostic report
      </button>
      {message && <p role="status">{message}</p>}
      {report && (
        <label className="field diagnostic-report">
          Diagnostic report preview
          <textarea readOnly rows={12} value={report} spellCheck={false} />
          <span className="field-help">Last dashboard reading. Nothing is sent automatically.</span>
        </label>
      )}
    </div>
  );
}

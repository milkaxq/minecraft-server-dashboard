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
      setMessage("Diagnostic report copied. Review it below before sharing.");
    } catch {
      setMessage("Copy is unavailable. Select and copy the report below manually.");
    }
  }
  return (
    <details className="lag-help">
      <summary>Players are experiencing lag?</summary>
      <p>
        Delayed block breaks and rubber-banding can come from server workload or a slow connection.
        Low FPS is usually a client-side issue.
      </p>
      <p>
        CPU and memory readings help spot resource pressure, but this dashboard does not measure
        Minecraft tick rate or each player’s ping. Compare whether everyone has delays or only one
        player before changing settings.
      </p>
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
      <div className="lag-guidance" aria-live="polite">
        {symptom === "blocks" && (
          <>
            <h3>Compare players before changing the server</h3>
            <ol>
              <li>
                Ask whether everyone gets the delay at the same time. If only one player does,
                compare their connection and distance from the server.
              </li>
              <li>
                If everyone is affected, watch CPU usage during the delay. A brief spike is not
                proof of a problem.
              </li>
              <li>
                If terrain generation is running, pause it from its guided setup. Try a lower view
                distance and change one setting at a time.
              </li>
            </ol>
          </>
        )}
        {symptom === "chunks" && (
          <>
            <h3>Check whether exploring new terrain triggers it</h3>
            <ol>
              <li>
                Compare an already explored area with new terrain. Generating new chunks adds server
                work.
              </li>
              <li>
                Try a lower view distance. If using Chunky, generate terrain while nobody is playing
                and pause it before inviting friends.
              </li>
              <li>
                If only one player sees slow chunks, compare their connection too. CPU usage alone
                cannot identify the cause.
              </li>
            </ol>
          </>
        )}
        {symptom === "fps" && (
          <>
            <h3>Start with the player’s graphics settings</h3>
            <ol>
              <li>
                Lower the client’s render distance and graphics quality, then test without shaders.
              </li>
              <li>
                Compare another area or an unmodified client profile where the server allows it.
                Keep required client mods installed.
              </li>
              <li>
                More server memory does not directly increase a player’s FPS. Check server workload
                separately if block actions are delayed too.
              </li>
            </ol>
          </>
        )}
        {server.permissions?.includes("settings") && symptom !== "fps" && (
          <Link className="text-link" href={`/servers/${server.id}/settings`}>
            Open Minecraft settings →
          </Link>
        )}
      </div>
      <p className="field-help">
        High memory use alone does not prove lag: Minecraft can reserve most of its allowance.
      </p>
      <button className="button secondary" type="button" onClick={copyReport}>
        Copy diagnostic report
      </button>
      {message && <p role="status">{message}</p>}
      {report && (
        <label className="field diagnostic-report">
          Diagnostic report preview
          <textarea readOnly rows={12} value={report} spellCheck={false} />
          <span className="field-help">
            A snapshot of the last dashboard reading when you pressed Copy. Capture time is not
            measurement time. Copy again to refresh the report. Nothing is sent automatically.
          </span>
        </label>
      )}
    </details>
  );
}

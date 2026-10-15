"use client";
import Link from "next/link";
import { LagHelp } from "./lag-help";
import { useResource } from "./data";
import { serverHealth } from "../lib/server-health";
import { ErrorMessage } from "./ui";
export function StartupCheck({ server, ready, powerState, usage, usageError, eula }) {
  const active = ready;
  const resource = useResource(
    active ? `/servers/${server.id}/startup-check` : null,
    powerState === "starting" ? 15000 : 60000,
  );
  const issues = resource.error ? [] : resource.data?.issues || [];
  const health = serverHealth({ server, ready, powerState, usage, usageError, eula });
  const reportParams = new URLSearchParams({
    server: server.id,
    page: `/servers/${server.id}`,
    reference:
      usageError?.requestId || resource.error?.requestId || resource.data?.request_reference || "",
    observed_state: usageError ? "unavailable" : powerState || server.status,
  });
  if (resource.data?.checked_at) reportParams.set("checked_at", resource.data.checked_at);
  return (
    <section className="panel server-health" aria-label="Server health">
      <h2>{health.title}</h2>
      {health.title !== "Minecraft is online" && <p>{health.help}</p>}
      {health.action === "storage" && server.permissions?.includes("files") && (
        <Link className="button secondary" href={`/servers/${server.id}/files`}>
          Review server files
        </Link>
      )}
      <LagHelp
        server={server}
        ready={ready}
        powerState={powerState}
        usage={usage}
        usageError={usageError}
      />
      <ErrorMessage error={resource.error} />
      {issues.length > 0 && (
        <ul role="status" aria-label="Startup alerts">
          {issues.map((issue) => (
            <li key={issue.code}>
              <strong>{issue.title}</strong>
              <p>{issue.help}</p>
            </li>
          ))}
        </ul>
      )}
      <div className="file-actions">
        <button
          className="button secondary"
          disabled={!ready || resource.loading}
          onClick={() => {
            resource.refresh();
          }}
        >
          Check startup
        </button>
        <Link className="text-link" href={`/servers/${server.id}/console`}>
          Open Console →
        </Link>
        <Link className="button secondary" href={`/feedback?${reportParams.toString()}`}>
          Report a problem →
        </Link>
      </div>
    </section>
  );
}

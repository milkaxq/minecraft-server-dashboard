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
      <span className="eyebrow">HOW IS MY SERVER DOING?</span>
      <h2>{health.title}</h2>
      <p>{health.help}</p>
      {issues.length > 0 && (
        <p className="notice">
          The latest startup check found {issues.length} possible{" "}
          {issues.length === 1 ? "issue" : "issues"}. Review Startup help below; a log finding does
          not by itself prove the game is offline.
        </p>
      )}
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
      <details className="startup-check" open={issues.length ? true : undefined}>
        <summary>
          Startup help
          {issues.length
            ? ` · ${issues.length} possible ${issues.length === 1 ? "issue" : "issues"}`
            : ""}
        </summary>
        <ErrorMessage error={resource.error} />
        {resource.loading ? (
          <p>Checking recent startup messages…</p>
        ) : resource.data ? (
          <>
            {!resource.data.checked ? (
              <p>
                {resource.data.reason ||
                  "Startup could not be checked. Open Console for live output."}
              </p>
            ) : issues.length ? (
              <ul role="status" aria-label="Startup alerts">
                {issues.map((issue) => (
                  <li key={issue.code}>
                    <strong>{issue.title}</strong>
                    <p>{issue.help}</p>
                  </li>
                ))}
              </ul>
            ) : (
              <p>
                No recognized startup problem was found in the latest log. This does not confirm
                every addon or player connection is working.
              </p>
            )}
            <p className="field-help">
              Checked {new Date(resource.data.checked_at).toLocaleTimeString()}. Guided checks show
              safe explanations; log contents are not included.
            </p>
          </>
        ) : (
          <p>
            Check recent Minecraft messages for missing dependencies, incompatible versions, and
            other startup problems.
          </p>
        )}
      </details>
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

"use client";
import Link from "next/link";
import { useState } from "react";
import { useResource } from "./data";
import { ErrorMessage } from "./ui";
export function StartupCheck({ server, ready, powerState }) {
  const [requested, setRequested] = useState(false);
  const active = ready && (requested || powerState === "starting" || powerState === "offline");
  const resource = useResource(active ? `/servers/${server.id}/startup-check` : null, 15000);
  const issues = resource.error ? [] : resource.data?.issues || [];
  return (
    <details className="startup-check panel" open={issues.length ? true : undefined}>
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
            <ul>
              {issues.map((issue) => (
                <li key={issue.code}>
                  <strong>{issue.title}</strong>
                  <p>{issue.help}</p>
                </li>
              ))}
            </ul>
          ) : (
            <p>
              No recognized startup problem was found in the latest log. This does not confirm every
              addon or player connection is working.
            </p>
          )}
          <p className="field-help">
            Checked {new Date(resource.data.checked_at).toLocaleTimeString()}. Guided checks show
            safe explanations; log contents are not included.
          </p>
        </>
      ) : (
        <p>
          Check recent Minecraft messages for missing dependencies, incompatible versions, and other
          startup problems.
        </p>
      )}
      <div className="file-actions">
        <button
          className="button secondary"
          disabled={!ready || resource.loading}
          onClick={() => {
            setRequested(true);
            resource.refresh();
          }}
        >
          Check startup
        </button>
        <Link className="text-link" href={`/servers/${server.id}/console`}>
          Open Console →
        </Link>
        <Link
          className="text-link"
          href={`/feedback?server=${server.id}&page=${encodeURIComponent(`/servers/${server.id}`)}&reference=${encodeURIComponent(resource.data?.request_reference || "")}`}
        >
          Report a problem →
        </Link>
      </div>
    </details>
  );
}

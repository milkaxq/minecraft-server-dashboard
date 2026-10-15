"use client";
import Link from "next/link";
import { useState } from "react";
import { useResource } from "./data";
import { useAuth } from "./auth";
import { ErrorMessage } from "./ui";
import { formatBytes } from "../lib/api";

export function Notifications() {
  const resource = useResource("/notifications", 30000);
  const { api } = useAuth();
  const [busy, setBusy] = useState(null);
  const [error, setError] = useState(null);
  const [dismissed, setDismissed] = useState([]);
  const items = Array.isArray(resource.data)
    ? resource.data.filter((item) => !dismissed.includes(item.id))
    : [];
  async function dismiss(id) {
    setBusy(id);
    setError(null);
    try {
      await api(`/notifications/${id}/read`, { method: "POST" });
      setDismissed((previous) => [...previous.slice(-99), id]);
    } catch (error) {
      setError(error);
    } finally {
      setBusy(null);
    }
  }
  if (!items.length && !resource.error && !error) return null;
  return (
    <section className="panel server-insights" aria-label="Notifications">
      <div className="section-heading">
        <h2>Notifications</h2>
        <span>{items.length ? `${items.length} new` : "Unavailable"}</span>
      </div>
      <ErrorMessage error={error || resource.error} />
      {resource.error && (
        <button className="button secondary" disabled={resource.loading} onClick={resource.refresh}>
          Refresh notifications
        </button>
      )}
      <ul className="activity-list">
        {items.map((item) => (
          <li key={item.id}>
            <div>
              <strong>{item.title}</strong>
              <p>
                {item.server_name} · {new Date(item.created_at).toLocaleString()}
              </p>
              <Link
                className="text-link"
                href={`/servers/${item.server_id}${item.title.includes("backup") ? "/backups" : ""}`}
              >
                Open server →
              </Link>
            </div>
            <button className="button secondary" disabled={!!busy} onClick={() => dismiss(item.id)}>
              {busy === item.id ? "Saving…" : "Mark read"}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function ServerInsights({ server, ready }) {
  const backups = useResource(
    ready && server.permissions?.includes("backups") ? `/servers/${server.id}/backups` : null,
    30000,
  );
  const failedBackups = Array.isArray(backups.data)
    ? backups.data.filter((backup) => backup.completed_at && !backup.is_successful)
    : [];
  const activity = useResource(ready ? `/servers/${server.id}/activity` : null, 15000);
  const storage = useResource(
    ready && server.permissions?.includes("files") ? `/servers/${server.id}/storage` : null,
  );
  const data = storage.error ? null : storage.data;
  const percent =
    data?.limit_bytes > 0 ? Math.round((data.used_bytes / data.limit_bytes) * 100) : 0;
  return (
    <section className="server-insights-grid" aria-label="Server history and storage">
      {failedBackups.length > 0 && (
        <div className="panel notice" role="status">
          <strong>
            {failedBackups.length} failed {failedBackups.length === 1 ? "backup" : "backups"} need
            attention
          </strong>
          <p>
            These copies are not available for restore. Keep a successful copy before changing your
            server.
          </p>
          <Link className="text-link" href={`/servers/${server.id}/backups`}>
            Review backups →
          </Link>
        </div>
      )}
      <section className="panel server-insights">
        <h3>Activity</h3>
        <ErrorMessage error={activity.error} />
        {activity.loading && <p>Loading activity…</p>}
        {Array.isArray(activity.data) && (
          <ul className="activity-list">
            {activity.data.map((item) => (
              <li key={item.id}>
                <div>
                  <strong>{item.title}</strong>
                  <p>
                    {item.actor} · {new Date(item.created_at).toLocaleString()}
                  </p>
                  {item.outcome === "REJECTED" && <small>Rejected before completion</small>}
                  {item.outcome === "UNKNOWN" && (
                    <small>Outcome uncertain — inspect before retrying</small>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
        {Array.isArray(activity.data) && !activity.data.length && <p>No recent activity.</p>}
        <button className="button secondary" onClick={activity.refresh} disabled={activity.loading}>
          Refresh activity
        </button>
        <p className="field-help">
          Latest 50 events. A successful request does not mean Minecraft has finished starting or
          stopping.
        </p>
      </section>
      {server.permissions?.includes("files") && (
        <section className="panel server-insights">
          <h3>Storage</h3>
          {!ready ? (
            <p>Available when server setup completes.</p>
          ) : (
            <>
              <ErrorMessage error={storage.error} />
              {storage.loading && <p>Calculating storage…</p>}
              {data && (
                <>
                  <p>
                    <strong>
                      {formatBytes(data.used_bytes)} / {formatBytes(data.limit_bytes)}
                    </strong>{" "}
                    · {percent}% of server storage
                  </p>
                  <progress
                    aria-label="Server storage used"
                    value={Math.min(percent, 100)}
                    max={100}
                  />
                  {percent >= 85 && (
                    <p className="notice">
                      Storage is nearly full. Export a backup before removing files; a full disk can
                      interrupt world saves.
                    </p>
                  )}
                  <ul className="storage-breakdown">
                    {data.categories?.map((category) => (
                      <li key={category.name}>
                        <span>{category.name}</span>
                        <strong>{formatBytes(category.bytes)}</strong>
                      </li>
                    ))}
                  </ul>
                  <p>
                    {data.backup_count} / {data.backup_limit} backups ·{" "}
                    {formatBytes(data.backup_bytes)} in backup storage
                  </p>
                  <p className="field-help">
                    Backups are shown separately from the server disk quota. Folder sizes are
                    approximate while the game writes files.
                    {data.partial
                      ? " This scan is partial: a size/time limit, unreadable folder or symbolic link prevented a complete breakdown."
                      : ""}
                  </p>
                  <Link className="text-link" href={`/servers/${server.id}/backups`}>
                    Manage backups →
                  </Link>
                </>
              )}
              <button
                className="button secondary"
                onClick={storage.refresh}
                disabled={storage.loading}
              >
                Refresh storage
              </button>
            </>
          )}
        </section>
      )}
    </section>
  );
}

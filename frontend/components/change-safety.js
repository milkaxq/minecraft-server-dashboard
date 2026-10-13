"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Select } from "./select";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage } from "./ui";

export function ChangeSafety({ server, offline, onDecision, busy = false }) {
  const { api } = useAuth();
  const backups = useResource(`/servers/${server.id}/backups`, 2000);
  const [selected, setSelected] = useState(null);
  const [skip, setSkip] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState(null);
  const key = useRef(null);
  const complete = (backups.data || [])
    .filter((b) => b.is_successful && b.completed_at && Number.isFinite(Date.parse(b.completed_at)))
    .sort((a, b) => Date.parse(b.completed_at) - Date.parse(a.completed_at));
  const value = selected ?? complete[0]?.uuid ?? "";
  const ready = !backups.error && complete.some((b) => b.uuid === value);
  useEffect(() => {
    onDecision(
      creating
        ? null
        : skip
          ? { acknowledge_no_backup: true, backup: null }
          : ready
            ? { backup: value, acknowledge_no_backup: false }
            : null,
    );
  }, [onDecision, skip, ready, value, creating]);
  async function create() {
    onDecision(null);
    setCreating(true);
    setError(null);
    setSkip(false);
    key.current ||= crypto.randomUUID();
    try {
      const result = await api(`/servers/${server.id}/backups`, {
        method: "POST",
        key: key.current,
        body: { name: "Before addon or version change" },
      });
      setSelected(result.uuid);
      key.current = null;
      backups.refresh();
    } catch (err) {
      setError(err);
    } finally {
      setCreating(false);
    }
  }
  return (
    <div className="change-safety" aria-label="Backup before changing server">
      <h3>Protect your world first</h3>
      <p>
        Backups include your world, settings, and installed addons. A completed backup can restore
        the whole server; it does not include progress made afterward.
      </p>
      <ErrorMessage error={error || backups.error} />
      <label>
        Completed backup
        <Select
          aria-label="Completed backup"
          value={ready ? value : ""}
          disabled={busy || creating || skip}
          onChange={(event) => setSelected(event.target.value)}
        >
          <option value="">Choose a completed backup</option>
          {complete.map((b) => (
            <option key={b.uuid} value={b.uuid}>
              {b.name} · {new Date(b.completed_at).toLocaleString()}
            </option>
          ))}
        </Select>
      </label>
      {selected && !ready && !skip && (
        <p role="status">
          Waiting for the selected backup to complete successfully. Pending or failed backups cannot
          protect this change.
        </p>
      )}
      <div className="file-actions">
        <button
          type="button"
          className="button secondary"
          disabled={
            !offline ||
            busy ||
            creating ||
            !backups.data ||
            backups.data.length >= server.backup_limit
          }
          onClick={create}
        >
          {creating ? "Requesting backup…" : "Create backup first"}
        </button>
        <Link className="text-link" href={`/servers/${server.id}/backups`}>
          View backups and downloads →
        </Link>
      </div>
      {!offline && (
        <p className="field-help">
          Stop the server before taking a consistent safety copy or installing addons.
        </p>
      )}
      {backups.data?.length >= server.backup_limit && (
        <p className="field-help">
          All {server.backup_limit} backup slots are used. Manage existing backups to free a slot;
          nothing is deleted automatically here.
        </p>
      )}
      <label className="checkbox-label">
        <input
          type="checkbox"
          checked={skip}
          disabled={busy || creating}
          onChange={(event) => setSkip(event.target.checked)}
        />
        I understand I am continuing without a backup.
      </label>
      <p className="field-help">
        Choose a completed backup or explicitly accept the risk. The server rechecks backup
        availability before installation.
      </p>
    </div>
  );
}

export function VersionSafety({ server, offline }) {
  const [open, setOpen] = useState(false);
  const [decision, setDecision] = useState(null);
  const [error, setError] = useState(null);
  return (
    <div>
      <h2>Change Minecraft version</h2>
      <p>
        Create a new server with the version you want. Back up and download this world before moving
        it; newer worlds may not open on older versions.
      </p>
      {!open ? (
        <button className="button secondary" onClick={() => setOpen(true)}>
          Prepare version change
        </button>
      ) : (
        <>
          <ChangeSafety server={server} offline={offline} onDecision={setDecision} />
          <p className="field-help">
            This creates a separate server. Your current server and world stay in place. Download
            the world on Worlds and import it only into a compatible version.
          </p>
          <ErrorMessage error={error} />
          <Link
            href="/servers/new"
            className="button secondary"
            onClick={(event) => {
              if (!decision) {
                event.preventDefault();
                setError("Choose a completed backup or confirm continuing without one first.");
              }
            }}
          >
            Create another server
          </Link>
          <Link className="text-link" href={`/servers/${server.id}/worlds`}>
            Download your world →
          </Link>
        </>
      )}
    </div>
  );
}

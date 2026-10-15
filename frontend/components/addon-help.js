"use client";
import Link from "next/link";
import { useRef, useState } from "react";
import { useAuth } from "./auth";
import { ErrorMessage } from "./ui";
import { Select } from "./select";
import { useResource } from "./data";
import { ChunkyProgress } from "./chunky-progress";

// Reviewed recipes only. Project descriptions must never become executable commands.
export function AddonHelp({ id, file, metadata, server, usage }) {
  const [open, setOpen] = useState(false);
  const projectURL = /^[a-zA-Z0-9]{8}$/.test(metadata?.project_id || "")
    ? `https://modrinth.com/project/${metadata.project_id}`
    : null;
  const chunky = /^chunky(?:[-_.]|\.jar$)/i.test(file.name);
  const voice = file.name.toLowerCase().includes("voicechat");
  return (
    <details
      className="installed-addon-help"
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary>Setup &amp; help</summary>
      {voice ? (
        <>
          <p>Set voice distance, group chats and player requirements with the guided settings.</p>
          {server.permissions?.includes("settings") && (
            <Link className="button secondary" href={`/servers/${id}/addons/simple-voice-chat`}>
              Open voice settings
            </Link>
          )}
          <p>
            Players join with the client mod, then press <kbd>V</kbd> to set up their microphone. If
            the key was changed, check Minecraft → Options → Controls → Key Binds → Voice Chat.
          </p>
        </>
      ) : chunky ? (
        open && <ChunkySetup id={id} server={server} usage={usage} />
      ) : (
        <p>
          This addon does not have a guided form yet. Start the server once to let the addon create
          its configuration, then stop it before editing files. Follow its author’s instructions;
          some addons need no changes.
        </p>
      )}
      <div className="file-actions">
        {chunky && (
          <a
            className="button secondary"
            href="https://github.com/pop4959/Chunky/wiki/Pregeneration"
            target="_blank"
            rel="noreferrer"
          >
            Chunky documentation ↗
          </a>
        )}
        {projectURL && (
          <a className="button secondary" href={projectURL} target="_blank" rel="noreferrer">
            Project &amp; documentation ↗
          </a>
        )}
        {server.permissions?.includes("files") && (
          <Link className="button secondary" href={`/servers/${id}/files`}>
            Configuration files
          </Link>
        )}
      </div>
      {!projectURL && !chunky && !voice && (
        <p className="field-help">
          No catalog project is linked to this file. Check the original download for its
          documentation.
        </p>
      )}
    </details>
  );
}

function ChunkySetup({ id, server, usage }) {
  const { api } = useAuth();
  const [radius, setRadius] = useState("500");
  const worlds = useResource(
    server.permissions?.includes("console") ? `/servers/${id}/addons/chunky/worlds` : null,
  );
  const [selection, setWorld] = useState(null);
  const [manual, setManual] = useState(false);
  const choices = worlds.data || [];
  const world = selection ?? choices[0]?.name ?? "";
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const operation = useRef(null);
  const allowed = server.permissions?.includes("console");
  const running = usage?.power_state === "running";
  const validWorld =
    /^[a-zA-Z0-9_-]{1,64}$/.test(world) &&
    (manual || choices.some((option) => option.name === world)) &&
    (manual || !worlds.error);
  async function send(action) {
    if (busy || !allowed || !running || !validWorld) return;
    if (action === "start" && (!confirmed || !["500", "1000", "2000"].includes(radius))) return;
    const command =
      action === "start"
        ? `chunky start ${world} circle 0 0 ${radius}`
        : `chunky ${action} ${world}`;
    if (operation.current?.command !== command || operation.current?.id !== id)
      operation.current = { command, id, key: crypto.randomUUID() };
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      await api(`/servers/${id}/commands`, {
        method: "POST",
        key: operation.current.key,
        body: { command },
      });
      setNotice(
        "Request sent. Check the server console for Chunky’s response and progress; this does not confirm completion.",
      );
      operation.current = null;
      setConfirmed(false);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="chunky-setup">
      <p>
        Prepare terrain before friends explore for smoother exploration. Players do not need Chunky.
      </p>
      <p className="field-help">
        Prepare a circle around coordinates 0, 0. Run while nobody is playing: generation uses CPU
        and disk space.
      </p>
      <ErrorMessage error={worlds.error} />
      {!manual &&
        (worlds.loading ? (
          <p>Finding your worlds…</p>
        ) : worlds.error ? (
          <p className="notice">
            World choices are unavailable. Retry the lookup or enter a known world name manually.
          </p>
        ) : choices.length ? (
          <label>
            Choose a world
            <Select
              aria-label="Choose a world"
              value={world}
              onChange={(event) => {
                setWorld(event.target.value);
                setConfirmed(false);
              }}
            >
              {choices.map((option) => (
                <option key={option.name} value={option.name}>
                  {option.label} · {option.name}
                </option>
              ))}
            </Select>
          </label>
        ) : (
          <p className="notice">
            No supported world folders were found. Start Minecraft once to create its world, or
            enter a custom world name.
          </p>
        ))}
      {!manual && worlds.error && (
        <button type="button" className="button secondary" onClick={worlds.refresh}>
          Retry world lookup
        </button>
      )}
      <button
        type="button"
        className="button secondary"
        onClick={() => {
          setManual(!manual);
          setWorld(null);
          setConfirmed(false);
        }}
      >
        {manual ? "Use detected worlds" : "Enter world name manually"}
      </button>
      {manual && (
        <label>
          World name
          <input
            value={world}
            maxLength={64}
            onChange={(event) => {
              setWorld(event.target.value);
              setConfirmed(false);
            }}
          />
        </label>
      )}
      {manual && !validWorld && (
        <p className="notice">
          Use a world name containing letters, numbers, underscores or hyphens.
        </p>
      )}
      <label>
        Area to prepare
        <Select
          value={radius}
          onChange={(e) => {
            setRadius(e.target.value);
            setConfirmed(false);
          }}
        >
          <option value="500">Small · 500-block radius</option>
          <option value="1000">Medium · 1,000-block radius</option>
          <option value="2000">Large · 2,000-block radius</option>
        </Select>
      </label>
      <p className="field-help">
        Start small and expand later. Doubling the radius prepares about four times the area.
      </p>
      <label className="chunky-confirm">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={(e) => setConfirmed(e.target.checked)}
        />
        I’m ready to generate this area using server CPU and disk space.
      </label>
      {!allowed && (
        <p className="notice">Console permission is required to manage terrain preparation.</p>
      )}
      {!running && (
        <p className="notice">Start your server to use these controls. Chunky must be loaded.</p>
      )}
      <ErrorMessage error={error} />
      {notice && (
        <p role="status" className="notice">
          {notice}
        </p>
      )}
      <ChunkyProgress id={id} world={world} running={running} allowed={allowed} />
      <div className="file-actions">
        <button
          className="button primary"
          disabled={busy || !allowed || !running || !validWorld || !confirmed}
          onClick={() => send("start")}
        >
          {busy ? "Sending…" : "Prepare terrain"}
        </button>
        <button
          className="button secondary"
          disabled={busy || !allowed || !running || !validWorld}
          onClick={() => send("pause")}
        >
          Pause preparation
        </button>
        <button
          className="button secondary"
          disabled={busy || !allowed || !running || !validWorld || !confirmed}
          onClick={() => send("continue")}
        >
          Resume preparation
        </button>
        {allowed && (
          <Link className="button secondary" href={`/servers/${id}/console`}>
            View progress
          </Link>
        )}
      </div>
    </div>
  );
}

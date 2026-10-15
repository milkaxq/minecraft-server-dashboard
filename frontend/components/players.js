"use client";

import Link from "next/link";
import { Select } from "./select";
import { ClientRequirementTag } from "./client-compatibility";
import { PlayerModeration } from "./player-moderation";
import { useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Loading } from "./ui";
import { Confirm } from "./confirm";

export function Players({ id, running, server, usage, usageError }) {
  const { api } = useAuth();
  const list = useResource(`/servers/${id}/whitelist`, 5000);
  const [name, setName] = useState("");
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState("");
  const inFlight = useRef(false);
  const attempt = useRef(null);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const [confirmation, setConfirmation] = useState(null);
  const settings = useResource(`/servers/${id}/settings`);
  const clients = useResource(`/servers/${id}/client-requirements`, 30000);
  const [problem, setProblem] = useState("version");
  const data = list.data;
  const validName = /^[A-Za-z0-9_]{3,16}$/.test(name);
  const existing = data?.players.some((player) => player.name.toLowerCase() === name.toLowerCase());
  const editable = running && data && !list.error && !busy;

  async function change(action, playerName = "", confirmed = false) {
    if (!editable || inFlight.current) return;
    inFlight.current = true;
    const fingerprint = JSON.stringify({ action, name: playerName, confirmed });
    if (attempt.current?.fingerprint !== fingerprint) {
      attempt.current = {
        fingerprint,
        key: crypto.randomUUID(),
        body: { action, name: playerName, revision: data.revision, confirmed },
      };
    }
    const { body, key } = attempt.current;
    setBusy(action);
    setError(null);
    setNotice("");
    try {
      const result = await api(`/servers/${id}/whitelist`, { method: "POST", body, key });
      attempt.current = null;
      setConfirmation(null);
      if (result.confirmed) {
        setNotice(
          action === "add"
            ? `${playerName} can now join when the whitelist is on.`
            : action === "remove"
              ? `${playerName} was removed from the whitelist.`
              : action === "on"
                ? "Whitelist is on. Only allowed players can join."
                : "Whitelist is off. Anyone who passes server login can join.",
        );
        if (action === "add") setName("");
      } else {
        setNotice(
          "Command sent, but Minecraft has not confirmed the change. Refresh the list and check the exact username. The console may explain why it failed.",
        );
      }
    } catch (e) {
      if ([400, 403, 404, 409, 422].includes(e.status)) attempt.current = null;
      setError(e);
    } finally {
      inFlight.current = false;
      setBusy("");
      list.refresh();
    }
  }

  const players =
    data?.players.filter((player) =>
      player.name.toLowerCase().includes(search.trim().toLowerCase()),
    ) || [];
  return (
    <>
      <section className="panel management-panel" aria-label="Live players">
        <div className="section-heading">
          <div>
            <h2>Playing together</h2>
            <p className="field-help">
              {usageError || !usage
                ? "Live player count unavailable."
                : usage.power_state === "offline"
                  ? "Minecraft is stopped. No players are connected."
                  : usage.players_available && Number.isInteger(usage.players_online)
                    ? `${usage.players_online} player${usage.players_online === 1 ? "" : "s"} online`
                    : "Minecraft is running, but player counts are unavailable."}
            </p>
          </div>
          {server.permissions?.includes("members") && (
            <Link className="button primary" href={`/servers/${id}#invite-friends`}>
              Invite friends
            </Link>
          )}
        </div>
        {!usageError && running && usage.players_available && usage.players_online > 0 && (
          <>
            <p className="field-help">
              Player names reported by Minecraft; the list may be incomplete.
            </p>
            {usage.player_names?.length ? (
              <ul className="player-name-sample">
                {usage.player_names.map((player) => (
                  <li key={player}>{player}</li>
                ))}
              </ul>
            ) : (
              <p>No player names reported.</p>
            )}
          </>
        )}
      </section>
      {list.loading && !data && <Loading message="Checking saved access…" />}
      <ErrorMessage error={error || list.error} />
      {list.error && (
        <button
          className="button secondary"
          disabled={!!busy || list.loading || list.error.status === 429}
          onClick={list.refresh}
        >
          Refresh access list
        </button>
      )}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      {!running && !usageError && usage && (
        <p className="notice">Start Minecraft to change player access.</p>
      )}
      {data && (
        <>
          <section className="panel management-panel">
            <div className="section-heading">
              <div className="whitelist-intro">
                <h2>Who can join?</h2>
              </div>
              <button
                className={`button ${data.enabled ? "secondary" : "primary"}`}
                disabled={!editable}
                onClick={() => (data.enabled ? setConfirmation({ action: "off" }) : change("on"))}
              >
                {busy === "on" || busy === "off"
                  ? "Updating…"
                  : data.enabled
                    ? "Turn whitelist off"
                    : "Turn whitelist on"}
              </button>
            </div>
            <p className="whitelist-status">
              <span className={`status-dot ${data.enabled ? "online" : "offline"}`} />
              {data.enabled
                ? "Whitelist on · only allowed players can join"
                : "Whitelist off · joining is open"}
            </p>
            <p className="field-help">Login rules still apply. No restart needed.</p>
          </section>
          <section className="panel management-panel">
            <div className="section-heading">
              <h2>
                Allowed players <span className="whitelist-count">{data.players.length}</span>
              </h2>
              <button
                className="button secondary"
                disabled={!!busy || list.loading || list.error?.status === 429}
                onClick={list.refresh}
              >
                Refresh list
              </button>
            </div>
            <form
              className="whitelist-add"
              onSubmit={(event) => {
                event.preventDefault();
                if (validName && !existing) change("add", name);
              }}
            >
              <label className="field">
                Minecraft username
                <input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="e.g. milkaxq"
                  maxLength={16}
                  pattern="[A-Za-z0-9_]{3,16}"
                  required
                  disabled={!!busy}
                  aria-describedby="whitelist-name-help"
                  autoComplete="off"
                  spellCheck={false}
                  autoCapitalize="none"
                />
              </label>
              <button className="button primary" disabled={!editable || !validName || existing}>
                {busy === "add" ? "Adding…" : "Add player"}
              </button>
            </form>
            <p id="whitelist-name-help" className="field-help">
              {existing
                ? "This player is already allowed."
                : "Use their Minecraft username, not their email."}
            </p>
            {data.players.length > 0 && (
              <label className="field whitelist-search">
                Find a player
                <input
                  type="search"
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder="Search usernames"
                />
              </label>
            )}
            {players.length ? (
              <ul className="whitelist-players">
                {players.map((player) => (
                  <li key={player.uuid}>
                    <div className="whitelist-player">
                      <span className="whitelist-avatar" aria-hidden="true">
                        {player.name.slice(0, 2).toUpperCase()}
                      </span>
                      <strong>{player.name}</strong>
                    </div>
                    <button
                      className="button danger-outline"
                      disabled={!editable}
                      aria-label={`Remove ${player.name}`}
                      onClick={() => setConfirmation({ action: "remove", name: player.name })}
                    >
                      Remove
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="management-empty">
                {data.players.length ? "No usernames match your search." : "No players added yet."}
              </p>
            )}
          </section>
        </>
      )}
      <PlayerModeration id={id} running={running} />
      <section className="panel management-panel" aria-label="Connection help">
        <h2>Friend cannot join?</h2>
        <label className="field">
          What does their game say?
          <Select
            aria-label="Connection problem"
            value={problem}
            onChange={(e) => setProblem(e.target.value)}
          >
            <option value="version">Wrong version or incompatible client</option>
            <option value="mods">Missing mods or modpack</option>
            <option value="whitelist">Not whitelisted</option>
            <option value="login">Invalid session or restart launcher</option>
            <option value="banned">Banned from this server</option>
            <option value="network">Timed out or cannot connect</option>
          </Select>
        </label>
        <p className="notice">
          {problem === "version"
            ? `This server uses ${server.server_type} with Minecraft ${server.minecraft_version}. Use the player invitation to choose a matching launcher profile; a compatibility plugin may allow other versions.`
            : problem === "mods"
              ? "Open the player invitation and import its current pack. Refresh the invitation after changing required mods. Server plugins usually stay on the server; a client-mod requirement is shown separately."
              : problem === "whitelist"
                ? list.error || !data
                  ? "The whitelist could not be checked. Reload the saved list before changing access."
                  : data.enabled
                    ? "Whitelist is on. Add their exact Minecraft username in Allowed players."
                    : "Whitelist is off. Reconnect and check the console: another plugin or proxy may be applying its own joining rules."
                : problem === "login"
                  ? settings.error || !settings.data
                    ? "Login settings could not be checked. Open Settings before deciding which login rule to change."
                    : settings.data.online_mode
                      ? "Microsoft verification is on. Sign into an account that owns Java Edition, then restart the launcher. A modpack does not bypass account verification."
                      : "Microsoft verification is off. Check authentication-plugin or proxy messages in the console. If AuthMe is installed, the player may need its in-game /register or /login command. Do not share passwords here."
                  : problem === "banned"
                    ? "Review Banned players above. Unban only if you intend to restore access; whitelist and login checks still apply. If they are not listed, check the console for IP bans or plugin/proxy rules."
                    : "Check that Minecraft is online and share the exact hostname and port from the invitation. A timeout alone does not identify a mod or login problem."}
        </p>
        {problem === "mods" && clients.data?.addons?.length > 0 && (
          <ul className="plugin-list">
            {clients.data.addons.map((addon) => (
              <li key={addon.filename}>
                <div>
                  <strong>{addon.title || addon.filename}</strong>
                  <ClientRequirementTag addon={addon} explain />
                </div>
              </li>
            ))}
          </ul>
        )}
        {problem === "mods" && clients.error && (
          <p className="field-help">
            Current client requirements could not be checked. Refresh the player setup before
            sharing it.
          </p>
        )}

        {server.permissions?.includes("console") && (
          <Link className="button secondary" href={`/servers/${id}/console`}>
            Open console
          </Link>
        )}
      </section>
      {confirmation && (
        <Confirm
          title={
            confirmation.action === "off"
              ? "Open joining to everyone?"
              : `Remove ${confirmation.name}?`
          }
          label={confirmation.action === "off" ? "Turn whitelist off" : "Remove player"}
          busy={!!busy}
          confirmDisabled={!editable}
          onCancel={() => setConfirmation(null)}
          onConfirm={() => change(confirmation.action, confirmation.name || "", true)}
        >
          <p>
            {confirmation.action === "off"
              ? "Anyone who passes server login will be able to join. Your allowed-player list will be kept for when you turn the whitelist back on."
              : "They will no longer be allowed to join while the whitelist is on. Their inventory and world progress are kept. This does not ban them."}
          </p>
          <ErrorMessage error={error} />
        </Confirm>
      )}
    </>
  );
}

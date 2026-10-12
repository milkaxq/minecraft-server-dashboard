"use client";

import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Loading } from "./ui";
import { Confirm } from "./management";

export function Players({ id, running }) {
  const { api } = useAuth();
  const list = useResource(`/servers/${id}/whitelist`, 5000);
  const [name, setName] = useState("");
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const [confirmation, setConfirmation] = useState(null);
  const [retry, setRetry] = useState(null);
  const data = list.data;
  const validName = /^[A-Za-z0-9_]{3,16}$/.test(name);
  const existing = data?.players.some((player) => player.name.toLowerCase() === name.toLowerCase());
  const editable = running && data && !list.error && !busy;

  async function change(action, playerName = "", confirmed = false) {
    if (!editable) return;
    const body = { action, name: playerName, revision: data.revision, confirmed };
    // Preserve the same operation key after a lost response to avoid replaying commands.
    const fingerprint = JSON.stringify(body);
    const key = retry?.fingerprint === fingerprint ? retry.key : crypto.randomUUID();
    setRetry({ fingerprint, key });
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      const result = await api(`/servers/${id}/whitelist`, { method: "POST", body, key });
      setRetry(null);
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
      setError(e);
    } finally {
      setBusy(false);
      list.refresh();
    }
  }

  if (list.loading && !data) return <Loading />;
  const players =
    data?.players.filter((player) => player.name.toLowerCase().includes(search.toLowerCase())) ||
    [];
  return (
    <>
      <ErrorMessage error={error || list.error} />
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      {!running && (
        <p className="notice">
          Start your server to change player access. You can still view the saved list while it is
          stopped.
        </p>
      )}
      {data && (
        <>
          <section className="panel management-panel">
            <div className="section-heading">
              <div className="whitelist-intro">
                <h2>Who can join?</h2>
                <p className="field-help">
                  Keep your world private with a list of allowed Minecraft usernames.
                </p>
              </div>
              <button
                className={`button ${data.enabled ? "secondary" : "primary"}`}
                disabled={!editable}
                onClick={() => (data.enabled ? setConfirmation({ action: "off" }) : change("on"))}
              >
                {busy ? "Updating…" : data.enabled ? "Turn whitelist off" : "Turn whitelist on"}
              </button>
            </div>
            <p className="whitelist-status">
              <span className={`status-dot ${data.enabled ? "online" : "offline"}`} />
              {data.enabled
                ? "Whitelist on · only allowed players can join"
                : "Whitelist off · joining is open"}
            </p>
            <p className="field-help">
              Players still need to pass Minecraft or AuthMe login. This list does not give anyone
              admin powers. Changes do not need a restart.
            </p>
          </section>
          <section className="panel management-panel">
            <h2>Add a player</h2>
            <p className="field-help">
              Ask your friend for their exact in-game username, including capitals. Use their
              Minecraft name, not an email or Blockyard account.
            </p>
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
                  disabled={busy}
                  aria-describedby="whitelist-name-help"
                  autoComplete="off"
                  spellCheck={false}
                  autoCapitalize="none"
                />
              </label>
              <button className="button primary" disabled={!editable || !validName || existing}>
                {busy ? "Updating…" : "Add player"}
              </button>
            </form>
            <p id="whitelist-name-help" className="field-help">
              {existing
                ? "This player is already allowed."
                : "3–16 letters, numbers or underscores. Microsoft accounts must have a valid Java Edition profile."}
            </p>
          </section>
          <section className="panel management-panel">
            <div className="section-heading">
              <h2>
                Allowed players <span className="whitelist-count">{data.players.length}</span>
              </h2>
              <button
                className="button secondary"
                disabled={busy || list.loading}
                onClick={list.refresh}
              >
                Refresh list
              </button>
            </div>
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
                {data.players.length
                  ? "No usernames match your search."
                  : "No players added yet. Add yourself and your friends before enabling the whitelist."}
              </p>
            )}
          </section>
        </>
      )}
      {confirmation && (
        <Confirm
          title={
            confirmation.action === "off"
              ? "Open joining to everyone?"
              : `Remove ${confirmation.name}?`
          }
          label={confirmation.action === "off" ? "Turn whitelist off" : "Remove player"}
          busy={busy}
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

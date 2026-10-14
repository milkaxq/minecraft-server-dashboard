"use client";
import { useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Confirm } from "./management";
import { ErrorMessage } from "./ui";
import { Select } from "./select";

export function PlayerModeration({ id, running }) {
  const { api } = useAuth();
  const bans = useResource(`/servers/${id}/players/bans`, 15000);
  const [name, setName] = useState("");
  const [action, setAction] = useState("kick");
  const [reason, setReason] = useState("");
  const [target, setTarget] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const attempt = useRef(null);
  const editable = running && !!bans.data && !bans.error && !busy;
  const valid = /^[A-Za-z0-9_]{3,16}$/.test(name) && reason.trim().length > 0;
  async function apply() {
    if (!editable || !target) return;
    const fingerprint = JSON.stringify(target);
    if (attempt.current?.fingerprint !== fingerprint)
      attempt.current = {
        fingerprint,
        key: crypto.randomUUID(),
        body: { ...target, revision: bans.data.revision, confirmed: true },
      };
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      const result = await api(`/servers/${id}/players/actions`, {
        method: "POST",
        key: attempt.current.key,
        body: attempt.current.body,
      });
      setNotice(
        target.action === "kick"
          ? "Kick command delivered. Check the console to confirm the player disconnected; they may join again."
          : result.confirmed
            ? `${target.name} ${target.action === "ban" ? "is banned" : "is no longer banned"}.`
            : "Command delivered, but Minecraft has not confirmed the ban-list change. Refresh and check the console before trying again.",
      );
      attempt.current = null;
      setTarget(null);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
      bans.refresh();
    }
  }
  return (
    <section className="panel management-panel" aria-label="Player moderation">
      <h2>Keep your world friendly</h2>
      <p className="field-help">
        Kick disconnects a player for now. Ban prevents that username from joining until you unban
        them. These actions do not delete player progress.
      </p>
      <ErrorMessage error={bans.error || (!target && error)} />
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      <form
        className="player-moderation-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (editable && valid) setTarget({ action, name, reason: reason.trim() });
        }}
      >
        <div className="field-row">
          <label className="field">
            Player username
            <input
              value={name}
              maxLength={16}
              pattern="[A-Za-z0-9_]{3,16}"
              required
              disabled={busy}
              onChange={(e) => setName(e.target.value)}
              autoComplete="off"
            />
          </label>
          <label className="field">
            Action
            <Select
              aria-label="Player action"
              value={action}
              disabled={busy}
              onChange={(e) => setAction(e.target.value)}
            >
              <option value="kick">Kick for now</option>
              <option value="ban">Ban player</option>
            </Select>
          </label>
        </div>
        <label className="field">
          Reason
          <input
            value={reason}
            maxLength={160}
            required
            disabled={busy}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Explain the rule or problem to the player"
          />
        </label>
        <button className="button danger-outline" disabled={!editable || !valid}>
          Review player action
        </button>
      </form>
      {!running && (
        <p className="field-help">
          Live server status must confirm Minecraft is running before player actions are available.
        </p>
      )}
      <div className="section-heading">
        <h3>Banned players</h3>
        <button className="button secondary" disabled={busy || bans.loading} onClick={bans.refresh}>
          Refresh bans
        </button>
      </div>
      {bans.loading ? (
        <p>Checking bans…</p>
      ) : (
        bans.data &&
        (bans.data.players.length ? (
          <ul className="whitelist-players">
            {bans.data.players.map((player) => (
              <li key={player.uuid}>
                <div>
                  <strong>{player.name}</strong>
                  <p className="field-help">{player.reason || "No reason recorded"}</p>
                </div>
                <button
                  className="button secondary"
                  disabled={!editable}
                  onClick={() => setTarget({ action: "unban", name: player.name, reason: "" })}
                >
                  Unban {player.name}
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="management-empty">No saved player bans.</p>
        ))
      )}
      {target && (
        <Confirm
          title={`${target.action === "unban" ? "Unban" : target.action === "ban" ? "Ban" : "Kick"} ${target.name}?`}
          label="Confirm player action"
          busy={busy}
          confirmDisabled={!editable}
          onCancel={() => setTarget(null)}
          onConfirm={apply}
        >
          <p>
            {target.action === "unban"
              ? "They can attempt to join again. Whitelist, login and client requirements still apply."
              : target.action === "ban"
                ? "They cannot join with this username until unbanned."
                : "They will be disconnected if online, but can join again."}
          </p>
          {target.reason && <p>Reason: {target.reason}</p>}
          <ErrorMessage error={error} />
          {error && (
            <p className="field-help">
              If delivery is uncertain, inspect the console before retrying. The same action keeps
              its original request reference.
            </p>
          )}
        </Confirm>
      )}
    </section>
  );
}

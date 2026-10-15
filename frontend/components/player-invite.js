"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage } from "./ui";
import { Confirm } from "./confirm";
import { LauncherSetup, LauncherPicker, useLauncherChoice } from "./player-launcher";
import { useReturningPlayer } from "./player-returning";
import { PlayerJoinHelp } from "./player-join-help";
import { request } from "../lib/api";

export function PlayerInvite({ server, ready }) {
  const { api } = useAuth();
  const permitted = server.permissions?.includes("members");
  const resource = useResource(permitted ? `/servers/${server.id}/player-invite` : null, 30000);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [optionalChoice, setOptionalChoice] = useState(null);
  const [betaChoice, setBetaChoice] = useState(null);
  const [copied, setCopied] = useState(null);
  const [fallback, setFallback] = useState(null);
  const [optionsDraft, setOptionsDraft] = useState(null);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!permitted) return;
    const timer = setInterval(() => setNow(Date.now()), 30000);
    return () => clearInterval(timer);
  }, [permitted]);
  const invite = resource.data;
  const includeOptional = optionalChoice ?? invite?.setup?.include_optional ?? false;
  const includeBeta = betaChoice ?? invite?.setup?.include_beta ?? false;
  const expired = invite && new Date(invite.expires_at).getTime() <= now;
  const optionsChanged =
    invite &&
    (includeBeta !== !!invite.setup?.include_beta ||
      includeOptional !== !!invite.setup?.include_optional);
  const shareReady =
    invite && !expired && !optionsChanged && !invite.setup?.pack_reason && !resource.error && ready;
  const href = invite?.slug && !expired ? `/join/${invite.slug}` : null;
  async function publish(
    selection = { include_beta: includeBeta, include_optional: includeOptional },
  ) {
    if (!ready || busy || resource.loading || resource.error) return;
    setBusy(true);
    setError(null);
    setCopied(null);
    setFallback(null);
    try {
      await api(`/servers/${server.id}/player-invite`, {
        method: "POST",
        body: selection,
      });
      setBetaChoice(selection.include_beta);
      setOptionalChoice(selection.include_optional);
      setOptionsDraft(null);
      resource.refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function copy() {
    if (!shareReady || !href || busy || resource.loading) return;
    setError(null);
    setCopied(null);
    setFallback(null);
    const text = new URL(href, location.origin).href;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(href);
    } catch {
      setFallback({ href, text });
    }
  }
  async function revoke() {
    if (busy || resource.error) return;
    setBusy(true);
    setError(null);
    try {
      await api(`/servers/${server.id}/player-invite`, { method: "DELETE" });
      setCopied(null);
      setFallback(null);
      setOptionsDraft(null);
      resource.refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  if (!permitted) return null;
  return (
    <section id="invite-friends" className="panel invite-friends" aria-label="Invite your friends">
      <div className="section-heading">
        <div>
          <h2>Invite your friends</h2>
        </div>
      </div>
      <ErrorMessage error={error || resource.error} />
      {resource.error && (
        <button
          className="button secondary"
          disabled={resource.loading || busy}
          onClick={resource.refresh}
        >
          Retry invitation
        </button>
      )}
      {resource.loading && <p role="status">Checking your player invitation…</p>}
      {invite && (
        <div className="invite-preview">
          <strong>
            {expired
              ? "Invite link expired"
              : optionsChanged
                ? "Save your player download choices"
                : invite.setup?.pack_reason
                  ? "Player setup needs a check"
                  : invite.setup?.pack_ready
                    ? "Player download ready"
                    : "Player page ready"}
          </strong>
          {(expired || optionsChanged || invite.setup?.pack_reason) && (
            <p>
              {expired
                ? "Refresh the invitation to share it again."
                : optionsChanged
                  ? "Refresh to apply your download choices."
                  : invite.setup.pack_reason}
            </p>
          )}
        </div>
      )}
      <div className="invite-actions">
        <button
          className={`button ${shareReady ? "secondary" : "primary"}`}
          disabled={!ready || busy || resource.loading || !!resource.error}
          onClick={() => publish()}
        >
          {busy
            ? "Preparing player setup…"
            : invite
              ? "Refresh player setup"
              : "Prepare invite link"}
        </button>
        {href && (
          <>
            <button
              className={`button ${shareReady ? "primary" : "secondary"}`}
              disabled={busy || !shareReady || resource.loading}
              onClick={copy}
            >
              Copy invite link
            </button>
            <Link className="button secondary" href={href} target="_blank" rel="noreferrer">
              Preview player page ↗
            </Link>
          </>
        )}
      </div>
      <button
        className="text-link invite-settings-button"
        type="button"
        disabled={busy || !ready || resource.loading || !!resource.error}
        onClick={() => {
          setError(null);
          setOptionsDraft({ include_beta: includeBeta, include_optional: includeOptional });
        }}
      >
        Download options
      </button>
      {copied === href && shareReady && (
        <p role="status">Invite link copied. Send it to your friends.</p>
      )}
      {fallback?.href === href && shareReady && (
        <label className="field invite-copy-fallback">
          Invite link
          <input
            aria-label="Invite link"
            readOnly
            spellCheck={false}
            value={fallback.text}
            onFocus={(event) => event.target.select()}
          />
          <span className="field-help">Select and copy this link.</span>
        </label>
      )}
      {!ready && <p className="field-help">Available after installation.</p>}
      {optionsDraft && (
        <Confirm
          title="Player download options"
          label="Update invitation"
          confirmVariant="primary"
          busy={busy}
          confirmDisabled={!ready || !!resource.error || resource.loading}
          onCancel={() => {
            setOptionsDraft(null);
            setError(null);
          }}
          onConfirm={() => publish(optionsDraft)}
        >
          <div className="invite-options">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={optionsDraft.include_beta}
                disabled={busy}
                onChange={(event) =>
                  setOptionsDraft({ ...optionsDraft, include_beta: event.target.checked })
                }
              />
              Include beta client builds
            </label>
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={optionsDraft.include_optional}
                disabled={busy}
                onChange={(event) =>
                  setOptionsDraft({ ...optionsDraft, include_optional: event.target.checked })
                }
              />
              Include optional client features
            </label>
          </div>
          <ErrorMessage error={error || resource.error} />
          {invite && (
            <div className="invite-link-access">
              <small>Link expires {new Date(invite.expires_at).toLocaleDateString()}.</small>
              <button
                className="button secondary"
                disabled={busy || !!resource.error}
                onClick={revoke}
              >
                Disable invite link
              </button>
            </div>
          )}
        </Confirm>
      )}
    </section>
  );
}

export function PublicPlayerJoin({ slug }) {
  const [launcher, setLauncher] = useLauncherChoice();
  const manualLauncher = ["official", "other"].includes(launcher);
  const [setup, setSetup] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState(false);
  const [downloadedRevision, setDownloadedRevision] = useState(null);
  const [busy, setBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const [showSetup, setShowSetup] = useState(false);
  const [showOptional, setShowOptional] = useState(false);
  const player = useReturningPlayer(setup);
  const packDownloaded = downloadedRevision !== null && downloadedRevision === player.revision;
  const requiredMods = setup?.addons?.some((addon) => addon.required);
  const setupIssue =
    setup?.pack_reason ||
    (requiredMods && !setup?.pack_ready
      ? "Required client downloads are not ready. Ask your host to refresh player setup."
      : null);
  const showDownloadSteps =
    setup?.pack_ready &&
    !setupIssue &&
    (requiredMods || showOptional) &&
    (!player.returning || showSetup);
  // A public player page deliberately never calls authenticated management APIs.
  useEffect(() => {
    const controller = new AbortController();
    let timer;
    let inFlight = false;
    async function refresh() {
      if (inFlight || controller.signal.aborted) return;
      inFlight = true;
      let delay = 60000;
      try {
        const data = await request(`/join/${encodeURIComponent(slug)}`, {
          signal: controller.signal,
        });
        if (!controller.signal.aborted) {
          setSetup(data);
          setError(null);
        }
      } catch (error) {
        if (!controller.signal.aborted) {
          setSetup(null);
          setError(error);
        }
        if (error.status === 429) delay = Math.max(delay, error.retryAfterMs || 60000);
      } finally {
        inFlight = false;
        if (!controller.signal.aborted) {
          setLoading(false);
          timer = setTimeout(refresh, delay);
        }
      }
    }
    refresh();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [slug, reload]);
  async function copy() {
    try {
      await navigator.clipboard.writeText(setup.address);
      setCopied(true);
    } catch {
      setError(new Error("Copy unavailable. Select the address below and copy it."));
    }
  }
  async function download() {
    setBusy(true);
    setError(null);
    try {
      const blob = await request(`/join/${encodeURIComponent(slug)}/pack`, { download: true });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "blockyard-player-setup.mrpack";
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 10000);
      setDownloadedRevision(player.revision);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="public-player-page">
      <header>
        <Link href="https://playblockyard.com" className="public-brand">
          blockyard<span>.</span>
        </Link>
        <span>YOU’RE INVITED</span>
      </header>
      {loading ? (
        <section className="panel">
          <h1>Getting your player setup…</h1>
        </section>
      ) : !setup ? (
        <section className="panel">
          <h1>
            {error?.status === 404
              ? "This invite is unavailable"
              : "Player setup couldn’t be loaded"}
          </h1>
          <p>
            {error?.status === 404
              ? "Ask your host for a fresh invite link."
              : "We’ll try again automatically. Your host may also be able to help."}
          </p>
          {error?.status !== 404 && error?.status !== 429 && (
            <button
              className="button secondary"
              onClick={() => {
                setLoading(true);
                setReload((value) => value + 1);
              }}
            >
              Try again
            </button>
          )}
        </section>
      ) : (
        <>
          <section className="player-hero">
            <span className="eyebrow">YOUR NEXT ADVENTURE</span>
            <h1>Join {setup.name}</h1>
            <p>Your friends are waiting. Let’s get you into their world.</p>
            <span className="player-game-tag">Minecraft Java {setup.minecraft_version}</span>
            <div className="player-address">
              <code>{setup.address}</code>
              <button className="button secondary" onClick={copy}>
                {copied ? "Copied" : "Copy address"}
              </button>
            </div>
          </section>
          {player.changed && !setupIssue && (
            <p className="notice" role="status">
              Your host published a new player setup.{" "}
              {requiredMods
                ? "Download and import the latest pack before joining."
                : "Review the current version and access rules before joining."}
            </p>
          )}
          {player.returning && !showSetup && (
            <section className="panel returning-player" aria-label="Returning player">
              <h2>Welcome back. Let’s play.</h2>
              <p>
                You marked this setup ready in this browser. Open your matching Minecraft profile,
                then use the address above.
              </p>
              {setup.pack_ready && (
                <button
                  className="button secondary"
                  onClick={() => {
                    setShowSetup(true);
                    if (!requiredMods) setShowOptional(true);
                  }}
                >
                  Show setup & downloads
                </button>
              )}
            </section>
          )}
          {(!player.returning || showSetup) && (
            <div className="player-setup-heading">
              <span className="eyebrow">LET’S PLAY</span>
              <h2>
                {showDownloadSteps
                  ? "Three steps to your adventure."
                  : requiredMods || setupIssue
                    ? "Your way into the world."
                    : "Open Minecraft. Join your friends."}
              </h2>
              <p>
                {showDownloadSteps
                  ? "Download once. Import into your launcher. Join your friends."
                  : setupIssue
                    ? "Your host needs to finish the setup below."
                    : requiredMods
                      ? "Use your imported player profile and the address above."
                      : "No required client downloads are listed in this invitation."}
              </p>
            </div>
          )}
          {setup.pack_ready && !requiredMods && !showOptional && (
            <button
              className="button secondary optional-player-extras"
              onClick={() => {
                setShowOptional(true);
                setShowSetup(true);
              }}
            >
              Set up optional player features
            </button>
          )}
          {showDownloadSteps && <LauncherPicker launcher={launcher} onChange={setLauncher} />}
          <section className="player-steps" aria-label="Join steps">
            {showDownloadSteps ? (
              <>
                <article className="panel player-step">
                  <span className="step-number">1</span>
                  <h2>
                    {manualLauncher
                      ? "Use this Minecraft profile"
                      : setup.addons?.some((addon) => addon.required)
                        ? "Download your player setup"
                        : "Add optional player features"}
                  </h2>
                  <p>Includes the matching loader and verified client mod download instructions.</p>
                  {manualLauncher && (
                    <p>
                      Download one client mods ZIP in step 2. The player pack is also available
                      below for compatible import tools.
                    </p>
                  )}
                  {manualLauncher ? (
                    <details>
                      <summary>Download .mrpack for an import tool</summary>
                      <button className="button secondary wide" disabled={busy} onClick={download}>
                        {busy ? "Preparing…" : "Download player pack"}
                      </button>
                    </details>
                  ) : (
                    <button className="button primary wide" disabled={busy} onClick={download}>
                      {busy ? "Preparing…" : "Download player pack"}
                    </button>
                  )}
                  {packDownloaded && (
                    <p role="status">Downloaded. Import it in your launcher next.</p>
                  )}
                </article>
                <article className="panel player-step">
                  <span className="step-number">2</span>
                  <LauncherSetup launcher={launcher} setup={setup} slug={slug} />
                </article>
              </>
            ) : setupIssue ? (
              <article className="panel player-step">
                <span className="step-number">1</span>
                <h2>{setupIssue ? "Ask your host to finish player setup" : "Open Minecraft"}</h2>
                <p>{setupIssue || `Launch Minecraft Java ${setup.minecraft_version}.`}</p>
                {setup.addons
                  ?.filter((addon) => setupIssue || addon.required)
                  .map((addon) => (
                    <p key={addon.project || addon.title}>
                      <strong>{addon.title}</strong> ·{" "}
                      {addon.required ? "Required to join" : "Optional features"}
                      {addon.project && (
                        <a
                          className="text-link"
                          href={`https://modrinth.com/project/${encodeURIComponent(addon.project)}`}
                          target="_blank"
                          rel="noreferrer"
                        >
                          Find download ↗
                        </a>
                      )}
                    </p>
                  ))}
              </article>
            ) : null}
            <article className="panel player-step">
              <span className="step-number">
                {showDownloadSteps ? "3" : setupIssue ? "2" : "1"}
              </span>
              <h2>Join and play</h2>
              <p>
                {!requiredMods &&
                  !showDownloadSteps &&
                  `Open Minecraft Java ${setup.minecraft_version}. `}
                Multiplayer → Add Server. Paste the address above, then Join Server.
              </p>
              <p>Ask your host to start the server before joining.</p>
              {setup.whitelist && (
                <p className="notice">
                  Send your exact Minecraft username to the host for whitelist approval.
                </p>
              )}
              {setup.verified_login && (
                <p>Sign into your launcher with a Microsoft account that owns Java Edition.</p>
              )}
              {!setup.verified_login && (
                <p className="field-help">
                  Microsoft account verification is off. A server login plugin may ask you to
                  register or log in.
                </p>
              )}
              <button className="button secondary" onClick={copy}>
                Copy server address
              </button>
              {setup.addons?.some((addon) => addon.project === "9eGKb6K1") && (
                <p>
                  For voice chat, press <kbd>V</kbd> after joining and select your microphone.
                </p>
              )}
            </article>
          </section>
          {!setupIssue && (!requiredMods || setup.pack_ready) && !player.returning && (
            <section className="panel player-ready-check" aria-label="Remember player setup">
              <h2>
                {requiredMods
                  ? "Already imported and launched the player pack?"
                  : "Have the matching Minecraft version ready?"}
              </h2>
              <p>
                Skip these instructions next time. This is your confirmation; the website cannot
                check your launcher.
              </p>
              <button
                className="button primary"
                onClick={() => {
                  player.ready();
                  setShowSetup(false);
                }}
              >
                My Minecraft setup is ready
              </button>
            </section>
          )}
          <PlayerJoinHelp setup={setup} />
          <details className="panel">
            <summary>
              {setup.pack_ready ? "What will be installed?" : "About this player setup"}
            </summary>
            {setup.notes?.map((note) => (
              <p key={note}>{note}</p>
            ))}
            <ul>
              {setup.addons?.map((addon) => (
                <li key={addon.project || addon.title}>
                  {addon.title} · {addon.required ? "Required" : "Optional"}
                </li>
              ))}
            </ul>
            <p>
              Published by your host on {new Date(setup.published_at).toLocaleString()}.{" "}
              {setup.pack_ready
                ? "This pack covers verified client JARs, not arbitrary custom configurations or manually uploaded files. "
                : "These are the host’s published joining requirements. "}
              If the host changes the server, ask them to refresh this setup.
            </p>
            <p>
              Ordinary Minecraft cannot install mods from its connection prompt. A launcher import
              downloads them before you play. Some changes require restarting Minecraft.
            </p>
          </details>
        </>
      )}
      {error && (
        <p role="alert" className="error-message">
          {error.status === 404
            ? "The link expired or was disabled. Ask your host for a new one."
            : error.message}
        </p>
      )}
      <footer>Play together. Your host controls this server.</footer>
    </main>
  );
}

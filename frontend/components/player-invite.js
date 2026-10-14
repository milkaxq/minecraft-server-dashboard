"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Icon } from "./ui";
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
  const [copied, setCopied] = useState(false);
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
  async function publish() {
    setBusy(true);
    setError(null);
    setCopied(false);
    try {
      await api(`/servers/${server.id}/player-invite`, {
        method: "POST",
        body: { include_beta: includeBeta, include_optional: includeOptional },
      });
      resource.refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(new URL(href, location.origin).href);
      setCopied(true);
    } catch {
      setError(new Error("Copy unavailable. Open the player page and share its address."));
    }
  }
  async function revoke() {
    setBusy(true);
    setError(null);
    try {
      await api(`/servers/${server.id}/player-invite`, { method: "DELETE" });
      setCopied(false);
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
          <span className="eyebrow">PLAY TOGETHER</span>
          <h2>Invite your friends</h2>
          <p>One player page. No dashboard account needed.</p>
        </div>
        <Icon name="user" size={28} />
      </div>
      <ErrorMessage error={error || resource.error} />
      {resource.loading && <p role="status">Checking your player invitation…</p>}
      {!resource.loading && !resource.error && !invite && ready && (
        <p className="notice">
          Prepare player setup before sharing. After changing addons or access rules, refresh player
          setup so friends get matching downloads.
        </p>
      )}
      {invite && (
        <div className="invite-preview">
          <strong>
            {expired
              ? "Invite link expired"
              : optionsChanged
                ? "Save your player download choices"
                : invite.setup?.pack_ready
                  ? "Player download ready"
                  : invite.setup?.pack_reason
                    ? "Player setup needs a check"
                    : "Player page ready"}
          </strong>
          <p>
            {expired
              ? "Refresh player setup to create a new working invite."
              : optionsChanged
                ? "Refresh player setup to apply these choices before sharing."
                : invite.setup?.pack_reason ||
                  (invite.setup?.pack_ready
                    ? "Their launcher can download the verified client mods."
                    : "Friends get the address, version and join steps.")}
          </p>
          {!!invite.setup?.notes?.length && (
            <details>
              <summary>What is included in the player setup?</summary>
              {invite.setup.notes.map((note) => (
                <p key={note}>{note}</p>
              ))}
            </details>
          )}
          <small>
            Published {new Date(invite.setup?.published_at).toLocaleString()}. Refresh after
            changing addons or access settings.
          </small>
        </div>
      )}
      <details className="invite-options">
        <summary>Player download options</summary>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={includeBeta}
            disabled={busy}
            onChange={(event) => setBetaChoice(event.target.checked)}
          />
          Include beta client builds
        </label>
        <p>
          Allows matching beta downloads from Modrinth. Version, loader and file checks still apply.
        </p>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={includeOptional}
            disabled={busy}
            onChange={(event) => setOptionalChoice(event.target.checked)}
          />
          Include optional client features
        </label>
        <p>
          Off by default: friends download only required client mods and their dependencies. Server
          plugins stay installed.
        </p>
      </details>
      <div className="invite-actions">
        <button
          className={`button ${shareReady ? "secondary" : "primary"}`}
          disabled={!ready || busy || resource.loading}
          onClick={publish}
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
              disabled={busy || !ready || resource.loading || !!resource.error}
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
      {copied && <p role="status">Invite link copied. Send it to your friends.</p>}
      {!ready && <p className="field-help">Available when server installation finishes.</p>}
      {invite && (
        <details>
          <summary>Link access</summary>
          <p>
            Anyone with this link can see the connection address and published player setup until{" "}
            {new Date(invite.expires_at).toLocaleDateString()}. It does not grant server management
            or whitelist access.
          </p>
          <button className="button secondary" disabled={busy} onClick={revoke}>
            Disable invite link
          </button>
        </details>
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

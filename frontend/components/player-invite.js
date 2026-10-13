"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Icon } from "./ui";
import { request } from "../lib/api";

export function PlayerInvite({ server, ready }) {
  const { api } = useAuth();
  const permitted = server.permissions?.includes("members");
  const resource = useResource(permitted ? `/servers/${server.id}/player-invite` : null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [result, setResult] = useState(null);
  const [copied, setCopied] = useState(false);
  const invite = result || resource.data;
  const href = invite?.slug ? `/join/${invite.slug}` : null;
  async function publish() {
    setBusy(true);
    setError(null);
    setCopied(false);
    try {
      setResult(await api(`/servers/${server.id}/player-invite`, { method: "POST" }));
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
      setResult(null);
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
    <section className="panel invite-friends" aria-label="Invite your friends">
      <div className="section-heading">
        <div>
          <span className="eyebrow">PLAY TOGETHER</span>
          <h2>Invite your friends</h2>
          <p>One player page. No dashboard account needed.</p>
        </div>
        <Icon name="user" size={28} />
      </div>
      <ErrorMessage error={error || resource.error} />
      {invite && (
        <div className="invite-preview">
          <strong>
            {invite.setup?.pack_ready
              ? "Player download ready"
              : invite.setup?.pack_reason
                ? "Player setup needs a check"
                : "Player page ready"}
          </strong>
          <p>
            {invite.setup?.pack_reason ||
              (invite.setup?.pack_ready
                ? "Their launcher can download the verified client mods."
                : "Friends get the address, version and join steps.")}
          </p>
          {invite.setup?.notes?.map((note) => (
            <p key={note}>{note}</p>
          ))}
          <small>
            Published {new Date(invite.setup?.published_at).toLocaleString()}. Refresh after
            changing addons or access settings.
          </small>
        </div>
      )}
      <div className="invite-actions">
        <button className="button primary" disabled={!ready || busy} onClick={publish}>
          {busy
            ? "Preparing player setup…"
            : invite
              ? "Refresh player setup"
              : "Prepare invite link"}
        </button>
        {href && (
          <>
            <button className="button secondary" disabled={busy} onClick={copy}>
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
  const [setup, setSetup] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copied, setCopied] = useState(false);
  const [packDownloaded, setPackDownloaded] = useState(false);
  const [busy, setBusy] = useState(false);
  // A public player page deliberately never calls authenticated management APIs.
  useEffect(() => {
    const controller = new AbortController();
    request(`/join/${encodeURIComponent(slug)}`, { signal: controller.signal })
      .then((data) => {
        if (!controller.signal.aborted) setSetup(data);
      })
      .catch((error) => {
        if (!controller.signal.aborted) setError(error);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [slug]);
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
      setPackDownloaded(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="public-player-page">
      <header>
        <Link href="/login" className="public-brand">
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
          <h1>This invite is unavailable</h1>
          <p>Ask your host for a fresh invite link.</p>
        </section>
      ) : (
        <>
          <section className="player-hero">
            <span className="player-hero-art" aria-hidden="true">
              🎮
            </span>
            <span className="eyebrow">YOUR NEXT ADVENTURE</span>
            <h1>Join {setup.name}</h1>
            <p>Minecraft Java {setup.minecraft_version}</p>
            <div className="player-address">
              <code>{setup.address}</code>
              <button className="button secondary" onClick={copy}>
                {copied ? "Copied" : "Copy address"}
              </button>
            </div>
          </section>
          <section className="player-steps" aria-label="Join steps">
            {setup.pack_ready ? (
              <>
                <article className="panel player-step">
                  <span className="step-number">1</span>
                  <h2>
                    {setup.addons?.some((addon) => addon.required)
                      ? "Download your player setup"
                      : "Add optional player features"}
                  </h2>
                  <p>Includes the matching loader and verified client mod download instructions.</p>
                  <button className="button primary wide" disabled={busy} onClick={download}>
                    {busy ? "Preparing…" : "Download player pack"}
                  </button>
                  {packDownloaded && (
                    <p role="status">Downloaded. Import it in your launcher next.</p>
                  )}
                </article>
                <article className="panel player-step">
                  <span className="step-number">2</span>
                  <h2>Import and launch</h2>
                  <p>
                    Open the downloaded .mrpack in Modrinth App. It creates a profile and downloads
                    its listed mods. Then click Play.
                  </p>
                  <p>Modrinth App requires a Microsoft account that owns Minecraft Java Edition.</p>
                  <a
                    className="button secondary"
                    href="https://modrinth.com/app"
                    target="_blank"
                    rel="noreferrer"
                  >
                    Get Modrinth App ↗
                  </a>
                  <details>
                    <summary>Use another launcher</summary>
                    <p>
                      Prism Launcher: Add Instance → Import → select the .mrpack. Other launchers
                      need support for importing Modrinth packs. Follow your launcher’s account
                      requirements.
                    </p>
                    <a href="https://prismlauncher.org/download/" target="_blank" rel="noreferrer">
                      Get Prism Launcher ↗
                    </a>
                    <a
                      href="https://docs.skmedix.pl/faq/launcher-related"
                      target="_blank"
                      rel="noreferrer"
                    >
                      SKlauncher setup ↗
                    </a>
                  </details>
                </article>
              </>
            ) : (
              <article className="panel player-step">
                <span className="step-number">1</span>
                <h2>
                  {setup.pack_reason ? "Ask your host to finish player setup" : "Open Minecraft"}
                </h2>
                <p>{setup.pack_reason || `Launch Minecraft Java ${setup.minecraft_version}.`}</p>
                {setup.addons?.map((addon) => (
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
            )}
            <article className="panel player-step">
              <span className="step-number">{setup.pack_ready ? "3" : "2"}</span>
              <h2>Join and play</h2>
              <p>Multiplayer → Add Server. Paste the address above, then Join Server.</p>
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
          <details className="panel">
            <summary>What will be installed?</summary>
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
              Published by your host on {new Date(setup.published_at).toLocaleString()}. This pack
              covers verified client JARs, not arbitrary custom configurations or manually uploaded
              files. If the host changes the server, ask them to refresh this setup.
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

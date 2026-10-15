"use client";
import { nextServerAction } from "../lib/next-action";
import { softwareName } from "../lib/software";
import { ServerInsights } from "./server-insights";
import { SetupProgress } from "./setup-progress";
import { PlayerInvite } from "./player-invite";
import { JoinServer } from "./join-server";
import { FeatureLinks } from "./management";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { VersionSafety } from "./change-safety";
import { StartupCheck } from "./startup-check";
import { useEffect, useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Badge, ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { consoleURL, duration, formatBytes } from "../lib/api";
function ConfirmStop({ onCancel, onConfirm, busy }) {
  const dialog = useRef(null);
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  return (
    <dialog
      ref={dialog}
      className="confirm-dialog"
      onCancel={(e) => {
        e.preventDefault();
        onCancel();
      }}
      aria-labelledby="confirm-title"
    >
      <span className="danger-icon">
        <Icon name="stop" size={25} />
      </span>
      <h2 id="confirm-title">Force stop this server?</h2>
      <p>Unsaved progress may be lost. Use this only when a normal stop is not responding.</p>
      <div className="dialog-actions">
        <button className="button secondary" onClick={onCancel} disabled={busy} autoFocus>
          Keep playing
        </button>
        <button className="button danger" onClick={onConfirm} disabled={busy}>
          {busy ? "Sending…" : "Force stop"}
        </button>
      </div>
    </dialog>
  );
}
function ConfirmDelete({ server, onCancel, onConfirm, busy, error }) {
  const dialog = useRef(null);
  const [name, setName] = useState("");
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  return (
    <dialog
      ref={dialog}
      className="confirm-dialog"
      aria-labelledby="delete-title"
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <h2 id="delete-title">Delete this server?</h2>
      <p>
        This permanently removes its world, files and backups and ends its subscription. Download
        anything you want to keep first.
      </p>
      <ErrorMessage error={error} />
      <label className="field">
        Type <strong>{server.name}</strong> to confirm
        <input value={name} onChange={(event) => setName(event.target.value)} autoComplete="off" />
      </label>
      <div className="dialog-actions">
        <button className="button secondary" disabled={busy} onClick={onCancel}>
          Keep server
        </button>
        <button
          className="button danger"
          disabled={busy || name !== server.name}
          onClick={onConfirm}
        >
          {busy ? "Queuing deletion…" : "Delete server"}
        </button>
      </div>
    </dialog>
  );
}
function ResourceChart({ samples }) {
  const width = 600,
    height = 140;
  const coordinates = samples.map(
    (sample, i) =>
      `${samples.length === 1 ? width / 2 : (i / (samples.length - 1)) * width},${height - (Math.min(100, Math.max(0, sample)) / 100) * height}`,
  );
  return (
    <div className="chart-wrap">
      <div className="chart-labels">
        <span>100%</span>
        <span>50%</span>
        <span>0%</span>
      </div>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        role="img"
        aria-label="CPU allocation usage from recent live samples"
      >
        <defs>
          <linearGradient id="cpu-fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#4c9a5f" stopOpacity=".2" />
            <stop offset="100%" stopColor="#4c9a5f" stopOpacity=".01" />
          </linearGradient>
        </defs>
        {[0, height / 2, height].map((y) => (
          <line key={y} x1="0" y1={y} x2={width} y2={y} stroke="#e8ece8" strokeDasharray="3 4" />
        ))}
        {samples.length > 1 && (
          <>
            <path
              d={`M0,${height} L${coordinates.join(" L")} L${width},${height} Z`}
              fill="url(#cpu-fill)"
            />
            <polyline
              points={coordinates.join(" ")}
              fill="none"
              stroke="#4c9a5f"
              strokeWidth="2.5"
              vectorEffect="non-scaling-stroke"
            />
          </>
        )}
        {samples.length === 1 && (
          <circle
            cx={width / 2}
            cy={height - (Math.min(100, samples[0]) / 100) * height}
            r="3"
            fill="#4c9a5f"
          />
        )}
      </svg>
      {!samples.length && <span className="chart-empty">Waiting for live samples</span>}
    </div>
  );
}
function LiveResourceChart({ usage, cpuLimit }) {
  const [history, setHistory] = useState({ usage: null, samples: [] });
  // Record each distinct API snapshot once, without an extra effect-driven render.
  if (usage && cpuLimit > 0 && history.usage !== usage) {
    setHistory({
      usage,
      samples: [...history.samples.slice(-59), (usage.cpu_percent / cpuLimit) * 100],
    });
  }
  return <ResourceChart samples={history.samples} />;
}

export function ServerDetail({ id, advanced = false }) {
  const { api, refreshServers, session } = useAuth();
  const router = useRouter();
  const { data: server, error, loading } = useResource(`/servers/${id}`, 10000);
  const {
    data: usage,
    error: usageError,
    refresh: refreshUsage,
  } = useResource(
    server?.status === "RUNNING" && server.management_available && !server.deletion_pending
      ? `/servers/${id}/usage`
      : null,
    5000,
  );
  const [busy, setBusy] = useState("");
  const [pendingPower, setPendingPower] = useState("");
  const [actionError, setActionError] = useState(null);
  const [notice, setNotice] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteKey, setDeleteKey] = useState("");
  const [copied, setCopied] = useState(false);
  const ready =
    server?.status === "RUNNING" && server.management_available && !server.deletion_pending;
  const eula = useResource(ready ? `/servers/${id}/eula` : null, 10000);
  const [agreementChecked, setAgreementChecked] = useState(false);
  const canStart = ready && eula.data?.accepted === true && !eula.error;
  const permissions = server?.permissions || [];
  const has = (action) => permissions.includes(action);
  const transitioning = ["starting", "stopping"].includes(usage?.power_state);
  const powerPending =
    ((pendingPower === "stop" || pendingPower === "kill") && usage?.power_state !== "offline") ||
    (pendingPower === "start" && usage?.power_state !== "running");
  const displayPowerState =
    powerPending && (pendingPower === "stop" || pendingPower === "kill")
      ? "stopping"
      : powerPending && pendingPower === "start"
        ? "starting"
        : usage?.power_state;
  const startDisabled =
    !canStart ||
    !!busy ||
    powerPending ||
    transitioning ||
    !!usageError ||
    usage?.power_state !== "offline";
  const [overviewView, setOverviewView] = useState("play");
  const nextAction = server
    ? nextServerAction({
        server,
        ready,
        powerState: displayPowerState,
        eula: eula.data?.accepted,
        eulaError: eula.error,
        usageError,
      })
    : null;
  const address = server?.hostname && server.port ? `${server.hostname}:${server.port}` : null;
  async function acceptAgreement() {
    if (!agreementChecked || busy) return;
    setBusy("eula");
    setActionError(null);
    try {
      await api(`/servers/${id}/eula`, {
        method: "POST",
        body: { accepted: true },
        key: crypto.randomUUID(),
      });
      setAgreementChecked(false);
      setNotice("Agreement accepted. Your server is ready to start.");
      eula.refresh();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy("");
    }
  }
  async function power(action) {
    if (busy || (powerPending && !(action === "kill" && pendingPower === "stop"))) return;
    setBusy(action);
    setActionError(null);
    setNotice("");
    try {
      await api(`/servers/${id}/${action}`, {
        method: "POST",
        body: action === "kill" ? { confirmation: id } : {},
        key: crypto.randomUUID(),
      });
      setPendingPower(action === "restart" ? "" : action);
      setNotice(
        action === "stop"
          ? "Shutting down safely. This can take a minute while Minecraft saves the world."
          : action === "kill"
            ? "Force stop requested. Waiting for the server to go offline."
            : `${action[0].toUpperCase() + action.slice(1)} requested. Watching for a status update.`,
      );
      setConfirm(false);
      refreshUsage();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy("");
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(address);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setNotice("Copy the server address from the connection panel below.");
    }
  }
  async function removeServer() {
    if (busy) return;
    setBusy("delete");
    setActionError(null);
    try {
      await api(`/servers/${id}/terminate`, {
        method: "POST",
        body: { reason: "Owner requested deletion", confirmation: id },
        key: deleteKey,
      });
      refreshServers();
      router.replace("/servers");
    } catch (err) {
      setActionError(err);
      setBusy("");
    }
  }
  return (
    <Shell>
      {loading && !server ? (
        <Loading message="Opening your server…" />
      ) : error && !server ? (
        <>
          <ErrorMessage error={error} />
          <Link href="/servers" className="button secondary">
            Back to servers
          </Link>
        </>
      ) : (
        server && (
          <>
            <Link className="back-link" href="/servers">
              ← All servers
            </Link>
            <PageHeading
              eyebrow={`${softwareName(server.server_type).toUpperCase()} · MINECRAFT ${server.minecraft_version}`}
              title={server.name}
              description="A little less setup. A lot more Minecraft."
              action={
                <Badge
                  state={
                    server.deletion_pending
                      ? "DELETING"
                      : ready && eula.data?.accepted === false
                        ? "ACCEPTANCE_REQUIRED"
                        : displayPowerState || server.status
                  }
                />
              }
            />
            <div className="server-toolbar">
              <div className="mode-switch">
                <Link href={`/servers/${id}`} className={!advanced ? "selected" : ""}>
                  Simple
                </Link>
                <Link href={`/servers/${id}/console`} className={advanced ? "selected" : ""}>
                  <Icon name="terminal" size={14} />
                  Advanced
                </Link>
              </div>
              <div id="server-controls" className="power-controls">
                {has("start") && (
                  <button
                    className="button primary"
                    onClick={() => power("start")}
                    disabled={startDisabled}
                  >
                    <Icon name="play" size={15} />
                    {busy === "start" || (powerPending && pendingPower === "start")
                      ? "Starting…"
                      : "Start"}
                  </button>
                )}
                {has("stop") && (
                  <button
                    className="button secondary"
                    onClick={() => power("stop")}
                    disabled={
                      !ready ||
                      !!busy ||
                      powerPending ||
                      transitioning ||
                      usage?.power_state !== "running"
                    }
                  >
                    <Icon name="stop" size={15} />
                    {busy === "stop" || (powerPending && pendingPower === "stop")
                      ? "Shutting down…"
                      : "Stop"}
                  </button>
                )}
                {has("restart") && (
                  <button
                    className="button secondary"
                    onClick={() => power("restart")}
                    disabled={
                      !canStart ||
                      !!busy ||
                      powerPending ||
                      transitioning ||
                      usage?.power_state !== "running"
                    }
                  >
                    <Icon name="restart" size={16} />
                    {busy === "restart" ? "Restarting…" : "Restart"}
                  </button>
                )}
              </div>
            </div>
            {server.deletion_pending && (
              <p className="notice" role="status">
                This server is being deleted. Its controls are disabled while files and hosting
                resources are removed.
              </p>
            )}
            <ErrorMessage error={error || actionError} />
            {ready && <ErrorMessage error={eula.error} />}
            {ready && eula.loading && !eula.data && (
              <p className="notice" role="status">
                Checking your server’s Minecraft agreement…
              </p>
            )}
            {ready && eula.data?.accepted === false && (
              <section
                id="minecraft-agreement"
                className="notice lifecycle-notice eula-notice"
                aria-label="Minecraft agreement"
              >
                <strong>One step before your first start.</strong>
                {eula.data.can_accept ? (
                  <>
                    <p>
                      Minecraft requires the server owner to accept its agreement before the server
                      can run.
                    </p>
                    <label className="eula-checkbox">
                      <input
                        type="checkbox"
                        checked={agreementChecked}
                        onChange={(event) => setAgreementChecked(event.target.checked)}
                        disabled={!!busy}
                      />
                      <span>
                        I have read and accept the{" "}
                        <a
                          href="https://www.minecraft.net/en-us/eula"
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          Minecraft EULA
                        </a>
                        .
                      </span>
                    </label>
                    <button
                      className="button primary"
                      disabled={!agreementChecked || !!busy || usage?.power_state !== "offline"}
                      onClick={acceptAgreement}
                    >
                      {busy === "eula" ? "Saving…" : "Accept agreement"}
                    </button>
                  </>
                ) : (
                  <p>
                    Ask the server owner to accept the Minecraft EULA. You can start the server once
                    they have accepted.
                  </p>
                )}
              </section>
            )}
            {notice && (
              <p className="notice success" role="status">
                {notice}
              </p>
            )}
            {server.status !== "RUNNING" && (
              <SetupProgress server={server} isAdmin={session?.user.is_admin} />
            )}
            {server.status === "RUNNING" && !server.management_available && (
              <p className="notice">
                Live controls are temporarily unavailable. Please contact hosting support.
              </p>
            )}
            <FeatureLinks id={id} server={server} advanced={advanced} />
            {!advanced && (
              <nav className="overview-views" aria-label="Overview views">
                {[
                  ["play", "Play"],
                  ["joining", "Join"],
                  ["health", "Health"],
                  ["maintenance", "Manage"],
                ]
                  .filter(([view]) => view !== "maintenance" || has("delete"))
                  .map(([view, label]) => (
                    <button
                      key={view}
                      type="button"
                      className={`button ${overviewView === view ? "primary" : "secondary"}`}
                      aria-pressed={overviewView === view}
                      onClick={() => setOverviewView(view)}
                    >
                      {label}
                    </button>
                  ))}
              </nav>
            )}
            {!advanced && overviewView === "play" && (
              <section className="server-play-card" aria-label="Play on your server">
                <h2>{nextAction.title}</h2>
                <div className="next-action-buttons">
                  {nextAction.kind === "start" && (
                    <button
                      className="button primary"
                      disabled={startDisabled}
                      onClick={() => power("start")}
                    >
                      Start your world
                    </button>
                  )}
                  {nextAction.kind === "invite" && (
                    <Link className="button primary" href="#invite-friends">
                      Invite friends
                    </Link>
                  )}
                  {nextAction.kind === "agreement" && (
                    <Link className="button primary" href="#minecraft-agreement">
                      Review Minecraft agreement
                    </Link>
                  )}
                  {nextAction.kind === "billing" && (
                    <Link className="button primary" href="/billing">
                      Continue in Billing
                    </Link>
                  )}
                  {nextAction.kind === "worlds" && (
                    <Link className="button secondary" href="/servers">
                      Back to your worlds
                    </Link>
                  )}
                  {nextAction.kind === "support" && (
                    <Link className="button secondary" href={`/feedback?server=${id}`}>
                      Get help
                    </Link>
                  )}
                </div>
                <div className="player-address">
                  <code>{address || "Address available after setup"}</code>
                  <button
                    className={`button ${nextAction.kind === "join" ? "primary" : "secondary"}`}
                    onClick={copy}
                    disabled={!address}
                  >
                    {copied ? "Copied" : "Copy server address"}
                  </button>
                </div>
              </section>
            )}
            {!advanced && overviewView === "play" && (
              <PlayerInvite key={`invite:${id}`} server={server} ready={ready} />
            )}
            {ready && !notice && ["starting", "stopping"].includes(displayPowerState) && (
              <p className="notice" role="status">
                {displayPowerState === "starting"
                  ? "Starting Minecraft. The world and plugins are loading; this page updates automatically."
                  : "Saving your world and shutting down. Wait for Offline before starting again."}
              </p>
            )}
            {advanced ? (
              <>
                <div className="section-heading">
                  <div>
                    <h2>Minecraft console</h2>
                    <span>Live output and server commands</span>
                  </div>
                  {has("kill") && (
                    <button
                      className="button danger-outline"
                      disabled={
                        !ready ||
                        !!busy ||
                        (powerPending && pendingPower !== "stop") ||
                        usage?.power_state === "offline"
                      }
                      onClick={() => setConfirm(true)}
                    >
                      Force stop
                    </button>
                  )}
                </div>
                {has("console") && server.console_available && ready ? (
                  <LiveConsole id={id} role={server.role} />
                ) : (
                  <div className="panel unavailable-panel">
                    <Icon name="terminal" size={28} />
                    <h3>Console unavailable</h3>
                    <p>
                      {!ready
                        ? "Your server must be ready before you can connect."
                        : "The live console connection has not been enabled. Please contact hosting support."}
                    </p>
                  </div>
                )}
              </>
            ) : (
              <>
                <ErrorMessage error={usageError} />
                <section
                  className="panel server-secondary"
                  data-section="joining"
                  hidden={overviewView !== "joining"}
                >
                  {!advanced && !server.deletion_pending && server.status !== "CANCELLED" && (
                    <JoinServer
                      key={server.id}
                      server={server}
                      ready={ready}
                      powerState={displayPowerState}
                      address={address}
                      onCopy={copy}
                    />
                  )}
                </section>
                <section
                  className="panel server-secondary"
                  data-section="health"
                  hidden={overviewView !== "health"}
                >
                  {overviewView === "health" && (
                    <>
                      {!server.deletion_pending && server.status !== "CANCELLED" && (
                        <StartupCheck
                          key={`startup:${id}`}
                          server={server}
                          ready={ready}
                          powerState={displayPowerState}
                          usage={usage}
                          usageError={usageError}
                          eula={eula.data?.accepted}
                        />
                      )}
                      <ServerInsights key={`insights:${id}`} server={server} ready={ready} />
                      <div className="metric-grid">
                        <Metric
                          icon="user"
                          title="Players online"
                          value={usage?.players_online ?? "—"}
                          detail={
                            usage?.players_max != null
                              ? `of ${usage.players_max} slots`
                              : usage?.players_available
                                ? "Ready for friends"
                                : "No player count available"
                          }
                        />
                        <Metric
                          icon="cpu"
                          title="CPU usage"
                          value={usage ? `${usage.cpu_percent.toFixed(1)}%` : "—"}
                          detail={`${server.cpu_limit}% allocated`}
                        />
                        <Metric
                          icon="server"
                          title="Memory usage"
                          value={formatBytes(usage?.memory_bytes)}
                          detail={`of ${formatBytes(server.ram_mb * 1048576, 0)} allocated`}
                        />
                        <Metric
                          icon="clock"
                          title="Uptime"
                          value={usage ? duration(usage.uptime_ms) : "—"}
                          detail={
                            usage?.power_state === "running"
                              ? "Since the last start"
                              : "Start your next adventure"
                          }
                        />
                      </div>
                      <div className="overview-grid server-performance-grid">
                        <section className="panel performance-panel">
                          <div className="section-heading">
                            <div>
                              <h2>Server performance</h2>
                              <span>CPU allocation usage</span>
                            </div>
                            <span className="live-label">
                              <span />
                              Recent live samples
                            </span>
                          </div>
                          <LiveResourceChart key={id} usage={usage} cpuLimit={server.cpu_limit} />
                          <div className="chart-caption">
                            <span>Older</span>
                            <span>Now</span>
                          </div>
                          <div className="disk-meter">
                            <div>
                              <span>Disk usage</span>
                              <strong>
                                {formatBytes(usage?.disk_bytes)}{" "}
                                <small>/ {formatBytes(server.disk_mb * 1048576, 0)}</small>
                              </strong>
                            </div>
                            <progress
                              max={server.disk_mb * 1048576}
                              value={usage?.disk_bytes || 0}
                              aria-label="Disk usage"
                            />
                          </div>
                        </section>
                      </div>
                      <section className="panel details-panel">
                        <div>
                          <h2>At a glance</h2>
                          <Link href="/guide">Setup and installation guide →</Link>
                          <p>The essentials for your world.</p>
                        </div>
                        <dl>
                          <div>
                            <dt>Server software</dt>
                            <dd>{softwareName(server.server_type)}</dd>
                          </div>
                          <div>
                            <dt>Minecraft version</dt>
                            <dd>{server.minecraft_version}</dd>
                          </div>
                          {server.loader_version && (
                            <div>
                              <dt>Loader version</dt>
                              <dd>{server.loader_version}</dd>
                            </div>
                          )}
                          <div>
                            <dt>Your access</dt>
                            <dd>
                              {server.role === "OWNER"
                                ? "Owner"
                                : server.role === "ADMIN"
                                  ? "Admin"
                                  : "Moderator"}
                            </dd>
                          </div>
                          <div>
                            <dt>Backup allowance</dt>
                            <dd>{server.backup_limit}</dd>
                          </div>
                        </dl>
                      </section>
                    </>
                  )}
                </section>
                {has("delete") && server.status !== "CANCELLED" && !server.deletion_pending && (
                  <section
                    className="panel server-secondary"
                    data-section="maintenance"
                    hidden={overviewView !== "maintenance"}
                  >
                    <section className="panel details-panel">
                      <VersionSafety server={server} offline={usage?.power_state === "offline"} />
                      <div>
                        <h2>Delete server</h2>
                        <p>Remove this server and its files after you have saved what you need.</p>
                        <button
                          className="button danger-outline"
                          onClick={() => {
                            setDeleteKey(crypto.randomUUID());
                            setActionError(null);
                            setConfirmDelete(true);
                          }}
                        >
                          Delete server
                        </button>
                      </div>
                    </section>
                  </section>
                )}
              </>
            )}
            {confirm && (
              <ConfirmStop
                onCancel={() => setConfirm(false)}
                onConfirm={() => power("kill")}
                busy={!!busy}
              />
            )}
            {confirmDelete && (
              <ConfirmDelete
                server={server}
                busy={busy === "delete"}
                error={actionError}
                onCancel={() => {
                  setConfirmDelete(false);
                  setActionError(null);
                }}
                onConfirm={removeServer}
              />
            )}
          </>
        )
      )}
    </Shell>
  );
}
function Metric({ icon, title, value, detail }) {
  return (
    <div className="metric-card">
      <div>
        <span>{title}</span>
        <Icon name={icon} size={17} />
      </div>
      <strong>{value}</strong>
      <small>{detail}</small>
    </div>
  );
}
function LiveConsole({ id, role }) {
  const { api } = useAuth();
  const [lines, setLines] = useState([]);
  const [connectionState, setState] = useState("connecting");
  const [error, setError] = useState(null);
  const [command, setCommand] = useState("");
  const [sending, setSending] = useState(false);
  const [notice, setNotice] = useState("");
  const [paused, setPaused] = useState(false);
  const state = paused ? "paused" : connectionState;
  const [generation, setGeneration] = useState(0);
  const [follow, setFollow] = useState(true);
  const log = useRef(null);
  useEffect(() => {
    if (follow && log.current) log.current.scrollTop = log.current.scrollHeight;
  }, [lines, follow]);
  useEffect(() => {
    if (paused) return;
    let alive = true,
      socket,
      timer,
      attempts = 0;
    const controller = new AbortController();
    async function connect() {
      if (!alive) return;
      setState(attempts ? "reconnecting" : "connecting");
      setError(null);
      try {
        const ticket = await api(`/servers/${id}/console-ticket`, {
          method: "POST",
          signal: controller.signal,
        });
        if (!alive) return;
        socket = new WebSocket(consoleURL(id), ["hosting.console.v1", `ticket.${ticket.ticket}`]);
        socket.onmessage = (event) => {
          if (!alive) return;
          let message;
          try {
            message = JSON.parse(event.data);
          } catch {
            return;
          }
          if (message.event === "connected") {
            attempts = 0;
            setState("connected");
            setLines([]);
          }
          if (message.event === "console output" && typeof message.args?.[0] === "string") {
            // Strip the intentional ANSI escape character from Minecraft output.
            // eslint-disable-next-line no-control-regex
            const line = message.args[0].replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "");
            setLines((old) => [...old.slice(-499), line]);
          }
        };
        socket.onerror = () => {
          if (alive) setError("Could not connect to the Minecraft console.");
        };
        socket.onclose = () => {
          if (!alive) return;
          setState("disconnected");
          if (attempts >= 8) {
            setError("Connection interrupted. Select reconnect to try again.");
            return;
          }
          attempts++;
          timer = setTimeout(connect, Math.min(1000 * 2 ** attempts, 30000));
        };
      } catch (err) {
        if (!alive || err.name === "AbortError") return;
        setError(err);
        setState("unavailable");
      }
    }
    connect();
    return () => {
      alive = false;
      controller.abort();
      clearTimeout(timer);
      socket?.close();
    };
  }, [id, api, paused, generation]);
  async function send(event) {
    event.preventDefault();
    if (!command.trim() || sending) return;
    setSending(true);
    setError(null);
    setNotice("");
    try {
      await api(`/servers/${id}/commands`, {
        method: "POST",
        body: { command: command.trim() },
        key: crypto.randomUUID(),
      });
      setCommand("");
      setNotice("Command accepted.");
    } catch (err) {
      setError(err);
    } finally {
      setSending(false);
    }
  }
  return (
    <>
      <ErrorMessage error={error} />
      <div className="console-panel">
        <div className="console-toolbar">
          <span className={`console-connection ${state}`}>
            <span />
            {
              {
                connected: "Connected",
                connecting: "Connecting",
                reconnecting: "Reconnecting",
                disconnected: "Disconnected",
                paused: "Paused",
                unavailable: "Unavailable",
              }[state]
            }
          </span>
          <div>
            <label className="follow-toggle">
              <input
                type="checkbox"
                checked={follow}
                onChange={(e) => setFollow(e.target.checked)}
              />
              Follow output
            </label>
            <button onClick={() => setLines([])}>Clear</button>
            <button
              onClick={() => {
                setPaused(false);
                setGeneration((v) => v + 1);
              }}
            >
              Reconnect
            </button>
            <button onClick={() => setPaused((v) => !v)}>{paused ? "Resume" : "Pause"}</button>
          </div>
        </div>
        <div
          className="console-log"
          ref={log}
          role="log"
          aria-label="Minecraft console output"
          aria-live="off"
        >
          {lines.length ? (
            lines.map((line, i) => (
              <div className="console-line" key={i}>
                <span>{String(i + 1).padStart(3, "0")}</span>
                <code>{line}</code>
              </div>
            ))
          ) : (
            <div className="console-empty">
              <Icon name="terminal" size={24} />
              <span>
                {state === "connected"
                  ? "Connected. Minecraft output will appear here."
                  : "Connecting to your Minecraft console…"}
              </span>
            </div>
          )}
        </div>
        <form className="console-input" onSubmit={send}>
          <span aria-hidden="true">❯</span>
          <input
            aria-label="Minecraft console command"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder={
              role === "MODERATOR" ? "Enter list, say, or help…" : "Enter a Minecraft command…"
            }
            maxLength={1024}
            autoComplete="off"
            spellCheck="false"
            disabled={state !== "connected"}
          />
          <button
            className="button primary"
            disabled={state !== "connected" || sending || !command.trim()}
          >
            {sending ? "Sending…" : "Send"}
            <Icon name="arrow" size={14} />
          </button>
        </form>
      </div>
      <div className="console-caption">
        <span>
          {role === "MODERATOR"
            ? "Moderator access: list, say and help commands."
            : "Commands go to your Minecraft server."}{" "}
          Console output is kept only while this page is open.
        </span>
        <span role="status">{notice}</span>
      </div>
    </>
  );
}

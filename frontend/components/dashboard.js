"use client";
import { Notifications } from "./server-insights";
import { Select } from "./select";
import { BetaGettingStarted } from "./first-server";
import { softwareName, supportsMods, supportsPlugins } from "../lib/software";
import Link from "next/link";
import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Badge, Empty, ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes, price } from "../lib/api";
export function ServerCard({ server }) {
  const { data: usage, error } = useResource(
    server.status === "RUNNING" && !server.deletion_pending ? `/servers/${server.id}/usage` : null,
    10000,
  );
  const address =
    server.hostname && server.port
      ? `${server.hostname}:${server.port}`
      : "Address assigned on activation";
  const Card = server.deletion_pending ? "article" : Link;
  return (
    <Card
      href={server.deletion_pending ? undefined : `/servers/${server.id}`}
      className="server-card"
    >
      <div className={`server-art art-${server.server_type.toLowerCase()}`}>
        <span className="art-grid" />
        <span className="pixel-tree tree-one" />
        <span className="pixel-tree tree-two" />
        <span className="pixel-terrain" />
        <div className="art-overlay">
          <span className="software-tag">{softwareName(server.server_type)}</span>
          <Badge
            state={
              server.deletion_pending
                ? "DELETING"
                : error
                  ? "UNKNOWN"
                  : usage?.power_state || server.status
            }
          />
        </div>
      </div>
      <div className="server-card-body">
        <div className="server-title">
          <h3>{server.name}</h3>
          <Icon name="arrow" size={18} />
        </div>
        <p className="server-version">Minecraft {server.minecraft_version}</p>
        <div className="card-resource-row">
          <span>
            <Icon name="cpu" size={14} />
            {formatBytes(server.ram_mb * 1048576, 0)} RAM
          </span>
          <span>
            <Icon name="server" size={14} />
            {formatBytes(server.disk_mb * 1048576, 0)} disk
          </span>
        </div>
        <div className="server-card-footer">
          <span className="address-truncated">{address}</span>
          <span>
            {server.deletion_pending
              ? "Deleting…"
              : error
                ? "Status unavailable"
                : usage?.players_available && usage?.players_online != null
                  ? `${usage.players_online} playing`
                  : "Manage"}{" "}
            <span aria-hidden="true">↗</span>
          </span>
        </div>
      </div>
    </Card>
  );
}
export function Dashboard({ serversOnly = false }) {
  const { session } = useAuth();
  const { data: servers, loading, error, refresh } = useResource("/servers", 10000);
  const { data: plans, error: plansError } = useResource("/plans");
  const { data: registration } = useResource("/auth/config");
  const { data: billing } = useResource("/billing/config");
  const beta = registration?.invite_required && billing?.provider === "disabled";
  const [query, setQuery] = useState("");
  const [view, setView] = useState("active");
  const visible = (servers || [])
    .filter(
      (server) => view === "all" || (server.status !== "CANCELLED" && !server.deletion_pending),
    )
    .filter((server) =>
      `${server.name} ${server.server_type}`.toLowerCase().includes(query.toLowerCase()),
    )
    .sort((a, b) => Number(b.status === "RUNNING") - Number(a.status === "RUNNING"));
  const current =
    servers?.find((server) => server.status === "RUNNING" && !server.deletion_pending) ||
    servers?.find((server) => !["CANCELLED"].includes(server.status) && !server.deletion_pending);
  const currentDetails = useResource(
    !serversOnly && current ? `/servers/${current.id}` : null,
    10000,
  );
  const activeWorld = currentDetails.data;
  const shortcuts =
    !currentDetails.error &&
    activeWorld?.status === "RUNNING" &&
    activeWorld.management_available &&
    !activeWorld.deletion_pending
      ? [
          [
            "settings",
            "Game rules",
            "Set the way you play",
            "world",
            `/servers/${current.id}/settings`,
          ],
          [
            "members",
            "Invite friends",
            "Prepare their player page",
            "user",
            `/servers/${current.id}#invite-friends`,
          ],
          [
            supportsMods(activeWorld.server_type) ? "files" : "plugins",
            "Add extras",
            "Browse matching mods or plugins",
            "plus",
            `/servers/${current.id}/${supportsMods(activeWorld.server_type) ? "mods" : "plugins"}`,
          ],
          [
            "backups",
            "Backups",
            "Protect what you build",
            "shield",
            `/servers/${current.id}/backups`,
          ],
        ].filter(
          ([permission, label]) =>
            activeWorld.permissions?.includes(permission) &&
            (label !== "Add extras" ||
              supportsMods(activeWorld.server_type) ||
              supportsPlugins(activeWorld.server_type)),
        )
      : [];
  const ready = servers?.filter((s) => s.status === "RUNNING" && !s.deletion_pending).length || 0;
  const deleting = servers?.filter((s) => s.deletion_pending).length || 0;
  return (
    <Shell>
      <PageHeading
        eyebrow={serversOnly ? "YOUR WORLDS" : "YOUR WORLDS"}
        title={
          serversOnly
            ? "Your servers"
            : `Welcome back, ${session?.user.name.split(" ")[0] || "player"}.`
        }
        description={
          serversOnly
            ? "Everything you need to keep your worlds running."
            : "Your worlds, your friends, your next adventure."
        }
        action={
          beta && !session?.user.is_admin ? (
            <Link className="button secondary" href="/guide#start-and-join">
              Setup guide
            </Link>
          ) : (
            <Link className="button primary" href="/servers/new">
              <Icon name="plus" size={17} />
              New server
            </Link>
          )
        }
      />
      {!serversOnly && beta && servers && (
        <details className="dashboard-hosting-details" open={!servers.length}>
          <summary>Private beta help</summary>
          <BetaGettingStarted servers={servers} />
        </details>
      )}
      {!serversOnly && (
        <>
          <section className="dashboard-play-hero" aria-label="Your next adventure">
            <span className="landing-kicker">LESS SETUP. MORE MINECRAFT.</span>
            <h2>Make tonight an adventure.</h2>
            <p>
              Your worlds and friends are one click away. Start playing, or make room for something
              new.
            </p>
            {current ? (
              <Link href={`/servers/${current.id}`} className="button">
                Open my world <Icon name="arrow" size={16} />
              </Link>
            ) : !beta || session?.user.is_admin ? (
              <Link href="/servers/new" className="button">
                Create your next world <Icon name="arrow" size={16} />
              </Link>
            ) : (
              <Link href="/guide#start-and-join" className="button">
                Find your first world <Icon name="arrow" size={16} />
              </Link>
            )}
          </section>
          {shortcuts.length > 0 && (
            <nav className="world-shortcuts" aria-label="Quick world actions">
              {shortcuts.map(([, label, help, icon, href]) => (
                <Link key={label} href={href}>
                  <span className="shortcut-icon">
                    <Icon name={icon} size={22} />
                  </span>
                  <div>
                    <strong>{label}</strong>
                    <small>{help}</small>
                  </div>
                  <Icon name="arrow" size={16} />
                </Link>
              ))}
            </nav>
          )}
          <Notifications />
          <details className="dashboard-hosting-details">
            <summary>Hosting details</summary>
            <div className="summary-grid">
              <div className="summary-card">
                <span className="summary-icon">
                  <Icon name="server" />
                </span>
                <div>
                  <span>Total servers</span>
                  <strong>
                    {servers
                      ? servers.filter((s) => s.status !== "CANCELLED" && !s.deletion_pending)
                          .length
                      : "—"}
                    <small>in your workspace</small>
                  </strong>
                </div>
              </div>
              <div className="summary-card">
                <span className="summary-icon green">
                  <Icon name="play" />
                </span>
                <div>
                  <span>Active hosting</span>
                  <strong>
                    {servers ? ready : "—"}
                    <small>activated servers</small>
                  </strong>
                </div>
              </div>
              <div className="summary-card">
                <span className="summary-icon amber">
                  <Icon name="cpu" />
                </span>
                <div>
                  <span>Allocated memory</span>
                  <strong>
                    {servers
                      ? formatBytes(
                          servers
                            .filter((s) => !["CANCELLED", "FAILED"].includes(s.status))
                            .reduce((sum, s) => sum + s.ram_mb * 1048576, 0),
                          0,
                        )
                      : "—"}
                    <small>across your worlds</small>
                  </strong>
                </div>
              </div>
            </div>
          </details>
        </>
      )}
      <div className="section-heading">
        <div>
          <h2>{serversOnly ? "All servers" : "Your servers"}</h2>
          <span>
            {visible.length} shown
            {view === "active" ? " · current hosting" : " · including cancelled"}
          </span>
        </div>
        <div className="server-list-tools">
          <Select aria-label="Server view" value={view} onChange={(e) => setView(e.target.value)}>
            <option value="active">Active servers</option>
            <option value="all">All servers</option>
          </Select>
          <label className="search-box">
            <span aria-hidden="true">⌕</span>
            <input
              aria-label="Search servers"
              placeholder="Search your servers…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
        </div>
      </div>
      {!!deleting && (
        <p role="status">
          {deleting === 1
            ? "Your server is being deleted."
            : `${deleting} servers are being deleted.`}{" "}
          It has been removed from active servers. This list updates automatically.
        </p>
      )}
      <ErrorMessage error={error} />
      {error && (
        <button className="button secondary" onClick={refresh}>
          Try again
        </button>
      )}
      {loading ? (
        <Loading message="Finding your worlds…" />
      ) : (
        servers &&
        (visible.length ? (
          <div className="server-grid">
            {visible.map((server) => (
              <ServerCard key={server.id} server={server} />
            ))}
          </div>
        ) : (
          <Empty
            title={query ? "No matching servers" : "Your next adventure starts here"}
            action={
              !query &&
              !beta && (
                <Link href="/servers/new" className="button primary">
                  <Icon name="plus" size={17} />
                  Create a server
                </Link>
              )
            }
          >
            {query
              ? "Try another name or server type."
              : beta
                ? "Ask your host for access to a beta server. It will appear here when available."
                : "Choose a plan and a Minecraft version to request your first world."}
          </Empty>
        ))
      )}
      {!serversOnly && !beta && (
        <section className="plans-section">
          <div className="section-heading">
            <div>
              <h2>Find your perfect fit</h2>
              <span>Plans for every kind of adventure</span>
            </div>
            <Link href="/servers/new" className="text-link">
              Explore plans <span>↗</span>
            </Link>
          </div>
          <ErrorMessage error={plansError} />
          <div className="plan-strip">
            {plans?.map((plan) => (
              <Link href={`/servers/new?plan=${plan.id}`} key={plan.id} className="plan-mini">
                <span className="plan-icon">
                  <Icon name="world" />
                </span>
                <div>
                  <strong>{plan.name}</strong>
                  <small>
                    {formatBytes(plan.ram_mb * 1048576, 0)} RAM · about {plan.player_recommendation}{" "}
                    players
                  </small>
                </div>
                <span className="plan-price">
                  {price(plan)}
                  <small>/{plan.billing_period === "YEARLY" ? "year" : "month"}</small>
                </span>
                <Icon name="arrow" size={15} />
              </Link>
            ))}
          </div>
        </section>
      )}
    </Shell>
  );
}

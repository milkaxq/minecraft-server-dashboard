"use client";
import { Notifications } from "./server-insights";
import { Select } from "./select";
import { softwareName, supportsMods, supportsPlugins } from "../lib/software";
import Link from "next/link";
import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Badge, Empty, ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes } from "../lib/api";
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
                : usage?.power_state === "running" &&
                    usage.players_available &&
                    usage.players_online != null
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
      `${server.name} ${server.server_type}`.toLowerCase().includes(query.trim().toLowerCase()),
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
          ["settings", "Game rules", "world", `/servers/${current.id}/settings`],
          ["members", "Invite friends", "user", `/servers/${current.id}#invite-friends`],
          [
            supportsMods(activeWorld.server_type) ? "files" : "plugins",
            "Add extras",
            "plus",
            `/servers/${current.id}/${supportsMods(activeWorld.server_type) ? "mods" : "plugins"}`,
          ],
          ["backups", "Backups", "shield", `/servers/${current.id}/backups`],
        ].filter(
          ([permission, label]) =>
            activeWorld.permissions?.includes(permission) &&
            (label !== "Add extras" ||
              supportsMods(activeWorld.server_type) ||
              supportsPlugins(activeWorld.server_type)),
        )
      : [];
  const deleting = servers?.filter((s) => s.deletion_pending).length || 0;
  const showTools =
    serversOnly ||
    query !== "" ||
    view !== "active" ||
    (servers?.length || 0) > 1 ||
    servers?.some((server) => server.status === "CANCELLED" || server.deletion_pending);
  return (
    <Shell>
      <PageHeading
        eyebrow="YOUR WORLDS"
        title={
          serversOnly
            ? "Your servers"
            : `Welcome back, ${session?.user.name.split(" ")[0] || "player"}.`
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
      {!serversOnly && (
        <>
          {current && (
            <section className="dashboard-resume" aria-label="Your next adventure">
              <div>
                <h2>Continue playing</h2>
                <p>{current.name}</p>
              </div>
              <Link href={`/servers/${current.id}`} className="button primary">
                Open my world <Icon name="arrow" size={16} />
              </Link>
            </section>
          )}
          {shortcuts.length > 0 && (
            <nav className="world-shortcuts" aria-label="Quick world actions">
              {shortcuts.map(([, label, icon, href]) => (
                <Link key={label} href={href}>
                  <span className="shortcut-icon">
                    <Icon name={icon} size={22} />
                  </span>
                  <div>
                    <strong>{label}</strong>
                  </div>
                  <Icon name="arrow" size={16} />
                </Link>
              ))}
            </nav>
          )}
          <Notifications />
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
        {showTools && (
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
        )}
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
            title={
              query
                ? "No matching servers"
                : beta
                  ? "Welcome to the private beta"
                  : "Your next adventure starts here"
            }
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
                ? "Your host provides server access; payments are disabled for this beta."
                : "Choose how to play and create your first world."}
          </Empty>
        ))
      )}
    </Shell>
  );
}

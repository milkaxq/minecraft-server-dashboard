"use client";
import { BetaGettingStarted } from "./first-server";
import { softwareName } from "../lib/software";
import Link from "next/link";
import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Badge, Empty, ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes, price } from "../lib/api";
export function ServerCard({ server }) {
  const { data: usage, error } = useResource(
    server.status === "RUNNING" ? `/servers/${server.id}/usage` : null,
    10000,
  );
  const address =
    server.hostname && server.port
      ? `${server.hostname}:${server.port}`
      : "Address assigned on activation";
  return (
    <Link href={`/servers/${server.id}`} className="server-card">
      <div className={`server-art art-${server.server_type.toLowerCase()}`}>
        <span className="art-grid" />
        <span className="pixel-tree tree-one" />
        <span className="pixel-tree tree-two" />
        <span className="pixel-terrain" />
        <div className="art-overlay">
          <span className="software-tag">{softwareName(server.server_type)}</span>
          <Badge state={usage?.power_state || server.status} />
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
            {usage?.players_online != null
              ? `${usage.players_online} playing`
              : error
                ? "Status unavailable"
                : "Manage"}{" "}
            <span aria-hidden="true">↗</span>
          </span>
        </div>
      </div>
    </Link>
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
    .filter((server) => view === "all" || server.status !== "CANCELLED")
    .filter((server) =>
      `${server.name} ${server.server_type}`.toLowerCase().includes(query.toLowerCase()),
    )
    .sort((a, b) => Number(b.status === "RUNNING") - Number(a.status === "RUNNING"));
  const ready = servers?.filter((s) => s.status === "RUNNING").length || 0;
  return (
    <Shell>
      <PageHeading
        eyebrow={serversOnly ? "YOUR WORLDS" : "YOUR WORKSPACE"}
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
      {!serversOnly && beta && servers && <BetaGettingStarted servers={servers} />}
      {!serversOnly && (
        <>
          {servers?.length === 0 && !beta && (
            <section className="welcome-banner">
              <div>
                <span className="banner-kicker">MAKE SOMETHING TOGETHER</span>
                <h2>
                  A little space.
                  <br />
                  Endless possibilities.
                </h2>
                <p>
                  Build a base, explore the unknown, or just hang out.
                  <br />
                  We’ll help you keep your world ready.
                </p>
                <Link href="/servers/new" className="button banner-button">
                  Create your next world <Icon name="arrow" size={16} />
                </Link>
              </div>
              <div className="hero-blocks" aria-hidden="true">
                <span className="block block-a" />
                <span className="block block-b" />
                <span className="block block-c" />
                <span className="block block-d" />
                <span className="hero-star">✦</span>
              </div>
            </section>
          )}
          <div className="summary-grid">
            <div className="summary-card">
              <span className="summary-icon">
                <Icon name="server" />
              </span>
              <div>
                <span>Total servers</span>
                <strong>
                  {servers ? servers.length : "—"}
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
          <select aria-label="Server view" value={view} onChange={(e) => setView(e.target.value)}>
            <option value="active">Active servers</option>
            <option value="all">All servers</option>
          </select>
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

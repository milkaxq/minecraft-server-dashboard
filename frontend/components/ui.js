"use client";
import Link from "next/link";
import { ConnectionStatus } from "./connection-status";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef } from "react";
import { useAuth } from "./auth";
import { STATE_LABELS } from "../lib/api";
const icons = {
  folder: (
    <path d="M20 20H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2Z" />
  ),
  file: (
    <>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z" />
      <path d="M14 2v6h6M8 13h8M8 17h6" />
    </>
  ),
  menu: <path d="M4 6h16M4 12h16M4 18h16" />,
  close: <path d="m6 6 12 12M18 6 6 18" />,
  dashboard: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
    </>
  ),
  server: (
    <>
      <rect x="3" y="3" width="18" height="7" rx="2" />
      <rect x="3" y="14" width="18" height="7" rx="2" />
      <path d="M7 6.5h.01M7 17.5h.01M15 6.5h3M15 17.5h3" />
    </>
  ),
  wallet: (
    <>
      <path d="M20 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h15V8H5a3 3 0 0 1 0-6" />
      <path d="M20 12h-5v5h5M17 14.5h.01" />
    </>
  ),
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21v-3a8 8 0 0 1 16 0v3" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  arrow: <path d="m9 5 7 7-7 7" />,
  logout: (
    <>
      <path d="M9 4H4v16h5M10 12h11m-4-4 4 4-4 4" />
    </>
  ),
  play: <path d="m7 4 14 8-14 8Z" />,
  stop: <rect x="5" y="5" width="14" height="14" rx="2" />,
  restart: (
    <>
      <path d="M20 6v5h-5M20 11a8 8 0 1 0-2 7" />
    </>
  ),
  terminal: (
    <>
      <path d="m4 6 6 6-6 6M13 18h7" />
    </>
  ),
  copy: (
    <>
      <rect x="8" y="8" width="13" height="13" rx="2" />
      <path d="M16 8V3H3v13h5" />
    </>
  ),
  check: <path d="m4 12 5 5L20 6" />,
  cpu: (
    <>
      <rect x="6" y="6" width="12" height="12" rx="2" />
      <path d="M9 1v5M15 1v5M9 18v5M15 18v5M1 9h5M1 15h5M18 9h5M18 15h5" />
    </>
  ),
  world: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a17 17 0 0 1 0 18 17 17 0 0 1 0-18" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  shield: (
    <>
      <path d="m12 2 9 4v6c0 5-9 10-9 10S3 17 3 12V6Z" />
      <path d="m8 12 3 3 5-6" />
    </>
  ),
};
export function Icon({ name, size = 20 }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {icons[name] || icons.server}
    </svg>
  );
}
export function Brand({ href = "/dashboard" }) {
  return (
    <Link href={href} className="brand">
      <span className="brand-mark">
        <span />
        <span />
        <span />
      </span>
      blockyard<span className="brand-dot">.</span>
    </Link>
  );
}
export function Shell({ children }) {
  const { session, logout, signingOut, restoring, restoreError, restoreSession, authNotice } =
    useAuth();
  const router = useRouter();
  const path = usePathname();
  const menu = useRef(null);
  useEffect(() => {
    const wide = window.matchMedia("(min-width: 901px)");
    const closeOnResize = () => {
      if (wide.matches) menu.current?.close();
    };
    wide.addEventListener("change", closeOnResize);
    return () => wide.removeEventListener("change", closeOnResize);
  }, []);
  useEffect(() => {
    if (!session && !restoring && !restoreError) router.replace("/login");
  }, [session, router, restoring, restoreError]);
  if (!session && restoreError)
    return (
      <div className="loading-full">
        <div>
          <ErrorMessage error={restoreError} />
          <button className="button primary" onClick={restoreSession} disabled={restoring}>
            Retry connection
          </button>
        </div>
      </div>
    );
  if (!session) return <div className="loading-full">Opening your dashboard…</div>;
  const nav = [
    ["dashboard", "/dashboard", "Dashboard"],
    ["server", "/servers", "Servers"],
    ["wallet", "/billing", "Billing"],
    ["user", "/account", "Account"],
    ["world", "/guide", "Guide"],
    ["user", "/feedback", "Feedback"],
    ...(session.user.is_admin ? [["shield", "/admin", "Admin"]] : []),
  ];
  const navigation = (
    <>
      {nav.map(([icon, href, label]) => (
        <Link
          onClick={() => menu.current?.close()}
          key={href}
          href={href}
          className={`nav-item ${path === href || path.startsWith(`${href}/`) ? "active" : ""}`}
          aria-current={path === href || path.startsWith(`${href}/`) ? "page" : undefined}
        >
          <Icon name={icon} />
          {label}
          {label === "Servers" && (
            <span className="nav-shortcut" aria-hidden="true">
              ↗
            </span>
          )}
        </Link>
      ))}
    </>
  );
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <header className="mobile-header">
        <Brand />
        <button
          className="mobile-menu-button"
          onClick={() => menu.current?.showModal()}
          aria-label="Open navigation"
          aria-haspopup="dialog"
          aria-controls="mobile-navigation"
        >
          <Icon name="menu" /> <span>Menu</span>
        </button>
      </header>
      <dialog
        ref={menu}
        id="mobile-navigation"
        className="mobile-navigation"
        aria-labelledby="mobile-menu-title"
      >
        <div className="mobile-menu-heading">
          <h2 id="mobile-menu-title">Your worlds</h2>
          <button
            className="mobile-menu-button"
            onClick={() => menu.current?.close()}
            aria-label="Close navigation"
            autoFocus
          >
            <Icon name="close" />
          </button>
        </div>
        <p className="mobile-menu-user">{session.user.name}</p>
        <nav aria-label="Main navigation">{navigation}</nav>
        <button className="button mobile-signout" disabled={signingOut} onClick={logout}>
          <Icon name="logout" size={18} />
          {signingOut ? "Signing out…" : "Sign out"}
        </button>
        {authNotice && (
          <p className="notice" role="status">
            {authNotice}
          </p>
        )}
      </dialog>
      <aside className="sidebar">
        <Brand />
        <div className="workspace">
          <span className="workspace-avatar">{session.user.name.slice(0, 1).toUpperCase()}</span>
          <div>
            <strong>My worlds</strong>
            <small>Personal hosting</small>
          </div>
          <span className="workspace-dot" />
        </div>
        <div className="nav-label">YOUR WORLDS</div>
        <nav aria-label="Main navigation">{navigation}</nav>
        <div className="sidebar-bottom">
          <button className="profile-button" disabled={signingOut} onClick={logout}>
            <span className="profile-avatar">{session.user.name.slice(0, 1).toUpperCase()}</span>
            <span>
              <strong>{session.user.name}</strong>
              <small>{signingOut ? "Signing out…" : "Sign out"}</small>
            </span>
            <Icon name="logout" size={17} />
          </button>
        </div>
      </aside>
      <div className="main-wrap">
        <header className="topbar">
          <span>
            Minecraft hosting <span className="breadcrumb-slash">/</span>{" "}
            <strong>
              {path.includes("/console")
                ? "Console"
                : path.startsWith("/servers/") && path !== "/servers/new"
                  ? "Your server"
                  : nav.find(([, href]) => path === href)?.[2] || "New server"}
            </strong>
          </span>
          <span className="topbar-tag">
            <span />
            Your worlds
          </span>
        </header>
        <main id="main-content" tabIndex={-1} className="main-content">
          <ConnectionStatus />
          {authNotice && (
            <p className="notice" role="alert">
              {authNotice}
            </p>
          )}
          {children}
        </main>
        <footer className="footer">
          Built for your next adventure.<span>Blockyard · Minecraft hosting</span>
        </footer>
      </div>
    </div>
  );
}
export function Badge({ state }) {
  return (
    <span className={`badge badge-${state?.toLowerCase()}`}>
      <span />
      {STATE_LABELS[state] || state || "Unknown"}
    </span>
  );
}
export function ErrorMessage({ error }) {
  const path = usePathname();
  const { session } = useAuth();
  const params = new URLSearchParams();
  const server = path?.match(/^\/servers\/([0-9a-f-]{36})(?:\/|$)/i)?.[1];
  if (server) params.set("server", server);
  if (path) params.set("page", path);
  if (typeof error?.requestId === "string" && /^[0-9a-f-]{36}$/i.test(error.requestId))
    params.set("reference", error.requestId);
  if (!error) return null;
  return (
    <div role="alert" className="error-message">
      <span>{typeof error === "string" ? error : error.message}</span>
      {error.requestId && <small>Reference: {error.requestId}</small>}
      {session && (
        <Link className="text-link error-report-link" href={`/feedback?${params.toString()}`}>
          Report this problem →
        </Link>
      )}
    </div>
  );
}
export function Loading({ message = "Loading your workspace…" }) {
  return (
    <div className="loading-state">
      <span className="spinner" />
      {message}
    </div>
  );
}
export function Empty({ title, children, action }) {
  return (
    <div className="empty-state">
      <span className="empty-icon">
        <Icon name="world" size={30} />
      </span>
      <h3>{title}</h3>
      <p>{children}</p>
      {action}
    </div>
  );
}
export function PageHeading({ eyebrow, title, description, action }) {
  return (
    <div className="page-heading">
      <div>
        {eyebrow && <span className="eyebrow">{eyebrow}</span>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {action}
    </div>
  );
}

// Preserve table semantics while giving narrow screens labeled, readable cards.
export function Table({ children, className = "", ...props }) {
  const ref = useRef(null);
  useEffect(() => {
    const table = ref.current;
    const headings = [...table.querySelectorAll("thead th")].map((cell) => cell.textContent.trim());
    for (const group of table.querySelectorAll("thead, tbody"))
      group.setAttribute("role", "rowgroup");
    for (const row of table.querySelectorAll("tr")) row.setAttribute("role", "row");
    for (const cell of table.querySelectorAll("thead th"))
      cell.setAttribute("role", "columnheader");
    for (const row of table.querySelectorAll("tbody tr")) {
      [...row.children].forEach((cell, index) => {
        cell.setAttribute("role", "cell");
        cell.dataset.label = cell.colSpan > 1 ? "" : headings[index] || "";
      });
    }
  }, [children]);
  return (
    <table {...props} ref={ref} role="table" className={`responsive-table ${className}`}>
      {children}
    </table>
  );
}

"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";
import { Brand, Icon } from "./ui";
import { request, price, formatBytes } from "../lib/api";

export function Landing({ appOrigin = "" }) {
  const appPath = (path) => (path.startsWith("#") ? path : `${appOrigin}${path}`);
  const { session } = useAuth();
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      request("/auth/config", {
        signal: controller.signal,
        baseURL: appOrigin ? "/api" : undefined,
        credentials: appOrigin ? "omit" : undefined,
      }),
      request("/plans", {
        signal: controller.signal,
        baseURL: appOrigin ? "/api" : undefined,
        credentials: appOrigin ? "omit" : undefined,
      }),
    ])
      .then(([registration, plans]) => {
        if (typeof registration?.invite_required !== "boolean" || !Array.isArray(plans))
          throw new Error("Invalid hosting choices");
        if (!controller.signal.aborted) {
          setData({ registration, plans });
          setError(null);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(e);
      });
    return () => controller.abort();
  }, [reload, appOrigin]);
  const beta = data?.registration.invite_required;
  const href = session ? "/dashboard" : !data || error ? "#how-it-works" : "/register";
  const cta = session
    ? "Open my worlds"
    : !data || error
      ? "See how it works"
      : beta
        ? "Join with an invitation"
        : "Create your world";
  return (
    <div className="landing">
      <a className="skip-link" href="#landing-main">
        Skip to content
      </a>
      <header className="landing-nav">
        <Brand href="/" />
        <nav aria-label="Website navigation">
          <a href="#how-it-works">How it works</a>
          <a href="#hosting-plans">Plans</a>
          <a href="#landing-help">Questions?</a>
        </nav>
        <Link
          className="button landing-signin"
          href={appPath(appOrigin || session ? "/dashboard" : "/login")}
        >
          {appOrigin ? "Dashboard" : session ? "My worlds" : "Sign in"}
          <Icon name="arrow" size={16} />
        </Link>
      </header>
      <main id="landing-main">
        <section className="landing-hero" aria-labelledby="landing-title">
          <div className="landing-hero-content">
            <span className="landing-kicker">
              <span />{" "}
              {beta ? "PRIVATE BETA · BUILT FOR PLAYERS" : "MINECRAFT JAVA · BUILT FOR PLAYERS"}
            </span>
            <h1 id="landing-title">
              Your world.
              <br />
              Your friends.
              <br />
              <em>Just play.</em>
            </h1>
            <p>
              A place to build, explore and hang out together. Choose your adventure. We handle the
              server setup.
            </p>
            <div className="landing-actions">
              <Link className="button landing-primary" href={appPath(href)}>
                {cta}
                <Icon name="arrow" size={19} />
              </Link>
              <a className="landing-text-link" href="#how-it-works">
                Take a look around <span aria-hidden="true">↓</span>
              </a>
            </div>
            <small>
              {beta
                ? "Invitation-only for now. Public checkout is not open during this beta."
                : "Simple controls. Optional extras. Your own Minecraft world."}
            </small>
          </div>
          <div className="landing-scene-caption" aria-hidden="true">
            <span>✦</span> A little space for big adventures.
          </div>
        </section>
        <div className="landing-benefits">
          <span>
            <Icon name="play" /> Start with a click
          </span>
          <span>
            <Icon name="world" /> Make it your world
          </span>
          <span>
            <Icon name="user" /> Bring your friends
          </span>
        </div>
        <section id="how-it-works" className="landing-section">
          <div className="landing-section-heading">
            <span className="landing-kicker">LESS SETUP. MORE MINECRAFT.</span>
            <h2>
              From an idea to
              <br />
              <em>“meet you in-game.”</em>
            </h2>
            <p>You don’t need to know how hosting works to enjoy your own world.</p>
          </div>
          <div className="landing-step-grid">
            {[
              [
                "01",
                "Choose your adventure",
                "Keep it simple with normal gameplay and server plugins, or choose Fabric or Forge for compatible mods.",
                "world",
              ],
              [
                "02",
                "Make yourself at home",
                "Name your server, choose a plan, then use friendly controls for game rules, worlds and backups.",
                "dashboard",
              ],
              [
                "03",
                "Bring your people",
                "Start Minecraft and share your player invitation. Friends get the address and any verified client downloads they need.",
                "user",
              ],
            ].map(([number, title, text, icon]) => (
              <article key={number}>
                <div className="landing-step-top">
                  <Icon name={icon} size={28} />
                  <span>{number}</span>
                </div>
                <h3>{title}</h3>
                <p>{text}</p>
              </article>
            ))}
          </div>
        </section>
        <section className="landing-section landing-features">
          <div className="landing-section-heading">
            <span className="landing-kicker">YOUR WORLD. YOUR WAY.</span>
            <h2>
              Made for players.
              <br />
              <em>Ready for game night.</em>
            </h2>
            <p>
              The everyday things are easy to find. The technical tools are there when you need
              them.
            </p>
            <Link className="button landing-primary" href={appPath(href)}>
              {cta}
              <Icon name="arrow" size={18} />
            </Link>
          </div>
          <div className="landing-feature-list">
            {[
              ["Start, stop, play", "Clear controls and live status, without a terminal.", "play"],
              [
                "Add something new",
                "Browse matching plugins or mods and review dependencies before installing.",
                "plus",
              ],
              [
                "Save what you build",
                "Create backups and recover a world with a clear confirmation.",
                "shield",
              ],
              [
                "Help friends get in",
                "Share a player page with version, address and launcher steps.",
                "user",
              ],
            ].map(([title, text, icon]) => (
              <article key={title}>
                <Icon name={icon} />
                <div>
                  <h3>{title}</h3>
                  <p>{text}</p>
                </div>
              </article>
            ))}
          </div>
        </section>
        <section id="hosting-plans" className="landing-section">
          <div className="landing-section-heading">
            <span className="landing-kicker">ROOM FOR YOUR NEXT WORLD</span>
            <h2>
              Start small.
              <br />
              <em>Dream big.</em>
            </h2>
            <p>
              {beta
                ? "Preview our hosting plans. Your host provides server access during the private beta; these are not paid checkout offers."
                : "Choose space for your friends and the extras you want to add."}
            </p>
          </div>
          {error ? (
            <div className="landing-load-error" role="alert">
              <p>Hosting plans could not be loaded. Sign-in is still available.</p>
              <button
                className="button landing-signin"
                onClick={() => setReload((value) => value + 1)}
              >
                Retry plans
              </button>
            </div>
          ) : !data ? (
            <p role="status">Loading hosting plans…</p>
          ) : data.plans.length ? (
            <div className="landing-plan-grid">
              {data.plans.map((plan, index) => (
                <article
                  key={plan.id}
                  className={index === 1 ? "landing-plan featured" : "landing-plan"}
                >
                  <span className="landing-plan-icon">
                    <Icon name="world" size={26} />
                  </span>
                  <h3>{plan.name}</h3>
                  <p>{plan.description}</p>
                  <div className="landing-plan-price">
                    {price(plan)}
                    <small>/{plan.billing_period === "YEARLY" ? "year" : "month"}</small>
                  </div>
                  <ul>
                    <li>About {plan.player_recommendation} players</li>
                    <li>{formatBytes(plan.ram_mb * 1048576, 0)} memory</li>
                    <li>{formatBytes(plan.disk_mb * 1048576, 0)} world & file storage</li>
                  </ul>
                  <Link
                    className="button landing-signin"
                    href={appPath(
                      session
                        ? beta && !session.user.is_admin
                          ? "/dashboard"
                          : `/servers/new?plan=${encodeURIComponent(plan.id)}`
                        : "/register",
                    )}
                  >
                    {beta
                      ? session
                        ? session.user.is_admin
                          ? "Create beta world"
                          : "Open my worlds"
                        : "Invitation needed"
                      : "Choose plan"}
                    <Icon name="arrow" size={16} />
                  </Link>
                  <small>Player counts are a guide; mods and worlds affect usage.</small>
                </article>
              ))}
            </div>
          ) : (
            <p>Plans are being prepared. You can still sign in to your existing worlds.</p>
          )}
        </section>
        <section id="landing-help" className="landing-section landing-faq">
          <div className="landing-section-heading">
            <span className="landing-kicker">GOOD QUESTIONS</span>
            <h2>
              New to all this?
              <br />
              <em>You’re in the right place.</em>
            </h2>
          </div>
          <div>
            {[
              [
                "Do I need technical experience?",
                "No terminal is needed for everyday hosting. Start and stop your world, manage friends and change common game rules from the dashboard. Advanced files and console tools are available separately.",
              ],
              [
                "Can friends use normal Minecraft?",
                "Many Paper plugins work with a normal Java client. Some features, such as voice chat, need a client mod. Fabric and Forge mods may need matching player installations; the invitation page explains the current requirements.",
              ],
              [
                "Will mods download when someone joins?",
                "Minecraft does not automatically install executable mods on connection. Your player invitation can provide a verified pack to import into a supported launcher before joining. Complete third-party modpacks may need the author’s setup steps.",
              ],
              [
                "How do I join the private beta?",
                "If the beta is invitation-only, your host sends an email-bound invitation. Use it to register and verify your account. Public paid checkout is not available during that beta.",
              ],
            ].map(([question, answer]) => (
              <details key={question}>
                <summary>
                  {question}
                  <Icon name="plus" size={18} />
                </summary>
                <p>{answer}</p>
              </details>
            ))}
          </div>
        </section>
        <section className="landing-last-call">
          <span className="landing-kicker">MAKE SOMETHING TOGETHER</span>
          <h2>
            Your next adventure
            <br />
            needs a place to call home.
          </h2>
          <Link className="button landing-primary" href={appPath(href)}>
            {cta}
            <Icon name="arrow" size={18} />
          </Link>
        </section>
      </main>
      <footer className="landing-footer">
        <Brand href="/" />
        <p>Less setup. More Minecraft.</p>
        <Link href={appPath("/login")}>Sign in</Link>
        <small>
          Independent hosting for Minecraft Java Edition. Not affiliated with Mojang or Microsoft.
        </small>
      </footer>
    </div>
  );
}

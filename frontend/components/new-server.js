"use client";
import { PLAYER_GROUPS, recommendGroupPlan } from "../lib/group-plans";
import { Select } from "./select";
import { NEW_SERVER_TYPES, softwareName, supportsMods } from "../lib/software";
import { useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes, price } from "../lib/api";
export function NewServer({ initialPlan = "", initialGroup = "friends" }) {
  const orderRequest = useRef(null);
  const {
    data: checkoutConfig,
    error: checkoutError,
    refresh: refreshCheckout,
  } = useResource("/billing/config");
  const { api, session } = useAuth();
  const router = useRouter();
  const { data: plans, error: plansError, loading, refresh: refreshPlans } = useResource("/plans");
  const {
    data: availableOptions,
    error: optionsError,
    refresh: refreshOptions,
  } = useResource("/minecraft/options");
  const options = availableOptions?.filter((option) =>
    NEW_SERVER_TYPES.includes(option.server_type),
  );
  const [group, setGroup] = useState(
    PLAYER_GROUPS.some((value) => value.id === initialGroup) ? initialGroup : "friends",
  );
  const [chosenPlan, setPlanID] = useState(initialPlan);
  const [chosenType, setType] = useState("PAPER");
  const [chosenVersion, setVersion] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const audience = PLAYER_GROUPS.find((value) => value.id === group);
  const recommended = recommendGroupPlan(plans, audience.players);
  const planID = plans?.find((p) => p.id === chosenPlan)?.id || recommended?.id || "";
  const type = options?.some((o) => o.server_type === chosenType)
    ? chosenType
    : options?.[0]?.server_type || "PAPER";
  const versions =
    options
      ?.filter((o) => o.server_type === type)
      .map((o) => o.minecraft_version)
      .sort((a, b) => a.localeCompare(b, undefined, { numeric: true })) || [];
  const version = versions.includes(chosenVersion) ? chosenVersion : versions.at(-1) || "";
  const selected = plans?.find((p) => p.id === planID);
  const beta = checkoutConfig?.provider === "disabled" && session?.user.is_admin;
  const software = [...new Set(options?.map((o) => o.server_type) || [])];
  async function submit(event) {
    event.preventDefault();
    if (
      busy ||
      !selected ||
      !version ||
      !name.trim() ||
      plansError ||
      optionsError ||
      checkoutError ||
      (!beta && !checkoutConfig?.checkout_enabled)
    )
      return;
    setBusy(true);
    setError(null);
    try {
      const body = {
        plan_id: planID,
        name: name.trim(),
        minecraft_version: version,
        server_type: type,
      };
      const fingerprint = JSON.stringify(body);
      if (orderRequest.current?.fingerprint !== fingerprint)
        orderRequest.current = { fingerprint, key: crypto.randomUUID() };
      if (beta) {
        const server = await api("/admin/beta-servers", {
          method: "POST",
          body,
          key: orderRequest.current.key,
        });
        router.push(`/servers/${server.id}`);
        return;
      }
      const order = await api("/orders", {
        method: "POST",
        body,
        key: orderRequest.current.key,
      });
      router.push(`/checkout/${order.id}`);
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }
  return (
    <Shell>
      <PageHeading
        eyebrow="A NEW ADVENTURE"
        title="Make room for your next world."
        description="Pick how you want to play. We handle the server setup."
      />
      <ErrorMessage error={plansError || optionsError || checkoutError || error} />
      {error && (
        <p className="notice" role="status">
          Your choices are still here. Check the error and try again. An unchanged request uses the
          same reference to avoid creating another order.
        </p>
      )}
      {(plansError || optionsError || checkoutError) && (
        <button
          className="button secondary"
          onClick={() => {
            refreshPlans();
            refreshOptions();
            refreshCheckout();
          }}
        >
          Reload setup choices
        </button>
      )}
      <nav className="creation-steps" aria-label="Server creation steps">
        <a href="#choose-play-style">
          <span>1</span> Choose how to play
        </a>
        <a href="#choose-server-plan">
          <span>2</span> Choose a plan
        </a>
        <a href="#name-server">
          <span>3</span> Name & review
        </a>
      </nav>
      {loading ? (
        <Loading message="Loading hosting plans…" />
      ) : (
        <form onSubmit={submit} className="create-layout">
          <div>
            <section
              id="choose-play-style"
              className="play-style-picker"
              aria-label="How do you want to play?"
            >
              <div className="step-heading">
                <span>01</span>
                <div>
                  <h2>How do you want to play?</h2>
                  <p>You can add extras after setup.</p>
                </div>
              </div>
              <div className="play-style-grid">
                {[
                  [
                    "PAPER",
                    "🎮",
                    "Play with friends",
                    "Start with normal Java gameplay. Add server plugins like homes or permissions later; voice chat also needs a client mod.",
                  ],
                  [
                    "FABRIC",
                    "🧩",
                    "Add new blocks & creatures",
                    "Fabric mods change the game. We help prepare your friends’ player setup.",
                  ],
                  [
                    "FORGE",
                    "📦",
                    "Use a Forge modpack",
                    "Choose this if your pack asks for Forge. Pack installation and settings still need the author’s requirements.",
                  ],
                ].map(([value, icon, title, text]) => (
                  <button
                    type="button"
                    key={value}
                    className={`play-style-card ${type === value ? "selected" : ""}`}
                    aria-pressed={type === value}
                    disabled={busy || !software.includes(value)}
                    onClick={() => setType(value)}
                  >
                    <span className="play-style-art" aria-hidden="true">
                      {icon}
                    </span>
                    <strong>{title}</strong>
                    <small>{text}</small>
                    <span className="play-style-label">
                      {software.includes(value)
                        ? type === value
                          ? "Selected"
                          : "Choose"
                        : "Not available yet"}
                    </span>
                  </button>
                ))}
              </div>
            </section>
            <div id="choose-server-plan" className="step-heading">
              <span>02</span>
              <div>
                <h2>Who’s joining your world?</h2>
                <p>
                  Start with your group size. We suggest an affordable plan that fits its player
                  recommendation.
                </p>
              </div>
            </div>
            <div className="group-picker" aria-label="Player group size">
              {PLAYER_GROUPS.map((value) => {
                const match = recommendGroupPlan(plans, value.players);
                return (
                  <button
                    type="button"
                    key={value.id}
                    className={`group-card ${group === value.id ? "selected" : ""}`}
                    aria-pressed={group === value.id}
                    disabled={busy}
                    onClick={() => {
                      setGroup(value.id);
                      setPlanID("");
                    }}
                  >
                    <span aria-hidden="true">{value.icon}</span>
                    <strong>{value.title}</strong>
                    <span>For up to {value.players} players</span>
                    <small>{value.description}</small>
                    <b>
                      {match
                        ? `${price(match)}/${match.billing_period === "YEARLY" ? "year" : "month"}`
                        : "Ask us for a suitable plan"}
                    </b>
                  </button>
                );
              })}
            </div>
            {!recommended && (
              <p className="notice">
                No enabled plan currently recommends this group size. Choose a smaller group or ask
                your host for a suitable plan.
              </p>
            )}
            <p className="field-help group-plan-note">
              Group sizes are recommendations, not player limits or performance guarantees. Mods and
              large worlds may need more memory. When one plan fits two group sizes, you pay the
              same price.
            </p>
            <details className="panel group-plan-options" open={!!chosenPlan}>
              <summary>
                {selected
                  ? `Your plan: ${selected.name} · ${price(selected)}`
                  : "Compare hosting plans"}
              </summary>
              <p>
                Keep the suggested plan or choose another. Match a modpack’s own memory requirements
                before buying.
              </p>
              <div className="plan-picker">
                {plans?.map((plan) => (
                  <label
                    key={plan.id}
                    className={`plan-option ${planID === plan.id ? "selected" : ""}`}
                  >
                    <input
                      type="radio"
                      name="plan"
                      value={plan.id}
                      checked={planID === plan.id}
                      disabled={busy}
                      onChange={() => setPlanID(plan.id)}
                    />
                    <div className="plan-option-head">
                      <strong>{plan.name}</strong>
                      <span>
                        {price(plan)}
                        <small>/{plan.billing_period === "YEARLY" ? "year" : "month"}</small>
                      </span>
                    </div>
                    <p>{plan.description}</p>
                    <div className="plan-specs">
                      <span>{formatBytes(plan.ram_mb * 1048576, 0)} RAM</span>
                      <span>{formatBytes(plan.disk_mb * 1048576, 0)} disk</span>
                    </div>
                    <details className="plan-technical-details">
                      <summary>Technical limits</summary>
                      <p>
                        About {plan.player_recommendation} players · {plan.cpu_limit}% CPU ·{" "}
                        {plan.backup_limit ?? 0} backups · {plan.database_limit ?? 0} databases.
                        Player counts are recommendations, not guarantees; mods and worlds affect
                        usage.
                      </p>
                    </details>
                  </label>
                ))}
              </div>
            </details>
            <section id="name-server" className="panel create-settings">
              <div className="step-heading">
                <span>03</span>
                <div>
                  <h2>Name your world</h2>
                  <p>We’ll take care of the setup.</p>
                </div>
              </div>
              <label className="field">
                Server name
                <input
                  value={name}
                  disabled={busy}
                  onChange={(e) => setName(e.target.value)}
                  maxLength={80}
                  minLength={1}
                  required
                  placeholder="The Friends’ Survival World"
                  autoComplete="off"
                />
              </label>
              <details className="creation-advanced">
                <summary>Version & advanced choices</summary>
                <div className="field-row">
                  <label className="field">
                    Server software
                    <Select
                      aria-label="Server software"
                      value={type}
                      onChange={(e) => setType(e.target.value)}
                      disabled={busy || !software.length}
                    >
                      {software.map((v) => (
                        <option key={v} value={v}>
                          {softwareName(v)}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <label className="field">
                    Minecraft version
                    <Select
                      aria-label="Minecraft version"
                      value={version}
                      onChange={(e) => setVersion(e.target.value)}
                      disabled={busy || !versions.length}
                    >
                      {versions.map((v) => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </Select>
                  </label>
                </div>
              </details>
              {options && !options.length && (
                <p className="notice">
                  No Minecraft versions are available yet. Please contact hosting support.
                </p>
              )}
              <div className="create-next-actions" aria-label="What happens next">
                <span>✓ Automatic server setup</span>
                <span>✓ Optional extras</span>
                <span>✓ Invite your friends</span>
              </div>
              {supportsMods(type) && (
                <p className="field-help">
                  Players may need matching mods. Your invite page will explain which verified
                  downloads are available.
                </p>
              )}
              <p className="field-help">
                Microsoft verification and whitelist start off. Change who can join in Players or
                Settings. A login plugin is optional; it is not installed automatically.
              </p>
            </section>
          </div>
          <aside className="panel order-summary">
            <span className="eyebrow">YOUR NEW SERVER</span>
            <h2>{name.trim() || "Your next world"}</h2>
            <p>
              {softwareName(type)} {version && `· Minecraft ${version}`}
            </p>
            {selected && (
              <>
                <hr />
                <div className="summary-line">
                  <span>{selected.name} plan</span>
                  <strong>{price(selected)}</strong>
                </div>
                <div className="summary-line">
                  <span>Billing period</span>
                  <span>{selected.billing_period === "YEARLY" ? "Yearly" : "Monthly"}</span>
                </div>
                <div className="summary-line">
                  <span>Memory</span>
                  <span>{formatBytes(selected.ram_mb * 1048576, 0)}</span>
                </div>
                <div className="summary-line">
                  <span>Disk</span>
                  <span>{formatBytes(selected.disk_mb * 1048576, 0)}</span>
                </div>
                <hr />
              </>
            )}
            <p className="activation-note">
              {beta
                ? "Private beta setup. No checkout or charge; your server will install automatically."
                : checkoutConfig?.provider === "mock"
                  ? "Development demo checkout. No real money is charged."
                  : "Checkout is currently unavailable. Please contact hosting support."}
            </p>
            <button
              className="button primary wide"
              disabled={
                busy ||
                !selected ||
                !!plansError ||
                !!optionsError ||
                !!checkoutError ||
                !version ||
                !name.trim() ||
                (!beta && !checkoutConfig?.checkout_enabled)
              }
            >
              {busy ? "Creating server…" : beta ? "Create beta server" : "Continue to checkout"}
              <Icon name="arrow" size={16} />
            </button>
          </aside>
        </form>
      )}
    </Shell>
  );
}

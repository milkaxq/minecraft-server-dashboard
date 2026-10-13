"use client";
import { Select } from "./select";
import { NEW_SERVER_TYPES, softwareName, supportsMods } from "../lib/software";
import { useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes, price } from "../lib/api";
export function NewServer({ initialPlan = "" }) {
  const orderRequest = useRef(null);
  const { data: checkoutConfig, error: checkoutError } = useResource("/billing/config");
  const { api, session } = useAuth();
  const router = useRouter();
  const { data: plans, error: plansError, loading } = useResource("/plans");
  const { data: availableOptions, error: optionsError } = useResource("/minecraft/options");
  const options = availableOptions?.filter((option) =>
    NEW_SERVER_TYPES.includes(option.server_type),
  );
  const [chosenPlan, setPlanID] = useState(initialPlan);
  const [chosenType, setType] = useState("PAPER");
  const [chosenVersion, setVersion] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const planID = plans?.find((p) => p.id === chosenPlan)?.id || plans?.[0]?.id || "";
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
    if (busy) return;
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
      {loading ? (
        <Loading message="Loading hosting plans…" />
      ) : (
        <form onSubmit={submit} className="create-layout">
          <div>
            <section className="play-style-picker" aria-label="How do you want to play?">
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
                    "Normal Minecraft. Add extras like homes, permissions and voice chat later.",
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
                    disabled={!software.includes(value)}
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
            <div className="step-heading">
              <span>02</span>
              <div>
                <h2>Choose your plan</h2>
                <p>Choose space for your friends and extras.</p>
              </div>
            </div>
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
                    <span>{plan.cpu_limit}% CPU</span>
                    <span>About {plan.player_recommendation} players</span>
                  </div>
                </label>
              ))}
            </div>
            <section className="panel create-settings">
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
                      disabled={!software.length}
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
                      disabled={!versions.length}
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

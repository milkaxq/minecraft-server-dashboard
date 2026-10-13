"use client";
import { Select } from "./select";
import { NEW_SERVER_TYPES, SOFTWARE, softwareName, supportsMods } from "../lib/software";
import Link from "next/link";
import { ContextHelp } from "./context-help";
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
  const loader = options?.find(
    (o) => o.server_type === type && o.minecraft_version === version,
  )?.loader_version;
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
        description="Choose Minecraft, pick a plan, and give your server a name."
      />
      <ErrorMessage error={plansError || optionsError || checkoutError || error} />
      {loading ? (
        <Loading message="Loading hosting plans…" />
      ) : (
        <form onSubmit={submit} className="create-layout">
          <div>
            <div className="step-heading">
              <span>01</span>
              <div>
                <h2>Choose your plan</h2>
                <p>Resources reserved just for your server.</p>
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
                <span>02</span>
                <div>
                  <h2>Make it yours</h2>
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
                        {`${softwareName(v)} — ${SOFTWARE[v]?.category === "mods" ? "supports mods" : SOFTWARE[v]?.category === "plugins" ? "supports plugins" : "classic Minecraft"}`}
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
              {options && !options.length && (
                <p className="notice">
                  No Minecraft versions are available yet. Please contact hosting support.
                </p>
              )}
              <p className="field-help">
                {SOFTWARE[type]?.description} {loader && `Loader version: ${loader}.`}{" "}
                {supportsMods(type) &&
                  "Players usually need matching mods and the same loader. Upload server mods on the Mods page after stopping the server."}
              </p>
              <div className="creation-guidance">
                <h3>Which software should I choose?</h3>
                <p>
                  <strong>Vanilla:</strong> original Minecraft. <strong>Paper:</strong> survival
                  with optional server plugins. <strong>Fabric, Forge and NeoForge:</strong> mods
                  and modpacks; choose the loader required by your pack.
                </p>
                <p>
                  Only versions supported by the selected software are listed. Players should use
                  the same Minecraft version. Plugins and mods are not installed automatically.
                </p>
                <h3>Who can join?</h3>
                <p>
                  New servers require Microsoft accounts that own Java Edition. No separate server
                  password is needed. To support offline accounts, install and configure a
                  compatible authentication plugin before disabling account verification in
                  Settings. Offline support is configured after installation.
                </p>
                <p>
                  After setup: review the EULA → start → add allowed players if the whitelist is
                  enabled → copy the address and join.
                </p>
              </div>
              <Link href="/guide" className="button secondary">
                Help me choose · Setup guide
              </Link>
              <ContextHelp topic="software" />
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

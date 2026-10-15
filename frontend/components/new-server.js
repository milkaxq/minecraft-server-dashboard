"use client";
import { PLAYER_GROUPS, recommendGroupPlan } from "../lib/group-plans";
import { Select } from "./select";
import { NEW_SERVER_TYPES, softwareName } from "../lib/software";
import { useState, useRef, useEffect } from "react";
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
  const [step, setStep] = useState(1);
  const [style, setStyle] = useState("regular");
  const heading = useRef(null);
  useEffect(() => {
    heading.current?.focus();
  }, [step]);
  const [chosenPlan, setPlanID] = useState(initialPlan);
  const [chosenType, setType] = useState("PAPER");
  const [chosenVersion, setVersion] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const audience = PLAYER_GROUPS.find((value) => value.id === group);
  const enabledPlans = plans?.filter((plan) => plan.enabled !== false);
  const recommended = recommendGroupPlan(enabledPlans, audience.players);
  const planID = enabledPlans?.find((p) => p.id === chosenPlan)?.id || recommended?.id || "";
  const type = options?.some((o) => o.server_type === chosenType)
    ? chosenType
    : options?.[0]?.server_type || "PAPER";
  const versions =
    options
      ?.filter((o) => o.server_type === type)
      .map((o) => o.minecraft_version)
      .sort((a, b) => a.localeCompare(b, undefined, { numeric: true })) || [];
  const version = versions.includes(chosenVersion) ? chosenVersion : versions.at(-1) || "";
  const selected = enabledPlans?.find((p) => p.id === planID);
  const beta = checkoutConfig?.provider === "disabled" && session?.user.is_admin;
  const software = [...new Set(options?.map((o) => o.server_type) || [])];
  async function submit(event) {
    event.preventDefault();
    if (
      step !== 3 ||
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
  const choicesReady = !!options?.length && !optionsError;
  const canReview = choicesReady && !!selected && !plansError;
  const createDisabled =
    busy ||
    !canReview ||
    !!checkoutError ||
    !version ||
    !name.trim() ||
    (!beta && !checkoutConfig?.checkout_enabled);
  const displayedStyle = type === "PAPER" ? (style === "mods" ? "regular" : style) : "mods";
  const modType = software.includes("FABRIC") ? "FABRIC" : "FORGE";
  const styles = [
    {
      id: "regular",
      type: "PAPER",
      icon: "🌳",
      name: "Regular Minecraft",
      description: "Classic adventures. No extra downloads.",
    },
    {
      id: "plugins",
      type: "PAPER",
      icon: "🏰",
      name: "Minecraft with plugins",
      description: "Add server plugins after setup.",
    },
    {
      id: "mods",
      type: modType,
      icon: "🧩",
      name: "Minecraft with mods",
      description: "New blocks, creatures and modpacks.",
    },
  ];
  return (
    <Shell>
      <PageHeading eyebrow="A NEW ADVENTURE" title="Create your world" />
      <ErrorMessage error={plansError || optionsError || checkoutError || error} />
      {error && (
        <p className="notice" role="status">
          Your choices are saved here. Try again when you’re ready.
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
        {["How to play", "Group size", "Name & create"].map((label, index) => (
          <button
            type="button"
            key={label}
            className={`button ${step === index + 1 ? "primary" : "secondary"}`}
            aria-current={step === index + 1 ? "step" : undefined}
            disabled={busy || (index === 1 && !choicesReady) || (index === 2 && !canReview)}
            onClick={() => setStep(index + 1)}
          >
            <span>{index + 1}</span>
            {label}
          </button>
        ))}
      </nav>
      {loading ? (
        <Loading message="Loading hosting plans…" />
      ) : (
        <form
          onSubmit={submit}
          className={`creation-wizard ${step === 3 ? "creation-review" : ""}`}
        >
          <section className="panel creation-step" aria-label={`Step ${step} of 3`}>
            <h2 ref={heading} tabIndex={-1}>
              {step === 1
                ? "How do you want to play?"
                : step === 2
                  ? "Who’s joining?"
                  : "Name your world"}
            </h2>
            {step === 1 && (
              <>
                <div className="play-style-grid">
                  {styles.map((choice) => (
                    <button
                      type="button"
                      key={choice.id}
                      className={`play-style-card ${displayedStyle === choice.id ? "selected" : ""}`}
                      aria-pressed={displayedStyle === choice.id}
                      disabled={busy || !software.includes(choice.type)}
                      onClick={() => {
                        setStyle(choice.id);
                        setType(choice.type);
                        setVersion("");
                      }}
                    >
                      <span className={`play-style-art style-art-${choice.id}`} aria-hidden="true">
                        {choice.icon}
                      </span>
                      <strong>{choice.name}</strong>
                      <small>{choice.description}</small>
                      <span className="play-style-label">
                        {!software.includes(choice.type)
                          ? "Not available yet"
                          : displayedStyle === choice.id
                            ? "Selected"
                            : "Choose"}
                      </span>
                    </button>
                  ))}
                </div>
                <p className="field-help">
                  {displayedStyle === "mods"
                    ? "Install your mods after setup. Players may need matching downloads."
                    : "You can add plugins after setup."}
                </p>
              </>
            )}
            {step === 2 && (
              <>
                <div className="group-picker" aria-label="Player group size">
                  {PLAYER_GROUPS.map((value) => {
                    const match = recommendGroupPlan(enabledPlans, value.players);
                    return (
                      <button
                        type="button"
                        key={value.id}
                        className={`group-card ${group === value.id ? "selected" : ""}`}
                        aria-pressed={group === value.id}
                        disabled={busy || !match}
                        onClick={() => {
                          setGroup(value.id);
                          setPlanID("");
                        }}
                      >
                        <span aria-hidden="true">{value.icon}</span>
                        <strong>{value.title}</strong>
                        <span>Up to {value.players} players</span>
                        <b>
                          {match
                            ? `${price(match)}/${match.billing_period === "YEARLY" ? "year" : "month"}`
                            : "Unavailable"}
                        </b>
                      </button>
                    );
                  })}
                </div>
                <p className="field-help">
                  Suggested group sizes. Modpacks may need a larger plan.
                </p>
                {!recommended && (
                  <p className="notice">No suitable plan is available for this group.</p>
                )}
              </>
            )}
            {step === 3 && (
              <>
                <label className="field">
                  Server name
                  <input
                    value={name}
                    disabled={busy}
                    onChange={(event) => setName(event.target.value)}
                    maxLength={80}
                    required
                    placeholder="Our survival world"
                    autoComplete="off"
                  />
                </label>
                <details className="creation-advanced">
                  <summary>Advanced settings</summary>
                  <div className="field-row">
                    <label className="field">
                      Server software
                      <Select
                        aria-label="Server software"
                        value={type}
                        onChange={(event) => {
                          setType(event.target.value);
                          setStyle(event.target.value === "PAPER" ? "plugins" : "mods");
                          setVersion("");
                        }}
                        disabled={busy || !software.length}
                      >
                        {software.map((value) => (
                          <option key={value} value={value}>
                            {softwareName(value)}
                          </option>
                        ))}
                      </Select>
                    </label>
                    <label className="field">
                      Minecraft version
                      <Select
                        aria-label="Minecraft version"
                        value={version}
                        onChange={(event) => setVersion(event.target.value)}
                        disabled={busy || !versions.length}
                      >
                        {versions.map((value) => (
                          <option key={value} value={value}>
                            {value}
                          </option>
                        ))}
                      </Select>
                    </label>
                    <label className="field">
                      Hosting plan
                      <Select
                        aria-label="Hosting plan"
                        value={planID}
                        disabled={busy || !enabledPlans?.length}
                        onChange={(event) => setPlanID(event.target.value)}
                      >
                        {enabledPlans?.map((plan) => (
                          <option key={plan.id} value={plan.id}>
                            {plan.name} · {price(plan)} /{" "}
                            {plan.billing_period === "YEARLY" ? "year" : "month"}
                          </option>
                        ))}
                      </Select>
                    </label>
                  </div>
                </details>
              </>
            )}
            {!optionsError && options && !options.length && (
              <p className="notice">No Minecraft versions are available yet.</p>
            )}
            <div className="wizard-actions">
              {step > 1 && (
                <button
                  type="button"
                  className="button secondary"
                  disabled={busy}
                  onClick={() => setStep(step - 1)}
                >
                  Back
                </button>
              )}
              {step < 3 && (
                <button
                  type="button"
                  className="button primary"
                  disabled={busy || !choicesReady || (step === 2 && !canReview)}
                  onClick={() => setStep(step + 1)}
                >
                  Next <Icon name="arrow" size={16} />
                </button>
              )}
            </div>
          </section>
          {step === 3 && (
            <aside className="panel order-summary">
              <h2>{name.trim() || "Your new world"}</h2>
              <p>
                {softwareName(type)} · Minecraft {version || "Unavailable"}
              </p>
              {selected && (
                <>
                  <div className="summary-line">
                    <span>{audience.title}</span>
                    <span>{selected.name}</span>
                  </div>
                  <div className="summary-line">
                    <span>{formatBytes(selected.ram_mb * 1048576, 0)} RAM</span>
                    <span>{formatBytes(selected.disk_mb * 1048576, 0)} storage</span>
                  </div>
                  <hr />
                  <div className="summary-line">
                    <strong>{price(selected)}</strong>
                    <span>/{selected.billing_period === "YEARLY" ? "year" : "month"}</span>
                  </div>
                </>
              )}
              <p className="activation-note">
                {beta
                  ? "Private beta · No charge"
                  : checkoutConfig?.provider === "mock"
                    ? "Demo checkout · No real charge"
                    : checkoutConfig?.checkout_enabled
                      ? "Review payment at checkout."
                      : "Checkout is currently unavailable."}
              </p>
              <button className="button primary wide" disabled={createDisabled}>
                {busy ? "Creating server…" : beta ? "Create beta server" : "Continue to checkout"}
                <Icon name="arrow" size={16} />
              </button>
            </aside>
          )}
        </form>
      )}
    </Shell>
  );
}

"use client";
import Link from "next/link";
import { useRef, useState } from "react";
import { useUnsavedChanges } from "./use-unsaved-changes";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Select } from "./select";
import { ErrorMessage, Loading, PageHeading, Shell } from "./ui";

export function AddonSettings({ id }) {
  const { api } = useAuth();
  const server = useResource(`/servers/${id}`, 3000);
  const permitted = server.data?.permissions?.includes("settings");
  const resource = useResource(permitted ? `/servers/${id}/addons/simple-voice-chat` : null, 15000);
  const usage = useResource(
    permitted && server.data?.management_available ? `/servers/${id}/usage` : null,
    3000,
  );
  const [draft, setDraft] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const request = useRef(null);
  const saveAttempt = useRef(null);
  const config = resource.data;
  const networkStatus = useResource(
    permitted && config?.installed ? `/servers/${id}/addons/simple-voice-chat/network` : null,
    3000,
  );
  const network = networkStatus.data || config?.network;
  const values = draft?.values || config?.values;
  const offline = usage.data?.power_state === "offline";
  const unsaved =
    !!draft &&
    !!config?.values &&
    Object.keys(draft.values).some((key) => draft.values[key] !== config.values[key]);
  useUnsavedChanges(unsaved);
  function edit(key, value) {
    setDraft({ revision: draft?.revision || config.revision, values: { ...values, [key]: value } });
    setNotice("");
  }
  async function prepare() {
    setBusy(true);
    setError(null);
    setNotice("");
    request.current ||= crypto.randomUUID();
    try {
      await api(`/servers/${id}/addons/simple-voice-chat/network`, {
        method: "POST",
        key: request.current,
        body: {},
      });
      request.current = null;
      setNotice("Preparing a dedicated voice port. This page will update automatically.");
      networkStatus.refresh();
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  async function save(event) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setNotice("");
    const body = { ...values, revision: draft?.revision || config.revision };
    const fingerprint = JSON.stringify([id, body]);
    if (saveAttempt.current?.fingerprint !== fingerprint)
      saveAttempt.current = { fingerprint, key: crypto.randomUUID() };
    try {
      await api(`/servers/${id}/addons/simple-voice-chat`, {
        method: "PUT",
        key: saveAttempt.current.key,
        body,
      });
      saveAttempt.current = null;
      setDraft(null);
      resource.refresh();
      setNotice("Settings saved. Start or restart your server to apply them.");
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Shell>
      <Link
        href={`/servers/${id}/${["PAPER", "SPIGOT", "PURPUR"].includes(server.data?.server_type) ? "plugins" : "mods"}`}
        className="back-link"
      >
        ← Plugins & mods
      </Link>
      <PageHeading
        eyebrow="GUIDED ADDON SETTINGS"
        title="Simple Voice Chat"
        description="Make it yours without editing a configuration file."
      />
      <ErrorMessage error={server.error || resource.error || networkStatus.error || error} />
      {resource.error && (
        <Link className="button secondary" href={`/servers/${id}/files`}>
          Review configuration in Files
        </Link>
      )}
      {notice && (
        <p className="notice success" role="status">
          {notice}
        </p>
      )}
      {server.loading || resource.loading ? (
        <Loading />
      ) : !permitted ? (
        <p className="notice">An owner or administrator can configure this addon.</p>
      ) : (
        config && (
          <div className="addon-settings-layout">
            <div>
              <section className="panel management-panel" aria-label="Voice chat networking">
                <div className="section-heading">
                  <div>
                    <h2>Connection setup</h2>
                    <span>Blockyard manages the voice port for your server.</span>
                  </div>
                  <span className="addon-state">
                    {network.status === "READY"
                      ? "Port assigned"
                      : network.status === "PREPARING"
                        ? "Preparing…"
                        : "Needs setup"}
                  </span>
                </div>
                {!config.installed ? (
                  <p>
                    Install Simple Voice Chat from the compatible catalog first. Its standard JAR
                    filename must contain “voicechat” so Blockyard can recognize it.
                  </p>
                ) : network.status === "READY" ? (
                  <>
                    <p className="addon-endpoint">
                      <code>
                        {network.hostname}:{network.port}
                      </code>
                      <span>Voice connection · UDP</span>
                    </p>
                    <p className="field-help">
                      Saving settings uses this dedicated port automatically. Your Minecraft address
                      stays the same.
                    </p>
                  </>
                ) : network.status === "PREPARING" ? (
                  <p role="status">
                    Preparing your voice connection. You can fill out the settings while you wait.
                  </p>
                ) : network.status === "FAILED" ? (
                  <>
                    <p className="notice">
                      Your host needs to review this port setup
                      {network.error_code === "node_capacity_unavailable"
                        ? " and make a voice-capable port available"
                        : " before it can continue"}
                      . Your existing game server and files are preserved.
                    </p>
                    <button className="button secondary" disabled={busy} onClick={prepare}>
                      Check setup again
                    </button>
                  </>
                ) : (
                  <>
                    <p className="field-help">
                      Prepare a separate voice connection before saving. No port numbers or firewall
                      changes are needed from you.
                    </p>
                    <button className="button primary" disabled={busy} onClick={prepare}>
                      {busy ? "Preparing…" : "Prepare voice connection"}
                    </button>
                  </>
                )}
              </section>
              <section className="panel management-panel" aria-label="Voice chat settings">
                <div className="section-heading">
                  <div>
                    <h2>Voice chat settings</h2>
                    <span>
                      {config.config_exists
                        ? "Loaded from your server’s configuration"
                        : "Recommended defaults for your first setup"}
                    </span>
                  </div>
                  <button
                    className="button secondary"
                    disabled={busy}
                    onClick={() =>
                      setDraft({
                        revision: draft?.revision || config.revision,
                        values: { ...config.defaults },
                      })
                    }
                  >
                    Reset to defaults
                  </button>
                </div>
                {unsaved && (
                  <p className="notice" role="status">
                    You have unsaved voice settings. Save them before leaving this page.
                  </p>
                )}
                {!offline && (
                  <p className="notice">
                    Stop your server before saving settings. You can edit the form now.
                  </p>
                )}
                {draft && config.revision !== draft.revision && (
                  <p className="notice">
                    The configuration changed while you were editing. Reload settings before saving
                    to avoid overwriting those changes.
                  </p>
                )}
                <form onSubmit={save}>
                  <div className="field-row">
                    {config.fields
                      .filter((field) => field.kind === "number")
                      .map((field) => (
                        <label className="field" key={field.key}>
                          {field.label}
                          <input
                            type="number"
                            aria-label={field.label}
                            required
                            min={field.min}
                            max={
                              field.key === "whisper_distance"
                                ? Math.min(field.max, values.max_voice_distance)
                                : field.max
                            }
                            step="any"
                            value={values[field.key]}
                            disabled={busy}
                            onChange={(event) =>
                              edit(
                                field.key,
                                event.target.value === "" ? "" : Number(event.target.value),
                              )
                            }
                          />
                          <span className="field-help">{field.help}</span>
                        </label>
                      ))}
                  </div>
                  <div className="settings-toggles">
                    {config.fields
                      .filter((field) => field.kind === "boolean")
                      .map((field) => (
                        <label className="setting-toggle" key={field.key}>
                          <span>
                            <strong>{field.label}</strong>
                            <small>{field.help}</small>
                          </span>
                          <input
                            type="checkbox"
                            aria-label={field.label}
                            checked={values[field.key]}
                            disabled={busy}
                            onChange={(event) => edit(field.key, event.target.checked)}
                          />
                        </label>
                      ))}
                  </div>
                  {values.force_voice_chat && (
                    <p className="notice">
                      Players must install the client mod before joining. Requiring it does not
                      offer an automatic mod download.
                    </p>
                  )}
                  <p className="field-help">
                    Changes take effect on the next server start. Unrelated configuration settings
                    stay intact.
                  </p>
                  <div className="file-actions addon-save-actions">
                    <button
                      className="button primary"
                      disabled={
                        busy ||
                        !offline ||
                        !config.installed ||
                        network.status !== "READY" ||
                        (draft && config.revision !== draft.revision)
                      }
                    >
                      {busy ? "Saving…" : "Save settings"}
                    </button>
                    {unsaved && (
                      <button
                        type="button"
                        className="button secondary"
                        disabled={busy}
                        onClick={() => {
                          setDraft(null);
                          setError(null);
                          setNotice("");
                        }}
                      >
                        Discard changes
                      </button>
                    )}
                    <button
                      type="button"
                      className="button secondary"
                      disabled={busy}
                      onClick={() => {
                        setDraft(null);
                        setError(null);
                        resource.refresh();
                      }}
                    >
                      Reload settings
                    </button>
                  </div>
                </form>
              </section>
            </div>
            <ClientSetup server={server.data} />
          </div>
        )
      )}
    </Shell>
  );
}

function ClientSetup({ server }) {
  const initial = ["FABRIC", "FORGE", "NEOFORGE"].includes(server.server_type)
    ? server.server_type.toLowerCase()
    : "fabric";
  const [loader, setLoader] = useState(initial);
  const [clientVersion, setClientVersion] = useState(server.minecraft_version);
  const validVersion = /^[0-9]+\.[0-9]+(?:\.[0-9]+)?$/.test(clientVersion);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState(null);
  const download = `https://modrinth.com/mod/simple-voice-chat/versions?g=${encodeURIComponent(clientVersion)}&l=${loader}`;
  const address = `${server.hostname || "Your server address"}:${server.port || ""}`;
  async function copy() {
    try {
      await navigator.clipboard.writeText(
        `Join ${server.name}\nMinecraft Java ${clientVersion}\nServer: ${address}\nInstall Simple Voice Chat for your client loader (${loader}): ${download}\nLaunch the modded profile, connect, and press V to set up your microphone.\nClient guide: https://modrepo.de/minecraft/voicechat/wiki/client_setup`,
      );
      setCopied(true);
      setError(null);
    } catch {
      setError(new Error("Copy failed. Share the client guide link below instead."));
    }
  }
  return (
    <aside className="panel addon-client-guide" aria-label="Player setup">
      <span className="eyebrow">FOR YOUR PLAYERS</span>
      <h2>Set up the client</h2>
      <p>
        Players need Simple Voice Chat on their computers, including when your server uses Paper.
      </p>
      <label className="field">
        Client loader
        <Select
          aria-label="Client loader"
          value={loader}
          onChange={(event) => {
            setLoader(event.target.value);
            setCopied(false);
          }}
        >
          <option value="fabric">Fabric</option>
          <option value="forge">Forge</option>
          <option value="neoforge">NeoForge</option>
        </Select>
        <span className="field-help">
          Choose the loader used by the player’s Minecraft profile or modpack.
        </span>
      </label>
      <label className="field">
        Player’s Minecraft version
        <input
          aria-label="Player’s Minecraft version"
          value={clientVersion}
          maxLength={32}
          onChange={(event) => {
            setClientVersion(event.target.value.trim());
            setCopied(false);
          }}
        />
        <span className="field-help">
          Use the version in the player’s launcher. Match your server’s version unless you have
          confirmed compatibility.
        </span>
      </label>
      <ol className="addon-setup-steps">
        <li>Install or open the matching modded Minecraft profile.</li>
        <li>
          Download Simple Voice Chat for Minecraft {clientVersion || "your client version"} and the
          selected client loader. Match the server’s voice-chat release when possible.
        </li>
        <li>Add it to that profile’s mods folder, then relaunch Minecraft.</li>
        <li>
          Join your server and press <kbd>V</kbd> to select microphone and speakers. Choose
          push-to-talk or voice activation, then use Enable microphone testing. Check that your
          microphone is not muted.
        </li>
      </ol>
      <p className="field-help">
        If V does nothing, check Minecraft’s key bindings. A disconnected plug icon means voice has
        not connected; ask your host to check the voice connection separately. On macOS, allow
        microphone access for the launcher or Java application in System Settings → Privacy &amp;
        Security → Microphone.
      </p>
      <a
        className="button primary"
        href={validVersion ? download : undefined}
        aria-disabled={!validVersion}
        target="_blank"
        rel="noreferrer"
      >
        Find client download ↗
      </a>
      <button className="button secondary" disabled={!validVersion} onClick={copy}>
        Copy setup instructions
      </button>
      {copied && <p role="status">Instructions copied.</p>}
      <ErrorMessage error={error} />
      <div className="addon-client-note">
        <h3>Can players accept a download when joining?</h3>
        <p>
          Minecraft’s built-in download prompt is for resource packs. It cannot install client mods.
          Mods must be installed before connecting; automated mod syncing needs a compatible
          launcher or a companion mod already installed.
        </p>
      </div>
      <a
        className="text-link"
        href="https://modrepo.de/minecraft/voicechat/wiki/client_setup"
        target="_blank"
        rel="noreferrer"
      >
        Official client setup guide ↗
      </a>
    </aside>
  );
}

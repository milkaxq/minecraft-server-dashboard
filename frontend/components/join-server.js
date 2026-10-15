"use client";
import Link from "next/link";
import { useRef, useState } from "react";
import { ClientRequirementsNotice, clientCompatibility } from "./client-compatibility";
import { Select } from "./select";
import { useResource } from "./data";
import { softwareName, supportsMods } from "../lib/software";

function joinInstructions(server, address, powerState, settings, setup) {
  const lines = [
    server.name,
    `Minecraft Java ${server.minecraft_version}`,
    `Address: ${address}`,
    `Server: ${softwareName(server.server_type)}`,
    `Status: ${powerState || "check with the host"}`,
  ];
  if (supportsMods(server.server_type))
    lines.push(
      `For required client mods, use ${softwareName(server.server_type)}${server.loader_version ? ` ${server.loader_version}` : ""} and the matching Minecraft version. Check each mod's instructions.`,
    );
  else
    lines.push(
      "Most plugins work with vanilla Java. Check the client requirements below for exceptions.",
    );
  if (!setup) lines.push("Client requirements could not be checked. Ask the host before joining.");
  else {
    const addons = (setup.addons || []).filter(
      (addon) => clientCompatibility(addon).kind !== "server",
    );
    if (!addons.length)
      lines.push(
        supportsMods(server.server_type)
          ? "No client-required mods were identified. Confirm the mod setup with the host."
          : "No client mods required by the installed addons were identified.",
      );
    for (const addon of addons) {
      const info = clientCompatibility(addon);
      lines.push(`${addon.title || addon.filename}: ${info.label}. ${info.help}`);
      if (addon.project_id)
        lines.push(`Setup: https://modrinth.com/project/${encodeURIComponent(addon.project_id)}`);
    }
    lines.push(
      "Client mods must be installed before joining. A resource-pack prompt does not install mods.",
    );
  }
  lines.push(
    settings?.online_mode === true
      ? "Sign in through Microsoft with an account that owns Minecraft Java Edition."
      : settings?.online_mode === false
        ? "Microsoft account verification is off. Ask the host for the server's login instructions; a login plugin may require a separate password."
        : "Ask the host which account and login method are required.",
  );
  lines.push(
    settings?.whitelist === true
      ? "Whitelist is enabled: send your exact Minecraft username to the host before joining."
      : settings?.whitelist === false
        ? "Whitelist is disabled. Other login and client requirements still apply."
        : "Ask the host whether your username must be added to the whitelist.",
  );
  lines.push(
    "Multiplayer → Add Server → paste the address → Join Server.",
    "Wait until the server is Online before connecting. Setup reflects saved settings; changes may need a restart.",
  );
  return lines.join("\n");
}

export function JoinServer({ server, ready, powerState, address, onCopy }) {
  const canReadSettings = server.permissions?.includes("settings");
  const settings = useResource(
    ready && canReadSettings ? `/servers/${server.id}/settings` : null,
    10000,
  );
  const setup = useResource(ready ? `/servers/${server.id}/client-requirements` : null, 15000);
  const saved = settings.error ? null : settings.data;
  const verified = saved?.online_mode;
  const whitelist = saved?.whitelist;
  const details = useRef(null);
  const [copied, setCopied] = useState(false);
  const [copying, setCopying] = useState(false);
  const [fallback, setFallback] = useState("");
  const [problem, setProblem] = useState("session");
  async function share() {
    setCopying(true);
    setCopied(false);
    setFallback("");
    const text = joinInstructions(
      server,
      address,
      powerState,
      saved,
      setup.error ? null : setup.data,
    );
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
    } catch {
      setFallback(text);
    } finally {
      setCopying(false);
    }
  }
  function downloadSetup() {
    const text = joinInstructions(
      server,
      address,
      powerState,
      saved,
      setup.error ? null : setup.data,
    );
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "minecraft-player-setup.txt";
    link.click();
    URL.revokeObjectURL(url);
  }
  const problems = {
    session: {
      title: "Invalid session or failed to verify username",
      text:
        verified === false
          ? "Microsoft verification is off in the saved settings. If Minecraft still reports an invalid session, the running server may need a restart to apply that change. Ask the host to check the running login configuration and any login plugin."
          : "Invalid session: sign out of your launcher and sign back in through Microsoft with an account that owns Java Edition. Restart Minecraft. The launcher name alone does not determine whether your account is verified.",
    },
    whitelist: {
      title: "You are not whitelisted",
      text: "Not whitelisted: send the host your exact Minecraft username. The host can add it on Players while the server is online. Changing launchers or account mode can change the identity associated with a username.",
    },
    version: {
      title: "Outdated client or incompatible version",
      text: `Open Minecraft Java ${server.minecraft_version}. A newer client is not automatically compatible with an older server. If the host uses a compatibility plugin, check the versions it actually supports.`,
    },
    mods: {
      title: "Missing mods, mismatched mod list, or incompatible loader",
      text: `Match Minecraft ${server.minecraft_version}, the required client loader, and each required mod's version and dependencies. Paper plugins are installed on the server; Forge, NeoForge, and Fabric gameplay mods may also need installation on your computer. Open the player setup for this server's requirements.`,
    },
    connection: {
      title: "Connection refused or timed out",
      text: `Wait for Online and use ${address || "the full server address"}, including the port. Double-check Java Edition rather than Bedrock. If the address is correct and the server is online, ask the host to check game-port connectivity.`,
    },
    voice: {
      title: "Joined successfully, but voice chat does not connect",
      text: "Install the Simple Voice Chat client mod with a supported loader and matching Minecraft version. Check microphone permissions and the mod's setup screen. The host should check Voice Chat settings and the voice connection separately from the normal game connection.",
    },
  };
  const help = problems[problem];
  return (
    <section className="panel join-server" aria-labelledby="join-server-title">
      <div className="section-heading">
        <div>
          <span className="eyebrow">PLAY TOGETHER</span>
          <h2 id="join-server-title">How to join</h2>
          <p>
            {powerState === "running"
              ? "Your server is online. Share its address and player setup with friends."
              : powerState === "starting"
                ? "Minecraft is starting. Wait for Online before connecting."
                : powerState === "stopping"
                  ? "Minecraft is saving and shutting down. Wait until it stops before starting again."
                  : "Finish setup and start your server before connecting."}
          </p>
        </div>
      </div>
      <div className="join-share-actions">
        <code>{address || "Address available after setup"}</code>
        <button className="button secondary" onClick={onCopy} disabled={!address}>
          Copy address
        </button>
        <button className="button primary" onClick={share} disabled={!address || copying}>
          {copying ? "Copying…" : "Copy join instructions"}
        </button>
      </div>
      <button
        className="button secondary"
        disabled={!address || setup.loading}
        onClick={downloadSetup}
      >
        Download player setup checklist
      </button>
      {copied && <p role="status">Join instructions copied. Send them to your friends.</p>}
      {fallback && (
        <label className="clipboard-fallback">
          Clipboard unavailable. Select and copy these instructions.
          <textarea
            aria-label="Join instructions to copy"
            readOnly
            value={fallback}
            rows={9}
            onFocus={(event) => event.target.select()}
          />
        </label>
      )}
      <section className="join-setup-details" ref={details}>
        <h3>Join instructions</h3>
        <ClientRequirementsNotice server={server} ready={ready} resource={setup} />
        <ol className="join-steps">
          <li>
            <strong>Open Minecraft Java {server.minecraft_version}</strong>
            <p>
              {supportsMods(server.server_type)
                ? `Use ${softwareName(server.server_type)}${server.loader_version ? ` ${server.loader_version}` : ""} with the required mods listed above.`
                : "Use this Java Edition version and the required client mods listed above."}
            </p>
          </li>
          <li>
            <strong>
              {verified === true
                ? "Sign in with Microsoft"
                : verified === false
                  ? "Check the server’s login instructions"
                  : "Check which account you need"}
            </strong>
            <p>
              {verified === true
                ? "Use your Microsoft account with Minecraft Java Edition."
                : verified === false
                  ? "Account verification is off. Follow the owner’s login instructions; plugins are not installed automatically."
                  : "Ask the owner which login to use."}
            </p>
          </li>
          <li>
            <strong>Multiplayer → Add Server</strong>
            <p>Paste the full address, including its port, then choose Join Server.</p>
            <p>
              {whitelist === true
                ? "Whitelist is enabled. Ask the owner to add your username."
                : whitelist === false
                  ? "Whitelist is disabled."
                  : "Ask the owner if you need whitelist access."}
            </p>
            {canReadSettings && (
              <Link className="text-link" href={`/servers/${server.id}/players`}>
                Manage allowed players →
              </Link>
            )}
          </li>
        </ol>
      </section>
      <section className="join-troubleshooting">
        <h3>Can’t connect?</h3>
        <label>
          What does Minecraft show?
          <Select value={problem} onChange={(event) => setProblem(event.target.value)}>
            {Object.entries(problems).map(([key, value]) => (
              <option key={key} value={key}>
                {value.title}
              </option>
            ))}
          </Select>
        </label>
        <div className="troubleshooting-answer">
          <h3>{help.title}</h3>
          <p>{help.text}</p>
          {problem === "mods" && (
            <button
              className="button secondary"
              onClick={() => {
                details.current.scrollIntoView({ block: "start", behavior: "smooth" });
              }}
            >
              Show player setup
            </button>
          )}
          {problem === "whitelist" && canReadSettings && (
            <Link className="button secondary" href={`/servers/${server.id}/players`}>
              Open Players
            </Link>
          )}
          {problem === "voice" && canReadSettings && (
            <Link
              className="button secondary"
              href={`/servers/${server.id}/addons/simple-voice-chat`}
            >
              Voice chat setup
            </Link>
          )}
        </div>
        <Link className="text-link" href={`/feedback?server=${server.id}`}>
          Still stuck? Contact your host →
        </Link>
      </section>
    </section>
  );
}

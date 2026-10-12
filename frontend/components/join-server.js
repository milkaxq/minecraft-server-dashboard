"use client";
import Link from "next/link";
import { useResource } from "./data";
import { softwareName, supportsMods } from "../lib/software";

export function JoinServer({ server, ready, powerState, address, onCopy }) {
  const canReadSettings = server.permissions?.includes("settings");
  const settings = useResource(
    ready && canReadSettings ? `/servers/${server.id}/settings` : null,
    10000,
  );
  const verified = settings.data?.online_mode;
  const whitelist = settings.data?.whitelist;
  return (
    <section className="panel join-server" aria-labelledby="join-server-title">
      <div>
        <span className="eyebrow">PLAY TOGETHER</span>
        <h2 id="join-server-title">How to join</h2>
        <p>
          {powerState === "running"
            ? "Your server is online. Follow these steps to join."
            : powerState === "starting"
              ? "Minecraft is starting. Wait for Online before connecting."
              : powerState === "stopping"
                ? "Minecraft is saving and shutting down. Wait until it stops before starting again."
                : "Finish setup and start your server before connecting."}
        </p>
      </div>
      <ol className="join-steps">
        <li>
          <strong>Open Minecraft Java {server.minecraft_version}</strong>
          <p>
            {supportsMods(server.server_type)
              ? `Install ${softwareName(server.server_type)}${server.loader_version ? ` ${server.loader_version}` : ""} and the mods required by this server on your computer. Match the server’s Minecraft version.`
              : "Use the matching Java Edition version. Server plugins do not need to be installed on your computer."}
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
              ? "Use an account that owns Minecraft Java Edition, including in SKlauncher. No separate server password is needed unless the owner installed a login plugin."
              : verified === false
                ? "Minecraft account verification is off. Ask the owner whether AuthMe or another login plugin is installed. Offline players need server authentication to protect their usernames; plugins are not installed automatically."
                : "Ask the owner whether Microsoft account verification is enabled or a server login plugin is required. Login settings could not be checked here."}
          </p>
        </li>
        <li>
          <strong>Multiplayer → Add Server</strong>
          <p>Paste the full address, including its port, then choose Join Server.</p>
          <div className="join-address">
            <code>{address || "Address available after setup"}</code>
            <button className="button secondary" onClick={onCopy} disabled={!address}>
              Copy address
            </button>
          </div>
          <p>
            {whitelist === true
              ? "Whitelist is enabled. The owner must add your Minecraft username before you can join."
              : whitelist === false
                ? "Whitelist is disabled. Anyone who meets the login requirements can join."
                : "Ask the owner whether your username needs to be added to the whitelist."}
          </p>
          {canReadSettings && (
            <Link className="text-link" href={`/servers/${server.id}/players`}>
              Manage allowed players →
            </Link>
          )}
        </li>
      </ol>
      <details>
        <summary>Having trouble connecting?</summary>
        <ul>
          <li>Invalid session: sign out of your launcher and sign back in through Microsoft.</li>
          <li>Not whitelisted: ask the owner to add your exact Minecraft username on Players.</li>
          <li>Wrong version or missing mods: match the version and loader shown above.</li>
          <li>Connection refused: wait for Online and check the address and port.</li>
        </ul>
        <Link className="text-link" href={`/feedback?server=${server.id}`}>
          Still stuck? Contact your host →
        </Link>
      </details>
    </section>
  );
}

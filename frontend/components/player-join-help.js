"use client";
import { useState } from "react";
import { Select } from "./select";

export function PlayerJoinHelp({ setup }) {
  const [problem, setProblem] = useState("version");
  const answers = {
    version: [
      "Wrong Minecraft version",
      `Open Minecraft Java ${setup.minecraft_version}. A newer client is not automatically compatible. Use the profile from this invitation if client mods are required.`,
    ],
    login: [
      "Invalid session or username verification",
      setup.verified_login
        ? "Sign out of your launcher, then sign in through Microsoft with an account that owns Java Edition. Restart Minecraft. An offline username cannot pass this server’s account verification."
        : "Microsoft verification is off in the published setup. Ask the host to check the running login settings and any login plugin; a restart may be needed after settings change. Follow the server’s registration or login instructions if prompted.",
    ],
    whitelist: [
      "Not whitelisted",
      setup.whitelist
        ? "Send your exact Minecraft username to the host and ask them to add it to the allowed players. Then try joining again."
        : "This invitation says the whitelist is off. Ask the host to refresh player setup and check the running access rules.",
    ],
    mods: [
      "Missing mods or incompatible loader",
      setup.pack_reason || (setup.addons?.some((addon) => addon.required) && !setup.pack_ready)
        ? "The host needs to finish this player setup first. Ask them to resolve the issue shown above and refresh the invitation."
        : setup.pack_ready
          ? `Import the latest player pack into a separate profile and launch that profile. Match Minecraft ${setup.minecraft_version}, its loader and required mods. Downloading the pack alone does not install it.`
          : "No required client downloads are listed here. Ask the host which mod and version the error names, and ask them to refresh the invitation.",
    ],
    connection: [
      "Connection refused or timed out",
      `Ask the host whether Minecraft is Online. Use ${setup.address}, including the port, in Java Edition’s Multiplayer screen.`,
    ],
    voice: [
      "Voice chat does not work",
      "Install the matching Simple Voice Chat client mod if you want voice features. After joining, press V, choose your microphone and check permissions. If the voice icon stays disconnected, ask the host to check the voice connection.",
    ],
  };
  return (
    <details className="panel player-connection-help">
      <summary>Can’t join? Find a quick fix</summary>
      <label className="field">
        What does Minecraft show?
        <Select value={problem} onChange={(event) => setProblem(event.target.value)}>
          {Object.entries(answers).map(([key, [title]]) => (
            <option key={key} value={key}>
              {title}
            </option>
          ))}
        </Select>
      </label>
      <h3>{answers[problem][0]}</h3>
      <p>{answers[problem][1]}</p>
    </details>
  );
}

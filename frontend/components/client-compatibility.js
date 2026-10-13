"use client";
import Link from "next/link";
import { softwareName, supportsMods } from "../lib/software";

export function clientCompatibility(addon) {
  const voice = addon.project_id === "9eGKb6K1" || addon.configuration_id === "simple-voice-chat";
  if (voice) {
    if (addon.voice_required === true)
      return {
        kind: "required",
        label: "Client mod required to join",
        help: "Install Simple Voice Chat on your computer. This server requires it to join.",
      };
    if (addon.voice_required === false)
      return {
        kind: "optional",
        label: "Client mod for voice",
        help: "This addon allows vanilla clients. To hear or speak, install Fabric, Forge, or NeoForge and the matching Simple Voice Chat client mod.",
      };
    return {
      kind: "unknown",
      label: "Client mod for voice · check join rules",
      help: "Voice needs the Simple Voice Chat client mod. Check Require voice chat in its settings to see whether players without it can join.",
    };
  }
  if (addon.client_side === "required")
    return {
      kind: "required",
      label: "Client mod required",
      help: "Install this addon on each player's computer using its supported loader and Minecraft version. Check the author's setup instructions.",
    };
  if (addon.client_side === "optional")
    return {
      kind: "optional",
      label: "Client mod optional",
      help: "The author marks client installation as optional. Some features may need it; check the project instructions.",
    };
  if (addon.client_side === "unsupported")
    return {
      kind: "server",
      label: "Server only",
      help: "Install this addon on the server, not on players' computers.",
    };
  return {
    kind: "unknown",
    label: "Client details not provided",
    help: addon.project_id
      ? "Modrinth does not specify whether players need a client mod. This does not block installation or the player pack; we leave it out of the client download. Check the project instructions if a feature does not work."
      : "This uploaded file has no catalog record. Check the author's instructions for player requirements.",
  };
}

export function ClientRequirementTag({ addon }) {
  const info = clientCompatibility(addon);
  return (
    <span className={`client-requirement-tag client-requirement-${info.kind}`} title={info.help}>
      {info.label}
    </span>
  );
}

export function ClientRequirementsNotice({ server, ready, resource }) {
  const addons = resource.data?.addons || [];
  const required = addons.some((addon) => clientCompatibility(addon).kind === "required");
  const unknown = addons.some((addon) => clientCompatibility(addon).kind === "unknown");
  const optional = addons.some((addon) => clientCompatibility(addon).kind === "optional");
  const modded = supportsMods(server.server_type);
  const heading = !ready
    ? "Player setup available after provisioning"
    : resource.loading
      ? "Checking player setup…"
      : !resource.data || resource.error
        ? "Player requirements could not be checked"
        : required
          ? "Modded client required"
          : unknown
            ? "Check client requirements"
            : optional
              ? "Client mods for extra features"
              : modded
                ? "Check your mod setup"
                : "Vanilla Java supported";
  return (
    <div className="client-setup-notice" aria-label="Player client requirements">
      <strong>{heading}</strong>
      <p>
        {modded
          ? `This server runs ${softwareName(server.server_type)}. Install the client mods required by its authors with the matching loader and Minecraft ${server.minecraft_version}. Some server-only mods do not require a modded client.`
          : `This server runs ${softwareName(server.server_type)}. Most plugins work with vanilla Java Minecraft ${server.minecraft_version}; exceptions are listed below.`}
      </p>
      {resource.error && (
        <button className="button secondary" onClick={resource.refresh}>
          Check again
        </button>
      )}
      {!ready && <p>Player requirements will be checked after server setup finishes.</p>}
      {resource.data && !resource.error && (
        <>
          <ul className="client-setup-list">
            {addons.map((addon) => (
              <li key={addon.filename}>
                <strong>{addon.title || addon.filename}</strong>
                <ClientRequirementTag addon={addon} />
                <p>{clientCompatibility(addon).help}</p>
                {addon.project_id && (
                  <a
                    className="text-link"
                    href={`https://modrinth.com/project/${encodeURIComponent(addon.project_id)}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Downloads and setup instructions ↗
                  </a>
                )}
                {addon.configuration_id === "simple-voice-chat" &&
                  server.permissions?.includes("settings") && (
                    <Link
                      className="text-link"
                      href={`/servers/${server.id}/addons/simple-voice-chat`}
                    >
                      Voice chat setup →
                    </Link>
                  )}
              </li>
            ))}
          </ul>
          <p className="field-help">
            Based on installed files and catalog metadata. Configuration changes may need a restart
            before these rules take effect. Client mods must be installed before joining; Minecraft
            does not automatically download them.
          </p>
        </>
      )}
    </div>
  );
}

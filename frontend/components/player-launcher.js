"use client";
import { useEffect, useState, useSyncExternalStore } from "react";
import { Select } from "./select";
import { request } from "../lib/api";

const launchers = {
  modrinth: "Modrinth App",
  sklauncher: "SKlauncher",
  prism: "Prism Launcher",
  official: "Minecraft Launcher (official)",
  other: "Another launcher",
};
const preferenceKey = "blockyard-player-launcher";
function readPreference() {
  try {
    const value = localStorage.getItem(preferenceKey);
    return Object.hasOwn(launchers, value) ? value : "modrinth";
  } catch {
    return "modrinth";
  }
}
function subscribePreference(listener) {
  window.addEventListener("storage", listener);
  return () => window.removeEventListener("storage", listener);
}
export function useLauncherChoice() {
  const remembered = useSyncExternalStore(subscribePreference, readPreference, () => "modrinth");
  const [choice, setChoice] = useState(null);
  function choose(value) {
    setChoice(value);
    try {
      localStorage.setItem(preferenceKey, value);
    } catch {
      /* Selection still works when browser storage is unavailable. */
    }
  }
  return [choice ?? remembered, choose];
}
export function LauncherPicker({ launcher, onChange }) {
  return (
    <section className="panel player-launcher-picker">
      <label className="field">
        Which launcher do you use?
        <Select
          aria-label="Your Minecraft launcher"
          value={launcher}
          onChange={(event) => onChange(event.target.value)}
        >
          {Object.entries(launchers).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
      </label>
      <p className="field-help">
        Choose yours to see its setup steps. This choice stays in this browser.
      </p>
    </section>
  );
}
export function LauncherSetup({ launcher, setup, slug }) {
  const manual = ["sklauncher", "official", "other"].includes(launcher);
  const [files, setFiles] = useState(null);
  const [error, setError] = useState(null);
  useEffect(() => {
    if (!manual) return;
    const controller = new AbortController();
    request(`/join/${encodeURIComponent(slug)}/files`, { signal: controller.signal })
      .then((data) => {
        if (!controller.signal.aborted) {
          setFiles(data);
          setError(null);
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted) setError(error);
      });
    return () => controller.abort();
  }, [manual, slug]);
  if (launcher === "modrinth")
    return (
      <>
        <h2>Import and launch</h2>
        <p>
          Open the downloaded .mrpack in Modrinth App. It creates a profile and downloads its listed
          mods. Then click Play.
        </p>
        <p>Modrinth App requires a Microsoft account that owns Minecraft Java Edition.</p>
        <a
          className="button secondary"
          href="https://modrinth.com/app"
          target="_blank"
          rel="noreferrer"
        >
          Get Modrinth App ↗
        </a>
      </>
    );
  if (launcher === "prism")
    return (
      <>
        <h2>Import into Prism</h2>
        <p>
          Add Instance → Import → select the downloaded .mrpack. Wait for downloads, then launch the
          new instance.
        </p>
        <a
          className="button secondary"
          href="https://prismlauncher.org/wiki/help-pages/Zip-import/"
          target="_blank"
          rel="noreferrer"
        >
          Prism import guide ↗
        </a>
      </>
    );
  const loader =
    { fabric: "Fabric", forge: "Forge", neoforge: "NeoForge", quilt: "Quilt" }[setup.loader] ||
    setup.loader;
  return (
    <>
      <h2>{launcher === "sklauncher" ? "Set up SKlauncher" : "Set up your installation"}</h2>
      <ol className="launcher-install-steps">
        <li>
          Create a separate installation for <strong>Minecraft {setup.minecraft_version}</strong>{" "}
          with{" "}
          <strong>
            {loader} {setup.loader_version}
          </strong>
          . Use this player-pack loader even if the server uses Paper.
        </li>
        <li>
          {launcher === "sklauncher"
            ? "In SKlauncher, right-click the installation → Open installation directory. Open or create its mods folder."
            : "Open that installation’s game directory. Open or create its mods folder."}
        </li>
        <li>
          Download every file below and put the JARs in that mods folder. Use this installation’s
          folder to keep other profiles separate.
        </li>
        <li>Launch this installation, then join using the server address.</li>
      </ol>
      <p className="field-help">
        These are the exact client files from the published player pack, including its required
        dependencies. A .mrpack itself is not a mod JAR.
      </p>
      {error ? (
        <p role="alert">{error.message}</p>
      ) : !files ? (
        <p role="status">Getting matching client downloads…</p>
      ) : (
        <ul className="launcher-download-list">
          {files.map((file) => (
            <li key={file.path}>
              <a href={file.downloads[0]} target="_blank" rel="noreferrer">
                Download {file.path.split("/").pop()} ↗
              </a>
              <small>{(file.fileSize / 1048576).toFixed(1)} MB</small>
            </li>
          ))}
        </ul>
      )}
      {launcher === "sklauncher" && (
        <>
          <p>
            SKlauncher 3.x uses Installations Manager for loaders and manual mod files. In 4.x, you
            can also manage mods in the launcher. These pinned downloads work with either setup
            method.
          </p>
          <a
            className="text-link"
            href="https://docs.skmedix.pl/faq/launcher-related"
            target="_blank"
            rel="noreferrer"
          >
            SKlauncher setup help ↗
          </a>
        </>
      )}
      <p className="field-help">
        Selecting a launcher changes the instructions; it does not install software or open
        Minecraft automatically.
      </p>
    </>
  );
}

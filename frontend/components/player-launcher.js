"use client";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { Select } from "./select";
import { request } from "../lib/api";

const launchers = {
  modrinth: "Modrinth App",
  sklauncher: "SKlauncher 4",
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
    </section>
  );
}
export function LauncherSetup({ launcher, setup, slug }) {
  const [fallbackOpen, setFallbackOpen] = useState(false);
  const manual =
    ["official", "other"].includes(launcher) || (launcher === "sklauncher" && fallbackOpen);
  const [files, setFiles] = useState(null);
  const [filesError, setFilesError] = useState(null);
  const [bundleError, setBundleError] = useState(null);
  const [fileReload, setFileReload] = useState(0);
  const [fileRetryAt, setFileRetryAt] = useState(0);
  useEffect(() => {
    if (!fileRetryAt) return;
    const timer = setTimeout(() => setFileRetryAt(0), Math.max(0, fileRetryAt - Date.now()));
    return () => clearTimeout(timer);
  }, [fileRetryAt]);
  const bundleRequest = useRef(null);
  useEffect(() => () => bundleRequest.current?.abort(), []);
  const [bundleBusy, setBundleBusy] = useState(false);
  const [bundleDownloaded, setBundleDownloaded] = useState(false);
  useEffect(() => {
    if (!manual) return;
    const controller = new AbortController();
    request(`/join/${encodeURIComponent(slug)}/files`, { signal: controller.signal })
      .then((data) => {
        if (!controller.signal.aborted) {
          setFiles(data);
          setFilesError(null);
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          setFiles(null);
          setFilesError(error);
          setFileRetryAt(error.status === 429 ? Date.now() + (error.retryAfterMs || 60000) : 0);
        }
      });
    return () => controller.abort();
  }, [manual, slug, fileReload]);

  async function downloadBundle() {
    if (bundleRequest.current) return;
    const controller = new AbortController();
    bundleRequest.current = controller;
    setBundleBusy(true);
    setBundleError(null);
    setBundleDownloaded(false);
    try {
      const blob = await request(`/join/${encodeURIComponent(slug)}/bundle`, {
        download: true,
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "blockyard-client-mods.zip";
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 10000);
      setBundleDownloaded(true);
    } catch (error) {
      if (!controller.signal.aborted) setBundleError(error);
    } finally {
      bundleRequest.current = null;
      if (!controller.signal.aborted) setBundleBusy(false);
    }
  }
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
  const manualSteps = (
    <>
      <h2>{launcher === "sklauncher" ? "Manual setup fallback" : "Set up your installation"}</h2>
      <button className="button primary wide" disabled={bundleBusy} onClick={downloadBundle}>
        {bundleBusy ? "Preparing verified ZIP…" : "Download client mods ZIP"}
      </button>
      {bundleBusy && (
        <p role="status">
          Downloading and checking the published client files. This can take a moment.
        </p>
      )}
      {bundleDownloaded && (
        <p role="status">
          ZIP downloaded. Extract it and copy the mods folder into your installation.
        </p>
      )}
      <ol className="launcher-install-steps">
        <li>
          Create a separate installation for <strong>Minecraft {setup.minecraft_version}</strong>{" "}
          with{" "}
          <strong>
            {loader} {setup.loader_version}
          </strong>
          .
        </li>
        <li>
          {launcher === "sklauncher"
            ? "In SKlauncher 4, open your instance’s game folder. Open or create its mods folder."
            : "Open that installation’s game directory. Open or create its mods folder."}
        </li>
        <li>
          Extract the downloaded ZIP, then copy its mods folder into the installation directory. Use
          this installation’s folder to keep other profiles separate.
        </li>
        <li>Launch this installation, then join using the server address.</li>
      </ol>
      <p className="field-help">
        These are the exact client files from the published player pack, including its required
        dependencies. A .mrpack itself is not a mod JAR.
      </p>
      {bundleError && <p role="alert">{bundleError.message}</p>}
      {filesError ? (
        <div>
          <p role="alert">{filesError.message}</p>
          <button
            className="button secondary"
            disabled={bundleBusy || fileRetryAt > 0}
            onClick={() => {
              setFiles(null);
              setFilesError(null);
              setFileReload((value) => value + 1);
            }}
          >
            Retry client downloads
          </button>
        </div>
      ) : !files ? (
        <p role="status">Getting matching client downloads…</p>
      ) : (
        <details>
          <summary>Individual downloads</summary>
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
        </details>
      )}
      {launcher === "sklauncher" && (
        <>
          <p>
            This fallback is for SKlauncher 4 if importing the player pack fails. Use a separate
            instance with the exact game version and loader shown above.
          </p>
          <a
            className="text-link"
            href="https://docs.skmedix.pl/4.0/"
            target="_blank"
            rel="noreferrer"
          >
            SKlauncher setup help ↗
          </a>
        </>
      )}
    </>
  );
  if (launcher === "sklauncher")
    return (
      <>
        <h2>Import into SKlauncher 4</h2>
        <p>
          Drag the downloaded .mrpack file into SKlauncher 4. Confirm the import, wait for setup to
          finish, then click Play on the new instance.
        </p>
        <p className="field-help">
          The pack specifies the Minecraft version, loader and required client mods. Let the
          launcher install this new instance; do not add these files to your existing Forge profile.
        </p>
        <a
          className="button secondary"
          href="https://next.skmedix.pl/downloads"
          target="_blank"
          rel="noreferrer"
        >
          Get SKlauncher 4 ↗
        </a>
        <details onToggle={(event) => setFallbackOpen(event.currentTarget.open)}>
          <summary>Import failed? Use the manual ZIP</summary>
          {manualSteps}
        </details>
      </>
    );
  return manualSteps;
}

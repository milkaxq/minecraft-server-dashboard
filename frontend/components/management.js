"use client";
import { supportsPlugins, supportsMods } from "../lib/software";
import Link from "next/link";
import { ContextHelp } from "./context-help";
import { useEffect, useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Badge, ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes } from "../lib/api";

export function FeatureLinks({ id, server, advanced = false, active = "" }) {
  const permissions = server.permissions || [];
  const items = advanced
    ? [
        ["console", "Console", `/servers/${id}/console`],
        ["files", "Files", `/servers/${id}/files`],
      ]
    : [
        ["view", "Overview", `/servers/${id}`],
        ["worlds", "World", `/servers/${id}/worlds`],
        ["settings", "Settings", `/servers/${id}/settings`],
        ["backups", "Backups", `/servers/${id}/backups`],
        ...(supportsMods(server.server_type) ? [["files", "Mods", `/servers/${id}/mods`]] : []),
        ...(supportsPlugins(server.server_type)
          ? [["plugins", "Plugins", `/servers/${id}/plugins`]]
          : []),
      ];
  if (permissions.includes("members")) items.push(["members", "Members", `/servers/${id}/members`]);
  return (
    <nav className="feature-tabs" aria-label="Server pages">
      {items
        .filter(([permission]) => permissions.includes(permission))
        .map(([key, label, href]) => (
          <Link
            href={href}
            key={key}
            aria-current={
              active === key ||
              (label === "Mods" && active === "mods") ||
              (!active && (advanced ? key === "console" : key === "view"))
                ? "page"
                : undefined
            }
          >
            {label}
          </Link>
        ))}
    </nav>
  );
}
export function ManagementPage({ id, section }) {
  const { api } = useAuth();
  const info = useResource(`/servers/${id}`, 10000);
  const server = info.data;
  const usage = useResource(
    server?.status === "RUNNING" && server.management_available ? `/servers/${id}/usage` : null,
    5000,
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  async function power(action) {
    setBusy(true);
    setError(null);
    try {
      await api(`/servers/${id}/${action}`, {
        method: "POST",
        body: {},
        key: crypto.randomUUID(),
      });
      usage.refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  const ready = server?.status === "RUNNING" && server.management_available;
  const offline = usage.data?.power_state === "offline";
  return (
    <Shell>
      {info.loading ? (
        <Loading />
      ) : !server ? (
        <ErrorMessage error={info.error} />
      ) : (
        <>
          <Link href={`/servers/${id}`} className="back-link">
            ← Server overview
          </Link>
          <PageHeading
            eyebrow={`${server.server_type} · MINECRAFT ${server.minecraft_version}`}
            title={server.name}
            description={
              section === "files"
                ? "Your Minecraft files, in one place."
                : "Make this world your own."
            }
            action={<Badge state={usage.data?.power_state || server.status} />}
          />
          <div className="server-toolbar">
            <div className="mode-switch">
              <Link href={`/servers/${id}`} className={section !== "files" ? "selected" : ""}>
                Simple
              </Link>
              <Link
                href={`/servers/${id}/console`}
                className={section === "files" ? "selected" : ""}
              >
                Advanced
              </Link>
            </div>
            <div className="power-controls">
              <button
                className="button primary"
                disabled={!ready || busy || !offline}
                onClick={() => power("start")}
              >
                Start
              </button>
              <button
                className="button secondary"
                disabled={!ready || busy || usage.data?.power_state !== "running"}
                onClick={() => power("stop")}
              >
                Stop
              </button>
              <button
                className="button secondary"
                disabled={!ready || busy || usage.data?.power_state !== "running"}
                onClick={() => power("restart")}
              >
                Restart
              </button>
            </div>
          </div>
          <FeatureLinks id={id} server={server} advanced={section === "files"} active={section} />
          <ContextHelp topic={section} />
          <ErrorMessage error={info.error || error} />
          {!server.permissions?.includes(section === "mods" ? "files" : section) ? (
            <div className="notice">Your server role does not allow this page.</div>
          ) : !ready ? (
            <div className="notice">
              Your server must be activated and connected before you can manage it.
            </div>
          ) : (
            <>
              <ErrorMessage error={usage.error} />
              {!offline && section !== "backups" && (
                <p className="notice">
                  Stop your server before changing {section}. You can browse while it is online.
                </p>
              )}
              {section === "files" ? (
                <Files id={id} offline={offline} />
              ) : section === "worlds" ? (
                <Worlds id={id} offline={offline} limit={server.backup_limit} />
              ) : section === "settings" ? (
                <Settings id={id} offline={offline} />
              ) : section === "mods" ? (
                supportsMods(server.server_type) ? (
                  <Plugins id={id} offline={offline} mods />
                ) : (
                  <p className="notice">Mods require Fabric, Forge, or NeoForge.</p>
                )
              ) : section === "plugins" ? (
                supportsPlugins(server.server_type) ? (
                  <Plugins id={id} offline={offline} />
                ) : (
                  <p className="notice">
                    Plugins are available for Paper, Spigot, and Purpur servers.
                  </p>
                )
              ) : (
                <Backups id={id} offline={offline} limit={server.backup_limit} />
              )}
            </>
          )}
        </>
      )}
    </Shell>
  );
}
export function Confirm({
  title,
  children,
  label = "Confirm",
  busy,
  confirmDisabled = false,
  onCancel,
  onConfirm,
}) {
  const ref = useRef(null);
  useEffect(() => {
    const el = ref.current;
    el.showModal();
    return () => el.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className="confirm-dialog"
      aria-label={title}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <div className="confirm-icon">
        <Icon name="shield" size={25} />
      </div>
      <h2>{title}</h2>
      <div>{children}</div>
      <div className="dialog-actions">
        <button className="button secondary" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <button className="button danger" disabled={busy || confirmDisabled} onClick={onConfirm}>
          {busy ? "Working…" : label}
        </button>
      </div>
    </dialog>
  );
}
function useAction(refresh) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  async function run(action, success) {
    if (busy) return false;
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      await action();
      setNotice(success);
      refresh?.();
      return true;
    } catch (e) {
      setError(e);
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { busy, error, notice, run };
}
function Feedback({ action }) {
  return (
    <>
      <ErrorMessage error={action.error} />
      {action.notice && (
        <p className="notice success" role="status">
          {action.notice}
        </p>
      )}
    </>
  );
}
function query(value) {
  return encodeURIComponent(value);
}
function filePath(dir, name) {
  return `${dir === "/" ? "" : dir}/${name}`;
}
async function download(api, path, name) {
  const blob = await api(path, { download: true });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}
function Files({ id, offline }) {
  const { api } = useAuth();
  const [dir, setDir] = useState("/");
  const resource = useResource(`/servers/${id}/files?path=${query(dir)}`);
  const action = useAction(resource.refresh);
  const [editor, setEditor] = useState(null);
  const [create, setCreate] = useState(null);
  const [name, setName] = useState("");
  const [target, setTarget] = useState(null);
  const uploader = useRef(null);
  async function edit(entry) {
    await action.run(
      async () =>
        setEditor(await api(`/servers/${id}/files/text?path=${query(filePath(dir, entry.name))}`)),
      "",
    );
  }
  async function change(input, message) {
    const ok = await action.run(
      () =>
        api(`/servers/${id}/files`, {
          method: "POST",
          key: crypto.randomUUID(),
          body: input,
        }),
      message,
    );
    if (ok) {
      setTarget(null);
      setCreate(null);
    }
    return ok;
  }
  async function upload(file) {
    if (!file) return;
    await action.run(
      () =>
        api(`/servers/${id}/files/upload?path=${query(dir)}&name=${query(file.name)}`, {
          method: "POST",
          key: crypto.randomUUID(),
          raw: true,
          body: file,
        }),
      "File uploaded. Start your server to use it.",
    );
  }
  return (
    <>
      <Feedback action={action} />
      <ErrorMessage error={resource.error} />
      <section className="panel management-panel">
        <div className="section-heading">
          <div>
            <h2>Files</h2>
            <span>Browse your Minecraft server folder</span>
          </div>
          <div className="file-actions">
            <button
              className="button secondary"
              disabled={!offline || action.busy}
              onClick={() => {
                setCreate("folder");
                setName("");
              }}
            >
              New folder
            </button>
            <button
              className="button secondary"
              disabled={!offline || action.busy}
              onClick={() => {
                setCreate("create");
                setName("");
              }}
            >
              New file
            </button>
            <button
              className="button primary"
              disabled={!offline || action.busy}
              onClick={() => uploader.current.click()}
            >
              Upload file
            </button>
            <input
              ref={uploader}
              hidden
              type="file"
              onChange={(e) => {
                upload(e.target.files[0]);
                e.target.value = "";
              }}
            />
          </div>
        </div>
        <div className="file-path">
          <button
            disabled={dir === "/"}
            onClick={() => {
              setDir(dir.slice(0, dir.lastIndexOf("/")) || "/");
              setEditor(null);
            }}
          >
            ↑ Parent
          </button>
          <code>{dir}</code>
          <button onClick={resource.refresh}>Refresh</button>
        </div>
        <p className="field-help">
          Text editor: up to 1 MB. Upload: up to 64 MB. Existing files are never overwritten by
          uploads.
        </p>
        {resource.loading ? (
          <Loading />
        ) : !resource.data?.length ? (
          <p className="management-empty">This folder is empty.</p>
        ) : (
          <div className="table-scroll">
            <table className="management-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Size</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {resource.data.map((entry) => (
                  <tr key={entry.name}>
                    <td>
                      <button
                        className="file-name"
                        disabled={entry.is_symlink || action.busy}
                        onClick={() =>
                          entry.is_file
                            ? edit(entry)
                            : (setDir(filePath(dir, entry.name)), setEditor(null))
                        }
                      >
                        <Icon name={entry.is_file ? "server" : "world"} size={16} />
                        {entry.name}
                        {entry.is_symlink && " (link — unavailable)"}
                      </button>
                    </td>
                    <td>{entry.is_file ? formatBytes(entry.size) : "Folder"}</td>
                    <td>
                      <div className="row-actions">
                        {entry.is_file && (
                          <button
                            disabled={entry.is_symlink || action.busy}
                            onClick={() =>
                              action.run(
                                () =>
                                  download(
                                    api,
                                    `/servers/${id}/files/download?path=${query(filePath(dir, entry.name))}`,
                                    entry.name,
                                  ),
                                "Download ready.",
                              )
                            }
                          >
                            Download
                          </button>
                        )}
                        <button
                          disabled={!offline || entry.is_symlink || action.busy}
                          onClick={() => {
                            setTarget({ action: "rename", name: entry.name });
                            setName(entry.name);
                          }}
                        >
                          Rename
                        </button>
                        <button
                          className="text-danger"
                          disabled={!offline || entry.is_symlink || action.busy}
                          onClick={() => setTarget({ action: "delete", name: entry.name })}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      {editor && (
        <section className="panel management-panel editor-panel">
          <div className="section-heading">
            <div>
              <h2>Edit file</h2>
              <span>{editor.path}</span>
            </div>
            <button className="button secondary" onClick={() => setEditor(null)}>
              Close editor
            </button>
          </div>
          <textarea
            aria-label="File content"
            className="file-editor"
            value={editor.content}
            onChange={(e) => setEditor({ ...editor, content: e.target.value })}
            spellCheck={false}
          />
          <div className="file-actions">
            <button
              className="button secondary"
              disabled={action.busy}
              onClick={() =>
                action.run(
                  async () =>
                    setEditor(await api(`/servers/${id}/files/text?path=${query(editor.path)}`)),
                  "File reloaded.",
                )
              }
            >
              Reload file
            </button>
            <button
              className="button primary"
              disabled={!offline || action.busy}
              onClick={async () => {
                if (
                  await change(
                    {
                      action: "write",
                      path: editor.path,
                      content: editor.content,
                      revision: editor.revision,
                    },
                    "File saved. Start your server to apply the change.",
                  )
                )
                  setEditor(null);
              }}
            >
              Save file
            </button>
          </div>
        </section>
      )}
      {create && (
        <Confirm
          title={create === "folder" ? "Create folder" : "Create file"}
          label="Create"
          busy={action.busy}
          onCancel={() => setCreate(null)}
          onConfirm={() =>
            change({ action: create, path: filePath(dir, name) }, "Created successfully.")
          }
        >
          <label className="field">
            Name
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={255}
            />
          </label>
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
      {target && (
        <Confirm
          title={target.action === "delete" ? "Delete from your server?" : "Rename file or folder"}
          label={target.action === "delete" ? "Delete" : "Rename"}
          busy={action.busy}
          onCancel={() => setTarget(null)}
          onConfirm={() =>
            change(
              {
                action: target.action,
                path: filePath(dir, target.name),
                ...(target.action === "delete"
                  ? { confirmation: filePath(dir, target.name) }
                  : { destination: filePath(dir, name) }),
              },
              target.action === "delete" ? "Deleted." : "Renamed.",
            )
          }
        >
          <p>
            {target.action === "delete"
              ? `This permanently deletes ${target.name} and any files inside it. Create a backup first if you need to keep them.`
              : target.name}
          </p>
          {target.action === "rename" && (
            <label className="field">
              New name
              <input value={name} onChange={(e) => setName(e.target.value)} maxLength={255} />
            </label>
          )}
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
    </>
  );
}
function Settings({ id, offline }) {
  const { api } = useAuth();
  const resource = useResource(`/servers/${id}/settings`);
  const [draft, setDraft] = useState(null);
  const action = useAction(resource.refresh);
  const form = draft?.source === resource.data ? draft.values : resource.data;
  const set = (key, value) =>
    setDraft({ source: resource.data, values: { ...form, [key]: value } });
  async function save(e) {
    e.preventDefault();
    await action.run(
      () =>
        api(`/servers/${id}/settings`, {
          method: "PUT",
          body: form,
          key: crypto.randomUUID(),
        }),
      "Settings saved. Start your server to apply them.",
    );
  }
  return (
    <>
      <Feedback action={action} />
      <ErrorMessage error={resource.error} />
      <section className="panel management-panel">
        <div className="section-heading">
          <div>
            <h2>Minecraft settings</h2>
            <span>Changes apply on the next server start</span>
          </div>
          <button className="button secondary" onClick={resource.refresh}>
            Reload settings
          </button>
        </div>
        {resource.loading ? (
          <Loading />
        ) : (
          form && (
            <form onSubmit={save} className="settings-form">
              <label className="field">
                Server message (MOTD)
                <input
                  value={form.motd}
                  maxLength={256}
                  onChange={(e) => set("motd", e.target.value)}
                />
                <small>This message appears in the Minecraft multiplayer list.</small>
              </label>
              <div className="field-row">
                <label className="field">
                  Difficulty
                  <select
                    value={form.difficulty}
                    onChange={(e) => set("difficulty", e.target.value)}
                  >
                    {["peaceful", "easy", "normal", "hard"].map((v) => (
                      <option key={v} value={v}>
                        {v}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="field">
                  Game mode
                  <select value={form.gamemode} onChange={(e) => set("gamemode", e.target.value)}>
                    {["survival", "creative", "adventure", "spectator"].map((v) => (
                      <option key={v} value={v}>
                        {v}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <div className="field-row">
                {[
                  ["max_players", "Maximum players", 1, 1000],
                  ["view_distance", "View distance (chunks)", 2, 32],
                  ["spawn_protection", "Spawn protection (blocks)", 0, 1000],
                ].map(([key, label, min, max]) => (
                  <label key={key} className="field">
                    {label}
                    <input
                      type="number"
                      min={min}
                      max={max}
                      required
                      value={form[key]}
                      onChange={(e) => set(key, Number(e.target.value))}
                    />
                  </label>
                ))}
              </div>
              <div className="settings-toggles">
                {[
                  ["hardcore", "Hardcore", "Players cannot respawn after dying."],
                  ["pvp", "Player vs. player", "Allow players to damage each other."],
                  ["whitelist", "Whitelist", "Only players on your whitelist may join."],
                  [
                    "online_mode",
                    "Verify Minecraft accounts",
                    "Require players to sign in with their Minecraft accounts.",
                  ],
                ].map(([key, label, help]) => (
                  <label key={key} className="setting-toggle">
                    <span>
                      <strong>{label}</strong>
                      <small>{help}</small>
                    </span>
                    <input
                      type="checkbox"
                      checked={form[key]}
                      onChange={(e) => set(key, e.target.checked)}
                    />
                  </label>
                ))}
              </div>
              {!form.online_mode && (
                <p className="notice">
                  Account verification is off. Players may join using another player&apos;s name.
                  Enable it unless you use a trusted proxy with its own authentication.
                </p>
              )}
              <p className="field-help">
                Your hosting resources stay allocated by your plan. More players or a larger view
                distance can use more memory and CPU.
              </p>
              <button className="button primary" disabled={!offline || action.busy}>
                {action.busy ? "Saving…" : "Save settings"}
              </button>
            </form>
          )
        )}
      </section>
    </>
  );
}
function Plugins({ id, offline, mods = false }) {
  const collection = mods ? "mods" : "plugins";
  const label = mods ? "mod" : "plugin";
  const { api } = useAuth();
  const resource = useResource(`/servers/${id}/${collection}`);
  const action = useAction(resource.refresh);
  const input = useRef(null);
  const [remove, setRemove] = useState(null);
  async function upload(file) {
    if (!file) return;
    await action.run(
      () =>
        api(`/servers/${id}/${collection}/upload?name=${query(file.name)}`, {
          method: "POST",
          raw: true,
          body: file,
          key: crypto.randomUUID(),
        }),
      `${mods ? "Mod" : "Plugin"} uploaded. Start your server to load it.`,
    );
  }
  async function erase() {
    if (
      await action.run(
        () =>
          api(`/servers/${id}/${collection}`, {
            method: "DELETE",
            key: crypto.randomUUID(),
            body: { name: remove, confirmation: remove },
          }),
        `${mods ? "Mod" : "Plugin"} deleted. Start your server to apply the change.`,
      )
    )
      setRemove(null);
  }
  return (
    <>
      <Feedback action={action} />
      <ErrorMessage error={resource.error} />
      <CatalogBrowser
        id={id}
        collection={collection}
        offline={offline}
        onInstalled={resource.refresh}
      />
      <section className="panel management-panel">
        <div className="section-heading">
          <div>
            <h2>Your {collection}</h2>
            <span>
              {mods
                ? "Server mods for your chosen loader"
                : "Compatible plugins installed in your server folder"}
            </span>
          </div>
          <button
            className="button primary"
            disabled={!offline || action.busy}
            onClick={() => input.current.click()}
          >
            Upload {label} .jar
          </button>
          <input
            hidden
            ref={input}
            type="file"
            accept=".jar"
            onChange={(e) => {
              upload(e.target.files[0]);
              e.target.value = "";
            }}
          />
        </div>
        <p className="field-help">
          Upload a trusted {label} .jar, up to 64 MB. Stop your server before adding or removing{" "}
          {collection}, then start it to load the changes.{" "}
          {mods &&
            "Match your Minecraft version and loader. Install required dependencies too; client-only mods belong on your computer."}{" "}
          <Link href="/guide">Read the setup guide</Link>.
        </p>
        {resource.loading ? (
          <Loading />
        ) : !resource.data?.length ? (
          <p className="management-empty">No {collection} installed yet.</p>
        ) : (
          <ul className="plugin-list">
            {resource.data.map((v) => (
              <li key={v.name}>
                <span>
                  <strong>{v.name}</strong>
                  <small>{formatBytes(v.size)}</small>
                </span>
                <button
                  className="button danger-outline"
                  disabled={!offline || action.busy}
                  onClick={() => setRemove(v.name)}
                >
                  Delete {label}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
      {remove && (
        <Confirm
          title={`Delete this ${label}?`}
          label={`Delete ${label}`}
          busy={action.busy}
          onCancel={() => setRemove(null)}
          onConfirm={erase}
        >
          <p>{remove} will be removed. Its configuration folders will stay in your files.</p>
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
    </>
  );
}
function CatalogBrowser({ id, collection, offline, onInstalled }) {
  const { api } = useAuth();
  const [search, setSearch] = useState("");
  const [results, setResults] = useState(null);
  const [preview, setPreview] = useState(null);
  const [installKey, setInstallKey] = useState(null);
  const action = useAction(onInstalled);
  async function browse(e) {
    e.preventDefault();
    await action.run(async () => {
      setResults(await api(`/servers/${id}/${collection}/catalog?q=${query(search)}`));
    });
  }
  async function review(project) {
    await action.run(async () => {
      setPreview(await api(`/servers/${id}/${collection}/catalog/${query(project)}`));
      setInstallKey(crypto.randomUUID());
    });
  }
  async function install() {
    if (
      await action.run(
        () =>
          api(`/servers/${id}/${collection}/catalog/${query(preview.project)}`, {
            method: "POST",
            key: installKey,
            body: { revision: preview.revision },
          }),
        "Installed. Start your server to load the changes.",
      )
    )
      setPreview(null);
  }
  return (
    <section className="panel management-panel addon-catalog">
      <h2>Find {collection}</h2>
      <p className="field-help">
        Browse Modrinth releases matched to your server software and Minecraft version. Review
        required dependencies before installing. Stop your server to install.
      </p>
      {collection === "plugins" ? (
        <p className="field-help">
          Want newer Java clients to join? Search for ViaVersion. Your server keeps its existing
          gameplay version.
        </p>
      ) : (
        <p className="field-help">
          Players may need the same mods on their computers. Complete modpacks also include settings
          and client files; use the pack author’s server setup instructions.
        </p>
      )}
      <Feedback action={action} />
      <form onSubmit={browse} className="section-heading">
        <label>
          Search {collection}
          <input
            value={search}
            maxLength={100}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={
              collection === "plugins" ? "ViaVersion, LuckPerms…" : "Search server mods…"
            }
          />
        </label>
        <button className="button primary" disabled={action.busy}>
          Search Modrinth
        </button>
      </form>
      {results && (
        <ul className="plugin-list">
          {results.map((project) => (
            <li key={project.project_id}>
              <span>
                <strong>{project.title}</strong>
                <small>{project.description}</small>
                <a
                  href={`https://modrinth.com/project/${encodeURIComponent(project.project_id)}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  Project details
                </a>
              </span>
              <button
                className="button"
                disabled={action.busy}
                onClick={() => review(project.project_id)}
              >
                Review install
              </button>
            </li>
          ))}
        </ul>
      )}
      {results?.length === 0 && <p>No compatible projects found. Try another search.</p>}
      {preview && (
        <Confirm
          title="Review installation"
          label="Install now"
          busy={action.busy}
          confirmDisabled={!offline}
          onCancel={() => setPreview(null)}
          onConfirm={install}
        >
          <p>
            For {preview.software} {preview.minecraft_version}. Required dependencies are included.
            Existing files with different contents will not be overwritten. Remove older versions of
            these projects first to avoid duplicates.
          </p>
          <ul>
            {preview.files.map((file) => (
              <li key={file.version_id}>
                {file.filename} · {formatBytes(file.size)}
              </li>
            ))}
          </ul>
          {!offline && <p>Stop your server before installing.</p>}
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
    </section>
  );
}
function Backups({ id, offline, limit }) {
  const { api } = useAuth();
  const resource = useResource(`/servers/${id}/backups`, 10000);
  const action = useAction(resource.refresh);
  const [name, setName] = useState("");
  const [target, setTarget] = useState(null);
  async function create(e) {
    e.preventDefault();
    if (
      await action.run(
        () =>
          api(`/servers/${id}/backups`, {
            method: "POST",
            key: crypto.randomUUID(),
            body: { name: name.trim() },
          }),
        "Backup started. This page will update when it finishes.",
      )
    )
      setName("");
  }
  async function change() {
    const v = target;
    if (
      await action.run(
        () =>
          api(
            `/servers/${id}/backups/${v.backup.uuid}${v.action === "restore" ? "/restore" : ""}`,
            {
              method: v.action === "restore" ? "POST" : "DELETE",
              key: crypto.randomUUID(),
              body: { confirmation: v.backup.uuid },
            },
          ),
        v.action === "restore"
          ? "Restore requested. Wait for it to finish before starting your server."
          : "Backup deleted.",
      )
    )
      setTarget(null);
  }
  return (
    <>
      <Feedback action={action} />
      <ErrorMessage error={resource.error} />
      <section className="panel management-panel">
        <div className="section-heading">
          <div>
            <h2>Backups</h2>
            <span>
              {resource.data?.length ?? "—"} of {limit} backup slots used
            </span>
          </div>
          <button className="button secondary" onClick={resource.refresh}>
            Refresh
          </button>
        </div>
        <p className="field-help">
          For a consistent world backup, stop your server first. You can create backups while it is
          online, but files may change during the backup.
        </p>
        <form className="backup-create" onSubmit={create}>
          <label className="field">
            Backup name
            <input
              value={name}
              maxLength={100}
              required
              onChange={(e) => setName(e.target.value)}
              placeholder="Before our next adventure"
            />
          </label>
          <button
            className="button primary"
            disabled={
              action.busy || !name.trim() || !resource.data || resource.data.length >= limit
            }
          >
            Create backup
          </button>
        </form>
        {resource.loading ? (
          <Loading />
        ) : !resource.data?.length ? (
          <p className="management-empty">
            No backups yet. Save a copy of your world before making big changes.
          </p>
        ) : (
          <div className="table-scroll">
            <table className="management-table">
              <thead>
                <tr>
                  <th>Backup</th>
                  <th>Status</th>
                  <th>Size</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {resource.data.map((b) => (
                  <tr key={b.uuid}>
                    <td>
                      <strong>{b.name}</strong>
                      <small>{new Date(b.created_at).toLocaleString()}</small>
                    </td>
                    <td>{!b.completed_at ? "Creating…" : b.is_successful ? "Ready" : "Failed"}</td>
                    <td>{b.completed_at ? formatBytes(b.bytes) : "—"}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          disabled={action.busy || !b.is_successful || !b.completed_at}
                          onClick={() =>
                            action.run(
                              () =>
                                download(
                                  api,
                                  `/servers/${id}/backups/${b.uuid}/download`,
                                  `backup-${b.uuid}.tar.gz`,
                                ),
                              "Download ready.",
                            )
                          }
                        >
                          Download
                        </button>
                        <button
                          disabled={!offline || action.busy || !b.is_successful || !b.completed_at}
                          onClick={() => setTarget({ action: "restore", backup: b })}
                        >
                          Restore
                        </button>
                        <button
                          className="text-danger"
                          disabled={action.busy || !b.completed_at}
                          onClick={() => setTarget({ action: "delete", backup: b })}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      {target && (
        <Confirm
          title={
            target.action === "restore"
              ? "Replace your server with this backup?"
              : "Delete this backup?"
          }
          label={target.action === "restore" ? "Restore backup" : "Delete backup"}
          busy={action.busy}
          onCancel={() => setTarget(null)}
          onConfirm={change}
        >
          <p>
            {target.action === "restore"
              ? `Restoring “${target.backup.name}” replaces all current server files, including your world, settings and plugins. Changes made after this backup will be lost.`
              : `“${target.backup.name}” will be permanently deleted.`}
          </p>
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
    </>
  );
}

function Worlds({ id, offline, limit }) {
  const { api } = useAuth();
  const resource = useResource(`/servers/${id}/world`, 10000);
  const backups = useResource(`/servers/${id}/backups`, 10000);
  const action = useAction(() => {
    resource.refresh();
    backups.refresh();
  });
  const input = useRef(null);
  const [target, setTarget] = useState(null);
  const [confirmation, setConfirmation] = useState("");
  const world = resource.data;
  const safety = backups.data?.find((b) => b.uuid === world?.safety_backup);
  const ready = safety?.is_successful && safety.completed_at;
  function confirm(value) {
    setConfirmation("");
    setTarget(value);
  }
  async function change() {
    const values = { backup: safety.uuid, revision: world.revision, confirmation };
    const upload = target instanceof File;
    const endpoint = upload
      ? `/servers/${id}/world/upload?${new URLSearchParams(values)}`
      : `/servers/${id}/world/reset`;
    if (
      await action.run(
        () =>
          api(endpoint, {
            method: "POST",
            key: crypto.randomUUID(),
            ...(upload ? { raw: true, body: target } : { body: values }),
          }),
        upload
          ? "World uploaded. Start your server to explore it."
          : "World reset. Start your server to generate a new world.",
      )
    )
      setTarget(null);
  }
  return (
    <>
      <Feedback action={action} />
      <ErrorMessage error={resource.error || backups.error} />
      <section className="panel management-panel world-panel">
        <div className="section-heading">
          <div>
            <h2>Your world</h2>
            <span>{world?.name || "Loading…"}</span>
          </div>
          <button className="button secondary" onClick={resource.refresh}>
            Refresh world
          </button>
        </div>
        {resource.loading ? (
          <Loading />
        ) : (
          world && (
            <>
              <p className="field-help">
                {world.generated
                  ? `Your world includes ${world.directories.join(", ")}. Downloads include every dimension.`
                  : "Your world will be generated when you start the server."}
              </p>
              <div className="file-actions">
                <button
                  className="button secondary"
                  disabled={!offline || !world.generated || action.busy}
                  onClick={() =>
                    action.run(
                      () => download(api, `/servers/${id}/world/download`, `${world.name}.zip`),
                      "World download ready.",
                    )
                  }
                >
                  Download world
                </button>
              </div>
              <h3>Before changing your world</h3>
              <p className="field-help">
                Stop the server, then create a safety backup. Wait for it to finish before uploading
                or resetting. Starting the server or editing files means you need a new backup.
              </p>
              <button
                className="button secondary"
                disabled={
                  !offline ||
                  action.busy ||
                  !backups.data ||
                  backups.data.length >= limit ||
                  (safety && !safety.completed_at)
                }
                onClick={() =>
                  action.run(
                    () =>
                      api(`/servers/${id}/world/backup`, {
                        method: "POST",
                        key: crypto.randomUUID(),
                        body: {},
                      }),
                    "Safety backup started. Wait for it to finish.",
                  )
                }
              >
                Create safety backup
              </button>
              <p role="status">
                {safety && !safety.completed_at
                  ? "Creating safety backup…"
                  : ready
                    ? "Safety backup ready."
                    : safety
                      ? "Safety backup failed. Create a new backup."
                      : "A fresh safety backup is required."}
              </p>
              {backups.data?.length >= limit && (
                <p className="notice">
                  All {limit} backup slots are used.{" "}
                  <Link href={`/servers/${id}/backups`}>Manage backups</Link> to free a slot if you
                  need a new safety backup.
                </p>
              )}
              <h3>Upload or reset</h3>
              <p className="field-help">
                Upload a .zip up to 64 MB, containing level.dat at the root, or a “{world.name}”
                folder with level.dat inside. Paper archives may also include “{world.name}_nether”
                and “{world.name}_the_end”. Limit: 128 MB unpacked, 32 MB per file, and 2,000
                entries. Replacing a world removes dimensions absent from the upload.
              </p>
              <div className="file-actions">
                <button
                  className="button primary"
                  disabled={!offline || !ready || action.busy}
                  onClick={() => input.current.click()}
                >
                  Upload world .zip
                </button>
                <button
                  className="button danger-outline"
                  disabled={!offline || !ready || !world.generated || action.busy}
                  onClick={() => confirm("reset")}
                >
                  Reset world
                </button>
              </div>
              <input
                hidden
                ref={input}
                type="file"
                accept=".zip"
                onChange={(e) => {
                  if (e.target.files[0]) confirm(e.target.files[0]);
                  e.target.value = "";
                }}
              />
              <p className="field-help">
                Settings and plugins stay in place. You can recover your previous world by restoring
                its safety backup from Backups.
              </p>
            </>
          )
        )}
      </section>
      {target && (
        <Confirm
          title={target === "reset" ? "Reset your world?" : "Replace your world?"}
          label={target === "reset" ? "Reset world" : "Replace world"}
          busy={action.busy}
          confirmDisabled={confirmation !== world?.name || !offline || !ready}
          onCancel={() => setTarget(null)}
          onConfirm={change}
        >
          <p>
            {target === "reset"
              ? "All world dimensions will be reset. Minecraft generates a new world on the next start."
              : `“${target.name}” will replace your world and its dimensions.`}{" "}
            Your completed safety backup preserves the old files.
          </p>
          <label className="field">
            Type {world?.name} to confirm
            <input
              autoFocus
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
            />
          </label>
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
    </>
  );
}

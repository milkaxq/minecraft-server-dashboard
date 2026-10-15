"use client";
import { Confirm } from "./confirm";
export { Confirm } from "./confirm";
import { TextFileEditor } from "./text-file-editor";
import { visibleFiles, folderBreadcrumbs } from "../lib/file-browser";
import { UnsavedChangesGuard } from "./unsaved-changes";
import { installedAddonInfo, hasGuidedSetup, filterInstalledAddons } from "../lib/installed-addons";
import { AddonHelp } from "./addon-help";
import { ChangeSafety } from "./change-safety";
import { PluginCheck } from "./plugin-check";
import { Select } from "./select";
import { ClientRequirementTag } from "./client-compatibility";
import { BackupSchedule } from "./backup-schedule";
import { ProjectIcon } from "./project-icon";
import { supportsPlugins, supportsMods, softwareName } from "../lib/software";
import { Players } from "./players";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Table, Badge, ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
import { formatBytes, backupPreflightRejected } from "../lib/api";

export function FeatureLinks({ id, server, advanced = false, active = "" }) {
  const navigation = useRef(null);
  useEffect(() => {
    const nav = navigation.current;
    const current = nav?.querySelector('[aria-current="page"]');
    if (!current) return;
    const item = current.getBoundingClientRect();
    const bounds = nav.getBoundingClientRect();
    if (item.right > bounds.right) nav.scrollLeft += item.right - bounds.right + 8;
    else if (item.left < bounds.left) nav.scrollLeft -= bounds.left - item.left + 8;
  }, [id, active, advanced]);
  const permissions = server.permissions || [];
  const items = advanced
    ? [
        ["console", "Console", `/servers/${id}/console`],
        ["files", "Files", `/servers/${id}/files`],
      ]
    : [
        ["view", "Overview", `/servers/${id}`],
        ["settings", "Players", `/servers/${id}/players`],
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
    <nav ref={navigation} className="feature-tabs" aria-label="Server pages">
      {items
        .filter(([permission]) => permissions.includes(permission))
        .map(([key, label, href]) => (
          <Link
            href={href}
            key={href}
            aria-current={
              (label === "Players" ? active === "players" : active === key) ||
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
  return <ManagementContent key={id} id={id} section={section} />;
}
function ManagementContent({ id, section }) {
  const { api } = useAuth();
  const info = useResource(`/servers/${id}`, 10000);
  const server = info.data;
  const usage = useResource(
    server?.status === "RUNNING" && server.management_available && !server.deletion_pending
      ? `/servers/${id}/usage`
      : null,
    5000,
  );
  const [busy, setBusy] = useState("");
  const [error, setError] = useState(null);
  const [pending, setPending] = useState(null);
  const [notice, setNotice] = useState("");
  const inFlight = useRef(false);
  const attempt = useRef(null);
  const latestUsage = useRef(null);
  useEffect(() => {
    latestUsage.current = usage.data;
  }, [usage.data]);
  const ready =
    server?.status === "RUNNING" && server.management_available && !server.deletion_pending;
  const has = (action) => server?.permissions?.includes(action);
  const pendingActive =
    pending &&
    (pending.action === "start"
      ? usage.data?.power_state !== "running"
      : pending.action === "stop"
        ? usage.data?.power_state !== "offline"
        : !usage.data || usage.data === pending.reading);
  const transitioning = ["starting", "stopping"].includes(usage.data?.power_state);
  const blocked =
    !ready ||
    !!info.error ||
    !!busy ||
    !!usage.error ||
    usage.loading ||
    transitioning ||
    pendingActive;
  const offline = !blocked && usage.data?.power_state === "offline";
  const displayState = usage.error
    ? "UNKNOWN"
    : pendingActive && pending.action !== "restart"
      ? pending.action === "start"
        ? "starting"
        : "stopping"
      : usage.data?.power_state || server?.status;
  async function power(action) {
    if (
      inFlight.current ||
      blocked ||
      !has(action) ||
      (action === "start"
        ? usage.data?.power_state !== "offline"
        : usage.data?.power_state !== "running")
    )
      return;
    inFlight.current = true;
    setBusy(action);
    setError(null);
    setNotice("");
    if (attempt.current?.action !== action) attempt.current = { action, key: crypto.randomUUID() };
    try {
      await api(`/servers/${id}/${action}`, {
        method: "POST",
        body: {},
        key: attempt.current.key,
      });
      setPending({ action, reading: latestUsage.current });
      attempt.current = null;
      setNotice(`${action[0].toUpperCase() + action.slice(1)} requested.`);
      usage.refresh();
    } catch (e) {
      setError(e);
    } finally {
      inFlight.current = false;
      setBusy("");
    }
  }
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
            action={<Badge state={displayState} />}
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
              {["start", "stop", "restart"].filter(has).map((action) => (
                <button
                  key={action}
                  className={`button ${action === "start" ? "primary" : "secondary"}`}
                  disabled={
                    blocked ||
                    (action === "start" ? !offline : usage.data?.power_state !== "running")
                  }
                  onClick={() => power(action)}
                >
                  {busy === action || (pendingActive && pending.action === action)
                    ? { start: "Starting…", stop: "Stopping…", restart: "Restarting…" }[action]
                    : action[0].toUpperCase() + action.slice(1)}
                </button>
              ))}
            </div>
          </div>
          {(notice || usage.error) && (
            <div className="management-power-status">
              <p role="status">
                {pendingActive && pending.action === "stop"
                  ? "Saving and stopping…"
                  : pendingActive && pending.action === "start"
                    ? "Starting Minecraft…"
                    : notice || "Live status unavailable."}
              </p>
              <button
                className="button secondary"
                disabled={usage.loading || !!busy || usage.error?.status === 429}
                onClick={usage.refresh}
              >
                Check status
              </button>
            </div>
          )}
          <FeatureLinks id={id} server={server} advanced={section === "files"} active={section} />
          <ErrorMessage error={info.error || error} />
          {!server.permissions?.includes(
            section === "mods" ? "files" : section === "players" ? "settings" : section,
          ) ? (
            <div className="notice">Your server role does not allow this page.</div>
          ) : !ready ? (
            <div className="notice">
              Your server must be activated and connected before you can manage it.
            </div>
          ) : (
            <>
              <ErrorMessage error={usage.error} />
              {!offline && section !== "backups" && section !== "players" && (
                <p className="notice">
                  {usage.error || info.error ? (
                    "Check live status before making changes."
                  ) : (
                    <>Stop your server before changing {section}.</>
                  )}
                </p>
              )}
              {section === "files" ? (
                <Files id={id} offline={offline} />
              ) : section === "worlds" ? (
                <Worlds id={id} offline={offline} limit={server.backup_limit} />
              ) : section === "players" ? (
                <Players
                  id={id}
                  server={server}
                  usage={usage.data}
                  usageError={usage.error}
                  running={!usage.error && usage.data?.power_state === "running"}
                />
              ) : section === "settings" ? (
                <Settings id={id} offline={offline} />
              ) : section === "mods" ? (
                supportsMods(server.server_type) ? (
                  <Plugins id={id} server={server} offline={offline} mods usage={usage.data} />
                ) : (
                  <p className="notice">Mods require Fabric, Forge, or NeoForge.</p>
                )
              ) : section === "plugins" ? (
                supportsPlugins(server.server_type) ? (
                  <Plugins id={id} server={server} offline={offline} usage={usage.data} />
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
          {section === "backups" && server.permissions?.includes("backups") && (
            <BackupSchedule key={id} id={id} limit={server.backup_limit} />
          )}
        </>
      )}
    </Shell>
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
  const [search, setSearch] = useState("");
  const [order, setOrder] = useState("name");
  const resource = useResource(`/servers/${id}/files?path=${query(dir)}`);
  const action = useAction(resource.refresh);
  const entries = visibleFiles(resource.data || [], search, order);
  const [editor, setEditor] = useState(null);
  const [pendingEditorAction, setPendingEditorAction] = useState(null);
  const changeAttempt = useRef(null);
  const editorDirty = !!editor && editor.content !== editor.originalContent;
  const [create, setCreate] = useState(null);
  const [name, setName] = useState("");
  const [target, setTarget] = useState(null);
  const uploader = useRef(null);
  async function executeEditorAction(next) {
    if (action.busy) return;
    if (next.kind === "close") {
      setEditor(null);
      return;
    }
    if (next.kind === "folder") {
      setSearch("");
      setDir(next.path);
      setEditor(null);
      return;
    }
    await action.run(
      async () => {
        const file = await api(`/servers/${id}/files/text?path=${query(next.path)}`);
        setEditor({ ...file, originalContent: file.content });
      },
      next.kind === "reload" ? "File reloaded." : "",
    );
  }
  function requestEditorAction(next) {
    if (action.busy) return;
    if (editorDirty) setPendingEditorAction(next);
    else executeEditorAction(next);
  }
  function edit(entry) {
    requestEditorAction({ kind: "open", path: filePath(dir, entry.name) });
  }
  async function change(input, message) {
    const fingerprint = JSON.stringify([id, input]);
    if (changeAttempt.current?.fingerprint !== fingerprint)
      changeAttempt.current = { fingerprint, key: crypto.randomUUID() };
    const ok = await action.run(
      () =>
        api(`/servers/${id}/files`, {
          method: "POST",
          key: changeAttempt.current.key,
          body: input,
        }),
      message,
    );
    if (ok) {
      changeAttempt.current = null;
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
      <UnsavedChangesGuard dirty={editorDirty} />
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
        <nav className="file-breadcrumbs" aria-label="Folder location">
          {folderBreadcrumbs(dir).map((part, index) => (
            <span key={part.path}>
              {index > 0 && (
                <span aria-hidden="true" className="breadcrumb-divider">
                  /
                </span>
              )}
              {part.path === dir ? (
                <span aria-current="page">{part.name}</span>
              ) : (
                <button
                  type="button"
                  disabled={action.busy}
                  onClick={() => requestEditorAction({ kind: "folder", path: part.path })}
                >
                  {part.name}
                </button>
              )}
            </span>
          ))}
        </nav>
        <div className="file-path">
          <button
            disabled={dir === "/" || action.busy}
            onClick={() =>
              requestEditorAction({
                kind: "folder",
                path: dir.slice(0, dir.lastIndexOf("/")) || "/",
              })
            }
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
        <div className="file-browser-tools">
          <label>
            Find in this folder
            <input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search filenames…"
            />
          </label>
          <label>
            Sort files
            <Select
              aria-label="Sort files"
              value={order}
              onChange={(event) => setOrder(event.target.value)}
            >
              <option value="name">Name A–Z</option>
              <option value="name-desc">Name Z–A</option>
              <option value="size">Largest files first</option>
            </Select>
          </label>
        </div>
        <p className="field-help" aria-live="polite">
          {resource.data
            ? `Showing ${entries.length} of ${resource.data.length} items in this folder. Folders stay above files.`
            : "Search checks this folder only."}
        </p>
        {resource.loading ? (
          <Loading />
        ) : resource.error && !resource.data ? (
          <p className="notice">Couldn’t load this folder. Use Refresh to try again.</p>
        ) : !resource.data?.length ? (
          <p className="management-empty">This folder is empty.</p>
        ) : !entries.length ? (
          <p className="management-empty" role="status">
            No filenames match “{search}” in this folder.{" "}
            <button className="button secondary" onClick={() => setSearch("")}>
              Clear file search
            </button>
          </p>
        ) : (
          <div className="table-scroll">
            <Table className="management-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Size</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.name}>
                    <td>
                      <button
                        className={`file-name ${entry.is_file ? "document-name" : "folder-name"}`}
                        disabled={entry.is_symlink || action.busy}
                        onClick={() =>
                          entry.is_file
                            ? edit(entry)
                            : requestEditorAction({
                                kind: "folder",
                                path: filePath(dir, entry.name),
                              })
                        }
                      >
                        <Icon name={entry.is_file ? "file" : "folder"} size={20} />
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
            </Table>
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
            <button
              className="button secondary"
              disabled={action.busy}
              onClick={() => requestEditorAction({ kind: "close" })}
            >
              Close editor
            </button>
          </div>
          <p className="field-help" role="status">
            {editorDirty ? "Unsaved edits — save before leaving." : "No unsaved edits."}
          </p>
          {!offline && (
            <p className="notice">
              Stop your server before saving. You can prepare your edits now.
            </p>
          )}
          <TextFileEditor
            key={editor.path}
            value={editor.content}
            busy={action.busy}
            onChange={(content) => setEditor({ ...editor, content })}
          />
          <div className="file-actions">
            <button
              className="button secondary"
              disabled={action.busy}
              onClick={() => requestEditorAction({ kind: "reload", path: editor.path })}
            >
              Reload file
            </button>
            <button
              className="button primary"
              disabled={!offline || action.busy || !editorDirty}
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
              {action.busy ? "Saving…" : "Save file"}
            </button>
          </div>
        </section>
      )}
      {pendingEditorAction && (
        <Confirm
          title="Discard unsaved file edits?"
          label="Discard and continue"
          busy={action.busy}
          onCancel={() => setPendingEditorAction(null)}
          onConfirm={async () => {
            const next = pendingEditorAction;
            setPendingEditorAction(null);
            await executeEditorAction(next);
          }}
        >
          <p>
            Your edits to {editor?.path} have not been saved. Cancel to keep editing, or discard
            them to continue.
          </p>
        </Confirm>
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
  const saveAttempt = useRef(null);
  const form = draft?.source === resource.data ? draft.values : resource.data;
  const set = (key, value) =>
    setDraft({ source: resource.data, values: { ...form, [key]: value } });
  const labels = {
    motd: "Server message",
    difficulty: "Difficulty",
    gamemode: "Game mode",
    max_players: "Maximum players",
    view_distance: "View distance",
    spawn_protection: "Spawn protection",
    hardcore: "Hardcore",
    pvp: "Player vs. player",
    whitelist: "Whitelist",
    online_mode: "Verify Minecraft accounts",
  };
  const changes =
    form && resource.data
      ? Object.keys(labels).filter((key) => form[key] !== resource.data[key])
      : [];
  const display = (value) => (typeof value === "boolean" ? (value ? "On" : "Off") : String(value));
  const presets = [
    {
      name: "Survival adventure",
      help: "Gather resources and survive together.",
      values: { gamemode: "survival", difficulty: "normal", hardcore: false, pvp: true },
    },
    {
      name: "Peaceful survival",
      help: "Build and explore without hostile mobs.",
      values: { gamemode: "survival", difficulty: "peaceful", hardcore: false, pvp: false },
    },
    {
      name: "Creative building",
      help: "Build freely with unlimited resources.",
      values: { gamemode: "creative", difficulty: "peaceful", hardcore: false, pvp: false },
    },
  ];
  async function save(e) {
    e.preventDefault();
    if (!offline || action.busy || resource.error || !changes.length) return;
    const fingerprint = JSON.stringify([id, form]);
    if (saveAttempt.current?.fingerprint !== fingerprint)
      saveAttempt.current = { fingerprint, key: crypto.randomUUID() };
    if (
      await action.run(
        () =>
          api(`/servers/${id}/settings`, {
            method: "PUT",
            body: form,
            key: saveAttempt.current.key,
          }),
        "Settings saved. Start your server to apply them.",
      )
    ) {
      saveAttempt.current = null;
      setDraft(null);
    }
  }
  return (
    <>
      <UnsavedChangesGuard dirty={changes.length > 0} />
      <Feedback action={action} />
      <ErrorMessage error={resource.error} />
      <section className="panel management-panel">
        <div className="section-heading">
          <div>
            <h2>Minecraft settings</h2>
            <span>Changes apply on the next server start</span>
          </div>
          <button
            className="button secondary"
            disabled={action.busy || changes.length > 0}
            onClick={resource.refresh}
          >
            Reload settings
          </button>
        </div>
        {resource.loading ? (
          <Loading />
        ) : (
          form && (
            <form onSubmit={save} className="settings-form">
              <fieldset disabled={action.busy} className="settings-fields">
                <div className="gameplay-presets">
                  <h3>How do you want to play?</h3>
                  <p className="field-help">
                    Choose a starting point, then adjust anything below. Joining rules and player
                    limits stay as you set them.
                  </p>
                  <div className="gameplay-preset-grid">
                    {presets.map((preset) => (
                      <button
                        key={preset.name}
                        type="button"
                        className="button secondary gameplay-preset"
                        disabled={action.busy}
                        onClick={() =>
                          setDraft({ source: resource.data, values: { ...form, ...preset.values } })
                        }
                      >
                        <strong>{preset.name}</strong>
                        <span>{preset.help}</span>
                      </button>
                    ))}
                  </div>
                </div>
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
                    <Select
                      aria-label="Difficulty"
                      value={form.difficulty}
                      onChange={(e) => set("difficulty", e.target.value)}
                    >
                      {["peaceful", "easy", "normal", "hard"].map((v) => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <label className="field">
                    Game mode
                    <Select
                      aria-label="Game mode"
                      value={form.gamemode}
                      onChange={(e) => set("gamemode", e.target.value)}
                    >
                      {["survival", "creative", "adventure", "spectator"].map((v) => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </Select>
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
                      "Require a Microsoft account that owns Java Edition. No server password is needed unless a login plugin is installed.",
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
                    Keep it enabled unless you have configured an authentication plugin such as
                    AuthMe or a trusted proxy with its own authentication.
                  </p>
                )}
                <p className="field-help">
                  Your hosting resources stay allocated by your plan. More players or a larger view
                  distance can use more memory and CPU.
                </p>
                <section
                  className="settings-review"
                  aria-label="Review settings changes"
                  aria-live="polite"
                >
                  <h3>
                    {changes.length
                      ? `Review ${changes.length} change${changes.length === 1 ? "" : "s"}`
                      : "No unsaved changes"}
                  </h3>
                  {changes.length > 0 && (
                    <>
                      <ul>
                        {changes.map((key) => (
                          <li key={key}>
                            <strong>{labels[key]}:</strong> {display(resource.data[key])} →{" "}
                            {display(form[key])}
                          </li>
                        ))}
                      </ul>
                      <button
                        type="button"
                        className="button secondary"
                        disabled={action.busy}
                        onClick={() => setDraft(null)}
                      >
                        Discard changes
                      </button>
                    </>
                  )}
                  <p className="field-help">
                    {offline
                      ? "Save these changes, then start your server to apply them."
                      : "Stop your server before saving. Your edits are kept on this page."}{" "}
                    Game mode sets the default for new players; existing players may keep their
                    current mode.
                  </p>
                </section>
                <button
                  className="button primary"
                  disabled={!offline || action.busy || !!resource.error || !changes.length}
                >
                  {action.busy ? "Saving…" : "Save settings"}
                </button>
              </fieldset>
            </form>
          )
        )}
      </section>
    </>
  );
}
function Plugins({ id, server, offline, mods = false, usage }) {
  const collection = mods ? "mods" : "plugins";
  const label = mods ? "mod" : "plugin";
  const { api } = useAuth();
  const resource = useResource(`/servers/${id}/${collection}`);
  const clientSetup = useResource(`/servers/${id}/client-requirements`, 15000);
  const refreshClientSetup = clientSetup.refresh;
  useEffect(() => {
    refreshClientSetup();
  }, [resource.data, refreshClientSetup]);
  const action = useAction(resource.refresh);
  const input = useRef(null);
  const [installedQuery, setInstalledQuery] = useState("");
  const [guidedOnly, setGuidedOnly] = useState(false);
  const installedFiles = filterInstalledAddons(
    resource.data || [],
    installedQuery,
    guidedOnly,
    server.permissions,
  );
  const guidedCount = (resource.data || []).filter((file) =>
    hasGuidedSetup(file.name, server.permissions),
  ).length;
  const [remove, setRemove] = useState(null);
  const [addonChange, setAddonChange] = useState(null);
  const [pendingUpload, setPendingUpload] = useState(null);
  const [uploadSafety, setUploadSafety] = useState(null);
  const uploadAttempt = useRef(null);
  async function upload() {
    if (!pendingUpload || !uploadSafety) return;
    const file = pendingUpload;
    const params = new URLSearchParams({
      name: file.name,
      acknowledge_no_backup: String(uploadSafety.acknowledge_no_backup),
    });
    if (uploadSafety.backup) params.set("backup", uploadSafety.backup);
    const fingerprint = JSON.stringify([file.name, file.size, file.lastModified, uploadSafety]);
    if (uploadAttempt.current?.fingerprint !== fingerprint)
      uploadAttempt.current = { fingerprint, key: crypto.randomUUID() };
    const success = await action.run(
      async () => {
        try {
          return await api(`/servers/${id}/${collection}/upload?${params}`, {
            method: "POST",
            raw: true,
            body: file,
            key: uploadAttempt.current.key,
          });
        } catch (error) {
          if (backupPreflightRejected(error)) uploadAttempt.current = null;
          throw error;
        }
      },
      `${mods ? "Mod" : "Plugin"} uploaded. Start your server to load it.`,
    );
    if (success) {
      setPendingUpload(null);
      setAddonChange({ message: `${file.name} uploaded` });
    }
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
    ) {
      setAddonChange({ message: `${remove} removed` });
      setRemove(null);
    }
  }
  return (
    <>
      <Feedback action={action} />
      <ErrorMessage error={resource.error} />
      {addonChange && (
        <section className="panel addon-next-steps" aria-label="Next steps after addon changes">
          <span className="eyebrow">WHAT TO DO NEXT</span>
          <h2>{addonChange.message}</h2>
          <ol>
            <li>
              <strong>Configure if needed.</strong> Keep the defaults or adjust the addon before
              starting. Guided configuration is available for Simple Voice Chat in your installed
              addons.
            </li>
            <li>
              <strong>
                {usage?.power_state === "running"
                  ? "Check the addon loaded."
                  : "Start your server."}
              </strong>{" "}
              Minecraft loads the changed files on its next start.{" "}
              {usage?.power_state === "running"
                ? "File presence alone does not confirm it loaded; check the console or loaded plugins below."
                : "Your changed files are waiting for the next start."}
            </li>
            <li>
              <strong>Refresh player setup.</strong> Update your invitation so friends get the
              current required client mods. Server-only plugins stay on the server.
            </li>
          </ol>
          <div className="invite-actions">
            <Link className="button primary" href={`/servers/${id}`}>
              Go to server controls
            </Link>
            {server.permissions?.includes("members") && (
              <Link className="button secondary" href={`/servers/${id}#invite-friends`}>
                Refresh player invitation
              </Link>
            )}
            {server.permissions?.includes("files") && (
              <Link className="button secondary" href={`/servers/${id}/files`}>
                Configuration files
              </Link>
            )}
          </div>
        </section>
      )}
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
              setUploadSafety(null);
              uploadAttempt.current = null;
              setPendingUpload(e.target.files[0]);
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
        {!!resource.data?.length && (
          <div>
            <div
              className="installed-addon-filters"
              role="group"
              aria-label="Installed addon filters"
            >
              <button
                type="button"
                className={`button ${!guidedOnly ? "primary" : "secondary"}`}
                aria-pressed={!guidedOnly}
                onClick={() => setGuidedOnly(false)}
              >
                All installed ({resource.data.length})
              </button>
              <button
                type="button"
                className={`button ${guidedOnly ? "primary" : "secondary"}`}
                aria-pressed={guidedOnly}
                onClick={() => setGuidedOnly(true)}
              >
                With guided setup ({guidedCount})
              </button>
            </div>
            <label className="installed-addon-search">
              Find an installed {label}
              <input
                type="search"
                value={installedQuery}
                onChange={(event) => setInstalledQuery(event.target.value)}
                placeholder={`Search names or filenames…`}
              />
            </label>
            <p className="field-help" aria-live="polite">
              Showing {installedFiles.length} of {resource.data.length} installed {collection}.
            </p>
          </div>
        )}
        {resource.loading ? (
          <Loading />
        ) : !resource.data?.length ? (
          <p className="management-empty">No {collection} installed yet.</p>
        ) : !installedFiles.length ? (
          <p className="management-empty" role="status">
            {guidedOnly
              ? `No installed ${collection} match your guided setup filter${installedQuery ? ` and “${installedQuery}”` : ""}.`
              : `No installed ${collection} match “${installedQuery}”.`}{" "}
            <button
              className="button secondary"
              onClick={() => {
                setInstalledQuery("");
                setGuidedOnly(false);
              }}
            >
              {guidedOnly ? "Show all installed" : "Clear search"}
            </button>
          </p>
        ) : (
          <ul className="plugin-list">
            {installedFiles.map((v) => (
              <li key={v.name}>
                <span>
                  <strong>{installedAddonInfo(v.name).name}</strong>
                  {installedAddonInfo(v.name).name !== v.name && (
                    <small className="installed-addon-filename">{v.name}</small>
                  )}
                  <small>{formatBytes(v.size)} · File installed</small>
                  <div className="installed-addon-status" aria-label={`${v.name} status`}>
                    <span className="client-requirement-tag">Installed</span>
                    <span className="client-requirement-tag">
                      {offline
                        ? "Start to load"
                        : usage?.power_state === "running"
                          ? "Check load status"
                          : "Waiting for server status"}
                    </span>
                    {hasGuidedSetup(v.name, server.permissions) && (
                      <span className="client-requirement-tag">Guided configuration available</span>
                    )}
                  </div>
                  <AddonHelp
                    id={id}
                    file={v}
                    metadata={clientSetup.data?.addons?.find((addon) => addon.filename === v.name)}
                    server={server}
                    usage={usage}
                  />
                  <ClientRequirementTag
                    explain
                    addon={
                      clientSetup.data?.addons?.find((addon) => addon.filename === v.name) || {}
                    }
                  />
                </span>
                <div className="file-actions installed-addon-actions">
                  {server.permissions?.includes("settings") &&
                    v.name.toLowerCase().includes("voicechat") &&
                    v.name.toLowerCase().endsWith(".jar") && (
                      <Link
                        className="button primary"
                        href={`/servers/${id}/addons/simple-voice-chat`}
                      >
                        Configure
                      </Link>
                    )}
                  {server.permissions?.includes("files") &&
                    !v.name.toLowerCase().includes("voicechat") && (
                      <Link className="button secondary" href={`/servers/${id}/files`}>
                        Configuration files
                      </Link>
                    )}
                  <button
                    className="button danger-outline"
                    disabled={!offline || action.busy}
                    onClick={() => setRemove(v.name)}
                  >
                    Delete {label}
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
      <PluginCheck key={id} id={id} mods={mods} usage={usage} files={resource.data} />
      <CatalogBrowser
        key={`${id}:${server.server_type}:${server.minecraft_version}`}
        server={server}
        id={id}
        collection={collection}
        offline={offline}
        onInstalled={() => {
          resource.refresh();
          setAddonChange({ message: "Addon files installed" });
        }}
        files={resource.data}
      />
      {pendingUpload && (
        <Confirm
          title={`Review ${label} upload`}
          label={`Upload ${label}`}
          confirmVariant="primary"
          busy={action.busy}
          confirmDisabled={!offline || !uploadSafety}
          onCancel={() => setPendingUpload(null)}
          onConfirm={upload}
        >
          <p>
            {pendingUpload.name} · {formatBytes(pendingUpload.size)}. Only upload trusted addons for
            your server software and Minecraft version.
          </p>
          <ChangeSafety
            key={`${pendingUpload.name}:${pendingUpload.lastModified}`}
            server={server}
            offline={offline}
            busy={action.busy}
            onDecision={setUploadSafety}
          />
          <ErrorMessage error={action.error} />
        </Confirm>
      )}
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
function CatalogBrowser({ id, server, collection, offline, onInstalled, files }) {
  const { api } = useAuth();
  const [search, setSearch] = useState("");
  const [filters, setFilters] = useState({ query: "", sort: "downloads", category: "" });
  const [offset, setOffset] = useState(0);
  const filterKey = JSON.stringify([id, collection, filters]);
  const params = new URLSearchParams({
    q: filters.query,
    sort: filters.sort,
    category: filters.category,
    offset: String(offset),
  });
  const resource = useResource(`/servers/${id}/${collection}/catalog?${params}`);
  const [loaded, setLoaded] = useState({ filterKey: "", response: null, items: [] });
  let results = loaded.filterKey === filterKey ? loaded.items : [];
  if (resource.data && resource.data !== loaded.response) {
    const combined =
      offset === 0 ? resource.data.projects : [...results, ...resource.data.projects];
    results = [...new Map(combined.map((project) => [project.project_id, project])).values()];
    setLoaded({ filterKey, response: resource.data, items: results });
  }
  const [preview, setPreview] = useState(null);
  const [reviewProject, setReviewProject] = useState(null);
  const [operationStage, setOperationStage] = useState("review");
  const [checkUpdates, setCheckUpdates] = useState(false);
  const updates = useResource(checkUpdates ? `/servers/${id}/${collection}/catalog/updates` : null);
  const attempt = useRef(null);
  const [safety, setSafety] = useState(null);
  const installed = useResource(`/servers/${id}/${collection}/catalog/installed`);
  const action = useAction(() => {
    installed.refresh();
    if (checkUpdates) updates.refresh();
  });
  const [activeInstall, setActiveInstall] = useState(null);
  const progress = useResource(
    activeInstall ? `/servers/${id}/${collection}/catalog/operations/${activeInstall}` : null,
    750,
  );
  const [receipt, setReceipt] = useState(null);
  const present = new Set((files || []).map((file) => file.name));
  const installedProjects = new Set(installed.data?.projects || []);
  for (const file of receipt?.files || [])
    if (present.has(file.filename)) installedProjects.add(file.project_id);
  const refreshInstalled = installed.refresh;
  useEffect(() => {
    refreshInstalled();
  }, [files, refreshInstalled]);
  function changeFilters(next) {
    setFilters(next);
    setOffset(0);
    setPreview(null);
  }
  function browse(e) {
    e.preventDefault();
    changeFilters({ ...filters, query: search.trim() });
    resource.refresh();
  }
  function resetFilters() {
    setSearch("");
    changeFilters({ query: "", sort: "downloads", category: "" });
    resource.refresh();
  }
  async function review(project) {
    setOperationStage("review");
    setReviewProject(project);
    setPreview(null);
    await action.run(async () => {
      setPreview(await api(`/servers/${id}/${collection}/catalog/${query(project)}`));
      attempt.current = null;
      setSafety(null);
    });
  }
  async function install() {
    if (!safety || !offline || action.busy) return;
    setOperationStage("install");
    const body = { revision: preview.revision, ...safety };
    const fingerprint = JSON.stringify(body);
    if (attempt.current?.fingerprint !== fingerprint)
      attempt.current = { fingerprint, key: crypto.randomUUID() };
    const installKey = attempt.current.key;
    setActiveInstall(installKey);
    const success = await action.run(async () => {
      try {
        const result = await api(`/servers/${id}/${collection}/catalog/${query(preview.project)}`, {
          method: "POST",
          key: installKey,
          body,
        });
        setReceipt(result);
      } catch (error) {
        if (backupPreflightRejected(error)) attempt.current = null;
        throw error;
      }
    }, "Installed. Start your server to load the changes.");
    setActiveInstall(null);
    if (success) {
      onInstalled();
      setPreview(null);
      setReviewProject(null);
    }
  }
  const failureHelp = action.error && (
    <div className="notice addon-install-help" aria-label="Addon failure help">
      <strong>
        {operationStage === "install"
          ? "Installation needs attention"
          : "Could not prepare this addon"}
      </strong>
      <p>
        {action.error.status === 429
          ? "Too many requests. Wait before trying again."
          : action.error.status === 409
            ? "The server or its files changed. Check installed files, then review the current release again."
            : operationStage === "install"
              ? "Some files may have been copied before the error. Keep the server stopped and check installed files before retrying. Retrying the same review reuses the installation request."
              : "A matching release and all required dependencies must be available. Retry the review or choose another project. This review has not installed anything."}
      </p>
      {operationStage === "review" && !preview && reviewProject && action.error.status !== 429 && (
        <button
          className="button secondary"
          disabled={action.busy}
          onClick={() => review(reviewProject)}
        >
          Retry addon review
        </button>
      )}
      <Link className="button secondary" href={`/servers/${id}/files`}>
        Check installed files
      </Link>
    </div>
  );
  return (
    <section className="panel management-panel addon-catalog">
      <h2>Find {collection}</h2>
      <p className="catalog-compatibility">
        For {softwareName(server.server_type)} · Minecraft {server.minecraft_version} · Stable
        matching releases only
      </p>
      <Feedback action={action} />
      {!preview && failureHelp}
      <ErrorMessage error={resource.error || installed.error} />
      <section className="addon-updates">
        <button
          className="button secondary"
          disabled={updates.loading || action.busy}
          onClick={() => {
            setCheckUpdates(true);
            updates.refresh();
          }}
        >
          Check addon updates
        </button>
        <ErrorMessage error={updates.error} />
        {updates.loading && <p>Checking installed releases…</p>}
        {Array.isArray(updates.data) && (
          <ul className="plugin-list">
            {updates.data.map((item) => (
              <li key={item.project}>
                <div>
                  <strong>{item.title}</strong>
                  <small>
                    {item.current || "Recorded release"} → {item.latest || "Not verified"}
                  </small>
                  <p>
                    {
                      {
                        current: "Latest compatible release installed",
                        available: "Compatible update available",
                        unavailable: "No installable release could be verified",
                        unchecked: "Not checked — check this project individually",
                      }[item.status]
                    }
                  </p>
                </div>
                {item.status === "available" && (
                  <button
                    className="button secondary"
                    disabled={action.busy}
                    onClick={() => review(item.project)}
                  >
                    Review update
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {Array.isArray(updates.data) && updates.data.length === 0 && (
          <p>No catalog-tracked addons found. Uploaded JARs are not checked automatically.</p>
        )}
      </section>
      <form onSubmit={browse} className="catalog-controls">
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
        <label>
          Sort catalog
          <Select
            aria-label="Sort catalog"
            value={filters.sort}
            onChange={(e) => changeFilters({ ...filters, sort: e.target.value })}
            disabled={action.busy}
          >
            <option value="downloads">Most downloaded</option>
            <option value="relevance">Relevance</option>
            <option value="newest">Newest</option>
            <option value="updated">Recently updated</option>
          </Select>
        </label>
        <label>
          Category filter
          <Select
            aria-label="Category filter"
            value={filters.category}
            onChange={(e) => changeFilters({ ...filters, category: e.target.value })}
            disabled={action.busy}
          >
            <option value="">All categories</option>
            {[
              ["utility", "Utilities"],
              ["management", "Server management"],
              ["optimization", "Performance"],
              ["economy", "Economy"],
              ["adventure", "Adventure"],
              ["worldgen", "World generation"],
            ].map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
        </label>
        <div className="catalog-filter-actions">
          <button className="button primary" disabled={action.busy}>
            Search Modrinth
          </button>
          <button
            type="button"
            className="button secondary"
            onClick={resetFilters}
            disabled={action.busy}
          >
            Reset filters
          </button>
        </div>
      </form>
      {resource.loading && <Loading message="Checking compatible releases…" />}
      {results.length > 0 && (
        <p className="field-help" role="status">
          {results.length} compatible {results.length === 1 ? "project" : "projects"} shown
        </p>
      )}
      {results && (
        <ul className="plugin-list">
          {results.map((project) => (
            <li key={project.project_id}>
              <div className="catalog-project">
                <ProjectIcon project={project} />
                <span>
                  <strong>{project.title}</strong>
                  <small>{project.description}</small>
                  <span className="client-requirement-tag client-requirement-server">
                    Matches your server
                  </span>
                  <ClientRequirementTag addon={project} explain />
                  <small>
                    {project.downloads?.toLocaleString() || "0"} downloads
                    {project.release_version ? ` · Release ${project.release_version}` : ""}
                  </small>
                  <a
                    href={`https://modrinth.com/project/${encodeURIComponent(project.project_id)}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Project details
                  </a>
                </span>
              </div>
              <div className="file-actions catalog-project-actions">
                {installedProjects.has(project.project_id) &&
                  project.configuration_id &&
                  server.permissions?.includes("settings") && (
                    <Link
                      className="button primary"
                      href={`/servers/${id}/addons/${encodeURIComponent(project.configuration_id)}`}
                    >
                      Configure
                    </Link>
                  )}
                <button
                  className="button"
                  disabled={action.busy || installedProjects.has(project.project_id)}
                  onClick={() => review(project.project_id)}
                >
                  {installedProjects.has(project.project_id) ? "File installed" : "Review install"}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {!resource.loading && !resource.error && results.length === 0 && (
        <p>
          {resource.data?.has_more
            ? "No matching stable releases on this page. Load more to continue checking."
            : "No compatible projects found. Try another search or reset your filters."}
        </p>
      )}
      {resource.data?.has_more && !resource.error && (
        <button
          className="button secondary"
          disabled={action.busy || resource.loading}
          onClick={() => setOffset(resource.data.next_offset)}
        >
          Load more
        </button>
      )}
      {resource.error && (
        <button className="button secondary" onClick={resource.refresh}>
          Retry catalog load
        </button>
      )}
      {preview && (
        <Confirm
          confirmVariant="primary"
          title={preview.replaces?.length ? "Review addon update" : "Review installation"}
          label={preview.replaces?.length ? "Update now" : "Install now"}
          busy={action.busy}
          confirmDisabled={!offline || !safety}
          onCancel={() => setPreview(null)}
          onConfirm={install}
        >
          <p>
            For {preview.software} {preview.minecraft_version}. Required dependencies are included.
            Catalog-tracked files listed below can be replaced after their contents are verified.
            Untracked or modified files are not automatically removed. Stop the server and read the
            project’s release notes first.
          </p>
          <h3>
            {preview.files.length} {preview.files.length === 1 ? "file" : "files"} to install
          </h3>
          <p className="field-help">
            Includes the addon and required server dependencies. Client downloads are prepared
            separately in your player invitation.
          </p>
          <ul>
            {preview.files.map((file) => (
              <li key={file.version_id}>
                {file.filename} · {formatBytes(file.size)}
                <ClientRequirementTag addon={file} explain />
              </li>
            ))}
          </ul>
          {preview.replaces?.length > 0 && (
            <div className="notice">
              <strong>Files to replace</strong>
              <ul>
                {preview.replaces.map((file) => (
                  <li key={file.filename}>
                    {file.filename} · {file.name}
                  </li>
                ))}
              </ul>
              <p>
                Keep a backup to recover the original files if the update fails. File replacement is
                not atomic; an interrupted update needs inspection before restarting.
              </p>
            </div>
          )}
          <ChangeSafety
            key={`${preview.project}:${preview.revision}`}
            server={server}
            offline={offline}
            busy={action.busy}
            onDecision={setSafety}
          />
          {activeInstall && (
            <div className="installation-progress" role="status">
              <strong>{progress.data?.phase || "Preparing installation…"}</strong>
              <progress
                aria-label="Installation progress"
                max={progress.data?.total || undefined}
                value={progress.data?.total ? progress.data.completed : undefined}
              />
              <small>
                {progress.data?.total
                  ? `${progress.data.completed} of ${progress.data.total} file steps completed`
                  : "Checking your server and compatible releases"}
              </small>
              {progress.error && progress.error.status !== 404 && (
                <p>Progress updates interrupted. Waiting for the installation result…</p>
              )}
            </div>
          )}
          {!offline && <p>Stop your server before installing.</p>}
          {failureHelp}
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
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60000);
    return () => clearInterval(timer);
  }, []);
  const [target, setTarget] = useState(null);
  const [restoreAcknowledged, setRestoreAcknowledged] = useState(false);
  const usable = (resource.data || []).filter(
    (backup) => backup.is_successful && backup.completed_at,
  );
  const latest = [...usable].sort((a, b) => new Date(b.created_at) - new Date(a.created_at))[0];
  const oldCopy = latest && now - Date.parse(latest.created_at) > 48 * 60 * 60 * 1000;
  const pending = (resource.data || []).filter((backup) => !backup.completed_at).length;
  const historyAvailable = !resource.loading && !resource.error && !!resource.data;
  const selectedAvailable =
    historyAvailable &&
    target &&
    resource.data.some((backup) => backup.uuid === target.backup.uuid && backup.completed_at);
  const selectedReady =
    target &&
    !resource.error &&
    resource.data?.some(
      (backup) => backup.uuid === target.backup.uuid && backup.is_successful && backup.completed_at,
    );

  async function create(e) {
    e.preventDefault();
    if (!historyAvailable || resource.data.length >= limit || !name.trim()) return;
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
    if (!selectedAvailable) return;
    if (target?.action === "restore" && (!offline || !restoreAcknowledged || !selectedReady))
      return;
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
      <section className="panel backup-protection" aria-label="Backup protection">
        <span className="eyebrow">PROTECT YOUR WORLD</span>
        <h2>
          {resource.loading
            ? "Checking your backups…"
            : resource.error
              ? "Backup status unavailable"
              : latest
                ? "You have a recovery copy"
                : pending
                  ? "Your first recovery copy is being created"
                  : "No usable recovery copy yet"}
        </h2>
        <p>
          {resource.error
            ? "We could not check backup history. Refresh it before relying on a recovery copy."
            : latest
              ? `${usable.length} completed ${usable.length === 1 ? "backup is" : "backups are"} available. Your newest usable copy is “${latest.name}”. Its backup started on ${new Date(latest.created_at).toLocaleString()}.`
              : pending
                ? "Wait for a backup to finish successfully before relying on it."
                : "Create a backup before changing your world, installing add-ons or updating Minecraft."}
        </p>
        {oldCopy && !resource.error && (
          <p className="notice">
            Your latest backup is over two days old. Create a fresh copy before changes.
          </p>
        )}
        {latest && !resource.error && (
          <div className="backup-summary-actions">
            <button
              className="button secondary"
              disabled={action.busy}
              onClick={() =>
                action.run(
                  () =>
                    download(
                      api,
                      `/servers/${id}/backups/${latest.uuid}/download`,
                      `backup-${latest.uuid}.tar.gz`,
                    ),
                  "Download ready.",
                )
              }
            >
              Download latest copy
            </button>
            <button
              className="button secondary"
              disabled={!offline || action.busy}
              onClick={() => {
                setRestoreAcknowledged(false);
                setTarget({ action: "restore", backup: latest });
              }}
            >
              Restore latest copy
            </button>
          </div>
        )}
      </section>
      <section className="panel management-panel">
        <div className="section-heading">
          <div>
            <h2>Backups</h2>
            <span>
              {historyAvailable ? resource.data.length : "—"} of {limit} backup slots used
            </span>
          </div>
          <button className="button secondary" onClick={resource.refresh}>
            Refresh
          </button>
        </div>
        <p className="field-help">Stop the server first for a consistent backup.</p>
        {limit <= 0 && (
          <p className="notice">
            This server’s plan has no backup slots. Ask your host about backup access before making
            changes you want to recover from.
          </p>
        )}
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
              action.busy || !name.trim() || !historyAvailable || resource.data.length >= limit
            }
          >
            Create backup
          </button>
        </form>
        {limit > 0 && resource.data && resource.data.length >= limit && (
          <p className="notice">
            Your backup slots are full. Download any copy you want to keep before deleting it to
            free a slot. Backups are not removed automatically by this page.
          </p>
        )}
        {resource.error && (
          <p className="notice" role="status">
            Couldn’t refresh backup history. Refresh to try again. Backup actions are paused until
            history loads successfully.
            {resource.data && " Entries below are from the last successful check."}
          </p>
        )}
        {resource.loading ? (
          <Loading />
        ) : !resource.data?.length && resource.error ? null : !resource.data?.length ? (
          <p className="management-empty">
            No backups yet. Save a copy of your world before making big changes.
          </p>
        ) : (
          <div className="table-scroll">
            <Table className="management-table">
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
                      {b.uuid === latest?.uuid && (
                        <small className="backup-latest-label">Newest usable copy</small>
                      )}
                      <small>{b.automatic ? "Automatic" : "Manual"}</small>
                      <small>{new Date(b.created_at).toLocaleString()}</small>
                    </td>
                    <td>{!b.completed_at ? "Creating…" : b.is_successful ? "Ready" : "Failed"}</td>
                    <td>{b.completed_at ? formatBytes(b.bytes) : "—"}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          disabled={
                            !historyAvailable || action.busy || !b.is_successful || !b.completed_at
                          }
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
                          disabled={
                            !historyAvailable ||
                            !offline ||
                            action.busy ||
                            !b.is_successful ||
                            !b.completed_at
                          }
                          onClick={() => {
                            setRestoreAcknowledged(false);
                            setTarget({ action: "restore", backup: b });
                          }}
                        >
                          Restore
                        </button>
                        <button
                          className="text-danger"
                          disabled={!historyAvailable || action.busy || !b.completed_at}
                          onClick={() => setTarget({ action: "delete", backup: b })}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
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
          confirmDisabled={
            !selectedAvailable ||
            (target.action === "restore" && (!offline || !selectedReady || !restoreAcknowledged))
          }
          onCancel={() => setTarget(null)}
          onConfirm={change}
        >
          <p>
            {target.action === "restore"
              ? `Restoring “${target.backup.name}” replaces all current server files, including your world, settings and plugins. Changes made after this backup will be lost.`
              : `“${target.backup.name}” will be permanently deleted.`}
          </p>
          {!selectedAvailable && target.action === "delete" && (
            <p className="notice">
              This backup could not be verified. Close this dialog and refresh history before
              deleting.
            </p>
          )}
          {target.action === "delete" &&
            usable.length === 1 &&
            latest?.uuid === target.backup.uuid && (
              <p className="notice">
                This is your only completed recovery copy. Download it before deleting if you want
                to keep a way back.
              </p>
            )}
          {target.action === "restore" && (
            <>
              <p>
                Selected backup: <strong>{target.backup.name}</strong> · created{" "}
                {new Date(target.backup.created_at).toLocaleString()}.
              </p>
              <p>
                First download or create a backup of the current files if you want to keep them.
                Files excluded from the selected backup cannot be recovered by this restore.
              </p>
              {!offline && (
                <p className="notice">Stop the server and wait for Offline before restoring.</p>
              )}
              {!selectedReady && (
                <p className="notice">
                  This backup is no longer available for restore. Close this dialog and refresh the
                  history.
                </p>
              )}
              <label className="restore-acknowledgement">
                <input
                  type="checkbox"
                  checked={restoreAcknowledged}
                  disabled={action.busy}
                  onChange={(event) => setRestoreAcknowledged(event.target.checked)}
                />
                I understand that the current server files will be replaced.
              </label>
            </>
          )}
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
  const inFlight = useRef(false);
  const attempts = useRef(new Map());
  const [pending, setPending] = useState("");
  const unavailable = resource.loading || !!resource.error || !resource.data;
  const backupsUnavailable = backups.loading || !!backups.error || !backups.data;
  const blocked = !offline || unavailable || action.busy;
  async function runWorld(kind, task, message) {
    if (inFlight.current || blocked || (kind !== "download" && backupsUnavailable)) return false;
    inFlight.current = true;
    setPending(kind);
    try {
      return await action.run(task, message);
    } finally {
      inFlight.current = false;
      setPending("");
    }
  }
  async function mutate(kind, values, file) {
    const fingerprint = JSON.stringify({
      kind,
      confirmation: values.confirmation,
      file: file ? [file.name, file.size, file.lastModified] : null,
    });
    if (!attempts.current.has(fingerprint) || attempts.current.get(fingerprint).file !== file)
      attempts.current.set(fingerprint, { key: crypto.randomUUID(), values, file });
    const attempt = attempts.current.get(fingerprint);
    const endpoint = file
      ? `/servers/${id}/world/upload?${new URLSearchParams(attempt.values)}`
      : `/servers/${id}/world/${kind}`;
    try {
      await api(endpoint, {
        method: "POST",
        key: attempt.key,
        ...(file ? { raw: true, body: file } : { body: attempt.values }),
      });
      attempts.current.delete(fingerprint);
    } catch (error) {
      if ([400, 403, 404, 409, 413, 422].includes(error.status))
        attempts.current.delete(fingerprint);
      throw error;
    }
  }
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
    if (blocked || backupsUnavailable || !ready || confirmation !== world?.name || !target) return;
    const values = { backup: safety.uuid, revision: world.revision, confirmation };
    const upload = target instanceof File;
    if (
      await runWorld(
        upload ? "upload" : "reset",
        () => mutate(upload ? "upload" : "reset", values, upload ? target : null),
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
            <span>{world?.name || (resource.loading ? "Loading…" : "World unavailable")}</span>
          </div>
          <button
            className="button secondary"
            disabled={
              action.busy ||
              resource.loading ||
              backups.loading ||
              [resource.error, backups.error].some((error) => error?.status === 429)
            }
            onClick={() => {
              resource.refresh();
              backups.refresh();
            }}
          >
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
                  ? "Download includes every dimension."
                  : "Your world will be generated when you start the server."}
              </p>
              <div className="file-actions">
                <button
                  className="button secondary"
                  disabled={blocked || !world.generated}
                  onClick={() =>
                    runWorld(
                      "download",
                      () => download(api, `/servers/${id}/world/download`, `${world.name}.zip`),
                      "World download ready.",
                    )
                  }
                >
                  {pending === "download" ? "Preparing download…" : "Download world"}
                </button>
              </div>
              <h3>Change your world</h3>
              <p className="field-help">
                Stop Minecraft, then create a safety backup to unlock changes.
              </p>
              <button
                className="button secondary"
                disabled={
                  blocked ||
                  backupsUnavailable ||
                  backups.data.length >= limit ||
                  (safety && !safety.completed_at)
                }
                onClick={() =>
                  runWorld(
                    "backup",
                    () => mutate("backup", {}),
                    "Safety backup started. Wait for it to finish.",
                  )
                }
              >
                {pending === "backup" ? "Starting backup…" : "Create safety backup"}
              </button>
              <p role="status">
                {resource.error || backups.error
                  ? "Safety backup unavailable. Refresh world to check again."
                  : backupsUnavailable || unavailable
                    ? "Checking safety backup…"
                    : safety && !safety.completed_at
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
              <p className="field-help">Upload a Minecraft world ZIP, up to 64 MB.</p>
              <div className="file-actions">
                <button
                  className="button primary"
                  disabled={blocked || backupsUnavailable || !ready}
                  onClick={() => input.current.click()}
                >
                  Upload world .zip
                </button>
                <button
                  className="button danger-outline"
                  disabled={blocked || backupsUnavailable || !ready || !world.generated}
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
            </>
          )
        )}
      </section>
      {target && (
        <Confirm
          title={target === "reset" ? "Reset your world?" : "Replace your world?"}
          label={target === "reset" ? "Reset world" : "Replace world"}
          busy={action.busy}
          confirmDisabled={confirmation !== world?.name || blocked || backupsUnavailable || !ready}
          onCancel={() => setTarget(null)}
          onConfirm={change}
        >
          <p>
            {target === "reset"
              ? "All world dimensions will be reset. Minecraft generates a new world on the next start."
              : `“${target.name}” will replace your world and its dimensions.`}{" "}
            Your completed safety backup preserves the old files.
          </p>
          {target instanceof File && (
            <p className="field-help">
              ZIP must contain level.dat at the root or inside “{world?.name}”. Limit: 128 MB
              unpacked, 32 MB per file, 2,000 entries. Dimensions absent from the upload are
              removed. Settings and plugins stay.
            </p>
          )}
          <label className="field">
            Type {world?.name} to confirm
            <input
              autoFocus
              disabled={action.busy}
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

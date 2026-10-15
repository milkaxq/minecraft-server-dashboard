"use client";
import { Select } from "./select";
import Link from "next/link";
import { useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { FeatureLinks } from "./management";
import { Confirm } from "./confirm";
import { ErrorMessage, Loading, PageHeading, Shell } from "./ui";

export function MembersPage({ id }) {
  return <MembersContent key={id} id={id} />;
}
function roleHelp(role) {
  return role === "ADMIN"
    ? "Manage settings, files, plugins, worlds and backups, plus power and console."
    : "View status, use the console, start and restart Minecraft.";
}
function MembersContent({ id }) {
  const { api } = useAuth();
  const info = useResource(`/servers/${id}`, 10000);
  const allowed = info.data?.permissions?.includes("members");
  const resource = useResource(allowed ? `/servers/${id}/members` : null, 10000);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("MODERATOR");
  const [remove, setRemove] = useState(null);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState("");
  const inFlight = useRef(false);
  const attempts = useRef(new Map());
  const unavailable =
    !allowed ||
    info.loading ||
    !!info.error ||
    resource.loading ||
    !!resource.error ||
    !resource.data;
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  async function change(path, method, body, message) {
    if (inFlight.current || unavailable) return false;
    inFlight.current = true;
    const fingerprint = JSON.stringify({ path, method, body });
    if (!attempts.current.has(fingerprint)) attempts.current.set(fingerprint, crypto.randomUUID());
    setPending(method === "POST" ? "add" : path);
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      await api(path, { method, body, key: attempts.current.get(fingerprint) });
      attempts.current.delete(fingerprint);
      resource.refresh();
      setNotice(message);
      return true;
    } catch (e) {
      if ([400, 403, 404, 409, 422].includes(e.status)) attempts.current.delete(fingerprint);
      setError(e);
      return false;
    } finally {
      inFlight.current = false;
      setPending("");
      setBusy(false);
    }
  }
  async function add(event) {
    event.preventDefault();
    if (
      await change(
        `/servers/${id}/members`,
        "POST",
        { email: email.trim(), role },
        "Member added. They can now open this server in their dashboard.",
      )
    )
      setEmail("");
  }
  return (
    <Shell>
      {(info.error || resource.error) && (
        <button
          className="button secondary"
          disabled={
            busy ||
            info.loading ||
            resource.loading ||
            [info.error, resource.error].some((error) => error?.status === 429)
          }
          onClick={() => {
            info.refresh();
            resource.refresh();
          }}
        >
          Refresh members
        </button>
      )}
      {info.loading ? (
        <Loading />
      ) : !info.data ? (
        <ErrorMessage error={info.error} />
      ) : (
        <>
          <Link href={`/servers/${id}`} className="back-link">
            ← Server overview
          </Link>
          <PageHeading
            eyebrow="SERVER ACCESS"
            title="Members"
            description={`Choose who can help manage ${info.data.name}.`}
          />
          <FeatureLinks id={id} server={info.data} active="members" />
          <ErrorMessage error={info.error || resource.error || error} />
          {busy && pending !== "add" && (
            <p className="notice" role="status">
              Updating access…
            </p>
          )}
          {notice && (
            <p className="notice success" role="status">
              {notice}
            </p>
          )}
          {!allowed ? (
            <p className="notice">Only the server owner can manage members.</p>
          ) : (
            <section className="panel management-panel">
              <h2>Share server access</h2>
              <p className="field-help">
                Add someone with a verified Blockyard account. Billing and ownership stay with you.
              </p>
              <form className="backup-create" onSubmit={add}>
                <label className="field">
                  Member email
                  <input
                    type="email"
                    autoComplete="email"
                    disabled={busy}
                    required
                    maxLength={254}
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="friend@example.com"
                  />
                </label>
                <label className="field">
                  Server role
                  <Select
                    aria-label="Server role"
                    aria-describedby="member-role-help"
                    disabled={busy}
                    value={role}
                    onChange={(e) => setRole(e.target.value)}
                  >
                    <option value="MODERATOR">Moderator</option>
                    <option value="ADMIN">Admin</option>
                  </Select>
                </label>
                <button
                  className="button primary"
                  disabled={
                    busy ||
                    unavailable ||
                    resource.data.filter((member) => member.role !== "OWNER").length >= 20
                  }
                >
                  {pending === "add" ? "Adding…" : "Add member"}
                </button>
              </form>
              <p id="member-role-help" className="field-help">
                {roleHelp(role)}
              </p>
              {resource.loading ? (
                <Loading />
              ) : resource.data?.length ? (
                <ul className="member-list" aria-label="Server members">
                  {resource.data.map((member) => (
                    <li key={member.user_id}>
                      <div className="member-person">
                        <strong>{member.name}</strong>
                        <span>{member.email}</span>
                        <small>
                          {member.role === "OWNER"
                            ? "Full access · billing and ownership"
                            : roleHelp(member.role)}
                        </small>
                      </div>
                      {member.role === "OWNER" ? (
                        <span className="billing-status">Owner</span>
                      ) : (
                        <div className="member-controls">
                          <Select
                            aria-label={"Role for " + member.name}
                            value={member.role}
                            disabled={busy || unavailable}
                            onChange={(event) =>
                              change(
                                "/servers/" + id + "/members/" + member.user_id,
                                "PUT",
                                { role: event.target.value },
                                "Member role updated.",
                              )
                            }
                          >
                            <option value="MODERATOR">Moderator</option>
                            <option value="ADMIN">Admin</option>
                          </Select>
                          <button
                            className="button danger-outline"
                            disabled={busy || unavailable}
                            onClick={() => setRemove(member)}
                          >
                            Remove member
                          </button>
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="management-empty">
                  {resource.error ? "Member list unavailable." : "No teammates added yet."}
                </p>
              )}
            </section>
          )}
          {remove && (
            <Confirm
              title="Remove this member?"
              label="Remove member"
              busy={busy}
              confirmDisabled={unavailable}
              onCancel={() => setRemove(null)}
              onConfirm={async () => {
                if (
                  await change(
                    `/servers/${id}/members/${remove.user_id}`,
                    "DELETE",
                    { confirmation: remove.user_id },
                    "Member removed.",
                  )
                )
                  setRemove(null);
              }}
            >
              <p>
                {remove.name} ({remove.email}) will lose access to this server. Their account and
                your server files remain in place. An open console may take up to 15 seconds to
                disconnect.
              </p>
              <ErrorMessage error={error} />
            </Confirm>
          )}
        </>
      )}
    </Shell>
  );
}

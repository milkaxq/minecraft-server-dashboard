"use client";
import Link from "next/link";
import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Confirm, FeatureLinks } from "./management";
import { ErrorMessage, Loading, PageHeading, Shell } from "./ui";

export function MembersPage({ id }) {
  const { api } = useAuth();
  const info = useResource(`/servers/${id}`, 10000);
  const allowed = info.data?.permissions?.includes("members");
  const resource = useResource(allowed ? `/servers/${id}/members` : null, 10000);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("MODERATOR");
  const [remove, setRemove] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  async function change(path, method, body, message) {
    if (busy) return false;
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      await api(path, { method, body, key: crypto.randomUUID() });
      resource.refresh();
      setNotice(message);
      return true;
    } catch (e) {
      setError(e);
      return false;
    } finally {
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
        "Member added. Their server will appear in their dashboard.",
      )
    )
      setEmail("");
  }
  return (
    <Shell>
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
                Ask your friend to register and verify their email first. Members sign in to their
                own account. Up to 20 members can be added.
              </p>
              <p className="field-help">
                <strong>Admin:</strong> console, files, settings, worlds, plugins, backups and power
                controls. <strong>Moderator:</strong> console, status, start and restart. Billing,
                member management and ownership stay with you.
              </p>
              <form className="backup-create" onSubmit={add}>
                <label className="field">
                  Member email
                  <input
                    type="email"
                    required
                    maxLength={254}
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="friend@example.com"
                  />
                </label>
                <label className="field">
                  Server role
                  <select
                    aria-label="Server role"
                    value={role}
                    onChange={(e) => setRole(e.target.value)}
                  >
                    <option value="MODERATOR">Moderator</option>
                    <option value="ADMIN">Admin</option>
                  </select>
                </label>
                <button
                  className="button primary"
                  disabled={busy || !resource.data || resource.data.length >= 21}
                >
                  Add member
                </button>
              </form>
              {resource.loading ? (
                <Loading />
              ) : (
                <div className="table-scroll">
                  <table className="management-table">
                    <thead>
                      <tr>
                        <th>Person</th>
                        <th>Role</th>
                        <th>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {resource.data?.map((member) => (
                        <tr key={member.user_id}>
                          <td>
                            <strong>{member.name}</strong>
                            <small>{member.email}</small>
                          </td>
                          <td>
                            {member.role === "OWNER" ? (
                              "Owner"
                            ) : (
                              <select
                                aria-label={`Role for ${member.name}`}
                                value={member.role}
                                disabled={busy}
                                onChange={(e) =>
                                  change(
                                    `/servers/${id}/members/${member.user_id}`,
                                    "PUT",
                                    { role: e.target.value },
                                    "Member role updated.",
                                  )
                                }
                              >
                                <option value="MODERATOR">Moderator</option>
                                <option value="ADMIN">Admin</option>
                              </select>
                            )}
                          </td>
                          <td>
                            {member.role === "OWNER" ? (
                              "Account owner"
                            ) : (
                              <button
                                className="button danger-outline"
                                disabled={busy}
                                onClick={() => setRemove(member)}
                              >
                                Remove member
                              </button>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <p className="field-help">
                Removing a member blocks their next API request. An already connected console closes
                at its next access check, normally within 15 seconds. In-progress operations finish
                before access changes.
              </p>
            </section>
          )}
          {remove && (
            <Confirm
              title="Remove this member?"
              label="Remove member"
              busy={busy}
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
                your server files remain in place.
              </p>
              <ErrorMessage error={error} />
            </Confirm>
          )}
        </>
      )}
    </Shell>
  );
}

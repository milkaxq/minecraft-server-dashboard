"use client";
import { useState } from "react";
import { useAuth } from "./auth";
import { ErrorMessage, Loading } from "./ui";
export function Invitations({ rows, loading, refresh }) {
  const { api } = useAuth();
  const [email, setEmail] = useState("");
  const [issued, setIssued] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  async function create(e) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    setIssued(null);
    setNotice("");
    try {
      setIssued(await api("/admin/invitations", { method: "POST", body: { email: email.trim() } }));
      setEmail("");
      refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function revoke(id) {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await api(`/admin/invitations/${id}/revoke`, { method: "POST", body: {} });
      if (issued?.id === id) setIssued(null);
      setNotice("Invitation revoked.");
      refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(issued.link);
      setNotice("Invitation link copied. Share it privately with this person.");
    } catch {
      setNotice("Select the invitation link below and copy it manually.");
    }
  }
  return (
    <section className="panel management-panel">
      <h2>Invite beta testers</h2>
      <p className="field-help">
        Each invitation works once, expires after seven days and only accepts the specified email.
        Send the link privately; no invitation email is sent automatically.
      </p>
      <ErrorMessage error={error} />
      {notice && (
        <p role="status" className="notice success">
          {notice}
        </p>
      )}
      <form className="backup-create" onSubmit={create}>
        <label className="field">
          Invite email
          <input
            type="email"
            required
            maxLength={254}
            autoComplete="off"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <button className="button primary" disabled={busy}>
          Create invitation
        </button>
      </form>
      {issued && (
        <div className="notice">
          <strong>Invitation for {issued.email}</strong>
          <p>This link is shown only now. Keep it private.</p>
          <label className="field">
            Invitation link
            <input readOnly value={issued.link} onFocus={(e) => e.target.select()} />
          </label>
          <button className="button secondary" onClick={copy}>
            Copy invitation link
          </button>
        </div>
      )}
      {loading ? (
        <Loading />
      ) : !rows.length ? (
        <p className="management-empty">No invitations yet.</p>
      ) : (
        <div className="table-scroll">
          <table className="management-table">
            <thead>
              <tr>
                <th>Email</th>
                <th>Status</th>
                <th>Expires</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.id}>
                  <td>{row.email}</td>
                  <td>{row.status}</td>
                  <td>{new Date(row.expires_at).toLocaleDateString()}</td>
                  <td>
                    {row.status === "ACTIVE" && (
                      <button
                        className="button secondary"
                        disabled={busy}
                        onClick={() => revoke(row.id)}
                      >
                        Revoke invitation
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

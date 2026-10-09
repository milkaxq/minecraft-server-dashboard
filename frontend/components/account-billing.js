"use client";
import { useAuth } from "./auth";
import { useState } from "react";
import { useResource } from "./data";
import { ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
export function Account() {
  const { logout } = useAuth();
  const { data: account, error, loading } = useResource("/account");
  return (
    <Shell>
      <PageHeading
        eyebrow="YOUR ACCOUNT"
        title="A place to call your own."
        description="Your profile and account security."
      />
      <ErrorMessage error={error} />
      {loading ? (
        <Loading />
      ) : (
        account && (
          <div className="account-grid">
            <section className="panel account-panel">
              <span className="account-avatar">{account.name[0].toUpperCase()}</span>
              <h2>{account.name}</h2>
              <p>{account.email}</p>
              <span className="verified-tag">
                <Icon name="check" size={14} />
                Email verified
              </span>
              <dl className="profile-details">
                <div>
                  <dt>Member since</dt>
                  <dd>
                    {new Date(account.created_at).toLocaleDateString(undefined, {
                      month: "long",
                      day: "numeric",
                      year: "numeric",
                    })}
                  </dd>
                </div>
                <div>
                  <dt>Account ID</dt>
                  <dd>
                    <code>{account.id}</code>
                  </dd>
                </div>
              </dl>
            </section>
            <section className="panel security-panel">
              <Icon name="shield" size={25} />
              <h2>Keep your account safe.</h2>
              <p>Your session expires automatically. Sign out when using a shared computer.</p>
              <p>Signing out revokes this session and disconnects its live consoles.</p>
              <button className="button secondary" onClick={logout}>
                <Icon name="logout" size={16} />
                Sign out of this session
              </button>
              <ChangePassword />
            </section>
          </div>
        )
      )}
    </Shell>
  );
}
function ChangePassword() {
  const { api, passwordChanged } = useAuth();
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  async function submit(event) {
    event.preventDefault();
    if (busy) return;
    setError(null);
    if (password !== confirmation) {
      setError(new Error("Passwords do not match."));
      return;
    }
    if (new TextEncoder().encode(password).length > 72) {
      setError(new Error("Choose a password no longer than 72 bytes."));
      return;
    }
    setBusy(true);
    try {
      await api("/account/password", {
        method: "PUT",
        body: { current_password: current, new_password: password },
      });
      setCurrent("");
      setPassword("");
      setConfirmation("");
      passwordChanged();
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit} className="password-change-form">
      <h3>Change password</h3>
      <p>Use at least 12 characters. Changing your password signs you out on every device.</p>
      <ErrorMessage error={error} />
      <label className="field">
        Current password
        <input
          type="password"
          autoComplete="current-password"
          required
          maxLength={72}
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
        />
      </label>
      <label className="field">
        New password
        <input
          type="password"
          autoComplete="new-password"
          required
          minLength={12}
          maxLength={72}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </label>
      <label className="field">
        Confirm new password
        <input
          type="password"
          autoComplete="new-password"
          required
          minLength={12}
          maxLength={72}
          value={confirmation}
          onChange={(e) => setConfirmation(e.target.value)}
        />
      </label>
      <button className="button primary" disabled={busy}>
        {busy ? "Changing…" : "Change password and sign out"}
      </button>
    </form>
  );
}
export { Billing } from "./billing";

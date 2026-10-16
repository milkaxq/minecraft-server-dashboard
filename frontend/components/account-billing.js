"use client";
import { useAuth } from "./auth";
import { useEffect, useRef, useState } from "react";
import { useResource } from "./data";
import { ErrorMessage, Icon, Loading, PageHeading, Shell } from "./ui";
export function Account() {
  const { logout, signingOut } = useAuth();
  const profile = useResource("/account");
  const { data: account, error, loading } = profile;
  return (
    <Shell>
      <PageHeading eyebrow="YOUR ACCOUNT" title="Account" />
      <ErrorMessage error={error} />
      {error && (
        <button
          className="button secondary"
          disabled={loading || error.status === 429}
          onClick={profile.refresh}
        >
          Refresh account
        </button>
      )}
      {loading ? (
        <Loading />
      ) : (
        account && (
          <div className="account-grid">
            <section className="panel account-panel">
              <span className="account-avatar">
                {account.name?.trim().charAt(0).toUpperCase() || "?"}
              </span>
              <h2>{account.name}</h2>
              <p>{account.email}</p>
              {account.verified_at && (
                <span className="verified-tag">
                  <Icon name="check" size={14} />
                  Email verified
                </span>
              )}
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
              </dl>
              <button className="button secondary" disabled={signingOut} onClick={logout}>
                <Icon name="logout" size={16} />
                {signingOut ? "Signing out…" : "Sign out of this session"}
              </button>
            </section>
            <section className="panel security-panel account-password-panel">
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
  const [visible, setVisible] = useState(false);
  const inFlight = useRef(false);
  const [error, setError] = useState(null);
  const confirmationInput = useRef(null);
  const passwordInput = useRef(null);
  const retryUntil = useRef(0);
  const [retrySeconds, setRetrySeconds] = useState(0);
  useEffect(() => {
    if (!retrySeconds) return;
    const timer = setTimeout(() => {
      setRetrySeconds(Math.max(0, Math.ceil((retryUntil.current - Date.now()) / 1000)));
    }, 1000);
    return () => clearTimeout(timer);
  }, [retrySeconds]);
  async function submit(event) {
    event.preventDefault();
    if (inFlight.current || Date.now() < retryUntil.current) return;
    setError(null);
    if (password !== confirmation) {
      setError(new Error("Passwords do not match."));
      confirmationInput.current?.focus();
      return;
    }
    if (new TextEncoder().encode(password).length > 72) {
      setError(new Error("Choose a password no longer than 72 bytes."));
      passwordInput.current?.focus();
      return;
    }
    inFlight.current = true;
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
      if (err.status === 429) {
        retryUntil.current = Date.now() + (err.retryAfterMs || 60000);
        setRetrySeconds(Math.ceil((retryUntil.current - Date.now()) / 1000));
      }
      inFlight.current = false;
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit} className="password-change-form" aria-busy={busy}>
      <h2>Change password</h2>
      <p>At least 12 characters. You’ll sign in again on all devices.</p>
      <ErrorMessage error={error} />
      <label className="field">
        Current password
        <input
          type={visible ? "text" : "password"}
          disabled={busy}
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
          ref={passwordInput}
          type={visible ? "text" : "password"}
          disabled={busy}
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
          ref={confirmationInput}
          type={visible ? "text" : "password"}
          disabled={busy}
          autoComplete="new-password"
          required
          minLength={12}
          maxLength={72}
          value={confirmation}
          onChange={(e) => setConfirmation(e.target.value)}
        />
      </label>
      <label className="account-password-visibility">
        <input
          type="checkbox"
          checked={visible}
          disabled={busy}
          onChange={(event) => setVisible(event.target.checked)}
        />
        Show passwords
      </label>
      <button className="button primary" disabled={busy || retrySeconds > 0}>
        {busy
          ? "Changing…"
          : retrySeconds
            ? `Try again in ${retrySeconds}s`
            : "Change password and sign out"}
      </button>
    </form>
  );
}
export { Billing } from "./billing";

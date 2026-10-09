"use client";
import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth";
import { Brand, ErrorMessage, Icon } from "./ui";
import { request } from "../lib/api";
const subscribeHash = (notify) => {
  window.addEventListener("hashchange", notify);
  return () => window.removeEventListener("hashchange", notify);
};
const readHash = () => window.location.hash;
const emptyHash = () => "";
export function AuthScreen({ mode = "login" }) {
  const { login, session, authNotice } = useAuth();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [done, setDone] = useState(false);
  const forgot = mode === "forgot-password";
  const reset = mode === "reset-password";
  const verify = mode === "verify";
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  useEffect(() => {
    if (session) router.replace("/dashboard");
  }, [session, router]);
  const hash = useSyncExternalStore(subscribeHash, readHash, emptyHash);
  const [consumedHash, setConsumedHash] = useState("");
  if (hash && consumedHash !== hash) {
    setConsumedHash(hash);
    if (reset || verify) {
      const value = new URLSearchParams(hash.slice(1)).get("token");
      if (value) setToken(value);
    }
  }
  useEffect(() => {
    // Fragments keep tokens out of HTTP logs; clear them from browser history too.
    if (consumedHash)
      window.history.replaceState(window.history.state, "", window.location.pathname);
  }, [consumedHash]);
  async function submit(event) {
    event.preventDefault();
    if (busy) return;
    setError(null);
    setNotice("");
    setBusy(true);
    try {
      if (forgot) {
        const result = await request("/auth/forgot-password", {
          method: "POST",
          body: { email: email.trim() },
        });
        setNotice(result.message);
      } else if (reset) {
        if (password !== confirmation) throw new Error("Passwords do not match.");
        if (new TextEncoder().encode(password).length > 72)
          throw new Error("Choose a password no longer than 72 bytes.");
        await request("/auth/reset-password", {
          method: "POST",
          body: { token: token.trim(), password },
        });
        setPassword("");
        setConfirmation("");
        setToken("");
        setDone(true);
        setNotice("Password reset. All sessions were signed out. Sign in with your new password.");
      } else if (mode === "login") await login(email.trim(), password);
      else if (mode === "register") {
        if (new TextEncoder().encode(password).length > 72)
          throw new Error("Choose a password no longer than 72 bytes.");
        await request("/auth/register", {
          method: "POST",
          body: { name: name.trim(), email: email.trim(), password },
        });
        router.push("/verify");
      } else {
        await request("/auth/verify-email", { method: "POST", body: { token: token.trim() } });
        setNotice("Email verified. You’re ready to sign in.");
        setDone(true);
        setToken("");
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  async function resend(event) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await request("/auth/resend-verification", {
        method: "POST",
        body: { email: email.trim() },
      });
      setNotice(result.message);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="auth-page">
      <section className="auth-story">
        <Brand />
        <div>
          <span className="eyebrow">LESS SETUP. MORE MINECRAFT.</span>
          <h1>
            Your world.
            <br />
            Your people.
            <br />
            <em>Your adventure.</em>
          </h1>
          <p>A home for the things you build together.</p>
          <div className="auth-landscape" aria-hidden="true">
            <span className="block block-a" />
            <span className="block block-b" />
            <span className="block block-c" />
          </div>
        </div>
        <span className="auth-story-footer">Minecraft hosting, made a little simpler.</span>
      </section>
      <main className="auth-form-wrap">
        <div className="auth-form">
          <span className="eyebrow">WELCOME TO BLOCKYARD</span>
          <h2>
            {forgot
              ? "Let’s get you back in."
              : reset
                ? "Choose a new password."
                : mode === "login"
                  ? "Good to see you again."
                  : mode === "register"
                    ? "Start your next adventure."
                    : done
                      ? "Your email is verified."
                      : "Check your inbox."}
          </h2>
          <p>
            {forgot
              ? "Enter your account email to request a password reset link."
              : reset
                ? "Use the token from your email. Links expire after 30 minutes and work once."
                : mode === "login"
                  ? "Sign in to get back to your worlds."
                  : mode === "register"
                    ? "Create an account to find a home for your world."
                    : done
                      ? "Sign in to choose a plan and create your first world."
                      : token && consumedHash
                        ? "Your verification link is ready. Confirm below to finish creating your account."
                        : "Open the link in your email, or paste its verification token below. Links expire after 24 hours."}
          </p>
          <ErrorMessage error={error} />
          {(notice || (mode === "login" && authNotice)) && (
            <div className="notice success" role="status">
              {notice || authNotice}
            </div>
          )}
          {!((reset || verify) && done) && (
            <form onSubmit={submit}>
              {mode === "register" && (
                <label className="field">
                  Your name
                  <input
                    autoComplete="name"
                    required
                    maxLength={80}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Alex"
                  />
                </label>
              )}
              {(mode === "login" || mode === "register" || forgot) && (
                <label className="field">
                  Email address
                  <input
                    type="email"
                    autoComplete="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@example.com"
                  />
                </label>
              )}
              {(mode === "login" || mode === "register" || reset) && (
                <>
                  <label className="field">
                    {reset ? "New password" : "Password"}
                    <input
                      type="password"
                      autoComplete={
                        mode === "register" || reset ? "new-password" : "current-password"
                      }
                      required
                      minLength={mode === "register" || reset ? 12 : undefined}
                      maxLength={72}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      placeholder={mode === "register" ? "At least 12 characters" : "Your password"}
                    />
                  </label>
                </>
              )}
              {reset && (
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
              )}
              {(reset || (verify && !(token && consumedHash))) && (
                <label className="field">
                  {reset ? "Reset token" : "Verification token"}
                  <input
                    autoComplete="off"
                    spellCheck="false"
                    required
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    placeholder="Paste the token from your email"
                  />
                </label>
              )}
              <button className="button primary wide" disabled={busy}>
                {busy
                  ? "Please wait…"
                  : forgot
                    ? "Send reset link"
                    : reset
                      ? "Reset password"
                      : mode === "login"
                        ? "Sign in"
                        : mode === "register"
                          ? "Create account"
                          : "Verify email"}
                <Icon name="arrow" size={16} />
              </button>
            </form>
          )}
          {verify && done && (
            <Link href="/login" className="button primary wide">
              Continue to sign in <Icon name="arrow" size={16} />
            </Link>
          )}
          <div className="auth-links">
            {mode === "login" ? (
              <>
                <p>
                  New around here? <Link href="/register">Create an account</Link>
                </p>
                <Link href="/verify">Verify your email</Link>
                <p>
                  <Link href="/forgot-password">Forgot your password?</Link>
                </p>
              </>
            ) : (
              <p>
                Already have an account? <Link href="/login">Sign in</Link>
              </p>
            )}
          </div>
          {(reset || forgot) && (
            <p>
              <Link href={reset ? "/forgot-password" : "/reset-password"}>
                {reset ? "Request a new reset link" : "Already have a reset token?"}
              </Link>
            </p>
          )}
          {verify && !done && (
            <form onSubmit={resend} className="resend-form">
              <label className="field">
                Need another verification email?
                <input
                  type="email"
                  autoComplete="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="you@example.com"
                />
              </label>
              <button className="button secondary wide" disabled={busy}>
                Resend verification
              </button>
            </form>
          )}
        </div>
        <span className="auth-form-footer">A home for your next world.</span>
      </main>
    </div>
  );
}

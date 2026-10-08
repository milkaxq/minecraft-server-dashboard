"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth";
import { Brand, ErrorMessage, Icon } from "./ui";
import { request } from "../lib/api";
export function AuthScreen({ mode = "login" }) {
  const { login, session } = useAuth();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  useEffect(() => {
    if (session) router.replace("/dashboard");
  }, [session, router]);
  async function submit(event) {
    event.preventDefault();
    setError(null);
    setNotice("");
    setBusy(true);
    try {
      if (mode === "login") await login(email.trim(), password);
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
            {mode === "login"
              ? "Good to see you again."
              : mode === "register"
                ? "Start your next adventure."
                : "One last step."}
          </h2>
          <p>
            {mode === "login"
              ? "Sign in to get back to your worlds."
              : mode === "register"
                ? "Create an account to find a home for your world."
                : "Enter the verification token from your email."}
          </p>
          <ErrorMessage error={error} />
          {notice && (
            <div className="notice success" role="status">
              {notice}
            </div>
          )}
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
            {mode !== "verify" ? (
              <>
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
                <label className="field">
                  Password
                  <input
                    type="password"
                    autoComplete={mode === "register" ? "new-password" : "current-password"}
                    required
                    minLength={mode === "register" ? 12 : undefined}
                    maxLength={72}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder={mode === "register" ? "At least 12 characters" : "Your password"}
                  />
                </label>
              </>
            ) : (
              <label className="field">
                Verification token
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
                : mode === "login"
                  ? "Sign in"
                  : mode === "register"
                    ? "Create account"
                    : "Verify email"}
              <Icon name="arrow" size={16} />
            </button>
          </form>
          <div className="auth-links">
            {mode === "login" ? (
              <>
                <p>
                  New around here? <Link href="/register">Create an account</Link>
                </p>
                <Link href="/verify">Verify your email</Link>
              </>
            ) : (
              <p>
                Already have an account? <Link href="/login">Sign in</Link>
              </p>
            )}
          </div>
          {mode === "verify" && (
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

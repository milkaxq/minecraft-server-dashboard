"use client";
import { useAuth } from "./auth";
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
            </section>
          </div>
        )
      )}
    </Shell>
  );
}
export { Billing } from "./billing";

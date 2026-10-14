"use client";
import Link from "next/link";

export function SetupProgress({ server, isAdmin = false }) {
  const state = server.status;
  const titles = {
    PENDING_PAYMENT: "Your payment is still pending.",
    PAYMENT_CONFIRMED: "Your server is queued for setup.",
    PROVISIONING: "Minecraft is installing.",
    FAILED: "Setup didn’t finish.",
    SUSPENDED: "This server is suspended.",
    CANCELLED: "This server has been cancelled.",
  };
  const messages = {
    PENDING_PAYMENT: "Finish checkout in Billing. Setup begins after payment is confirmed.",
    PAYMENT_CONFIRMED:
      "Your order is confirmed. We are waiting for hosting space before installation begins.",
    PROVISIONING:
      "Your isolated server is being prepared. This page checks progress automatically; you can leave and return later.",
    FAILED:
      "Your server request and configuration are kept. Ask your host to retry this server; you do not need to submit a new creation request.",
    SUSPENDED: "Contact your host to reactivate this server.",
    CANCELLED: "Your other worlds are still available from Servers.",
  };
  const stages = ["Order confirmed", "Install Minecraft", "Ready for first start"];
  const active = state === "PAYMENT_CONFIRMED" ? 1 : state === "PROVISIONING" ? 1 : 0;
  const showStages = ["PENDING_PAYMENT", "PAYMENT_CONFIRMED", "PROVISIONING", "FAILED"].includes(
    state,
  );
  return (
    <section className="panel setup-progress" aria-label="Server setup progress">
      <span className="eyebrow">YOUR SERVER SETUP</span>
      <h2>{titles[state] || "Checking server setup"}</h2>
      <p>{messages[state] || "Wait for the next status update."}</p>
      {showStages && (
        <ol>
          {stages.map((title, index) => {
            const done =
              state !== "FAILED" &&
              index === 0 &&
              ["PAYMENT_CONFIRMED", "PROVISIONING"].includes(state);
            const current = state !== "FAILED" && index === active;
            return (
              <li
                key={title}
                className={done ? "complete" : current ? "current" : ""}
                aria-current={current ? "step" : undefined}
              >
                <span aria-hidden="true">{done ? "✓" : index + 1}</span>
                <div>
                  <strong>{title}</strong>
                  <small>
                    {done
                      ? "Complete"
                      : state === "FAILED"
                        ? "Setup interrupted"
                        : current
                          ? state === "PROVISIONING"
                            ? "In progress"
                            : "Waiting"
                          : "Up next"}
                  </small>
                </div>
              </li>
            );
          })}
        </ol>
      )}
      {state === "PROVISIONING" && (
        <p className="field-help">
          When installation finishes, review the Minecraft agreement, click Start, then prepare your
          player invitation. No estimated completion time is available.
        </p>
      )}
      <div className="invite-actions">
        {state === "PENDING_PAYMENT" && (
          <Link className="button primary" href="/billing">
            Open Billing
          </Link>
        )}
        {state === "FAILED" && isAdmin && (
          <Link className="button primary" href="/admin">
            Open setup recovery
          </Link>
        )}
        {["FAILED", "SUSPENDED"].includes(state) && (
          <Link
            className="button secondary"
            href={`/feedback?server=${server.id}&page=server-setup`}
          >
            Ask for setup help
          </Link>
        )}
        {state === "CANCELLED" && (
          <Link className="button secondary" href="/servers">
            Your other servers
          </Link>
        )}
      </div>
    </section>
  );
}

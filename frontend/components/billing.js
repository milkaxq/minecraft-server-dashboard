"use client";
import Link from "next/link";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Table, ErrorMessage, Loading, PageHeading, Shell } from "./ui";
import { price } from "../lib/api";
function money(item) {
  return price({
    price_cents: item.price_cents,
    currency: item.currency || "USD",
  });
}
function date(value) {
  return new Date(value).toLocaleString();
}
export function Checkout({ id }) {
  const { api } = useAuth();
  const info = useResource(`/orders/${id}`, 3000);
  const config = useResource("/billing/config");
  const order = info.data;
  const [busy, setBusy] = useState("");
  const inFlight = useRef(false);
  const unavailable = info.loading || !!info.error || !order;
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  async function pay() {
    if (
      inFlight.current ||
      unavailable ||
      order.status !== "PENDING" ||
      config.error ||
      !config.data?.checkout_enabled ||
      config.data?.provider !== "mock"
    )
      return;
    inFlight.current = true;
    setBusy("pay");
    setError(null);
    try {
      await api(`/orders/${id}/checkout`, { method: "POST", body: {} });
      await api(`/orders/${id}/mock-pay`, { method: "POST", body: {} });
      setNotice("Demo payment confirmed. Your server is being prepared.");
      info.refresh();
    } catch (e) {
      setError(e);
    } finally {
      inFlight.current = false;
      setBusy("");
    }
  }
  async function cancel() {
    if (inFlight.current || unavailable || order.status !== "PENDING") return;
    inFlight.current = true;
    setBusy("cancel");
    setError(null);
    try {
      await api(`/orders/${id}/cancel`, { method: "POST", body: {} });
      info.refresh();
    } catch (e) {
      setError(e);
    } finally {
      inFlight.current = false;
      setBusy("");
    }
  }
  return (
    <Shell>
      <PageHeading eyebrow="YOUR ORDER" title="Review your order." />
      <ErrorMessage error={info.error || config.error || error} />
      {(info.error || config.error) && (
        <button
          className="button secondary"
          disabled={
            info.loading ||
            config.loading ||
            info.error?.status === 429 ||
            config.error?.status === 429
          }
          onClick={() => {
            info.refresh();
            config.refresh();
          }}
        >
          Refresh order
        </button>
      )}
      {notice && (
        <p className="notice success" role="status">
          {notice}
        </p>
      )}
      {info.loading ? (
        <Loading />
      ) : (
        order && (
          <section className="panel checkout-card">
            <span className="eyebrow">
              {order.kind === "RENEWAL" ? "RENEW YOUR WORLD" : "MINECRAFT HOSTING"}
            </span>
            <h2>{order.plan_name}</h2>
            <div className="checkout-amount">
              {money(order)}
              <small>/{order.billing_period === "YEARLY" ? "year" : "month"}</small>
            </div>
            <dl className="checkout-details">
              <div>
                <dt>Order</dt>
                <dd>
                  <code>{order.id.slice(0, 8)}</code>
                </dd>
              </div>
              <div>
                <dt>Status</dt>
                <dd>
                  {order.status === "PAID"
                    ? "Payment confirmed"
                    : order.status === "PENDING"
                      ? "Awaiting payment"
                      : order.status}
                </dd>
              </div>
              <div>
                <dt>Billing</dt>
                <dd>Manual renewal. No automatic charges.</dd>
              </div>
            </dl>
            {order.status === "PENDING" ? (
              <>
                <div className="notice">
                  <strong>Development demo checkout</strong>
                  <p>No real money is charged. Do not enter card or bank details.</p>
                </div>
                <button
                  className="button primary wide"
                  disabled={
                    !!busy ||
                    unavailable ||
                    !!config.error ||
                    !config.data?.checkout_enabled ||
                    config.data?.provider !== "mock"
                  }
                  onClick={pay}
                >
                  {busy === "pay" ? "Confirming…" : "Confirm demo payment"}
                </button>
                <button
                  className="button secondary wide"
                  disabled={!!busy || unavailable}
                  onClick={cancel}
                >
                  {busy === "cancel" ? "Cancelling…" : "Cancel this order"}
                </button>
              </>
            ) : order.status === "PAID" ? (
              <>
                <p>Your payment is confirmed. Follow setup progress on your server page.</p>
                <Link className="button primary wide" href={`/servers/${order.server_id}`}>
                  Go to your server
                </Link>
              </>
            ) : (
              <Link className="button secondary wide" href="/billing">
                Return to billing
              </Link>
            )}
          </section>
        )
      )}
    </Shell>
  );
}
export function Billing() {
  const { api } = useAuth();
  const router = useRouter();
  const orders = useResource("/orders", 10000);
  const subscriptions = useResource("/subscriptions", 10000);
  const config = useResource("/billing/config");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const inFlight = useRef(false);
  const renewalAttempts = useRef(new Map());
  const subscriptionsUnavailable = subscriptions.loading || !!subscriptions.error;
  async function renew(id) {
    if (
      inFlight.current ||
      subscriptionsUnavailable ||
      config.error ||
      !config.data?.checkout_enabled
    )
      return;
    inFlight.current = true;
    if (!renewalAttempts.current.has(id)) renewalAttempts.current.set(id, crypto.randomUUID());
    setBusy("renew:" + id);
    setError(null);
    try {
      const order = await api(`/servers/${id}/renewals`, {
        method: "POST",
        body: {},
        key: renewalAttempts.current.get(id),
      });
      router.push(`/checkout/${order.id}`);
    } catch (e) {
      setError(e);
    } finally {
      inFlight.current = false;
      setBusy("");
    }
  }
  async function cancellation(s) {
    if (inFlight.current || subscriptionsUnavailable) return;
    inFlight.current = true;
    setBusy("subscription:" + s.server_id);
    setError(null);
    try {
      await api(`/servers/${s.server_id}/subscription`, {
        method: "PUT",
        body: { cancel_at_period_end: !s.cancel_at_period_end },
      });
      setNotice(
        s.cancel_at_period_end
          ? "Your service will remain eligible for renewal."
          : "Your service will end at the paid-through date.",
      );
      subscriptions.refresh();
    } catch (e) {
      setError(e);
    } finally {
      inFlight.current = false;
      setBusy("");
    }
  }
  return (
    <Shell>
      <PageHeading
        eyebrow="YOUR BILLING"
        title="Billing"
        action={
          <Link className="button secondary" href="/servers/new">
            Choose a hosting plan
          </Link>
        }
      />
      <ErrorMessage error={orders.error || subscriptions.error || config.error || error} />
      {(orders.error || subscriptions.error || config.error) && (
        <button
          className="button secondary"
          disabled={
            orders.loading ||
            subscriptions.loading ||
            config.loading ||
            [orders.error, subscriptions.error, config.error].some((e) => e?.status === 429)
          }
          onClick={() => {
            orders.refresh();
            subscriptions.refresh();
            config.refresh();
          }}
        >
          Refresh billing
        </button>
      )}
      {notice && (
        <p role="status" className="notice success">
          {notice}
        </p>
      )}
      <p className="notice">
        {config.data?.provider === "mock" ? "Demo payments only — no real money is charged. " : ""}
        Manual renewal. No automatic charges.
      </p>
      <section className="panel management-panel">
        <div className="section-heading">
          <h2>Your subscriptions</h2>
        </div>
        {subscriptions.loading ? (
          <Loading />
        ) : subscriptions.error && !subscriptions.data ? (
          <p className="management-empty">Subscriptions unavailable.</p>
        ) : !subscriptions.data?.length ? (
          <p className="management-empty">No subscriptions yet.</p>
        ) : (
          <ul className="subscription-list">
            {subscriptions.data.map((s) => {
              const expired = new Date(s.current_period_end) <= new Date();
              return (
                <li key={s.id}>
                  <div>
                    <Link href={`/servers/${s.server_id}`}>
                      <strong>{s.server_name}</strong>
                    </Link>
                    <p>
                      {s.price_cents != null ? money(s) : "Operator managed"} ·{" "}
                      {s.billing_period === "YEARLY" ? "yearly" : "monthly"}
                    </p>
                    <small>
                      {expired ? "Period ended" : "Paid through"} {date(s.current_period_end)}
                      {s.cancel_at_period_end ? " · Ends at this date" : ""}
                    </small>
                  </div>
                  <div className="row-actions">
                    <span className="billing-status">
                      {expired
                        ? s.status === "CANCELLED"
                          ? "Ended"
                          : "Renewal due"
                        : s.cancel_at_period_end
                          ? "Ending"
                          : "Active"}
                    </span>
                    <button
                      className="button primary"
                      disabled={
                        !!busy ||
                        subscriptionsUnavailable ||
                        !!config.error ||
                        !config.data?.checkout_enabled
                      }
                      onClick={() => renew(s.server_id)}
                    >
                      {busy === "renew:" + s.server_id ? "Opening checkout…" : "Renew"}
                    </button>
                    {!expired && s.status === "ACTIVE" && (
                      <button
                        className="button secondary"
                        disabled={!!busy || subscriptionsUnavailable}
                        onClick={() => cancellation(s)}
                      >
                        {busy === "subscription:" + s.server_id
                          ? "Updating…"
                          : s.cancel_at_period_end
                            ? "Keep service"
                            : "End after this period"}
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </section>
      <section className="panel management-panel">
        <div className="section-heading">
          <h2>Order history</h2>
          <span>Latest 100 orders</span>
        </div>
        {orders.loading ? (
          <Loading />
        ) : orders.error && !orders.data ? (
          <p className="management-empty">Order history unavailable.</p>
        ) : !orders.data?.length ? (
          <p className="management-empty">Your orders will appear here.</p>
        ) : (
          <div className="table-scroll">
            <Table className="management-table">
              <thead>
                <tr>
                  <th>Order</th>
                  <th>Plan</th>
                  <th>Amount</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {orders.data.map((o) => (
                  <tr key={o.id}>
                    <td>
                      <code>{o.id.slice(0, 8)}</code>
                      <small>{o.kind === "RENEWAL" ? "Renewal" : "New server"}</small>
                    </td>
                    <td>{o.plan_name}</td>
                    <td>{money(o)}</td>
                    <td>{o.status}</td>
                    <td>{date(o.created_at)}</td>
                    <td>
                      <Link href={`/checkout/${o.id}`}>
                        {o.status === "PENDING" ? "Finish checkout" : "View order"}
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        )}
      </section>
    </Shell>
  );
}

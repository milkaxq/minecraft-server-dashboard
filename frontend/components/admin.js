"use client";
import { JobHelp, NodeCapacity } from "./admin-health";
import { useEffect, useRef, useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Loading, PageHeading, Shell, Badge } from "./ui";
import { ReportList } from "./feedback";
import { Invitations } from "./invitations";
import { formatBytes, price } from "../lib/api";
const tabs = ["plans", "servers", "users", "orders", "nodes", "audit", "invitations", "feedback"];
const numbers = [
  ["price_cents", "Price (cents)", 0, 100000000],
  ["ram_mb", "RAM (MB)", 512, 131072],
  ["disk_mb", "Disk (MB)", 1024, 2097152],
  ["cpu_limit", "CPU (%)", 1, 10000],
  ["backup_limit", "Backup limit", 0, 100],
  ["database_limit", "Database limit", 0, 100],
  ["player_recommendation", "Player recommendation", 1, 1000],
];
export function Admin() {
  const { api } = useAuth();
  const account = useResource("/account");
  const [tab, setTab] = useState("servers");
  const [offset, setOffset] = useState(0);
  const resource = useResource(
    account.data?.is_admin
      ? `/admin/${tab}${["users", "servers", "orders", "audit", "feedback"].includes(tab) ? `?offset=${offset}` : ""}`
      : null,
    10000,
  );
  const capacityPlans = useResource(
    account.data?.is_admin && tab === "nodes" ? "/admin/plans" : null,
    10000,
  );
  const [edit, setEdit] = useState(null);
  const [action, setAction] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  async function savePlan(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api(`/admin/plans${edit.id ? `/${edit.id}` : ""}`, {
        method: edit.id ? "PUT" : "POST",
        body: edit,
      });
      setEdit(null);
      setNotice(
        "Plan saved. Existing server resources and subscriptions keep their purchased terms.",
      );
      resource.refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  async function lifecycle(input) {
    setBusy(true);
    setError(null);
    try {
      const job = await api(`/admin/servers/${action.server.id}/${action.kind}`, {
        method: "POST",
        body: input,
        key: action.key,
      });
      setNotice(`Action queued. Job ${job.id.slice(0, 8)}.`);
      setAction(null);
      resource.refresh();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  const rows = resource.data || [];
  return (
    <Shell>
      <PageHeading
        eyebrow="HOSTING OPERATIONS"
        title="Admin workspace."
        description="Plans, customers, capacity and audited server actions."
      />
      <ErrorMessage error={account.error || resource.error || capacityPlans.error || error} />
      {notice && (
        <p className="notice success" role="status">
          {notice}
        </p>
      )}
      {account.loading ? (
        <Loading />
      ) : !account.data?.is_admin ? (
        <p className="notice">Administrator access is required.</p>
      ) : (
        <>
          <nav className="feature-tabs" aria-label="Admin sections">
            {tabs.map((t) => (
              <button
                key={t}
                aria-current={tab === t ? "page" : undefined}
                onClick={() => {
                  setTab(t);
                  setOffset(0);
                  setEdit(null);
                  setAction(null);
                  setError(null);
                }}
              >
                {t[0].toUpperCase() + t.slice(1)}
              </button>
            ))}
          </nav>
          {tab === "plans" && (
            <button
              className="button primary"
              onClick={() =>
                setEdit({
                  name: "",
                  description: "",
                  price_cents: 0,
                  currency: "USD",
                  billing_period: "MONTHLY",
                  ram_mb: 2048,
                  disk_mb: 10240,
                  cpu_limit: 100,
                  backup_limit: 2,
                  database_limit: 0,
                  player_recommendation: 10,
                  enabled: true,
                })
              }
            >
              Create plan
            </button>
          )}
          {edit && (
            <section className="panel management-panel admin-plan-editor">
              <h2>{edit.id ? "Edit plan" : "Create plan"}</h2>
              <form onSubmit={savePlan}>
                <div className="field-row">
                  <label className="field">
                    Plan name
                    <input
                      required
                      maxLength={80}
                      value={edit.name}
                      onChange={(e) => setEdit({ ...edit, name: e.target.value })}
                    />
                  </label>
                  <label className="field">
                    Currency
                    <input
                      required
                      minLength={3}
                      maxLength={3}
                      pattern="[A-Z]{3}"
                      value={edit.currency}
                      onChange={(e) =>
                        setEdit({
                          ...edit,
                          currency: e.target.value.toUpperCase(),
                        })
                      }
                    />
                  </label>
                </div>
                <label className="field">
                  Description
                  <textarea
                    maxLength={1000}
                    value={edit.description}
                    onChange={(e) => setEdit({ ...edit, description: e.target.value })}
                  />
                </label>
                <div className="admin-plan-numbers">
                  {numbers.map(([key, label, min, max]) => (
                    <label className="field" key={key}>
                      {label}
                      <input
                        required
                        type="number"
                        min={min}
                        max={max}
                        value={edit[key]}
                        onChange={(e) => setEdit({ ...edit, [key]: Number(e.target.value) })}
                      />
                    </label>
                  ))}
                </div>
                <label className="field">
                  Billing period
                  <select
                    value={edit.billing_period}
                    onChange={(e) => setEdit({ ...edit, billing_period: e.target.value })}
                  >
                    <option value="MONTHLY">Monthly</option>
                    <option value="YEARLY">Yearly</option>
                  </select>
                </label>
                <label className="setting-toggle">
                  <strong>Available for new orders</strong>
                  <input
                    type="checkbox"
                    checked={edit.enabled}
                    onChange={(e) => setEdit({ ...edit, enabled: e.target.checked })}
                  />
                </label>
                <div className="file-actions">
                  <button className="button primary" disabled={busy}>
                    Save plan
                  </button>
                  <button type="button" className="button secondary" onClick={() => setEdit(null)}>
                    Cancel edit
                  </button>
                </div>
              </form>
            </section>
          )}
          {tab === "invitations" ? (
            <Invitations rows={rows} loading={resource.loading} refresh={resource.refresh} />
          ) : tab === "feedback" ? (
            <section className="panel management-panel feedback-panel">
              <h2>Beta feedback inbox</h2>
              <p className="field-help">
                Private reports from testers. Review, reply and resolve them here.
              </p>
              <ReportList
                rows={rows}
                loading={resource.loading}
                offset={offset}
                setOffset={setOffset}
                admin
                refresh={resource.refresh}
              />
            </section>
          ) : (
            <section className="panel management-panel">
              {tab === "nodes" && resource.data && (
                <NodeCapacity nodes={rows} plans={capacityPlans.data} />
              )}
              {resource.loading ? (
                <Loading />
              ) : !rows.length ? (
                <p className="management-empty">No {tab} to display.</p>
              ) : (
                <div className="table-scroll">
                  <table className="management-table">
                    {tab === "plans" ? (
                      <>
                        <thead>
                          <tr>
                            <th>Plan</th>
                            <th>Price</th>
                            <th>Resources</th>
                            <th>Enabled</th>
                            <th />
                          </tr>
                        </thead>
                        <tbody>
                          {rows.map((p) => (
                            <tr key={p.id}>
                              <td>
                                {p.name}
                                <small>{p.description}</small>
                              </td>
                              <td>
                                {price(p)}
                                <small>{p.billing_period}</small>
                              </td>
                              <td>
                                {formatBytes(p.ram_mb * 1048576)} RAM ·{" "}
                                {formatBytes(p.disk_mb * 1048576)} disk · {p.cpu_limit}% CPU
                              </td>
                              <td>{p.enabled ? "Yes" : "No"}</td>
                              <td>
                                <button
                                  className="button secondary"
                                  onClick={() => setEdit({ ...p })}
                                >
                                  Edit {p.name}
                                </button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </>
                    ) : tab === "servers" ? (
                      <>
                        <thead>
                          <tr>
                            <th>Server</th>
                            <th>Customer</th>
                            <th>Status</th>
                            <th>Latest job</th>
                            <th>Actions</th>
                          </tr>
                        </thead>
                        <tbody>
                          {rows.map((s) => {
                            const pending = ["PENDING", "RUNNING"].includes(s.job_status);
                            return (
                              <tr key={s.id}>
                                <td>
                                  <strong>{s.name}</strong>
                                  <small>{s.id}</small>
                                  <small>
                                    {s.plan_name} · {s.server_type} {s.minecraft_version}
                                  </small>
                                </td>
                                <td>{s.owner_email}</td>
                                <td>
                                  <Badge state={s.status} />
                                  {s.suspension_reason && (
                                    <small>
                                      {s.suspension_reason === "BILLING"
                                        ? "Subscription expiry"
                                        : "Admin suspension"}
                                    </small>
                                  )}
                                </td>
                                <td>
                                  {s.job_action ? `${s.job_action} · ${s.job_status}` : "—"}
                                  <JobHelp server={s} />
                                </td>
                                <td>
                                  <div className="row-actions">
                                    {s.status === "FAILED" && (
                                      <button
                                        disabled={pending || busy}
                                        onClick={() =>
                                          setAction({
                                            kind: "retry",
                                            server: s,
                                            key: crypto.randomUUID(),
                                          })
                                        }
                                      >
                                        Retry setup
                                      </button>
                                    )}
                                    {s.status === "RUNNING" && (
                                      <button
                                        disabled={pending || busy}
                                        onClick={() =>
                                          setAction({
                                            kind: "suspend",
                                            server: s,
                                            key: crypto.randomUUID(),
                                          })
                                        }
                                      >
                                        Suspend
                                      </button>
                                    )}
                                    {s.status === "SUSPENDED" && (
                                      <button
                                        disabled={pending || busy}
                                        onClick={() =>
                                          setAction({
                                            kind: "unsuspend",
                                            server: s,
                                            key: crypto.randomUUID(),
                                          })
                                        }
                                      >
                                        Unsuspend
                                      </button>
                                    )}
                                    {!["CANCELLED", "PROVISIONING"].includes(s.status) && (
                                      <button
                                        className="text-danger"
                                        disabled={pending || busy}
                                        onClick={() =>
                                          setAction({
                                            kind: "terminate",
                                            server: s,
                                            key: crypto.randomUUID(),
                                          })
                                        }
                                      >
                                        Terminate
                                      </button>
                                    )}
                                  </div>
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </>
                    ) : tab === "users" ? (
                      <>
                        <thead>
                          <tr>
                            <th>Name</th>
                            <th>Email</th>
                            <th>Verified</th>
                            <th>Admin</th>
                            <th>Joined</th>
                          </tr>
                        </thead>
                        <tbody>
                          {rows.map((u) => (
                            <tr key={u.id}>
                              <td>
                                {u.name}
                                <small>{u.id}</small>
                              </td>
                              <td>{u.email}</td>
                              <td>{u.verified_at ? "Yes" : "No"}</td>
                              <td>{u.is_admin ? "Yes" : "No"}</td>
                              <td>{new Date(u.created_at).toLocaleDateString()}</td>
                            </tr>
                          ))}
                        </tbody>
                      </>
                    ) : tab === "orders" ? (
                      <>
                        <thead>
                          <tr>
                            <th>Order</th>
                            <th>Customer</th>
                            <th>Plan</th>
                            <th>Amount</th>
                            <th>Status</th>
                          </tr>
                        </thead>
                        <tbody>
                          {rows.map((o) => (
                            <tr key={o.id}>
                              <td>
                                <code>{o.id}</code>
                                <small>{o.kind}</small>
                              </td>
                              <td>
                                <code>{o.user_id}</code>
                              </td>
                              <td>{o.plan_name}</td>
                              <td>{price(o)}</td>
                              <td>{o.status}</td>
                            </tr>
                          ))}
                        </tbody>
                      </>
                    ) : tab === "nodes" ? (
                      <>
                        <thead>
                          <tr>
                            <th>Node</th>
                            <th>RAM allocated / capacity</th>
                            <th>Disk allocated / capacity</th>
                            <th>CPU allocated / capacity</th>
                            <th>Enabled</th>
                          </tr>
                        </thead>
                        <tbody>
                          {rows.map((n) => (
                            <tr key={n.id}>
                              <td>{n.name}</td>
                              <td>
                                {formatBytes(n.allocated_ram_mb * 1048576)} /{" "}
                                {formatBytes(n.ram_capacity_mb * 1048576)}
                              </td>
                              <td>
                                {formatBytes(n.allocated_disk_mb * 1048576)} /{" "}
                                {formatBytes(n.disk_capacity_mb * 1048576)}
                              </td>
                              <td>
                                {n.allocated_cpu}% / {n.cpu_capacity}%
                              </td>
                              <td>{n.enabled ? "Yes" : "No"}</td>
                            </tr>
                          ))}
                        </tbody>
                      </>
                    ) : (
                      <>
                        <thead>
                          <tr>
                            <th>Action</th>
                            <th>Actor</th>
                            <th>Server</th>
                            <th>Request</th>
                            <th>Time</th>
                          </tr>
                        </thead>
                        <tbody>
                          {rows.map((v) => (
                            <tr key={v.id}>
                              <td>{v.action}</td>
                              <td>
                                <code>{v.actor_id || "System"}</code>
                              </td>
                              <td>
                                <code>{v.server_id || "—"}</code>
                              </td>
                              <td>
                                <code>{v.request_id}</code>
                              </td>
                              <td>{new Date(v.created_at).toLocaleString()}</td>
                            </tr>
                          ))}
                        </tbody>
                      </>
                    )}
                  </table>
                </div>
              )}
              {["users", "servers", "orders", "audit"].includes(tab) && (
                <div className="admin-pagination">
                  <button
                    className="button secondary"
                    disabled={offset === 0}
                    onClick={() => setOffset((v) => Math.max(0, v - 50))}
                  >
                    Previous
                  </button>
                  <span>Page {offset / 50 + 1}</span>
                  <button
                    className="button secondary"
                    disabled={rows.length < 50}
                    onClick={() => setOffset((v) => v + 50)}
                  >
                    Next
                  </button>
                </div>
              )}
            </section>
          )}
          {action && (
            <LifecycleDialog
              action={action}
              busy={busy}
              error={error}
              onCancel={() => setAction(null)}
              onConfirm={lifecycle}
            />
          )}
        </>
      )}
    </Shell>
  );
}
function LifecycleDialog({ action, busy, error, onCancel, onConfirm }) {
  const ref = useRef(null);
  const [reason, setReason] = useState("");
  const [recovery, setRecovery] = useState(false);
  useEffect(() => {
    const el = ref.current;
    el.showModal();
    return () => el.close();
  }, []);
  const title = {
    retry: "Retry server setup",
    suspend: "Suspend this server",
    unsuspend: "Unsuspend this server",
    terminate: "Permanently terminate this server",
  }[action.kind];
  return (
    <dialog
      ref={ref}
      className="confirm-dialog"
      aria-label={title}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <h2>{title}</h2>
      <p>{action.server.name}</p>
      {action.kind === "retry" && <JobHelp server={action.server} />}
      {action.kind === "terminate" && (
        <p>
          This deletes the Minecraft container and all server data. The subscription ends
          immediately. This does not issue a refund.
        </p>
      )}
      <ErrorMessage error={error} />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onConfirm({
            reason,
            ...(action.kind === "terminate" ? { confirmation: action.server.id } : {}),
            ...(recovery
              ? {
                  recovery_confirmation: "PANEL_OPERATIONS_FINISHED_AND_ABSENT",
                }
              : {}),
          });
        }}
      >
        <label className="field">
          Reason
          <textarea
            required
            minLength={3}
            maxLength={500}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </label>
        {action.kind === "retry" && action.server.job_error === "panel_create_outcome_unknown" && (
          <label className="setting-toggle">
            <span>
              After inspecting Panel, I confirm all create operations finished and the requested
              server is absent.
            </span>
            <input
              type="checkbox"
              checked={recovery}
              onChange={(e) => setRecovery(e.target.checked)}
            />
          </label>
        )}
        <div className="dialog-actions">
          <button type="button" className="button secondary" disabled={busy} onClick={onCancel}>
            Cancel
          </button>
          <button
            className={`button ${action.kind === "terminate" ? "danger" : "primary"}`}
            disabled={
              busy ||
              reason.trim().length < 3 ||
              (action.kind === "retry" &&
                action.server.job_error === "panel_create_outcome_unknown" &&
                !recovery)
            }
          >
            {busy ? "Queuing…" : "Confirm action"}
          </button>
        </div>
      </form>
    </dialog>
  );
}

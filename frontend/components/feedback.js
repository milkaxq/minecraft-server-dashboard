"use client";
import { Select } from "./select";
import { useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { UnsavedChangesGuard } from "./unsaved-changes";
import { Shell, PageHeading, ErrorMessage, Loading } from "./ui";

const categories = {
  BUG: "Something is broken",
  QUESTION: "I need help",
  IDEA: "An improvement idea",
};
const statuses = { NEW: "New", REVIEWED: "Reviewed", RESOLVED: "Resolved" };

export function Feedback() {
  const { api } = useAuth();
  const params = useSearchParams();
  const servers = useResource("/servers");
  const [offset, setOffset] = useState(0);
  const reports = useResource(`/feedback?offset=${offset}`, 15000);
  const [category, setCategory] = useState("BUG");
  const [serverId, setServerId] = useState(null);
  const [title, setTitle] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const submission = useRef(null);
  const inFlight = useRef(false);
  const requested = params.get("server");
  const reference = /^[0-9a-f-]{36}$/i.test(params.get("reference") || "")
    ? params.get("reference")
    : null;
  const sourcePage = params.get("page") || "";
  const safePage =
    /^\/(?:dashboard|servers(?:\/new|\/[0-9a-f-]{36}(?:\/(?:plugins|mods|files|console|backups|settings|worlds|members|players|addons\/simple-voice-chat))?)?|billing|account|guide|admin|feedback|login|register)?$/i.test(
      sourcePage,
    )
      ? sourcePage
      : "";
  const allowedStates = [
    "running",
    "offline",
    "starting",
    "stopping",
    "unavailable",
    "RUNNING",
    "PENDING_PAYMENT",
    "PAYMENT_CONFIRMED",
    "PROVISIONING",
    "SUSPENDED",
    "FAILED",
    "CANCELLED",
  ];
  const observedState = allowedStates.includes(params.get("observed_state"))
    ? params.get("observed_state")
    : "";
  const rawCheck = params.get("checked_at") || "";
  const checkedAt =
    rawCheck.length <= 40 &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(rawCheck) &&
    Number.isFinite(Date.parse(rawCheck))
      ? new Date(rawCheck).toISOString()
      : "";
  const healthContext = observedState
    ? `\n\nDashboard context (reported by browser):\nDisplayed state: ${observedState}${checkedAt ? `\nStartup check time: ${checkedAt}` : ""}`
    : "";
  const selection = serverId ?? (servers.data?.some((s) => s.id === requested) ? requested : "");
  const serverUnavailable =
    servers.loading || (!!servers.error && (!!selection || (serverId === null && !!requested)));
  async function submit(e) {
    e.preventDefault();
    if (inFlight.current || serverUnavailable) return;
    if (title.trim().length < 3 || message.trim().length < 10) {
      setError(
        new Error("Add a short summary and at least 10 characters describing what happened."),
      );
      return;
    }
    inFlight.current = true;
    setBusy(true);
    setError(null);
    setNotice("");
    const body = {
      category,
      title: title.trim(),
      message: message.trim() + healthContext,
      server_id: selection || null,
      request_reference: reference,
      source_page: safePage,
    };
    const fingerprint = JSON.stringify(body);
    if (submission.current?.fingerprint !== fingerprint)
      submission.current = { fingerprint, key: crypto.randomUUID() };
    try {
      const report = await api("/feedback", { method: "POST", body, key: submission.current.key });
      setNotice(`Report ${report.id.slice(0, 8)} received. Replies appear below.`);
      setTitle("");
      setMessage("");
      submission.current = null;
      setOffset(0);
      reports.refresh();
    } catch (err) {
      setError(err);
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  return (
    <Shell>
      <UnsavedChangesGuard dirty={!!title.trim() || !!message.trim()} />
      <PageHeading eyebrow="PRIVATE BETA" title="Help us improve." />
      <section className="panel management-panel feedback-panel">
        <h2>Send a report</h2>
        <p className="field-help">
          Private to you and your host. Leave out passwords and private links.
        </p>
        <ErrorMessage error={error || servers.error} />
        {servers.error && (
          <button
            className="button secondary"
            disabled={busy || servers.loading || servers.error.status === 429}
            onClick={servers.refresh}
          >
            Refresh servers
          </button>
        )}
        {notice && (
          <p className="notice success" role="status">
            {notice}
          </p>
        )}
        {(reference || safePage) && (
          <p className="feedback-context">
            Included: {safePage && <code>{safePage}</code>}{" "}
            {reference && <span>Support reference: {reference}</span>}. No logs or files are
            attached.
          </p>
        )}
        {healthContext && (
          <p className="feedback-context">
            Dashboard state: {observedState}
            {checkedAt ? ` · Checked: ${checkedAt}` : ""}.
          </p>
        )}
        <form className="feedback-form" onSubmit={submit}>
          <div className="feedback-fields">
            <label className="field">
              What kind of report?
              <Select
                aria-label="What kind of report?"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                disabled={busy}
              >
                {Object.entries(categories).map(([v, label]) => (
                  <option key={v} value={v}>
                    {label}
                  </option>
                ))}
              </Select>
            </label>
            <label className="field">
              Related server (optional)
              <Select
                aria-label="Related server (optional)"
                value={selection}
                onChange={(e) => setServerId(e.target.value)}
                disabled={busy || servers.loading}
              >
                <option value="">Dashboard or general feedback</option>
                {servers.data?.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </Select>
            </label>
          </div>
          <label className="field">
            Short summary
            <input
              required
              minLength={3}
              maxLength={120}
              value={title}
              disabled={busy}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="For example: I cannot join my server"
            />
          </label>
          <label className="field">
            What happened?
            <textarea
              required
              minLength={10}
              maxLength={4000 - healthContext.length}
              rows={6}
              value={message}
              disabled={busy}
              onChange={(e) => setMessage(e.target.value)}
              placeholder="What were you trying to do? Include the error you saw."
            />
          </label>
          <button className="button primary" disabled={busy || serverUnavailable}>
            {busy ? "Sending…" : "Send report"}
          </button>
        </form>
      </section>
      <section className="panel management-panel feedback-panel">
        <h2>Your reports</h2>
        <ErrorMessage error={reports.error} />
        {reports.error && (
          <button
            className="button secondary"
            disabled={busy || reports.loading || reports.error.status === 429}
            onClick={reports.refresh}
          >
            Refresh reports
          </button>
        )}
        <ReportList
          error={reports.error}
          rows={reports.data || []}
          loading={reports.loading}
          offset={offset}
          setOffset={setOffset}
        />
      </section>
    </Shell>
  );
}

export function ReportList({ rows, loading, offset, setOffset, admin = false, refresh, error }) {
  return (
    <>
      {loading ? (
        <Loading />
      ) : !rows.length ? (
        <p className="management-empty">
          {error ? "Reports unavailable." : "No reports on this page."}
        </p>
      ) : (
        <div className="feedback-list">
          {rows.map((report) => (
            <ReportCard key={report.id} report={report} admin={admin} refresh={refresh} />
          ))}
        </div>
      )}
      {(offset > 0 || rows.length >= 50) && (
        <div className="admin-pagination">
          <button
            className="button secondary"
            disabled={offset === 0 || loading || !!error}
            onClick={() => setOffset(Math.max(0, offset - 50))}
          >
            Previous
          </button>
          <span>Page {offset / 50 + 1}</span>
          <button
            className="button secondary"
            disabled={rows.length < 50 || loading || !!error || offset >= 100000}
            onClick={() => setOffset(offset + 50)}
          >
            Next
          </button>
        </div>
      )}
    </>
  );
}
function ReportCard({ report, admin, refresh }) {
  const { api } = useAuth();
  const [edit, setEdit] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  async function save(e) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await api(`/admin/feedback/${report.id}`, { method: "PUT", body: edit });
      setEdit(null);
      refresh();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <article className="feedback-card" aria-label={`Report: ${report.title}`}>
      <div className="feedback-card-heading">
        <h3>{report.title}</h3>
        <span className="feedback-status">{statuses[report.status] || report.status}</span>
      </div>
      <p className="field-help">
        {categories[report.category]} · {new Date(report.created_at).toLocaleString()} · Reference{" "}
        {report.id.slice(0, 8)}
      </p>
      {admin && (
        <p className="field-help">
          From {report.author_name} ({report.author_email})
          {report.server_id ? ` · Server ${report.server_id}` : ""}
        </p>
      )}
      <p className="feedback-text">{report.message}</p>
      {(report.request_reference || report.source_page) && (
        <p className="feedback-context">
          {report.source_page && <code>Page: {report.source_page}</code>}
          {report.request_reference && <span>Support reference: {report.request_reference}</span>}
        </p>
      )}
      {report.response && (
        <div className="feedback-response">
          <strong>Reply from your host</strong>
          <p className="feedback-text">{report.response}</p>
        </div>
      )}
      {admin &&
        (edit ? (
          <form className="feedback-form" onSubmit={save}>
            <ErrorMessage error={error} />
            <label className="field">
              Report status
              <Select
                aria-label="Report status"
                value={edit.status}
                disabled={busy}
                onChange={(e) => setEdit({ ...edit, status: e.target.value })}
              >
                {Object.entries(statuses).map(([v, label]) => (
                  <option key={v} value={v}>
                    {label}
                  </option>
                ))}
              </Select>
            </label>
            <label className="field">
              Reply to tester
              <textarea
                rows={4}
                maxLength={2000}
                value={edit.response}
                disabled={busy}
                onChange={(e) => setEdit({ ...edit, response: e.target.value })}
              />
            </label>
            <p className="field-help">
              The tester can see this reply in their reports. No notification email is sent.
            </p>
            <div className="button-row">
              <button className="button primary" disabled={busy}>
                {busy ? "Saving…" : "Save response"}
              </button>
              <button
                type="button"
                className="button secondary"
                disabled={busy}
                onClick={() => {
                  setEdit(null);
                  setError(null);
                }}
              >
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <button
            className="button secondary"
            onClick={() =>
              setEdit({
                status: report.status,
                response: report.response,
                revision: report.revision,
              })
            }
          >
            Review report
          </button>
        ))}
    </article>
  );
}

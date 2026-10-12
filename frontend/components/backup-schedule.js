"use client";
import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { ErrorMessage, Loading } from "./ui";

const warnings = {
  insufficient_allowance:
    "Daily backups need at least two backup slots to keep your last successful copy safe.",
  server_unavailable:
    "Your server is unavailable. New daily backups will wait until it is active again.",
  capacity_full:
    "No safe backup slot is available. Manual backups and your last successful automatic backup will be kept.",
  needs_inspection:
    "A backup request needs inspection by your host. Automatic creation is paused to avoid duplicates.",
  pending_inspection:
    "Another backup is unfinished or needs inspection. This daily run was skipped.",
  panel_unavailable:
    "The backup service could not be reached. Check the recent run status or contact your host.",
  panel_rejected: "The backup service declined this request. Contact your host if this continues.",
  backup_failed:
    "The last automatic backup failed. Your earlier successful copy is still preserved.",
  schedule_changed: "This run was skipped because the schedule or server owner changed.",
};
const states = {
  QUEUED: "Queued",
  CREATING: "Creating…",
  COMPLETED: "Completed",
  FAILED: "Failed",
  SKIPPED: "Skipped",
  NEEDS_INSPECTION: "Needs inspection",
};
function dateLabel(value, timezone) {
  if (!value) return "—";
  return `${new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: timezone,
  }).format(new Date(value))} (${timezone})`;
}

export function BackupSchedule({ id, limit }) {
  const { api } = useAuth();
  const resource = useResource(`/servers/${id}/backup-schedule`, 15000);
  const [action, setAction] = useState({ busy: false, error: null, notice: "", conflict: false });
  async function save(input) {
    setAction({ busy: true, error: null, notice: "", conflict: false });
    try {
      await api(`/servers/${id}/backup-schedule`, { method: "PUT", body: input });
      setAction({ busy: false, error: null, notice: "Schedule saved.", conflict: false });
      resource.refresh();
    } catch (error) {
      setAction({ busy: false, error, notice: "", conflict: error.status === 409 });
    }
  }
  const schedule = resource.data;
  return (
    <section className="panel management-panel daily-backups" aria-labelledby="daily-backups-title">
      <div className="section-heading">
        <div>
          <h2 id="daily-backups-title">Daily backups</h2>
          <span>Save a daily copy of your world, settings and server files.</span>
        </div>
        {schedule && <span className="schedule-state">{schedule.enabled ? "On" : "Off"}</span>}
      </div>
      <ErrorMessage error={resource.error || action.error} />
      {action.notice && (
        <p role="status" className="notice">
          {action.notice}
        </p>
      )}
      {action.conflict && (
        <button
          className="button secondary"
          onClick={() => {
            setAction({ busy: false, error: null, notice: "", conflict: false });
            resource.refresh();
          }}
        >
          Reload schedule
        </button>
      )}
      {resource.loading ? (
        <Loading message="Loading backup schedule…" />
      ) : (
        schedule && (
          <>
            {!schedule.available && (
              <p className="notice">
                Your host has not enabled daily backups yet. You can still turn an existing schedule
                off.
              </p>
            )}
            {schedule.warning && (
              <p className="notice" role="status">
                {warnings[schedule.warning] ||
                  "A backup needs your host’s attention. Check the recent run status."}
              </p>
            )}
            <dl className="schedule-summary">
              <div>
                <dt>Next backup</dt>
                <dd>
                  {schedule.enabled ? dateLabel(schedule.next_run, schedule.timezone) : "Off"}
                </dd>
              </div>
              <div>
                <dt>Last successful backup</dt>
                <dd>{dateLabel(schedule.last_success, schedule.timezone)}</dd>
              </div>
            </dl>
            {schedule.can_manage ? (
              <ScheduleEditor
                key={id}
                schedule={schedule}
                limit={limit}
                busy={action.busy}
                save={save}
              />
            ) : (
              <p className="field-help">Only the server owner can change this schedule.</p>
            )}
            <p className="field-help">
              Enabling daily backups allows older automatic copies to be rotated when space is
              needed. Manual backups and the last successful automatic copy are preserved. Online
              backups may not include consistent data from every plugin or external database.
            </p>
            {!!schedule.runs.length && (
              <div className="schedule-runs">
                <h3>Recent daily runs</h3>
                <ul>
                  {schedule.runs.map((run) => (
                    <li key={run.id}>
                      <span>{dateLabel(run.scheduled_at, schedule.timezone)}</span>
                      <strong>{states[run.status] || "Needs attention"}</strong>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </>
        )
      )}
    </section>
  );
}

function ScheduleEditor({ schedule, limit, busy, save }) {
  const [revision] = useState(schedule.revision);
  const [draft, setDraft] = useState(() => ({
    enabled: schedule.enabled,
    time: `${String(schedule.hour).padStart(2, "0")}:${String(schedule.minute).padStart(2, "0")}`,
    timezone:
      schedule.revision === 0
        ? Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"
        : schedule.timezone,
  }));
  function submit(event) {
    event.preventDefault();
    const [hour, minute] = draft.time.split(":").map(Number);
    save({
      enabled: draft.enabled,
      hour,
      minute,
      timezone: draft.timezone.trim(),
      revision,
    });
  }
  return (
    <form className="schedule-form" onSubmit={submit}>
      <label className="schedule-toggle">
        <input
          type="checkbox"
          checked={draft.enabled}
          disabled={busy || ((!schedule.available || limit < 2) && !draft.enabled)}
          onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })}
        />
        Enable daily backups
      </label>
      {limit < 2 && (
        <p className="notice">Your plan needs at least two backup slots for daily backups.</p>
      )}
      <div className="schedule-fields">
        <label className="field">
          Backup time
          <input
            type="time"
            required
            value={draft.time}
            disabled={busy}
            onChange={(event) => setDraft({ ...draft, time: event.target.value })}
          />
        </label>
        <label className="field">
          Timezone
          <input
            type="text"
            required
            maxLength={100}
            value={draft.timezone}
            disabled={busy}
            onChange={(event) => setDraft({ ...draft, timezone: event.target.value })}
            placeholder="Europe/Berlin"
          />
        </label>
      </div>
      <p className="field-help">
        Use a timezone such as Europe/Berlin or Asia/Ashgabat. The backup follows its local time,
        including daylight saving changes.
      </p>
      <button
        className="button primary"
        disabled={busy || (draft.enabled && (!schedule.available || limit < 2))}
      >
        {busy ? "Saving…" : "Save schedule"}
      </button>
    </form>
  );
}

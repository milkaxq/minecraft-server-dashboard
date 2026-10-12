# Daily automatic Minecraft backups

## Purpose and scope

Let a server owner enable one daily backup, choose its time and timezone, and
see the next run, last successful completion and any problem. Extend the
existing Backups page and Pterodactyl Client API integration. Plugin updates
are a later task.

Automatic backups are off by default. Enabling the schedule is consent to
rotate eligible automatic backups. Manual backups are never rotated. Restore
continues to require the existing explicit confirmation and stopped server.

## Customer experience

- Add a Daily backups card above the existing backup history.
- Owners can enable, change or disable the schedule. Other members with backup
  permission can view its status. Moderators cannot access it.
- Use an explicit IANA timezone, initially the browser's timezone, and a daily
  local time. Show the timezone alongside both next run and last success.
- Show automatic/manual labels using database provenance, not backup names.
- Show queued, creating, completed, failed, skipped and needs-inspection states.
  Receiving a Panel UUID does not count as successful completion.
- Display warnings for insufficient allowance, locked backups, suspension,
  failed runs and ambiguous Panel responses. Preserve the last success when
  a later attempt fails.
- Explain that backups can be made while online, but an online Minecraft
  backup is not a guarantee that every plugin's external data is consistent.
  Do not issue Minecraft commands or stop the server automatically.

## Architecture

Persist schedules and runs in PostgreSQL. Add a bounded backup scheduler to
the existing worker process, independent of the provisioning queue. Use
Pterodactyl's existing backup operations; never read host game directories.

Keep schedule business logic in an internal backup scheduling package. Reuse
the management service's server locking and allowance checks so manual and
automatic creation cannot consume the same remaining slot. Do not change
provisioning job types to represent backup completion.

The worker needs a Client API credential permitted to manage backups. Add an
explicit worker backup credential through secrets/configuration; do not reuse
the Application API key or the installation verifier credential implicitly.
Disabled scheduling requires no new credential. An enabled scheduler without
its credential fails configuration validation rather than silently doing no
work. Logs contain IDs and controlled error codes, not credentials or Panel
response bodies.

## Database and scheduling

Add one schedule per application server UUID: enabled, local hour/minute,
IANA timezone, revision, next scheduled instant and last successful run.
Store durable runs with schedule revision, scheduled instant, status, lease
expiry, attempts, Panel server identity, backup UUID, timestamps and controlled
failure code. A unique (server UUID, scheduled instant) prevents duplicate
work when workers overlap or restart.

Claim due work using PostgreSQL locking and renewable leases. Check the
schedule revision and server eligibility again before issuing a Panel request.
Disabling a schedule prevents new requests; a request already sent is still
monitored to a terminal outcome.

Recalculate daily instants with timezone data. In a daylight-saving gap, use
the first valid local instant after the chosen time; in a repeated hour, run
once using the earlier instant. After downtime, run at most one catch-up
backup, then advance to the next future daily instant.

Only active, provisioned, non-deleting servers are eligible. Suspension or
deletion prevents creation and rotation. Recheck the current owner, server
identity and purchased backup limit; none are supplied by the browser.

## Creation, completion and retention

1. Persist creation intent and its deterministic operation key before calling
   Panel. Use the shared server lock for the allowance decision and mutation.
2. List current Panel backups. Reserve room for the new backup without
   exceeding the server's purchased allowance.
3. When full, delete only completed, unlocked automatic backups whose UUIDs
   were recorded by this scheduler for this Panel server. Preserve the most
   recent successful backup until its replacement succeeds. Do not identify
   automatic backups by a name prefix.
4. If no safe candidate exists, skip with a visible capacity warning. A plan
   with fewer than two backup slots cannot enable safe automatic rotation.
   Manual backups can also leave insufficient room; never delete them to fix
   this condition.
5. Create the backup through Panel, persist its returned UUID and monitor its
   completion. Only a completed, successful backup advances last-success.
6. Retain as many successful automatic backups as the plan and manual backups
   safely allow, with room for replacement. Clean up completed failed
   automatic attempts when doing so is safe; never delete in-progress or
   locked backups.

An uncertain create/delete response must not trigger a blind retry. Keep its
intent and reconcile against Panel. If identity or outcome cannot be established,
mark needs-inspection and block further automatic creation on that server.
Transient failures before a mutation can retry with bounded backoff.

Automatic deletions and creation/status transitions create audit entries with
the run ID and an explicit automation marker. Retain run history even when
the corresponding automatic backup has been rotated.

## REST API

- GET /servers/:id/backup-schedule: schedule and recent run status; requires
  backup permission.
- PUT /servers/:id/backup-schedule: enable/time/timezone changes; requires
  ownership, session CSRF protection and an optimistic revision.
- Existing backup history includes optional automatic provenance, without
  changing manual download, delete or restore behavior.

Validate local times and IANA timezone identifiers. Return a conflict on stale
revisions. Resolve plan limits, ownership and Panel identities from PostgreSQL.
Apply existing request limits, structured errors and authorization conventions.

## Validation and delivery

Write tests before implementation for timezone/DST behavior, catch-up bounds,
ownership and revision checks, concurrent claims, worker restart/idempotency,
manual/automatic allowance races, safe retention, locked/in-progress backups,
ambiguous responses and success/failure reporting.

Test the Backups UI on phone and laptop sizes: default-off state, enabling and
disabling, explicit timezone, stale changes, permission restrictions and warnings.
Run the full Go suite with PostgreSQL, affected race checks, lint/formatting,
frontend production build and browser tests. Request code review before commit.

Apply the migration and deploy backend, worker and frontend. Keep automatic
backups off for existing servers. Verify application readiness after deployment.
No live backup/restore test should require creating a Minecraft world or
accepting the EULA on the customer's behalf.

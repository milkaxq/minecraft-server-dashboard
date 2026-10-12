# Daily Automatic Backups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give owners a daily backup schedule that preserves manual backups and the latest successful automatic backup.

**Architecture:** Persist schedules, leased runs and remote mutation intents in PostgreSQL. An independent runner in the existing worker uses the same server advisory lock as manual management and calls only Pterodactyl Client API backup operations. Extend the existing Backups page with owner controls and permission-filtered status.

**Tech Stack:** Go, pgx/PostgreSQL, existing Pterodactyl clients, Next.js/React, Playwright; no new runtime dependency.

**Spec:** `docs/superpowers/specs/2026-10-13-automatic-backups-design.md`

## Global Constraints

- Automatic backups are off by default.
- Owners configure schedules; members with backup permission read status; moderators are denied.
- Manual backups are never rotated. Restore requires existing confirmation and a stopped server.
- Fewer than two backup slots cannot enable safe automatic rotation.
- Never delete locked or in-progress backups, or the latest successful automatic backup before its replacement succeeds.
- Never identify automatic backups by name prefixes; use recorded UUID and Panel server identity.
- Do not stop servers, issue console commands, read host game directories or accept the EULA.
- Use an explicit worker Client API credential; never implicitly reuse Application or installation credentials.
- Uncertain mutation outcomes block further automatic creation until reconciled.
- Log controlled codes and IDs, never credentials, signed URLs or Panel response bodies.

## Review Focus

1. DST gap/repeated hour: one daily run at the specified deterministic instant (Task 1).
2. Worker crash after remote creation: no second creation, including after lease expiry (Task 3).
3. Manual creation competing for the last slot: combined count never exceeds allowance (Task 3).
4. Owner disables or server is suspended during a claim: no subsequent creation or rotation; monitor existing creation (Tasks 2–3).
5. Panel returns a UUID but later fails: retain previous success and show failure (Tasks 3–4).

## File map

Create `internal/backups/{schedule.go,schedule_test.go,service.go,service_test.go,retention.go,retention_test.go,worker.go,worker_test.go}` for scheduling, authorized persistence, rotation policy and the leased runner. Create `migrations/014_backup_schedules.sql`, `internal/httpapi/backup_schedule.go`, `internal/httpapi/backup_schedule_test.go`, and `frontend/components/backup-schedule.js`.

Modify `internal/management/backups.go` only for unresolved scheduled mutation guards and provenance; `internal/httpapi/{api.go,management.go}`, `cmd/{api,worker}/main.go` for wiring; `internal/config/worker.go` and its tests for explicit secrets; `frontend/components/management.js`, `frontend/app/globals.css`, `frontend/tests/dashboard.spec.mjs` for the card and responsive checks. Update `.env.example`, `docker-compose.yml`, and `README.md` for operator configuration. Do not restructure unrelated code.

### Task 1: Deterministic local-time scheduling

**Files:** Create `internal/backups/schedule.go`, `internal/backups/schedule_test.go`.

**Interfaces:** Produce `NextDaily(after time.Time, hour, minute int, zone string) (time.Time, error)`. Returned instant is strictly after `after`. Embed timezone data with `_ "time/tzdata"` so minimal worker images work.

- [ ] Write failing table tests for invalid hours/minutes, unknown zones, a normal day, a non-hour-offset zone, DST gaps and repeated hours. Include this exact regression:

```go
func TestNextDailyDSTGap(t *testing.T) {
    after := time.Date(2026, 3, 28, 23, 0, 0, 0, time.UTC)
    got, err := NextDaily(after, 2, 30, "Europe/Berlin")
    want := time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)
    if err != nil || !got.Equal(want) { t.Fatalf("got %v, %v; want %v", got, err, want) }
}
```

- [ ] Run `go test ./internal/backups -run TestNextDaily -count=1`; expect undefined `NextDaily` initially.
- [ ] Implement validation and daily candidate enumeration. For each local calendar date, enumerate UTC minutes across the date's possible offset range and compare local date/time. Select the earliest exact match; if none, select the first valid local minute later than the requested time. Determine that date's canonical instant before comparing it with `after`, so a repeated hour's second occurrence is never selected. Iterate calendar dates using `AddDate`, not a fixed 24-hour duration. Return the first canonical instant strictly after `after`.

```go
if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
    return time.Time{}, server.ErrInvalid
}
loc, err := time.LoadLocation(zone)
if err != nil { return time.Time{}, server.ErrInvalid }
```

- [ ] Pin autumn behavior: Berlin 2026-10-25 02:30 selects 00:30 UTC; asking after that instant selects the next calendar day, never 01:30 UTC. Include `Asia/Kathmandu` and `Australia/Lord_Howe` cases.
- [ ] Re-run the tests and commit `feat: calculate deterministic daily backup times`.

### Task 2: Durable schedules and authorized API

**Files:** Create migration, `internal/backups/service.go`, `service_test.go`, HTTP schedule handlers/tests. Modify API wiring and route registration.

**Interfaces:** `Service{DB *pgxpool.Pool, Control *control.Service}`; `Get(ctx context.Context, actor, serverID uuid.UUID) (View, error)`; `Put(ctx context.Context, actor, serverID uuid.UUID, in Update, requestID string) (View, error)`. `Update` contains `Enabled bool`, `Hour,Minute int`, `Timezone string`, `Revision int64`. `View` contains these schedule fields plus `CanManage bool`, `NextRun,LastSuccess *time.Time`, `Runs []Run`, and `Warning string`. Define `ErrRevision` and `ErrAllowance`; map both to 409 with controlled messages. JSON uses snake_case.

- [ ] Add PostgreSQL tests using existing fixture conventions: outsider GET/PUT denied; ADMIN GET permitted but PUT denied; moderator denied; owner enable with limit 1 rejected; stale revision rejected; disabling clears next-run without deleting run history. Run them first expecting missing service methods.
- [ ] Add schema. Use server UUID foreign keys, checked hour/minute/status values and indexes below. Schedules have enabled default false, revision default 0, timezone, next_run, last_success and timestamps. Runs retain schedule revision, scheduled instant, owner snapshot, Panel identity, backup UUID, lease token/expiry, attempts, error code and timestamps. Remote intents retain run ID, action CREATE/DELETE, target backup UUID, deterministic operation UUID, state INTENDED/CONFIRMED/UNKNOWN and result UUID.

```sql
CREATE UNIQUE INDEX backup_runs_one_instant
 ON backup_runs(server_id, scheduled_at);
CREATE INDEX backup_schedules_due ON backup_schedules(next_run)
 WHERE enabled;
CREATE INDEX backup_runs_claimable ON backup_runs(status, lease_expires_at);
```

- [ ] GET resolves membership using server/owner/member records without requiring live Panel availability, so suspended owners can disable schedules and see warnings. Return a default disabled view when no schedule exists. Query recent runs with a fixed limit of 20; never return Panel identifiers or raw failures.
- [ ] PUT validates time/timezone through `NextDaily`, obtains `server:<UUID>` transaction advisory lock, rechecks owner/current allowance/lifecycle, and updates with optimistic revision. A missing row accepts only revision 0. Enabling requires active provisioned non-deleting eligibility and at least 2 slots; disabling remains permitted when suspended. Increment revision for each successful update, calculate next-run in UTC and record an audit entry in the same transaction.

```sql
UPDATE backup_schedules
 SET enabled=$2,hour=$3,minute=$4,timezone=$5,
     revision=revision+1,next_run=$6,updated_at=now()
 WHERE server_id=$1 AND revision=$7
 RETURNING revision;
```

- [ ] Register GET/PUT `/servers/{id}/backup-schedule` with existing `protected` and `liveLimit` middleware. Use existing bounded JSON decoding, UUID parsing, session actor and request-ID helpers. Test PUT without CSRF returns 403, forged resource fields are rejected, and revision conflicts return 409.
- [ ] Run `go test ./internal/backups ./internal/httpapi ./migrations -count=1` with `TEST_DATABASE_URL` set. Commit `feat: persist owner-controlled daily backup schedules`.

### Task 3: Leased runner, safe retention and remote uncertainty

**Files:** Create retention and worker files/tests. Modify `internal/management/backups.go` and its integration tests.

**Interfaces:** `Worker{DB *pgxpool.Pool, Panel pterodactyl.ManagementClient, Control *control.Service, Log *slog.Logger}` with `Run(ctx context.Context) error` and `Tick(ctx context.Context, now time.Time) error`. `RotationCandidate(items []pterodactyl.Backup, owned map[string]bool) (string, bool)` is a pure policy function: only recorded, completed, unlocked candidates; always preserve the latest successful recorded backup, including when locked. Worker uses existing `Control.Access` with the current owner and `server.Backups` to recheck entitlement and lifecycle.

- [ ] Write failing policy tests with manual, failed, locked and in-progress backups. Assert names like `Automatic backup` convey no ownership. Assert one last-good plus one manual backup has no candidate; one last-good plus one older successful automatic backup selects the older copy. Run `go test ./internal/backups -run TestRotation -count=1` before implementation.
- [ ] Implement the pure policy and transactional claims. On a 15-second tick select due schedule rows `FOR UPDATE SKIP LOCKED`, insert at most one catch-up run per server with unique scheduled instant, and advance next-run strictly past now using `NextDaily`. Cap each tick at 10 runs. Claim run with a unique lease token for 90 seconds, renew every 20 seconds, and fence every update on the token. Bound remote calls at 30 seconds. Do not hold a pooled connection while waiting for the next tick.

```sql
UPDATE backup_runs SET lease_token=$2,lease_expires_at=$3,attempts=attempts+1
 WHERE id=$1 AND (lease_expires_at IS NULL OR lease_expires_at<$4)
 RETURNING id;
```

- [ ] Before each new remote mutation acquire the same session advisory lock `server:<UUID>` used by `management.Service.mutate`. Re-read schedule revision/enabled, owner, entitlement, lifecycle, Panel identity and stored allowance. Check `Panel.Ready`. A stale revision or disabled schedule skips unissued work; suspended/deleting state permits monitoring only. Release locks using bounded background cleanup and close the connection if unlock cannot be confirmed, matching management's existing pattern.
- [ ] List backups and block when any manual `backup.create`/`world.backup` operation or scheduled mutation has unresolved intent. When full, choose only a safe rotation candidate. Record DELETE intent and automation audit transaction before calling Panel; confirm deletion with list results before considering the slot free. If no candidate exists, mark skipped with a controlled capacity warning. Repeat until a free slot exists, never exceeding purchased allowance.
- [ ] Commit CREATE intent and deterministic operation key before the request. Use a diagnostic name containing run UUID, but never treat the name as proof of ownership. Persist a validated returned UUID in both intent/run. Poll `ListBackups` to terminal completion. `completed_at != nil && is_successful` is success; completed unsuccessful is failed. Advance schedule last-success only for a successful backup matching the recorded Panel identity. Audit all transitions and rotations with `automation:true` and run ID.
- [ ] Treat post-send timeout, invalid UUID, lost response or lost persistence as UNKNOWN/needs-inspection. On lease recovery, never resend an intended CREATE: a diagnostic name alone is insufficient evidence, so leave needs-inspection when no durable returned UUID exists. Reconcile uncertain DELETE by UUID absence/presence; never retry a delete of a changed/locked target blindly. Block both automatic and manual new backup creation while scheduled CREATE/DELETE uncertainty exists, preserving safe allowance decisions. Retry only failures known to occur before mutation, with attempts capped at 3 and delays of 30/120/300 seconds; exhausted runs fail visibly.
- [ ] Completion monitoring survives disabling and worker restart. Keep latest successful even if a later run fails; clean completed failed owned backups only under the same lock and policy. Never delete an in-progress or locked backup. Keep run/audit records after remote rotation. If a recorded backup disappears before completion, mark needs-inspection rather than successful.
- [ ] Add concurrent PostgreSQL/mock Panel tests: two workers one creation; crash after intent zero retry; crash after UUID polls existing backup; expired lease cannot write after a new claim; manual-vs-automatic count bounded; revision/owner/lifecycle change prevents remote mutation; ambiguous create/delete blocks future creation; failed completion preserves last-success; downtime produces one catch-up; disabling still polls pending completion. Assert remote call counts and recorded audit metadata, not only statuses.
- [ ] Run `go test -race ./internal/backups ./internal/management -count=1` against PostgreSQL. Commit `feat: run safe idempotent automatic backups`.

### Task 4: Explicit worker configuration and customer Backups UI

**Files:** Modify worker config/tests, worker main, `.env.example`, Compose, existing Backups component/CSS/browser tests; create `backup-schedule.js`. Extend existing history response with optional provenance using recorded UUID plus Panel identity.

**Interfaces:** Worker config adds `BackupSchedulerEnabled bool` and `BackupClientKey string`, from `BACKUP_SCHEDULER_ENABLED=false` and `PTERODACTYL_BACKUP_CLIENT_API_KEY`. Card receives `id` and uses existing `useResource`, `api` and error components. GET response is Task 2's `View`. History adds optional `automatic` boolean without changing existing UUID/download/restore fields.

- [ ] Write failing config tests: default disabled needs no key; true without key fails; invalid boolean fails; application/install key alone does not satisfy backup key. Do not include secret values in error output.
- [ ] Wire a separate `NewServerClient` with backup key and control service only when enabled. Start scheduler independently of provisioning queue; cancel/join goroutines before pool close. Operational tick failures log controlled categories and recover on later ticks; configuration failure terminates startup. Pass the explicit variables only to worker Compose environment.

```env
BACKUP_SCHEDULER_ENABLED=false
PTERODACTYL_BACKUP_CLIENT_API_KEY=
```

- [ ] Write browser tests before the card: disabled default, owner enable/save and disable, ADMIN read-only, moderator no access, timezone displayed next to dates, limit<2 warning, stale 409 refresh without silent overwrite, skipped/failed/needs-inspection messages, and last-success unchanged after failure. Mock API through existing test server fixtures; assert submitted revision/time/timezone and absence of plan/Panel fields.
- [ ] Add card above history. Initialize timezone from `Intl.DateTimeFormat().resolvedOptions().timeZone` for an unsaved schedule; preserve saved timezone thereafter. Inputs use a local time control and explicit IANA timezone field. Explain rotation consent, manual preservation, two-slot requirement and online-backup limits. Disable Save during requests, retain input on recoverable errors, show success only after API confirmation. Format UTC instants in selected timezone and label it visibly. Poll status using existing resource mechanism; do not render Panel response bodies or secrets.
- [ ] Add Automatic/Manual labels by backend provenance. Preserve all existing destructive confirmation dialogs. Check phone 390px and laptop 1366px widths with browser assertions for no horizontal overflow and usable controls.
- [ ] Run `go test ./internal/config ./internal/backups ./internal/httpapi -count=1`; run `npm run check`, `npm run build`, `npm test` in `frontend`. Commit `feat: expose daily backup controls and worker configuration`.

### Task 5: Full verification, operator docs and deployment

**Files:** Update `README.md`; fix only issues demonstrated by tests/review in affected files.

- [ ] Document opt-in worker setup, issuing a backup-capable Client API credential, timezone/DST/catch-up rules, two-slot rule, safe rotation and needs-inspection. Document inspection recovery: disable schedule; stop scheduler worker; compare recorded run/intent/Panel IDs; never clear an uncertain creation merely to retry. A confirmed missing remote object or verified recorded UUID is required before an operator repairs the run transactionally with an audit entry. No general automatic recovery claim when Panel cannot establish identity.
- [ ] Format changed Go files with `go tool goimports -local minecraft-hosting -w` followed by exact filenames. Run the full verification once PostgreSQL is available:

```bash
go test -count=1 ./cmd/... ./internal/... ./migrations/...
go test -race -count=1 ./internal/backups ./internal/management
go vet ./...
go tool staticcheck ./...
bash scripts/check-go-format.sh
```

From `frontend`, run `npm run check`, `npm run build`, `npm test`. Expected: all pass, no skipped PostgreSQL checks due to missing `TEST_DATABASE_URL`. If local fixture database must be started, follow existing README test database instructions; never point tests at production.

- [ ] Obtain independent code review under `superpowers:requesting-code-review`, focusing on durable intent, shared locks, lease fencing and permission changes. Resolve material findings and rerun affected checks before claiming completion.
- [ ] Commit and push verified changes. Deploy source through existing archive/SSH workflow without removing Wings/game containers. Apply `docker compose -f docker-compose.yml -f docker-compose.pterodactyl.yml -f docker-compose.vps.yml run --rm migrate` before rebuilding backend/worker/frontend with the established profiles. Keep schedules and global scheduler disabled until an explicit backup-capable secret is configured; never silently copy another key.
- [ ] Verify API readiness and Backups UI over HTTPS. Report deployed commit, test results and whether scheduler is configured. If no eligible existing server exists, explicitly report that live backup completion remains untested; do not create a world or accept EULA to manufacture this check.

## Self-review

All spec sections map to Tasks 1–5. Five Review Focus cases have explicit regression tests. `View`, `Update`, `NextDaily`, `Worker.Tick` and configuration names are consistent across tasks. No new payment/provisioning actions or plugin update work is included. Remote creation reconciliation deliberately stops at needs-inspection when durable identity is unavailable; a name match never grants deletion rights. Existing manual restore safety remains intact.

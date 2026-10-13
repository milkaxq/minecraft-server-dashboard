# Beta safety and launch checks — 2026-10-13

## Changes

- Catalog installs and dedicated plugin/mod uploads offer existing completed backups,
  creating a new backup, or explicit acknowledgement of proceeding without one.
  Pending and failed backups cannot be selected. Full backup allowances show a link
  to manage existing copies; this flow does not delete backups automatically.
- Selected backup UUIDs are verified against this server's Panel backup list before
  mutations. Catalog installs recheck after package downloads. Missing decisions
  and unavailable backup reads are rejected preflight, without leaving an ambiguous
  remote-write intent. Existing management locks, plan quotas and idempotency apply.
- Version changes prepare a backup and world export before opening new-server
  creation. The current world is not converted or overwritten. Older backups do not
  include progress made after their snapshot. Advanced raw file editing remains an
  expert action rather than a promise of automatic protection for every file change.
- Startup help reads only `/logs/latest.log` through Panel, bounded to 1 MB and
  checked against current container uptime when available. It classifies known EULA,
  dependency, Java, addon, world-version, memory and port errors. Unknown or missing
  evidence links to Console. Results never include raw logs, paths from exceptions,
  environment variables or credentials. Recognized issues are guidance, not a
  guarantee of server health or an automatic fix.
- Feedback links attach the server, allowlisted page path and UUID support reference.
  URL queries/fragments and raw logs are excluded. Reports remain private to their
  author and hosting admins; context grants no additional access. Migration
  `016_feedback_context.sql` adds the metadata columns. Context participates in
  submission idempotency. Existing feedback quotas and audited responses remain.

## API changes

Known backup preflight failures carry explicit error codes so retries can use a
fresh request key. Uncertain remote writes retain their original key and cannot be
blindly repeated.

Catalog install JSON includes either `backup` (completed owned backup UUID) with
`acknowledge_no_backup: false`, or `backup: null` with
`acknowledge_no_backup: true`. The existing revision and UUID Idempotency-Key remain
required. Dedicated plugin/mod binary uploads use the same two choice fields in
query parameters. Arbitrary resource allocations and Panel IDs remain unsupported.

`GET /servers/{id}/startup-check` requires server view permission. It returns only
safe issue codes, explanations, check time and a request reference. Changing settings,
installing addons and creating/restoring backups retain their existing permissions.

## Validation evidence

| Check | Result |
| --- | --- |
| Full Go/PostgreSQL suite with race detector | Passed across cmd, internal and migrations packages |
| Account recovery, token expiry/reuse and session invalidation | Automated tests passed with isolated accounts and test delivery |
| Foreign-server access and OWNER/ADMIN/MODERATOR authorization | Automated tests passed |
| Concurrent node allocation and provisioning/payment idempotency | Automated tests passed |
| Backup limits, completion, ownership and explicit install choice | Automated tests passed |
| Feedback context validation, privacy and retry idempotency | Automated tests passed |
| Frontend lint, formatting, production build | Passed |
| Browser regression suite | 130 passed after simplifying new-server choices; mobile history/storage, addon update review, notifications and player checklist covered |
| Real Panel backup/restore on a disposable stopped server | Passed: synthetic world marker backed up, changed and restored through Panel |
| Current public Minecraft protocol/connectivity check | Passed: production Minecraft status responds from VPS, 0/10 players |
| Real player joining and authenticating | Passed joining: owner reports SKlauncher, Forge profile, Java 26.3; voice connected and microphone test works (second-person audio still pending) |
| Deployment and production readiness after migration | Passed: migration 016 applied; backend/frontend deployed; HTTPS readiness and authorized startup check pass |

The existing production world has not been stopped, restored or modified for these
checks. Recovery tests do not reset the operator's production password. A real
player login, mod handshake and in-game voice session cannot be established by a
status ping or unit tests.

## Reproduce automated checks

Use a dedicated disposable PostgreSQL test database; never point TEST_DATABASE_URL
at production.

```sh
TEST_DATABASE_URL='postgres://hosting:TEST_PASSWORD@localhost:55434/hosting_test?sslmode=disable' \
  go test -race -count=1 ./cmd/... ./internal/... ./migrations/...
make lint-go
./scripts/check-go-format.sh
npm --prefix frontend run check
npm --prefix frontend run build
npm --prefix frontend test
```

## Live-check evidence

The disposable server `a7124794-3e14-4dd5-b36c-f81bfed421b0` was provisioned
through the regular transactional/job path, kept stopped, and tested through Panel.
A synthetic `/world/beta-restore-check.txt` was backed up, changed, restored, and
verified. No Minecraft EULA was accepted or game process started for this check.
Termination succeeded, Panel returned not-found for its original identifier,
and its node allocation was released. Node usage returned
to the original 2048 MB RAM, 10240 MB disk and 100 CPU units. The existing server
startup check returned checked=true with zero recognized issues.

## Live-check procedure

1. Verify HTTPS readiness and the existing server's status without altering it.
2. Record node allocations and free capacity. Create a separate temporary beta
   server through the same transactional server creation/provisioning path.
3. Keep it stopped; do not accept an EULA or start Minecraft on the owner's behalf.
   Write a synthetic world marker through Panel, create and await a completed
   backup, change the marker, restore only that temporary server, then verify the
   original marker contents.
4. Terminate the temporary server through the normal job lifecycle. Verify the
   deletion job succeeded, Panel removed it, and node reservations returned to the
   recorded baseline. Keep the audit history.
5. Build the updated backend/frontend, run migrations including 016 before starting
   the new API, then verify readiness and authenticated feature requests.
6. When a real client is available, match the Minecraft version and required client
   mods; test whitelist/login and voice chat with that player. Record actual results.

## Customer beta improvements — follow-up

- Server overview now has a member-authorized activity timeline (latest 50 events)
  and an on-demand storage breakdown. Only safe labels, display names, outcomes
  and timestamps leave the audit service. Commands, paths, metadata and email
  addresses are not exposed. Storage uses Panel/Wings only, skips symbolic links,
  scans at most 200 folders / 5000 entries / eight seconds, and labels partial
  results. Live disk usage remains authoritative; backup bytes are separate.
- Dashboard notifications poll durable lifecycle and automatic-backup audit events
  for the last 30 days. Read state is persistent and individual to each user
  (migration 017). Authorization is embedded in list/read queries. Direct failed
  completed Panel backups and recognized startup errors are also shown on the
  server overview. These are in-app alerts, not email or browser push.
- Addon update checks compare up to 20 catalog-tracked projects against stable
  releases for the saved server software/version. Required dependencies still
  resolve through the existing catalog client. Reviews list old JARs to replace.
  Only successful receipts for the exact Panel identity can identify replacement
  candidates; old files are hash-verified twice before mutation. Modified uploads
  are rejected, idempotency and backup choices remain mandatory, and the server
  must be stopped. New files upload before obsolete filenames are removed. This
  multi-file operation is not atomic: uncertain results need inspection and backup
  recovery before starting. Untracked JARs are not automatically deleted.
- Downloadable player setup is a plain-text checklist with project links and
  unknown-file caveats. It does not silently install client mods, redistribute
  JARs or promise a complete modpack. Forge handshakes may report incompatibility;
  syncing mods requires a compatible launcher/helper already on the client.
- Voice client help explains V, device selection, microphone testing, keybind
  conflicts and microphone permissions. Owner reported successful real joining,
  connected voice and a working microphone test; another person's audio has not
  been tested.

New API: GET /servers/:id/activity, GET /servers/:id/storage,
GET /servers/:id/{plugins|mods}/catalog/updates, GET /notifications,
POST /notifications/:id/read. Expensive scans/update checks have per-user rate
limits and share the existing two-operation work bound. No game-world changes or
server restarts are needed to deploy this dashboard release.

Follow-up validation: 132 browser tests passed; management/server/HTTP/migration
race suites passed against disposable PostgreSQL; Go vet, staticcheck, import
formatting and frontend lint/format/build passed. Additional addon tests verified
same-filename replacement and retained UNKNOWN outcomes after an interrupted
old-file removal, without repeating remote writes.

Deployment: code commit `4c1fa1e` deployed; migration 017 applied before API start.
HTTPS readiness passed. Authorized live domain checks returned 16 activity events,
one unread notification, six tracked addon checks, and 287479956 bytes server
disk usage against 10737418240 bytes quota. Storage reported partial=true rather
than claiming a complete folder scan. Minecraft status responded with one player
online and startup help found zero recognized issues. Production addon files,
worlds and game power state were not modified by these checks.

## Simplified new-server offering

New creation offers only Vanilla, Paper and Fabric. Public software/version options
filter the configured catalog to those types; creation enforces the same offering
server-side. The frontend filters stale options defensively and the beginner Guide
compares only those three choices. Paper remains the default. Legacy types stay
recognized in management, client setup and provisioning so existing Spigot, Purpur,
Forge and NeoForge records and pending provisioning jobs remain supported. No
templates, eggs or customer files were deleted or disabled.

Validation: 130 browser tests passed, including all offered choices and stale-option
filtering. PostgreSQL race tests passed for server, billing and provisioning; the
legacy provisioning fixture verifies existing loader snapshots still work. Go vet,
staticcheck, import formatting, frontend lint, formatting and production build passed.

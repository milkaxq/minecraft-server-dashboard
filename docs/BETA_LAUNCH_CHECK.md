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
| Browser regression suite | 128 passed, including mobile backup flow and contextual reports |
| Real Panel backup/restore on a disposable stopped server | Pending: VPS SSH connection times out; no temporary server created |
| Current public Minecraft protocol/connectivity check | Pending: connection from this machine times out |
| Real player joining and authenticating | Pending: owner has no Minecraft client available |
| Deployment and production readiness after migration | Pending: restore VPS connectivity first |

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

## Finish live checks once connected

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

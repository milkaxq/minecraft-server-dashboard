# Security and reliability review

This review covers the current REST routes, server RBAC, payment event handling,
provisioning retries, resource reservations, file operations and backup restores.
The source review is covered by unit and PostgreSQL integration tests.
The complete database-backed race suite passed on 2026-10-08. Live Panel/Wings
validation is recorded separately from these source-level checks.

## Changes

| Area | Finding | Change | Regression coverage |
| --- | --- | --- | --- |
| Management authorization | File/settings/plugin/backup mutations used the permission snapshot taken before recording durable intent. Revocation during that step could still permit the mutation. | Recheck membership, entitlement and lifecycle state before issuing the mutation. Record denied operations as `REJECTED`. | PostgreSQL test revokes an admin member during the readiness check and verifies no file is written. Passed in the full PostgreSQL-backed race run. |
| Node reservations | The reservation helper depended on callers locking the server and did not compare supplied resources with the purchased snapshot. Existing reservations returned before input validation. | Lock the server inside `Reserve`, compare RAM/disk/CPU with its stored snapshot, and reject inconsistent node/allocation mappings. | Existing concurrent allocation test now relies on the helper's lock and rejects changed resource limits on replays. Passed in the full PostgreSQL-backed race run. |
| Panel readiness | A valid identifier for a different server was accepted in the readiness response. | Require the response identifier to match the requested server. | Offline HTTP test returns another server's identifier; the original code fails and the changed code passes. |
| Mock payment timestamps | Subtracting untrusted signed timestamps could overflow `int64` and admit an event outside the freshness window. HMAC verification was still required. | Read the current time once and compare the timestamp directly with the permitted bounds. | Offline signed-event tests cover stale/future timestamps and integer extremes. The overflow case fails against the original code and passes with the change. |

The redundant plugin-directory inspection was also removed; the remaining
inspection still rejects symlinks and validates the destination before upload.

## Existing protections checked

- Customer server UUIDs are resolved in PostgreSQL. Ownership/member roles control
  access, and unauthorized users receive the same result as missing servers.
- Plans and resource limits are resolved and snapshotted server-side. Customers do
  not supply prices, Panel IDs or editable CPU/RAM/disk allocations.
- Payment event identity, amount, currency, checkout reference and provider are
  checked under transaction locks. Unique event keys prevent duplicate fulfillment.
- Provisioning records remote creation intent, reconciles by external UUID, and
  retains reservations when creation/deletion outcomes are uncertain. Confirmed
  absence is required before resources are released.
- Management and power mutations share the worker's server advisory lock.
  Ambiguous writes are not automatically repeated.
- File paths reject traversal and alternate encodings. Every component is inspected
  for symlinks. Signed transfers use configured origins, refuse redirects and do
  not carry Panel API credentials to Wings or the browser.
- Backups enforce the purchased allowance. Restore requires an explicit matching
  backup UUID, a completed successful backup belonging to that server, and an
  offline server. Destructive confirmation now also has an offline regression test.
- Console access uses one-use tickets and periodic authorization checks. Browser
  messages are not forwarded as arbitrary Wings operations.

The new offline HTTP regression test exercises 53 private routes and verifies
that missing authentication is rejected before business logic or body parsing.
It covers authentication guards; database-backed tests separately cover ownership,
roles, subscription entitlement and concurrent behavior.

## Validation

Completed on 2026-10-08 using a dedicated local `hosting_test` database:

```sh
TEST_DATABASE_URL='<dedicated test database URL>' \
  go test -race -count=1 ./cmd/... ./internal/... ./migrations/...
GOPROXY=off GOSUMDB=off make lint-go fmt-check-go
make test-compose
cd frontend && npm run check && npm run build && npm test
```

The full race run passed **124 test cases/subtests with no skips or failures**,
including membership revocation, reservation replay, concurrent capacity allocation,
payment replay, ownership and lifecycle checks. All **22 Playwright tests** and
**5 Compose tests** passed. Go vet, Staticcheck, Go formatting, ESLint and Prettier
checks passed. Fresh backend, worker, migration and frontend Docker builds succeeded.
The dashboard also passed a browser login and navigation check against the real API.

Earlier baseline source overlays confirmed that the identity and timestamp
regressions fail against the original implementation.

## Deployment validation

For later reruns, use `TEST_DATABASE_URL='<dedicated test database URL>' make integration`.
Do not use a production database.

Live development smoke checks also passed against the pinned Panel v2 build and
Wings 1.13.3 in a native Linux Lima VM on macOS:

- Paper and Vanilla installation through mock payment and asynchronous provisioning.
- Order/payment replay, audited administrator recovery, suspend/unsuspend job replay,
  termination and capacity release after confirmed remote deletion.
- Cross-customer status, file, backup, metrics and power denial.
- File create/edit, stale revision rejection, folders, rename, upload/download,
  explicit deletion, and path traversal rejection.
- Minecraft settings persistence and validation; plugin upload/list/delete transport
  using a descriptor fixture removed before starting Minecraft.
- Completed backup download, quota enforcement, restore confirmation, real restore
  of the prior file set, and backup deletion.
- Browser console WebSocket through the Go authorization relay and offline metrics.
- Container inspection: non-root UID 988, exact 2 GiB RAM and 100% CPU limits,
  no privileged mode, no Docker socket mount, own server-data mount, and disabled
  direct communication between game containers.

The live checks exposed and fixed `.localhost` resolution in PHP/cURL, a Wings
subnet collision, and the missing `/run/wings` host mount required for machine IDs.
The bundled override example now disables CPU burst and extra RAM overhead so
actual container limits match the application's accounted capacity. JVM templates
used in development reserve native-memory headroom with a 75% heap limit.

After explicit operator EULA acceptance, the Paper demo also passed real start,
graceful stop, restart, and UUID-confirmed force-stop checks. The Go API returned
running CPU/RAM/disk/uptime metrics and Minecraft player counts. A browser sent
`list` through the authorized command endpoint and received Minecraft's output
through the WebSocket relay. A Minecraft status handshake from the Mac to
`127.0.0.1:25565` confirmed the forwarded connection, Paper 1.21.1 and the configured
10-player limit. The demo was left running.

The Lima-only Compose override resolves the public local game hostname to the
VM's allocation IP inside the backend. An explicit Lima port-forward rule keeps
the Mac connection address available after restarting the VM. These adjustments
are local machine configuration; they do not change production DNS requirements.

An authenticated player has not joined with a Minecraft game client. A plugin
transport fixture does not establish compatibility of a real plugin. No live
concurrent-customer load or deliberate ambiguous network-failure test was
performed; those behaviors are covered by the PostgreSQL regression suite.
These development smoke checks are not a production deployment audit.

## First-start agreement update

The dashboard now requires an explicit owner checkbox before calling the audited,
idempotent agreement endpoint. Start and restart read the fixed `eula.txt` through
Pterodactyl under the server lock and fail closed when acceptance is missing or
cannot be checked. No migration or automatic acceptance was introduced. Existing
accepted files remain valid. Stop and force-stop do not depend on acceptance.
The new tests cover owner/member isolation, explicit consent, replay, symlinks,
offline writes, missing/read-failing checkers, and installation/failure guidance.
See [the first-start runbook](FIRST_START.md) for routes and implementation details.

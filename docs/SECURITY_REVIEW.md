# Security and reliability review

This review covers the current REST routes, server RBAC, payment event handling,
provisioning retries, resource reservations, file operations and backup restores.
It is a source review with offline regression tests, not a live Panel/Wings audit.
Project services and installation downloads remain stopped.

## Changes

| Area | Finding | Change | Regression coverage |
| --- | --- | --- | --- |
| Management authorization | File/settings/plugin/backup mutations used the permission snapshot taken before recording durable intent. Revocation during that step could still permit the mutation. | Recheck membership, entitlement and lifecycle state before issuing the mutation. Record denied operations as `REJECTED`. | PostgreSQL test revokes an admin member during the readiness check and verifies no file is written. Pending database execution. |
| Node reservations | The reservation helper depended on callers locking the server and did not compare supplied resources with the purchased snapshot. Existing reservations returned before input validation. | Lock the server inside `Reserve`, compare RAM/disk/CPU with its stored snapshot, and reject inconsistent node/allocation mappings. | Existing concurrent allocation test now relies on the helper's lock and rejects changed resource limits on replays. Pending database execution. |
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

The new offline HTTP regression test exercises 51 private routes and verifies
that missing authentication is rejected before business logic or body parsing.
It covers authentication guards; database-backed tests separately cover ownership,
roles, subscription entitlement and concurrent behavior.

## Validation

Completed without downloads or running services:

```sh
env -u TEST_DATABASE_URL GOPROXY=off GOSUMDB=off \
  go test -race ./cmd/... ./internal/... ./migrations/...
GOPROXY=off GOSUMDB=off make lint-go fmt-check-go
git diff --check
```

The offline race run passed 85 test cases/subtests; 33 PostgreSQL-dependent tests
were skipped. Go vet, Staticcheck and Go formatting checks passed. Baseline source
overlays confirmed that the new identity and timestamp tests fail against the
original implementation.

## Remaining validation when services can run

Run `TEST_DATABASE_URL='<dedicated test database URL>' make integration` to execute
the database tests, including the new revoked-membership and reservation replay
cases. Do not use a production database.

Then run the real Panel/Wings smoke tests: paid provisioning and retry recovery,
capacity contention, cross-customer management denial, upload/symlink handling,
backup creation/restore, and container resource limits/isolation. Offline mocks
cannot establish the deployed container or filesystem isolation guarantees.

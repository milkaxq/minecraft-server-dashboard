# Phase 2 — provisioning and lifecycle

This phase adds the real Pterodactyl HTTP clients, a durable PostgreSQL job queue,
a separate Go worker, locked node/allocation reservations, and audited admin-only
provisioning, retry, suspension, unsuspension, and termination operations. Customer
power controls, console and dashboard remain Phase 3. Payments remain Phase 5.

## Architecture and changes

```mermaid
flowchart LR
  Customer[Customer: create pending server] --> API[Go REST API]
  Operator[Authorized admin grant / lifecycle command] --> API
  API --> DB[(PostgreSQL: state + jobs + audits)]
  DB --> Worker[Go worker]
  Worker --> Budget[Reserve node resources and allocation]
  Budget --> Panel[Pterodactyl Application API]
  Panel --> Wings[Pterodactyl Wings]
  Wings --> Container[Isolated Minecraft container]
```

The queue lives in PostgreSQL so the state, job and audit entry commit together;
there is no database/Redis dual-write race. Redis is still available for later
cache and notification needs. Workers poll once per second when idle and can scale
as separate processes. Every claimed job holds a PostgreSQL session advisory lock
for its server throughout remote operations. All worker writes use that locking
connection. Direct PostgreSQL or a session-pooling proxy is required; transaction
pooling does not support this locking scheme. This design assumes reliable database
failure detection; ambiguous remote creation is additionally protected by durable
creation markers, independently of lock ownership.

New files:

```text
cmd/worker/main.go                    worker process and graceful cancellation
cmd/control/main.go                   operator admin/software/node configuration
internal/audit/audit.go               transactional audit helper
internal/config/worker.go             separate worker/privileged config
internal/httpapi/lifecycle.go          admin command endpoints and job reads
internal/jobs/service.go              transactional, idempotent command scheduling
internal/jobs/claim.go                crash-recoverable job claims and server locks
internal/node/reservation.go          resource/allocation reservation and release
internal/node/register.go             empty dedicated-node discovery and import
internal/provisioning/worker.go       provisioning and lifecycle reconciliation
internal/provisioning/software.go     operator-managed eggs and supported versions
internal/pterodactyl/http.go          real Application and Client API adapters
internal/server/software.go           public software/version options
internal/testutil/database.go         disposable integration schemas
migrations/003_provisioning.sql       jobs, templates, versions, allocations, intents
```

Tests are alongside these packages. Existing API/server/config startup files,
Dockerfile, Compose, environment example and README were extended. The first two
migration files are unchanged. Frontend functionality is unchanged.

## Resource and isolation guarantees

Customer `POST /servers` accepts only plan UUID, name, approved version and software.
RAM/disk/CPU and feature limits come from its stored plan snapshot, never request
fields. Customers cannot set Panel IDs, node IDs, allocation IDs, image, startup,
variables or entitlement. The worker creates each server with its own reserved
allocation, purchased limits, zero swap, and `start_on_completion=false`.
Application and Client HTTP clients have distinct credentials and constructors;
the worker only needs the Application key. No key or Panel integer ID is returned
in customer server responses. The Go application never uses host directories,
Docker APIs or shell commands to manage Minecraft.

Operators register **empty, dedicated Panel nodes**, with no assigned allocations,
and exclusive management by this application. Budgets must fit Panel's physical
RAM/disk settings, without counting over-allocation. CPU is configured in percentage
points (100 per CPU core), after host headroom. Registration imports the node's
allocations, including hostname aliases. Keep host/Wings headroom in every budget.
Do not create other servers or change allocations directly on a registered node:
PostgreSQL can coordinate this application's workers, not independent external
provisioners. Adding node-management UI and allocation synchronization is later work.

Reservation transactions lock the server and an eligible node row, then reserve one
allocation and increment all resource counters together. Repeated reservations
reuse the existing allocation without incrementing counters. Database checks also
prevent counters exceeding budgets or falling below zero. A reservation is retained
while failed/uncertain provisioning might still exist in Panel. Termination releases
it only after a normal Panel deletion and a confirmed absent external record; force
DELETE is never used. Suspension retains capacity.

Panel/Wings must be configured to enforce its container isolation and limits.
Application API credentials need users/server read-write and node/allocation read
permissions. Keep Panel administration private; do not give customers Panel admin,
host, root or Docker credentials. This implementation provides the platform boundary;
a live deployment still needs its own correctly configured Wings nodes.

## Provisioning and recovery

1. Customer creates a `PENDING_PAYMENT` server record.
2. Before billing exists, a verified administrator can explicitly grant a
   complimentary server with an audit reason. The same lifecycle confirmation
   state (`PAYMENT_CONFIRMED`) is used to schedule provisioning, but no order/payment
   is marked paid. This route is not a payment verification endpoint.
3. The grant transaction commits state, a job, and an audit entry together.
4. The worker marks `PROVISIONING`, reserves resources, and snapshots its complete
   request: node, allocation, image, startup, variables, egg and plan-derived limits.
5. A per-user advisory lock serializes Panel user creation across that user's servers.
6. The worker finds or creates the Panel user/server by the application's UUID as
   `external_id`, stores the returned IDs internally, and waits for installation.
7. Only successful installation and matching owner/node/allocation/egg/resource
   limits lead to `RUNNING`. Here RUNNING means provisioned and ready; Minecraft
   power state remains offline until the future customer Start action.

Remote calls have a 20-second HTTP timeout; each job attempt has a 2-minute deadline.
Transient failures and installation polling retry with capped exponential delay
(up to 30 seconds), for at most 60 normal failed/polling attempts. The attempt budget
is a bound on reported failures, not a fencing lease; a process crash can reclaim
an unfinished final attempt once more to reconcile its effects. Each job retains
its original request ID and controlled error code for tracing. Raw Panel errors,
response bodies, passwords and keys are never logged or saved as errors.

A marker is committed **before** every create call. A timeout or process crash
cannot authorize another create simply because a lookup returns 404. If the remote
UUID is found, the worker verifies its ownership and limits and adopts it. If a
prior create is uncertain and the UUID is absent, it marks FAILED, retains the
reservation, and requires operator reconciliation. Definitive 4xx rejection can be
retried without that recovery confirmation. A retry of an installation failure
requests a reinstall once; it does not create another server.

For an uncertain create, first inspect Panel and verify that any original request
has finished. A normal retry reconciles an existing matching server. Only when the
operation has finished **and** the resource is absent should an administrator submit
`recovery_confirmation: "PANEL_OPERATIONS_FINISHED_AND_ABSENT"` with a new key and a
reason. This audited assertion clears the create intent, allowing exactly one new
attempt. It is an operator decision, not something the application infers from
elapsed time. It can also be supplied when terminating a FAILED server whose
uncertain creation has been confirmed absent. Existing requests remain immutable.
If resource/owner comparisons fail, repair the trusted Panel configuration before
retrying; the worker will not adopt or delete an unrelated resource.

Graceful shutdown cancels work. An interrupted RUNNING job stays durable and is
reclaimed after its database locking session exits. No in-memory queue or expiring
lease can silently lose the command. Lifecycle jobs reconcile before acting, so a
lost suspend/unsuspend/delete response can be retried without repeating allocation.

## Run after internet and Panel access are available

Use the normal Phase 1 registration/verification/login flow and apply migrations:

```sh
cp .env.example .env
# Set matching local database passwords.
docker compose up --build -d
```

Promote an existing verified account using the privileged CLI (never public signup):

```sh
docker compose exec backend control admin \
  --email operator@example.com --reason 'Initial platform operator'
# Copy the returned administrator UUID into ADMIN_ID in your local shell.
```

Configure a JSON template from the egg actually installed in your Panel. The sample
[paper-template.example.json](paper-template.example.json) is illustrative: **replace
its egg ID and verify its image, startup, required variables and supported versions
against your egg**. Egg IDs vary between installations. Use `server_type: "VANILLA"`
for a second template and its own version-variable name. Customers see only approved
software/version pairs from `GET /minecraft/options`, never the underlying config.

```sh
docker compose cp docs/paper-template.example.json backend:/tmp/paper.json
docker compose exec backend control software --actor "$ADMIN_ID" --file /tmp/paper.json
```

Set `PTERODACTYL_URL` and `PTERODACTYL_APPLICATION_API_KEY` in `.env`. Production
requires HTTPS Panel transport and a PostgreSQL URL with `sslmode=verify-full`.
The key is passed to the worker, not the backend API service or browser.
Register an empty dedicated node using the actual Panel node ID and sellable budgets:

```sh
docker compose --profile pterodactyl run --rm --entrypoint control worker node \
  --actor "$ADMIN_ID" --panel-id 1 --ram-mb 12288 --disk-mb 102400 --cpu 400 --dedicated
# The example budgets must fit your node; choose your own host headroom.
docker compose --profile pterodactyl up --build -d
```

Native development uses `localhost` in DATABASE_URL, `make migrate`, `make run`,
and `make worker` in separate shells. Operator commands are `go run ./cmd/control …`.
Environment files are not automatically loaded by native Go commands.

Create a customer server using an approved version, then use the operator's login
bearer token to grant provisioning:

```sh
curl -s http://localhost:8080/admin/servers/$SERVER_ID/provision \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" -H 'Content-Type: application/json' \
  -d '{"reason":"Complimentary development server"}'
# Response: 202 with a job UUID. Inspect status using:
curl -s http://localhost:8080/admin/jobs/$JOB_ID -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s http://localhost:8080/servers/$SERVER_ID -H "Authorization: Bearer $CUSTOMER_TOKEN"
docker compose logs -f worker
```

Keep an idempotency UUID stable when retrying the **same HTTP request**. A replay
returns the original job even after completion. Changing the action/body with that
key returns 409. Retrying a FAILED operation deliberately needs a **new** key.
Only one active command per server is allowed.

| Method | Route | Behavior |
|---|---|---|
| GET | /minecraft/options | Public approved software/version pairs |
| POST | /admin/servers/{uuid}/provision | Audited grant for a pending server; can also retry FAILED |
| POST | /admin/servers/{uuid}/retry | Same provisioning scheduler, normally used for FAILED |
| POST | /admin/servers/{uuid}/suspend | Queue suspension of a RUNNING server |
| POST | /admin/servers/{uuid}/unsuspend | Queue unsuspension of a SUSPENDED server |
| POST | /admin/servers/{uuid}/terminate | Queue deletion; body confirmation must equal the server UUID |
| GET | /admin/jobs/{uuid} | Admin-only status, attempts and controlled error code |

All mutation routes require a UUID `Idempotency-Key` and `reason` (3–500 bytes).
Termination also requires `confirmation: "<server UUID>"`; no customer deletion
route exists yet. A valid customer session returns 403 on these admin routes,
including for their own servers. Every service scheduling call independently
rechecks administrator permission; handlers cannot grant it from request fields.

## Verification

```sh
go vet ./...
go build ./cmd/api ./cmd/migrate ./cmd/worker ./cmd/control
TEST_DATABASE_URL='postgres://postgres:test-only-password@localhost:55432/hosting_test?sslmode=disable' make integration
```

Use a dedicated disposable PostgreSQL database as described in the main README.
Tests cover 20 concurrent resource reservations, repeated release, parallel queue
replays, competing worker claims, restart recovery, lost user/server/delete responses,
ambiguous creation recovery, definitive rejection retry, install polling/reinstall,
request immutability, resource mismatches, failed capacity allocation, lifecycle,
customer/admin authorization, template allowlists, and node registration constraints.
HTTP fixtures check actual Panel paths/payloads, parsing, allocation pagination,
separate credentials, path validation, and redirect/secret protection.

No live Panel/Wings installation was available for an end-to-end container smoke
test. The real adapter was checked against Panel's official
[Application routes](https://github.com/pterodactyl/panel/blob/1.0-develop/routes/api-application.php),
[server request validation](https://github.com/pterodactyl/panel/blob/1.0-develop/app/Http/Requests/Api/Application/Servers/StoreServerRequest.php),
[server response transformer](https://github.com/pterodactyl/panel/blob/1.0-develop/app/Transformers/Api/Application/ServerTransformer.php),
and [node response transformer](https://github.com/pterodactyl/panel/blob/1.0-develop/app/Transformers/Api/Application/NodeTransformer.php).
Docker/npm image/dependency builds remain to be verified with working registry access.

Phase 2 is ready for review. Phase 3 has not been started.

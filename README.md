# Minecraft hosting — Phases 1–5

A Go REST API backed by PostgreSQL, with authentication, configurable plans,
authorized server records, durable provisioning jobs, safe resource reservations,
and a real Pterodactyl integration. The application never executes Minecraft
processes. The dashboard includes power controls, live console, metrics, files, settings, backups
plugins and mods, demo checkout, manually renewed subscriptions and administration.
The [first-start flow](docs/FIRST_START.md) adds owner EULA acceptance, installation
guidance, and backend start/restart enforcement.
See [server members](docs/MEMBERS.md), [plugins/custom gameplay](docs/CUSTOM_GAMEPLAY.md),
and [Minecraft software setup](docs/MINECRAFT_SOFTWARE.md). The website includes
a beginner Guide and supports Vanilla, Paper, Spigot, Purpur, Fabric, Forge and NeoForge.

See [world management](docs/WORLDS.md) for safety backups, ZIP uploads/downloads, and confirmed resets.

See [Phase 5 architecture and runbook](docs/PHASE5.md) for billing configuration,
new routes, file changes and deployment limits.

See [the Phase 2 architecture and runbook](docs/PHASE2.md) for Panel configuration,
operator commands, lifecycle routes, recovery behavior, file changes and tests.
See [Phase 4 management architecture and setup](docs/PHASE4.md) for the new dashboard
pages, Client API endpoints and transfer configuration.

## Architecture

```mermaid
flowchart LR
  Browser[Next.js customer dashboard] --> API[Go HTTP handlers]
  API --> Auth[Authentication service]
  API --> Plans[Plan service]
  API --> Servers[Server service + authorization]
  Auth --> PG[(PostgreSQL)]
  Plans --> PG
  Servers --> PG
  API --> Management[Files, settings, backups and plugins]
  Management --> Client
  Management --> PG
  API --> Controls[Authorized power, commands and usage]
  Controls --> Client[Pterodactyl Client API]
  API --> Relay[Filtered console relay]
  Relay --> Client
  Relay --> WingsSocket[Wings WebSocket]
  API --> Billing[Orders, payment events and subscriptions]
  Billing --> PG
  Billing --> Jobs
  API --> Admin[Audited administration]
  Admin --> PG
  API --> Jobs[Durable PostgreSQL jobs]
  Jobs --> Worker[Go provisioning worker]
  Worker --> Panel[Pterodactyl Application API]
  Panel --> Wings[Wings → isolated container]
```

Handlers handle HTTP; services handle validation, authorization, and transactional
writes. PostgreSQL stores sessions, shared authentication rate limits, and audit
logs. Redis is available in local development. Jobs use PostgreSQL for transactional
consistency; the in-memory mock is used only in tests. No application/client credentials are sent to browsers.

## Files

```text
cmd/api/main.go                  HTTP startup, shutdown, cleanup, metrics listener
cmd/migrate/main.go              explicit migration command
cmd/worker/main.go               background provisioning/lifecycle jobs
cmd/control/main.go              privileged operator configuration
internal/auth/                  passwords, sessions, verification, rate limits
internal/config/                environment validation
internal/database/              embedded, locked, checksummed migrations
internal/httpapi/               REST routes, middleware, HTTP/integration tests
internal/mail/                  local private spool and TLS SMTP delivery
internal/plan/                  database-backed hosting plans
internal/server/                model, roles, states, authorized service
internal/pterodactyl/           real Application/Client adapters and test mock
internal/jobs/                  durable queue and session-lock claims
internal/node/                  safe reservations and node registration
internal/provisioning/          worker orchestration and software configuration
internal/audit/                 transactional audit helper
internal/user/                  public user model
internal/billing/               checkout, settlement, renewals and expiry
internal/admin/                 plan administration and operator views
migrations/                    schema and configurable sample plans
internal/management/           files, settings, plugins, mods and backups
internal/control/              authorized power, commands and usage
internal/console/              single-use tickets and filtered WebSocket relay
internal/minecraft/             bounded Java status ping for player counts
frontend/                      Next.js customer dashboard and browser tests
Dockerfile, docker-compose.yml  local services and one-shot migrations
docker-compose.pterodactyl.yml   pinned Panel v2 + MariaDB/Panel Redis
docker-compose.wings.yml         opt-in Linux Wings infrastructure agent
scripts/pterodactyl.sh           container setup commands
scripts/test-compose.py          rendered deployment security checks
.env.example, Makefile          configuration and checks
```

Tables: users, sessions, email_verifications, plans, servers, server_members,
nodes, node_allocations, software_templates, minecraft_versions, jobs, control_requests, console_tickets, management_operations, orders,
subscriptions, payment_events, audit_logs, auth_rate_limits. OWNER is derived
from servers.owner_id; members can be ADMIN or MODERATOR, preventing multiple owners.
Application server identifiers are UUIDs. Internal Panel IDs and error details are
excluded from public models. Lifecycle state RUNNING will mean provisioned/active;
live Minecraft power state is separate.

## Run locally

For the containerized **Pterodactyl v2** development stack, start with
[the v2 Docker runbook](docs/PTERODACTYL_V2.md). It adds a pinned Panel source build,
MariaDB/Panel Redis and an optional native-Linux Wings container. The commands
below still support backend-only development and an external Panel.

Prerequisites: Docker Compose; Go 1.26 for native development.

```sh
cp .env.example .env
# Edit .env: replace both local password placeholders with the same URL-safe password.
docker compose up --build -d
curl http://localhost:8080/health/ready
curl http://localhost:8080/plans
```

The migration service runs before the API and applies each SQL file once, under an
advisory lock, in a transaction. Existing migration edits fail checksum validation;
add a new numbered migration instead. PostgreSQL and Redis persist in named volumes.
Ports bind to loopback. Use `docker compose down` to stop without deleting data.

Customer dashboard (dependency installation requires npm registry access):

```sh
docker compose --profile frontend up --build -d
# http://localhost:3000
```

The public API is on :8080. Private Prometheus-format counters are on
http://localhost:9090/metrics. Metrics measure request and internal-failure totals.
Readiness checks PostgreSQL. JSON request logs include generated request ID, route,
status and duration; they omit bodies, tokens, connection strings and internal errors.

Native API: start PostgreSQL/Redis with Compose, then export a DATABASE_URL using
`localhost:5432` instead of `postgres:5432`. Run `make migrate` and `make run`.
Environment files are not automatically loaded by native Go commands.

## Try the API

```sh
# Registration is invite-only by default. For the first-account invitation,
# follow docs/PRIVATE_BETA.md, then include its token below.
curl -s http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"player@example.com","name":"Player","password":"a-long-unique-password","invitation_token":"YOUR_INVITATION_TOKEN"}'
# Read development delivery files (contains secret verification tokens):
docker compose exec backend sh -c 'cat /var/lib/hosting/mail/*.txt'
# Paste the verification token from your email file:
curl -i http://localhost:8080/auth/verify-email \
  -H 'Content-Type: application/json' -d '{"token":"PASTE_VERIFICATION_TOKEN"}'
curl -s http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"player@example.com","password":"a-long-unique-password"}'
# Set TOKEN to the login response token in your local shell.
# Configure approved software first; see docs/PHASE2.md.
curl -s http://localhost:8080/orders -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d '{"plan_id":"10000000-0000-4000-8000-000000000001","name":"Our world","minecraft_version":"1.21.1","server_type":"PAPER"}'
curl -s http://localhost:8080/servers -H "Authorization: Bearer $TOKEN"
```

Customer order creation persists a PENDING_PAYMENT server and snapshots price and
RAM/disk/CPU, backup and database limits from an enabled plan in one transaction.
Approved releases and software must first be configured using the Phase 2 operator
commands. Enable development demo checkout as described in [Phase 5](docs/PHASE5.md).
Payment confirmation atomically creates entitlement and queues provisioning.
Prices are integer cents. Admin accounts can manage plans from the dashboard.

REST routes:

| Method | Path | Access |
|---|---|---|
| POST | /auth/register | Public, rate limited |
| POST | /auth/verify-email | Public, single-use token, rate limited |
| POST | /auth/resend-verification | Public, rate limited |
| POST | /auth/login | Public, verified accounts, rate limited |
| POST | /auth/forgot-password | Public, generic response, queues recovery email |
| POST | /auth/reset-password | Public, single-use 30-minute token |
| PUT | /account/password | Authenticated; current password required, revokes all sessions |
| POST | /auth/logout | Authenticated; revokes current session |
| GET | /account | Authenticated |
| GET | /plans | Public, enabled plans only |
| GET / POST | /servers | Authenticated |
| GET | /servers/{uuid} | Owner or member |
| GET | /health/live, /health/ready | Public |

Unknown JSON fields (including price, resources, role and internal Panel IDs) are
rejected. Unauthorized server lookups return the same 404 as missing servers.
Role permissions are centralized and default-deny. Phase 2 adds admin-only command
routes documented in its runbook; Phase 3 customer routes are documented in [its runbook](docs/PHASE3.md). Phase 4 management routes are documented in [its runbook](docs/PHASE4.md). Phase 5 billing and admin routes are documented in [its runbook](docs/PHASE5.md).

## Security decisions and deployment boundaries

Passwords use salted bcrypt cost 12, with a 12–72 byte password policy. Random
256-bit sessions expire after 24 hours; only SHA-256 digests are stored.
The dashboard uses a host-only HttpOnly cookie and restores its session after
refresh. Cookie-authenticated changes require the exact frontend origin and a
session-bound CSRF header. Production requires HTTPS and Secure cookies.
Bearer authentication remains available for API clients. Never store session
credentials in localStorage. See [browser sessions](docs/SESSIONS.md).

Authentication endpoints share an atomic database rate limit of 30 requests per
15-minute window per direct peer IP. Forwarded headers are ignored. Login also permits at most 10 attempts per account per 15 minutes, keyed by an
email digest. Before putting this API behind a reverse proxy, enforce client-IP limits at that
trusted edge (the built-in limiter otherwise sees the proxy IP). Billing and administration have shared database quotas; payment webhooks are signed
and idempotent. Public generic traffic limits remain deployment work.
Phase 2 adds persistent job retries, reconciliation and node reservation locking.

Local mail is delivered to private files in a volume, never logs or API responses.
Production requires implicit TLS SMTP configuration, an HTTPS frontend origin,
and a PostgreSQL URL with `sslmode=verify-full`. If delivery fails after registration,
the account remains; `/auth/resend-verification` recovers it. Tokens expire after
24 hours; verification emails contain a link that fills the token without sending it
in HTTP URLs. The customer confirms verification before signing in. Resending
invalidates the previous token. Metrics must remain private.
Compose is a development configuration, not a production deployment recipe.

## Account recovery

Use **Forgot your password?** on the login page, or change your password in
**Account**. Reset links expire after 30 minutes and work once. Both operations
revoke every login session, console ticket and outstanding reset link. Password
reset requires an already verified account and does not sign you in automatically.

Generate a backend-only encryption key with `openssl rand -base64 32` and set
`RECOVERY_TOKEN_KEY` in your private `.env` or production secret store. Keep the
same key across API instances and restarts so queued emails remain decryptable.
Production requires this key; development without it returns HTTP 503 for reset
requests. Never put it in frontend configuration.

Recovery emails use a durable PostgreSQL outbox with bounded delivery retries.
The token is hashed for verification and encrypted in the mail outbox. Links use
the trusted `FRONTEND_ORIGIN` and carry tokens in URL fragments; the reset page
removes the fragment from browser history. Requests return the same response for
unknown, unverified and throttled accounts.

Local development writes emails to private files under
`/var/lib/hosting/mail` in the backend container; they do not arrive in Gmail.
Read the latest file privately with `docker compose exec backend sh` and follow
its reset link. Production sends emails using the configured TLS SMTP provider.
Apply migrations before starting the updated backend: `docker compose run --rm migrate`.

## Tests

Local code formatting, linting and tests are described in [the code-quality guide](docs/CODE_QUALITY.md).
The latest source review, fixes and pending live checks are documented in
[the security and reliability review](docs/SECURITY_REVIEW.md).
Install tooling with `npm --prefix frontend ci`, use `make fmt` to format code,
and run `make lint` or `make check` before committing.

```sh
make check             # lint + formatting + race detector; DB tests skip without URL
# Example disposable test database (choose a free host port):
docker run -d --name hosting-test -p 127.0.0.1:55432:5432 \
  -e POSTGRES_PASSWORD=test-only-password -e POSTGRES_DB=hosting_test postgres:18-alpine
# Wait for pg_isready before testing.
TEST_DATABASE_URL='postgres://postgres:test-only-password@localhost:55432/hosting_test?sslmode=disable' make integration
docker rm -f hosting-test
```

Integration tests create/drop a unique schema in the supplied dedicated database.
They cover migrations, registration/verification, verification replay, unauthorized
access, membership, plan enforcement, disabled plans, resource-injection rejection,
audit persistence, logout, expiry and concurrent database rate limits. Unit tests
cover password/token handling, role matrices, lifecycle transitions, validation,
and concurrent Pterodactyl mock idempotency with defensive map copies.
Phase 2 adds node allocation, worker idempotency and real HTTP adapter tests.
Phase 5 adds concurrent payment replay, subscription expiry and renewal races,
admin authorization and billing/browser integration tests.

Phases 1–5 are implemented. See [Phase 5 architecture, file changes and setup](docs/PHASE5.md).
Payments are a development mock; real financial processing remains future work.

## Validation in this workspace

Go builds, vet, and PostgreSQL-backed race tests pass. The Next.js production build,
frontend browser tests and Compose configuration are checked through Phase 5 (17 browser scenarios).
A live Panel/Wings smoke test remains pending your deployment configuration.
See [Phase 5](docs/PHASE5.md) for repeatable commands and integration limits.

For the invited-beta and production prerequisites, see [launch readiness](docs/LAUNCH_READINESS.md).

Private beta setup and invitation management: [PRIVATE_BETA.md](docs/PRIVATE_BETA.md).

Tester checklist and private feedback/review workflow: [BETA_TESTING.md](docs/BETA_TESTING.md).

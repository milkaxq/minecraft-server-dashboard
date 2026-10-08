# Phase 3 — customer dashboard, power, console and usage

Phase 3 extends the existing API and provisioning worker. The Next.js dashboard
calls the Go REST API; Go resolves the customer's application UUID, checks their
membership and permissions, then calls the Pterodactyl Client API. Minecraft still
runs exclusively in isolated Wings containers. The Application API key remains
in the worker. The Client API key and short-lived Wings JWT remain in the backend.

## Code and database changes

| Location | Responsibility |
|---|---|
| `migrations/004_customer_controls.sql` | Durable command/power intents and hashed single-use console tickets |
| `internal/control/` | Authorization, plan-preserving power/commands, idempotency and usage |
| `internal/console/` | Ticket/session binding, safe Wings relay and periodic access checks |
| `internal/minecraft/` | Bounded Java status ping for player counts |
| `internal/httpapi/controls.go` | REST controls, quotas and console upgrade |
| `internal/pterodactyl/http.go` | Separate Client API WebSocket credentials adapter |
| `internal/server/` | Central role permissions, including force stop |
| `internal/config/`, `cmd/api/` | Client API configuration and relay lifetime |
| `frontend/app/`, `frontend/components/`, `frontend/lib/` | Responsive customer dashboard, authentication, servers, account and billing-plan views |
| `frontend/tests/`, `frontend/playwright.config.mjs` | Browser flows against explicit test-only API fixtures |
| `frontend/package-lock.json`, `frontend/Dockerfile` | Locked dependencies and standalone production image |
| `.env.example`, `docker-compose.yml` | Browser API URL and backend-only credentials |

Dashboard plans and server resources come from PostgreSQL. New server requests use
approved versions/software and snapshot limits through the existing Phase 2 service.
Checkout and invoices remain Phase 5; requests await an audited operator grant.
Files, settings, backups, plugins and worlds remain Phase 4 or later. The Simple
view shows power, players, address, resource usage and software. Advanced adds the
Minecraft console and confirmed force stop.

## REST and authorization

All routes below use public application server UUIDs, never Panel integer IDs.

| Method | Path | Permission |
|---|---|---|
| GET | `/servers/{id}/usage` | Owner, admin, moderator |
| POST | `/servers/{id}/start` | Owner, admin, moderator |
| POST | `/servers/{id}/stop` | Owner, admin |
| POST | `/servers/{id}/restart` | Owner, admin, moderator |
| POST | `/servers/{id}/kill` | Owner, admin; explicit confirmation |
| POST | `/servers/{id}/commands` | Owner, admin, moderator with command restrictions |
| POST | `/servers/{id}/console-ticket` | Owner, admin, moderator |
| GET (WebSocket) | `/servers/{id}/console` | Single-use session-bound ticket; exact browser origin |

REST uses the existing verified-account Bearer session. Power and commands require
a UUID `Idempotency-Key`. Power requests accept `{}`; kill requires
`{"confirmation":"<application-server-uuid>"}`. Commands accept
`{"command":"list"}`. Unknown fields are rejected. Moderator commands are limited
to `list`, `say` and `help`, including their `minecraft:` forms, so a console command
cannot bypass the moderator's power restrictions. Owner/admin commands remain
Minecraft console commands, never a host shell.

Every operation checks membership and requires an active provisioned server without
an outstanding lifecycle job. Foreign server IDs return 404. Power and command
execution share the worker's per-server PostgreSQL advisory lock. Before calling
Panel, the API persists an intent and audit entry. Replaying an accepted request
never repeats its Panel call; changed payloads conflict. An uncertain external
outcome is recorded as UNKNOWN and cannot be replayed automatically. Check current
status before consciously issuing a new request. Commands and upstream error bodies
are excluded from logs/audits; limits can never be changed by these endpoints.

Persistent quotas apply per customer: 60 power requests, 300 commands and 60 tickets
per 15 minutes. Console admission is limited to two streams per customer and 128
per API process. Use edge limits and connection limits when scaling API replicas.

## Console boundary

A REST call mints a random 45-second, single-use ticket. Only its SHA-256 hash is
stored, bound to the server, user and current session. The browser connects with
WebSocket subprotocols `hosting.console.v1` and `ticket.<ticket>`; credentials never
appear in a URL. The server selects only `hosting.console.v1`.

Go obtains the Wings JWT and socket through Panel, checks the socket against exact
configured origins and the expected server path, and authenticates to Wings before
accepting the browser. The upstream Origin identifies Panel. Only console output
and valid power-state events reach the browser. Installation, transfer and privileged
events are excluded. Browser frames cannot control Wings; commands go through the
REST service and its permission checks. Frames, retained lines and write deadlines
are bounded. Session expiry/logout, revoked membership or suspension closes a live
stream on the next access check (15 seconds by default). API shutdown also cancels
streams. Reconnection requests a fresh ticket; Wings token refresh stays internal.

The relay follows the official [Panel WebSocket controller](https://github.com/pterodactyl/panel/blob/develop/app/Http/Controllers/Api/Client/Servers/WebsocketController.php)
and [Wings WebSocket protocol](https://github.com/pterodactyl/wings/blob/develop/router/websocket/websocket.go).

## Usage and state

Database RUNNING means the server is provisioned and entitled to management; it does
not mean Minecraft is currently online. Actual power state, CPU, RAM, disk and uptime
come from the Client API resource endpoint. Overview polls every five seconds; server
cards poll every ten seconds. CPU history records observed samples only.

Player counts use a two-second Java Server List Ping against the stored authorized
allocation hostname/port. No customer-provided probe target is accepted. A server
that blocks status pings displays player counts as unavailable. The backend must be
able to reach the advertised game address; a publicly advertised hostname may need
appropriate DNS or routing in your deployment. No fake players or usage are shown.

## Run

1. Copy `.env.example` to `.env` and replace the database password placeholders.
2. Set `PTERODACTYL_URL` and a **Client API** key from a dedicated trusted Panel
   operator account authorized to access the hosted servers. Keep this account and
   its API key private. Do not reuse an Application API key.
3. Set `PTERODACTYL_WEBSOCKET_ORIGINS` to exact Wings origins, for example
   `wss://wings.example.com:8080`. Multiple origins are comma-separated. HTTP/ws are
   accepted only in development; production requires HTTPS/WSS. Go must reach Panel
   and Wings and trust their TLS certificates.
4. Follow [Phase 2](PHASE2.md) to configure nodes, approved eggs/versions, the worker
   Application API key and an operator entitlement grant.
5. Start local services and the dashboard:

```sh
docker compose --profile frontend --profile pterodactyl up --build -d
# Dashboard: http://localhost:3000; API: http://localhost:8080
```

Without a configured external Panel, omit `--profile pterodactyl`. Registration,
verification, plans, account and pending server requests work; management reports
that the hosting connection is unavailable rather than simulating a real server.
Development verification emails are in the existing private mail volume:

```sh
docker compose exec backend sh -c 'cat /var/lib/hosting/mail/*.txt'
```

Use the token on `/verify`, then sign in. Sessions stay in browser memory and are
cleared by a page reload; sign in again after refreshing. Nothing is stored in
localStorage or browser cookies. `NEXT_PUBLIC_API_URL` is the browser-reachable
API address embedded at frontend build time; rebuild after changing it.
`FRONTEND_ORIGIN` must match the exact dashboard origin. Production requires HTTPS,
the existing production mail/database configuration and a WebSocket-capable proxy.

Native frontend development:

```sh
cd frontend
npm ci
NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev
```

## Validation

```sh
go build ./cmd/api ./cmd/migrate ./cmd/worker ./cmd/control
go vet ./...
# Supply a disposable PostgreSQL database; integration tests create unique schemas.
TEST_DATABASE_URL='postgres://postgres:TEST_PASSWORD@localhost:55432/hosting_test?sslmode=disable' go test -race -count=1 ./...
cd frontend
npm ci
npm run build
npx playwright install chromium
npm test
npm audit
```

On machines with Chrome already installed, set `PLAYWRIGHT_EXECUTABLE_PATH` to its
executable instead of downloading Chromium. Browser tests run the production build
on port 3005 with test-only REST/WebSocket fixtures; fixtures are not shipped into
product pages. They cover login/logout, registration/verification, database-backed
plan rendering, constrained request payloads, power controls, moderator UI, console
commands, destructive confirmation, error/empty states and mobile layout.

Backend tests exercise cross-customer denial before Panel calls, role enforcement,
unchanged purchased resources, durable power/command replay, uncertain outcomes,
kill confirmation, real player-ping parsing, socket-origin restrictions, one-time
session-bound tickets, a real WebSocket relay, privileged-event filtering, browser
frame rejection and logout revocation. Existing provisioning/capacity tests remain.
A live external Panel/Wings smoke test is pending deployment credentials and network
configuration; test adapters do not establish that a particular live egg is ready.

# First start and Minecraft agreement

After payment, the dashboard shows a queued state followed by **Installing**.
It refreshes automatically. Failed setup provides support guidance without
exposing internal provisioning errors. Once installation finishes, the server
owner reviews the Minecraft EULA, checks the agreement checkbox, and clicks
**Accept agreement**. This does not start Minecraft automatically. **Start**
becomes available after acceptance and an offline power-state check. A successful
start changes the display to **Online**; stopping shows **Ready to start**.

Existing servers with an accepted `eula.txt` do not need to repeat acceptance.
Administrators and moderators can view the agreement status, but only the server
owner can use the acceptance endpoint. Advanced file editing remains available
according to the existing file permissions.

## API

| Method | Route | Behavior |
| --- | --- | --- |
| GET | `/servers/{id}/eula` | Authorized members receive `accepted`, `can_accept`, and the official EULA `url`. |
| POST | `/servers/{id}/eula` | Owner only; requires `{"accepted":true}` and a UUID `Idempotency-Key`. |

The backend resolves the application server UUID and permissions in PostgreSQL.
Agreement reads and writes use Pterodactyl's Client API, with the existing path,
symlink, readiness and subscription checks. A missing file means not accepted.
Accepting creates the fixed container file `/eula.txt` if necessary. Writing it
requires the server to be offline. If it already contains valid acceptance,
the operation succeeds without changing the file, including while online.

Acceptance uses the existing durable management intent, per-server lock and audit
records (`eula.accept`); no database migration is needed. Unknown remote outcomes
follow the existing management recovery behavior and are not automatically retried.
The file remains the source of truth, so file edits and backup restores are respected.
Start and restart check acceptance under the same server lock and return HTTP 409
if acceptance is missing. A read failure or missing checker prevents a new start
or restart. Stop and force-stop remain available without an agreement check.

## Implementation

- `internal/management/eula.go`: status, safe file checks and audited acceptance.
- `internal/httpapi/eula.go`: authenticated status and acceptance routes.
- `internal/control/service.go`: start/restart enforcement.
- `internal/server`: owner-only acceptance permission.
- `frontend/components/server-detail.js`: agreement prompt and power-control states.
- `frontend/lib/api.js`: beginner-facing installation and power labels.

## Validation and running

The PostgreSQL-backed race suite passes 124 test cases/subtests, including
authorization, explicit consent, replay, symlink rejection, offline requirements
and start/restart enforcement. All 22 Playwright tests pass, including the owner
checkbox flow, moderator guidance, agreement read failure, and setup guidance.
Go and frontend lint/format checks and production builds also pass.

Run the existing application commands described in README and the deployment
runbooks; no additional environment variables or migration are required. On this
Mac, the updated app is available at http://127.0.0.1:3000, and the local VM restart
instructions are in the ignored `.local/RUNNING.md` file.

The real Panel/Wings smoke check recognized the existing accepted server, rejected
an unaccepted direct start and cross-customer agreement requests, and completed
the owner's browser checkbox → acceptance → Start → Online flow. The same Paper
world was preserved and left running. The prompt's checkbox size and layout were
also checked at desktop and mobile widths.

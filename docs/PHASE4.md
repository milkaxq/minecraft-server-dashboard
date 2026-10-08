# Phase 4 — files, settings, backups and Paper plugins

The customer dashboard now offers Settings, Backups and Paper Plugins in Simple
mode, with Files beside Console in Advanced mode. Go authorizes each operation using
application UUIDs, translates it into the Pterodactyl Client API, and proxies signed
Wings/object-store transfers. Go never reads game-server directories from the host
and never executes a Minecraft or plugin process.

## Architecture and file changes

```mermaid
flowchart LR
  Dashboard[Next.js customer pages] --> REST[Go authorized REST handlers]
  REST --> Management[Management service]
  Management --> PG[(PostgreSQL intents and audit)]
  Management --> Client[Pterodactyl Client API adapter]
  Client --> Panel[External Panel]
  Client --> Transfer[Allowed Wings or backup-storage origin]
  Panel --> Wings[Isolated Minecraft container]
```

| Files | Change |
|---|---|
| `migrations/005_management.sql` | Durable management intents, fingerprints and safe result metadata |
| `internal/management/service.go` | File/plugin business logic, authorization, serialized mutations and revision checks |
| `internal/management/paths.go` | Strict container-path and filename validation |
| `internal/management/settings.go` | Java properties parsing, validation and preservation of unrelated settings |
| `internal/management/backups.go` | Purchased backup allowance, ownership and confirmed restore/delete |
| `internal/pterodactyl/management.go` | Real Client API file/backup methods and bounded, allowlisted signed transfers |
| `internal/httpapi/management.go` | Customer REST endpoints, input bounds, downloads and quotas |
| `internal/control/service.go` | Reject power/console controls while Panel reports restore/install/transfer state |
| `internal/config/`, `cmd/api/`, `.env.example`, `docker-compose.yml` | Management wiring and transfer-origin configuration |
| `frontend/components/management.js` | Four management views and destructive-action dialogs |
| `frontend/app/servers/[id]/{files,settings,backups,plugins}/page.js` | New dashboard pages |
| `frontend/components/server-detail.js`, `frontend/lib/api.js`, `frontend/app/globals.css` | Feature navigation, authenticated binary transfers and responsive styling |
| Management, adapter, HTTP and browser tests | Security and real protocol coverage |

No new runtime service or credentials are required. The worker still holds the
Application API key; management uses the backend-only Client API key. Redis remains
available; these operations do not need another queue. Panel performs backup creation
and restoration asynchronously, and the UI polls their actual status.

## Behavior and safety

Only OWNER and ADMIN may use files, settings, plugins or backups. MODERATOR has no
access to these endpoints. Foreign server IDs and backup UUIDs are denied before
performing the requested external action. Public identifiers remain application
UUIDs, and Panel credentials/signed URLs never appear in browser responses.

File paths must be canonical absolute container paths. Traversal, encoded escapes,
backslashes, control characters, repeated separators and root mutation are rejected.
The service checks directory components using Panel file metadata and refuses
symlinks. Wings remains the filesystem sandbox, including races with container code.
Directories can be browsed and text files read while online. Stop Minecraft before
creating, writing, renaming, deleting or uploading files. Text edits are limited to
1 MiB, uploads to 64 MiB. Uploads/create/rename never overwrite existing paths.
Text saves require the previous SHA-256 revision. A stale save returns 409; reload
explicitly before saving again. Deletion requires confirmation of the exact path.

Settings edit `server.properties`: MOTD, difficulty, game mode, hardcore, PvP,
maximum players, whitelist, account verification, view distance and spawn protection.
The Java-properties parser understands escaped keys/values, Unicode, continuation
lines, whitespace separators and duplicate keys. Saving removes duplicate managed
keys while preserving comments and unrelated properties. Invalid properties require
repair through the raw editor. Resources cannot be changed through settings; player
recommendations remain recommendations, not resource allocations. Changes require
Minecraft to be stopped and apply when it starts again.

Paper plugins are listed from `/plugins` and uploaded as `.jar` files. The backend
checks the ZIP structure for a bounded `plugin.yml` or `paper-plugin.yml` descriptor;
it does not run Java or fetch a plugin store. The customer chooses trusted plugins.
Deletion removes the jar and preserves its configuration folders. Start/restart
controls apply plugin changes. Vanilla plugin endpoints reject plugin operations.

Backup history comes from Panel. The application's purchased backup limit is checked
under the same PostgreSQL server advisory lock as provisioning and power operations.
All existing history entries count toward the allowance, including failed backups;
delete completed failed entries to reclaim slots. The adapter additionally checks
Panel's actual limit before creation to avoid its rotating retention on configuration
drift. New backups are locked in Panel; explicit deletion unlocks a locked backup
before deleting it. Creating a backup while online is supported, but stopping first
provides a more consistent world snapshot.

Restore requires a successful completed backup belonging to that server, an offline
Minecraft process and explicit confirmation of the backup UUID. Restoration replaces
all current files (`truncate=true`). Panel's restoring status blocks further
management and power/console operations until it clears. Do not start Minecraft
until restoration completes. Downloads require a completed successful backup.

Each mutation requires a UUID `Idempotency-Key` and persists an intent before its
external call. Replaying a successful matching key returns saved metadata without
repeating the operation. A changed payload conflicts. An uncertain response or
interrupted operation is not automatically repeated. Unresolved backup creation
blocks another creation, including with a new key, to avoid spending capacity twice.
Audit entries contain operation IDs, actions and outcomes, not file/settings/plugin
contents or signed tokens. Endpoints share a durable 240-requests/15-minute quota per
user. Upload bodies are read only after HTTP authentication and server authorization.
Each API process buffers at most two uploads at a time.

## REST endpoints

All requests require the existing verified-account Bearer session.

| Method | Path | Request |
|---|---|---|
| GET | `/servers/{id}/files?path=/` | Browse a directory |
| GET | `/servers/{id}/files/text?path=/server.properties` | Read text with revision |
| POST | `/servers/{id}/files` | File mutation JSON |
| POST | `/servers/{id}/files/upload?path=/&name=example.txt` | Raw `application/octet-stream` |
| GET | `/servers/{id}/files/download?path=/example.txt` | Authenticated attachment stream |
| GET / PUT | `/servers/{id}/settings` | Settings and revision |
| GET | `/servers/{id}/plugins` | Installed Paper jars |
| POST | `/servers/{id}/plugins/upload?name=Example.jar` | Raw jar bytes |
| DELETE | `/servers/{id}/plugins` | `name` and matching `confirmation` |
| GET / POST | `/servers/{id}/backups` | History / create with `name` |
| POST | `/servers/{id}/backups/{uuid}/restore` | Matching `confirmation` |
| DELETE | `/servers/{id}/backups/{uuid}` | Matching `confirmation` |
| GET | `/servers/{id}/backups/{uuid}/download` | Authenticated archive stream |

File mutation JSON uses `action` (`create`, `write`, `folder`, `rename`, `delete`) and
`path`. `write` includes `content` and `revision`; `create` can include initial content;
`rename` includes `destination` in the same parent directory; `delete` includes
`confirmation` equal to the full path. Unknown fields, including allocation or Panel
ID overrides, are rejected. All POST/PUT/DELETE operations require `Idempotency-Key`.

## Run and configuration

Continue the [Phase 3 setup](PHASE3.md). Add exact transfer origins to `.env`:

```dotenv
# Wings HTTP origin; this is distinct from its wss:// console origin.
PTERODACTYL_TRANSFER_ORIGINS=https://wings.example.com:8080
# For S3 backups, also list the exact origin returned by your Panel's signed URLs.
# Example: https://wings.example.com:8080,https://your-bucket.s3.your-region.amazonaws.com
```

Origins must be exact, without paths, wildcards, credentials, query or fragment.
Production requires HTTPS. Development permits HTTP. Redirects are denied and the
Panel Authorization header is never forwarded to transfer hosts. Keep Panel and
Wings current and reachable from Go; configure storage URLs to resolve directly
rather than redirect through an unlisted host.

```sh
docker compose --profile frontend --profile pterodactyl up --build -d
# http://localhost:3000 — open a ready server, then Settings/Backups/Plugins.
# Advanced → Files shows the raw file manager.
```

Migration 005 is applied by the existing migration container before API startup.
For native backend development, export your database/environment variables and run
`make migrate` before `make run`. The frontend still keeps the session in memory;
refreshing the browser requires signing in again.

Transfers stay bounded: uploads have a two-minute read deadline; downloads have a
ten-minute deadline and a resource-based byte ceiling (disk allocation for files,
twice disk allocation for compressed backup transfers). Dashboard downloads use an
authenticated fetch and a browser Blob, so very large backups consume browser memory.
For large archives, stream directly with the REST endpoint and a Bearer header:

```sh
# Set TOKEN and SERVER_ID/BACKUP_ID locally; never put tokens in URLs.
curl --fail --header "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/servers/$SERVER_ID/backups/$BACKUP_ID/download" \
  --output backup.tar.gz
```

An UNKNOWN backup creation requires operator reconciliation before more creation.
Inspect `management_operations` for that server and match the requested operation
against Panel history. After verifying the remote outcome, resolve its row as
SUCCEEDED with matching safe backup metadata, or REJECTED only when no backup was
created. Never clear uncertainty without checking Panel; the API intentionally does
not offer a customer bypass. A richer operator recovery UI belongs to Phase 5.

## Tests and verification

```sh
go build ./cmd/api ./cmd/migrate ./cmd/worker ./cmd/control
go vet ./...
TEST_DATABASE_URL='postgres://postgres:TEST_PASSWORD@localhost:55432/hosting_test?sslmode=disable' go test -race -count=1 ./...
cd frontend
npm ci
npm run build
# Once per machine, if Chromium is not installed:
npx playwright install chromium
npm test
npm audit
```

Backend integration tests use unique schemas in a disposable PostgreSQL database.
They cover cross-customer and moderator denial, every management HTTP route, strict
JSON/CORS, traversal and symlinks, offline enforcement, revisions, properties
preservation, immutable plan resources, plugin lifecycle, quota concurrency,
creation/restore replay and unknown-outcome capacity protection. HTTP fixtures
exercise actual Panel request paths, raw writes, multipart upload, signed-download
credential separation, redirect denial and backup payloads. Browser tests cover
all four pages, save payloads, binary uploads and destructive confirmation, alongside
existing Phase 3 flows.

Validated in this workspace: Go build/vet, PostgreSQL-backed race tests, Next.js
production build and 12 browser tests. A live external Panel/Wings smoke test still
needs your deployment credentials and network. Dedicated world upload/download/reset,
SFTP credential delegation, customer startup-variable editing and billing/admin UI
are not part of this phase's four-feature implementation. No placeholders simulate
these functions. Phase 5 has not started.

Protocol sources: official [Panel Client routes](https://github.com/pterodactyl/panel/blob/develop/routes/api-client.php),
[File controller](https://github.com/pterodactyl/panel/blob/develop/app/Http/Controllers/Api/Client/Servers/FileController.php),
[Backup controller](https://github.com/pterodactyl/panel/blob/develop/app/Http/Controllers/Api/Client/Servers/BackupController.php),
and [Wings file handling](https://github.com/pterodactyl/wings/blob/develop/router/router_server_files.go).

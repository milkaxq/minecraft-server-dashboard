# World management

Open a ready server and choose **World** in Simple mode. Owners and server admins
can manage worlds. Moderators cannot download or change them.

Stop the server before downloading or changing a world. Download produces a ZIP
with the configured world folder and, for Paper, its Nether and End folders.
Vanilla dimensions are already inside the main world. All data comes through
Pterodactyl's container-scoped APIs; the Go application does not open server paths
on the host or extract archives there.

## Upload or reset

1. Stop the server and click **Create safety backup**. This consumes one purchased
   backup slot; existing backups are never evicted automatically.
2. Wait for **Safety backup ready**. Backups continue asynchronously in Wings,
   and the page polls for completion. The selected backup survives page reloads.
3. Choose **Upload world .zip** or **Reset world**. Type the current world name
   in the confirmation dialog. Reset removes all configured dimensions; the next
   start generates a world using the existing server settings and seed.
4. Start the server. Settings, plugins, resource limits, and EULA acceptance remain
   in place. Use **Backups** to restore the safety backup if necessary; restoring
   a backup replaces all server files, not just world files.

Starting/restarting, restoring a backup, or changing files after the safety
snapshot requires a new safety backup. If slots are full, review backups and
explicitly delete one before creating another. A failed, missing, foreign, or
unfinished backup cannot authorize a world change.

ZIP uploads support `level.dat` at the archive root, or a folder matching the
configured `level-name` containing `level.dat`. Paper's matching `_nether` and
`_the_end` folders may accompany a wrapped world. Missing dimensions are removed
and regenerated on the next start. Use a world compatible with the selected
Minecraft version; the application does not convert saves between versions.

MVP upload limits: 64 MiB compressed, 128 MiB expanded, 32 MiB per file, 2,000 ZIP
entries, and 32 levels of nesting. The server must have room within its purchased
disk allocation for both old and staged new files. Traversal, symlinks, special
files, encrypted ZIPs, duplicate names, conflicting paths, and CRC failures are
rejected before staging. Nested `level-name` paths and custom plugin worlds are
outside this MVP. World browsing/download is limited to 2,000 entries as well.

## Implementation

- `internal/management/worlds.go`: properties-based world discovery, no-follow
  traversal, safety snapshots, ZIP validation, staged replacement/reset, streamed
  downloads, and shared server locks.
- `internal/httpapi/worlds.go`: authenticated REST routes and bounded upload bodies.
- `frontend/components/management.js` and
  `frontend/app/servers/[id]/worlds/page.js`: world page, backup progress, and typed
  confirmation.
- Existing `management_operations`, `control_requests`, and audit logs record
  requests, snapshots, replay protection, and mutation results. No migration is
  required. Snapshot revisions include file metadata and properties revisions;
  this is a freshness check, not a cryptographic filesystem integrity guarantee.

Downloads hold the shared server advisory lock while streaming a ZIP from remote
file downloads, preventing application power/file mutations during the transfer.
Uploads validate first, stage new files, move old dimensions into the staging
folder, switch replacements into place, then remove staging. No old world is
removed before a completed safety backup exists. An uncertain multi-step result
is recorded as `UNKNOWN`, never automatically replayed, and blocks Start/Restart.
Stop remains available. An operator must inspect Panel files and restore the
backup or recover `.hosting-world-<uuid>/old` before reconciling the unresolved
`management_operations` record. Never mark an operation successful merely to
clear its lock. Separate Panel/SFTP access can bypass application locking; normal
customers are not given operator credentials.

REST routes (application server UUIDs):

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/servers/{id}/world` | Current world and safety backup reference |
| GET | `/servers/{id}/world/download` | Stream a ZIP; stopped server required |
| POST | `/servers/{id}/world/backup` | Start a stopped-world safety backup |
| POST | `/servers/{id}/world/reset` | Reset with backup, revision, confirmation |
| POST | `/servers/{id}/world/upload` | Replace with validated ZIP |

POST routes require a UUID `Idempotency-Key`. Backup accepts `{}`. Reset accepts
`{"backup":"<backup UUID>","revision":"<world revision>","confirmation":"world"}`.
Upload accepts the same fields as query parameters and a raw ZIP body with
`Content-Type: application/octet-stream`. Do not send resource limits or Panel IDs.

Run the usual Docker Compose stack; **World** appears on owner/admin servers.
Validate with `make check`, `TEST_DATABASE_URL=<dedicated-test-db> make integration`,
and `npm --prefix frontend run build && npm --prefix frontend test`.

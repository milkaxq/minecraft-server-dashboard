# Server members

Open your server and select **Members**. Only the server owner can see the
member list, add members, change roles, or remove members. The page also works
before provisioning or while suspended, so owners can revoke access at any time.

Ask the other person to register and verify their email first. Enter that email
and choose a role. There is no email invitation workflow in this MVP: existing
verified accounts receive access immediately, and the shared server appears in
their dashboard. Up to 20 additional members are supported.

| Role | Access |
| --- | --- |
| Owner | All features, billing, members, EULA acceptance and deletion |
| Admin | Status, console, power, files, settings, worlds, plugins and backups |
| Moderator | Status, console, start and restart; commands are restricted by the existing console policy |

Ownership cannot be reassigned through these endpoints, and owners cannot be
removed or demoted. Global platform administrator status is never granted by
server membership. Each customer keeps their own login; no Panel credentials,
Docker access, host paths, or resource-limit controls are shared.

## Architecture and API

`internal/server/members.go` owns the PostgreSQL transactions, owner checks,
verified-account lookup, role validation and member limit. A transaction shares
the existing server advisory lock with power/file operations. It atomically
commits the membership change, its replay receipt, and an audit entry. Replaying
a successful request returns its previous result without recreating removed
access. Reusing a key with different input returns 409.

`internal/httpapi/members.go` exposes authenticated, rate-limited REST routes.
`migrations/007_members.sql` adds the receipt table. The UI is in
`frontend/components/members.js` and `frontend/app/servers/[id]/members/page.js`.

| Method | Route | JSON body |
| --- | --- | --- |
| GET | `/servers/{id}/members` | None |
| POST | `/servers/{id}/members` | `{"email":"friend@example.com","role":"MODERATOR"}` |
| PUT | `/servers/{id}/members/{user_id}` | `{"role":"ADMIN"}` |
| DELETE | `/servers/{id}/members/{user_id}` | `{"confirmation":"<user_id>"}` |

Mutations require a UUID `Idempotency-Key`. Every route verifies ownership;
missing and unauthorized servers return the same 404. Audit entries contain
member UUIDs and roles, not passwords or tokens. Replay receipts retain the
member name/email response visible to the owner.

Revocation blocks the next authorized API operation. Existing console streams
recheck authorization normally every 15 seconds and close when access is removed.
A previously started operation completes before a membership change acquires its
lock; if the lock is busy, the owner gets 409 and can retry. Downloads already
sent cannot be recalled.

## Run and verify

Run the normal Compose migration service before starting the updated backend:

```sh
docker compose run --rm migrate
docker compose --profile frontend up -d --build backend frontend
```

Include your existing Panel/Lima overlays when using that deployment. No change
to Panel users or credentials is required.

Run `make check`, `TEST_DATABASE_URL=<dedicated-test-db> make integration`,
and `npm --prefix frontend run build && npm --prefix frontend test`.

# Player setup and invitations

The simple flow is: choose a play style, create a server, add compatible extras,
prepare a player invitation, then share its public link. New server choices are
Paper (normal Minecraft with optional plugins), Fabric, and Forge. Existing
server types remain supported. Detailed guide content and version choices are
collapsed until needed.

Owners prepare, refresh, or disable invitations on the server overview. A guest
needs no Blockyard account: the page shows the server address and, when verified,
a downloadable `.mrpack`. Modrinth App is the default import option; Prism is an
alternative. Modrinth App requires a Microsoft account owning Java Edition.
Players must install a suitable launcher first. Stock Minecraft cannot install
executable mods from a server connection prompt. No companion mod or in-game
download prompt is included in this implementation.

## Backend

Migration 018 adds `player_invites`. `internal/playerjoin` publishes an expiring,
revocable snapshot under an unguessable public link. Owner authorization, a
shared server advisory lock, and source JAR hash verification run before
publication. Public reads use PostgreSQL snapshots rather than Panel calls.

The catalog resolver pins client releases to installed server releases and checks
required dependencies, loaders, Minecraft versions, official download URLs, and
SHA-1/SHA-512 hashes. The generated ZIP contains `modrinth.index.json`; it does not
redistribute uploaded JARs, server configuration, credentials, or private files.
Unknown files or unavailable compatible releases disable automatic pack download.

Routes:

- `GET/POST/DELETE /servers/{id}/player-invite`: owner-only management.
- `GET /join/{slug}`: public connection/setup snapshot.
- `GET /join/{slug}/pack`: verified player pack manifest.

Relevant managed changes invalidate the downloadable pack until the owner
refreshes it. Changes made outside Blockyard, such as SFTP edits, require manual
refresh; publication verifies tracked JAR contents again. Links expire after 30
days. A disabled or expired link returns 404. Invitations do not grant dashboard
access, whitelist approval, or bypass launcher account requirements.

## Scope and validation

This generates client mod download instructions for verified catalog-tracked
addons and their dependencies. It is not an arbitrary full modpack importer and
does not export custom client configuration or overrides. The public page does
not claim that the game process is running; hosts must start it before players
join. Actual launcher import and joining require a Minecraft client test.

Go integration tests cover ownership, expiry/revocation, changed JAR rejection,
manifest content, dependency resolution, and invalidation. Browser tests cover
mobile/desktop guest setup, downloads, owner publication/revocation, and blocked
or expired invitations. The full browser suite passes 136 tests. Go race tests
passed for catalog, playerjoin, management, server, provisioning, and HTTP API;
Go vet/staticcheck and frontend lint/format checks passed.

## Run and deploy

Use the repository's existing development setup and database migration command.
Apply migration 018 before starting the updated backend: managed mutations now
invalidate player invitations in PostgreSQL. Rebuild backend and frontend;
Pterodactyl remains responsible for game containers. No new launcher dependency,
Minecraft restart, or game world change is required to deploy the website/API.

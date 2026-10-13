# Add plugins and mods from the dashboard

On a server's Plugins or Mods page:

1. Make a backup, then stop the server.
2. Search Modrinth for a project. Results match the server's Minecraft version and software.
3. Open its project details and read the installation instructions.
4. Choose **Review install** to see the stable release and required dependency files.
5. Remove older versions of the same projects first. Choose **Install now**, wait for the installation to finish, and start the server.

Paper, Spigot and Purpur install plugins into `/plugins`. Fabric, Forge and NeoForge install mods into `/mods`. Vanilla has no addon catalog. Client-only mods belong on players' computers. Gameplay mods often require matching client installations; the dashboard does not modify players' computers.

## Installation feedback

The review dialog shows real stages and a progress bar based on completed download/verification and upload steps. Release resolution is indeterminate until the file count is known; the bar does not estimate elapsed time or download bytes. Keep the page open until the installation response arrives. Closing or refreshing the page can interrupt the request; an ambiguous remote write requires inspection rather than another blind attempt.

Catalog projects installed through Blockyard show **Installed**, including after refresh. The backend matches durable installation receipts against JAR filenames and sizes still present in the server's plugin/mod folder. Removing a package clears its badge. Manually uploaded JARs remain listed under **Your plugins/mods**; they cannot reliably be attributed to a catalog project just from their filename. An Installed badge indicates package presence, not successful loading by Minecraft.

## ViaVersion

Search **ViaVersion** on the Plugins page. ViaVersion allows newer Java clients to connect to supported older servers; it does not upgrade the world's blocks, mobs or gameplay. ViaBackwards serves the opposite direction (older clients on newer servers), with its own compatibility requirements and ViaVersion dependency. Java/Bedrock crossplay is a different feature and requires a separate Geyser/Floodgate setup.

## Backend implementation

The backend uses the official Modrinth v2 API, not store scraping. New REST endpoints:

- `GET /servers/{id}/plugins/catalog?q=...` (or `mods`)
- `GET /servers/{id}/plugins/catalog/{project}` returns a review with a revision
- `GET /servers/{id}/plugins/catalog/installed` lists projects with recorded packages still present
- `GET /servers/{id}/plugins/catalog/operations/{key}` exposes safe progress metadata for the authenticated actor’s operation
- `POST /servers/{id}/plugins/catalog/{project}` takes `{"revision":"..."}` and an `Idempotency-Key` UUID

No additional credentials, migrations or dependencies are needed. Server software/version come from PostgreSQL. Authorization occurs before catalog access, and installation shares the server advisory lock and durable management-operation audit/idempotency records.

Only stable releases with supported server-side metadata are eligible. Required dependencies are resolved recursively; incompatible declarations, conflicting dependency versions, unsupported files and unavailable releases reject the bundle. Bounds: 20 projects, depth 8, 64 MB per JAR and 128 MB per bundle. Optional dependencies are not automatically installed.

Downloads use HTTPS `cdn.modrinth.com` only, reject redirects, verify exact size and SHA-512, then validate the loader/plugin descriptors. The complete download is checked before file writes. Purchased disk capacity, offline state and membership are checked before uploading through Pterodactyl/Wings. Existing identical files are retained; different contents are never overwritten. An interrupted remote write is marked UNKNOWN and must be inspected instead of blindly repeated. Third-party releases remain executable code and are not certified by Blockyard.

## Modpacks

Automatic complete modpack installation is not included. A pack can contain many mods, loader versions, configuration overrides and client-only files; a `.mrpack` or client ZIP is not a server JAR. Follow the author's server-pack instructions, select its exact Minecraft version and loader, and use Mods/Files/SFTP for its server files. Back up existing worlds and configuration before applying a pack. A future pack installer needs a separate review and recovery workflow for overrides.

## Verification

Run the normal Go tests with `TEST_DATABASE_URL` set for PostgreSQL integration coverage. `internal/catalog` tests cover stable selection, loader/version matching, dependency resolution, download origins and hashes. Management integration tests cover ownership, stale reviews, verified uploads, repeat requests and existing-file conflicts. Frontend Playwright tests cover search, review and explicit installation.

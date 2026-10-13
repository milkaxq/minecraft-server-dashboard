# Guided addon configuration

Simple Voice Chat is the first supported addon. Install its compatible server JAR from Plugins/Mods, then choose **Configure voice chat**. The usual vendor filename includes `voicechat`; renamed JARs cannot always be recognized. A catalog project also shows **Configure** once its recorded package is installed.

The page reads saved configuration through Pterodactyl. Missing fields use the official defaults: voice distance 48, whisper distance 24, groups and recording enabled, client-mod requirement and spectator interaction disabled. **Reset to defaults** changes the form only; **Save settings** applies it while the server is stopped. Distances support 1–256 blocks and whisper distance must not exceed voice distance. Unsupported custom values can be reviewed in Files.

The editor preserves comments, unknown properties and their original text. It replaces duplicate managed keys, checks a revision before saving, validates input, checks current allocation ownership and purchased disk capacity, and rejects symlinks. Fixed paths:

- Paper/Spigot/Purpur: `/plugins/voicechat/voicechat-server.properties`
- Fabric/Forge/NeoForge: `/config/voicechat/voicechat-server.properties`

The backend owns the field definitions, defaults and validation in `internal/management/addons.go`; the frontend renders these definitions. Future supported addons need an explicit schema and safe file adapter rather than arbitrary configuration keys or customer-selected paths.

## Managed voice connections

**Prepare voice connection** enqueues `VOICE_PORT`. The worker reserves a separate allocation from the same PostgreSQL-locked pool as game provisioning, confirms the server/actor/entitlement, then attaches it through the Application API. Existing game allocations, RAM, CPU, disk, backup and database limits are preserved. Customers cannot submit port numbers, allocation IDs or resource limits. The API retains only its existing Client API credentials; Application credentials stay in the worker.

An uncertain assignment retains its reservation and attempt marker. **Check setup again** reconciles Panel state; an absent allocation after an uncertain request is never blindly assigned again. An operator must confirm the original request has finished before manually recovering it. Capacity exhaustion produces a clear setup failure without marking the Minecraft server FAILED. Reservations are released after confirmed server termination, not merely after an HTTP timeout.

The host prepares an approved UDP pool once:

1. Create/import unassigned Pterodactyl allocations on the registered node.
2. Allow their UDP ports in the node/provider firewall. Game DNS must resolve directly to the node.
3. Run the operator-only command (inside the worker container in production):

```sh
control voice-ports --actor ADMIN_UUID --node NODE_UUID \
  --ports 25565,25566,25567 --udp-ready
```

The command enables only distinct, unreserved allocations on that node and records an audit entry. It does not open a firewall. No ports are voice-enabled by default. Ports below 1024 are excluded. One dedicated voice allocation is supported per server in this MVP.

A “Port assigned” label confirms Panel allocation ownership; it is not proof of Internet UDP connectivity or a loaded voice-chat plugin. Save, start the server, and test voice chat in-game. Provider firewall rules remain the host's responsibility.

## Client mods and connection prompts

Players must install Simple Voice Chat for their own client loader, even with a Paper server. The page supplies version-filtered official download links, Fabric/Forge/NeoForge choices, an editable client Minecraft version, and copyable setup instructions. It does not download or execute JARs on players' computers.

Minecraft Java's built-in connection download prompt supports resource packs, not executable mods. General automatic mod synchronization requires an appropriate launcher/modpack workflow or a companion mod already installed before connection. No automatic client-mod installation or launcher is included here. Requiring the client mod disconnects players who do not have it; the default leaves that requirement off.

Official references: [server configuration](https://modrepo.de/minecraft/voicechat/wiki/server_config), [installation](https://modrepo.de/minecraft/voicechat/wiki/installation), [client setup](https://modrepo.de/minecraft/voicechat/wiki/client_setup).

## API and verification

- `GET /servers/{id}/addons/simple-voice-chat`: schema, defaults, current values, revision and safe network status.
- `PUT /servers/{id}/addons/simple-voice-chat`: validated values + revision, with UUID `Idempotency-Key`.
- `GET /servers/{id}/addons/simple-voice-chat/network`: database-only job/port status polling.
- `POST /servers/{id}/addons/simple-voice-chat/network`: empty JSON + UUID `Idempotency-Key`.

OWNER/ADMIN with settings permission can use these endpoints; moderators and foreign users cannot. Migration `015_addon_ports.sql` adds the reservation and capability records. Run migrations before starting updated API/worker processes. Use normal Go tests with `TEST_DATABASE_URL` for database integration tests, including concurrent allocation, ownership, lost-response reconciliation, state/entitlement checks and property preservation. Frontend tests include guided settings, polling/stale edits, role checks, client links, and mobile/laptop screenshots.

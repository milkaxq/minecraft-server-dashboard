# Guided addon setup

Installed plugins and mods expose **Setup & help** beside their installed file. A catalog-linked addon gets a Modrinth project link; manually uploaded unknown files never receive a guessed documentation URL. Project descriptions and remote documentation are never evaluated as code or converted automatically into console commands.

Supported guides:

- Simple Voice Chat: existing server settings form, assigned voice port and player microphone instructions. Default key V is shown with explicit contrasting colors. Players with a changed binding should check Minecraft Controls → Key Binds → Voice Chat.
- Chunky: a reviewed form sends a single explicit `chunky start <world> circle 0 0 <radius>` command through the existing authenticated, audited, idempotent server command API. Radius choices are 500, 1000 and 2000 blocks. World names are restricted to a single safe argument. Acknowledgement is required before starting or resuming generation. Pause targets the selected world. Opening the guide does not execute commands, change selection or restart the server. Responses confirm command submission only; the console remains the source of progress, completion and plugin errors.

Other addons retain configuration-file access and author documentation. Loading an addon once normally creates its initial configuration. Guided forms must be added as reviewed recipes based on the author's supported settings and version differences. An arbitrary JAR has no universal configuration schema; inferring settings from prose could produce invalid or unsafe configurations.

For future file-based recipes, use the Voice Chat backend pattern: server authorization, explicitly allowed settings, bounded values, Pterodactyl file APIs, revision conflict detection, preservation of unrecognized settings, audit logging and restart guidance. Never expose host files, resource limits or secrets. Add version-aware fixtures and authorization tests before enabling a new recipe.

Chunky command source: https://github.com/pop4959/Chunky/wiki/Commands

## Progress and edit reliability

Chunky's expanded setup panel opens a scoped console WebSocket only when the user has console permission and the server is running. It parses the plugin's English progress messages into a world-specific progress bar; chat and unknown formats are ignored. It executes no command on opening. The console may replay previous log output on connection, so receipt time is not the report’s original timestamp. Progress is labelled as last reported, becomes stale after 30 seconds, and is explicitly unconfirmed after disconnection. Reconnect is manual. Closing the panel disconnects the stream. Progress does not confirm task completion; use the full console for unsupported messages and outcomes.

Installed addon search filters the already-loaded file list locally and does not search or modify the public catalog.

Chunky command retries and Voice Chat save retries retain the same operation key while the exact request remains unchanged. A successful response clears the key; changed input creates a new operation. These keys are held only in the mounted form, not across page reloads.

Voice Chat and Minecraft settings warn on browser refresh/tab close/full navigation when there are unsaved changes. Ordinary same-tab dashboard links also ask whether to stay or leave without saving. Hash-only links, downloads and new-tab links are left alone. Programmatic redirects, sign-out actions and browser back/forward navigation are not intercepted by the link guard. Voice Chat also displays an unsaved indicator and an explicit discard action.

Recurring authenticated dashboard reads pause while the tab is hidden or the browser reports offline. Returning to a visible, connected tab triggers a refresh, while preserving any outstanding HTTP 429 Retry-After delay. In-flight requests are not duplicated. One-time resource reads keep their existing behavior.

## Detected worlds and lifecycle reports

`GET /servers/:id/addons/chunky/worlds` requires the same server console permission as Chunky commands. It reads the actual `level-name` and a single top-level directory listing through Pterodactyl, without recursively scanning world files. Only existing, non-symlink directories with command-safe names are offered. Paper-family servers can offer existing Nether and End roots too; other loaders get the main world only. Other properties, including credentials, are never included in the response.

The guide defaults to a detected world instead of guessing `world`. Failed/empty lookups block the normal controls until a detected world is available or the user explicitly chooses manual entry. Manual input still permits only one ASCII command-safe world argument. Custom dimension identifiers outside that format require the advanced console. The start command remains a circle centered at 0, 0; it does not imply spawn coordinates or change the world border.

Recognized English Chunky lifecycle messages display the last reported paused, resuming, cancelled, stopped or finished state. A running task showing 100% remains classified as running until Chunky emits its explicit finish message. Historical replay and disconnection warnings continue to apply. Unknown/localized messages remain available in the full console.

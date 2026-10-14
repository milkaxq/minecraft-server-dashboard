# Players and connection help

The Players page combines live readings, invitations, whitelist management, moderation and guided connection help. The existing Settings permission permits owner/admin access; moderators and other users do not receive these controls. Backend endpoints check permissions before contacting Pterodactyl.

Live counts come from the existing authorized usage endpoint. Its Minecraft server-list probe now collects up to 100 distinct valid username samples. Names are explicitly a sample: servers can hide them, provide a partial list or add non-player text. Invalid names are filtered, output is escaped, and missing/failed readings are never rendered as an empty online list. Samples are not used as proof that a kick worked.

GET /servers/{id}/players/bans reads banned-players.json through Pterodactyl with the existing safe path/symlink checks. POST /servers/{id}/players/actions accepts only kick, ban and unban, a validated Minecraft username, confirmation, revision and a bounded single-line reason for kick/ban. Unban translates to the built-in pardon command. No UUID, Pterodactyl identifier or arbitrary console command is accepted from the browser.

The server must be running. Actions use the existing per-server mutation lock, authorization recheck, durable idempotency, audit and resource/lifecycle guards. Ban/unban success is observed in the saved ban file; delivery without an observed change is explicitly unconfirmed. Kick only reports command delivery because public name samples cannot prove a disconnect. Username bans do not replace whitelist, authentication, IP bans or plugin/proxy rules.

The confirmation dialog keeps the original request body/key after a lost response, even if a ban-list poll changes the revision. Uncertain commands are never automatically repeated. Inspect the console and list after an ambiguous result. The page does not grant OP or alter login/whitelist defaults.

Connection help is user-selected guidance, not automatic diagnosis. It reads current login/whitelist/client requirements and distinguishes unavailable settings from off. Invite friends links to the owner's existing player-invitation controls.

Verification uses a disposable PostgreSQL database, mock Pterodactyl commands, a local Minecraft status protocol fixture, and browser fixtures. Deployment checks are read-only; no live player is kicked, banned or unbanned for testing.

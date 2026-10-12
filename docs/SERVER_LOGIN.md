# Minecraft server accounts

Blockyard website accounts and Minecraft player accounts are separate.

## Join an AuthMe-enabled server

Hosts can manage the whitelist under **Servers → their server → Players**. Add exact Minecraft usernames, search the saved list, remove entries with confirmation, and turn the whitelist on or off. Start Minecraft before making changes; no restart is required. Removing an entry blocks future joins while the whitelist is on; it does not delete player inventory or world progress. Minecraft operators can bypass the whitelist. Website Members are separate from game players.

The dashboard uses `GET /servers/{id}/whitelist` and idempotent `POST /servers/{id}/whitelist`. Only owners/admins with Settings permission can access these routes. Reads use Pterodactyl's sandboxed files API, with symlink checks. Writes send a fixed `minecraft:whitelist` command through Pterodactyl, using validated usernames and a snapshot revision. UUID resolution stays inside Minecraft. Changes share the server operation lock and audit log; the backend checks saved state before claiming success. Command delivery without an observed result is shown as unconfirmed.

Use the exact username allowed by the host. Connect to the server in Minecraft Java with the server's version (or a compatible version supported by its plugins).

On your first visit, complete the AuthMe registration dialog, or enter `/register yourPassword yourPassword` in Minecraft. On later visits, use the login dialog or `/login yourPassword`. Use a unique password of at least 10 characters; never reuse your Microsoft or hosting-dashboard password. Do not send passwords in dashboard support tickets or the web console.

Offline players use this server password. Microsoft-account owners register and log in once, then run `/premium` in Minecraft to opt in to verified auto-login. On subsequent connections, AuthMe and PacketEvents verify the Minecraft session before skipping the password. Use Microsoft login in the launcher; an offline session cannot authenticate an enrolled premium account. Use `/freemium` while authenticated to opt out. Merely using the official launcher or matching a username does not prove ownership. An SKlauncher website account is also separate from a Microsoft Minecraft account.

Servers without a separate login plugin normally use `online-mode=true` and require Microsoft Minecraft authentication. SKlauncher supports both Microsoft and offline login, but an offline session cannot join an online-mode server. If Microsoft login reports an invalid session, sign out of the launcher account and sign back in before launching Minecraft again.

Official launcher guide: https://docs.skmedix.pl/getting-started/login

## Current private-beta setup

The existing Paper 26.3 Private Beta server uses:

- AuthMe 6.0.1 Paper and PacketEvents 2.14.0 for inventory protection and cryptographically verified Minecraft-session login.
- Server-local SQLite for AuthMe accounts, kept inside the isolated Minecraft container.
- BCrypt passwords, minimum length 10, maximum length 64.
- Forced registration/login; unauthenticated movement and chat blocked.
- AuthMe IP-based login sessions disabled.
- AuthMe premium mode enabled; each Microsoft-account owner must opt in with `/premium` after password authentication. IP sessions remain disabled.
- Shutdown on AuthMe database failure enabled.
- `online-mode=false`, `enforce-secure-profile=false`, whitelist enabled.

A completed Pterodactyl backup was made before switching the server's account mode. AuthMe was first started while Minecraft account verification remained enabled; the login mode was changed only after successful plugin initialization. No game operator permissions were granted.

This is an explicit configuration of this server, not the default for newly provisioned servers. Do not remove the login plugin while leaving offline access enabled. Whitelist names alone do not authenticate players. Changing account mode changes Minecraft UUID semantics, so check player inventories, permissions and whitelist entries and retain backups when migrating an existing populated server.

AuthMe documentation: https://github.com/AuthMe/AuthMeReloaded/tree/master/docs

PacketEvents release: https://github.com/retrooper/packetevents/releases/tag/v2.14.0

Premium setup and verification: https://github.com/AuthMe/AuthMeReloaded/blob/master/docs/premium.md

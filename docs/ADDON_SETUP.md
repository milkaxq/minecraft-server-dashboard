# Guided addon setup

Installed plugins and mods expose **Setup & help** beside their installed file. A catalog-linked addon gets a Modrinth project link; manually uploaded unknown files never receive a guessed documentation URL. Project descriptions and remote documentation are never evaluated as code or converted automatically into console commands.

Supported guides:

- Simple Voice Chat: existing server settings form, assigned voice port and player microphone instructions. Default key V is shown with explicit contrasting colors. Players with a changed binding should check Minecraft Controls → Key Binds → Voice Chat.
- Chunky: a reviewed form sends a single explicit `chunky start <world> circle 0 0 <radius>` command through the existing authenticated, audited, idempotent server command API. Radius choices are 500, 1000 and 2000 blocks. World names are restricted to a single safe argument. Acknowledgement is required before starting or resuming generation. Pause targets the selected world. Opening the guide does not execute commands, change selection or restart the server. Responses confirm command submission only; the console remains the source of progress, completion and plugin errors.

Other addons retain configuration-file access and author documentation. Loading an addon once normally creates its initial configuration. Guided forms must be added as reviewed recipes based on the author's supported settings and version differences. An arbitrary JAR has no universal configuration schema; inferring settings from prose could produce invalid or unsafe configurations.

For future file-based recipes, use the Voice Chat backend pattern: server authorization, explicitly allowed settings, bounded values, Pterodactyl file APIs, revision conflict detection, preservation of unrecognized settings, audit logging and restart guidance. Never expose host files, resource limits or secrets. Add version-aware fixtures and authorization tests before enabling a new recipe.

Chunky command source: https://github.com/pop4959/Chunky/wiki/Commands

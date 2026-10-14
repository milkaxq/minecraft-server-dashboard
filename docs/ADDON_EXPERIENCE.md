# Guided addon installation

Catalog cards show that the release matches the server alongside the publisher's client requirement. Matching is enforced by the existing backend resolver, including Minecraft version, loader, stable release, dependencies and file integrity. It is not a guarantee that an addon runs successfully with every other installed addon.

Installation review shows the file count and dependencies before the existing backup safety decision. It explains that server installation and client downloads are separate. Existing progress polling and installed-file badges remain in place.

After a successful installation, upload or removal, the page offers configuration, server controls and a link to refresh the player invitation. It does not start Minecraft or publish a new pack automatically. Guided configuration currently supports Simple Voice Chat; other addons use configuration files and their author's instructions. The next-steps panel describes changes made during the current page visit.

Failed reviews have a read-only retry. Installation errors warn about partial writes and link to inspect files. Retrying an unchanged installation retains its idempotency key; backend handling of uncertain writes remains authoritative. Rate-limit failures instruct the user to wait.

The backend invalidates published player downloads after addon mutations, including uncertain outcomes. The invitation link remains available but downloads are blocked until the owner refreshes setup. The addon page links directly to the invitation card.

Tests use mocked installations; deployment does not install addons, change player access, restart Minecraft or publish invitations on the live server.

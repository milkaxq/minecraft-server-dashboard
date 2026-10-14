# Guided game settings

The Settings page offers Survival adventure, Peaceful survival and Creative building as starting points. Each choice changes only game mode, difficulty, hardcore and PvP. Login verification, whitelist, player limits and hosting resources remain unchanged.

A live review lists each unsaved value beside its saved value. Choosing a preset does not save anything. Discard changes restores the loaded values; reloading is disabled while edits are pending to avoid losing them accidentally.

The server must be stopped before saving. Changes apply on the next start. Game mode sets the default for new players; existing players may retain their mode.

The existing backend remains authoritative: it checks permissions, validates fields, requires an offline server and checks the server.properties revision to reject stale edits. Presets do not change provisioning defaults or installed plugins.

Browser tests cover reviewed edits, discarding, explicit saving, preserved joining rules and limits, and revision submission. Deployment checks do not change live Minecraft settings or restart the game.

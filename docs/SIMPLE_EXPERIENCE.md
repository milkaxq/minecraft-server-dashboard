# Simple dashboard

The server overview defaults to Play: power controls, connection address and an
invitation. Join, Health and Manage are direct view buttons, not nested help
accordions. The first-session checklist and generic contextual-help boxes are no
longer rendered. The dedicated Guide remains available for detailed instructions.

Joining uses short steps. Invite download choices and link revocation are directly
accessible. Installed add-ons link to documentation without tutorial panels;
Chunky's terrain form remains on demand because it contains actual controls and
opens a scoped console connection. Voice Chat keeps its existing Configure link.
Addon updates are checked from a visible button without an extra accordion.

Health details load when that view is selected. Startup issues appear only when
found; the symptom selector gives one short suggestion. Diagnostic copying and
its manual-copy fallback remain available. No new telemetry is claimed.

Destructive confirmation, backup requirements, authorization, resource limits,
unsaved-edit protection and payment/provisioning logic are unchanged. This is a
presentation change, not removal of safeguards or automatic Minecraft actions.

## Creating a server

Creation shows one of three steps at a time: play style, group size, then name and
review. Back/step navigation preserves choices. Regular Minecraft and plugin
choices provision Paper without automatic add-ons; mods prefer Fabric when
available, otherwise Forge. Advanced settings contain approved software/version
choices and an enabled-plan override. The default version is the newest configured
version for the selected software, not a claim about the latest global release.

Prices and group recommendations use API plans. Disabled plans are excluded from
recommendation and manual selection. Order bodies still contain only the plan
UUID, name, configured software and version. No navigation action provisions a
server; the final submit retains the existing payment/beta permissions and
idempotency key for identical retries. Selecting mods does not install a modpack.

Checks cover the three-step flow at phone/laptop sizes, focus and Back navigation,
no early submission, software availability, disabled plans, latest configured
version, order payloads, retry keys, checkout and beta paths. API fixtures are used
without creating customer servers.

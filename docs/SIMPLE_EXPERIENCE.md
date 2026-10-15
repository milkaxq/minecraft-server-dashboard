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

## Returning to play

The dashboard uses a compact Continue playing card for the first available server
and permission-checked quick actions. It no longer embeds a plan catalogue,
hosting-total accordion, repeat beta checklist or promotional hero. Plans remain
in the creation flow, so returning customers avoid that extra catalogue request.
An empty private beta account gets one access message in the server empty state.
Search ignores leading/trailing spaces.

Notifications appear as a direct list only when unread items or an error exist.
No empty-inbox panel or help accordion is rendered. Mark read hides an item only
after the backend confirms success, without clearing other rows for a list reload.
A bounded local set prevents an old list from flashing the acknowledged item back;
normal polling remains authoritative for newly arriving notifications. Failed
reads show a retry button, and failed writes keep the item available for retry.
No acknowledgement is sent automatically.

## Sharing with fewer choices

A one-active-server dashboard omits search/view controls. The Servers directory
keeps them, and the dashboard also shows them for multiple servers, cancellation
history, pending deletion or an existing filter. Removed servers therefore remain
reachable through All servers.

The invitation card keeps publish/copy/preview as its main actions. Beta and
optional-download choices and link revocation live in the Player download options
dialog. The dialog uses a separate draft; Cancel discards it without publishing.
Update invitation explicitly publishes and closes only after success. Basic
invitations still default to stable, required-only downloads.

Copy requires the currently loaded, unexpired setup to be shareable. Errors or a
changed/blocked pack pause copying. A pack error takes priority over a ready flag
in the displayed status. Denied clipboard access shows a selectable public URL;
it does not expose dashboard credentials. Old fallback/success messages are hidden
when the invite changes or is no longer shareable. Failed reads offer a GET retry
without replacing or revoking the invitation.

## The player's invitation page

The main join path removes repeated promotional/setup sentences. Joining help and
host-published notes/mod requirements open in optional dialogs with one Close
button, rather than expanding the main page. Required loader/mod steps and
conditional whitelist/login rules remain visible where needed. Returning-player
confirmation is still an explicit browser preference, never launcher detection.

Copying a game address uses a selectable read-only fallback when clipboard access
is denied. This is separate from API errors: a copy failure does not turn a valid
invitation into an error page. Copy feedback is tied to the current address.

Public setup refresh pauses while the page is hidden or the browser is offline.
Returning/reconnecting requests a current read without overlapping an existing
request or bypassing a rate-limit cooldown. Failed/removed setup still clears the
usable page and download controls. No download or Minecraft action is retried by
these refresh events.

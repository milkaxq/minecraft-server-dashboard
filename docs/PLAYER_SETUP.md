# Player setup and invitations

The simple flow is: choose a play style, create a server, add compatible extras,
prepare a player invitation, then share its public link. New server choices are
Paper (normal Minecraft with optional plugins), Fabric, and Forge. Existing
server types remain supported. Detailed guide content and version choices are
collapsed until needed.

Owners prepare, refresh, or disable invitations on the server overview. A guest
needs no Blockyard account: the page shows the server address and, when verified,
a downloadable `.mrpack`. Modrinth App is the default import option; Prism is an
alternative. Modrinth App requires a Microsoft account owning Java Edition.
Players must install a suitable launcher first. Stock Minecraft cannot install
executable mods from a server connection prompt. No companion mod or in-game
download prompt is included in this implementation.

## Backend

Migration 018 adds `player_invites`. `internal/playerjoin` publishes an expiring,
revocable snapshot under an unguessable public link. Owner authorization, a
shared server advisory lock, and source JAR hash verification run before
publication. Public reads use PostgreSQL snapshots rather than Panel calls.

The catalog resolver pins client releases to installed server releases and checks
required dependencies, loaders, Minecraft versions, official download URLs, and
SHA-1/SHA-512 hashes. The generated ZIP contains `modrinth.index.json`; it does not
redistribute uploaded JARs, server configuration, credentials, or private files.
Uploaded files without catalog records and unavailable required client releases disable automatic pack download. Catalog addons with missing client metadata do not block publication: they stay on the server and are omitted from the client manifest with a named note. Optional addons without a compatible client release are omitted the same way. This is not a claim that unknown addons support vanilla clients.

Routes:

- `GET/POST/DELETE /servers/{id}/player-invite`: owner-only management.
- `GET /join/{slug}`: public connection/setup snapshot.
- `GET /join/{slug}/pack`: verified player pack manifest.

Relevant managed changes invalidate the downloadable pack until the owner
refreshes it. Changes made outside Blockyard, such as SFTP edits, require manual
refresh; publication verifies tracked JAR contents again. Links expire after 30
days. A disabled or expired link returns 404. Invitations do not grant dashboard
access, whitelist approval, or bypass launcher account requirements.

## Scope and validation

This generates client mod download instructions for verified catalog-tracked
addons and their dependencies. It is not an arbitrary full modpack importer and
does not export custom client configuration or overrides. The public page does
not claim that the game process is running; hosts must start it before players
join. Actual launcher import and joining require a Minecraft client test.

Go integration tests cover ownership, expiry/revocation, changed JAR rejection,
manifest content, dependency resolution, and invalidation. Browser tests cover
mobile/desktop guest setup, downloads, owner publication/revocation, and blocked
or expired invitations. The full browser suite passes 136 tests. Go race tests
passed for catalog, playerjoin, management, server, provisioning, and HTTP API;
Go vet/staticcheck and frontend lint/format checks passed.

## Run and deploy

Use the repository's existing development setup and database migration command.
Apply migration 018 before starting the updated backend: managed mutations now
invalidate player invitations in PostgreSQL. Rebuild backend and frontend;
Pterodactyl remains responsible for game containers. No new launcher dependency,
Minecraft restart, or game world change is required to deploy the website/API.

## Deployment check — 2026-10-13

Commit `3b46148` deployed to the VPS; migration 018 applied before backend startup.
The public readiness endpoint returned ready. Publishing an owner invitation for
Private Beta succeeded. Its automatic pack remains disabled because a legacy or
uploaded addon has no verified catalog record. No plugins were removed or game
settings changed. Local fixture tests validate ready-pack generation; the live
server still needs its unverified addon reconciled before testing launcher import.

The client-metadata follow-up removes the blanket unknown-metadata block. Modrinth project/version environment fields describe required, optional, and server-only use; absent or conflicting metadata remains unspecified. Catalog provenance and client compatibility are separate checks.

Live follow-up: VeinMiner no longer blocks publication. Simple Voice Chat 2.6.24
client builds for Minecraft 26.3 are marked beta by Modrinth (Fabric, Forge, and
NeoForge). The stable-only client resolver now reports that exact reason instead
of describing the source as unverified. Private Beta's required voice client
still prevents a stable-only automatic pack; its server plugins remain installed.

Owners can now select **Include beta client builds** before publishing or
refreshing an invitation. The API accepts `{ "include_beta": true }`; the choice
is saved with the published snapshot and audit event. It only relaxes the release
channel for client pack resolution: pinned release matching, loader/game checks,
required dependencies, approved CDN URLs, and hashes still apply. Alpha builds
remain excluded. Public setup notes disclose beta files. Server catalog install
policy is unchanged.

Beta opt-in live check: Private Beta was republished with `include_beta=true`.
The public pack endpoint returns a valid ZIP manifest for Minecraft 26.3 and
Fabric Loader 0.19.5, with Simple Voice Chat 2.6.24, compatible optional client
builds, and required dependencies. All manifest downloads have SHA-1/SHA-512
hashes. The public page discloses beta builds. Actual launcher import and joining
remain a player-side test.

## Launcher selector

The public player page offers Modrinth App, SKlauncher, Prism, the official
Minecraft Launcher, and another launcher. The browser remembers its selection.
Modrinth and Prism show `.mrpack` import steps. SKlauncher/official/other show
manual profile setup and the exact client downloads from the published manifest,
including dependencies. Selecting a launcher does not change the pack's loader,
launch Minecraft, or install software. Players must use the displayed loader;
Fabric client JARs do not belong in a Forge profile.

`GET /join/{slug}/files` serves only the public manifest's approved CDN URLs,
paths, sizes and hashes. It shares invitation expiry, revocation, invalidation,
no-store headers, and public rate limiting. It never reads server files directly.

Instructions refer to official documentation:
- https://docs.skmedix.pl/faq/launcher-related (3.x manual files; 4.x mod management)
- https://prismlauncher.org/wiki/help-pages/Zip-import/ (Prism `.mrpack` import)
- https://support.modrinth.com/en/articles/8802250-modpacks-on-modrinth

Launcher selector deployment check: seven focused browser tests passed, including
SKlauncher selection, remembered choice, pinned links and mobile layout. Go race
tests for playerjoin and HTTP API, vet/staticcheck, frontend lint/build/format
passed. A live mobile browser selected SKlauncher and displayed all five pinned
Private Beta client downloads with no horizontal overflow; readiness returned 200.
No launcher was installed or Minecraft client launched during these checks.

## Required-only player packs and manual ZIP

New and refreshed invitations omit optional client roots by default. Required
mods still resolve their required dependencies. Simple Voice Chat follows its
actual `Require voice chat` setting; optional voice downloads are included only
when the owner enables **Include optional client features**. This only changes
player downloads and leaves server plugins/mods installed.

Manual launchers now offer **Download client mods ZIP**. `GET /join/{slug}/bundle`
fetches the published files from the approved Modrinth CDN, checks file sizes and
SHA-512/SHA-1, then builds `mods/*.jar`, `README.txt`, and `client-files.json`.
Paths, file count and total size are bounded. Bundle work shares the two transfer
slots, has a separate six-per-minute public budget and a 90-second timeout.
Expiry/revocation/setup validity is checked again after file downloads. Individual
CDN downloads remain available under a collapsed section.

The ZIP does not install the Minecraft loader or launch the game. Extract it and
copy the `mods` folder into a separate installation using the displayed version
and loader. Do not put the ZIP itself in the mods folder.

Required-only ZIP live check: Private Beta was republished with beta builds enabled
and optional client features disabled. Its client manifest now contains only
`voicechat-fabric-2.6.24+26.3.jar`. The public ZIP returned 5,613,921 bytes and exactly
that JAR, README.txt, and client-files.json; the downloaded JAR matched both stored
hashes. A live 390px browser displayed the SKlauncher ZIP button without horizontal
overflow. Readiness returned 200. No server plugins were removed. Go race tests,
vet/staticcheck, frontend build/lint/format and seven focused browser tests passed.

## SKlauncher 4 import-first flow

The SKlauncher choice now targets version 4, with one `.mrpack` download as its
primary path: drag it into SKlauncher 4, confirm import, wait for setup, and launch
the new instance. The manual ZIP and individual downloads remain under a closed
fallback section, and their file list is fetched only when the fallback opens.
No SKlauncher 3 setup steps appear in the primary flow.

Verification used the official SKlauncher 4.0.54 release from
https://github.com/sklauncher/binaries/releases/tag/v4.0.54 . Its shipped import
file filter accepts `mrpack`, `zip`, and `skpack`; its archive detector and parser
read `modrinth.index.json` and support formatVersion 1 / game minecraft. Only the
application archive was read from bounded ranges of the official release ZIP;
no launcher was installed or executed. This verifies format support, not a
completed real-world import or Minecraft launch.

The official changelog documents drag-and-drop modpack imports from 4.0.30:
https://next.skmedix.pl/changelog . Current launcher downloads are linked through
https://next.skmedix.pl/downloads . OS double-click association is not assumed.

Import-first deployment verification: seven focused browser tests, frontend
lint/build/format passed. A live 390px browser selected SKlauncher 4 and displayed
the player-pack download and import instructions with the manual ZIP hidden;
there was no horizontal overflow, and readiness returned 200. Actual import and
Minecraft launch remain a client-side acceptance test.

## Owner invitation card

The owner card makes copying a ready invitation the primary action. Player
download options and detailed inclusion notes are collapsed; changed choices,
blocked setup and expiry bring refreshing back to the foreground. Expired invite
URLs are not offered for copying or previewing.

The card now reads the saved invitation every 30 seconds and after publishing or
revoking. It does not retain a separate publish response that could mask later
backend invalidation. During initial loading, copying/publishing is disabled.
Public access rules and backend compatibility checks remain unchanged.

Focused browser tests cover normal publication/revocation, changed setup polling,
expired links, guest download paths, and SKlauncher 4 with its manual fallback.

Owner-card validation: all 140 browser tests passed, including mobile/laptop
visual checks, with frontend lint, production build and formatting checks passing.
This is a dashboard improvement; no Minecraft client gameplay test was performed.

# Minecraft software and beginner guide

Seven software types are supported: Vanilla, Paper, Spigot, Purpur, Fabric,
Forge and NeoForge. Available versions come exclusively from PostgreSQL;
adding frontend labels does not enable an unconfigured release.

## Architecture

`internal/server/catalog.go` is the backend software allowlist and capability
mapping. Migration 008 expands database constraints and adds per-release egg
variables. Operator configuration in `internal/provisioning/software.go` accepts
`version_environment`; the worker merges it over the software environment and
then sets the selected Minecraft version. The resolved request is persisted
before provisioning, so retrying retains the selected loader and resources.
Only a loader version is exposed publicly, never the egg ID or environment map.
The new eggs write a completion receipt only after all installation commands
succeed. The worker checks that receipt and the launcher through the separate
Client API before marking the server ready. This also catches Wings versions
that report completion without checking an installer exit code. Configure
`PTERODACTYL_INSTALLATION_API_KEY` as a separate privileged Client API token
for the worker. Keep it server-side; the verifier only uses file listing and
reading. The API retains its own management token, and only the worker has
the Application API key.
Existing servers display the loader from their provisioning snapshot.

Paper/Spigot/Purpur share plugin management and separate Nether/End world
folders. Fabric/Forge/NeoForge have Mods pages and use their matching loader
metadata. Mod operations require the existing Files permission (OWNER/ADMIN),
use the same offline checks, no-follow path validation, idempotency and audit
transactions as other file operations, and delegate all transfers to Wings.
Upload checks identify loader descriptors; they are not a full dependency or
Minecraft-version solver. Advanced Files remains available for server packs and
configuration. No automatic marketplace scraping or arbitrary host scripts.

## Install the new templates on Panel v2

Reviewed PTDL_v2 egg exports are in `deploy/pterodactyl/eggs`. These are project
installers using official distribution endpoints, not upstream source clones.
They use Java 21, leave EULA acceptance to the customer, and reserve 25% of the
container RAM outside the Java heap. Forge and NeoForge verify Mojang artifacts
against their per-release SHA-1. Installers stop on failed downloads and never
silently replace an unavailable requested release with another version.

Initial examples target Minecraft **1.21.1**:

| Software | Build/loader |
| --- | --- |
| Spigot | BuildTools `--rev 1.21.1`, compiled from official sources |
| Purpur | 2329 |
| Fabric | Loader 0.19.5 / installer 1.1.2 |
| Forge | 1.21.1-52.1.0 |
| NeoForge | 21.1.256 |

BuildTools itself uses its official last-successful artifact; the Minecraft
revision is fixed. Spigot compilation is expensive: allocate sufficient
installation memory and disk, and validate your node's installation limits.
Pterodactyl runs the installer in an isolated Wings installation container.
The supplied Wings settings reserve a 2 GiB installer limit. A 1 GiB installer
was insufficient for a fresh Spigot 1.21.1 build. Account for this temporary
memory separately from running game servers and limit concurrent installations.

First build the installation image on every Wings node (or publish it to your
private registry and update each egg's installer image):

```sh
make minecraft-installer
# Or build directly on a standalone Wings host:
docker build -t blockyard/minecraft-installer:java21 deploy/pterodactyl/installer
```

The installer extends the Java 21 image with root inside its disposable
installation container, as required by Wings. The customer runtime remains the
non-root Java image. Do not use the installer image as a game runtime.

Import via Panel's administrator UI or use the optional operator script:

```sh
# Supply a privileged Panel v2 admin API token through the operator environment.
# Do not add this token to customer/browser configuration.
python3 scripts/install-minecraft-software.py \
  --panel-url http://127.0.0.1:8090 --output .local/software-config
```

The script reads `PTERODACTYL_ADMIN_API_KEY`, imports the eggs, and writes JSON
with the destination Panel's real egg IDs. Re-running reuses matching eggs and
rejects conflicting executable configurations. Use `--update-reviewed` only
after reviewing an intentional change to a project egg. `egg_id: 0` in checked-in
examples intentionally prevents configuration before an actual import.
It does not modify existing servers or enable customer orders.

After applying migrations, test each installation on your node and then register
its JSON through the operator CLI:

```sh
DATABASE_URL='your operator database connection' \
  go run ./cmd/control software -actor YOUR_ADMIN_UUID \
  -file .local/software-config/fabric.json
```

Repeat for Forge, NeoForge, Purpur and Spigot. In Docker, mount/copy the JSON and
run `control software` in the backend container using its database environment.
The CLI requires an application administrator and audits every configuration.
No capacity is added by importing an egg; order provisioning still reserves
purchased resources under PostgreSQL locks.

For another Minecraft release, review its Java requirement, installer, loader
compatibility and memory needs before adding it. The current template image is
Java 21 for all approved releases; do not add Java 17/25-only releases without
updating the template design accordingly. Loader pins and `MOJANG_SERVER_SHA1`
are operator-managed. Forge/NeoForge
pre-download the verified Mojang artifact with bounded curl retries to avoid
their installer’s short network timeout. To add a release, obtain its official
server checksum and review the installer artifact path too. The completion
receipt must remain part of any replacement template for these five types.
Changing a template affects future provisioning, not already provisioned servers.

## Customer workflow

Open **Guide** for the interactive software chooser and installation tutorial.
Its topic links jump directly to setup, extras, worlds, troubleshooting and
custom code. Expand the help panel on server creation or a management page for
short instructions and a direct link to the relevant tutorial. Troubleshooting
links automatically open the matching answer and move keyboard focus to it.
Choose the required software during server creation. For a loader server, stop
it, open **Mods**, upload compatible `.jar` files and their dependencies, and
start it again. To replace a jar, explicitly delete the old file first.
Players may need the matching loader/modpack installed on their own computers.
There is no in-place software conversion; back up and test a world copy on a new
server before moving between software types.

Official references: [Spigot BuildTools](https://www.spigotmc.org/wiki/buildtools/),
[Purpur API](https://purpurmc.org/docs/purpur/downloads/),
[Fabric server](https://wiki.fabricmc.net/player:tutorials:server:windows),
[Forge downloads](https://files.minecraftforge.net/net/minecraftforge/forge/index_1.21.1.html),
[NeoForge server](https://docs.neoforged.net/user/docs/server/).

## Live validation status (2026-10-09)

Live checks now cover Vanilla, Paper, Spigot, Purpur, Fabric, Forge and NeoForge
for the approved Minecraft 1.21.1 configuration. The resumed Spigot build
completed inside the isolated 2 GiB installer container; its completion receipt,
Java launcher, EULA gate, plugin upload/list/delete and idempotent upload replay
passed. Vanilla installation, launcher and EULA refusal also passed. Earlier
loader checks verified the corresponding mod operations and purchased limits.
These are installation and management smoke tests, not player-load benchmarks
or compatibility certification for arbitrary plugins/modpacks.

Disposable validation servers were terminated, reservations released, the test
plan disabled, and the temporary allocation removed. Cancelled test history was
archived under the local fixture account; the original Paper world was retained.
Normal backend/frontend/migrate/worker Docker builds passed without the offline
packaging workaround. Additional Minecraft releases still require their own
reviewed templates and live validation before being offered.

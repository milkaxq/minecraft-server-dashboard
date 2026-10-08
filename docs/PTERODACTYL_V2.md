# Pterodactyl v2 with Docker

This project now targets a pinned Pterodactyl **v2 development build**. Panel and
Wings are containerized infrastructure dependencies. Go still manages customers,
plans, billing, authorization and capacity in PostgreSQL; it never runs Minecraft.

Panel v2 remains pre-release. Its official Docker instructions warn that
`ghcr.io/pterodactyl/panel:latest` does not yet mean v2. Our overlay builds the
upstream Dockerfile at commit `f154449a8075595d12b8b01fa52cab570d04056d`, with local
image tag `blockyard/pterodactyl-panel:v2-f154449a8075`. Review and update both
values together when deliberately upgrading. Wings uses `v1.13.3`, which the v2
documentation lists as tested with Panel v2. Panel v2 does not imply Wings v2.

## Layout

| Service | Responsibility |
|---|---|
| frontend / backend | Customer dashboard and Go REST API |
| postgres / redis | Hosting application storage and optional cache |
| worker | Durable provisioning/lifecycle jobs; Application API key only |
| panel | Panel v2, including its PHP queue worker and scheduler |
| panel-database | Separate MariaDB database required by Panel |
| panel-cache | Separate Panel Redis cache/session/queue storage |
| wings | Linux infrastructure agent controlling isolated game containers |

`docker-compose.yml` remains usable for backend development or an external Panel.
`docker-compose.pterodactyl.yml` adds Panel and its dependencies, internal routing,
health dependencies and persistent keys/logs/extensions. `docker-compose.wings.yml`
adds the opt-in Linux node. Panel database/cache have no published ports and use
an internal network. Only Wings mounts the Docker socket and game-server paths;
Go, the frontend and Panel have neither. Wings runs without `privileged: true`,
but its Docker socket still grants host-level authority: it is a trusted operator
service, never customer-accessible infrastructure.

The Application and Client API paths remain `/api/application` and `/api/client`.
Our Go adapter needs no Admin API or nest endpoints. Existing allocation/resource
checks remain authoritative; the v2 status decoder adds suspension handling and
rejects conflicting installation status. API media-type `v1` is independent of
Panel's major version and remains the compatibility protocol used by this adapter.

## 1. Prepare development configuration

Run the commands from the repository root. If `.env` already exists, edit it;
do not overwrite your existing configuration.

```sh
cp .env.example .env   # Only for a new setup
openssl rand -hex 32   # Generate each password/secret separately
```

Set:

- `POSTGRES_PASSWORD` and the matching password in `DATABASE_URL`.
- `PANEL_DATABASE_PASSWORD` and a separate `PANEL_DATABASE_ROOT_PASSWORD`.
- `APP_ENV=development`; leave `PANEL_PUBLIC_URL=http://localhost:8090` for local use.
- Optionally `PAYMENT_PROVIDER=mock` and a generated `MOCK_PAYMENT_SECRET`.

Keep `.env` uncommitted. The Panel creates and persists its own encryption key and
Hashids salt in `panel_keys:/app/var`; never regenerate these for an existing database.
Back up that volume together with the Panel MariaDB volume. PostgreSQL remains a
separate backup obligation. Avoid `docker compose down -v` on persistent data.

## 2. Build and initialize Panel

Requires Docker Compose and network access to GitHub, Docker registries, npm and
Composer. The PHP/Node image build can take several minutes.

```sh
make panel-build
make panel-init      # Start MariaDB/Redis, apply migrations and seed the fresh Panel
make panel-admin     # Interactive: create the Panel administrator
make panel-up
```

Open `http://localhost:8090`. The Panel image includes its scheduler and queue
worker; do not add another cron or `pteroq` service. Migrations are explicit:
`panel-up` does not silently migrate an existing installation. These initialization
steps target a fresh v2 database, not migration of an existing 1.x deployment.

Panel and the hosting application can run on Docker Desktop on macOS. For the
complete game-hosting stack below, use a native rootful Linux Docker Engine on a Linux VM/VPS/server.
The provided Wings setup relies on host networking and identical host/container
bind paths and intentionally refuses `make wings-up` on macOS. It is not a tested
Docker Desktop game-node recipe.

## 3. Configure the Linux Wings node

For the provided single-host development setup, run Panel and Wings on the same
Linux Docker host. In Panel, create an empty dedicated node with:

- FQDN `wings.localhost`, HTTP, daemon port **8180**, SFTP port **2022**.
- Data directory `/var/lib/pterodactyl/volumes`.
- Physical RAM/disk budgets that leave room for the OS and this control plane.
- Allocations using the Linux host's actual reachable IP and available Minecraft
  ports, such as 25565–25570. Do not use a localhost hostname for player allocations.

Panel and Go map `wings.localhost` to Docker's host gateway. The Wings API listens
on the Linux host at 8180; the existing Go API uses 8080, avoiding a collision.
The local Panel is published only on loopback at 8090. These addresses are for
single-host development; remote access requires real DNS and an appropriate TLS/
reverse-proxy configuration, not simply exposing this HTTP example publicly.

Create the node directories on Linux:

```sh
mkdir -p .local/wings
chmod 700 .local/wings
sudo install -d -m 0750 /var/lib/pterodactyl /tmp/pterodactyl
```

Copy the **complete configuration generated by Panel** into
`.local/wings/config.yml`, then `chmod 600 .local/wings/config.yml`.
Merge the settings in `deploy/pterodactyl/wings-settings.example.yml` into that
file while preserving its generated `uuid`, `token_id` and `token`.
The settings file is an override example, not a standalone runnable configuration.

Important values:

- `api.port: 8180`, with TLS disabled only for this private development setup.
- `remote: http://127.0.0.1:8090`: host networking reaches the published Panel port.
- `allowed_origins` includes `http://panel`: our Go relay uses its internal Panel
  origin, which differs from the browser's `http://localhost:8090` address.
- `system.data`, backups and temporary paths match the mounted host paths.

```sh
make wings-up
```

Wings manages sibling game containers through the Linux host's Docker daemon.
It does not run Docker-in-Docker. Game containers receive only their own server
files, not the Wings configuration or Docker socket. Check node connectivity in
Panel before ordering servers. Choose egg images supporting your node's architecture.

## 4. Connect our application

Create an Application API key with user/server read-write and node/allocation read
permissions. Create a separate Client API key from a trusted Panel administrator
account able to access the hosted servers. Put them in `.env`:

```dotenv
PTERODACTYL_APPLICATION_API_KEY=YOUR_PRIVATE_APPLICATION_KEY
PTERODACTYL_CLIENT_API_KEY=YOUR_PRIVATE_CLIENT_KEY
```

For the bundled stack, leave `PTERODACTYL_URL` empty: the overlay supplies
`http://panel`, reachable inside Compose. It also supplies exact Wings WebSocket
and transfer origins. Do not set it to browser `localhost:8090`, which means the
Go container itself rather than Panel. External deployments can still set a real
HTTPS Panel URL and matching allowed origins.

```sh
make pterodactyl-config  # Validate quietly without dumping interpolated secrets
make hosting-up         # Build/start backend, frontend and provisioning worker
```

Open `http://localhost:3000`, register and verify an application operator account.
Promote it with the CLI; this account is separate from the Panel administrator:

```sh
docker compose exec backend control admin \
  --email operator@example.com --reason 'Initial platform operator'
```

Use the returned UUID as `ADMIN_ID`. Import software templates and register the
empty node using the [Phase 2 operator commands](PHASE2.md), adding
`-f docker-compose.yml -f docker-compose.pterodactyl.yml` to their Compose commands.
For example, with the actual Panel node ID and your sellable budgets:

```sh
docker compose -f docker-compose.yml -f docker-compose.pterodactyl.yml \
  --profile pterodactyl run --rm --entrypoint control worker node \
  --actor "$ADMIN_ID" --panel-id 1 --ram-mb 12288 --disk-mb 102400 --cpu 400 --dedicated
```

Those budgets are examples, not detected host capacity. Verify egg IDs/startup/
variables against your installed Vanilla/Paper eggs; the older example template's
ID is not an automatic v2 egg mapping. v2 groups eggs with tags rather than nests.

Create a customer account and test choose plan → demo payment → provisioning → Start.
Then verify console, usage, settings, files, plugins, backups, expiry suspension and
renewal. Confirm another customer cannot access this server. Demo payments collect
no money.

## Commands, checks and limits

```sh
make test-compose       # Deployment isolation/credential/path/pinning checks; no containers start
make lint fmt-check
make pterodactyl-down   # Stop control-plane services, preserving volumes
```

`pterodactyl-down` does not stop or delete game containers independently managed by
Wings. Stop game servers through their dashboard before maintenance. Never expose
the Docker daemon over TCP or give customers host/Panel administrator credentials.

Validated in this workspace: Go build, lint/format checks, the full PostgreSQL-backed
race suite, and four Compose isolation/credential/path/pinning tests. The added
source-contract tests cover v2 status/ownership/resources alongside existing
Application/Client API fixture tests. They do not constitute a live Panel/Wings
Minecraft smoke test. The upstream Panel Docker image build was attempted but
failed when the Alpine package indexes timed out; the failed fetch then caused
package-not-found errors. Wings registry inspection also encountered a TLS timeout.
No live Panel/Wings instance was started. Rerun `make panel-build` with reliable
registry access before the initialization steps above.

Production needs a separately reviewed deployment with TLS,
trusted proxy/client-IP policy, secret management and reliable backups.

Official references: [v2 Docker deployment](https://docs.pterodactyl.io/v2/panel/docker),
[requirements](https://docs.pterodactyl.io/v2/panel/requirements),
[Wings compatibility/setup](https://docs.pterodactyl.io/v2/wings/installing),
[API changes](https://docs.pterodactyl.io/v2/upgrading/changes-from-v1).

# PlayBlockyard private-beta deployment

The VPS uses Ubuntu 24.04, rootful Docker, and host-installed Caddy. The
customer dashboard and API share `https://app.playblockyard.com`; Caddy strips
the `/api` prefix before forwarding requests to Go. Panel and Wings use
`panel.playblockyard.com` and `node.playblockyard.com`. Minecraft connections
use `play.playblockyard.com` with the allocated game port. DNS records are
DNS-only A records. The root domain is not configured as a website yet.

Panel returns explicit port 443 in node URLs. Configure the exact origins:

```dotenv
PTERODACTYL_URL=https://panel.playblockyard.com
PTERODACTYL_WEBSOCKET_ORIGINS=wss://node.playblockyard.com:443
PTERODACTYL_TRANSFER_ORIGINS=https://node.playblockyard.com,https://node.playblockyard.com:443
```

When transferring an archive from macOS, use `COPYFILE_DISABLE=1 tar ...`
to omit AppleDouble metadata. Files named `._*.sql` must not be included in
migration directories; Docker also excludes these metadata files explicitly.

`deploy/Caddyfile` provides the HTTPS routes. Docker publishes application and
Panel ports on loopback only. The VPS overlay removes PostgreSQL and Redis
published ports entirely. Only the trusted Wings service may access Docker and
the game-server filesystem, as in the existing Wings configuration.

Use these Compose files in this order from `/opt/blockyard`:

```sh
docker compose -f docker-compose.yml -f docker-compose.pterodactyl.yml \
  -f docker-compose.vps.yml --profile frontend --profile pterodactyl config --quiet
```

Add `-f docker-compose.wings.yml --profile wings` only after creating the full
private Wings configuration and its host directories. Never use
`--remove-orphans`: Wings manages sibling game containers independently.

## Secrets and database TLS

The server has its own mode-600 `.env`; do not copy development accounts,
tokens, or databases to the VPS. Use `APP_ENV=production`, invite-only
registration, disabled payments, and an HTTPS frontend origin. Build the
frontend with `NEXT_PUBLIC_API_URL=https://app.playblockyard.com/api`.

Generate a private PostgreSQL CA and server certificate with `DNS:postgres`
in its subject alternative names. Store them in `.local/postgres-tls` on the
VPS. The server key must be mode 600 and owned by the image's PostgreSQL UID;
the CA and server certificates must be readable by the application containers.
Use `sslmode=verify-full&sslrootcert=/etc/hosting/ca.crt` in `DATABASE_URL`.
The CA certificate is mounted read-only into API, worker and migration
containers. Keep the CA signing key private and renew the server certificate
before its expiry. The mounted `pg_hba.conf` rejects plaintext TCP connections
and requires SCRAM authentication over TLS. Local Unix-socket access stays
available inside the trusted database container for initialization and operations.

The VPS application uses the `hosting_app` database owner role, which has no
superuser, role creation, database creation, replication or RLS bypass privileges.
The PostgreSQL bootstrap role is retained only for infrastructure operations.

## Connect to PostgreSQL from your Mac

For an interactive SQL prompt without installing a database client:

```sh
ssh -i ~/.ssh/minecraft_vps -t root@169.58.54.42 \
  'docker exec -it blockyard-postgres-1 psql -U blockyard_readonly -d hosting'
```

For DataGrip, DBeaver, TablePlus or a local `psql`, run this from the repository:

```sh
bash scripts/vps-db-tunnel.sh
```

Leave that terminal open. Connect to `localhost:55433`, database `hosting`, user
`blockyard_readonly`. The password is stored privately in
`.local/vps-database-access.json`. Enable SSL with full certificate/hostname
verification and select `.local/vps-postgres-ca.crt` as the trusted CA. The server
certificate includes `postgres`, `localhost`, and `127.0.0.1` for internal and
tunneled connections. Stop the tunnel with Ctrl+C; restart it if the database
container is recreated, since its private IP can change. The script discovers
the container address without publishing PostgreSQL on the host.

The read-only role can view hosting tables, including sensitive account records;
keep its credentials private. Writes are denied. Use the privileged operator CLI
for supported administration rather than manually editing server/job/resource
records. Local Unix-socket access is trusted inside the database container, so
SSH/Docker access remains infrastructure-level authority.

Pterodactyl stores its own data in MariaDB, not this PostgreSQL database.

## Email is a launch prerequisite

Create a transactional email provider account, authenticate the sending domain
with its exact DNS records, and configure implicit-TLS SMTP, usually port 465:

```dotenv
SMTP_ADDRESS=smtp-relay.brevo.com:465
SMTP_USERNAME=provider-issued-smtp-login
SMTP_PASSWORD=provider-issued-smtp-key
SMTP_FROM=accounts@playblockyard.com
```

Enter credentials privately on the VPS, never in chat or tracked files. Provider
account approval and domain authentication must succeed before testing delivery.
SMTP sends mail; it does not create a receiving mailbox for the sender address.

The API deliberately refuses production startup without SMTP configuration.
Do not switch to development mode or configure fictional credentials to bypass
this requirement. After real delivery works, bootstrap the first administrator
using [PRIVATE_BETA.md](PRIVATE_BETA.md).

## Remaining launch checks

Initialize Panel migrations and an administrator, issue separate Application,
Client and installation-receipt tokens, configure Wings, and register capacity
with OS/control-plane headroom. Keep the existing reviewed Panel revision pinned.
Before inviting users, verify HTTPS cookie persistence, verification/recovery
emails, provisioning, console, uploads/backups and an external Minecraft join.
Configure off-host backups and test restoration. See
[LAUNCH_READINESS.md](LAUNCH_READINESS.md) for the complete checklist.

Use a separate infrastructure email for the Panel administrator, such as
`panel-operator@playblockyard.com`. Customer provisioning creates a distinct
Panel user with the customer's email and application UUID. Reusing a customer's
email for the Panel administrator makes that creation fail; do not adopt a Panel
administrator as a customer server owner.

The checked-in Paper and Vanilla eggs fail if the chosen version is unavailable
instead of substituting another version. They use the isolated Java installer
image, verify the downloaded JAR, and leave EULA acceptance to the server owner.
Vanilla also checks Mojang's published SHA-1. The existing software import script
imports all reviewed eggs and writes configurations with actual destination egg
IDs; never use example egg IDs directly. Build the installer image on each Wings
host before creating servers.

SSH password authentication is disabled after verifying key-based access.
Retain the matching private key and provider-console access. UFW allows SSH and
HTTP/HTTPS; game and SFTP ports are opened when their services are configured.
Docker-published game ports also require review of Docker's forwarding rules;
UFW alone does not reliably restrict published container ports.

## Validated on the VPS

On 2026-10-12: dashboard/API HTTPS, real account verification email, administrator
login, Panel/Wings connectivity, capacity registration, async Paper 1.21.1
provisioning, file listing/writing through Panel, and TLS console WebSocket
authentication passed. The database tunnel passed full certificate verification
and a read-only write-denial check. Vanilla 1.21.1 downloaded and passed Mojang's
checksum/JAR checks in an isolated installer. Unavailable Paper/Vanilla releases
were rejected without substituting a different version.

The complimentary `Blockyard Beta` server was started after the owner accepted
Minecraft's EULA. Its Paper 1.21.1 status ping from a Mac, console WebSocket,
`list` command and resource metrics passed. A real player login, off-host backup
restore and the remaining launch-readiness checks are still required.

The dashboard now keeps a requested shutdown visible while Minecraft saves its
world, including after a browser refresh. Repeat Stop requests are blocked for a
short interval; Force stop remains available if graceful shutdown gets stuck.

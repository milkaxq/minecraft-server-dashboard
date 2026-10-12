# Launch readiness

Reviewed on 2026-10-09. This repository runs a tested local development stack;
that is different from a public production deployment.

## First launch: invited beta

Start with a small invited beta before collecting payments. Use a real Linux
server with Docker support, public networking and enough spare resources for the
Panel, databases, API, installer containers and operating system. Purchased game
allocations are only part of the machine's memory/disk budget. Keep game servers
in isolated Wings containers; only Wings receives Docker/host mounts.

Pterodactyl's [official documentation](https://docs.pterodactyl.io/) currently
labels 2.0 as pre-release and not ready for production. This project pins a
reviewed 2.0 development revision. Keep it for controlled testing; paid public
hosting needs either a production-ready v2 release, or a separately validated
stable-version deployment. Do not silently upgrade the development branch.

Before inviting external players:

1. **Hosting and addresses:** choose a region close to the first players, set up
   Linux/Docker, and configure real DNS for the dashboard, API, Panel and Wings.
   Replace localhost aliases with publicly reachable game allocations. Test the
   Minecraft connection from another network, not just from the server itself.
2. **HTTPS and networking:** terminate TLS for the dashboard/API and configure
   HTTPS/WSS for Panel/Wings. Keep PostgreSQL, MariaDB, Redis, metrics and the
   Docker socket private. Expose only the required HTTPS/game ports; keep admin
   access restricted. Configure trusted proxies and edge request limits because
   the API intentionally ignores forwarded IP headers.
3. **Secrets and email:** install server-only credentials and retain
   RECOVERY_TOKEN_KEY across instances/restarts. Set a trusted FRONTEND_ORIGIN
   and rebuild the frontend with its real NEXT_PUBLIC_API_URL. Configure a TLS
   SMTP sender and its domain verification records. Verify registration,
   password recovery and deliverability using real inboxes. Private local mail
   files do not constitute production email delivery.
4. **Production database:** the API requires APP_ENV=production, a PostgreSQL
   connection with sslmode=verify-full, valid SMTP settings and HTTPS origins.
   The bundled development Compose stack does not automatically provide a TLS
   PostgreSQL deployment. Set this up before attempting production mode.
5. **Capacity and templates:** register the real node/allocations, reserve
   operating-system and installation headroom, and publish only validated
   software/version combinations. Test a paid-sized server under realistic
   player/modpack load. Database reservation locking does not reserve memory
   for Spigot compilation or the rest of the machine.
6. **Recovery:** schedule encrypted, off-host backups of the hosting database,
   Panel database and Panel keys, and worlds/server backups. Restore a database
   and world into a separate environment and verify they actually work.
   Panel's [Docker documentation](https://docs.pterodactyl.io/v2/panel/docker)
   requires its database and persistent key file to be backed up together;
   game files live on Wings nodes.
7. **Operations:** add uptime, failed-job, disk-space and backup-age alerts, a
   support contact and a maintenance procedure. Restart the host and prove the
   worker resumes safely without duplicating servers or resource reservations.
8. **Access:** restrict the beta to invited users. Use REGISTRATION_MODE=invite_only and the administrator
   invitation flow described in [PRIVATE_BETA.md](PRIVATE_BETA.md). Do not advertise the demo payment flow as actual payment.

Keep billing disabled for a non-commercial beta. Operator-created servers are
already supported. The development mock checkout remains a local demonstration,
and production startup explicitly rejects it.

## Before accepting money

A real payment adapter is not implemented. The existing provider interface is
ready for it, but selecting a gateway requires the business's operating country,
currency and account eligibility. Never assume a provider works in every country.

Implement and verify hosted checkout, signed webhooks, duplicate/delayed events,
refunds, payment-versus-provisioning failures, renewal and subscription-expiry
behavior. Keep the persisted amount/resource snapshots authoritative. Decide
whether renewals are manual or automatically charged; current billing is manual.

Publish pricing, service/support expectations, privacy/terms and a clear refund
policy appropriate to the business and deployment. Verify a complete purchase
from an external network with real provider test-mode events before enabling
live charges. Keep an explicit capacity cap for the first launch.

## Next customer-experience priorities

- Email verification links and private invitations are implemented; test real
  SMTP delivery before inviting external users.
- The first-server checklist is implemented. Test setup, EULA acceptance,
  startup and joining with novice players and the correct Minecraft client.
- Understandable queued/installing/failed states with a support reference; avoid
  fake progress percentages when the installer cannot report them accurately.
- Browser login persistence is implemented with HttpOnly cookies, fixed expiry
  and CSRF checks. Verify it on your real HTTPS dashboard/API addresses; see
  [SESSIONS.md](SESSIONS.md).
- Backup reminders and visible backup results before world/mod changes.
- Real inbox delivery, real external join tests and feedback from a handful of
  novice players before adding more infrastructure features. The private Feedback
  page and admin review inbox are implemented; use [BETA_TESTING.md](BETA_TESTING.md)
  to run a small, repeatable test session.

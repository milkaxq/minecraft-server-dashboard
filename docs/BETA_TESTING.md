# A small private beta test

Start with a few invited players, including someone who has never used a hosting
dashboard. Use disposable test worlds and ask them to describe anything confusing.
Complete the external hosting, HTTPS, email and recovery checks in
[LAUNCH_READINESS.md](LAUNCH_READINESS.md) before inviting remote players.

## Prepare

- Keep `REGISTRATION_MODE=invite_only` and `PAYMENT_PROVIDER=disabled`.
- Send each tester their own email-bound link from **Admin → Invitations**.
- Grant server access separately in Members. Use an appropriate role and explain
  that only the owner accepts the Minecraft EULA and manages members.
- Check that the tester's server software/version is actually supported by the
  published catalog. Modded clients need the matching loader and required mods.
- Configure real SMTP for external testers. The development mail spool is local
  and cannot deliver to their inboxes.

## Ask each tester to try

1. Register, verify the email, sign in, and find the server. Try on both a laptop
   and a phone. Tell the host if the link, wording, or navigation is confusing.
2. Follow **Your first session** on Overview, start the server, copy its address,
   and join with Minecraft Java using the stated version. Confirm joining from
   another network. A successful address copy alone does not prove connectivity.
3. Stop and restart, view players and usage, and find the console. Confirm that
   their role permits only the expected controls.
4. On a disposable world, create a backup and wait for success before changing
   settings or mods. Test a restore with the required confirmation and verify
   the restored world in-game. Never use a player's only copy for this exercise.
5. Open **Feedback**, choose the related server if relevant, and send a short
   report. Include what they tried, what happened, steps to reproduce, their
   Minecraft version, and a visible support reference if one appeared. Do not
   include passwords, invitation/recovery links, access tokens or private files.
6. Return to **Feedback → Your reports** to read the host's reply. Reports are
   private to the author and hosting admins, not to other server members. Replies
   appear in the dashboard; automatic email notifications are not implemented.

Refreshing restores a valid browser session, up to its fixed 24-hour expiry.
Logout and password changes revoke it; feedback form drafts are not persisted. This inbox is for beta reports,
not an emergency support service; arrange any urgent contact separately.

## Host review

Open **Admin → Feedback** to read reports. Choose **Review report**, reply in
plain text, and set New, Reviewed or Resolved. Replies and status changes are
visible to the reporter. If another admin updated a report first, refresh and
review the new version before saving; stale revisions cannot overwrite it.
Do not paste privileged diagnostics or credentials into a reply.

List pages contain up to 50 reports, ordered newest first. Use Previous/Next to
see older reports. Submissions are limited to 10 requests per 15-minute API
window and 20 new reports in a rolling 24-hour period per account. Retrying an
unchanged form reuses its submission key, so an uncertain response cannot create
a second copy. Changing the form after an uncertain response starts a new report;
check Your reports first. Limits still apply to API retry requests.

The report stores only the submitted text, an optional authorized application
server UUID, author, timestamps, status and host reply. It does not collect
console output, filesystem contents, browser telemetry or Panel identifiers.
Audit entries record report IDs/actions rather than report text. Include these
records in the hosting database's backup and privacy/retention arrangements.

## Implementation and validation

- `migrations/011_beta_feedback.sql`: persisted reports, uniqueness and indexes.
- `internal/feedback`: validation, ownership, quotas, idempotency and admin review.
- `internal/httpapi/feedback.go`: authenticated REST endpoints; admin routes
  require live admin authorization and responses use existing request IDs.
- `frontend/components/feedback.js`: report form, personal history and admin inbox.
- `/feedback`, `GET/POST /feedback`, `GET /admin/feedback`,
  `PUT /admin/feedback/{report}`. POST requires a UUID Idempotency-Key; PUT requires
  the report's current revision. Lists accept the existing offset pagination.

PostgreSQL tests cover ownership, server access, revocation, concurrent retries,
quotas, stale updates, input validation and private audit/log contents. Browser
tests cover retries, replies, mobile layout, untrusted text and pagination. Use
the commands in [CODE_QUALITY.md](CODE_QUALITY.md) with a dedicated test database.

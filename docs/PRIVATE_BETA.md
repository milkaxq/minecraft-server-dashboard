# Private beta

Set `REGISTRATION_MODE=invite_only` and `PAYMENT_PROVIDER=disabled`. Registration
is invite-only by default, including when the variable is omitted. To deliberately
open registration later, set `REGISTRATION_MODE=open` and restart the API.
Existing accounts can still log in, verify email and recover passwords.

## Invite a tester

1. Sign in as an administrator and open **Admin → Invitations**.
2. Enter the tester's email and choose **Create invitation**.
3. Copy the link immediately and share it privately with that person. The system
   does not send an invitation email automatically. Links are shown only once;
   issuing another invitation does not revoke an earlier one, so revoke unwanted
   active links explicitly.
4. The tester opens the link, creates an account with that email and verifies it
   through the normal verification email. The link works once and expires after
   seven days. The register page removes its fragment from browser history.
5. Grant server access separately using the existing Members controls and the
   appropriate role. An invitation creates no server, billing entitlement or
   administrator privileges. Resource allocation and ownership checks still apply.

Invitation lists show Active, Used, Expired or Revoked without exposing tokens.
Revocation takes effect for unused invitations; it does not disable an account
that already registered. Tokens are random, stored only as SHA-256 hashes, and
consumed in the same transaction as account creation. Failed account creation
rolls the invitation consumption back. If verification email delivery fails after
account creation, the account persists and resend-verification recovers it.

## Bootstrap a brand-new database

Keep public registration closed. After applying migrations and starting the API,
use the privileged database operator CLI to issue the first-account invitation:

```sh
mkdir -p .local
(umask 077; docker compose exec -T backend control bootstrap-invite \
  -email owner@example.com > .local/bootstrap-invite.json)
```

Read the token privately from this file, paste it into the invitation-code field
on `/register`, and use the same email. Verify the email normally, then promote
that verified account using the existing operator command:

```sh
docker compose exec backend control admin -email owner@example.com \
  -reason 'Initial private beta operator'
```

Bootstrap is allowed only while there are no accounts and no active bootstrap
invitation. It is not an HTTP endpoint and does not automatically verify an email
or promote a user. If the account already exists, use verification/resend and
`control admin`; do not reopen bootstrap. The private file contains a credential;
delete it after use. Never commit or paste it into logs.

## First-session checklist

When payments are disabled in invite-only mode, the dashboard explains how to
request access from the host rather than presenting checkout as a required step.
The server Overview has **Your first session**: finish setup, owner accepts the
Minecraft agreement, start and wait for Online, then copy the address and join.
Completed steps come from actual server/EULA/runtime state, not a fabricated
installation percentage. Copy completion only records a successful clipboard
operation in the current page; it does not claim the player has joined.
Forge/Fabric/NeoForge guidance explains the matching loader and mod requirements.

## Validation

Use the code-quality commands in CODE_QUALITY.md. PostgreSQL tests cover closed
registration, email binding, expiry, revocation, concurrent consumption, rollback,
bootstrap restrictions, admin authorization and token/log privacy. Browser tests
cover invitation links, configuration failures, admin creation/revocation, beta
onboarding and real-state checklist transitions.

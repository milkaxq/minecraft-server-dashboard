# Phase 5: billing, subscriptions and administration

Phase 5 adds a complete **demo payment** flow, manually renewed subscriptions,
expiry suspension, and an admin dashboard. No money is collected. Production
startup rejects the mock provider; a real payment adapter remains future work.

## Architecture

Customers create an order from an enabled database plan. The Go service snapshots
its price and resources in one PostgreSQL transaction. Checkout uses the
`PaymentProvider` interface (`CreateCheckout`, `VerifyPayment`, `Refund`). The mock
provider produces signed payment events; both its customer confirmation endpoint
and its webhook use the same transactional settlement logic.

Settlement locks the server and order, verifies the persisted checkout reference,
amount and currency, records an idempotent payment event, creates the subscription,
and queues provisioning atomically. Replayed events do not provision twice or
extend the subscription twice. Customers supply no prices, resource limits or Panel IDs.
The existing worker reserves node capacity and provisions through Pterodactyl.

Renewals retain the subscription's original price and server resource allocation.
Calendar periods use UTC and clamp month-end dates. Renewals extend from the later
of now or the existing period end. Billing is manual: there are no recurring card
charges. Cancellation at period end prevents continued entitlement; an explicit
paid renewal can reactivate it.

Both API and worker run a minute-based subscription reconciler. PostgreSQL locking
makes concurrent replicas safe. Expired subscriptions immediately lose dashboard
control access and queue a durable suspension job. The worker must run for actual
Panel suspension. Paid renewals resume billing suspensions, including renewals
received while suspension is queued. Admin suspensions remain in place until an
admin lifts them. Failed automatic lifecycle jobs require operator attention;
they are not endlessly recreated.

## File changes

| Location | Responsibility |
|---|---|
| `migrations/006_billing.sql` | Order snapshots, checkout references, payment events, subscription terms and suspension reasons |
| `internal/billing/` | Provider boundary, mock signatures, orders, renewals, settlement, expiry and tests |
| `internal/admin/` | Plan editing, bounded administrative reads and audited changes |
| `internal/httpapi/billing.go`, `admin.go` | Strict REST inputs, authentication, quotas and error mapping |
| `internal/server/service.go` | Transactional server creation shared by order creation |
| `internal/jobs/service.go`, `internal/control/service.go` | Entitlement checks, termination and admin hold handling |
| `cmd/api/`, `cmd/worker/` | Billing wiring and reconciler startup |
| `frontend/components/billing.js`, `admin.js` | Checkout, subscriptions, orders and administration |
| `frontend/app/checkout/[id]/`, `frontend/app/admin/` | New dashboard routes |
| `frontend/components/new-server.js`, `data.js` | Order creation and request/session-bound resource results |
| `.env.example`, `docker-compose.yml` | Server-only payment configuration |

## Run locally

1. Copy `.env.example` to `.env` and configure the database passwords and Panel
   integration described in [Phase 2](PHASE2.md). Configure Client API and transfer
   settings as described in [Phase 4](PHASE4.md).
2. Set `PAYMENT_PROVIDER=mock` in development and generate a private signing secret:

   ```sh
   openssl rand -hex 32
   ```

   Put the result in `MOCK_PAYMENT_SECRET` in your uncommitted `.env`. The default
   `PAYMENT_PROVIDER=disabled` prevents customer checkout. Never put this secret
   into frontend environment variables.
3. Start the application:

   ```sh
   docker compose --profile frontend up --build -d
   ```

   Open `http://localhost:3000`, register, verify using development mail, and log in.
   Approved software versions and nodes must already be configured. Choose a plan,
   name the server, confirm the clearly labeled demo payment, and watch provisioning.
   The server page then provides its existing Minecraft controls.

Billing shows order history, subscription dates, cancellation and manual renewal.
Enable an operator using the existing `cmd/control` admin command in the
[Phase 2 runbook](PHASE2.md). Its verified account receives the Admin navigation
link. The dashboard manages plans, views users/orders/servers/node capacity/audits,
and submits confirmed, reason-bearing retry/suspend/unsuspend/terminate actions.
Disabling or repricing a plan does not change existing subscription terms.

## REST additions

All customer routes require a verified bearer session. Order creation and renewals
require a UUID `Idempotency-Key` header. Public webhook signatures are checked before
settlement. Administrative routes check the current database admin flag.

| Method | Route |
|---|---|
| GET | `/billing/config` |
| GET / POST | `/orders` |
| GET | `/orders/{order}` |
| POST | `/orders/{order}/checkout`, `/mock-pay`, `/cancel` |
| GET | `/subscriptions` |
| POST | `/servers/{server}/renewals` |
| PUT | `/servers/{server}/subscription` |
| POST | `/payments/webhooks/mock` |
| GET / POST | `/admin/plans` |
| PUT | `/admin/plans/{plan}` |
| GET | `/admin/users`, `/servers`, `/orders`, `/nodes`, `/audit` |

The grouped admin GET paths above all have the `/admin` prefix. Subscription PUT
requires `{"cancel_at_period_end":true}` or false. Checkout, mock confirmation,
order cancellation and renewal accept `{}`. Lifecycle routes remain those documented
in Phase 2. Webhooks accept bounded JSON and `X-Payment-Signature`, an HMAC-SHA256
hex signature. Mock events have a short timestamp validity window.

Billing and admin routes have persistent shared quotas. Ownership checks return
404 for another customer's order or server. Admin responses exclude password
hashes, session tokens, checkout references and internal Panel credentials.
Plan writes and lifecycle changes create audit records. The frontend never sets
an admin flag or trusts a navigation link as authorization.

## Tests and limits

```sh
# Use a dedicated PostgreSQL database; each integration test owns its schema.
TEST_DATABASE_URL='postgres://postgres:YOUR_TEST_PASSWORD@localhost:55432/hosting_test?sslmode=disable' make integration
make check
cd frontend
npm run build
npm test
```

Tests cover concurrent order/payment replay, signed-event tampering and event-ID
reuse, ownership, cancellation, frozen renewal prices/resources, expiry access
revocation, suspension/renewal races, admin holds, termination, expiry batch
fairness, live admin revocation, plan editing and HTTP authorization. The browser
suite has 17 scenarios covering existing management plus checkout, subscriptions,
admin changes and disabled payments.

Legacy operator-granted servers without subscriptions retain their previous
manual entitlement behavior. Failed paid provisioning is visible for admin retry.
There is no automatic refund endpoint or real financial settlement: a future
provider must implement its own checkout/webhook adapter and late-payment/refund
policy. The interface's mock refund method only models that boundary.

Live Panel/Wings deployment remains an external smoke test. Worlds, delegated SFTP,
startup/environment editing and member-management UI remain outside the implemented
phases; this phase does not claim those original roadmap features are complete.

ALTER TABLE orders DROP CONSTRAINT orders_server_id_key;
ALTER TABLE orders ADD COLUMN kind text NOT NULL DEFAULT 'NEW' CHECK(kind IN ('NEW','RENEWAL'));
ALTER TABLE orders ADD COLUMN idempotency_key uuid;
ALTER TABLE orders ADD COLUMN request_fingerprint text;
ALTER TABLE orders ADD COLUMN plan_name text NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN checkout_reference text UNIQUE;
ALTER TABLE orders ADD COLUMN checkout_url text;
ALTER TABLE orders ADD COLUMN provider text;
ALTER TABLE orders ADD COLUMN paid_at timestamptz;
CREATE UNIQUE INDEX orders_request_key ON orders(user_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX orders_pending_server ON orders(server_id) WHERE status='PENDING';
CREATE INDEX orders_user_history ON orders(user_id,created_at DESC,id DESC);
ALTER TABLE subscriptions ADD COLUMN price_cents bigint CHECK(price_cents>=0);
ALTER TABLE subscriptions ADD COLUMN currency text CHECK(currency ~ '^[A-Z]{3}$');
ALTER TABLE subscriptions ADD COLUMN billing_period text CHECK(billing_period IN ('MONTHLY','YEARLY'));
ALTER TABLE subscriptions ADD COLUMN cancel_at_period_end boolean NOT NULL DEFAULT false;
ALTER TABLE servers ADD COLUMN suspension_reason text CHECK(suspension_reason IN ('ADMIN','BILLING'));
CREATE INDEX subscriptions_expiry ON subscriptions(current_period_end) WHERE status='ACTIVE';
CREATE TABLE payment_events (
 provider text NOT NULL,
 event_id text NOT NULL,
 order_id uuid NOT NULL REFERENCES orders(id),
 fingerprint text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(provider,event_id)
);

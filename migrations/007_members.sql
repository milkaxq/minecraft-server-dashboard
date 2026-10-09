CREATE TABLE member_operations (
 server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 actor_id uuid NOT NULL REFERENCES users(id),
 idempotency_key uuid NOT NULL,
 fingerprint text NOT NULL,
 result jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(server_id,actor_id,idempotency_key)
);

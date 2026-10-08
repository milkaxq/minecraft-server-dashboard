CREATE TABLE management_operations (
 id uuid PRIMARY KEY,
 server_id uuid NOT NULL REFERENCES servers(id),
 user_id uuid NOT NULL REFERENCES users(id),
 idempotency_key uuid NOT NULL,
 action text NOT NULL,
 fingerprint text NOT NULL,
 status text NOT NULL CHECK(status IN ('PENDING','SUCCEEDED','UNKNOWN','REJECTED')),
 result jsonb,
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(server_id,user_id,idempotency_key)
);
CREATE INDEX management_operations_unresolved ON management_operations(server_id,action) WHERE status IN ('PENDING','UNKNOWN');

CREATE TABLE control_requests (
 id uuid PRIMARY KEY,
 server_id uuid NOT NULL REFERENCES servers(id),
 user_id uuid NOT NULL REFERENCES users(id),
 idempotency_key uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('start','stop','restart','kill','command')),
 fingerprint text NOT NULL,
 status text NOT NULL CHECK(status IN ('PENDING','SUCCEEDED','UNKNOWN','REJECTED')),
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(server_id,user_id,idempotency_key)
);
CREATE INDEX control_requests_created_idx ON control_requests(created_at);
CREATE TABLE console_tickets (
 token_hash bytea PRIMARY KEY,
 server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 session_hash bytea NOT NULL REFERENCES sessions(token_hash) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX console_tickets_expiry_idx ON console_tickets(expires_at);

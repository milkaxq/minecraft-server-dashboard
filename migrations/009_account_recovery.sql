CREATE TABLE password_resets (
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX password_resets_user_idx ON password_resets(user_id);
CREATE INDEX password_resets_expiry_idx ON password_resets(expires_at);
-- Only the delivery worker can decrypt these tokens using an environment secret.
CREATE TABLE recovery_mail (
 id uuid PRIMARY KEY,
 token_hash bytea NOT NULL UNIQUE REFERENCES password_resets(token_hash) ON DELETE CASCADE,
 encrypted_token bytea NOT NULL CHECK(octet_length(encrypted_token)<=128),
 request_id text NOT NULL,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 available_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recovery_mail_available_idx ON recovery_mail(available_at);

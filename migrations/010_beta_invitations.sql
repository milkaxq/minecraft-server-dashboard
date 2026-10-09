CREATE TABLE beta_invitations (
 id uuid PRIMARY KEY,
 email text NOT NULL CHECK(email=lower(email)),
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 -- NULL creator is reserved for the first-account operator CLI bootstrap.
 created_by uuid REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 revoked_at timestamptz
);
CREATE INDEX beta_invitations_created_idx ON beta_invitations(created_at DESC,id);

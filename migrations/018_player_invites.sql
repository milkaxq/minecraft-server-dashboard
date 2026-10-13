CREATE TABLE player_invites (
 server_id uuid PRIMARY KEY REFERENCES servers(id),
 slug text NOT NULL UNIQUE CHECK(length(slug)=43),
 created_by uuid NOT NULL REFERENCES users(id),
 snapshot jsonb NOT NULL,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE users ADD COLUMN pterodactyl_user_id integer UNIQUE CHECK(pterodactyl_user_id > 0);
ALTER TABLE users ADD COLUMN panel_create_attempted boolean NOT NULL DEFAULT false;
ALTER TABLE servers ADD COLUMN panel_create_attempted boolean NOT NULL DEFAULT false;
ALTER TABLE servers ADD COLUMN provisioning_request jsonb;

-- Operators register dedicated nodes and unassigned Panel allocations. Allocation
-- IDs are privileged configuration, never supplied by a customer request.
CREATE TABLE node_allocations (
 id uuid PRIMARY KEY,
 node_id uuid NOT NULL REFERENCES nodes(id),
 pterodactyl_allocation_id integer NOT NULL UNIQUE CHECK(pterodactyl_allocation_id > 0),
 hostname text NOT NULL CHECK(length(hostname) BETWEEN 1 AND 253),
 port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
 enabled boolean NOT NULL DEFAULT true,
 reserved_server_id uuid UNIQUE REFERENCES servers(id)
);
CREATE INDEX node_allocations_available_idx ON node_allocations(node_id) WHERE enabled AND reserved_server_id IS NULL;

CREATE TABLE software_templates (
 server_type text PRIMARY KEY CHECK(server_type IN ('VANILLA','PAPER')),
 egg_id integer NOT NULL CHECK(egg_id > 0),
 docker_image text NOT NULL CHECK(length(docker_image)>0),
 startup text NOT NULL CHECK(length(startup)>0),
 version_variable text NOT NULL CHECK(version_variable ~ '^[A-Z][A-Z0-9_]*$'),
 environment jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(environment)='object'),
 enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE minecraft_versions (
 server_type text NOT NULL REFERENCES software_templates(server_type),
 version text NOT NULL CHECK(version ~ '^[0-9]+[.][0-9]+([.][0-9]+)?$'),
 enabled boolean NOT NULL DEFAULT true,
 PRIMARY KEY(server_type,version)
);

CREATE TABLE jobs (
 id uuid PRIMARY KEY,
 server_id uuid NOT NULL REFERENCES servers(id),
 actor_id uuid NOT NULL REFERENCES users(id),
 action text NOT NULL CHECK(action IN ('PROVISION','SUSPEND','UNSUSPEND','TERMINATE')),
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','RUNNING','SUCCEEDED','FAILED')),
 idempotency_key uuid NOT NULL,
 request_fingerprint text NOT NULL,
 request_id text NOT NULL,
 retry_install boolean NOT NULL DEFAULT false,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 max_attempts integer NOT NULL DEFAULT 60 CHECK(max_attempts>0),
 available_at timestamptz NOT NULL DEFAULT now(),
 last_error text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(server_id,idempotency_key)
);
CREATE UNIQUE INDEX jobs_one_active_server_idx ON jobs(server_id) WHERE status IN ('PENDING','RUNNING');
CREATE INDEX jobs_available_idx ON jobs(available_at,created_at) WHERE status IN ('PENDING','RUNNING');

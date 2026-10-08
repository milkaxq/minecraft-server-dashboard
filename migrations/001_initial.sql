CREATE TABLE users (
 id uuid PRIMARY KEY, email text NOT NULL UNIQUE CHECK(email = lower(email)),
 password_hash text NOT NULL, name text NOT NULL CHECK(length(name) BETWEEN 1 AND 80),
 verified_at timestamptz, is_admin boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash bytea PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX sessions_expiry_idx ON sessions(expires_at);
CREATE TABLE email_verifications (
 token_hash bytea PRIMARY KEY, user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL
);
CREATE TABLE plans (
 id uuid PRIMARY KEY, name text NOT NULL UNIQUE, description text NOT NULL,
 price_cents bigint NOT NULL CHECK(price_cents >= 0), currency text NOT NULL DEFAULT 'USD' CHECK(currency ~ '^[A-Z]{3}$'),
 billing_period text NOT NULL CHECK(billing_period IN ('MONTHLY','YEARLY')),
 ram_mb integer NOT NULL CHECK(ram_mb > 0), disk_mb integer NOT NULL CHECK(disk_mb > 0),
 cpu_limit integer NOT NULL CHECK(cpu_limit > 0), backup_limit integer NOT NULL CHECK(backup_limit >= 0),
 database_limit integer NOT NULL CHECK(database_limit >= 0), player_recommendation integer NOT NULL CHECK(player_recommendation > 0),
 enabled boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE nodes (
 id uuid PRIMARY KEY, pterodactyl_node_id integer NOT NULL UNIQUE CHECK(pterodactyl_node_id > 0), name text NOT NULL,
 cpu_capacity integer NOT NULL CHECK(cpu_capacity >= 0), ram_capacity_mb integer NOT NULL CHECK(ram_capacity_mb >= 0),
 disk_capacity_mb integer NOT NULL CHECK(disk_capacity_mb >= 0), allocated_cpu integer NOT NULL DEFAULT 0 CHECK(allocated_cpu >= 0 AND allocated_cpu <= cpu_capacity),
 allocated_ram_mb integer NOT NULL DEFAULT 0 CHECK(allocated_ram_mb >= 0 AND allocated_ram_mb <= ram_capacity_mb),
 allocated_disk_mb integer NOT NULL DEFAULT 0 CHECK(allocated_disk_mb >= 0 AND allocated_disk_mb <= disk_capacity_mb), enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE servers (
 id uuid PRIMARY KEY, owner_id uuid NOT NULL REFERENCES users(id), plan_id uuid NOT NULL REFERENCES plans(id), node_id uuid REFERENCES nodes(id),
 pterodactyl_server_id integer UNIQUE CHECK(pterodactyl_server_id > 0), pterodactyl_identifier text UNIQUE,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 80),
 status text NOT NULL DEFAULT 'PENDING_PAYMENT' CHECK(status IN ('PENDING_PAYMENT','PAYMENT_CONFIRMED','PROVISIONING','RUNNING','SUSPENDED','FAILED','CANCELLED')),
 minecraft_version text NOT NULL CHECK(minecraft_version ~ '^[0-9]+[.][0-9]+([.][0-9]+)?$'), server_type text NOT NULL CHECK(server_type IN ('VANILLA','PAPER')),
 hostname text, port integer CHECK(port BETWEEN 1 AND 65535),
 ram_mb integer NOT NULL CHECK(ram_mb > 0), disk_mb integer NOT NULL CHECK(disk_mb > 0), cpu_limit integer NOT NULL CHECK(cpu_limit > 0),
 backup_limit integer NOT NULL CHECK(backup_limit >= 0), database_limit integer NOT NULL CHECK(database_limit >= 0),
 internal_error text, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(status NOT IN ('RUNNING','SUSPENDED') OR (pterodactyl_server_id IS NOT NULL AND node_id IS NOT NULL))
);
CREATE INDEX servers_owner_idx ON servers(owner_id);
CREATE TABLE server_members (
 server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 role text NOT NULL CHECK(role IN ('ADMIN','MODERATOR')), created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(server_id,user_id)
);
CREATE INDEX server_members_user_idx ON server_members(user_id);
CREATE TABLE orders (
 id uuid PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id), server_id uuid NOT NULL UNIQUE REFERENCES servers(id), plan_id uuid NOT NULL REFERENCES plans(id),
 price_cents bigint NOT NULL CHECK(price_cents >= 0), currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 billing_period text NOT NULL CHECK(billing_period IN ('MONTHLY','YEARLY')),
 status text NOT NULL CHECK(status IN ('PENDING','PAID','REFUNDED','CANCELLED')), payment_reference text UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE subscriptions (
 id uuid PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id), server_id uuid NOT NULL UNIQUE REFERENCES servers(id), plan_id uuid NOT NULL REFERENCES plans(id),
 status text NOT NULL CHECK(status IN ('ACTIVE','PAST_DUE','CANCELLED')), current_period_end timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE audit_logs (
 id uuid PRIMARY KEY, actor_id uuid REFERENCES users(id), server_id uuid REFERENCES servers(id), action text NOT NULL,
 request_id text NOT NULL, metadata jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_server_idx ON audit_logs(server_id,created_at);
CREATE TABLE auth_rate_limits (bucket text PRIMARY KEY, window_start timestamptz NOT NULL, attempts integer NOT NULL CHECK(attempts > 0));

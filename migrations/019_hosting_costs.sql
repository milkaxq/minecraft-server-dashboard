CREATE TABLE hosting_cost_profiles (
 node_id uuid PRIMARY KEY REFERENCES nodes(id),
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 node_monthly_cents bigint NOT NULL CHECK(node_monthly_cents BETWEEN 0 AND 100000000),
 overhead_monthly_cents bigint NOT NULL CHECK(overhead_monthly_cents BETWEEN 0 AND 100000000),
 per_server_monthly_cents bigint NOT NULL CHECK(per_server_monthly_cents BETWEEN 0 AND 100000000),
 fee_bps integer NOT NULL CHECK(fee_bps BETWEEN 0 AND 5000),
 fee_fixed_cents bigint NOT NULL CHECK(fee_fixed_cents BETWEEN 0 AND 1000000),
 tax_bps integer NOT NULL CHECK(tax_bps BETWEEN 0 AND 5000),
 target_margin_bps integer NOT NULL CHECK(target_margin_bps BETWEEN 0 AND 9500),
 occupancy_bps integer NOT NULL CHECK(occupancy_bps BETWEEN 1 AND 10000),
 updated_at timestamptz NOT NULL DEFAULT now()
);

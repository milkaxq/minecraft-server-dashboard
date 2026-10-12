-- The admin beta flow uses one stable key to avoid duplicate servers on retries.
ALTER TABLE servers ADD COLUMN creation_key uuid;
ALTER TABLE servers ADD COLUMN creation_fingerprint text;
CREATE UNIQUE INDEX servers_owner_creation_key_idx ON servers(owner_id,creation_key) WHERE creation_key IS NOT NULL;

-- Keep the latest power command cheap to read while the dashboard polls usage.
CREATE INDEX control_requests_latest_power_idx ON control_requests(server_id,created_at DESC,id DESC)
 WHERE action IN ('start','stop','restart','kill');

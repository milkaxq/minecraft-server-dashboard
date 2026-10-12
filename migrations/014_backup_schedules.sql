CREATE TABLE backup_schedules (
 server_id uuid PRIMARY KEY REFERENCES servers(id),
 enabled boolean NOT NULL DEFAULT false,
 hour integer NOT NULL CHECK(hour BETWEEN 0 AND 23),
 minute integer NOT NULL CHECK(minute BETWEEN 0 AND 59),
 timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 100),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 next_run timestamptz, last_success timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(enabled=(next_run IS NOT NULL))
);
CREATE INDEX backup_schedules_due ON backup_schedules(next_run) WHERE enabled;
CREATE TABLE backup_runs (
 id uuid PRIMARY KEY,server_id uuid NOT NULL REFERENCES backup_schedules(server_id),
 schedule_revision bigint NOT NULL,scheduled_at timestamptz NOT NULL,
 owner_id uuid NOT NULL REFERENCES users(id),panel_identifier text NOT NULL,
 status text NOT NULL CHECK(status IN ('QUEUED','CREATING','COMPLETED','FAILED','SKIPPED','NEEDS_INSPECTION')),
 backup_uuid uuid,lease_token uuid,lease_expires_at timestamptz,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),retry_at timestamptz,
 error_code text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz
);
CREATE UNIQUE INDEX backup_runs_one_instant ON backup_runs(server_id,scheduled_at);
CREATE INDEX backup_runs_claimable ON backup_runs(status,lease_expires_at);
CREATE UNIQUE INDEX backup_runs_remote_uuid ON backup_runs(panel_identifier,backup_uuid) WHERE backup_uuid IS NOT NULL;
CREATE TABLE backup_intents (
 id uuid PRIMARY KEY,run_id uuid NOT NULL REFERENCES backup_runs(id),
 action text NOT NULL CHECK(action IN ('CREATE','DELETE')),target_uuid uuid,
 status text NOT NULL CHECK(status IN ('INTENDED','CONFIRMED','UNKNOWN')),
 result_uuid uuid,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((action='CREATE' AND target_uuid IS NULL) OR (action='DELETE' AND target_uuid IS NOT NULL))
);
CREATE INDEX backup_intents_unresolved ON backup_intents(run_id) WHERE status<>'CONFIRMED';

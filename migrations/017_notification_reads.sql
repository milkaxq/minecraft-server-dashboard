CREATE TABLE notification_reads (
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 audit_id uuid NOT NULL REFERENCES audit_logs(id) ON DELETE CASCADE,
 read_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,audit_id)
);

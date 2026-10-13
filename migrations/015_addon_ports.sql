ALTER TABLE node_allocations ADD COLUMN voice_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE node_allocations ADD CONSTRAINT node_allocations_voice_unprivileged CHECK(NOT voice_enabled OR port>=1024);
-- Extra ports share the node allocation pool, but never replace a game's primary port.
CREATE TABLE server_addon_ports (
 server_id uuid PRIMARY KEY REFERENCES servers(id),
 allocation_id uuid NOT NULL UNIQUE REFERENCES node_allocations(id),
 addon text NOT NULL CHECK(addon='simple-voice-chat'),
 status text NOT NULL DEFAULT 'RESERVED' CHECK(status IN ('RESERVED','READY','FAILED')),
 assignment_attempted boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE jobs DROP CONSTRAINT jobs_action_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_action_check CHECK(action IN ('PROVISION','SUSPEND','UNSUSPEND','TERMINATE','VOICE_PORT'));

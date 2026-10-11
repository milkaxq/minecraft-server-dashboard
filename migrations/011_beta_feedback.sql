CREATE TABLE beta_feedback (
 id uuid PRIMARY KEY,
 author_id uuid NOT NULL REFERENCES users(id),
 server_id uuid REFERENCES servers(id) ON DELETE SET NULL,
 submission_key uuid NOT NULL,
 category text NOT NULL CHECK(category IN ('BUG','QUESTION','IDEA')),
 title text NOT NULL CHECK(char_length(title) BETWEEN 3 AND 120),
 message text NOT NULL CHECK(char_length(message) BETWEEN 10 AND 4000),
 status text NOT NULL DEFAULT 'NEW' CHECK(status IN ('NEW','REVIEWED','RESOLVED')),
 response text NOT NULL DEFAULT '' CHECK(char_length(response)<=2000),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(author_id,submission_key)
);
CREATE INDEX beta_feedback_author_idx ON beta_feedback(author_id,created_at DESC,id);
CREATE INDEX beta_feedback_created_idx ON beta_feedback(created_at DESC,id);

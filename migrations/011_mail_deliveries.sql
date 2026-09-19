-- Every attempt to send a notification mail, whether or not the relay took
-- it. An administrator answering "the mail never came" needs the successes
-- as much as the failures. The body is deliberately absent: the subject and
-- the recipient say what left the building, and a table that also held the
-- text would be a second copy of every notification.
CREATE TABLE IF NOT EXISTS mail_deliveries (
  id uuid PRIMARY KEY,
  event text NOT NULL,
  recipient text NOT NULL,
  subject text NOT NULL,
  resource_type text NOT NULL DEFAULT '',
  resource_id text NOT NULL DEFAULT '',
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','failed')),
  attempts integer NOT NULL DEFAULT 0,
  error_message text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mail_deliveries_created_idx ON mail_deliveries(created_at DESC);

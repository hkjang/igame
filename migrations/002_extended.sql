ALTER TABLE users ADD COLUMN IF NOT EXISTS team text NOT NULL DEFAULT '';

ALTER TABLE scores ADD COLUMN IF NOT EXISTS moderation_status text NOT NULL DEFAULT 'valid';
DO $$ BEGIN
  ALTER TABLE scores ADD CONSTRAINT scores_moderation_status_check
    CHECK (moderation_status IN ('valid','flagged','excluded'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

WITH stale AS (
  SELECT id,row_number() OVER(PARTITION BY user_id,game_id ORDER BY started_at DESC) AS rn
  FROM game_sessions WHERE status='active'
)
UPDATE game_sessions gs SET status='abandoned',ended_at=now(),duration_ms=GREATEST(0,extract(epoch FROM(now()-gs.started_at))*1000)::bigint
FROM stale WHERE gs.id=stale.id AND stale.rn>1;
CREATE UNIQUE INDEX IF NOT EXISTS game_sessions_one_active_idx ON game_sessions(user_id,game_id) WHERE status='active';

CREATE TABLE IF NOT EXISTS tournaments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  game_id uuid REFERENCES games(id) ON DELETE SET NULL,
  format text NOT NULL DEFAULT 'score_attack'
    CHECK (format IN ('score_attack','time_attack','survival','bracket','team_battle')),
  max_participants integer NOT NULL DEFAULT 128 CHECK (max_participants > 0),
  starts_at timestamptz NOT NULL DEFAULT now(),
  ends_at timestamptz,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','closed','cancelled')),
  rules jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at IS NULL OR ends_at > starts_at)
);

CREATE TABLE IF NOT EXISTS rewards (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  reward_type text NOT NULL DEFAULT 'badge' CHECK (reward_type IN ('badge','title','avatar_frame')),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title text NOT NULL,
  content text NOT NULL,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
  pinned boolean NOT NULL DEFAULT false,
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notices_public_idx ON notices(pinned DESC, published_at DESC) WHERE status='published';

CREATE TABLE IF NOT EXISTS banners (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title text NOT NULL,
  image_url text NOT NULL,
  link_url text NOT NULL DEFAULT '',
  starts_at timestamptz NOT NULL DEFAULT now(),
  ends_at timestamptz,
  sort_order integer NOT NULL DEFAULT 0,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at IS NULL OR ends_at > starts_at)
);
CREATE INDEX IF NOT EXISTS banners_public_idx ON banners(enabled, starts_at, ends_at);

UPDATE system_settings
SET value=value||'{"separation_of_duties":true}'::jsonb
WHERE key='approval' AND NOT (value ? 'separation_of_duties');

UPDATE system_settings
SET value=value||'{"bootstrap_login_enabled":true}'::jsonb
WHERE key='service' AND NOT (value ? 'bootstrap_login_enabled');

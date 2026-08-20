CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username text NOT NULL UNIQUE,
  display_name text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  department text NOT NULL DEFAULT '',
  password_hash text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'user' CHECK (role IN ('user','manager','operator','admin')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  oidc_subject text UNIQUE,
  avatar_url text NOT NULL DEFAULT '',
  nickname text NOT NULL DEFAULT '',
  ranking_opt_out boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  user_agent text NOT NULL DEFAULT '',
  remote_addr text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS auth_sessions_user_idx ON auth_sessions(user_id);

CREATE TABLE IF NOT EXISTS oidc_flows (
  state_hash bytea PRIMARY KEY,
  nonce text NOT NULL,
  code_verifier text NOT NULL,
  return_to text NOT NULL DEFAULT '/',
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS system_settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL,
  secret boolean NOT NULL DEFAULT false,
  updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_preferences (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  value jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  key_prefix text NOT NULL,
  key_hash bytea NOT NULL UNIQUE,
  permissions text[] NOT NULL DEFAULT ARRAY[]::text[],
  expires_at timestamptz,
  last_used_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  rotated_from uuid REFERENCES api_keys(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS api_keys_user_idx ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS categories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  category_id uuid REFERENCES categories(id) ON DELETE SET NULL,
  tags text[] NOT NULL DEFAULT ARRAY[]::text[],
  thumbnail_url text NOT NULL DEFAULT '',
  banner_url text NOT NULL DEFAULT '',
  game_url text NOT NULL,
  game_type text NOT NULL DEFAULT 'iframe' CHECK (game_type IN ('iframe','embedded','external')),
  multiplayer boolean NOT NULL DEFAULT false,
  ranking_enabled boolean NOT NULL DEFAULT true,
  achievement_enabled boolean NOT NULL DEFAULT true,
  season_enabled boolean NOT NULL DEFAULT true,
  min_players integer NOT NULL DEFAULT 1,
  max_players integer NOT NULL DEFAULT 1,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','maintenance','disabled')),
  version text NOT NULL DEFAULT '1.0.0',
  developer text NOT NULL DEFAULT '',
  score_order text NOT NULL DEFAULT 'desc' CHECK (score_order IN ('asc','desc')),
  score_rules jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS games_status_idx ON games(status);

CREATE TABLE IF NOT EXISTS favorites (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id, game_id)
);

CREATE TABLE IF NOT EXISTS seasons (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','closed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at > starts_at)
);

CREATE TABLE IF NOT EXISTS game_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  season_id uuid REFERENCES seasons(id) ON DELETE SET NULL,
  session_token_hash bytea NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','finished','abandoned','invalid')),
  started_at timestamptz NOT NULL DEFAULT now(),
  ended_at timestamptz,
  duration_ms bigint,
  result jsonb NOT NULL DEFAULT '{}'::jsonb,
  client_info jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS game_sessions_user_idx ON game_sessions(user_id, started_at DESC);

CREATE TABLE IF NOT EXISTS game_telemetry (
  id bigserial PRIMARY KEY,
  session_id uuid NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  event text NOT NULL,
  data jsonb NOT NULL DEFAULT '{}'::jsonb,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS game_telemetry_session_idx ON game_telemetry(session_id, received_at);

CREATE TABLE IF NOT EXISTS scores (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  session_id uuid NOT NULL UNIQUE REFERENCES game_sessions(id) ON DELETE CASCADE,
  season_id uuid REFERENCES seasons(id) ON DELETE SET NULL,
  score bigint NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  verified boolean NOT NULL DEFAULT true,
  rejection_reason text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS scores_rank_idx ON scores(game_id, score DESC, created_at ASC) WHERE verified;

CREATE TABLE IF NOT EXISTS achievements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid REFERENCES games(id) ON DELETE CASCADE,
  code text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  icon_url text NOT NULL DEFAULT '',
  criteria jsonb NOT NULL DEFAULT '{}'::jsonb,
  xp integer NOT NULL DEFAULT 0,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_achievements (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  achievement_id uuid NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
  unlocked_at timestamptz NOT NULL DEFAULT now(),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY(user_id, achievement_id)
);

CREATE TABLE IF NOT EXISTS events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  event_type text NOT NULL DEFAULT 'score_attack',
  game_id uuid REFERENCES games(id) ON DELETE SET NULL,
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','closed','cancelled')),
  rules jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at > starts_at)
);

CREATE TABLE IF NOT EXISTS event_participants (
  event_id uuid NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(event_id,user_id)
);

CREATE TABLE IF NOT EXISTS workflow_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requester_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reviewer_id uuid REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id uuid,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled','applied')),
  comment text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz,
  applied_at timestamptz
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id bigserial PRIMARY KEY,
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL,
  resource_type text NOT NULL DEFAULT '',
  resource_id text NOT NULL DEFAULT '',
  remote_addr text NOT NULL DEFAULT '',
  user_agent text NOT NULL DEFAULT '',
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_created_idx ON audit_logs(created_at DESC);

INSERT INTO system_settings(key,value) VALUES
 ('service', '{"display_name":"iGame","timezone":"Asia/Seoul","public_url":"","trust_proxy":false,"bootstrap_login_enabled":true,"allowed_frame_origins":[],"allowed_connect_origins":[]}'::jsonb),
 ('oidc', '{"enabled":false,"issuer":"","client_id":"","client_secret":""}'::jsonb),
 ('approval', '{"enabled":false,"manager_required":false}'::jsonb),
 ('ai', '{"enabled":false,"base_url":"","api_key":"","default_model":"","max_tokens":262144}'::jsonb),
 ('api_keys', '{"available_permissions":["api:access","mcp:access","games:read","sessions:write","scores:write","rankings:read","profile:read","ai:invoke","workflow:write","admin:*"],"role_permissions":{"user":["api:access","mcp:access","games:read","sessions:write","scores:write","rankings:read","profile:read","ai:invoke","workflow:write"],"manager":["api:access","mcp:access","games:read","sessions:write","scores:write","rankings:read","profile:read","ai:invoke","workflow:write"],"operator":["api:access","mcp:access","games:read","sessions:write","scores:write","rankings:read","profile:read","ai:invoke","workflow:write"],"admin":["api:access","mcp:access","games:read","sessions:write","scores:write","rankings:read","profile:read","ai:invoke","workflow:write","admin:*"]},"max_keys":10,"max_ttl_days":365}'::jsonb),
 ('privacy', '{"ranking_name":"nickname","show_department":true,"ranking_opt_out":true}'::jsonb),
 ('play_policy', '{"enabled":false,"windows":[],"daily_limits":{}}'::jsonb)
ON CONFLICT (key) DO NOTHING;

INSERT INTO categories(slug,name,description,sort_order) VALUES
 ('arcade','아케이드','빠르게 즐기는 클래식 아케이드 게임',10),
 ('puzzle','퍼즐','집중력과 사고력을 쓰는 퍼즐 게임',20),
 ('skill','스킬','반응 속도와 입력 능력을 겨루는 게임',30)
ON CONFLICT(slug) DO NOTHING;

INSERT INTO achievements(code,name,description,criteria,xp,active) VALUES
 ('first-play','첫 플레이','iGame에서 첫 게임을 시작했습니다.','{"type":"play_count","count":1}'::jsonb,100,true),
 ('explorer','탐험가','서로 다른 게임 5개를 플레이했습니다.','{"type":"distinct_games","count":5}'::jsonb,300,true)
ON CONFLICT(code) DO NOTHING;

INSERT INTO games(slug,name,description,category_id,tags,thumbnail_url,banner_url,game_url,game_type,ranking_enabled,achievement_enabled,season_enabled,status,version,developer,score_rules)
VALUES
 ('2048','2048','숫자 타일을 합쳐 2048을 만드세요.',(SELECT id FROM categories WHERE slug='puzzle'),ARRAY['퍼즐','숫자'],'/assets/games/2048.svg','/assets/games/2048-banner.svg','/play/2048','embedded',true,true,true,'active','1.0.0','iGame', '{"min_score":0,"max_score":10000000,"min_duration_ms":1000}'::jsonb),
 ('snake','Snake','먹이를 먹으며 가장 긴 뱀에 도전하세요.',(SELECT id FROM categories WHERE slug='arcade'),ARRAY['아케이드','클래식'],'/assets/games/snake.svg','/assets/games/snake-banner.svg','/play/snake','embedded',true,true,true,'active','1.0.0','iGame', '{"min_score":0,"max_score":1000000,"min_duration_ms":1000}'::jsonb),
 ('memory','Memory Cards','같은 그림의 카드를 기억해 맞추세요.',(SELECT id FROM categories WHERE slug='puzzle'),ARRAY['퍼즐','기억력'],'/assets/games/memory.svg','/assets/games/memory-banner.svg','/play/memory','embedded',true,true,true,'active','1.0.0','iGame', '{"min_score":0,"max_score":1000000,"min_duration_ms":1000}'::jsonb),
 ('reaction','Reaction Test','신호가 나타나는 순간 가장 빠르게 반응하세요.',(SELECT id FROM categories WHERE slug='skill'),ARRAY['반응속도','스킬'],'/assets/games/reaction.svg','/assets/games/reaction-banner.svg','/play/reaction','embedded',true,true,true,'active','1.0.0','iGame', '{"min_score":1,"max_score":60000,"min_duration_ms":500}'::jsonb),
 ('typing','Typing Game','정확하고 빠르게 문장을 입력하세요.',(SELECT id FROM categories WHERE slug='skill'),ARRAY['타이핑','스킬'],'/assets/games/typing.svg','/assets/games/typing-banner.svg','/play/typing','embedded',true,true,true,'active','1.0.0','iGame', '{"min_score":0,"max_score":1000000,"min_duration_ms":1000}'::jsonb)
ON CONFLICT(slug) DO NOTHING;

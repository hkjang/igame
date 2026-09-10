-- Demo content for the guide screen captures.
--
-- Every name, address and department in this file is invented. The captures in
-- docs/assets/guide are published, so nothing here may resemble a real person
-- or a real internal host. Passwords are not set: these accounts exist to fill
-- lists and rankings, and none of them can log in.
--
-- This script writes rows that igame's own API cannot create (users, and the
-- finished sessions a ranking is built from). It is destructive by design and
-- only ever runs against the disposable database the capture host starts.

BEGIN;

INSERT INTO users(username,display_name,email,department,team,role,status,nickname,last_login_at) VALUES
  ('minji.kang','강민지','minji.kang@example.internal','플랫폼개발실','포털파트','operator','active','민지',now()-interval '2 hours'),
  ('jihoon.seo','서지훈','jihoon.seo@example.internal','플랫폼개발실','런타임파트','manager','active','지훈',now()-interval '5 hours'),
  ('yuna.park','박유나','yuna.park@example.internal','정보보안팀','보안운영파트','user','active','유나',now()-interval '1 day'),
  ('dohyun.lim','임도현','dohyun.lim@example.internal','정보보안팀','보안운영파트','user','active','도현',now()-interval '3 hours'),
  ('suji.na','나수지','suji.na@example.internal','인사총무팀','조직문화파트','user','active','수지',now()-interval '9 hours'),
  ('taeyang.oh','오태양','taeyang.oh@example.internal','인사총무팀','조직문화파트','user','active','태양',now()-interval '2 days'),
  ('haeun.jung','정하은','haeun.jung@example.internal','데이터분석팀','ML파트','user','active','하은',now()-interval '30 minutes'),
  ('junseo.moon','문준서','junseo.moon@example.internal','데이터분석팀','ML파트','user','disabled','준서',now()-interval '21 days')
ON CONFLICT (username) DO NOTHING;

-- The bootstrap administrator shows up in every list and on the profile screen,
-- so give it the same invented identity as the rest of the demo data.
UPDATE users SET display_name='데모 관리자',email='admin@example.internal',department='플랫폼개발실',team='운영파트',nickname='데모관리자'
WHERE display_name='' OR display_name=username;

-- Finished sessions and the scores that come from them. Rankings, the play
-- history and every dashboard tile read these, so an empty table means an empty
-- screen capture.
WITH demo AS (
  SELECT u.id AS user_id, u.username, g.id AS game_id, g.slug,
         row_number() OVER(PARTITION BY g.slug ORDER BY u.username) AS seat
  FROM users u
  CROSS JOIN games g
  WHERE (u.username IN ('minji.kang','jihoon.seo','yuna.park','dohyun.lim','suji.na','taeyang.oh','haeun.jung') OR u.role='admin')
    AND g.slug IN ('2048','memory','reaction','snake','typing')
), inserted AS (
  INSERT INTO game_sessions(user_id,game_id,session_token_hash,status,started_at,ended_at,duration_ms,result)
  SELECT user_id, game_id,
         sha256(convert_to('guide-capture:'||username||':'||slug,'UTF8')),
         'finished',
         now()-((seat*37)||' minutes')::interval,
         now()-((seat*37-6)||' minutes')::interval,
         360000,
         '{"source":"guide-capture"}'::jsonb
  FROM demo
  ON CONFLICT (session_token_hash) DO NOTHING
  RETURNING id, user_id, game_id, started_at
)
INSERT INTO scores(user_id,game_id,session_id,score,verified,created_at)
SELECT user_id, game_id, id,
       12000 - (row_number() OVER(PARTITION BY game_id ORDER BY started_at DESC))*830,
       true,
       started_at
FROM inserted
ON CONFLICT (session_id) DO NOTHING;

COMMIT;

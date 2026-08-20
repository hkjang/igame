INSERT INTO categories(slug,name,description,sort_order)
VALUES ('strategy','전략','방어선과 영웅을 운용하는 전략 게임',40)
ON CONFLICT(slug) DO NOTHING;

INSERT INTO games(slug,name,description,category_id,tags,thumbnail_url,banner_url,game_url,game_type,ranking_enabled,achievement_enabled,season_enabled,status,version,developer,score_order,score_rules)
VALUES ('realmguard','RealmGuard','영웅과 타워로 왕국을 수호하는 데이터 기반 타워 디펜스.',(SELECT id FROM categories WHERE slug='strategy'),ARRAY['전략','타워 디펜스','캠페인'],'/assets/games/realmguard.svg','/assets/games/realmguard-banner.svg','/games/realmguard','embedded',true,true,true,'active','0.2.0','iGame','desc','{"server_authoritative":true}'::jsonb)
ON CONFLICT(slug) DO UPDATE SET version='0.2.0',game_url='/games/realmguard',score_rules=excluded.score_rules,updated_at=now();

CREATE TABLE realmguard_content_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  version_no integer NOT NULL UNIQUE CHECK (version_no > 0),
  label text NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','testing','pending_approval','approved','published','archived')),
  content_version text NOT NULL,
  stage_version text NOT NULL,
  balance_version text NOT NULL,
  asset_version text NOT NULL,
  checksum text NOT NULL DEFAULT '',
  notes text NOT NULL DEFAULT '',
  content jsonb NOT NULL,
  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  approved_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  tested_at timestamptz,
  approval_requested_at timestamptz,
  approved_at timestamptz,
  review_comment text NOT NULL DEFAULT '',
  reviewed_at timestamptz,
  published_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX realmguard_one_published_idx ON realmguard_content_versions((status)) WHERE status='published';
CREATE INDEX realmguard_content_status_idx ON realmguard_content_versions(status,version_no DESC);

ALTER TABLE game_sessions ADD COLUMN realmguard_content_version_id uuid REFERENCES realmguard_content_versions(id) ON DELETE RESTRICT;
CREATE INDEX game_sessions_realmguard_version_idx ON game_sessions(realmguard_content_version_id) WHERE realmguard_content_version_id IS NOT NULL;

CREATE TABLE realmguard_user_progress (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  stage_id text NOT NULL,
  difficulty text NOT NULL CHECK (difficulty IN ('casual','normal','veteran')),
  unlocked boolean NOT NULL DEFAULT false,
  completed boolean NOT NULL DEFAULT false,
  stars smallint NOT NULL DEFAULT 0 CHECK (stars BETWEEN 0 AND 3),
  best_score bigint NOT NULL DEFAULT 0 CHECK (best_score >= 0),
  best_duration_ms bigint,
  best_hero_id text NOT NULL DEFAULT '',
  wins integer NOT NULL DEFAULT 0 CHECK (wins >= 0),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  total_playtime_ms bigint NOT NULL DEFAULT 0 CHECK (total_playtime_ms >= 0),
  content_version_id uuid REFERENCES realmguard_content_versions(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,stage_id,difficulty)
);

CREATE TABLE realmguard_user_heroes (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  hero_id text NOT NULL,
  unlocked boolean NOT NULL DEFAULT false,
  level integer NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 100),
  xp bigint NOT NULL DEFAULT 0 CHECK (xp >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,hero_id)
);

CREATE TABLE realmguard_user_skills (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  skill_id text NOT NULL,
  unlocked boolean NOT NULL DEFAULT false,
  level integer NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 20),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,skill_id)
);

CREATE TABLE realmguard_user_loadouts (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  hero_id text NOT NULL,
  skill_ids text[] NOT NULL DEFAULT ARRAY[]::text[],
  settings jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE realmguard_results (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id uuid NOT NULL UNIQUE REFERENCES game_sessions(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content_version_id uuid NOT NULL REFERENCES realmguard_content_versions(id) ON DELETE RESTRICT,
  stage_id text NOT NULL,
  mode text NOT NULL CHECK (mode IN ('campaign','endless')),
  difficulty text NOT NULL CHECK (difficulty IN ('casual','normal','veteran')),
  duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
  remaining_lives integer NOT NULL CHECK (remaining_lives >= 0),
  remaining_gold bigint NOT NULL CHECK (remaining_gold >= 0),
  earned_gold bigint NOT NULL CHECK (earned_gold >= 0),
  spent_gold bigint NOT NULL CHECK (spent_gold >= 0),
  sold_gold bigint NOT NULL CHECK (sold_gold >= 0),
  kills integer NOT NULL CHECK (kills >= 0),
  escaped integer NOT NULL CHECK (escaped >= 0),
  spawned integer NOT NULL CHECK (spawned >= 0),
  waves_completed integer NOT NULL CHECK (waves_completed >= 0),
  hero_id text NOT NULL,
  hero_level integer NOT NULL CHECK (hero_level BETWEEN 1 AND 100),
  score bigint NOT NULL CHECK (score >= 0),
  stars smallint NOT NULL CHECK (stars BETWEEN 0 AND 3),
  verified boolean NOT NULL DEFAULT true,
  rejection_reason text NOT NULL DEFAULT '',
  score_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb,
  proof text NOT NULL DEFAULT '',
  events jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX realmguard_results_rank_idx ON realmguard_results(content_version_id,mode,stage_id,difficulty,score DESC,created_at) WHERE verified;
CREATE INDEX realmguard_results_user_idx ON realmguard_results(user_id,created_at DESC);

WITH campaign(n,name,subtitle,theme,gimmick) AS (VALUES
  (1,'이끼빛 관문','첫 장막이 흔들린다','verdant',''),
  (2,'유리바람 평원','빛나는 들판의 추격전','verdant',''),
  (3,'속삭임 습지','늪의 속삭임을 잠재워라','verdant',''),
  (4,'잿불 고개','불씨를 지키는 길','ember','ember_vents'),
  (5,'별뿌리 성소','공허왕의 첫 강림','void',''),
  (6,'부서진 월교','두 세계를 잇는 마지막 다리','ember',''),
  (7,'서리결 골짜기','멈춘 겨울의 심장','frost','winter_blessing'),
  (8,'무명의 회랑','기억을 잃는 길','void',''),
  (9,'시간의 균열','시간룡이 깨어난다','frost','time_surge'),
  (10,'새벽 없는 왕좌','Realm의 운명을 건 수호전','void','')
), stage_base AS (
  SELECT n,'stage-'||n AS id,name,subtitle,'campaign' AS mode,theme,gimmick,
    LEAST(15,8+((n+1)/2)) AS wave_count,280+n*10 AS starting_gold,20 AS lives
  FROM campaign
  UNION ALL SELECT 11,'endless-rift','끝없는 균열','한계 없이 밀려오는 장막','endless','void','',15,420,25
), stage_seed AS (
  SELECT s.*,
    CASE (CASE WHEN n=11 THEN 3 ELSE ((n-1)%4) END)
      WHEN 0 THEN '[{"x":-30,"y":250},{"x":210,"y":250},{"x":210,"y":430},{"x":520,"y":430},{"x":520,"y":190},{"x":900,"y":190},{"x":900,"y":390},{"x":1310,"y":390}]'::jsonb
      WHEN 1 THEN '[{"x":-30,"y":150},{"x":250,"y":150},{"x":360,"y":350},{"x":640,"y":350},{"x":760,"y":560},{"x":990,"y":560},{"x":1080,"y":270},{"x":1310,"y":270}]'::jsonb
      WHEN 2 THEN '[{"x":-30,"y":520},{"x":180,"y":520},{"x":330,"y":300},{"x":560,"y":300},{"x":680,"y":120},{"x":930,"y":120},{"x":1060,"y":450},{"x":1310,"y":450}]'::jsonb
      ELSE '[{"x":-30,"y":360},{"x":180,"y":360},{"x":310,"y":130},{"x":530,"y":130},{"x":650,"y":500},{"x":880,"y":500},{"x":1020,"y":260},{"x":1310,"y":260}]'::jsonb END AS path,
    CASE (CASE WHEN n=11 THEN 1 ELSE ((n-1)%2) END)
      WHEN 0 THEN '[{"x":120,"y":160},{"x":310,"y":340},{"x":400,"y":520},{"x":610,"y":300},{"x":780,"y":100},{"x":820,"y":300},{"x":1030,"y":480},{"x":1130,"y":300}]'::jsonb
      ELSE '[{"x":140,"y":260},{"x":310,"y":190},{"x":430,"y":450},{"x":570,"y":250},{"x":700,"y":460},{"x":850,"y":620},{"x":970,"y":430},{"x":1130,"y":180}]'::jsonb END AS spot_template
  FROM stage_base s
), wave_raw AS (
  SELECT s.id AS stage_id,s.n,s.wave_count,w AS number,LEAST(10,2+s.n) AS pool_size,
    ARRAY['mireling','thornback','glintfox','cloudray','bloomseer','shardling','ironroot','veilrunner','rammer','rimeheart']::text[] AS enemy_ids
  FROM stage_seed s CROSS JOIN LATERAL generate_series(1,s.wave_count) AS w
), wave_seed AS (
  SELECT stage_id,number,28+n*4+(number-1)*3 AS reward,
    jsonb_build_array(jsonb_build_object('enemy',enemy_ids[mod(number-1+n,pool_size)+1],'count',5+floor((number-1+n)*0.75)::int,'interval',GREATEST(0.35,0.85-n*0.025),'modifiers',
      CASE WHEN n>=9 AND mod(number-1,4)=3 THEN '["immune_stun"]'::jsonb
           WHEN n>=8 AND mod(number-1,4)=2 THEN '["berserk"]'::jsonb
           WHEN n>=7 AND mod(number-1,4)=1 THEN '["stealth"]'::jsonb
           WHEN n>=6 AND mod(number-1,4)=0 THEN '["magic_resist"]'::jsonb
           ELSE '[]'::jsonb END))
    || CASE WHEN number>2 THEN jsonb_build_array(jsonb_build_object('enemy',enemy_ids[mod(number+n+2,pool_size)+1],'count',2+floor((number-1+n)/3.0)::int,'interval',1.05,'delay',1.5)) ELSE '[]'::jsonb END
    || CASE WHEN n=5 AND number=wave_count THEN '[{"enemy":"hollow_king","count":1,"interval":1.5,"delay":2}]'::jsonb
            WHEN n=10 AND number=wave_count THEN '[{"enemy":"timewyrm","count":1,"interval":1.5,"delay":2}]'::jsonb ELSE '[]'::jsonb END AS entries
  FROM wave_raw
), content_seed AS (
  SELECT jsonb_build_object(
    'schema_version','0.2.0',
    'stages',(SELECT jsonb_agg(jsonb_strip_nulls(jsonb_build_object(
      'id',id,'number',n,'name',name,'subtitle',subtitle,'mode',mode,'theme',theme,'path',path,
      'tower_spots',(SELECT jsonb_agg(jsonb_build_object('id',CASE WHEN id='endless-rift' THEN 'endless-spot-'||ordinality ELSE 's'||n||'-spot-'||ordinality END,'x',(spot->>'x')::int,'y',(spot->>'y')::int) ORDER BY ordinality) FROM jsonb_array_elements(spot_template) WITH ORDINALITY AS points(spot,ordinality)),
      'starting_gold',starting_gold,'lives',lives,'version',CASE WHEN id='endless-rift' THEN '1.0.0' ELSE '1.'||n||'.0' END,'gimmick',NULLIF(gimmick,''))) ORDER BY n) FROM stage_seed),
    'waves',(SELECT jsonb_agg(jsonb_build_object('id','s'||CASE WHEN stage_id='endless-rift' THEN '11' ELSE split_part(stage_id,'-',2) END||'-w'||number,'stage_id',stage_id,'number',number,'label',number||' 파동','entries',entries,'reward',reward) ORDER BY stage_id,number) FROM wave_seed),
    'towers','[
      {"id":"sunspire","name":"태양첨탑","role":"빠른 단일 사격","color":16762967,"cost":75,"damage":18,"range":150,"fire_rate":0.52,"projectile_speed":440,"damage_type":"physical","branches":[{"id":"dawn_volley","name":"여명 연사","description":"공격 속도와 관통 강화","rate_multiplier":0.58,"pierce":2},{"id":"eagle_oath","name":"독수리 맹세","description":"사거리와 치명 피해 강화","range_multiplier":1.35,"damage_multiplier":1.75}]},
      {"id":"runebloom","name":"룬꽃 정원","role":"방어 무시 비전 공격","color":13016319,"cost":105,"damage":33,"range":138,"fire_rate":0.92,"projectile_speed":360,"damage_type":"arcane","branches":[{"id":"star_lattice","name":"별 격자","description":"주변 적에게 연쇄 피해","damage_multiplier":1.4,"splash":58},{"id":"null_petal","name":"무효의 꽃잎","description":"강한 적 방어 관통","damage_multiplier":1.85,"pierce":3}]},
      {"id":"stonepulse","name":"석맥 포대","role":"느린 광역 포격","color":15238236,"cost":125,"damage":62,"range":170,"fire_rate":1.55,"projectile_speed":290,"damage_type":"siege","branches":[{"id":"quake_drum","name":"지진북","description":"거대한 폭발 반경","damage_multiplier":1.3,"splash":96},{"id":"ember_core","name":"잿불핵","description":"집중 고열탄","damage_multiplier":2.2,"splash":44}]},
      {"id":"windward","name":"바람수호 병영","role":"병사 소환·길목 저지","color":6937828,"cost":95,"damage":16,"range":92,"fire_rate":0.72,"projectile_speed":390,"damage_type":"physical","branches":[{"id":"shield_line","name":"방패선","description":"강력한 지상 저지와 방어","slow":0.68,"damage_multiplier":1.35},{"id":"skyrider_watch","name":"하늘기수 초소","description":"비행 대응과 빠른 공격","rate_multiplier":0.58,"range_multiplier":1.45,"damage_multiplier":1.55}]}
    ]'::jsonb,
    'enemies','[
      {"id":"mireling","name":"습지 꼬마","color":8376181,"hp":45,"speed":54,"armor":0,"reward":8,"life_damage":1,"radius":12,"traits":[]},
      {"id":"thornback","name":"가시등","color":5212504,"hp":120,"speed":34,"armor":0.24,"reward":15,"life_damage":1,"radius":15,"traits":["armored"]},
      {"id":"glintfox","name":"섬광여우","color":16107370,"hp":58,"speed":92,"armor":0,"reward":11,"life_damage":1,"radius":10,"traits":["swift"]},
      {"id":"cloudray","name":"구름가오리","color":9430015,"hp":85,"speed":68,"armor":0.08,"reward":14,"life_damage":1,"radius":13,"traits":["flying"]},
      {"id":"bloomseer","name":"꽃점술사","color":15765205,"hp":105,"speed":42,"armor":0.05,"reward":18,"life_damage":1,"radius":13,"traits":["healer"]},
      {"id":"shardling","name":"파편충","color":12033279,"hp":88,"speed":56,"armor":0.06,"reward":14,"life_damage":1,"radius":13,"traits":["splitting"]},
      {"id":"ironroot","name":"철근목","color":9273442,"hp":260,"speed":25,"armor":0.38,"reward":25,"life_damage":2,"radius":19,"traits":["armored","regenerating"]},
      {"id":"veilrunner","name":"장막질주자","color":9144279,"hp":125,"speed":74,"armor":0.1,"reward":19,"life_damage":1,"radius":12,"traits":["phasing","swift"]},
      {"id":"rammer","name":"성문분쇄자","color":14316893,"hp":360,"speed":28,"armor":0.32,"reward":34,"life_damage":3,"radius":21,"traits":["siege","armored"]},
      {"id":"rimeheart","name":"서리심장","color":7391720,"hp":210,"speed":38,"armor":0.18,"reward":24,"life_damage":2,"radius":17,"traits":["regenerating"]}
    ]'::jsonb,
    'bosses','[
      {"id":"hollow_king","name":"공허왕 오르반","color":10312680,"hp":2200,"speed":22,"armor":0.35,"reward":220,"life_damage":10,"radius":32,"traits":["boss","phasing","armored"]},
      {"id":"timewyrm","name":"시간룡 세라크","color":16746090,"hp":4100,"speed":27,"armor":0.3,"reward":400,"life_damage":15,"radius":38,"traits":["boss","swift","regenerating"]}
    ]'::jsonb,
    'heroes','[
      {"id":"aerin","name":"에어린","title":"새벽 추적자","color":16765739,"hp":520,"damage":32,"range":115,"speed":150,"respawn_seconds":9,"skill1":"별빛 화살","skill2":"황혼 도약","ultimate":"새벽의 비","unlock_stage":1},
      {"id":"brann","name":"브란","title":"석문 파수꾼","color":14255970,"hp":880,"damage":48,"range":48,"speed":112,"respawn_seconds":12,"skill1":"방패 강타","skill2":"철벽진","ultimate":"대지의 맹세","unlock_stage":3},
      {"id":"nyra","name":"니라","title":"서리결 마도사","color":8183026,"hp":430,"damage":38,"range":130,"speed":132,"respawn_seconds":10,"skill1":"빙결파","skill2":"거울 서리","ultimate":"백야","unlock_stage":6}
    ]'::jsonb,
    'skills','[
      {"id":"meteor","name":"별똥 낙하","description":"선택 지점에 강력한 범위 피해","cooldown":38,"color":"#ff8b5e","unlock_stage":1},
      {"id":"reinforcement","name":"수호대 소집","description":"길목을 지키는 수호대 배치","cooldown":28,"color":"#ffd36b","unlock_stage":4},
      {"id":"freeze","name":"시간 서리","description":"모든 적을 잠시 둔화","cooldown":44,"color":"#73dcff","unlock_stage":7}
    ]'::jsonb,
    'balance','{"difficulties":{"casual":{"enemy_hp":0.82,"enemy_speed":0.92,"gold":1.18,"score":0.8,"difficulty_bonus":0},"normal":{"enemy_hp":1,"enemy_speed":1,"gold":1,"score":1,"difficulty_bonus":5000},"veteran":{"enemy_hp":1.38,"enemy_speed":1.12,"gold":0.9,"score":1.5,"difficulty_bonus":10000}},"tower_upgrade_cost":[0,70,120],"hero_level_xp":[0,8,20,38,62,92,130,176,230,292],"endless_ramp":0.085,"sell_refund_rate":0.65,"clear_time_target_ms":900000,"clear_time_bonus_divisor":100,"endless_wave_bonus":1000,"duration_tolerance_ms":5000,"min_wave_duration_ms":5000,"server_uses_score_multiplier":false,"score_formula":"remaining_lives*1000 + remaining_gold*10 + clear_time_bonus + difficulty_bonus"}'::jsonb
  ) AS content
)
INSERT INTO realmguard_content_versions(version_no,label,status,content_version,stage_version,balance_version,asset_version,checksum,notes,content,published_at)
SELECT 1,'v0.2.0','published','0.2.0','2026.08.1','2026.08.1','procedural-1',encode(digest(convert_to(content::text,'UTF8'),'sha256'),'hex'),'RealmGuard canonical v0.2.0 snapshot',content,now()
FROM content_seed;

INSERT INTO achievements(game_id,code,name,description,criteria,xp,active)
SELECT (SELECT id FROM games WHERE slug='realmguard'),v.code,v.name,v.description,v.criteria::jsonb,v.xp,true
FROM (VALUES
 ('realmguard-first-defense','첫 방어','RealmGuard 전투를 처음 완료했습니다.','{"server_rule":"first_verified_result"}',50),
 ('realmguard-first-victory','첫 승리','캠페인 전투에서 처음 승리했습니다.','{"server_rule":"first_campaign_victory"}',100),
 ('realmguard-stage-1','이끼빛 수호자','스테이지 1을 완료했습니다.','{"server_rule":"campaign_stage","stage":1}',100),
 ('realmguard-stage-2','평원의 추격자','스테이지 2를 완료했습니다.','{"server_rule":"campaign_stage","stage":2}',100),
 ('realmguard-stage-3','습지 정화자','스테이지 3을 완료했습니다.','{"server_rule":"campaign_stage","stage":3}',120),
 ('realmguard-stage-4','잿불지기','스테이지 4를 완료했습니다.','{"server_rule":"campaign_stage","stage":4}',120),
 ('realmguard-stage-5','공허왕 격파','스테이지 5를 완료했습니다.','{"server_rule":"campaign_stage","stage":5}',150),
 ('realmguard-stage-6','월교의 파수꾼','스테이지 6을 완료했습니다.','{"server_rule":"campaign_stage","stage":6}',150),
 ('realmguard-stage-7','겨울의 심장','스테이지 7을 완료했습니다.','{"server_rule":"campaign_stage","stage":7}',170),
 ('realmguard-stage-8','기억의 수호자','스테이지 8을 완료했습니다.','{"server_rule":"campaign_stage","stage":8}',170),
 ('realmguard-stage-9','시간을 거스른 자','스테이지 9를 완료했습니다.','{"server_rule":"campaign_stage","stage":9}',200),
 ('realmguard-stage-10','RealmGuard','캠페인 최종 스테이지를 완료했습니다.','{"server_rule":"campaign_stage","stage":10}',300),
 ('realmguard-three-star','완벽에 가까운 방어','캠페인에서 별 3개를 획득했습니다.','{"server_rule":"stars","minimum":3}',150),
 ('realmguard-campaign-master','왕국의 수호자','캠페인 10개 스테이지를 완료했습니다.','{"server_rule":"campaign_complete"}',500),
 ('realmguard-endless-10','균열 도전자','끝없는 균열에서 10 파동을 버텼습니다.','{"server_rule":"endless_waves","minimum":10}',150),
 ('realmguard-endless-25','균열 정복자','끝없는 균열에서 25 파동을 버텼습니다.','{"server_rule":"endless_waves","minimum":25}',300),
 ('realmguard-veteran','베테랑 수호대','베테랑 난이도 캠페인을 완료했습니다.','{"server_rule":"difficulty_victory","difficulty":"veteran"}',200),
 ('realmguard-flawless','한 치의 빈틈도 없이','생명을 하나도 잃지 않고 승리했습니다.','{"server_rule":"flawless"}',200),
 ('realmguard-wealthy','황금 수호자','전투 종료 시 금화 1,000 이상을 보유했습니다.','{"server_rule":"remaining_gold","minimum":1000}',120),
 ('realmguard-hero-master','영웅의 귀환','영웅 레벨 10을 달성했습니다.','{"server_rule":"hero_level","minimum":10}',250)
) AS v(code,name,description,criteria,xp)
ON CONFLICT(code) DO NOTHING;

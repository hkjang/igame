INSERT INTO categories(slug,name,description,sort_order)
VALUES ('defense-series','Defense Series','업무·보안·AI 지식을 함께 익히는 데이터 기반 디펜스 시리즈',45)
ON CONFLICT(slug) DO UPDATE SET name=excluded.name,description=excluded.description,sort_order=excluded.sort_order;

INSERT INTO games(slug,name,description,category_id,tags,thumbnail_url,banner_url,game_url,game_type,ranking_enabled,achievement_enabled,season_enabled,status,version,developer,score_order,score_rules)
VALUES
 ('office-guardians','Office Guardians','조직과 직무 캐릭터가 회사의 핵심 자산을 지키는 사내 캐릭터 디펜스.',(SELECT id FROM categories WHERE slug='defense-series'),ARRAY['Defense Series','조직','타워 디펜스'],'/assets/games/office-guardians.svg','/assets/games/office-guardians-banner.svg','/games/office-guardians','embedded',true,true,true,'active','0.3.0','iGame','desc','{"server_authoritative":true,"result_endpoint":"/api/v1/defense/office-guardians/results"}'::jsonb),
 ('cyber-fortress','Cyber Fortress','실제 보안 위협과 대응 원리를 게임으로 학습하는 보안교육 디펜스.',(SELECT id FROM categories WHERE slug='defense-series'),ARRAY['Defense Series','보안교육','타워 디펜스'],'/assets/games/cyber-fortress.svg','/assets/games/cyber-fortress-banner.svg','/games/cyber-fortress','embedded',true,true,true,'active','0.3.0','iGame','desc','{"server_authoritative":true,"result_endpoint":"/api/v1/defense/cyber-fortress/results"}'::jsonb),
 ('ai-nexus-defense','AI Nexus Defense','AI 플랫폼의 품질·보안·비용을 방어하며 AI 거버넌스를 익히는 디펜스.',(SELECT id FROM categories WHERE slug='defense-series'),ARRAY['Defense Series','AI교육','타워 디펜스'],'/assets/games/ai-nexus-defense.svg','/assets/games/ai-nexus-defense-banner.svg','/games/ai-nexus-defense','embedded',true,true,true,'active','0.3.0','iGame','desc','{"server_authoritative":true,"result_endpoint":"/api/v1/defense/ai-nexus-defense/results"}'::jsonb)
ON CONFLICT(slug) DO UPDATE SET name=excluded.name,description=excluded.description,category_id=excluded.category_id,tags=excluded.tags,thumbnail_url=excluded.thumbnail_url,banner_url=excluded.banner_url,game_url=excluded.game_url,status='active',version='0.3.0',score_rules=excluded.score_rules,updated_at=now();

CREATE TABLE defense_content_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  version_no integer NOT NULL CHECK (version_no > 0),
  label text NOT NULL,
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','testing','pending_approval','approved','published','archived')),
  content_version text NOT NULL,
  policy_version text NOT NULL,
  asset_version text NOT NULL,
  checksum text NOT NULL DEFAULT '',
  notes text NOT NULL DEFAULT '',
  content jsonb NOT NULL,
  source_version_id uuid REFERENCES defense_content_versions(id) ON DELETE SET NULL,
  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  approved_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  tested_at timestamptz,
  approval_requested_at timestamptz,
  approved_at timestamptz,
  review_comment text NOT NULL DEFAULT '',
  reviewed_at timestamptz,
  published_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(game_id,version_no),
  UNIQUE(game_id,label)
);
CREATE UNIQUE INDEX defense_one_published_idx ON defense_content_versions(game_id) WHERE status='published';
CREATE INDEX defense_content_status_idx ON defense_content_versions(game_id,status,version_no DESC);

ALTER TABLE game_sessions ADD COLUMN defense_content_version_id uuid REFERENCES defense_content_versions(id) ON DELETE RESTRICT;
CREATE INDEX game_sessions_defense_version_idx ON game_sessions(defense_content_version_id) WHERE defense_content_version_id IS NOT NULL;

CREATE TABLE defense_results (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id uuid NOT NULL UNIQUE REFERENCES game_sessions(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  content_version_id uuid NOT NULL REFERENCES defense_content_versions(id) ON DELETE RESTRICT,
  stage_id text NOT NULL,
  difficulty text NOT NULL CHECK (difficulty IN ('casual','normal','veteran')),
  duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
  remaining_health bigint NOT NULL CHECK (remaining_health >= 0),
  remaining_resource bigint NOT NULL CHECK (remaining_resource >= 0),
  kills integer NOT NULL CHECK (kills >= 0),
  escaped integer NOT NULL CHECK (escaped >= 0),
  spawned integer NOT NULL CHECK (spawned >= 0),
  waves_completed integer NOT NULL CHECK (waves_completed >= 0),
  victory boolean NOT NULL,
  score bigint NOT NULL CHECK (score >= 0),
  stars smallint NOT NULL CHECK (stars BETWEEN 0 AND 3),
  learning_score smallint NOT NULL DEFAULT 0 CHECK (learning_score BETWEEN 0 AND 100),
  policy_version text NOT NULL,
  resource_state jsonb NOT NULL DEFAULT '{}'::jsonb,
  score_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb,
  learning_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb,
  answers jsonb NOT NULL DEFAULT '[]'::jsonb,
  request_hash text NOT NULL,
  verification_method text NOT NULL DEFAULT 'server_received_telemetry_v1',
  attestation jsonb NOT NULL DEFAULT '{}'::jsonb,
  server_proof text NOT NULL DEFAULT '',
  verified boolean NOT NULL DEFAULT true,
  rejection_reason text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX defense_results_rank_idx ON defense_results(game_id,content_version_id,stage_id,difficulty,score DESC,created_at) WHERE verified;
CREATE INDEX defense_results_user_idx ON defense_results(user_id,game_id,created_at DESC);

CREATE TABLE defense_user_progress (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  stage_id text NOT NULL,
  difficulty text NOT NULL CHECK (difficulty IN ('casual','normal','veteran')),
  unlocked boolean NOT NULL DEFAULT false,
  completed boolean NOT NULL DEFAULT false,
  stars smallint NOT NULL DEFAULT 0 CHECK (stars BETWEEN 0 AND 3),
  best_score bigint NOT NULL DEFAULT 0 CHECK (best_score >= 0),
  best_learning_score smallint NOT NULL DEFAULT 0 CHECK (best_learning_score BETWEEN 0 AND 100),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  completions integer NOT NULL DEFAULT 0 CHECK (completions >= 0),
  total_playtime_ms bigint NOT NULL DEFAULT 0 CHECK (total_playtime_ms >= 0),
  content_version_id uuid NOT NULL REFERENCES defense_content_versions(id) ON DELETE RESTRICT,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,game_id,content_version_id,stage_id,difficulty)
);

CREATE TABLE defense_event_answers (
  session_id uuid NOT NULL REFERENCES game_sessions(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  content_version_id uuid NOT NULL REFERENCES defense_content_versions(id) ON DELETE RESTRICT,
  event_id text NOT NULL,
  question_id text NOT NULL,
  answer_id text NOT NULL,
  topic text NOT NULL,
  correct boolean NOT NULL,
  score smallint NOT NULL CHECK (score BETWEEN 0 AND 100),
  policy_version text NOT NULL,
  request_hash text NOT NULL,
  answered_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(session_id,event_id)
);
CREATE INDEX defense_answers_learning_idx ON defense_event_answers(user_id,game_id,topic,answered_at DESC);

CREATE TABLE defense_campaign_progress (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  campaign_id text NOT NULL,
  completed boolean NOT NULL DEFAULT false,
  completed_stages integer NOT NULL DEFAULT 0 CHECK (completed_stages >= 0),
  required_stages integer NOT NULL DEFAULT 0 CHECK (required_stages >= 0),
  learning_score smallint NOT NULL DEFAULT 0 CHECK (learning_score BETWEEN 0 AND 100),
  content_version_id uuid NOT NULL REFERENCES defense_content_versions(id) ON DELETE RESTRICT,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,game_id,content_version_id,campaign_id)
);

WITH specs(slug,stage_count,tower_count,enemy_count,boss_count,hero_count,event_count,question_count,education_enabled,policy_version) AS (VALUES
 ('office-guardians',8,6,10,2,3,0,0,false,'office-policy-2026.08'),
 ('cyber-fortress',10,8,15,3,3,30,50,true,'security-policy-2026.08'),
 ('ai-nexus-defense',10,10,15,4,5,30,50,true,'ai-policy-2026.08')
), packs AS (
 SELECT s.*,jsonb_build_object(
  'schema_version','0.3.0',
  'stages',(SELECT jsonb_agg(jsonb_build_object('id','stage-'||n,'number',n,'name',CASE s.slug WHEN 'office-guardians' THEN 'Company Zone ' WHEN 'cyber-fortress' THEN 'Security Scenario ' ELSE 'AI Nexus ' END||n,'mode','campaign','starting_health',20,'starting_resource',220+n*10,'version','1.'||n||'.0') ORDER BY n) FROM generate_series(1,s.stage_count) n),
  'waves',(SELECT jsonb_agg(jsonb_build_object('id','s'||st||'-w'||w,'stage_id','stage-'||st,'number',w,'reward',30+st*2+w*3,'entries',jsonb_build_array(jsonb_build_object('enemy','enemy-'||(1+mod(st+w-2,s.enemy_count)),'count',5+st+w,'interval',0.65))) ORDER BY st,w) FROM generate_series(1,s.stage_count) st CROSS JOIN generate_series(1,5) w),
  'towers',(SELECT jsonb_agg(jsonb_build_object('id','tower-'||n,'name',CASE s.slug WHEN 'office-guardians' THEN 'Office Unit ' WHEN 'cyber-fortress' THEN 'Security Control ' ELSE 'AI Component ' END||n,'cost',70+n*15,'damage',16+n*4,'range',120+n*5,'fire_rate',0.55+n*0.06,'damage_type',CASE mod(n,3) WHEN 0 THEN 'true' WHEN 1 THEN 'physical' ELSE 'magic' END) ORDER BY n) FROM generate_series(1,s.tower_count) n),
  'enemies',(SELECT jsonb_agg(jsonb_build_object('id','enemy-'||n,'name',CASE s.slug WHEN 'office-guardians' THEN 'Work Hazard ' WHEN 'cyber-fortress' THEN 'Cyber Threat ' ELSE 'AI Risk ' END||n,'hp',50+n*32,'speed',35+mod(n,5)*8,'armor',round((mod(n,4)*0.08)::numeric,2),'reward',7+n*2,'health_damage',1+CASE WHEN n=s.enemy_count THEN 1 ELSE 0 END,'traits',CASE WHEN mod(n,3)=0 THEN jsonb_build_array('armored') ELSE '[]'::jsonb END) ORDER BY n) FROM generate_series(1,s.enemy_count) n),
  'bosses',(SELECT jsonb_agg(jsonb_build_object('id','boss-'||n,'name',CASE s.slug WHEN 'office-guardians' THEN 'Company Crisis ' WHEN 'cyber-fortress' THEN 'Major Incident ' ELSE 'AI Failure ' END||n,'hp',1800+n*900,'speed',20+n*2,'armor',0.3,'reward',180+n*50,'health_damage',8+n,'traits',jsonb_build_array('boss')) ORDER BY n) FROM generate_series(1,s.boss_count) n),
  'heroes',(SELECT jsonb_agg(jsonb_build_object('id','hero-'||n,'name',CASE WHEN s.slug='ai-nexus-defense' THEN 'Agent ' ELSE 'Guardian ' END||n,'role',CASE mod(n,3) WHEN 0 THEN 'support' WHEN 1 THEN 'tank' ELSE 'ranged' END,'hp',400+n*100,'damage',25+n*8,'unlock_stage',GREATEST(1,n*2-1)) ORDER BY n) FROM generate_series(1,s.hero_count) n),
  'skills',(SELECT jsonb_agg(jsonb_build_object('id','skill-'||n,'name','Active Skill '||n,'cooldown',20+n*8,'effect',CASE n WHEN 1 THEN 'area_damage' WHEN 2 THEN 'reinforcement' ELSE 'freeze' END) ORDER BY n) FROM generate_series(1,3) n),
  'resource_rules',CASE WHEN s.slug='ai-nexus-defense' THEN '{"compute_start":1000,"token_start":1000,"trust_start":100,"latency_max":100,"wave_compute_cost":5,"wave_token_cost":7,"escaped_trust_cost":4,"escaped_latency_cost":3}'::jsonb ELSE '{}'::jsonb END,
  'model_profiles',CASE WHEN s.slug='ai-nexus-defense' THEN '[{"id":"small","name":"Small Model","tower_id":"tower-5","compute_cost":5,"token_cost":4,"latency_cost":2,"accuracy":68,"damage_multiplier":0.8},{"id":"medium","name":"Medium Model","tower_id":"tower-5","compute_cost":10,"token_cost":8,"latency_cost":4,"accuracy":78,"damage_multiplier":1},{"id":"large","name":"Large Model","tower_id":"tower-5","compute_cost":22,"token_cost":18,"latency_cost":8,"accuracy":88,"damage_multiplier":1.35},{"id":"reasoning","name":"Reasoning Model","tower_id":"tower-9","compute_cost":30,"token_cost":26,"latency_cost":12,"accuracy":94,"damage_multiplier":1.6},{"id":"vision","name":"Vision Model","tower_id":"tower-9","compute_cost":24,"token_cost":15,"latency_cost":10,"accuracy":90,"damage_multiplier":1.45}]'::jsonb ELSE '[]'::jsonb END,
  'events',COALESCE((SELECT jsonb_agg(jsonb_build_object('id','event-'||n,'stage_id','stage-'||(1+mod(n-1,s.stage_count)),'trigger','wave-'||(1+mod(n-1,5)),'education_id','question-'||(1+mod(n-1,s.question_count)),'reward',jsonb_build_object('resource',100),'penalty',jsonb_build_object('resource',50)) ORDER BY n) FROM generate_series(1,s.event_count) n),'[]'::jsonb),
  'education',COALESCE((SELECT jsonb_agg(jsonb_build_object('id','question-'||n,'topic',CASE mod(n,5) WHEN 0 THEN 'governance' WHEN 1 THEN 'recognition' WHEN 2 THEN 'response' WHEN 3 THEN 'protection' ELSE 'operations' END,'question',CASE s.slug WHEN 'cyber-fortress' THEN '보안 상황 ' ELSE 'AI 운영 상황 ' END||n||'에서 승인된 대응을 선택하십시오.','answers',jsonb_build_array(jsonb_build_object('id','A','text','선택 A'),jsonb_build_object('id','B','text','선택 B'),jsonb_build_object('id','C','text','선택 C')),'correct_answer_id',(ARRAY['A','B','C'])[1+mod(n-1,3)],'score',100,'explanation','게시 전 Content Studio 검증이 필요한 초기 중립 템플릿입니다.') ORDER BY n) FROM generate_series(1,s.question_count) n),'[]'::jsonb),
  'balance',jsonb_build_object('difficulties',jsonb_build_object('casual',jsonb_build_object('difficulty_bonus',0,'enemy_hp',0.82,'enemy_speed',0.92,'gold',1.18,'score',0.8),'normal',jsonb_build_object('difficulty_bonus',5000,'enemy_hp',1.0,'enemy_speed',1.0,'gold',1.0,'score',1.0),'veteran',jsonb_build_object('difficulty_bonus',10000,'enemy_hp',1.38,'enemy_speed',1.12,'gold',0.9,'score',1.5)),'health_score_factor',1000,'resource_score_factor',10,'wave_score_factor',500,'clear_time_target_ms',900000,'clear_time_bonus_divisor',100,'min_wave_duration_ms',3000,'duration_tolerance_ms',5000),
  'campaigns',jsonb_build_array(jsonb_build_object('id','core-campaign','name',CASE s.slug WHEN 'office-guardians' THEN 'Company City' WHEN 'cyber-fortress' THEN '2026 보안교육' ELSE 'Enterprise AI' END,'stage_ids',(SELECT jsonb_agg('stage-'||n ORDER BY n) FROM generate_series(1,s.stage_count) n),'required_learning_score',CASE WHEN s.education_enabled THEN 70 ELSE 0 END))
 ) AS content
 FROM specs s
)
INSERT INTO defense_content_versions(game_id,version_no,label,status,content_version,policy_version,asset_version,checksum,notes,content,published_at)
SELECT g.id,1,'v0.3.0','published','0.3.0',p.policy_version,'procedural-1',encode(digest(convert_to(p.content::text,'UTF8'),'sha256'),'hex'),'Defense Series canonical v0.3.0 content pack',p.content,now()
FROM packs p JOIN games g ON g.slug=p.slug
ON CONFLICT(game_id,version_no) DO NOTHING;

-- Replace mechanical seed labels with the canonical, domain-specific roster.
-- The web client must execute this published pack verbatim for ranked sessions;
-- any bundled fallback content is practice-only.
WITH roster(slug,stage_names,tower_ids,tower_names,enemy_ids,enemy_names,boss_ids,boss_names,hero_ids,hero_names,policy_version) AS (VALUES
 ('office-guardians',
  ARRAY['신규 서비스 오픈','트래픽 폭주','DB 장애','배포 장애','API 공격','데이터 오류','레거시 시스템','Company Crisis'],
  ARRAY['developer','dba','security','infra','ai_engineer','operations'],ARRAY['개발자','DBA','보안 담당자','인프라 담당자','AI 엔지니어','운영 담당자'],
  ARRAY['bug','traffic_monster','legacy_beast','data_corruptor','bot','shadow_user','incident','deadline','dependency_breaker','alert_storm'],ARRAY['Bug','Traffic Monster','Legacy Beast','Data Corruptor','Bot','Shadow User','Incident','Deadline','Dependency Breaker','Alert Storm'],
  ARRAY['outage_overlord','crisis_core'],ARRAY['Outage Overlord','Company Crisis Core'],
  ARRAY['architect','security_master','operations_master'],ARRAY['Architect','Security Master','Operations Master'],'office-policy-2026.08'),
 ('cyber-fortress',
  ARRAY['Phishing Attack','Password Siege','Malware Outbreak','Web Breach','DDoS Storm','Insider Shadow','Data Leakage','Supply Chain','Zero Day','Critical Incident'],
  ARRAY['firewall','waf','ids','ips','edr','mfa','dlp','soc'],ARRAY['Firewall','WAF','IDS','IPS','EDR','MFA','DLP','SOC'],
  ARRAY['phishing','malware','ransomware','sql_injection','xss','credential_stuffing','ddos','insider','zero_day','supply_chain','data_exfiltration','botnet','privilege_escalation','session_hijack','cloud_misconfig'],ARRAY['Phishing','Malware','Ransomware','SQL Injection','XSS','Credential Stuffing','DDoS','Insider Threat','Zero Day','Supply Chain Attack','Data Exfiltration','Botnet','Privilege Escalation','Session Hijack','Cloud Misconfiguration'],
  ARRAY['ransom_lord','zero_day_phantom','apt_commander'],ARRAY['Ransom Lord','Zero Day Phantom','APT Commander'],
  ARRAY['incident_commander','threat_hunter','forensic_lead'],ARRAY['Incident Commander','Threat Hunter','Forensic Lead'],'security-policy-2026.08'),
 ('ai-nexus-defense',
  ARRAY['LLM Basics','RAG Pipeline','Agent Network','AI Security','Model Routing','Responsible AI','Enterprise AI','Context Crisis','Trust Recovery','AI Incident'],
  ARRAY['ai_gateway','prompt_guard','rag','vector_search','model_router','guardrail','evaluator','cache','agent_node','human_review'],ARRAY['AI Gateway','Prompt Guard','RAG','Vector Search','Model Router','Guardrail','Evaluator','Cache','Agent','Human Review'],
  ARRAY['hallucination','prompt_injection','bad_context','data_poisoning','token_monster','latency_beast','model_drift','rogue_agent','context_overflow','sensitive_leak','tool_abuse','bias_wraith','retrieval_noise','cost_spike','shadow_model'],ARRAY['Hallucination','Prompt Injection','Bad Context','Data Poisoning','Token Monster','Latency Beast','Model Drift','Rogue Agent','Context Overflow','Sensitive Data Leak','Tool Abuse','Bias Wraith','Retrieval Noise','Cost Spike','Shadow Model'],
  ARRAY['hallucination_king','injection_master','token_hydra','rogue_overseer'],ARRAY['Hallucination King','Prompt Injection Master','Token Hydra','Rogue Agent Overseer'],
  ARRAY['research_agent','security_agent','coding_agent','data_agent','supervisor_agent'],ARRAY['Research Agent','Security Agent','Coding Agent','Data Agent','Supervisor Agent'],'ai-policy-2026.08')
), education_roster(slug,contexts,prompts,preferred_actions,explanations,topics,policy_reference) AS (VALUES
 ('cyber-fortress',
  ARRAY['이메일 수신 중','원격근무 중','서비스 운영 중','협력사 작업 중','사고 대응 훈련 중'],
  ARRAY['출처가 불분명한 첨부파일이 도착했습니다. 어떻게 해야 합니까?','여러 서비스에서 같은 비밀번호를 사용 중입니다. 가장 안전한 조치는?','PC에서 랜섬웨어 의심 파일 암호화가 시작됐습니다. 첫 대응은?','웹 요청에서 SQL Injection 징후가 탐지됐습니다. 어떻게 대응합니까?','브라우저 입력에서 XSS 페이로드가 발견됐습니다. 무엇을 확인해야 합니까?','짧은 시간에 다수 계정 로그인 실패가 발생했습니다. 우선 조치는?','트래픽이 급증해 DDoS가 의심됩니다. 가장 적절한 대응은?','정상 권한 사용자가 대량 파일을 반출합니다. 어떻게 처리합니까?','알려지지 않은 취약점 악용 정황이 있습니다. 무엇을 우선합니까?','업데이트된 협력사 패키지에서 이상 통신이 보입니다. 안전한 조치는?'],
  ARRAY['실행하지 않고 보안 신고 채널로 전달한다','고유한 비밀번호로 변경하고 MFA를 활성화한다','단말을 네트워크에서 격리하고 보안팀에 신고한다','WAF 차단 규칙을 적용하고 취약 쿼리를 수정한다','출력 인코딩과 입력 검증을 적용하고 영향 범위를 조사한다','로그인 시도를 제한하고 MFA·이상 로그를 점검한다','트래픽 우회·제한 정책을 적용하고 공격 원본을 분석한다','DLP로 전송을 차단하고 권한·행위 로그를 조사한다','영향 시스템을 격리하고 탐지 규칙과 패치를 긴급 검토한다','배포를 중단하고 서명·SBOM·공급 경로를 검증한다'],
  ARRAY['의심 파일은 실행 전에 격리하고 공식 신고 절차로 분석해야 합니다.','비밀번호 재사용을 없애고 MFA를 적용하면 계정 탈취 위험을 크게 줄입니다.','랜섬웨어 확산을 막기 위해 즉시 격리한 뒤 증거를 보존해야 합니다.','WAF 임시 차단과 서버 측 파라미터 바인딩을 함께 적용해야 합니다.','컨텍스트별 출력 인코딩과 입력 검증이 XSS 방어의 기본입니다.','속도 제한과 MFA, 로그인 이상 탐지를 함께 적용해야 합니다.','가용성을 먼저 보호하면서 네트워크·애플리케이션 계층을 함께 분석해야 합니다.','정상 계정이라도 비정상 대량 반출은 차단 후 최소 권한 관점에서 조사해야 합니다.','미확인 공격은 격리·관찰·보완 통제를 병행해 피해 범위를 제한해야 합니다.','공급망 변경은 신뢰하지 말고 출처·서명·구성요소를 독립적으로 검증해야 합니다.'],
  ARRAY['phishing_awareness','account_security','malware_response','web_security','web_security','account_security','availability','data_protection','incident_response','supply_chain'],'SEC-POL-2026.08'),
 ('ai-nexus-defense',
  ARRAY['고객 상담 Agent에서','사내 지식검색에서','코딩 Agent에서','문서 자동화에서','AI 운영 점검 중'],
  ARRAY['모델이 근거 없는 답변을 확신 있게 생성했습니다. 우선 조치는?','사용자 입력이 시스템 지시를 무시하라고 요구합니다. 어떻게 대응합니까?','검색된 문서가 질문과 무관하거나 오래됐습니다. 무엇을 개선합니까?','학습·검색 데이터에 조작된 문서가 섞였습니다. 안전한 조치는?','한 요청이 과도한 토큰과 컨텍스트를 소비합니다. 어떻게 제어합니까?','정확하지만 응답 지연이 SLA를 넘습니다. 가장 적절한 대응은?','배포 후 모델 품질이 지속적으로 낮아집니다. 무엇을 해야 합니까?','Agent가 승인되지 않은 도구를 호출하려 합니다. 어떻게 처리합니까?','민감정보가 모델 응답에 포함될 가능성이 있습니다. 우선 통제는?','대형 모델만 사용해 비용이 급증했습니다. 어떤 전략이 적절합니까?'],
  ARRAY['근거를 표시하고 Evaluator·Human Review를 적용한다','Prompt Guard로 지시 계층을 보호하고 도구 권한을 제한한다','문서 신선도·관련도 필터와 재랭킹을 적용한다','오염 데이터를 격리하고 출처·무결성을 재검증한다','입력 한도·요약·예산 정책을 적용한다','Model Router와 Cache를 사용하고 병목을 측정한다','기준 평가셋으로 회귀를 확인하고 롤백 또는 재검증한다','도구 호출을 차단하고 승인 게이트와 최소 권한을 적용한다','출력 Guardrail과 DLP를 적용하고 민감 필드를 마스킹한다','작업 난이도에 따라 소형·대형 모델을 라우팅한다'],
  ARRAY['근거·평가·사람 검토를 결합해야 환각이 업무 결과로 확산되는 것을 막습니다.','외부 입력은 신뢰하지 않고 시스템 지시와 도구 권한을 분리해야 합니다.','RAG 품질은 관련성뿐 아니라 신선도와 출처 신뢰도를 함께 관리해야 합니다.','데이터 오염은 격리 후 provenance와 무결성을 확인해야 재유입을 막습니다.','토큰·컨텍스트 예산을 요청 단위로 제한하고 초과 시 안전하게 중단해야 합니다.','지연 문제는 무조건 모델을 줄이기보다 라우팅·캐시·측정으로 최적화해야 합니다.','모델 변경은 고정 평가셋과 운영 지표로 감시하고 안전한 롤백 경로를 가져야 합니다.','Agent 도구는 최소 권한과 명시적 승인으로 실제 시스템 영향을 제한해야 합니다.','민감정보는 입력·검색·출력 모든 경로에서 분류하고 차단해야 합니다.','작업별 모델 라우팅은 품질을 유지하면서 비용과 지연을 줄입니다.'],
  ARRAY['llm_quality','ai_security','rag_quality','data_governance','cost_management','latency_management','model_governance','agent_governance','data_protection','model_routing'],'AI-POL-2026.08')
), semantic_packs AS (
 SELECT r.slug,r.policy_version,jsonb_build_object(
  'schema_version','0.3.0',
  'stages',(SELECT jsonb_agg(jsonb_build_object('id','stage-'||n,'number',n,'name',r.stage_names[n],'mode','campaign','theme',(ARRAY['verdant','void','ember','frost'])[1+mod(n-1,4)],'gimmick',CASE mod(n,4) WHEN 0 THEN 'winter_blessing' WHEN 2 THEN 'time_surge' WHEN 3 THEN 'ember_vents' ELSE '' END,'path','[{"x":-30,"y":360},{"x":180,"y":360},{"x":310,"y":130},{"x":530,"y":130},{"x":650,"y":500},{"x":880,"y":500},{"x":1020,"y":260},{"x":1310,"y":260}]'::jsonb,'tower_spots',jsonb_build_array(jsonb_build_object('id','stage-'||n||'-spot-1','x',140,'y',240),jsonb_build_object('id','stage-'||n||'-spot-2','x',300,'y',280),jsonb_build_object('id','stage-'||n||'-spot-3','x',430,'y',450),jsonb_build_object('id','stage-'||n||'-spot-4','x',570,'y',250),jsonb_build_object('id','stage-'||n||'-spot-5','x',700,'y',420),jsonb_build_object('id','stage-'||n||'-spot-6','x',850,'y',620),jsonb_build_object('id','stage-'||n||'-spot-7','x',970,'y',430),jsonb_build_object('id','stage-'||n||'-spot-8','x',1130,'y',180)),'starting_health',20,'starting_resource',300+(n-1)*18,'version','3.'||n||'.0') ORDER BY n) FROM generate_subscripts(r.stage_names,1) n),
  'waves',(SELECT jsonb_agg(jsonb_build_object('id','stage-'||st||'-wave-'||w,'stage_id','stage-'||st,'number',w,'reward',30+st*5+w*2,'entries',jsonb_build_array(jsonb_build_object('enemy',r.enemy_ids[1+mod(st+w-2,array_length(r.enemy_ids,1))],'count',4+st+w,'interval',GREATEST(0.42,0.9-st*0.025)),jsonb_build_object('enemy',r.enemy_ids[1+mod(st*2+w+2,array_length(r.enemy_ids,1))],'count',2+floor((st+w)/3.0)::int,'interval',1.05)) || CASE WHEN w=8 AND st>array_length(r.stage_names,1)-array_length(r.boss_ids,1) THEN jsonb_build_array(jsonb_build_object('enemy',r.boss_ids[st-(array_length(r.stage_names,1)-array_length(r.boss_ids,1))],'count',1,'interval',1.5)) ELSE '[]'::jsonb END) ORDER BY st,w) FROM generate_subscripts(r.stage_names,1) st CROSS JOIN generate_series(1,8) w),
  'towers',(SELECT jsonb_agg(jsonb_build_object('id',r.tower_ids[n],'name',r.tower_names[n],'role',CASE mod(n-1,3) WHEN 0 THEN 'rapid response' WHEN 1 THEN 'control' ELSE 'area defense' END,'color',6571775+mod(n*137123,9000000),'cost',70+(n-1)*12,'damage',17+(n-1)*5,'range',132+(n-1)*5,'fire_rate',0.52+mod(n-1,3)*0.25,'projectile_speed',360+n*8,'damage_type',CASE mod(n-1,3) WHEN 0 THEN 'physical' WHEN 1 THEN 'magic' ELSE 'true' END,'effective_against',CASE r.slug WHEN 'office-guardians' THEN (ARRAY['["software","dependency"]','["data","legacy"]','["identity","automation"]','["traffic","availability"]','["automation","alert"]','["availability","schedule"]'])[n]::jsonb WHEN 'cyber-fortress' THEN (ARRAY['["network"]','["web"]','["unknown","network"]','["network","web"]','["malware"]','["account"]','["data"]','["unknown","social"]'])[n]::jsonb ELSE (ARRAY['["token","latency"]','["injection"]','["hallucination","context"]','["context"]','["token","latency"]','["injection","sensitive_data"]','["hallucination","model"]','["token","latency"]','["rogue_agent"]','["rogue_agent","sensitive_data"]'])[n]::jsonb END,'effective_multiplier',1.6,'branches',jsonb_build_array(jsonb_build_object('id',r.tower_ids[n]||'_precision','name',r.tower_names[n]||' 정밀화','description','핵심 대상 피해와 사거리를 강화합니다.','damage_multiplier',1.75,'range_multiplier',1.2),jsonb_build_object('id',r.tower_ids[n]||'_network','name',r.tower_names[n]||' 연계망','description','빠른 연계와 범위 대응을 강화합니다.','rate_multiplier',0.64,'damage_multiplier',1.25))) ORDER BY n) FROM generate_subscripts(r.tower_ids,1) n),
  'enemies',(SELECT jsonb_agg(jsonb_build_object('id',r.enemy_ids[n],'name',r.enemy_names[n],'hp',52+(n-1)*24,'speed',38+mod(n-1,5)*8,'armor',round((mod(n-1,4)*0.07)::numeric,2),'radius',11+mod(n-1,7),'reward',9+(n-1)*3,'health_damage',1+floor((n-1)/6.0)::int,'threat_type',CASE r.slug WHEN 'office-guardians' THEN (ARRAY['software','traffic','legacy','data','automation','identity','availability','schedule','dependency','alert'])[n] WHEN 'cyber-fortress' THEN (ARRAY['social','malware','malware','web','web','account','network','data','unknown','supply','data','network','account','account','cloud'])[n] ELSE (ARRAY['hallucination','injection','context','data','token','latency','model','rogue_agent','token','sensitive_data','rogue_agent','data','context','token','model'])[n] END,'resource_effect',CASE WHEN r.slug='ai-nexus-defense' THEN (ARRAY['{"trust":6}','{"trust":8}','{"trust":4}','{"trust":9}','{"token":25}','{"latency":12}','{"trust":5}','{"trust":10,"compute":8}','{"token":20,"compute":10}','{"trust":15}','{"compute":10,"trust":6}','{"trust":8}','{"token":8,"latency":4}','{"compute":25,"token":12}','{"compute":18,"trust":4}'])[n]::jsonb ELSE '{}'::jsonb END,'traits',CASE WHEN n=1 THEN '[]'::jsonb ELSE jsonb_build_array((ARRAY['swift','armored','regenerating','healer','flying','phasing','siege','stealth','magic_resist','berserk'])[1+mod(n-2,10)]) END) ORDER BY n) FROM generate_subscripts(r.enemy_ids,1) n),
  'bosses',(SELECT jsonb_agg(jsonb_build_object('id',r.boss_ids[n],'name',r.boss_names[n],'hp',2100+(n-1)*600,'speed',22+n,'armor',0.32,'radius',31+n,'reward',240+n*50,'health_damage',8+n,'threat_type',CASE r.slug WHEN 'office-guardians' THEN (ARRAY['availability','software'])[n] WHEN 'cyber-fortress' THEN (ARRAY['malware','unknown','supply'])[n] ELSE (ARRAY['hallucination','injection','token','rogue_agent'])[n] END,'resource_effect',CASE WHEN r.slug='ai-nexus-defense' THEN (ARRAY['{"trust":24}','{"trust":30,"compute":12}','{"token":60,"compute":25}','{"trust":35,"compute":30}'])[n]::jsonb ELSE '{}'::jsonb END,'traits',jsonb_build_array('boss')) ORDER BY n) FROM generate_subscripts(r.boss_ids,1) n),
  'heroes',(SELECT jsonb_agg(jsonb_build_object('id',r.hero_ids[n],'name',r.hero_names[n],'title',r.hero_names[n]||' 전문가','role',CASE mod(n-1,3) WHEN 0 THEN 'support' WHEN 1 THEN 'tank' ELSE 'ranged' END,'color',7266406+mod(n*248719,9000000),'hp',500+(n-1)*95,'damage',30+(n-1)*7,'range',CASE WHEN mod(n,2)=0 THEN 54 ELSE 118 END,'speed',125+n*4,'respawn_seconds',9+n,'skill1',r.hero_names[n]||' 분석','skill2',r.hero_names[n]||' 대응','ultimate',r.hero_names[n]||' 총력전','unlock_stage',GREATEST(1,n*2-1)) ORDER BY n) FROM generate_subscripts(r.hero_ids,1) n),
  'skills',jsonb_build_array(jsonb_build_object('id','emergency_response','name','긴급 대응','description','선택 지점의 위협을 집중 제거합니다.','cooldown',38,'color','#ff8b5e','effect','area_damage'),jsonb_build_object('id','reinforcement','name','대응팀 투입','description','길목에 임시 대응팀을 배치합니다.','cooldown',28,'color','#72e0a6','effect','reinforcement'),jsonb_build_object('id','flow_control','name','흐름 제어','description','모든 위협의 진행을 잠시 늦춥니다.','cooldown',44,'color','#91a7ff','effect','freeze')),
  'resource_rules',CASE WHEN r.slug='ai-nexus-defense' THEN '{"compute_start":1000,"token_start":1000,"trust_start":100,"latency_max":100,"wave_compute_cost":5,"wave_token_cost":7,"escaped_trust_cost":4,"escaped_latency_cost":3}'::jsonb ELSE '{}'::jsonb END,
  'model_profiles',CASE WHEN r.slug='ai-nexus-defense' THEN jsonb_build_array(jsonb_build_object('id','small','name','Small Model','tower_id','model_router','compute_cost',5,'token_cost',4,'latency_cost',2,'accuracy',68,'damage_multiplier',0.8),jsonb_build_object('id','medium','name','Medium Model','tower_id','model_router','compute_cost',10,'token_cost',8,'latency_cost',4,'accuracy',78,'damage_multiplier',1.0),jsonb_build_object('id','large','name','Large Model','tower_id','model_router','compute_cost',22,'token_cost',18,'latency_cost',8,'accuracy',88,'damage_multiplier',1.35),jsonb_build_object('id','reasoning','name','Reasoning Model','tower_id','agent_node','compute_cost',30,'token_cost',26,'latency_cost',12,'accuracy',94,'damage_multiplier',1.6),jsonb_build_object('id','vision','name','Vision Model','tower_id','agent_node','compute_cost',24,'token_cost',15,'latency_cost',10,'accuracy',90,'damage_multiplier',1.45)) ELSE '[]'::jsonb END,
  'events',CASE WHEN er.slug IS NULL THEN '[]'::jsonb ELSE (SELECT jsonb_agg(jsonb_build_object('id',r.slug||'-event-'||lpad(n::text,2,'0'),'stage_id','stage-'||(1+mod(n-1,array_length(r.stage_names,1))),'trigger','wave-'||(1+mod(n-1,8)),'education_id',r.slug||'-question-'||lpad(n::text,2,'0'),'reward',CASE WHEN r.slug='ai-nexus-defense' THEN jsonb_build_object('resource',100,'trust',2,'latency_headroom',2) ELSE jsonb_build_object('resource',100) END,'penalty',CASE WHEN r.slug='ai-nexus-defense' THEN jsonb_build_object('resource',50,'trust',5,'latency_headroom',5) ELSE jsonb_build_object('resource',50) END) ORDER BY n) FROM generate_series(1,30)n) END,
  'education',CASE WHEN er.slug IS NULL THEN '[]'::jsonb ELSE (SELECT jsonb_agg(jsonb_build_object('id',r.slug||'-question-'||lpad(((c-1)*10+q)::text,2,'0'),'topic',er.topics[q],'question',er.contexts[c]||' '||er.prompts[q],'answers',(SELECT jsonb_agg(jsonb_build_object('id',(ARRAY['A','B','C'])[a],'text',CASE WHEN a=1+mod((c-1)*10+q-1,3) THEN er.preferred_actions[q] WHEN a=1+mod((c-1)*10+q,3) THEN '검증 없이 그대로 진행한다' ELSE '판단을 미루고 아무 조치도 하지 않는다' END) ORDER BY a) FROM generate_series(1,3)a),'correct_answer_id',(ARRAY['A','B','C'])[1+mod((c-1)*10+q-1,3)],'score',100,'explanation',er.explanations[q],'policy_reference',er.policy_reference) ORDER BY c,q) FROM generate_series(1,5)c CROSS JOIN generate_series(1,10)q) END,
  'balance',jsonb_build_object('difficulties',jsonb_build_object('casual',jsonb_build_object('difficulty_bonus',0,'enemy_hp',0.82,'enemy_speed',0.92,'gold',1.18,'score',0.8),'normal',jsonb_build_object('difficulty_bonus',5000,'enemy_hp',1.0,'enemy_speed',1.0,'gold',1.0,'score',1.0),'veteran',jsonb_build_object('difficulty_bonus',10000,'enemy_hp',1.38,'enemy_speed',1.12,'gold',0.9,'score',1.5)),'health_score_factor',1000,'resource_score_factor',10,'wave_score_factor',500,'clear_time_target_ms',900000,'clear_time_bonus_divisor',100,'min_wave_duration_ms',3000,'duration_tolerance_ms',5000,'tower_upgrade_cost',jsonb_build_array(0,70,120),'sell_refund_rate',0.65,'resource_state_limits',CASE WHEN r.slug='ai-nexus-defense' THEN jsonb_build_object('compute',1000,'token',1000,'trust',100,'latency',100) ELSE '{}'::jsonb END,'ai_resource_score_factors',CASE WHEN r.slug='ai-nexus-defense' THEN jsonb_build_object('compute',2,'token',1,'trust',100,'latency',20) ELSE '{}'::jsonb END),
  'campaigns',jsonb_build_array(jsonb_build_object('id','core-campaign','name',CASE r.slug WHEN 'office-guardians' THEN 'Company City' WHEN 'cyber-fortress' THEN '2026 보안교육' ELSE 'Enterprise AI' END,'stage_ids',(SELECT jsonb_agg('stage-'||n ORDER BY n) FROM generate_subscripts(r.stage_names,1)n),'required_learning_score',CASE WHEN er.slug IS NULL THEN 0 ELSE 70 END))
 ) AS content
 FROM roster r LEFT JOIN education_roster er ON er.slug=r.slug
)
UPDATE defense_content_versions v
SET content=p.content,policy_version=p.policy_version,checksum=encode(digest(convert_to(p.content::text,'UTF8'),'sha256'),'hex'),updated_at=now()
FROM semantic_packs p JOIN games g ON g.slug=p.slug
WHERE v.game_id=g.id AND v.version_no=1;

INSERT INTO achievements(game_id,code,name,description,criteria,xp,active)
SELECT g.id,g.slug||'-first-defense','첫 방어',g.name||' 첫 전투를 완료했습니다.','{"server_rule":"first_verified_result"}'::jsonb,100,true
FROM games g WHERE g.slug IN ('office-guardians','cyber-fortress','ai-nexus-defense')
ON CONFLICT(code) DO NOTHING;

INSERT INTO achievements(code,name,description,criteria,xp,active) VALUES
 ('defender','Defender','Defense Series 첫 검증 전투를 완료했습니다.','{"server_rule":"first_verified_defense"}'::jsonb,100,true),
 ('triple-guardian','Triple Guardian','세 Defense Series 게임에서 검증 전투를 완료했습니다.','{"server_rule":"all_defense_games_played"}'::jsonb,300,true),
 ('security-guardian','Security Guardian','Cyber Fortress 교육 캠페인을 완료했습니다.','{"server_rule":"cyber_campaign_complete"}'::jsonb,400,true),
 ('ai-guardian','AI Guardian','AI Nexus Defense 교육 캠페인을 완료했습니다.','{"server_rule":"ai_campaign_complete"}'::jsonb,400,true),
 ('defense-master','Defense Master','세 Defense Series 캠페인을 모두 완료했습니다.','{"server_rule":"all_defense_campaigns_complete"}'::jsonb,700,true)
ON CONFLICT(code) DO NOTHING;

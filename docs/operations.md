# 운영 및 장애 대응

## 일상 점검

```bash
docker compose ps
docker compose logs --since=30m igame
curl --fail --max-time 5 http://127.0.0.1:8080/healthz
curl --fail --max-time 5 http://127.0.0.1:8080/readyz
```

`/healthz`는 프로세스 감시에, `/readyz`는 로드밸런서 readiness에 사용합니다. 비정상 상태에서 무조건 재시작하기 전에 PostgreSQL 연결 수, 저장 공간, DNS, 인증서와 시간 동기화를 확인합니다. 암호, 토큰, API 키, OIDC code와 전체 프롬프트를 로그에 남기지 않고, reverse proxy access log의 correlation ID로 요청을 추적하는 것이 운영 원칙입니다.

권장 경보는 readiness 3회 연속 실패, HTTP 5xx 비율, p95 지연, DB 연결 실패, 디스크 80%, 인증 실패 급증, 점수 이상 탐지와 AI provider 오류율입니다.

감사 로그, 사용자, 게임 목록은 `/admin`에서 페이지 단위로 조회합니다. 응답은 현재 페이지와 함께 필터 적용 후 전체 건수를 돌려주므로 목록이 조용히 잘리지 않습니다. 감사 로그와 사용자 목록은 검색어로 좁힐 수 있고(감사: 수행자·작업·대상·IP), 페이지당 25~200건을 선택합니다. 감사 로그는 현재 검색 조건 그대로 CSV로 내보낼 수 있습니다. 화면의 페이지가 아니라 조건에 맞는 전체 기록을 스트리밍하므로 기록이 많으면 시간이 걸립니다. Excel이 한국어를 바르게 읽도록 UTF-8 BOM을 붙이고, 수식으로 해석될 수 있는 값(`=`, `+`, `-`, `@`로 시작)은 앞에 작은따옴표를 넣어 무력화합니다. 내보내기 실행도 `audit.export` 항목으로 남습니다.

`/admin` 대시보드 하단의 **서비스 상태** 카드가 설치 상태를 한곳에 모읍니다. DB 도달 여부·응답 지연·커넥션 pool, 사내 SSO/관리자 로그인/승인 흐름/AI/플레이 정책의 on-off, 게임별 현재 게시 콘텐츠 버전, 그리고 감사 로그·telemetry·세션·점수의 누적 행 수를 표시합니다. 최종 runtime image에는 shell이 없어 컨테이너 내부를 직접 볼 수 없으므로 이 화면이 1차 진단 지점입니다. 행 수는 `pg_class` 통계 기반 추정치이며 대시보드 조회가 대형 table을 순차 스캔하지 않도록 의도한 선택입니다.

## 관리자 설정

게시한 공지는 포털 홈에 최신 4건이 노출되고, 사용자는 `/notices`에서 게시된 전체 공지를 검색해 볼 수 있습니다. 휴가 등으로 자리를 비운 사용자가 지난 공지를 놓치지 않도록 별도 보관 화면을 제공합니다.

실행 시 필요한 네 환경변수 외의 설정은 모두 `/admin`에서 관리합니다. 요청 경로에서 읽는 설정 값은 최대 5초 동안 인스턴스 메모리에 캐시되고, 설정을 저장한 인스턴스에서는 즉시 무효화됩니다. 즉 저장한 화면에서는 바로, 다른 인스턴스에서는 최대 5초 뒤에 적용됩니다.

- 서비스: 표시명, 공개 URL, 허용 origin, Bootstrap 로그인, 플레이 정책 기준 timezone(기본 `Asia/Seoul`)
- 개인화(사용자별): 화면 모드(시스템/밝게/어둡게), 글자 크기 100~125%, 화면 전환 애니메이션. 브라우저에 저장되므로 서버 설정이 아니며, 사이트 데이터가 차단된 환경에서는 해당 세션에만 적용됩니다
- 인증: Keycloak issuer/client와 claim/role mapping
- AI: OpenAI-compatible base URL/model/key, timeout, 기본 streaming 호출, max token 상한(최대 262,144)
- 키: 역할별 scope, 개인 키 만료/회전/폐기, MCP/API 접근
- 워크플로: 게임 등록·변경에 대한 팀장/운영자 검토와 승인/반려
- 개인정보: 랭킹 실명/닉네임, 조직 공개, 사용자 랭킹 opt-out
- 플레이 정책: 허용 시간, 일일 제한, 게임별 예외
- 브라우저 연결 정책: iframe과 API 연결 허용 origin

일반 카탈로그 승인 워크플로가 꺼져 있으면 지원되는 게임 생성·변경 요청을 바로 반영합니다. 켜져 있을 때만 pending 요청을 만들고, reviewer 역할 또는 `manager_required` 설정에 따라 팀장/관리자가 승인·반려합니다. 기본적으로 요청자는 자기 요청을 검토할 수 없습니다. 일반 workflow, RealmGuard Designer와 Defense Content Studio는 manager 자신의 team이 없으면 review 목록을 거부하고 비어 있지 않은 동일 team 작성자의 항목만 보여 줍니다. 직접 preview/review가 있는 경로는 manager와 작성자 어느 한쪽의 team이 없으면 `team_required`, 서로 다르면 `different_team`으로 fail-closed합니다. 반려 사유는 필수이며 모든 전이는 감사 로그에 남깁니다. 일반 카탈로그의 직접 변경과 승인된 workflow 적용 모두 내장 RealmGuard와 세 Defense game의 예약 slug 전환을 거부합니다.

## AI 운영

브라우저는 공급자 API를 직접 호출하지 않습니다. 서버가 자격 증명을 복호화해 호출하며 기본 응답은 SSE streaming입니다. `max_tokens`는 최대 262,144이며 요청이 관리자 설정 상한을 넘으면 거부합니다. 공급자 자체 상한은 관리자가 그보다 작게 설정해야 합니다. 연결 해제 시 upstream 요청을 취소하며 서버가 AI 요청을 자동 재시도하지는 않습니다.

AI가 비활성 또는 미설정이면 AI 게임과 메뉴는 숨기고 일반 게임 기능은 정상 운영합니다.

## RealmGuard 운영

RealmGuard는 `/games/realmguard`에서 실행하고 runtime은 현재 `published` 콘텐츠만 사용합니다. 브라우저는 config를 읽은 뒤 그 `version.id`를 세션 metadata의 `realmguard_version_id`로 pin합니다. 게시 race로 UUID가 stale이면 서버가 `409 realmguard_config_stale`로 시작을 막고 UI가 최신 config를 다시 읽으므로, 이 오류를 연습 모드로 조용히 우회하지 않습니다. 게시 전에는 Designer의 Stages, Waves, Enemies, Bosses, Towers, Heroes, Skills, Balance, Versions, Telemetry 탭에서 참조 무결성과 운영 지표를 확인하고, Test 단계에서 campaign 최소 10개·endless 최소 1개, stage별 8~15 waves, wave당 entry 8개·기본 spawn 500 상한, path/tower spot, 1~32자 ID grammar, 일반 enemy 10~16종과 boss 2~4종 및 최악 조건 histogram의 4 KiB budget을 검증합니다. Endless는 10,000 wave까지 확장한 기본·파생 최대 spawn counter가 signed 32-bit 안에 드는지도 확인합니다. 난이도별 시작금, 전투 중 처치 기반 hero level, 18/10 lives의 star 경계와 결과 제출을 representative stage에서 직접 시험합니다.

승인 정책을 사용하지 않아도 draft를 Test로 전환한 뒤 admin/operator가 게시합니다. 정책을 사용하면 test 후 게시 요청, 팀장/관리자 검토, 관리자 최종 게시 순서가 적용됩니다. RealmGuard manager에게 team이 없으면 pending 목록을 거부하고 목록에는 비어 있지 않은 동일 team 작성자의 항목만 표시합니다. Preview/review는 manager 또는 작성자의 team이 빠져도 fail-closed합니다. 작성자와 검토자의 분리 및 이 team 제한을 지키고, 반려할 때는 수정 근거가 되는 comment를 남깁니다. 게시 직전과 직후에 version tuple/checksum, 작성자·검토자, 기준 telemetry를 감사 기록과 change ticket에 남깁니다. 미게시 version의 preview는 `practice_only`이므로 공식 기록 검증에는 사용하지 않습니다.

Designer section 조회 응답의 `ETag`/checksum을 저장 요청의 `If-Match`에 넣습니다. `428 precondition_required`면 client가 checksum을 보내지 않은 것이고, `409 stale_version`이면 다른 변경이 먼저 저장된 것이므로 자동 덮어쓰지 말고 최신 section을 다시 읽어 차이를 병합합니다. 정상 저장 응답의 새 ETag를 다음 편집에 사용합니다.

새 게시물은 이후 시작한 세션에만 적용됩니다. 진행 중 세션은 시작 시 고정된 snapshot으로 계속 검증되므로 balance 변경 중 강제로 종료하지 않습니다. 이상이 있으면 이전 이미지만 되돌리지 말고 알려진 정상 콘텐츠를 새 draft로 복구해 같은 절차로 게시하거나, 해당 이미지·DB backup 쌍을 함께 복원합니다. 전용 telemetry는 run/unique user/평균 score·duration/최고 wave/승리·거부, 실패 wave와 stage·hero·난이도 breakdown을 제공하며 `realmguard.tower.build`/`realmguard.skill.cast` event를 tower·skill 사용량으로 집계합니다.

공식 결과 전에는 UUID `client_event_id`와 1-based 연속 `sequence`로 ready, wave start/complete와 battle complete를 전송합니다. 완료 snapshot의 적별 defeated/escaped/spawned histogram과 전투 누적값은 뒤로 갈 수 없습니다. 같은 event 재전송은 전체 payload가 같을 때만 idempotent하며 sequence conflict는 누락 event부터 다시 보내야 합니다. Event `data` 4 KiB 한도를 지키고 고빈도 frame/position event는 보내지 않습니다. 선택 event 전체는 128개, ready/complete는 각 1개, wave start/complete는 각각 10,001/10,000개, tower build/upgrade/sell은 합계 10,000개까지 받습니다. 필수 class별 용량은 독립적으로 예약되며 해당 class가 차면 `telemetry_limit`을 반환합니다. `server_received_telemetry_v1`은 이 브라우저 자가보고 원장의 서버 수신·순서·시각·누적 일관성 검증이며 완전한 서버 시뮬레이션은 아닙니다. 운영 분석과 이상 탐지는 이 보장 경계를 기준으로 해석합니다.

## Defense Series 운영

Office Guardians, Cyber Fortress와 AI Nexus Defense는 `/games/{slug}`에서 실행하고 현재 `published` content pack만 공식 session에 사용합니다. 브라우저는 config의 `version.id`를 session metadata의 `defense_content_version_id`로 pin합니다. 누락 또는 stale UUID를 연습 mode나 일반 score API로 우회하지 않습니다.

게시 전 `/admin/defense`에서 게임과 section을 선택해 schema/reference 검증, Test와 연습 preview를 완료합니다. Cyber/AI와 교육을 추가한 Office pack은 교육 event의 모든 답안, 정답, topic과 reward/penalty 연결도 함께 확인합니다. 새 Draft의 `policy_version`을 실제 적용 정책과 맞추고, 롤백은 과거 UUID를 `source_version_id`로 복제한 새 Draft에서 수행합니다. 과거 published row를 직접 재활성화하지 않습니다. Draft 저장은 GET 응답의 최신 checksum을 `If-Match`로 사용하고 충돌 시 최신 section을 다시 읽어 병합합니다. 승인 정책이 꺼져 있으면 Test 후 바로 게시하고, 켜져 있으면 같은 team 검토·승인 또는 반려 뒤 게시합니다.

새 published UUID는 이후 생성하는 session에만 적용됩니다. 이미 진행 중인 session은 pin한 snapshot으로 완료합니다. 콘텐츠 회귀 시 이전 이미지만 되돌리지 말고 알려진 정상 콘텐츠를 새 Draft로 복구해 게시하거나, 호환되는 이미지와 DB backup을 함께 복원합니다.

Cyber/AI 운영자는 게임 지표와 학습 지표를 구분합니다. `telemetry`는 현재 published UUID의 실행·완료·`average_game_score` 등 게임 운영에, `learning-report`는 같은 UUID/policy의 참여·정답률·topic 취약 영역에 사용합니다. 호환용 `average_score`는 telemetry의 `average_game_score`와 같은 값입니다. Learning report의 완료율은 모든 campaign 완료 사용자 비율이고 전투 승률은 `battle_clear_rate`로 별도 표시합니다. 개인 원시 답안과 부서 통계는 개인정보 설정과 승인된 교육 보존 정책을 적용합니다. 자세한 검증 순서는 [Defense Series 운영 가이드](defense-series.md)를 따릅니다.

## 무중단에 가까운 업그레이드

1. 릴리스 checksum/SBOM 검토와 스테이징 시험을 완료합니다.
2. [백업 절차](backup-restore.md)에 따라 DB와 `/app/data`를 백업합니다.
3. 새 `igame:vX.Y.Z` 이미지를 `docker load`합니다.
4. compose 파일의 정확한 이미지 태그를 변경합니다. `latest`는 사용하지 않습니다.
5. 유지보수 창에서 `docker compose up -d --no-deps igame`을 실행합니다.
6. `/readyz`, 로그인, 버전, 게임 실행, 점수 제출, 관리자 화면을 확인합니다.

DB 마이그레이션은 전진 적용을 기본으로 합니다. 이전 이미지가 새 schema와 호환된다는 릴리스 노트가 없으면 이미지 태그만 되돌리지 말고 검증된 DB 백업을 함께 복구합니다.

## 장애별 확인

| 증상 | 우선 확인 |
| --- | --- |
| `/healthz` 실패 | 프로세스 종료, OOM, 포트 충돌, 필수 env 형식 |
| `/readyz`만 실패 | PostgreSQL DNS/TLS/권한/연결 수, 마이그레이션 오류 |
| SSO redirect loop | public URL, 프록시 forwarded headers, redirect URI, cookie secure 설정 |
| 로그인만 실패하고 화면은 열림 (`csrf_rejected`) | 상태 변경 요청은 브라우저 `Origin`이 **설정된 `public_url`이거나 요청이 실제로 도착한 주소**여야 합니다. 둘 다 아니면 거부합니다. 서버 로그의 `request origin rejected` 항목이 받은 `origin`과 허용된 목록을 함께 남기므로 대조합니다. reverse proxy 뒤에서 TLS를 종료한다면 `trust_proxy`를 켜고 프록시가 원래 `Host`와 `X-Forwarded-Proto`/`X-Forwarded-Host`를 전달해야 합니다. 그렇지 않으면 서비스는 자신이 평문 HTTP로 보인다고 판단합니다 |
| 토큰 검증 실패 | issuer/audience, JWKS 접근, 시계 오차, Keycloak key rotation |
| AI 응답 중단 | provider 접근, timeout, SSE buffering, 모델 token 상한 |
| 게임 iframe 차단 | allowlist, CSP frame-src, 게임의 frame-ancestors/X-Frame-Options |
| 점수 미반영 | session token/소유권, 같은 세션의 중복 점수, 게임별 점수·시간 규칙 |
| RealmGuard `version_mismatch` | 세션 시작 후 게시 변경 여부, 결과의 content/balance/asset 및 stage 객체 version |
| RealmGuard `realmguard_version_required`/`realmguard_config_stale` | config의 `version.id`를 session metadata에 보냈는지, 게시 변경 후 config를 다시 읽었는지 |
| RealmGuard `invalid_gold` | 난이도 시작금 multiplier, kill/wave/조기 호출 보상과 build/upgrade/sell 합계 |
| RealmGuard `hero_level_mismatch` | 계정 level이 아닌 해당 전투의 처치 기반 hero level 제출 여부 |
| RealmGuard `telemetry_sequence_conflict` | 세션별 sequence가 1부터 연속인지, 재시도 queue에서 누락·중복 순서가 없는지 |
| RealmGuard `telemetry_attestation_failed` | ready/wave/battle milestone, receipt 시간, 누적 histogram과 tower/economy 원장이 최종 결과와 일치하는지 |
| Designer `precondition_required`/`stale_version` | GET의 최신 ETag를 `If-Match`로 보냈는지, 충돌 편집을 다시 읽고 병합했는지 |
| Manager `team_required`/`different_team` | manager와 작성자 양쪽 team claim이 비어 있지 않고 정확히 같은지 |
| Defense `defense_version_required`/`defense_config_stale` | 해당 slug config의 `version.id`를 session metadata에 보냈는지, 게시 변경 후 config를 다시 읽었는지 |
| Defense 결과 `409` | session의 game slug·owner·`defense_content_version_id`가 전용 결과 경로와 일치하는지, 일반 score 경로를 사용하지 않았는지 |
| 교육 답안 거부 | session과 pinned version에 event/answer가 존재하는지, Cyber/AI slug를 혼용하지 않았는지 |
| Studio `precondition_required`/`stale_version` | 최신 section checksum을 `If-Match`로 보냈는지, 동시 편집 충돌을 병합했는지 |

이미지 자체 healthcheck는 shell command가 아니라 `/app/igame healthcheck`를 실행합니다. 이 command는 설정과 DB 연결을 새로 만들지 않고 실행 중인 `127.0.0.1:8080/healthz`만 확인하므로, 컨테이너 내부에서 `sh`, `curl`, `wget`를 사용한 진단은 지원하지 않습니다. 상세 진단은 호스트의 `docker inspect`, `docker logs`와 외부 `/readyz` 요청을 사용합니다.

## 프록시

정적 bundle 자산은 content hash가 붙은 `/assets/<name>-<hash>.<ext>`만 `immutable`로 1년 캐시하고 나머지 파일과 `index.html`은 매번 재검증하므로, 업그레이드한 이미지가 워크스테이션의 오래된 자산을 계속 제공하지 않습니다.

TLS 프록시는 원래 `Host`, `X-Forwarded-Proto: https`, 신뢰할 수 있는 client address를 전달해야 합니다. SSE 경로는 proxy buffering을 끄고 idle timeout을 AI 최대 응답 시간보다 길게 둡니다. 업로드 본문과 WebSocket을 허용할 경우 경로별 한도와 timeout을 별도로 설정합니다.

## 용량 계획과 보존

초기 권장은 2 vCPU, RAM 2 GiB에서 시작하고 실제 DAU와 AI 동시 stream을 기준으로 조정하는 것입니다. 현재 내장된 자동 보존/삭제 job은 없으므로 감사 로그, telemetry와 게임 세션의 보존 기간을 조직 정책으로 정하고 승인된 DB 유지보수 절차를 마련합니다. 점수/시즌 확정 기록과 승인 기록의 법적·조직 정책을 우선하고 삭제 실행 이력도 별도로 보존합니다.

# REST 및 streaming API

기본 경로는 `/api/v1`이며 JSON을 사용합니다. 공개 endpoint는 health, readiness, version, 공개 설정과 OIDC 시작/callback입니다. 브라우저는 보안 session cookie, 자동화는 `Authorization: Bearer <personal-api-key>`를 사용합니다. 개인 키의 scope와 만료/폐기 상태를 매 요청 확인합니다.

## 공통 규칙

- 시간은 UTC RFC 3339, ID는 UUID, 점수는 signed 64-bit integer입니다.
- 목록은 `limit`(기본 50, 최대 200)과 0부터 시작하는 `offset`을 사용합니다.
- 변경 요청은 `Content-Type: application/json`을 요구합니다.
- 알 수 없는 필드, 범위를 벗어난 수치, 허용되지 않은 상태 전이는 400/409로 거부합니다.

오류 형식:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "요청 값을 확인하세요."
  }
}
```

## 주요 endpoint

| Method | Path | Scope/설명 |
| --- | --- | --- |
| GET | `/api/v1/version` | 공개 build version/commit/date |
| GET | `/api/v1/games` | `games:read`, 공개 게임 검색 |
| GET | `/api/v1/games/{gameId}` | `games:read`, 게임 metadata |
| POST | `/api/v1/games/{gameId}/sessions` | `sessions:write`, server session/token 생성 |
| POST | `/api/v1/sessions/{sessionId}/finish` | `sessions:write`, 결과 종료 |
| POST | `/api/v1/scores` | `scores:write`, 검증 가능한 점수 제출 |
| POST | `/api/v1/telemetry` | `sessions:write`, 세션 token으로 SDK event 제출; RealmGuard와 Defense Series는 UUID/순서 원장 계약 적용 |
| GET | `/api/v1/rankings` | `rankings:read`, 기간/팀/부서 랭킹 |
| GET | `/api/v1/rankings/{gameId}` | `rankings:read`, 게임별 랭킹 |
| GET | `/api/v1/achievements` | 로그인, 업적 목록 |
| POST | `/api/v1/me/achievements` | `scores:write`, client-unlockable 업적 해제 |
| GET | `/api/v1/me` | `profile:read`, 본인 프로필 |
| PATCH | `/api/v1/me` | `profile:write`, 본인 개인정보/공개 설정 |
| GET/PUT | `/api/v1/me/preferences` | `profile:read` 조회 / `profile:write` 변경 |
| PUT | `/api/v1/me/password` | interactive session 전용, 로컬 비밀번호 변경 |
| GET | `/api/v1/me/history` | 본인 플레이 기록 |
| GET/POST | `/api/v1/me/api-keys` | 개인 키 목록/생성; 원문은 생성 응답 1회 |
| PATCH/DELETE | `/api/v1/me/api-keys/{id}` | 개인 키 변경/폐기 |
| POST | `/api/v1/me/api-keys/{id}/rotate` | 개인 키 즉시 회전; 새 원문은 응답 1회 |
| GET | `/api/v1/events` | 공개 가능한 이벤트 |
| GET | `/api/v1/events/{eventId}` | 이벤트 상세와 본인 참가 상태 |
| POST/DELETE | `/api/v1/games/{gameId}/favorite` | `profile:write`, 즐겨찾기 추가/해제 |
| POST | `/api/v1/events/{eventId}/join` | `profile:write`, 이벤트 참가 |
| GET | `/api/v1/seasons` | 시즌 목록 |
| GET | `/api/v1/notices` | 게시된 공지 목록 |
| GET | `/api/v1/banners` | 현재 노출 가능한 배너 목록 |
| POST | `/api/v1/ai/chat/completions` | AI 게임용 기본 streaming proxy |
| GET | `/api/v1/realmguard/config` | `games:read`, 현재 게시된 RealmGuard 실행 설정 |
| GET | `/api/v1/realmguard/version` | `games:read`, 게시 version tuple/checksum |
| GET | `/api/v1/realmguard/progress` | `profile:read`, 본인 진행도 조회 |
| PUT | `/api/v1/realmguard/progress` | `profile:write`, 본인 loadout·개인 설정 변경 |
| POST | `/api/v1/realmguard/results` | `scores:write`, 세션에 고정된 설정으로 전투 결과 검증·완료 |
| GET | `/api/v1/realmguard/rankings` | `rankings:read`, RealmGuard 전용 필터 랭킹 |
| GET | `/api/v1/defense/{slug}/{config,version}` | `games:read`, 게시된 Defense 콘텐츠와 version |
| GET | `/api/v1/defense/{slug}/{progress,learning}` | `profile:read`, 게임 진행도와 개인 학습 결과 |
| POST | `/api/v1/defense/{slug}/results` | `scores:write`, session/version에 고정된 공식 결과 |
| GET | `/api/v1/defense/{slug}/rankings` | `rankings:read`, Defense 게임별 공식 랭킹 |
| POST | `/api/v1/defense/{slug}/education/events/{eventID}/answer` | `scores:write`, session에 속한 교육 선택 제출 |
| GET | `/api/v1/admin/dashboard` | 관리자/운영자 session 또는 `admin:*`, 운영 요약 |
| GET | `/api/v1/admin/analytics` | 관리자/운영자 session 또는 `admin:*`, DAU/WAU/MAU 등 |
| GET | `/api/v1/admin/settings` | admin session 또는 admin 역할 + `admin:*` 키, 전체 설정 조회 |
| GET/PUT | `/api/v1/admin/settings/{key}` | admin session 또는 admin 역할 + `admin:*` 키, 일반 설정 조회/변경 |
| GET/PUT | `/api/v1/admin/oidc` | admin session 또는 admin 역할 + `admin:*` 키, OIDC 설정 |
| GET/PUT | `/api/v1/admin/ai` | admin session 또는 admin 역할 + `admin:*` 키, AI 설정 |
| GET/POST/PUT/DELETE | `/api/v1/admin/{games,categories,seasons,events,achievements}` | 카탈로그와 참여 콘텐츠 관리 |
| GET/POST/PUT/DELETE | `/api/v1/admin/{tournaments,rewards,notices,banners}` | 운영 콘텐츠 관리 |
| GET/PUT/DELETE | `/api/v1/admin/rankings[/{id}]` | 점수 검토·제외 |
| GET/POST | `/api/v1/admin/realmguard/versions` | admin/operator session; 개인 키는 동일 role + `admin:*`, 목록/초안 생성 |
| GET/PUT/POST/DELETE | `/api/v1/admin/realmguard/drafts/{section}[/*]` | admin/operator session; 개인 키는 동일 role + `admin:*`, section/item 편집 |
| POST | `/api/v1/admin/realmguard/versions/{id}/{test,publish}` | admin/operator session; 개인 키는 동일 role + `admin:*`; 승인 사용 시 최종 publish는 admin |
| GET | `/api/v1/admin/realmguard/telemetry` | admin/operator session; 개인 키는 동일 role + `admin:*`, `days=1..365` 집계 |
| GET | `/api/v1/realmguard/versions/pending` | manager/admin session; 개인 키는 동일 role + `admin:*`, 검토 대기 version |
| GET | `/api/v1/realmguard/versions/{id}/preview` | manager/operator/admin session; 개인 키는 동일 role + `admin:*`, 비공개 연습 설정 |
| POST | `/api/v1/realmguard/versions/{id}/review` | manager/admin session; 개인 키는 동일 role + `admin:*`, 승인/반려 (`approve` alias) |
| GET/POST | `/api/v1/admin/defense/{slug}/versions` | admin/operator session; Defense version 목록/초안 생성 |
| GET/PUT | `/api/v1/admin/defense/{slug}/drafts/{section}` | admin/operator session; checksum/`If-Match` 기반 section 편집 |
| POST | `/api/v1/admin/defense/{slug}/versions/{id}/{test,publish}` | admin/operator session; Test 및 승인 정책에 따른 게시 요청 |
| GET | `/api/v1/admin/defense/{slug}/{telemetry,learning-report}` | admin/operator session; 게임 운영·교육 집계 |
| GET | `/api/v1/defense/versions/pending` | manager/admin session; 동일 team 검토 대기 version |
| GET | `/api/v1/defense/{slug}/versions/{id}/preview` | manager/operator/admin session; 기록 없는 연습 설정 |
| POST | `/api/v1/defense/versions/{id}/review` | manager/admin session; 승인/반려 |
| GET | `/api/v1/admin/status` | admin/operator, 설치 상태 조회: 서비스 버전·timezone·공개 URL, DB 도달 여부·지연·pool 사용량, 정책 5종 on/off, 게임별 게시 콘텐츠 버전, 증가형 table의 통계 기반 행 수 추정 |
| GET | `/api/v1/admin/audit` | admin session 또는 admin 역할 + `admin:*` 키, 감사 조회. `limit`(기본 50, 최대 200)·`offset`·`q`를 받으며 응답에 필터 적용 후 전체 건수 `total`을 함께 반환합니다. `q`는 수행자 아이디·작업·대상 유형·대상 ID·IP를 부분 일치로 검색합니다. `format=csv`를 붙이면 같은 `q` 조건의 **전체 기록**을 UTF-8 BOM CSV로 스트리밍하며(`limit`/`offset` 무시), 내보내기 자체도 `audit.export`로 감사에 남습니다 |

OIDC client secret과 AI API key는 write-only입니다. 설정 조회 응답은 원문 대신 `client_secret_configured` 또는 `api_key_configured` 상태를 반환합니다.

개인 키 생성·변경·회전·폐기와 로컬 비밀번호 변경은 로그인된 브라우저 session에서만 허용되며, 개인 API 키로 키나 비밀번호를 관리할 수 없습니다. 관리자 API를 개인 키로 호출하려면 관리자 역할과 `admin:*` scope가 모두 필요합니다. 기존에 발급된 키도 매 요청 시 저장 scope와 현재 전역 허용 목록·현재 역할 정책의 교집합만 유효하므로, 관리자가 권한을 제거하거나 사용자 역할을 바꾸면 즉시 축소 적용됩니다. `profile:write`는 `profile:read`와 별도이며 기존 키의 저장 scope에 자동 추가되지 않습니다. 개인 변경 API가 필요한 사용자는 관리자가 역할 정책에 허용한 뒤 브라우저 개인화 페이지에서 명시적으로 scope를 추가하거나 새 키를 발급합니다.

로컬 비밀번호 변경 요청은 `current_password`와 12자 이상의 다른 `new_password`를 받습니다. 성공하면 현재 session을 제외한 해당 사용자의 다른 session을 폐기합니다. OIDC 전용 사용자처럼 로컬 password hash가 없는 계정에는 적용되지 않습니다.

## 세션과 점수 예

```http
POST /api/v1/games/snake/sessions HTTP/1.1
Authorization: Bearer igk_...
Content-Type: application/json

{"metadata":{"client_version":"1.2.0"}}
```

서버는 session ID와 추측하기 어려운 session token을 돌려줍니다. 점수 제출은 단독 숫자가 아니라 해당 session ID와 token을 포함합니다.

```http
POST /api/v1/scores HTTP/1.1
Authorization: Bearer igk_...
Content-Type: application/json

{
  "game_id":"snake",
  "session_id":"...",
  "score":3250,
  "session_token":"igs_...",
  "metadata":{"level":7}
}
```

정상 저장은 201, 게임에 설정된 점수·플레이 시간 규칙 위반은 422, 같은 세션의 중복 제출은 409를 반환합니다.

RealmGuard는 일반 점수·랭킹 endpoint를 사용하지 않습니다. `/api/v1/scores`에 RealmGuard 세션을 보내면 `409 authoritative_result_required`, `/api/v1/rankings/realmguard` 또는 `/api/v1/rankings?game_id=realmguard`는 `409 realmguard_ranking_required`를 반환합니다. 공식 랭킹은 `/api/v1/realmguard/rankings`를 사용합니다.

Defense Series도 일반 점수·랭킹 endpoint를 사용하지 않습니다. 세 game session은 `defense_content_version_id`로 published snapshot을 pin하고 `/api/v1/defense/{slug}/results`와 `/api/v1/defense/{slug}/rankings`만 사용합니다. 다른 slug의 전용 경로, 일반 `/api/v1/scores`, `/api/v1/rankings/{gameId}`를 이용한 우회는 각각 `409 defense_authoritative_result_required` 또는 `409 defense_ranking_required`로 거부합니다.

## Defense Series runtime

지원 slug는 `office-guardians`, `cyber-fortress`, `ai-nexus-defense`입니다. 먼저 `GET /api/v1/defense/{slug}/config`를 읽고 응답 `version.id`를 session metadata에 넣습니다.

```http
POST /api/v1/games/cyber-fortress/sessions HTTP/1.1
Authorization: Bearer igk_...
Content-Type: application/json

{"metadata":{"client":"gamehub-js","client_version":"0.6.1","defense_content_version_id":"9ea33ec1-39a7-4e65-ad57-ae11a6b2790f"}}
```

pin이 없으면 `428 defense_version_required`, UUID가 현재 slug의 published snapshot이 아니면 `409 defense_config_stale`입니다. 성공 응답의 `session.defense_content_version_id`는 요청 UUID와 정확히 같아야 합니다. 게시 race가 발생한 client는 config를 다시 읽고 새 session을 생성합니다. 이전 UUID를 새 session에 묵시적으로 대입하지 않습니다.

공식 결과 전에는 UUID `client_event_id`와 session별 1부터 연속인 `sequence`로 Defense battle ready, wave start/complete와 battle complete를 전송합니다. 완료 milestone에는 적별 defeated/escaped/spawned histogram과 health/resource/earned/spent/sold 누적값이 포함되며 뒤로 감소할 수 없습니다. 서버는 session/slug/version, 수신 시각, stage 시작 health/resource, wave별 spawn/reward budget과 이 원장을 검증한 뒤 score와 star를 다시 계산합니다. body만 보낸 perfect 결과, 조작된 zero-wave 패배, 누락·역전·변조 원장, 불가능한 승리·spawn·경제·duration과 다른 slug/session 조합은 거부합니다.

Defense 원장 `data`는 실제 직렬화된 JSON 기준 최대 4 KiB입니다. 초과하면 `400 invalid_telemetry`이며 Studio Test/Publish도 stage별 최악 누적 `battle.complete` 표본이 같은 한도를 넘는 content pack을 `422 content_validation_failed`로 거부합니다. Class 한도는 선택 event 전체 128, battle ready/complete 각 1, wave start 101, wave complete 100, server-validated answer를 반영하는 education apply 500, tower build/upgrade/sell 합계 10,000입니다. 선택 class가 차더라도 필수 class의 예약 용량은 유지됩니다. 동일 UUID/payload 재전송은 `202 duplicate:true`, 같은 UUID의 다른 payload와 sequence 누락은 `409`, class 포화는 `429`입니다.

Defense 성공 결과의 `verification_method`와 `attestation.method`는 `server_received_telemetry_v1`이고 digest는 서버가 수신한 canonical 원장에서 계산됩니다. 이는 browser event의 서버 수신·순서·시각·누적 일관성 검증이며, 서버가 전투를 독립적으로 재실행하는 replay 증명은 아닙니다. RealmGuard가 `server_replay_v1`로 옮겨간 것과 달리 Defense Series는 AI 자원 모델과 교육 결과가 결합돼 있어 이번 릴리스에서는 이 계약을 유지합니다. `duration_ms`는 두 게임 모두 시뮬레이션 시간이며 세션 경과 시간의 최대 2배까지 허용합니다.

Cyber Fortress와 AI Nexus Defense는 전투 중 published education event를 선택합니다. public config/preview에는 중립적인 `A`/`B`/`C` answer ID만 있고 `correct_answer_id`, `correct`, `explanation`은 없습니다. 정답 위치는 문제 전체에서 분산되며 browser bundle에도 mapping을 넣지 않습니다. 답 요청은 session ID/token과 answer ID를 포함하며 서버는 session owner, slug, pinned version과 event/answer 참조, 해당 wave 도달 원장을 확인합니다. 성공한 답 응답에서만 정답 여부와 해설을 반환합니다. 개인 `GET /learning`은 topic별 attempts/correct 집계를, 관리자 `learning-report`는 권한과 개인정보 설정을 적용한 참여·정답률 집계를 반환합니다. 게임 점수와 학습 점수는 별도 지표입니다.

AI Nexus Defense config는 `small`, `medium`, `large`, `reasoning`, `vision` 다섯 `model_profiles`와 `resource_rules`를 제공합니다. profile은 실제 tower ID와 Compute/Token/Latency 비용, 정확도, 피해 배수를 연결합니다. 통과한 적은 자신의 `resource_effect`에 더해 `escaped_trust_cost`와 `escaped_latency_cost`를 누적합니다. 비용은 0에서 포화되므로 음수 잔액이나 이월 debt가 없고, AI 결과의 `resource_state`는 `compute`, `token`, `trust`, `latency` 각각의 `start`, `spent`, `remaining`과 `remaining = start - spent`를 만족해야 합니다. 네 지표 중 하나가 0이면 패배이며 승리 결과는 모두 양수여야 합니다. Office/Cyber 결과에는 이 필드를 보낼 수 없습니다.

`GET /api/v1/defense/{slug}/versions/{id}/preview`는 `practice_only:true`인 complete config를 반환합니다. Preview session이나 미게시 UUID로 공식 result, progress, ranking 또는 학습 완료를 생성할 수 없습니다.

## Defense Content Studio와 검토

Studio section은 `stages`, `waves`, `towers`, `enemies`, `bosses`, `heroes`, `skills`, `events`, `education`, `balance`, `campaigns`, `resource_rules`, `model_profiles`입니다. AI 전용 두 section도 같은 version/checksum 경계에서 편집·검증됩니다. Office의 기본 seed에는 교육이 없지만 `events`와 `education`을 함께 유효하게 구성해 게시하면 교육 선택과 learning/report가 활성화됩니다. Draft 생성 요청의 `policy_version`은 새 정책 경계를 기록하며, 생략하면 source 값을 계승합니다. 과거 snapshot UUID를 `source_version_id`로 보내면 그 내용을 복제한 롤백 Draft를 만들고 Test·승인·Publish를 다시 거칩니다. Draft section GET의 `version.checksum`/`ETag`를 PUT의 `If-Match`로 보내야 합니다. 누락은 `428 precondition_required`, 오래된 checksum은 `409 stale_version`, 잘못된 형식은 `400 invalid_precondition`입니다.

초안은 `/test`를 통과해야 합니다. 승인 정책이 꺼져 있으면 tested version을 바로 publish하고, 켜져 있으면 publish 요청이 `202`와 `pending_approval`을 반환합니다. `/api/v1/defense/versions/{id}/review`의 decision은 `approved` 또는 `rejected`이며 반려 comment는 필수입니다. Manager는 본인과 작성자의 team이 모두 비어 있지 않고 같을 때만 pending 목록, preview와 review를 사용할 수 있습니다. Preview는 연습 전용이고 검토 중 공식 데이터를 만들지 않습니다.

## RealmGuard runtime

게임 실행 전 `GET /api/v1/realmguard/config`를 읽고 응답 `version.id`를 세션 요청 metadata의 `realmguard_version_id`로 그대로 보냅니다. 세션 transaction은 그 UUID가 요청 시점에도 현재 `published`인지 확인한 뒤 같은 snapshot을 고정합니다. 값이 없으면 `428 realmguard_version_required`, UUID가 stale·unpublished·존재하지 않으면 `409 realmguard_config_stale`이며 클라이언트는 최신 config를 다시 읽어야 합니다.

```http
POST /api/v1/games/realmguard/sessions HTTP/1.1
Authorization: Bearer igk_...
Content-Type: application/json

{"metadata":{"client":"gamehub-js","client_version":"0.6.1","realmguard_version_id":"9ea33ec1-39a7-4e65-ad57-ae11a6b2790f"}}
```

성공 응답의 `session.realmguard_version_id`는 요청한 UUID와 같습니다. `GET /api/v1/realmguard/config`는 stage의 전체 path, tower spot, wave와 gimmick을 포함한 실행 설정을 반환합니다. `GET /api/v1/realmguard/version`은 게시 version의 `content_version`, `stage_version`, `balance_version`, `asset_version`과 checksum을 반환합니다. 전투 결과의 `stage_version`에는 이 전역 stage-content version이 아니라 선택한 stage 객체의 `version`을 제출합니다. 응답은 둘을 각각 `stage_version`, `stage_content_version`으로 구분합니다.

RealmGuard 전투 event는 일반 SDK event보다 강한 세션 원장 계약을 사용합니다. `client_event_id`는 요청 최상위의 유효한 UUID, `sequence`는 세션별 1부터 빈틈없이 증가하는 1~100,000 정수입니다. `data`는 event당 최대 4 KiB입니다. 같은 `client_event_id`·event·sequence·data의 재전송은 `202`와 `duplicate:true`로 같은 event를 재사용하며, 같은 ID의 다른 payload는 `409 telemetry_event_conflict`, 순서 누락·역전은 `409 telemetry_sequence_conflict`입니다.

한도는 필수 milestone이 선택 event에 밀려 사라지지 않도록 class별로 분리합니다.

| Event class | 세션당 수신 한도 |
| --- | ---: |
| 필수가 아닌 event 전체 | 128 |
| `realmguard.battle.ready` | 1 |
| `realmguard.battle.complete` | 1 |
| `realmguard.wave.start` | 10,001 |
| `realmguard.wave.complete` | 10,000 |
| `realmguard.tower.build` + `tower.upgrade` + `tower.sell` 합계 | 10,000 |

해당 class가 차면 `429 telemetry_limit`입니다. 한 class의 포화는 다른 필수 class의 예약 용량을 사용하지 않습니다.

```http
POST /api/v1/telemetry HTTP/1.1
Authorization: Bearer igk_...
Content-Type: application/json

{
  "game_id":"realmguard",
  "session_id":"...",
  "session_token":"igs_...",
  "event":"realmguard.battle.ready",
  "client_event_id":"8b184b42-bc41-49f6-aed4-227694307011",
  "sequence":1,
  "data":{"stage_id":"stage-1","difficulty":"normal","hero_id":"aerin"}
}
```

검증 가능한 전투에는 시작 직후 `realmguard.battle.ready`, 도달한 각 wave의 순서대로 `realmguard.wave.start`, 완료한 각 wave의 `realmguard.wave.complete`, 마지막 `realmguard.battle.complete`가 필요합니다. `wave.complete`는 lives/gold/earned/spent/sold/kills/escaped/spawned/hero level과 `defeated_by_enemy`, `escaped_by_enemy`, `spawned_by_enemy` 누적 histogram을 포함합니다. `battle.complete`는 이 최종 누적값, `waves`/`waves_completed`, hero, 승패와 네 version을 결과 요청과 정확히 맞춥니다. 서버는 수신 시각, 연속 sequence, wave 순서·최소 milestone 시간, 누적값 단조성, tower build/upgrade/sell 원장, 적별 spawn 예산·보상·life damage를 확인합니다.

공식 결과 요청은 session ID/token, stage/mode/difficulty, hero, 네 version 값과 전투 입력 원장 `ledger`를 포함합니다. `ledger`는 `{rules_version, config_digest, ticks, commands[]}` 형태이고 각 command는 `{tick, op, …}`입니다. `op`는 `wave`, `build`, `upgrade`, `sell`, `target`, `skill`, `meteor`, `reinforce`, `hero`, `economy`, `defeat`이며, 명령은 최대 6,000개, `ticks`는 최대 288,000입니다. 함께 보내는 duration, lives/gold, kills/escaped/spawned, completed waves와 histogram은 호환을 위해 유지되지만 서버는 이를 사용하지 않고 재현 결과로 덮어씁니다.

서버는 세션에 고정된 콘텐츠를 kernel 입력으로 투영해 `config_digest`를 다시 계산하고, 일치하면 원장을 처음부터 재생해 lives, gold, kills, escaped, spawned, 완료 wave, 전투 hero level, 승패와 적별 histogram을 직접 산출합니다. `duration_ms`는 시뮬레이션 시간(tick × 50ms)이며 세션 경과 시간의 최대 2배까지 허용합니다. 원장이 없으면 `422 missing_ledger`, 규칙 버전이 다르면 `409 ledger_rules_mismatch`, 투영 digest가 다르면 `409 content_projection_mismatch`, 명령 수·tick·순서가 범위를 벗어나면 `422 invalid_ledger`입니다.

이어서 score, star, 승리, 경제 예산과 영웅 XP를 재계산하고 RealmGuard result, 공통 score, session 종료, progress와 unlock을 하나의 transaction으로 반영합니다. 첫 성공은 `201`이며 응답의 `verification_method`는 `server_replay_v1`입니다. 이는 서버가 게시된 콘텐츠와 플레이어 입력만으로 전투를 다시 실행해 결과를 확정했다는 뜻입니다. 수정된 client는 자신의 입력을 바꿀 수는 있어도 그 입력의 결과를 바꿀 수 없습니다. 기존 telemetry 검증은 그 전투가 실제 이 session에서 진행됐음을 확인하는 두 번째 방어선으로 유지되며 attestation에 함께 기록됩니다. 요청의 호환용 `proof`와 `events`는 공식 증거로 사용하거나 저장하지 않고, 서버가 attestation digest를 포함한 암호화 receipt를 별도로 생성합니다. 재현에 사용한 원장은 `realmguard_results.ledger`에 보존됩니다.

같은 세션/token의 성공 결과 재전송은 저장된 공식 결과와 `idempotent:true`를 `200`으로 반환합니다. 다른 방식으로 이미 종료된 세션, 세션/token 또는 version 불일치는 `409`, 전투 수치 또는 필수 telemetry 불일치는 `422`입니다.

`PUT /api/v1/realmguard/progress`는 다음처럼 잠금 해제된 hero/skill loadout과 최대 64 KiB JSON object인 개인 설정만 변경합니다. stage, 난이도, star, score, hero level 또는 unlock을 포함하면 `400 authoritative_progress`입니다.

```json
{"hero_id":"aerin","skill_ids":["meteor"],"settings":{"camera_shake":true}}
```

RealmGuard 랭킹 query는 다음 값을 조합합니다.

| Query | 값/기본값 |
| --- | --- |
| `mode` | `campaign`(기본), `endless` |
| `difficulty` | 비우거나 `casual`, `normal`, `veteran` |
| `group` | `individual`(기본), `stage` alias, `department`, `hero` |
| `metric` | `score`(기본); `stars`는 부서 캠페인 랭킹에서만 지원 |
| `period` | `daily`, `weekly`, `monthly`, `season`, `all`/`all_time` |
| `stage_id`, `hero_id` | 선택 필터 |
| `limit` | 기본 50, 최대 200 |

랭킹은 현재 게시 콘텐츠 version, 검증·moderation된 결과, 랭킹 opt-out과 조직 공개 설정을 적용합니다. Campaign은 성공해 star를 받은 결과만 공식 랭킹에 들어가고 패배 기록은 progress/telemetry에만 반영됩니다. `stars`와 `individual|hero` 조합은 `400 metric_not_supported`이며 Endless mode는 star를 부여하지 않으므로 score metric을 사용합니다.

## RealmGuard Designer와 검토

Designer section은 `stages`, `waves`, `enemies`, `bosses`, `towers`, `heroes`, `skills`, `balance`입니다. Section 전체 조회·교체 경로는 `/api/v1/admin/realmguard/drafts/{section}`이고 `version_id` query로 draft를 지정할 수 있습니다. Array section은 하위 `/items` POST와 `/items/{itemID}` PUT/DELETE도 지원하지만 `balance`는 section 전체 PUT만 사용합니다. 모든 편집은 전체 문서 스키마와 참조를 검증하고 version 상태를 `draft`로 되돌립니다. 콘텐츠 ID는 소문자로 시작하고 이후 소문자·숫자·`_`·`-`만 사용하는 1~32자 값입니다. 일반 enemy는 10~16종, boss는 2~4종, wave 하나의 entry는 최대 8개이고 entry count 합계는 최대 500입니다. 모든 적 ID를 최대 누적값으로 넣은 `battle.complete` histogram의 실제 JSON이 4 KiB 이하여야 하며, endless 10,000 wave를 확장한 `BaseSpawns`와 splitting/boss 소환을 포함한 `MaxSpawns`도 각각 signed 32-bit 이하여야 Test를 통과합니다.

Draft section GET은 현재 SHA-256 checksum을 응답 `version.checksum`과 `ETag`에 제공합니다. Section PUT과 item POST/PUT/DELETE는 이 값을 `If-Match: "<checksum>"`으로 보내야 합니다. 헤더가 없으면 `428 precondition_required`, 다른 작업이 먼저 저장해 값이 오래되었으면 `409 stale_version`이며, 성공 응답의 새 checksum/ETag로 다음 변경을 이어갑니다. 형식이 잘못된 checksum은 `400 invalid_precondition`입니다.

```http
POST /api/v1/admin/realmguard/versions/{id}/test
POST /api/v1/admin/realmguard/versions/{id}/publish
```

초안을 만든 뒤 `/test`를 반드시 성공시켜 `testing` 상태로 전환합니다. 승인 정책이 꺼져 있으면 admin/operator가 이 tested version을 `200`으로 즉시 publish합니다. 켜져 있으면 publish 요청이 `202`와 `pending_approval`을 반환하고 manager/admin이 다음 endpoint에서 검토합니다. `decision`을 생략하면 `approved`입니다. 반려에는 comment가 필수이고 version은 comment/review 시각을 보존한 편집 가능한 `draft`로 돌아갑니다.

```http
POST /api/v1/realmguard/versions/{id}/review HTTP/1.1
Content-Type: application/json

{"decision":"rejected","comment":"veteran 시작금과 9번 stage wave 구성을 다시 조정하세요."}
```

승인되면 admin이 같은 version의 publish endpoint를 호출해 최종 게시합니다. `separation_of_duties` 자기 검토 금지를 서버가 강제합니다. Manager 자신의 team이 비어 있으면 pending 목록 조회가 `403 team_required`이고, 목록에는 team이 비어 있지 않으며 manager와 같은 team인 작성자의 version만 나타납니다. Manager preview/review는 manager와 작성자 어느 한쪽이라도 team이 없으면 `403 team_required`, 다르면 `403 different_team`으로 fail-closed합니다. `/review`의 `/approve` alias와 관리자 prefix alias도 같은 검토 handler를 사용하지만 route guard와 내부 승인 역할 검사를 모두 통과해야 합니다.

`GET /api/v1/realmguard/versions/{id}/preview`는 해당 version의 complete config envelope에 `practice_only:true`를 붙입니다. Manager preview에는 위의 same-team/fail-closed 조건을 적용합니다. 미게시 초안 검증용이며 이 응답으로 공식 score/progress를 제출할 수 없습니다.

## AI streaming

AI 설정은 관리자 화면에서 OpenAI-compatible base URL, 기본 model, API key, timeout과 최대 token을 저장합니다. 클라이언트는 공급자 key를 보내지 않습니다.

```http
POST /api/v1/ai/chat/completions HTTP/1.1
Authorization: Bearer igk_...
Accept: text/event-stream
Content-Type: application/json

{"model":"configured-alias","messages":[{"role":"user","content":"..."}],"max_tokens":262144,"stream":true}
```

`stream`을 생략하면 서버가 `true`로 설정하고 upstream의 SSE byte stream과 content type을 변형 없이 전달합니다. 따라서 delta와 종료 표시는 연결한 OpenAI-compatible 공급자 형식을 따릅니다. 연결이 끊기면 upstream context도 취소됩니다. `stream:false`는 일반 JSON 응답을 그대로 relay합니다. `max_tokens` 또는 `max_completion_tokens`가 관리자 상한이나 262,144를 넘으면 400으로 거부되며 자동 clamp하지 않습니다.

프록시가 있다면 이 endpoint의 buffering을 끄고 idle timeout을 충분히 늘립니다.

## 선택형 승인 API

관리자 승인 정책이 활성화된 경우 지원되는 게임 생성·변경을 `/api/v1/workflow/requests`로 제출하고 관리자는 `/api/v1/admin/workflow/requests/{id}/review`에서 승인 또는 반려합니다. 팀장은 `/api/v1/workflow/reviews`에서 검토 대상을 조회하고 `/api/v1/workflow/requests/{id}/review`에서 처리합니다. 정책이 비활성이면 별도 검토 상태를 만들지 않고 요청 payload를 바로 반영합니다. 정상 처리는 `pending → applied|rejected`이며 적용에 실패하면 다시 `pending`으로 남습니다. `separation_of_duties`는 기본적으로 자기 요청 검토를 막습니다. Manager 자신의 team이 비어 있으면 review 목록을 `403 team_required`로 거부하고, 목록에는 team이 비어 있지 않은 동일 team 요청만 포함합니다. 직접 review도 manager와 요청자 어느 한쪽의 team이 비면 `403 team_required`, 다르면 `403 different_team`입니다. 반려에는 비어 있지 않은 `comment`가 필요하며 처리 내역은 감사 로그에 기록됩니다. `realmguard`, `office-guardians`, `cyber-fortress`, `ai-nexus-defense`는 내장 authoritative runtime의 예약 slug입니다. 일반 게임 생성·변경으로 이 식별자를 만들거나 기존 내장 게임을 rename·disable·replace하려는 요청은 직접 관리자 API와 generic workflow 제출 단계에서 모두 `409 protected_game_identity`로 거부됩니다.

## 배포 경계의 요청 제한

운영 reverse proxy에서 인증, score, AI, 관리자 변경 경로에 각각 적절한 요청 속도와 본문 크기 제한을 적용합니다. 클라이언트는 429/5xx에 지수 backoff와 jitter를 적용하되 mutation을 무조건 자동 재시도하지 않습니다.

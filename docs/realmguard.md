# RealmGuard 운영 가이드

RealmGuard는 igame `v0.2.0`에 포함된 데이터 기반 tower defense 게임입니다. Phaser runtime, React HUD, Go API와 PostgreSQL 콘텐츠 저장소를 같은 서비스 경계에서 운영하며 브라우저가 공급자 key나 관리 권한을 가지지 않습니다.

## 독자 IP와 자산 경계

RealmGuard의 stage명, enemy, tower, hero, skill, 수치 데이터와 코드 생성 그래픽은 이 프로젝트를 위해 작성한 독자 콘텐츠입니다. Kingdom Rush를 포함한 제3자 게임의 코드, 서사, 캐릭터, map, sprite, 음원 또는 UI 자산을 복제하거나 포함하지 않습니다. 공식 lore를 별도로 가정하지 않으며 화면과 콘텐츠 데이터에 실제 존재하는 명칭만 운영 문서의 기준으로 삼습니다.

Phaser 3은 게임 loop와 Canvas/WebGL rendering에 사용하는 MIT 라이선스 framework입니다. production dependency는 lockfile에 고정되고 package metadata와 `LICENSE.md`가 이미지의 `/licenses/phaser`에 포함됩니다. stage 배경, path, tower/enemy/hero 표현과 effect는 bundle된 코드가 도형과 색상으로 생성하므로 원격 image, font, audio 또는 CDN 요청이 필요하지 않습니다. 외부 자산을 추가하려면 출처·라이선스·checksum을 검토하고 이미지 안에 고정한 뒤 SBOM 및 반입 검사를 다시 수행해야 합니다.

## 기본 콘텐츠

초기 내장 설정의 version tuple은 다음과 같습니다.

| 구분 | 기본값 | 의미 |
| --- | --- | --- |
| Content | `0.2.0` | stage, wave, enemy, tower, hero, skill 구조 |
| Balance | `2026.08.1` | 난이도 배율, 가격, 피해량, 성장 곡선 |
| Asset | `procedural-1` | 코드 생성 시각 자산 규격 |
| Stage | `2026.08.1` | path, tower spot와 wave 구성 |

내장 roster는 다음 범위입니다.

- Campaign stage 10개와 endless mode `끝없는 균열`
- 일반 enemy 10종과 boss 2종: armored, swift, flying, healer, splitting, regenerating, phasing, siege 등의 trait 조합
- Boss `공허왕 오르반`, `시간룡 세라크`; 체력 66%/33% 구간의 phase 전이에서 tower 비활성화, 하수인 소환 또는 가속 gimmick 수행
- Tower 4종: `태양첨탑`, `룬꽃 정원`, `석맥 포대`, 병사 소환·저지 계열 `바람수호 병영`; tower마다 두 upgrade branch
- Targeting `first`, `last`, `strong`, `weak`, `closest`
- Hero 3명: `에어린`, `브란`, `니라`; 각각 일반 능력 2개와 ultimate 1개, 체력 소진 후 hero별 respawn 시간 적용
- Active skill 3종: `별똥 낙하`, `수호대 소집`, `시간 서리`
- 난이도 `casual`, `normal`, `veteran`; mode `campaign`, `endless`
- Campaign 잔여 lives 18 이상은 3성, 10 이상은 2성, 그 아래 승리는 1성이며 endless와 패배는 별을 주지 않음
- `잿불 고개`, `서리결 골짜기`, `시간의 균열`에서 12초 주기로 발동하는 stage별 gimmick 적용

내장 설정은 fresh database와 장애 복구의 기준값입니다. Runtime API는 `published` 상태의 snapshot만 반환합니다. RealmGuard session 생성 metadata에는 브라우저가 렌더링한 config의 `version.id`를 `realmguard_version_id`로 보내야 하며, 서버는 그 UUID가 여전히 published인지 확인해 `game_sessions.realmguard_content_version_id`에 고정합니다. 누락은 `428 realmguard_version_required`, stale/unpublished UUID는 `409 realmguard_config_stale`로 거부해 config 조회와 세션 생성 사이의 게시 race를 닫습니다. 게시 후에도 이미 시작한 session은 고정된 content/stage/balance/asset version으로 검증되며, 각 version의 content SHA-256 checksum을 API와 감사 기록에서 확인할 수 있습니다.

## Runtime API

RealmGuard endpoint는 모두 로그인 세션 또는 해당 scope의 개인 API key를 요구합니다.

| Method | Path | 계약 |
| --- | --- | --- |
| `GET` | `/api/v1/realmguard/config` | `games:read`; 게시 snapshot과 실행 콘텐츠 |
| `GET` | `/api/v1/realmguard/version` | `games:read`; 현재 게시 version tuple/checksum |
| `GET` | `/api/v1/realmguard/progress` | `profile:read`; stage/hero/skill/loadout 진행도 |
| `PUT` | `/api/v1/realmguard/progress` | `profile:write`; 잠금 해제된 hero/skill의 loadout과 개인 설정만 변경 |
| `POST` | `/api/v1/telemetry` | `sessions:write`; session token, UUID와 연속 sequence로 전투 원장 제출 |
| `POST` | `/api/v1/realmguard/results` | `scores:write`; session token과 서버 수신 원장을 이용한 공식 결과 완료 |
| `GET` | `/api/v1/realmguard/rankings` | `rankings:read`; 개인/부서/hero 기준 랭킹 |

`PUT progress`는 stage, 별, score, hero level 또는 unlock 값을 받지 않습니다. 이 값들은 검증된 결과 transaction으로만 변경됩니다. 일반 `/api/v1/scores`로 RealmGuard score를 보내면 `409 authoritative_result_required`, 일반 `/api/v1/rankings` 경로로 RealmGuard를 조회하면 `409 realmguard_ranking_required`로 거부됩니다. 세부 request/response는 [REST API](api.md)를 참조하세요.

Campaign 공식 랭킹은 완주해 star를 받은 검증 결과만 포함합니다. 패배도 검증된 시도·플레이 시간으로 progress와 telemetry에 남지만 경쟁 랭킹을 밀어 올리지 않습니다.

## 콘텐츠 Designer

관리자 RealmGuard Designer는 다음 영역을 독립적으로 조회·편집하도록 구성합니다.

| 영역 | 주요 검증 |
| --- | --- |
| Stages | 고유 ID/번호, mode/theme, path 좌표 범위, tower spot, 시작 gold/lives, stage version |
| Waves | enemy 참조, count/interval/delay, reward, boss 배치 |
| Enemies | HP/speed/armor/reward/life damage와 trait |
| Bosses | boss HP/speed/armor/life damage, phase·gimmick에서 사용하는 trait |
| Towers | cost/damage/range/fire rate, damage type, branch modifier |
| Heroes | HP/damage/range/speed, ability와 성장 참조 |
| Skills | cooldown, 효과 식별자와 표시 metadata |
| Balance | 난이도 배율, upgrade 비용, hero XP, endless ramp |
| Versions | content/balance/asset/stage version과 게시 상태 |
| Telemetry | 기간별 run/unique user/승리/거부, 최고·실패 wave, stage·hero·난이도별 평균 score/duration, tower·skill 사용량 |

새 version은 현재 게시 snapshot을 복제한 `draft`로 생성됩니다. Section/item 편집은 `draft` 또는 `testing`에서만 가능하고, 전체 스키마·참조 무결성을 즉시 검증한 뒤 상태를 `draft`로 돌리고 해당 version/checksum을 갱신합니다. Section GET의 `ETag`/`version.checksum`을 Section PUT과 item POST/PUT/DELETE의 `If-Match`로 보내야 하며, 누락은 `428 precondition_required`, stale checksum은 `409 stale_version`으로 거부됩니다. 성공한 변경의 새 ETag를 다음 저장에 사용합니다. Test는 `draft|testing → testing`으로 전환하며 campaign 최소 10개, endless 최소 1개, stage별 wave 8~15개, tower/branch/enemy/boss/hero/skill 수량과 ID 참조를 검사합니다. 모든 콘텐츠 ID는 소문자로 시작하고 이후 소문자·숫자·`_`·`-`만 사용하는 1~32자이며, 일반 enemy 10~16종과 boss 2~4종만 허용합니다. Wave당 entry는 최대 8개이고 기본 count 합계는 최대 500입니다. 최대 누적값을 가진 모든 적별 histogram을 포함한 최악 조건의 `battle.complete` JSON은 실제 4 KiB 이하여야 하며, endless 10,000 wave 확장 시 `BaseSpawns`와 splitting/boss 파생 소환을 포함한 `MaxSpawns`가 각각 signed 32-bit를 넘지 않아야 합니다.

승인 정책이 꺼져 있어도 Test를 통과한 `testing|approved` version만 admin/operator가 바로 publish할 수 있습니다. 정책이 켜져 있으면 `testing → pending_approval → approved → published` 순서를 사용합니다. manager/admin이 승인하고 admin이 최종 publish합니다. 검토자가 `decision:rejected`와 필수 comment를 보내면 review comment/시각을 남기고 version을 편집 가능한 `draft`로 되돌립니다. 반려 감사 action은 `realmguard.version.reject`이며 comment와 함께 추적합니다. `separation_of_duties`는 작성자의 자기 승인을 막습니다. Manager 자신의 team이 비어 있으면 pending 목록을 `403 team_required`로 거부하고, 목록에는 비어 있지 않은 동일 team 작성자의 version만 포함합니다. Preview/review에서는 manager와 작성자 어느 한쪽이라도 team이 없으면 `403 team_required`, 서로 다르면 `403 different_team`으로 fail-closed합니다.

`GET /api/v1/realmguard/versions/{id}/preview`는 선택 version의 complete config에 `practice_only:true`를 붙여 반환합니다. Manager preview에는 위의 same-team/fail-closed 조건이 그대로 적용됩니다. 미게시 초안을 실행 검증하는 용도이며 공식 score/progress는 session에 고정된 published snapshot만 받습니다.

Publish transaction은 기존 `published`를 `archived`로 전환하고 선택 version 하나만 `published`로 만듭니다. 게시·보관 version의 content는 Designer 편집 API로 변경할 수 없습니다. 현재 자동 rollback/archived-clone endpoint는 없으므로, 알려진 정상 콘텐츠를 새 draft에 복구해 같은 검증·승인 절차로 재게시하거나 DB/image 복구 세트를 사용합니다.

JSON 직접 편집은 대량 데이터를 다룰 때만 사용하고 일반 변경은 typed form을 우선합니다. 저장 전 server validation 결과를 확인하고, 게시 전에는 representative stage를 각 난이도와 1배/2배속으로 완주하여 path, targeting, pause/resume, skill cooldown과 결과 제출을 점검합니다.

## Score 검증 경계

브라우저의 `calculateLocalResult`는 HUD와 즉시 feedback을 위한 잠정 계산입니다. 전투 중에는 `POST /api/v1/telemetry`에 최상위 UUID `client_event_id`와 세션별 1부터 연속 증가하는 1~100,000 `sequence`를 보냅니다. 같은 ID·순서·event·data 재전송은 idempotent하고, 같은 ID의 다른 payload 또는 sequence 누락·역전은 `409`로 거부됩니다. RealmGuard event `data`는 최대 4 KiB입니다.

원장 용량은 선택 event 전체 128개, `battle.ready` 1개, `battle.complete` 1개, `wave.start` 10,001개, `wave.complete` 10,000개, tower build/upgrade/sell 합계 10,000개로 분리됩니다. class별 예약으로 선택 event가 포화되어도 결과 검증에 필요한 milestone을 계속 받을 수 있으며, 각 class 한도를 넘으면 `429 telemetry_limit`입니다.

원장은 시작 직후 `realmguard.battle.ready`, 도달한 wave별 `realmguard.wave.start`, 완료한 wave별 `realmguard.wave.complete`, 마지막 `realmguard.battle.complete`를 포함합니다. Wave 완료와 battle 완료에는 lives/gold/earned/spent/sold/kills/escaped/spawned/hero level 및 `defeated_by_enemy`, `escaped_by_enemy`, `spawned_by_enemy` 누적 histogram이 들어갑니다. 공식 기록은 `POST /api/v1/realmguard/results`에 같은 누적 histogram, game session/token, stage/mode/difficulty, duration, 경제·전투 수치, completed wave, hero와 content/balance/stage/asset version을 제출합니다. 클라이언트는 공식 `score` 또는 `stars`를 결정하지 않습니다. 결과의 remaining/earned/spent/sold gold는 fresh schema와 v0.1.x upgrade migration에서 모두 PostgreSQL `bigint`로 저장됩니다.

서버는 현재 다음을 검증합니다.

- session 사용자·게임 소유권, active/finished 상태, token hash와 중복 제출
- session 시작 시 고정된 게시 version tuple과 결과의 version 일치
- server receipt 기준 연속 event sequence, ready/wave start·complete/battle complete의 순서와 최소 milestone 시간
- wave snapshot 누적값과 적별 spawned/defeated/escaped histogram의 단조성·합계·고정 spawn 예산
- tower build/upgrade/sell event 원장과 spend/sell 합계, early-call 및 enemy/wave reward와 life damage
- 존재하는 stage/mode/difficulty 조합, stage와 hero 잠금 해제, 검증된 처치 수로 계산한 1~10 전투 hero level
- 서버 경과 시간과 client duration의 허용 오차
- campaign/endless wave 상한, 승리 파생, 잔여 lives와 kill/escaped/spawned 범위
- `round(stage.starting_gold × difficulty.gold)` 시작금, enemy/wave/조기 호출 reward와 spend/sell을 포함한 경제 예산
- 고정 snapshot의 공식으로 서버가 재계산한 score, campaign star와 hero XP/unlock

검증 결과의 `server_received_telemetry_v1`은 인증된 session token으로 브라우저가 자가 보고한 event를 서버가 수신하고 receipt time, 순서와 누적 원장 일관성을 검사했다는 보장입니다. 모든 프레임·적 이동·충돌을 서버가 독립적으로 재실행하는 완전한 시뮬레이션/replay 또는 조작 불가능성 증명은 아닙니다. 결과 payload의 호환용 client `proof`/`events`는 공식 증거로 사용하거나 저장하지 않으며 서버가 attestation digest와 암호화 receipt를 생성합니다.

계정 hero level은 저장된 장기 성장값이며 client 전투 능력치에 적용됩니다. 결과의 `hero_level`은 이 값이 아니라 매 전투 1에서 시작해 검증된 처치로 올라간 battle level입니다. 검증이 성공하면 RealmGuard result, 공통 score, session finish, progress, 계정 hero XP, 잠금 해제와 achievement를 하나의 DB transaction으로 반영하고 `201`에서 서버 score/stars/breakdown과 attestation을 반환합니다. 같은 session/token으로 성공 요청을 다시 보내면 새 기록을 만들지 않고 저장된 결과를 `200`과 `idempotent:true`로 반환합니다. 검증 실패는 공식 ranking/progress에 일부도 반영하지 않고 `realmguard.result.reject` 감사 event로 사유 code를 남깁니다. 주요 반환은 잘못된 입력 `400`, 잠긴 콘텐츠 `403`, 잘못된/별도 종료된 session·version mismatch `409`, stage·duration·wave·lives·kill·gold·hero·telemetry 검증 실패 `422`입니다.

## Version과 운영

Content, balance, stage와 asset version은 각각 변경 이유와 rollback 범위가 다르므로 하나의 문자열로 합치지 않습니다. 운영자는 게시 전후에 다음을 기록합니다.

1. 변경자, 승인자, ticket과 변경 요약
2. 이전/신규 version tuple과 checksum
3. test session 결과 및 주요 telemetry 기준선
4. 게시 시각, 대상 사용자 범위와 rollback 판단 기준

Balance 조정은 신규 session부터 적용하고 진행 중인 session은 시작 때 고정된 version으로 검증합니다. 관리자 telemetry는 선택한 콘텐츠 version과 기간을 기준으로 stage·hero·난이도별 run, 평균 score/duration, campaign 승리, 최고 wave, 실패 wave와 거부 수를 집계합니다. Runtime이 `realmguard.tower.build`와 `realmguard.skill.cast`로 보낸 일반 게임 telemetry도 각각 tower·skill 사용량으로 합산합니다.

## Backup과 복구

RealmGuard의 `realmguard_content_versions`, `realmguard_user_progress`, `realmguard_user_heroes`, `realmguard_user_skills`, `realmguard_user_loadouts`, `realmguard_results`와 연결된 `game_sessions`, `scores`, achievement/audit 및 `game_telemetry` 원장을 하나의 PostgreSQL 백업 시점으로 보존합니다. Result의 `verification_method`, `attestation`, 서버 생성 암호화 proof와 telemetry의 `client_event_id`/`sequence_no`도 복구 검증 대상입니다. 별도 active-pointer table은 없고 단 하나의 `realmguard_content_versions.status='published'` row가 active version입니다. 코드 생성 graphic과 Phaser bundle은 `igame:v0.2.0` 이미지에 있으므로 같은 이미지 archive와 checksum을 DB 복구 세트에 함께 보관합니다. 운영자가 별도 upload 자산을 도입한 경우에만 `/app/data` snapshot도 같은 복구 시점으로 맞춥니다.

복구 훈련에서는 공통 [백업과 복구](backup-restore.md) 절차에 더해 다음을 확인합니다.

1. RealmGuard schema migration과 seed version이 모두 적용됨
2. active content/balance version이 백업 시점과 같음
3. published snapshot checksum과 참조 무결성이 유지됨
4. 기존 session/result/progress의 version tuple이 보존됨
5. `/games/realmguard` 새로고침과 Phaser bundle이 외부 연결 없이 실행됨
6. test session의 1-based telemetry sequence, 누적 histogram, `server_received_telemetry_v1` attestation과 결과 idempotency가 보존됨
7. Designer draft와 audit/telemetry 조회가 복구 기준 시점과 일치함

DB만 최신 시점으로 복구하고 더 오래되거나 더 새로운 이미지로 실행하지 않습니다. migration은 전진 적용이므로 rollback 필요 시 검증된 DB backup과 해당 version의 image archive를 한 쌍으로 복원합니다.

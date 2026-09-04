# RealmGuard 운영 가이드

RealmGuard는 igame `v0.2.0`에 포함된 데이터 기반 tower defense 게임입니다. Phaser runtime, React HUD, Go API와 PostgreSQL 콘텐츠 저장소를 같은 서비스 경계에서 운영하며 브라우저가 공급자 key나 관리 권한을 가지지 않습니다.

## 콘텐츠 0.3.1 캐릭터 표현 고도화

콘텐츠 `0.3.1`은 전투 수치를 바꾸지 않고 코드 생성 캐릭터 자산 규격을 `procedural-2`로 올립니다. 영웅 선택 화면은 에어린·브란·니라의 고유 초상, 체력·공격·사거리·기동 수치, 일반 기술 두 개와 궁극기, 계정 레벨과 정확한 해금 stage를 함께 보여 줍니다. 전장에서는 활·방패·마법 계열 실루엣과 공격 효과가 구분되고, 영웅 머리 위 체력바와 HUD의 현재/최대 HP, 전투 레벨, 부활 시간을 실시간으로 확인할 수 있습니다.

초상과 전투 표현은 SVG/Phaser 도형을 결정론적으로 생성하며 외부 이미지 요청이나 새 런타임 자산을 추가하지 않습니다. 운영자가 자체 콘텐츠를 게시한 설치는 자동으로 덮어쓰지 않으며 기존 게시본을 유지합니다. 정본 `0.3.0`을 사용하던 설치만 동일한 전투 콘텐츠를 가진 새 immutable snapshot으로 전환됩니다. 결과와 랭킹은 새 UUID에 기록되지만 캠페인·영웅·스킬·loadout 진행도는 그대로 이어집니다.


## 콘텐츠 0.3.0 밸런스 재조정

콘텐츠 `0.3.0`은 타워 분기 네 값을 조정합니다. `windward/shield_line`의 `slow`가 0.68이었는데 이 값은 이동속도 배수라 windward 기본값 0.52보다 **둔화가 약했습니다**. "강력한 지상 저지"라는 설명과 반대로 동작했고, 골드당 피해도 업그레이드하지 않은 상태보다 낮았습니다. 0.34로 고쳐 분기가 설명대로 동작합니다.

나머지 셋은 선택지를 되살리기 위한 조정입니다. `stonepulse/ember_core`가 골드당 피해에서 2위를 13% 앞서면서 광역까지 갖춰 대안이 없었고, `quake_drum`과 `star_lattice`는 같은 타워 안에서 24~69% 뒤처져 사문화되어 있었습니다. `ember_core` 2.2→1.9, `quake_drum` 1.3→1.5, `star_lattice` 1.4→1.55로 분기 간 격차가 약 200%에서 34%로 좁혀졌습니다.

**결과와 랭킹은 게시된 콘텐츠 UUID별로 격리됩니다.** 따라서 `0.3.0` 게시 이후의 결과는 새 스냅샷에 쌓이고, `0.2.0`에서 만들어진 기존 결과는 보존되지만 새 랭킹에 섞이지 않습니다. 캠페인 stage·hero·skill·loadout 진행도는 콘텐츠 UUID가 바뀌어도 이어집니다. 서로 다른 밸런스의 점수를 한 순위표에서 비교하지 않으면서 사용자의 해금 상태는 유지하기 위한 설계입니다.

마이그레이션은 게시본이 **손대지 않은 정본 시드일 때만** 적용됩니다. Designer로 직접 게시한 팩이 있는 설치에서는 그 콘텐츠를 그대로 두고 아무것도 바꾸지 않습니다.

## 전장에서 읽히는 것

`v0.7.0`부터 적과 타워는 자기 행동에서 파생된 외형을 갖습니다. 이전에는 12종 전부가 색만 다른 원이었고, 상대법을 결정하는 특성(방어·비행·위상·분열·공성)이 정작 결정을 내리는 전장에서 보이지 않았습니다.

- 정본 12종은 각자의 실루엣을 가지며, 이 client가 처음 보는 원격 설정 적은 **하는 일**에서 모양을 받습니다. 비행이면 가오리, 공성이면 공성 야수, 치유면 로브를 걸친 점술사입니다.
- 특성 표식은 실루엣이 이미 말하지 않을 때만 그립니다. 보스는 이름표가 놓이는 금색 링만 답니다.
- 타워는 3레벨에서 고른 분기를 표식으로 보여줍니다. 표식은 분기 이름이 아니라 분기가 실제로 움직이는 수치에서 정해지므로 운영자가 만든 분기도 읽히며, 규칙이 저지 타워로 취급하는 것은 이름과 무관하게 병영으로 그립니다. 레벨은 링 위의 pip입니다.
- stage 선택 화면의 로스터는 해당 stage의 wave 표에서 읽은 개체와 그 특성에 대한 대처를 함께 보여줍니다.

이 외형은 모두 서비스 공통 renderer이며 콘텐츠의 id와 특성에서 파생됩니다. 새 콘텐츠 version이나 `asset_version` 변경을 요구하지 않고, 전투 규칙에도 닿지 않습니다. kernel이 적의 반지름과 위치만 넘겨주므로 화면이 그리는 것은 결과를 바꿀 수 없습니다.

## 독자 IP와 자산 경계

RealmGuard의 stage명, enemy, tower, hero, skill, 수치 데이터와 코드 생성 그래픽은 이 프로젝트를 위해 작성한 독자 콘텐츠입니다. Kingdom Rush를 포함한 제3자 게임의 코드, 서사, 캐릭터, map, sprite, 음원 또는 UI 자산을 복제하거나 포함하지 않습니다. 공식 lore를 별도로 가정하지 않으며 화면과 콘텐츠 데이터에 실제 존재하는 명칭만 운영 문서의 기준으로 삼습니다.

Phaser 3은 게임 loop와 Canvas/WebGL rendering에 사용하는 MIT 라이선스 framework입니다. production dependency는 lockfile에 고정되고 package metadata와 `LICENSE.md`가 이미지의 `/licenses/phaser`에 포함됩니다. stage 배경, path, tower/enemy/hero 표현과 effect는 bundle된 코드가 도형과 색상으로 생성하므로 원격 image, font, audio 또는 CDN 요청이 필요하지 않습니다. 외부 자산을 추가하려면 출처·라이선스·checksum을 검토하고 이미지 안에 고정한 뒤 SBOM 및 반입 검사를 다시 수행해야 합니다.

## 기본 콘텐츠

초기 내장 설정의 version tuple은 다음과 같습니다.

| 구분 | 기본값 | 의미 |
| --- | --- | --- |
| Content | `0.3.1` | stage, wave, enemy, tower, hero, skill 구조 |
| Balance | `2026.08.2` | 난이도 배율, 가격, 피해량, 성장 곡선 |
| Asset | `procedural-2` | 고유 초상·전장 실루엣·공격/체력 효과를 포함한 코드 생성 시각 자산 규격 |
| Stage | `2026.08.1` | path, tower spot와 wave 구성 |

내장 roster는 다음 범위입니다.

- Campaign stage 10개와 endless mode `끝없는 균열`
- 일반 enemy 10종과 boss 2종: armored, swift, flying, healer, splitting, regenerating, phasing, siege 등의 trait 조합
- Boss `공허왕 오르반`, `시간룡 세라크`; 체력 66%/33% 구간의 phase 전이에서 tower 비활성화, 하수인 소환 또는 가속 gimmick 수행
- Tower 4종: `태양첨탑`, `룬꽃 정원`, `석맥 포대`, 병사 소환·저지 계열 `바람수호 병영`; tower마다 두 upgrade branch
- Targeting `first`, `last`, `strong`, `weak`, `closest`
- Hero 3명: `에어린`, `브란`, `니라`; 고유 초상과 전장 실루엣, 일반 능력 2개와 ultimate 1개, 실시간 HP 표시와 체력 소진 후 hero별 respawn 시간 적용
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

`v0.6.0`부터 RealmGuard 전투의 결과는 서버가 직접 재현합니다. 브라우저는 무엇이 일어났는지 보고하지 않고, 플레이어가 무엇을 했는지만 원장으로 제출합니다.

### 결정론적 전투 kernel과 입력 원장

전투 규칙 전체는 renderer와 분리된 결정론적 kernel 하나에 있습니다. kernel은 고정 50ms step으로만 진행하고 벽시계와 난수를 쓰지 않으며, 사칙연산과 제곱근만 사용해 브라우저와 Go가 같은 배정밀도 결과를 내도록 작성했습니다. 화면의 Phaser scene은 kernel이 만든 상태를 그리기만 하므로 tween이나 그리기 코드가 전투 결과를 바꿀 수 없습니다.

플레이어의 모든 조작은 `{tick, op, …}` 형태로 원장(ledger)에 기록됩니다. `op`는 wave 조기 호출, tower build/upgrade/sell, targeting 변경, skill 사용과 좌표 시전, 영웅 이동, 교육 이벤트의 자원 조정, 항복입니다. 원장 하나에 최대 6,000개 명령과 288,000 tick(시뮬레이션 기준 4시간)까지 허용하며 이를 넘으면 결과가 거부됩니다.

결과 제출의 `ledger`에는 규칙 버전(`realmguard-kernel-1`), 콘텐츠 투영 digest, 총 tick 수와 명령 목록이 들어갑니다. 서버는 세션에 고정된 게시 콘텐츠를 같은 방식으로 투영해 digest를 다시 계산하고, 일치할 때만 원장을 재생해 lives·gold·kills·escaped·spawned·완료 wave·영웅 level·승패와 적별 histogram을 **직접 산출**합니다. 브라우저가 보낸 전투 수치는 이 시점에서 전부 버려집니다. digest가 다르면 화면과 서버가 콘텐츠를 다르게 읽고 있다는 뜻이므로 점수를 매기지 않고 `409 content_projection_mismatch`로 거부하고 재게시를 요청합니다.

두 구현이 갈라지지 않도록 브라우저가 생성한 replay vector와 콘텐츠 투영 fixture를 저장소에 커밋하고, Go와 vitest 양쪽 test가 같은 파일을 검증합니다. 한쪽에만 규칙 변경이 들어가면 build가 실패합니다.

`duration_ms`는 이제 벽시계가 아니라 시뮬레이션 시간(tick × 50ms)입니다. 배속 재생으로 clear time bonus를 얻을 수 없으며, 서버는 세션 경과 시간의 최대 2배까지만 허용합니다.

재현에 사용한 원장은 결과와 함께 `realmguard_results.ledger`에 보존되므로, 이의 제기나 규칙 변경 시 같은 전투를 다시 판정할 수 있습니다.

### telemetry 원장

브라우저의 `calculateLocalResult`는 HUD와 즉시 feedback을 위한 잠정 계산입니다. 전투 중에는 `POST /api/v1/telemetry`에 최상위 UUID `client_event_id`와 세션별 1부터 연속 증가하는 1~100,000 `sequence`를 보냅니다. 같은 ID·순서·event·data 재전송은 idempotent하고, 같은 ID의 다른 payload 또는 sequence 누락·역전은 `409`로 거부됩니다. RealmGuard event `data`는 최대 4 KiB입니다.

원장 용량은 선택 event 전체 128개, `battle.ready` 1개, `battle.complete` 1개, `wave.start` 10,001개, `wave.complete` 10,000개, tower build/upgrade/sell 합계 10,000개로 분리됩니다. class별 예약으로 선택 event가 포화되어도 결과 검증에 필요한 milestone을 계속 받을 수 있으며, 각 class 한도를 넘으면 `429 telemetry_limit`입니다.

원장은 시작 직후 `realmguard.battle.ready`, 도달한 wave별 `realmguard.wave.start`, 완료한 wave별 `realmguard.wave.complete`, 마지막 `realmguard.battle.complete`를 포함합니다. Wave 완료와 battle 완료에는 lives/gold/earned/spent/sold/kills/escaped/spawned/hero level 및 `defeated_by_enemy`, `escaped_by_enemy`, `spawned_by_enemy` 누적 histogram이 들어갑니다. 공식 기록은 `POST /api/v1/realmguard/results`에 같은 누적 histogram, game session/token, stage/mode/difficulty, duration, 경제·전투 수치, completed wave, hero와 content/balance/stage/asset version을 제출합니다. 클라이언트는 공식 `score` 또는 `stars`를 결정하지 않습니다. 결과의 remaining/earned/spent/sold gold는 fresh schema와 v0.1.x upgrade migration에서 모두 PostgreSQL `bigint`로 저장됩니다.

서버는 재현한 결과에 대해 다음을 함께 검증합니다.

- 제출된 원장의 규칙 버전, 명령 수·tick 상한, tick 단조성과 콘텐츠 투영 digest 일치
- session 사용자·게임 소유권, active/finished 상태, token hash와 중복 제출
- session 시작 시 고정된 게시 version tuple과 결과의 version 일치
- server receipt 기준 연속 event sequence, ready/wave start·complete/battle complete의 순서와 최소 milestone 시간
- wave snapshot 누적값과 적별 spawned/defeated/escaped histogram의 단조성·합계·고정 spawn 예산
- tower build/upgrade/sell event 원장과 spend/sell 합계, early-call 및 enemy/wave reward와 life damage
- 존재하는 stage/mode/difficulty 조합, stage와 hero 잠금 해제, 검증된 처치 수로 계산한 1~10 전투 hero level
- 서버 경과 시간과 재현한 시뮬레이션 시간의 허용 오차(최대 2배속)
- campaign/endless wave 상한, 승리 파생, 잔여 lives와 kill/escaped/spawned 범위
- `round(stage.starting_gold × difficulty.gold)` 시작금, enemy/wave/조기 호출 reward와 spend/sell을 포함한 경제 예산
- 고정 snapshot의 공식으로 서버가 재계산한 score, campaign star와 hero XP/unlock

검증 결과의 `server_replay_v1`은 서버가 게시된 콘텐츠와 플레이어 입력만으로 전투를 처음부터 다시 실행해 점수와 별을 확정했다는 뜻입니다. 적 이동·타겟팅·피해·보스 단계·분열·차단까지 모두 서버 코드가 계산하므로, 수정된 client는 자신의 입력을 바꿀 수는 있어도 그 입력의 결과를 바꿀 수 없습니다. attestation에는 replay 정보(`rules_version`, `config_digest`, tick·명령 수)와 함께 기존 telemetry 검증 결과가 같이 저장됩니다. telemetry 검증은 그 전투가 실제로 이 session에서 진행됐음을 확인하는 두 번째 방어선으로 유지되며, 재현한 결과와 브라우저가 흘려보낸 milestone이 어긋나면 거부합니다. 결과 payload의 호환용 client `proof`/`events`는 공식 증거로 사용하거나 저장하지 않으며 서버가 attestation digest와 암호화 receipt를 생성합니다.

Defense Series는 같은 실행 engine을 사용하지만 AI 자원 모델과 교육 결과가 얽혀 있어 이번 릴리스에서는 `server_received_telemetry_v1` 검증을 유지합니다. 시뮬레이션 시간 기준 duration 계약만 RealmGuard와 동일하게 맞췄습니다.

계정 hero level은 저장된 장기 성장값이며 client 전투 능력치에 적용됩니다. 결과의 `hero_level`은 이 값이 아니라 매 전투 1에서 시작해 검증된 처치로 올라간 battle level입니다. 검증이 성공하면 RealmGuard result, 공통 score, session finish, progress, 계정 hero XP, 잠금 해제와 achievement를 하나의 DB transaction으로 반영하고 `201`에서 서버 score/stars/breakdown과 attestation을 반환합니다. 같은 session/token으로 성공 요청을 다시 보내면 새 기록을 만들지 않고 저장된 결과를 `200`과 `idempotent:true`로 반환합니다. 검증 실패는 공식 ranking/progress에 일부도 반영하지 않고 `realmguard.result.reject` 감사 event로 사유 code를 남깁니다. 주요 반환은 잘못된 입력 `400`, 잠긴 콘텐츠 `403`, 잘못된/별도 종료된 session·version mismatch `409`, stage·duration·wave·lives·kill·gold·hero·telemetry 검증 실패 `422`입니다.

## Version과 운영

Content, balance, stage와 asset version은 각각 변경 이유와 rollback 범위가 다르므로 하나의 문자열로 합치지 않습니다. 운영자는 게시 전후에 다음을 기록합니다.

1. 변경자, 승인자, ticket과 변경 요약
2. 이전/신규 version tuple과 checksum
3. test session 결과 및 주요 telemetry 기준선
4. 게시 시각, 대상 사용자 범위와 rollback 판단 기준

Balance 조정은 신규 session부터 적용하고 진행 중인 session은 시작 때 고정된 version으로 검증합니다. 관리자 telemetry는 선택한 콘텐츠 version과 기간을 기준으로 stage·hero·난이도별 run, 평균 score/duration, campaign 승리, 최고 wave, 실패 wave와 거부 수를 집계합니다. Runtime이 `realmguard.tower.build`와 `realmguard.skill.cast`로 보낸 일반 게임 telemetry도 각각 tower·skill 사용량으로 합산합니다.

## Backup과 복구

RealmGuard의 `realmguard_content_versions`, `realmguard_user_progress`, `realmguard_user_heroes`, `realmguard_user_skills`, `realmguard_user_loadouts`, `realmguard_results`와 연결된 `game_sessions`, `scores`, achievement/audit 및 `game_telemetry` 원장을 하나의 PostgreSQL 백업 시점으로 보존합니다. Result의 `verification_method`, `attestation`, 재현에 사용한 `ledger`, 서버 생성 암호화 proof와 telemetry의 `client_event_id`/`sequence_no`도 복구 검증 대상입니다. 별도 active-pointer table은 없고 단 하나의 `realmguard_content_versions.status='published'` row가 active version입니다. 코드 생성 graphic과 Phaser bundle은 실제 배포한 igame 서비스 이미지(현재 `igame:v0.7.6`)에 있으므로 그 이미지 archive와 checksum을 DB 복구 세트에 함께 보관합니다. RealmGuard 콘텐츠 버전 `0.3.1`과 서비스 이미지 버전을 혼동하지 않습니다. 운영자가 별도 upload 자산을 도입한 경우에만 `/app/data` snapshot도 같은 복구 시점으로 맞춥니다.

복구 훈련에서는 공통 [백업과 복구](backup-restore.md) 절차에 더해 다음을 확인합니다.

1. RealmGuard schema migration과 seed version이 모두 적용됨
2. active content/balance version이 백업 시점과 같음
3. published snapshot checksum과 참조 무결성이 유지됨
4. 기존 session/result/progress의 version tuple이 보존됨
5. `/games/realmguard` 새로고침과 Phaser bundle이 외부 연결 없이 실행됨
6. test session의 1-based telemetry sequence, 누적 histogram, `server_replay_v1` attestation과 보존된 입력 원장, 결과 idempotency가 보존됨
7. Designer draft와 audit/telemetry 조회가 복구 기준 시점과 일치함

DB만 최신 시점으로 복구하고 더 오래되거나 더 새로운 이미지로 실행하지 않습니다. migration은 전진 적용이므로 rollback 필요 시 검증된 DB backup과 해당 version의 image archive를 한 쌍으로 복원합니다.

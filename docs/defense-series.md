# Defense Series 운영 가이드

## 범위

Defense Series는 RealmGuard에서 검증한 데이터 기반 방어 메커니즘을 재사용하는 세 개의 독립 게임입니다.

| slug | 화면 | 목적 | 별도 학습 결과 |
| --- | --- | --- | --- |
| `office-guardians` | `/games/office-guardians` | 조직·직무 협업과 사내 이벤트 | 기본 seed에는 없음 |
| `cyber-fortress` | `/games/cyber-fortress` | 보안 위협 인지와 대응 교육 | Security Learning Score |
| `ai-nexus-defense` | `/games/ai-nexus-defense` | AI 구조·보안·비용·거버넌스 교육 | AI Literacy Score |

공통 엔진은 path, tower, enemy, hero, skill, wave, economy와 결과 화면을 제공합니다. 각 게임의 콘텐츠, 진행도, 결과, 랭킹, 교육 기록과 published version은 slug로 격리됩니다. 한 게임의 version UUID나 session token을 다른 slug에서 사용할 수 없습니다.

교육 기능은 slug에 하드코딩하지 않고 published content로 결정합니다. 기본 Office Guardians seed는 `events`와 `education`이 모두 비어 있어 별도 학습 결과가 없지만, Studio에서 두 section을 유효한 참조로 함께 추가해 게시하면 같은 교육 선택·learning·report 계약이 활성화됩니다. 두 section 중 하나만 설정한 pack은 Test를 통과하지 못합니다.

## 콘텐츠 0.4.0 전장 지도

Defense Series 콘텐츠 `0.4.0`은 28개 stage별 지도 identity와 10개 전술 geometry, 게임별 `map_style`을 제공합니다. 직선 압박, S자 우회, 순환로, 교차로, 상·하 합류와 평행 이중 진입로를 포함하며, 선택 카드의 미니맵에서 경로·건설 지점·진입점·방어 목표와 lane 수를 전투 전에 확인할 수 있습니다. Office는 서비스·클라우드·데이터센터, Cyber는 경계망·방화벽·데이터 금고, AI Nexus는 RAG·Agent·Model Router·Nexus Core의 코드 생성 배경을 사용합니다.

`paths`가 둘 이상인 stage의 wave는 `path_index`로 실제 lane을 나눠 사용합니다. 엔진은 모든 lane의 진입점과 목표를 표시하고, 배럭 병사를 모든 선분 중 가장 가까운 위치에 배치하며, waypoint 개수가 아닌 전체 경로 이동 거리 비율로 선두·후미 targeting을 비교합니다. Studio와 브라우저는 lane 범위를 벗어난 `path_index`, 비정상 delay, 중복·알 수 없는 modifier를 거부합니다.

마이그레이션은 정본 `0.3.0` published snapshot만 새 immutable `0.4.0` snapshot으로 전환하고 직접 게시한 운영자 콘텐츠는 유지합니다. 이전 UUID의 결과·진행도·랭킹은 보존되지만 새 UUID와 섞이지 않으므로, 정본 설치의 `0.4.0` 캠페인 진행도와 랭킹은 stage 1부터 새로 시작합니다.

기존에 게시된 custom schema `0.3.x` 팩은 종전의 관대한 wave 정규화로 계속 실행됩니다. 다만 새 Draft의 Test·Publish에는 서버의 엄격한 `0.4.0` routing 규칙이 적용되므로, 해당 팩을 다시 게시하기 전 delay, lane index, parallel 값과 modifier allowlist를 먼저 정리해야 합니다.

## 실행 계약

브라우저는 전투 전에 다음 순서로 실행합니다.

1. `GET /api/v1/defense/{slug}/config`에서 published content와 UUID를 읽습니다.
2. `POST /api/v1/games/{slug}/sessions`의 metadata에 정확한 `defense_content_version_id`를 넣습니다.
3. 포털이 발급한 session ID/token으로 게임을 실행합니다.
4. 게임별 전용 `POST /api/v1/defense/{slug}/results`로 결과를 제출합니다.

version pin이 없으면 HTTP `428`과 `defense_version_required`, 현재 published snapshot과 다르면 HTTP `409`와 `defense_config_stale`을 반환합니다. 일반 `/api/v1/scores`와 일반 game ranking 경로는 Defense Series의 공식 결과나 랭킹을 받지 않습니다. 이를 통해 전용 검증과 콘텐츠 버전 경계를 우회할 수 없습니다.

게시 config를 읽지 못한 경우 브라우저는 번들된 화면·경로 자산으로 명시적인 Demo/연습 mode만 제공할 수 있습니다. 이 fallback에는 교육 문제·정답이 포함되지 않으며 GameHub session, 교육 답안, 공식 result/progress/ranking 요청을 전송하지 않습니다. 네트워크가 복구되면 published config를 새로 읽어 별도의 기록 session을 시작합니다.

클라이언트에서 표시한 점수는 참고값입니다. 서버는 session, slug, content version, server 경과 시간, 게시된 stage/wave budget과 서버가 순서대로 수신한 전투 원장에 대해 제출 필드를 검증하고 공식 점수·진행도·랭킹을 다시 계산합니다. 각 원장 event는 UUID와 session별 1-based 연속 sequence를 사용하며 ready, wave start/complete와 battle complete의 누적 적 histogram·경제·health를 결과와 대조합니다. 이 `server_received_telemetry_v1` 검증은 브라우저가 보고한 event의 서버 수신·순서·시각·누적 일관성을 보장하지만 모든 frame·충돌을 서버가 재실행하거나 실제 사용자 입력을 암호학적으로 증명하는 것은 아닙니다. 경쟁 강도가 높은 이벤트는 운영 이상 탐지와 사후 검토를 병행합니다. 클라이언트는 AI 공급자 secret이나 서비스 암호화 키를 받지 않습니다.

필수 milestone이 pause나 hero 이동 같은 선택 event에 밀리지 않도록 수신 한도를 class별로 예약합니다.

| Event class | session당 한도 |
| --- | ---: |
| 선택 event 전체 (`game.pause`, `game.resume`, skill, hero, education prompt) | 128 |
| `defense.battle.ready` | 1 |
| `defense.battle.complete` | 1 |
| `defense.wave.start` | 101 |
| `defense.wave.complete` | 100 |
| `defense.education.apply` | 500 |
| tower build + upgrade + sell 합계 | 10,000 |

각 event의 실제 JSON `data`는 최대 4 KiB입니다. Studio Test와 Publish도 stage별 최악 누적 enemy histogram 세 개, 최대 counter와 100자 content/policy version, AI resource state를 포함한 `battle.complete` 표본을 실제 직렬화해 이 한도를 넘는 pack을 거부합니다. 같은 UUID와 동일한 event/sequence/data 재전송은 중복으로 안전하게 처리합니다. UUID를 다른 payload에 재사용하거나 sequence를 건너뛰면 `409`, 해당 class 한도를 넘으면 `429`입니다.

## 교육 이벤트와 학습 결과

Cyber Fortress와 AI Nexus Defense content pack은 전투 중 선택형 교육 이벤트를 포함합니다. 문제와 중립적인 `A`/`B`/`C` 선택지는 게시된 콘텐츠 버전에 속하지만 `correct_answer_id`와 해설은 서버 전용 material입니다. public config와 preview, JavaScript bundle에는 정답 mapping이나 의미론적 `safe`/`unsafe` answer ID가 포함되지 않습니다. 정답 위치와 ID도 문제 전체에서 분산합니다. 브라우저는 다음 전용 경로에 답을 제출하며, 정답 여부와 해설은 실제로 도달한 event에 답을 보낸 뒤에만 받습니다.

```text
POST /api/v1/defense/{slug}/education/events/{eventID}/answer
GET  /api/v1/defense/{slug}/learning
```

서버는 현재 사용자와 session, slug, published version, event/answer 조합을 확인하고 정답 여부와 학습 topic을 기록합니다. 같은 event에 대한 재전송은 중복 학습 실적으로 늘리지 않아야 합니다. 게임 점수와 학습 결과는 분리해 보관하며, 운영자는 교육에 필요한 최소 범위만 조회합니다.

## AI Nexus 자원과 model profile

AI Nexus Defense의 published pack에는 `Small`, `Medium`, `Large`, `Reasoning`, `Vision` 다섯 model profile과 각 profile의 tower mapping, Compute/Token/Latency 비용, 정확도와 피해 배수가 들어 있습니다. `resource_rules`는 Compute, Token, Trust, Latency 시작값, wave 비용과 적 통과 시 공통 `escaped_trust_cost`/`escaped_latency_cost`를 정의하며 `balance.resource_state_limits`와 일치해야 합니다. 통과한 적마다 그 적의 `resource_effect`와 공통 escaped 비용을 모두 누적합니다. 비용이 남은 양보다 크면 `remaining`은 0에서 포화되며 음수 잔액이나 다음 판으로 이월되는 숨은 debt를 만들지 않습니다. `spent`는 항상 `start - remaining`이고 네 지표 중 하나라도 0이면 자원 소진 패배입니다. 공식 결과는 각 자원의 `{start, spent, remaining}` 원장을 제출하고 서버가 이 보존식, 허용 key와 고정된 시작 한도를 확인합니다. 다른 두 게임은 이 AI resource state를 제출할 수 없습니다.

## MCP config, session pin과 랭킹

MCP `game_session_start`로 세 게임을 시작할 때도 먼저 `defense_config_get` 또는 REST config의 `version.id`를 읽고 `defense_content_version_id`로 전달합니다. Defense slug에는 이 값이 필수이고 누락은 `428`, stale·unpublished·다른 slug UUID는 `409`입니다. `realmguard_version_id`와 `defense_content_version_id`는 상호 배타적이며 대상 게임과 다른 종류의 pin을 보내지 않습니다. 일반 `score_submit`과 `leaderboard_get`은 Defense 공식 경계를 우회할 수 없습니다. 결과는 전용 REST endpoint로 제출하고, 랭킹은 version-pinned REST 또는 `defense_rankings_get`으로 daily/weekly/monthly/season/all_time을 조회합니다.

개인 학습 화면과 관리 report는 다음 원칙을 따릅니다.

- 게임별 참여·완료·정답률과 topic별 집계를 구분합니다.
- 부서 통계는 권한과 개인정보 공개 정책을 따릅니다.
- 개인 원시 답안은 감사·교육 목적에 필요한 기간만 보존합니다.
- 인사평가나 징계에 연결하려면 별도 사내 정책과 고지를 먼저 적용합니다.

거부된 공식 결과는 `defense.result.reject` 감사 항목으로 slug·오류 code·사유와 함께 남고, telemetry report의 `rejected_results`가 조회 기간의 건수를 보여 줍니다. attestation은 완전한 서버 시뮬레이션이 아니므로 이 수치의 급증을 이상 탐지 신호로 감시합니다.

관리 report는 현재 published UUID와 그 `policy_version`만 집계합니다. Telemetry의 `average_game_score`는 해당 snapshot의 검증된 게임 점수 평균이며 호환 필드 `average_score`와 같은 값입니다. Learning report의 `participants`는 해당 snapshot에서 검증 결과를 가진 고유 사용자, `plays`는 검증된 시도, `battle_clear_rate`는 승리 시도 비율입니다. Learning report의 `completion_rate`는 개별 전투 승률이 아니라 해당 pack의 모든 campaign 완료 사용자 비율이며, `topics`/`questions`는 서버가 판정한 답안만 집계합니다. 과거 snapshot 결과는 보존되지만 새 policy report에 섞이지 않습니다.

## Defense Content Studio

관리자는 `/admin/defense`에서 게임과 section을 선택해 콘텐츠를 관리합니다. 지원 section은 `stages`, `waves`, `towers`, `enemies`, `bosses`, `heroes`, `skills`, `events`, `education`, `balance`, `campaigns`, `resource_rules`, `model_profiles`입니다. 마지막 두 section은 AI Nexus Defense의 자원·model tower 정책을 관리하며 다른 게임에서는 빈 설정을 유지합니다.

```text
Draft 작성
  → If-Match checksum 저장
  → Test
  → 연습 미리보기
  → 승인 요청 또는 즉시 게시
  → 승인/반려
  → Publish
```

새 Draft에는 최대 100자의 `policy_version`을 지정할 수 있습니다. 비우면 복제한 source snapshot의 값을 계승하고, 변경하면 결과·학습 기록과 운영 report가 새 정책 경계를 보존합니다. 기존 version을 되돌릴 때는 해당 UUID를 `source_version_id`로 선택해 새 Draft를 만듭니다. 과거 row를 다시 활성화하거나 현재 published row를 덮어쓰지 않으며, 복제된 Draft도 Test·미리보기·승인 정책·Publish를 처음부터 다시 거칩니다.

Draft 저장에는 서버가 직전에 반환한 checksum을 `If-Match` header로 보내야 합니다. header가 없으면 HTTP `428`, 다른 편집자가 먼저 저장해 checksum이 바뀌었으면 HTTP `409`를 반환합니다. 충돌 시 서버의 최신 section을 다시 읽고 사용자 변경을 명시적으로 병합합니다. checksum을 무시한 강제 덮어쓰기는 지원하지 않습니다.

Test는 schema, ID 참조, stage/wave 구성, 수치 범위와 교육 event/answer 연결을 검증합니다. preview 경로의 전투는 연습 전용이며 진행도·공식 점수·랭킹·교육 완료 기록을 만들지 않습니다.

## 승인 정책

서비스 관리자가 승인 흐름을 끄면 검토 요청 없이 Test를 통과한 version을 게시할 수 있습니다. 승인 흐름을 켜면 pending version만 검토 큐에 들어갑니다.

- Manager 검토는 Manager와 작성자의 team이 모두 비어 있지 않고 동일할 때만 허용됩니다.
- 승인과 반려에는 감사 기록과 comment를 남깁니다.
- 반려된 version은 published snapshot을 바꾸지 않고 수정 가능한 Draft로 돌아갑니다.
- Publish는 새 immutable snapshot을 활성화하며 이미 시작한 session은 자신이 pin한 이전 UUID 경계를 유지합니다.

## API 요약

사용자 API:

```text
GET  /api/v1/defense/{slug}/config
GET  /api/v1/defense/{slug}/version
GET  /api/v1/defense/{slug}/progress
POST /api/v1/defense/{slug}/results
GET  /api/v1/defense/{slug}/rankings
GET  /api/v1/defense/{slug}/learning
POST /api/v1/defense/{slug}/education/events/{eventID}/answer
GET  /api/v1/defense/{slug}/versions/{id}/preview
GET  /api/v1/defense/versions/pending
POST /api/v1/defense/versions/{id}/review
```

관리자 API:

```text
GET|PUT /api/v1/admin/defense/{slug}/drafts/{section}
GET|POST /api/v1/admin/defense/{slug}/versions
POST     /api/v1/admin/defense/{slug}/versions/{id}/test
POST     /api/v1/admin/defense/{slug}/versions/{id}/publish
GET      /api/v1/admin/defense/{slug}/telemetry
GET      /api/v1/admin/defense/{slug}/learning-report
```

전체 request/response 예와 오류 코드는 [REST/SSE API](api.md)를 참조하세요.

## 운영 점검

배포 후 세 게임 각각에 대해 다음을 확인합니다.

1. portal card와 direct route가 열리고 새로고침 후에도 같은 화면을 복원합니다.
2. published config/version UUID와 콘텐츠 버전이 이미지 버전과 일치합니다.
3. 전투 canvas가 외부 HTTP 요청 없이 표시됩니다.
4. 3개 thumbnail/banner SVG 여섯 개가 HTTP 200이고 remote href/url을 포함하지 않습니다.
5. Cyber/AI의 첫 교육 선택과 서버 피드백이 동작하고 AI model profile·자원 HUD가 표시됩니다.
6. progress/result/ranking이 다른 Defense slug에 섞이지 않습니다.
7. Studio Draft 저장, Test, preview, 승인/반려와 Publish를 시험합니다.
8. telemetry와 learning report가 관리자 권한에서만 열리는지 확인합니다.

릴리스 전에는 fresh PostgreSQL에서 `scripts/smoke-defense-series.sh`를 실행합니다. 이 smoke와 RealmGuard 회귀 smoke가 모두 통과해야 `igame:v0.7.6` archive를 만들 수 있습니다.

## 백업과 복구

Defense 콘텐츠 version, Draft, 진행도, 결과, 학습 기록은 PostgreSQL 백업 범위에 포함됩니다. 업로드된 사용자 자산을 사용하는 content pack은 `/app/data`도 같은 복구 시점으로 백업합니다. 복구 후에는 active published UUID와 checksum, 세 게임 route, 학습 report를 확인한 뒤 사용자 트래픽을 엽니다. 일반 절차는 [백업과 복구](backup-restore.md)를 따릅니다.

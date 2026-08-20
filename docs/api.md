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
| POST | `/api/v1/telemetry` | `sessions:write`, 세션 token으로 SDK event 제출 |
| GET | `/api/v1/rankings` | `rankings:read`, 기간/팀/부서 랭킹 |
| GET | `/api/v1/rankings/{gameId}` | `rankings:read`, 게임별 랭킹 |
| GET | `/api/v1/achievements` | 로그인, 업적 목록 |
| POST | `/api/v1/me/achievements` | 로그인, client-unlockable 업적 해제 |
| GET | `/api/v1/me` | `profile:read`, 본인 프로필 |
| PATCH | `/api/v1/me` | 본인 개인정보/공개 설정 |
| PUT | `/api/v1/me/password` | interactive session 전용, 로컬 비밀번호 변경 |
| GET | `/api/v1/me/history` | 본인 플레이 기록 |
| GET/POST | `/api/v1/me/api-keys` | 개인 키 목록/생성; 원문은 생성 응답 1회 |
| PATCH/DELETE | `/api/v1/me/api-keys/{id}` | 개인 키 변경/폐기 |
| POST | `/api/v1/me/api-keys/{id}/rotate` | 개인 키 즉시 회전; 새 원문은 응답 1회 |
| GET | `/api/v1/events` | 공개 가능한 이벤트 |
| GET | `/api/v1/events/{eventId}` | 이벤트 상세와 본인 참가 상태 |
| POST | `/api/v1/events/{eventId}/join` | 이벤트 참가 |
| GET | `/api/v1/seasons` | 시즌 목록 |
| GET | `/api/v1/notices` | 게시된 공지 목록 |
| GET | `/api/v1/banners` | 현재 노출 가능한 배너 목록 |
| POST | `/api/v1/ai/chat/completions` | AI 게임용 기본 streaming proxy |
| GET | `/api/v1/admin/dashboard` | 관리자/운영자 session 또는 `admin:*`, 운영 요약 |
| GET | `/api/v1/admin/analytics` | 관리자/운영자 session 또는 `admin:*`, DAU/WAU/MAU 등 |
| GET | `/api/v1/admin/settings` | admin session 또는 admin 역할 + `admin:*` 키, 전체 설정 조회 |
| GET/PUT | `/api/v1/admin/settings/{key}` | admin session 또는 admin 역할 + `admin:*` 키, 일반 설정 조회/변경 |
| GET/PUT | `/api/v1/admin/oidc` | admin session 또는 admin 역할 + `admin:*` 키, OIDC 설정 |
| GET/PUT | `/api/v1/admin/ai` | admin session 또는 admin 역할 + `admin:*` 키, AI 설정 |
| GET/POST/PUT/DELETE | `/api/v1/admin/{games,categories,seasons,events,achievements}` | 카탈로그와 참여 콘텐츠 관리 |
| GET/POST/PUT/DELETE | `/api/v1/admin/{tournaments,rewards,notices,banners}` | 운영 콘텐츠 관리 |
| GET/PUT/DELETE | `/api/v1/admin/rankings[/{id}]` | 점수 검토·제외 |
| GET | `/api/v1/admin/audit` | admin session 또는 admin 역할 + `admin:*` 키, 감사 조회 |

OIDC client secret과 AI API key는 write-only입니다. 설정 조회 응답은 원문 대신 `client_secret_configured` 또는 `api_key_configured` 상태를 반환합니다.

개인 키 생성·변경·회전·폐기와 로컬 비밀번호 변경은 로그인된 브라우저 session에서만 허용되며, 개인 API 키로 키나 비밀번호를 관리할 수 없습니다. 관리자 API를 개인 키로 호출하려면 관리자 역할과 `admin:*` scope가 모두 필요합니다. 기존에 발급된 키도 매 요청 시 저장 scope와 현재 전역 허용 목록·현재 역할 정책의 교집합만 유효하므로, 관리자가 권한을 제거하거나 사용자 역할을 바꾸면 즉시 축소 적용됩니다.

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

관리자 승인 정책이 활성화된 경우 지원되는 게임 생성·변경을 `/api/v1/workflow/requests`로 제출하고 관리자는 `/api/v1/admin/workflow/requests/{id}/review`에서 승인 또는 반려합니다. 팀장은 `/api/v1/workflow/reviews`에서 검토 대상을 조회하고 `/api/v1/workflow/requests/{id}/review`에서 처리합니다. 정책이 비활성이면 별도 검토 상태를 만들지 않고 요청 payload를 바로 반영합니다. 정상 처리는 `pending → applied|rejected`이며 적용에 실패하면 다시 `pending`으로 남습니다. `separation_of_duties`는 기본적으로 자기 요청 검토를 막고, 팀장 역할은 요청자와 검토자 모두의 팀 정보가 있을 때 같은 팀 요청으로 제한되며, 반려에는 비어 있지 않은 `comment`가 필요합니다. 처리 내역은 감사 로그에 기록됩니다.

## 배포 경계의 요청 제한

운영 reverse proxy에서 인증, score, AI, 관리자 변경 경로에 각각 적절한 요청 속도와 본문 크기 제한을 적용합니다. 클라이언트는 429/5xx에 지수 backoff와 jitter를 적용하되 mutation을 무조건 자동 재시도하지 않습니다.

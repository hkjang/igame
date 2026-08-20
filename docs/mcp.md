# MCP server

igame은 같은 서비스의 `/mcp`에서 MCP Streamable HTTP를 제공합니다. 기준 protocol revision은 `2025-11-25`, message는 JSON-RPC 2.0입니다. 별도 MCP daemon이나 인터넷 연결이 필요하지 않습니다.

## 인증

원격 MCP client는 개인화 페이지에서 만든 `mcp:access` 포함 개인 API 키를 Bearer token으로 사용합니다. 동일 origin의 로그인된 브라우저 session도 인증되지만 자동화 client에는 cookie 대신 범위 제한 개인 키를 사용합니다.

서버는 개인 키의 owner, scope, 만료, 폐기와 호출 tool의 추가 scope를 검사합니다. 기존 키도 매 요청 시 저장 scope와 현재 전역 허용 목록·현재 역할 정책의 교집합만 인정하므로 관리자 정책 축소나 역할 변경이 즉시 반영됩니다. 키 원문은 발급 때 한 번만 표시됩니다. 회전 API는 새 키를 발급하는 동시에 이전 키를 폐기하므로 consumer를 원자적으로 교체하거나 새 키를 별도로 만든 뒤 전환해야 합니다. 관리자 페이지에서 역할별 허용 scope와 최대 유효기간을 바꿀 수 있습니다.

MCP 사양의 OAuth 자동 discovery가 필요한 client와 Keycloak Bearer token 직접 인증은 현재 범위에 포함되지 않습니다. 사내 client에는 미리 발급한 개인 키를 안전한 credential store로 전달합니다.

## 연결

초기화 예:

```bash
curl --no-buffer https://igame.company.local/mcp \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  --data '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"initialize",
    "params":{
      "protocolVersion":"2025-11-25",
      "capabilities":{},
      "clientInfo":{"name":"offline-client","version":"1.0.0"}
    }
  }'
```

이후 요청에는 `MCP-Protocol-Version: 2025-11-25`를 포함합니다. 현재 서버는 stateless 방식이라 `Mcp-Session-Id`와 재개 가능한 event ID를 발급하지 않습니다. 응답을 SSE로 받을 때는 proxy buffering을 끕니다. JSON-RPC batch는 지원하지 않습니다.

## 제공 tool

| MCP name | 추가 scope | 설명 |
| --- | --- | --- |
| `games_list` | `games:read` | query/category/limit으로 활성 게임 검색 |
| `game_get` | `games:read` | UUID 또는 slug로 게임 metadata 조회 |
| `leaderboard_get` | `rankings:read` | 게임/기간/개인·부서·팀 랭킹 조회 |
| `profile_get` | `profile:read` | 인증 사용자 프로필 조회 |
| `events_list` | `games:read` | 공개 가능한 이벤트 조회 |
| `game_session_start` | `sessions:write` | server-validated 게임 세션과 token 생성 |
| `score_submit` | `scores:write` | 세션 ID/token과 함께 검증 가능한 점수 제출 |

관리자 설정, 키 관리와 게임 게시 tool은 실수나 prompt injection 영향을 줄이기 위해 노출하지 않습니다. `game_session_start`와 `score_submit`은 상태를 만들므로 MCP client가 호출 전에 사용자 확인을 표시하는 것이 권장됩니다.

tool 호출 예:

```bash
curl --no-buffer https://igame.company.local/mcp \
  -H 'Authorization: Bearer <token>' \
  -H 'MCP-Protocol-Version: 2025-11-25' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  --data '{
    "jsonrpc":"2.0",
    "id":2,
    "method":"tools/call",
    "params":{"name":"leaderboard_get","arguments":{"game_id":"snake","period":"weekly","limit":10}}
  }'
```

## 오류와 제한

잘못된 JSON-RPC 또는 method는 protocol error, tool 인자·권한·업무 규칙·검증 실패는 `result.isError: true`인 tool execution error로 반환합니다. 오류에 secret, 내부 SQL, 다른 사용자의 개인정보를 포함하지 않습니다. 서버는 요청 본문을 2 MiB로 제한하며 운영 proxy에서 별도 rate limit을 적용합니다.

Streamable HTTP의 `Origin`이 있으면 서비스 origin과 정확히 비교해 DNS rebinding을 막습니다. 서버 자체는 평문 HTTP listener이므로 remote MCP 공개 시 TLS reverse proxy를 반드시 사용합니다. 구현 기준은 [MCP 2025-11-25 specification](https://modelcontextprotocol.io/specification/2025-11-25), [transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports), [authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)을 참조하십시오.

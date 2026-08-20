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

## 관리자 설정

실행 시 필요한 네 환경변수 외의 설정은 모두 `/admin`에서 관리합니다.

- 서비스: 표시명, 공개 URL, 허용 origin, Bootstrap 로그인, 플레이 정책 기준 timezone(기본 `Asia/Seoul`)
- 인증: Keycloak issuer/client와 claim/role mapping
- AI: OpenAI-compatible base URL/model/key, timeout, 기본 streaming 호출, max token 상한(최대 262,144)
- 키: 역할별 scope, 개인 키 만료/회전/폐기, MCP/API 접근
- 워크플로: 게임 등록·변경에 대한 팀장/운영자 검토와 승인/반려
- 개인정보: 랭킹 실명/닉네임, 조직 공개, 사용자 랭킹 opt-out
- 플레이 정책: 허용 시간, 일일 제한, 게임별 예외
- 브라우저 연결 정책: iframe과 API 연결 허용 origin

승인 워크플로가 꺼져 있으면 지원되는 게임 생성·변경 요청을 바로 반영합니다. 켜져 있을 때만 pending 요청을 만들고, reviewer 역할 또는 `manager_required` 설정에 따라 팀장/관리자가 승인·반려합니다. 기본적으로 요청자는 자기 요청을 검토할 수 없고 팀장 역할은 양쪽에 팀 정보가 있으면 같은 팀의 요청만 검토할 수 있습니다. 반려 사유는 필수이며 모든 전이는 감사 로그에 남깁니다.

## AI 운영

브라우저는 공급자 API를 직접 호출하지 않습니다. 서버가 자격 증명을 복호화해 호출하며 기본 응답은 SSE streaming입니다. `max_tokens`는 최대 262,144이며 요청이 관리자 설정 상한을 넘으면 거부합니다. 공급자 자체 상한은 관리자가 그보다 작게 설정해야 합니다. 연결 해제 시 upstream 요청을 취소하며 서버가 AI 요청을 자동 재시도하지는 않습니다.

AI가 비활성 또는 미설정이면 AI 게임과 메뉴는 숨기고 일반 게임 기능은 정상 운영합니다.

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
| 토큰 검증 실패 | issuer/audience, JWKS 접근, 시계 오차, Keycloak key rotation |
| AI 응답 중단 | provider 접근, timeout, SSE buffering, 모델 token 상한 |
| 게임 iframe 차단 | allowlist, CSP frame-src, 게임의 frame-ancestors/X-Frame-Options |
| 점수 미반영 | session token/소유권, 같은 세션의 중복 점수, 게임별 점수·시간 규칙 |

## 프록시

TLS 프록시는 원래 `Host`, `X-Forwarded-Proto: https`, 신뢰할 수 있는 client address를 전달해야 합니다. SSE 경로는 proxy buffering을 끄고 idle timeout을 AI 최대 응답 시간보다 길게 둡니다. 업로드 본문과 WebSocket을 허용할 경우 경로별 한도와 timeout을 별도로 설정합니다.

## 용량 계획과 보존

초기 권장은 2 vCPU, RAM 2 GiB에서 시작하고 실제 DAU와 AI 동시 stream을 기준으로 조정하는 것입니다. 현재 내장된 자동 보존/삭제 job은 없으므로 감사 로그, telemetry와 게임 세션의 보존 기간을 조직 정책으로 정하고 승인된 DB 유지보수 절차를 마련합니다. 점수/시즌 확정 기록과 승인 기록의 법적·조직 정책을 우선하고 삭제 실행 이력도 별도로 보존합니다.

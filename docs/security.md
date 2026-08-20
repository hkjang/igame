# 보안 및 키 관리

## 경계와 기본값

igame은 브라우저를 신뢰하지 않습니다. 점수, 업적, 승인 전이, 역할, AI 호출은 모두 서버에서 검증합니다. 컨테이너는 non-root, read-only root filesystem, 모든 Linux capability 제거, `no-new-privileges`로 실행합니다. 인터넷/CDN 의존성은 없습니다. 제공된 Compose 파일은 egress를 차단하지 않으므로 운영 방화벽이나 컨테이너 network policy에서 PostgreSQL, Keycloak과 승인된 AI endpoint만 허용해야 합니다. 별도 URL 게임 origin은 사용자 브라우저 망에서 제한합니다.

운영 TLS는 reverse proxy에서 종료할 수 있지만 proxy와 igame 사이도 신뢰되지 않은 구간이면 TLS/mTLS를 적용합니다. 세션 cookie는 Secure, HttpOnly, SameSite 정책을 사용하고 상태 변경 요청에는 CSRF 방어를 적용합니다.

## 세 가지 키 계층

1. `ENCRYPTION_KEY`: 인스턴스 마스터 키. DB의 OIDC/AI secret을 인증 암호화합니다. 정확히 32바이트이며 별도 비밀 관리소에 보관합니다.
2. 공급자 secret: Keycloak client secret, AI API key 등. 관리자만 등록·교체할 수 있고 저장 후 평문 조회는 불가합니다.
3. 개인 API/MCP 키: 사용자별로 충분한 entropy로 발급하며 원문은 한 번만 보여 줍니다. 서버에는 SHA-256 검증값과 prefix, owner, scope, 만료, 최근 사용 시각만 저장합니다.

개인 키는 사용자 페이지에서 생성·회전·폐기합니다. 회전 동작은 새 키 발급과 동시에 이전 키를 폐기합니다. 무중단 전환이 필요하면 별도 새 키를 만든 뒤 consumer를 전환하고 이전 키를 폐기합니다. 관리자는 역할별 허용 scope, 최대 유효기간과 동시 활성 키 수를 변경할 수 있습니다. 기존 키의 실제 권한도 매 요청 시 저장 scope와 현재 전역·역할 정책의 교집합으로 다시 계산되므로 정책 축소와 역할 변경이 즉시 적용됩니다. 다른 사용자의 원문 키는 관리자도 볼 수 없습니다.

권장 scope 예:

- `api:access`, `mcp:access`, `profile:read`, `games:read`, `rankings:read`
- `sessions:write`, `scores:write`
- `ai:invoke`, `workflow:write`
- `admin:*`(관리자 역할만)

기본 발급은 read-only와 필요한 최소 write scope만 허용합니다. 관리자 scope는 일반 개인 키에 추가할 수 없게 분리하는 것이 권장됩니다.

## Bootstrap 관리자

bootstrap 계정은 최초 설정과 SSO 장애 복구용입니다. 최초 로그인 직후 프로필의 비밀번호 변경 기능으로 현재 암호를 12자 이상의 새 암호로 바꾸면 현재 것을 제외한 다른 session도 폐기됩니다. DB에 같은 사용자가 이미 있으면 기존 암호 hash는 덮어쓰지 않지만 매 기동 시 `admin`/`active` 상태를 유지합니다. `.env`의 bootstrap 암호도 빈 DB 복구용 별도 임의 값으로 교체해 비밀 관리소에 보관합니다. 애플리케이션에는 로그인 실패 rate limiter가 내장되어 있지 않으므로 reverse proxy에서 제한하고 인증 실패 급증을 경보로 감시합니다.

## 점수와 게임 격리

현재 점수 검증은 server-issued session token, 사용자 소유권, 세션 상태, 한 세션당 한 점수, 게임별 최소/최대 점수와 최소/최대 플레이 시간을 확인합니다. 경쟁 강도가 높은 게임에는 SDK 서명만 신뢰하지 말고 server-side event sequence 또는 replay 검증과 이상 탐지 절차를 추가하는 것이 권장됩니다.

iframe 게임은 서비스 설정의 허용 origin과 reverse proxy CSP를 함께 사용하고 sandbox 권한을 최소화해야 합니다. 임의 game URL은 내부 관리망 노출 위험이 있으므로 운영자가 HTTPS, DNS/IP 범위와 redirect 목적지를 검토하고 필요하면 승인 워크플로를 적용합니다. 외부 게임 구현은 브라우저 메시지의 정확한 `targetOrigin`과 schema를 검증해야 합니다.

## AI

AI API key는 서버 밖으로 노출하지 않습니다. 애플리케이션은 absolute HTTP(S) endpoint만 받으므로 운영 firewall/proxy allowlist로 SSRF egress를 제한해야 합니다. 서버는 timeout과 관리자 `max_tokens`를 적용하며 검증 상한은 262,144입니다. 공급자의 더 작은 상한을 관리자 값에 반영하고 프롬프트/응답 로그는 최소화합니다.

## 승인과 감사

승인 기능이 꺼져 있으면 지원되는 게임 생성·변경 요청을 바로 반영하고 별도 검토 상태를 만들지 않습니다. 켜져 있으면 reviewer 역할을 검사하며 `manager_required` 정책으로 팀장 또는 관리자의 검토를 요구할 수 있습니다. `separation_of_duties`는 기본적으로 켜져 있고 요청자 본인의 검토를 서버에서 거부합니다. 팀장 역할은 요청자와 검토자 양쪽에 팀 정보가 있으면 같은 팀의 요청만 검토할 수 있으며, 팀 정보가 비어 있으면 이 비교를 적용할 수 없으므로 Keycloak team claim mapping과 사용자 정보를 함께 관리해야 합니다. 반려에는 사유가 필수입니다. 애플리케이션의 인증, 설정, workflow, 키, AI 호출 등 중요 동작은 비밀 원문을 제외한 메타데이터를 감사 로그에 남깁니다. backup/restore 실행 기록은 운영 시스템에서 별도로 보존합니다.

## 운영 점검표

- 이미지 digest와 SHA-256, SBOM 검토 후 반입
- `.env` 0600, Docker socket 및 backup 최소 권한
- PostgreSQL TLS/최소 권한/별도 DB/정기 restore 시험
- Keycloak issuer/audience/redirect exact match, 짧은 토큰 수명
- 관리자와 개인 키 정기 회전, 퇴사자 즉시 폐기
- CORS/Origin/CSP allowlist, rate limit, body size 제한
- 로그와 지원 번들의 token/secret/PII 마스킹 확인
- 호스트/Keycloak/PostgreSQL NTP 동기화

MCP HTTP transport는 Origin을 검증하고 인증을 필수화하며, OAuth 토큰을 다른 upstream에 전달하지 않습니다. 상세 프로토콜 기준은 [MCP Authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)과 [Transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)를 따릅니다.

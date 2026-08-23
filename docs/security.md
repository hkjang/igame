# 보안 및 키 관리

## 경계와 기본값

igame은 브라우저를 신뢰하지 않습니다. 점수, 업적, 승인 전이, 역할, AI 호출은 모두 서버에서 검증합니다. 최종 컨테이너는 package manager와 shell이 없는 `scratch` runtime에서 non-root, read-only root filesystem, 모든 Linux capability 제거, `no-new-privileges`로 실행합니다. `/tmp`만 Compose tmpfs로 제공하고 헬스체크는 정적 Go binary의 `healthcheck` mode로 수행합니다. 릴리스 CI는 최종 이미지에 High/Critical 취약점이 있으면 중단합니다. 인터넷/CDN 의존성은 없습니다. 제공된 Compose 파일은 egress를 차단하지 않으므로 운영 방화벽이나 컨테이너 network policy에서 PostgreSQL, Keycloak과 승인된 AI endpoint만 허용해야 합니다. 별도 URL 게임 origin은 사용자 브라우저 망에서 제한합니다.

운영 TLS는 reverse proxy에서 종료할 수 있지만 proxy와 igame 사이도 신뢰되지 않은 구간이면 TLS/mTLS를 적용합니다. 세션 cookie는 Secure, HttpOnly, SameSite 정책을 사용하고 상태 변경 요청에는 CSRF 방어를 적용합니다. 요청이 HTTPS로 제공된다고 설정(`public_url` 또는 `trust_proxy`가 켜진 상태의 forwarded header)이 확인해 줄 때만 `Strict-Transport-Security`를 보냅니다. 신뢰하지 않는 forwarded header가 평문 배포에 HSTS를 고정하지 못하게 하기 위해서입니다.

## 세 가지 키 계층

1. `ENCRYPTION_KEY`: 인스턴스 마스터 키. DB의 OIDC/AI secret을 인증 암호화합니다. 정확히 32바이트이며 별도 비밀 관리소에 보관합니다.
2. 공급자 secret: Keycloak client secret, AI API key 등. 관리자만 등록·교체할 수 있고 저장 후 평문 조회는 불가합니다.
3. 개인 API/MCP 키: 사용자별로 충분한 entropy로 발급하며 원문은 한 번만 보여 줍니다. 서버에는 SHA-256 검증값과 prefix, owner, scope, 만료, 최근 사용 시각만 저장합니다.

개인 키는 사용자 페이지에서 생성·회전·폐기합니다. 회전 동작은 새 키 발급과 동시에 이전 키를 폐기합니다. 무중단 전환이 필요하면 별도 새 키를 만든 뒤 consumer를 전환하고 이전 키를 폐기합니다. 관리자는 역할별 허용 scope, 최대 유효기간과 동시 활성 키 수를 변경할 수 있습니다. 기존 키의 실제 권한도 매 요청 시 저장 scope와 현재 전역·역할 정책의 교집합으로 다시 계산되므로 정책 축소와 역할 변경이 즉시 적용됩니다. `profile:write`는 읽기 scope와 분리되며 기존 키의 저장 scope에 자동 추가되지 않으므로 개인 변경 기능이 필요한 키에만 브라우저 개인화 페이지에서 명시적으로 부여합니다. 다른 사용자의 원문 키는 관리자도 볼 수 없습니다.

권장 scope 예:

- `api:access`, `mcp:access`, `profile:read`, `profile:write`, `games:read`, `rankings:read`
- `sessions:write`, `scores:write`
- `ai:invoke`, `workflow:write`
- `admin:*`(관리자 역할만)

기본 발급은 read-only와 필요한 최소 write scope만 허용합니다. 관리자 scope는 일반 개인 키에 추가할 수 없게 분리하는 것이 권장됩니다.

## Bootstrap 관리자

bootstrap 계정은 최초 설정과 SSO 장애 복구용입니다. 최초 로그인 직후 프로필의 비밀번호 변경 기능으로 현재 암호를 12자 이상의 새 암호로 바꾸면 현재 것을 제외한 다른 session도 폐기됩니다. DB에 같은 사용자가 이미 있으면 기존 암호 hash는 덮어쓰지 않지만 매 기동 시 `admin`/`active` 상태를 유지합니다. `.env`의 bootstrap 암호도 빈 DB 복구용 별도 임의 값으로 교체해 비밀 관리소에 보관합니다. 로컬 암호 로그인에는 rate limiter가 내장되어 있습니다. 같은 계정+출발지 조합은 15분 안에 10회, 같은 출발지는 계정과 무관하게 50회 실패하면 남은 시간 동안 429와 `Retry-After`로 거부하고 `local sign-in throttled` 경고를 남깁니다. 성공하면 해당 counter는 초기화됩니다. 알 수 없는 계정도 실제 계정과 같은 비용의 bcrypt 비교를 수행하므로 응답 시간으로 계정 존재 여부를 알아낼 수 없습니다. counter는 인스턴스 메모리에 있고 재기동 시 사라지므로 다중 인스턴스나 장기 차단이 필요하면 reverse proxy 제한을 함께 적용하고 인증 실패 급증을 경보로 감시합니다.

감사 로그 CSV 내보내기는 admin 역할만 사용할 수 있고 실행 자체가 `audit.export`로 기록됩니다. 내보낸 셀 중 `=`, `+`, `-`, `@`, 탭, CR로 시작하는 값은 spreadsheet 수식 주입을 막기 위해 앞에 작은따옴표를 붙입니다. user agent와 대상 ID는 외부 입력이 섞일 수 있는 필드입니다.

## 점수와 게임 격리

현재 점수 검증은 server-issued session token, 사용자 소유권, 세션 상태, 한 세션당 한 점수, 게임별 최소/최대 점수와 최소/최대 플레이 시간을 확인합니다. 경쟁 강도가 높은 게임에는 SDK 서명만 신뢰하지 말고 server-side event sequence 또는 replay 검증과 이상 탐지 절차를 추가하는 것이 권장됩니다.

RealmGuard는 이 일반 점수·랭킹 경로를 차단하고 세션에 고정된 immutable 콘텐츠 snapshot으로 결과를 검증합니다. 세션 생성 시 클라이언트가 방금 읽은 published config의 `version.id`를 `realmguard_version_id`로 제출해야 하며 누락·stale pin은 428/409로 fail-closed합니다. 전투 event는 최상위 UUID `client_event_id`와 세션별 1부터 연속 증가하는 1~100,000 `sequence`를 사용하며 event `data`는 최대 4 KiB입니다. 원장 DoS 경계는 선택 event 128, ready/complete 각 1, wave start 10,001, wave complete 10,000, tower build/upgrade/sell 합계 10,000의 독립 class 한도입니다. 선택 event 포화가 필수 milestone 용량을 소모하지 않습니다. 콘텐츠 ID는 제한된 1~32자 ASCII grammar, 일반 enemy 10~16종, boss 2~4종, wave entry 8개와 기본 spawn 500 상한으로 묶고 최악 조건의 누적 histogram JSON도 4 KiB gate를 통과해야 합니다. Endless 10,000 wave의 기본·파생 최대 spawn도 signed 32-bit counter 범위로 제한합니다. 서버는 실제 수신한 ready/wave start·complete/battle complete의 순서·receipt time, 누적 적별 histogram, tower 경제 원장과 결과를 대조하고 mode/stage/difficulty, version tuple, 경과 시간, lives, spawn/kill/escape, 시작금·보상·지출·판매, hero와 전투 레벨을 검사한 뒤 score/star/progress를 한 transaction으로 확정합니다. 같은 event 재전송은 ID와 전체 payload가 같을 때만 idempotent하고, sequence gap/재사용 충돌은 거부합니다. 같은 세션/token의 성공 결과 재전송은 저장된 결과만 반환하며 다른 payload로 새 점수를 만들지 않습니다.

Defense Series 세 game도 일반 score/ranking을 차단하고 전용 결과 경로를 사용합니다. session 생성 시 해당 slug의 현재 published config UUID를 `defense_content_version_id`로 보내야 하며 누락은 428, stale·unpublished·다른 slug UUID는 409로 거부합니다. 결과, 진행도, 랭킹과 학습 기록은 session owner·slug·pin이 모두 일치할 때만 생성하고 published UUID가 바뀌면 이전 snapshot의 기록을 새 progress/report에 섞지 않습니다. Office Guardians의 session/result를 Cyber Fortress나 AI Nexus Defense 전용 경로에 제출할 수 없습니다. 서버는 UUID/1-based sequence로 실제 수신한 ready/wave/battle milestone의 순서·시각, 누적 적 histogram과 경제 원장을 게시 stage/wave budget 및 최종 결과와 대조해 score/star를 재계산합니다. Event `data`는 최대 4 KiB이며 Studio Test/Publish도 최대 ID·counter·histogram·version과 AI resource state를 넣은 stage별 누적 snapshot을 실제 직렬화해 같은 한도를 넘는 pack을 차단합니다. 누락·gap·변조 원장, body-only perfect 결과, 조작된 zero-wave 패배와 불가능한 health/spawn/승리/시간은 거부합니다. AI Nexus의 공식 패배는 정확한 네 metric 자원 원장에서 실제 소진된 값이 있을 때만 health가 남은 상태를 허용하며, 서버는 wave·적 통과·교육·model profile 비용과 포화된 잔액을 재계산합니다. 거부된 Defense 결과는 RealmGuard의 `realmguard.result.reject`와 같은 방식으로 `defense.result.reject` 감사 항목을 남기며 slug별 telemetry report의 `rejected_results`로 집계됩니다. `server_received_telemetry_v1`은 이 브라우저 자가보고 원장의 서버 수신 일관성 검증이며 browser frame을 완전히 재실행하거나 실제 플레이를 암호학적으로 증명하지는 않으므로 운영 이상 탐지를 병행합니다. 교육 답안은 pinned content에 실제로 존재하는 event/answer만 받으며 client가 보낸 정답 여부나 학습 점수는 신뢰하지 않습니다. Public config/preview/bundle은 정답 mapping과 해설을 포함하지 않고, 정답 여부와 해설은 도달한 event의 답안 제출 뒤에만 반환합니다. Preview는 `practice_only`이고 공식 결과·학습 완료를 만들지 않습니다.

`server_received_telemetry_v1` attestation은 인증된 session token으로 브라우저가 자가 보고한 event를 서버가 수신해 순서·시간·누적 원장 일관성을 검증했다는 보장입니다. 서버가 게임 프레임, 적 이동과 충돌을 독립적으로 완전 재실행한 시뮬레이션/replay 증명은 아닙니다. 브라우저의 로컬 score, 결과 payload의 client `proof`와 호환용 `events` 묶음은 공식 증거로 신뢰하거나 저장하지 않으며, 서버가 attestation digest 기반 암호화 receipt를 생성합니다. 운영자는 이 경계를 전제로 이상 탐지와 경쟁 기록의 사후 검토를 병행합니다.

iframe 게임은 서비스 설정의 허용 origin과 reverse proxy CSP를 함께 사용하고 sandbox 권한을 최소화해야 합니다. 임의 game URL은 내부 관리망 노출 위험이 있으므로 운영자가 HTTPS, DNS/IP 범위와 redirect 목적지를 검토하고 필요하면 승인 워크플로를 적용합니다. 외부 게임 구현은 브라우저 메시지의 정확한 `targetOrigin`과 schema를 검증해야 합니다.

## AI

AI API key는 서버 밖으로 노출하지 않습니다. 애플리케이션은 absolute HTTP(S) endpoint만 받으므로 운영 firewall/proxy allowlist로 SSRF egress를 제한해야 합니다. 서버는 timeout과 관리자 `max_tokens`를 적용하며 검증 상한은 262,144입니다. 공급자의 더 작은 상한을 관리자 값에 반영하고 프롬프트/응답 로그는 최소화합니다.

## 승인과 감사

승인 기능이 꺼져 있으면 지원되는 게임 생성·변경 요청을 바로 반영하고 별도 검토 상태를 만들지 않습니다. 켜져 있으면 reviewer 역할을 검사하며 `manager_required` 정책으로 팀장 또는 관리자의 검토를 요구할 수 있습니다. `separation_of_duties`는 기본적으로 켜져 있고 요청자 본인의 검토를 서버에서 거부합니다. 일반 게임의 직접 변경과 workflow 승인 적용은 내장 RealmGuard와 세 Defense game의 예약 slug를 가져가거나 변경하지 못하게 막습니다. 일반 workflow, RealmGuard Designer와 Defense Content Studio에서 Manager에게 team이 없으면 pending 목록을 거부하고, 목록에는 team이 비어 있지 않은 동일 team 작성자의 항목만 포함합니다. 직접 review와 preview도 Manager와 작성자 어느 한쪽이라도 team이 누락되면 `team_required`, 서로 다르면 `different_team`으로 fail-closed하므로 Keycloak team claim mapping과 사용자 정보를 함께 관리해야 합니다. 반려에는 사유가 필수입니다. 애플리케이션의 인증, 설정, workflow, 키, AI 호출 등 중요 동작은 비밀 원문을 제외한 메타데이터를 감사 로그에 남깁니다. backup/restore 실행 기록은 운영 시스템에서 별도로 보존합니다.

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

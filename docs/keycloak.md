# Keycloak OIDC 연동

Keycloak 설정은 환경변수가 아니라 `/admin`의 **시스템 설정 → 인증 → Keycloak OIDC**에 저장됩니다. client secret은 `ENCRYPTION_KEY`로 봉인되고 평문으로 다시 표시되지 않습니다. OIDC가 비활성 또는 미설정이고 서비스 정책에서 Bootstrap 로그인을 켠 경우 로컬 관리자로 접근할 수 있습니다.

## Keycloak client 생성

예시는 서비스 공개 주소가 `https://igame.company.local`인 경우입니다.

1. 전용 realm에서 OpenID Connect client `igame`을 만듭니다.
2. Client authentication을 켜 confidential client로 설정합니다.
3. Standard flow만 켜고 Implicit flow, Direct access grants, Service accounts는 필요하지 않으면 끕니다.
4. Valid redirect URI에 `https://igame.company.local/api/v1/auth/oidc/callback`을 정확히 등록합니다.
5. 조직 정책상 Web origin을 등록해야 한다면 정확한 `https://igame.company.local`만 허용하고 광범위한 `*` origin은 사용하지 않습니다. 현재 igame 로그아웃은 로컬 세션만 종료하므로 Keycloak post-logout redirect는 사용하지 않습니다.
6. 기본 scope는 `openid profile email`이며 역할 연동 시 `groups` claim mapper를 추가합니다.

Issuer URL은 realm의 주소입니다.

```text
https://keycloak.company.local/realms/<realm>
```

igame은 issuer의 `/.well-known/openid-configuration`을 이용해 authorization, token과 JWKS endpoint를 자동 발견합니다. hostname이 인증서 SAN 및 issuer 값과 정확히 같아야 합니다. Authorization Code flow에는 state, nonce와 PKCE S256 검증을 적용합니다.

## igame 관리자 설정

로컬 관리자로 로그인해 다음 값만 입력합니다.

| 항목 | 값 |
| --- | --- |
| 활성화 | 시험 성공 후 켬 |
| Issuer URL | Keycloak realm URL |
| Client ID | `igame` 등 등록 ID |
| Client secret | Keycloak credentials에서 발급한 값 |
| Scopes | `openid profile email` 및 선택 claim scope |
| Username claim | 일반적으로 `preferred_username` |
| Display name claim | 일반적으로 `name` |
| Email claim | 일반적으로 `email` |
| Groups claim | 일반적으로 `groups` |
| Department claim | 일반적으로 `department` |
| Team claim | 일반적으로 `team` |
| 관리자 그룹 | `admin` 역할로 매핑할 Keycloak group 값 목록 |
| 운영자 그룹 | `operator` 역할로 매핑할 Keycloak group 값 목록 |
| 팀장 그룹 | `manager` 역할로 매핑할 Keycloak group 값 목록 |

서비스 공개 URL과 허용 origin은 별도의 **시스템 설정 → 서비스**에서 관리합니다. 설정한 `public_url`이 redirect URI의 기준이며, 비어 있으면 요청 scheme/host를 사용합니다. 신뢰할 수 있는 reverse proxy 뒤에서는 `trust_proxy`를 켜고 `X-Forwarded-Host`와 `X-Forwarded-Proto`를 정확히 전달해야 합니다.

설정을 저장한 뒤 현재 로컬 관리자 세션을 유지하고 새 private 창에서 시험 로그인하십시오. 성공한 Keycloak group을 시스템 관리자 역할에 매핑한 상태를 확인해야 관리자 잠금을 피할 수 있습니다.

## 역할 매핑

설정한 group claim은 `admin`, `operator`, `manager` 그룹 목록과 비교하고 어느 쪽에도 해당하지 않으면 `user`로 provisioning됩니다. 여러 목록에 일치할 때 우선순위는 `admin → operator → manager`이며 OIDC 사용자의 역할은 로그인 때 다시 계산됩니다. `department`와 `team`도 지정한 claim에서 갱신됩니다. 승인 queue는 소속별 reviewer에게 자동 배정하지 않지만, 팀장 역할의 검토는 요청자와 검토자 양쪽에 team 값이 있으면 같은 팀으로 제한됩니다. 조직별 분리를 사용할 때는 team claim mapping의 누락을 함께 점검합니다.

## Secret 회전

1. 유지보수 창과 로컬 관리자 세션을 확보합니다.
2. Keycloak에서 새 client secret을 생성합니다.
3. igame 관리자 화면에 즉시 저장한 뒤 새 SSO 로그인을 확인합니다.
4. 감사 로그에서 변경자와 시각을 확인합니다.

client secret은 로그, 지원 번들, API 응답에 포함되면 안 됩니다.

## MCP 접근

현재 `/mcp` 자동화 인증은 개인화 페이지에서 발급한 범위 제한 개인 API 키를 사용합니다. Keycloak은 브라우저 SSO와 사용자 provisioning을 담당하며 Keycloak access token을 MCP Bearer token으로 직접 받지 않습니다.

## 장애 복구

Keycloak 장애 시 기존 애플리케이션 session은 만료될 때까지 사용할 수 있지만 새 SSO 로그인은 실패합니다. Bootstrap 로그인을 비상 절차로 유지한다면 네트워크와 강한 암호로 제한합니다. 이를 껐다면 DB 운영자가 `service.bootstrap_login_enabled`를 복구하는 승인된 break-glass 절차를 별도로 준비해야 합니다. issuer 변경, CA 만료, 시간 오차, redirect URI 불일치 순서로 점검합니다.

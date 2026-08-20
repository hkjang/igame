# igame

`igame`은 사내 폐쇄망에서 게임을 독립적으로 등록하고 운영하는 게임 플랫폼입니다. Go 모듈러 모놀리스, React 포털, PostgreSQL, Keycloak OIDC, Game Runtime/SDK, 랭킹·시즌·업적·이벤트, 선택형 검토/승인 흐름, 개인 키와 MCP/API를 한 개의 Docker 이미지로 제공합니다.

## 기술 선택

- Backend: Go, PostgreSQL, REST/SSE, MCP Streamable HTTP
- Frontend: React + TypeScript + Vite, Phaser(RealmGuard runtime)
- UI: 접근성과 장기 유지보수가 검증된 Material UI(MUI)를 사용하고 애플리케이션 자산은 번들에 포함합니다. 본문 기준 16px, 100~125% 개인 글자 확대, 키보드 포커스와 대비를 유지합니다.
- Deployment: `igame:v<version>` 단일 이미지. 외부 CDN이나 실행 중 패키지 다운로드가 없습니다.

초기에는 PostgreSQL만 사용하는 모듈러 모놀리스로 운영하고, 동시 접속 규모가 커질 때에만 실시간 런타임이나 랭킹을 분리하는 것이 권장됩니다.

## RealmGuard

`v0.2.0`은 igame의 독자 타워 디펜스 IP인 **RealmGuard**를 포함합니다. 10개 캠페인 stage와 끝없는 균열, 일반 enemy 10종과 boss 2종, 4종 tower와 8개 upgrade branch, 3명 hero, 3개 active skill을 데이터 기반으로 구성합니다. `/games/realmguard`의 게임 화면은 Phaser를 사용하지만 엔진과 모든 실행 자산은 Vite/Go 정적 bundle과 단일 Docker 이미지 안에 포함되어 폐쇄망에서 CDN 없이 실행됩니다.

RealmGuard의 명칭·등장 개체·stage/balance 데이터와 코드 생성 그래픽은 이 프로젝트의 독자 구현이며, Kingdom Rush를 포함한 제3자 게임의 코드·서사·캐릭터·맵·시청각 자산을 포함하지 않습니다. Phaser는 MIT 라이선스의 실행 framework로만 사용하며 package metadata와 license를 이미지 `/licenses/phaser`에 함께 보관합니다.

관리자는 `/admin/realmguard`의 Designer에서 checksum/`If-Match`로 콘텐츠 초안을 편집·Test·게시하고, 승인 정책을 켜면 `/reviews`에서 미리보기 후 승인·반려합니다. Manager 검토는 manager/작성자 모두 같은 비어 있지 않은 team일 때만 열립니다. 게임 세션은 화면이 읽은 published config UUID를 다시 확인해 pin하며, 공식 전투 결과는 UUID와 1-based sequence로 서버가 수신한 브라우저 자가보고 원장의 순서·시각·누적 일관성을 `server_received_telemetry_v1`으로 검증합니다. 이는 완전한 서버 게임 시뮬레이션은 아닙니다. 게시 version, 검증 경계, telemetry와 운영·백업 절차는 [RealmGuard 운영 가이드](docs/realmguard.md)를 참조하세요.

## 빠른 시작

PostgreSQL 데이터베이스를 먼저 준비한 뒤 `.env.example`을 `.env`로 복사하고 네 값을 채웁니다. 애플리케이션이 받는 환경변수는 다음 네 개뿐입니다.

| 변수 | 용도 |
| --- | --- |
| `POSTGRES_DSN` | PostgreSQL 연결 문자열 |
| `BOOTSTRAP_ADMIN` | 최초 로컬 관리자 ID |
| `BOOTSTRAP_ADMIN_PASSWORD` | 최초 로컬 관리자 암호 |
| `ENCRYPTION_KEY` | 저장 비밀을 감싸는 32바이트 마스터 키 |

```bash
umask 077
cp .env.example .env
docker compose up -d
docker compose ps
bash ./scripts/smoke-test.sh http://127.0.0.1:8080
```

`ENCRYPTION_KEY` 생성 예:

```bash
printf 'ENCRYPTION_KEY=base64:%s\n' "$(openssl rand -base64 32 | tr -d '\n')"
```

최초 로그인 후 `/admin`에서 공개 URL, Keycloak, AI 공급자, 개인정보, 시간 정책, 승인 흐름, 역할과 키 권한을 설정합니다. 설정이 없는 승인 흐름은 실행 경로 자체에서 제외됩니다. 서비스 버전은 로그인 화면과 프로필 메뉴에서 확인할 수 있습니다.

상세 설치 절차는 [오프라인 설치](docs/offline-install.md), SSO는 [Keycloak 연동](docs/keycloak.md)을 따릅니다.

## 개발과 검증

```bash
make test
make build
make docker-build
make smoke
```

릴리스 이미지는 `VERSION`을 기준으로 만듭니다. 결과물 `dist/igame-v<version>.tar.gz`는 별도 tar 포장 없이 `docker save igame:v<version> | gzip`의 출력입니다.

서비스, Docker image, web application, RealmGuard content bundle과 `gamehub-js` SDK는 이 release에서 root `VERSION` `0.2.0`으로 정렬됩니다. SDK의 server-authoritative completion API도 `gamehub-js` `0.2.0`에 포함됩니다.

```bash
make release
bash ./scripts/verify-release.sh dist/igame-v$(cat VERSION).tar.gz
gzip -dc dist/igame-v$(cat VERSION).tar.gz | docker load
```

## 문서

- [오프라인 설치](docs/offline-install.md)
- [Keycloak OIDC](docs/keycloak.md)
- [운영 및 장애 대응](docs/operations.md)
- [백업과 복구](docs/backup-restore.md)
- [보안 및 키 관리](docs/security.md)
- [REST/SSE API](docs/api.md)
- [MCP](docs/mcp.md)
- [RealmGuard 운영 가이드](docs/realmguard.md)
- [릴리스 절차](docs/release.md)

## 라이선스

Apache License 2.0. 자세한 내용은 [LICENSE](LICENSE)를 참조하세요.

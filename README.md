# igame

`igame`은 사내 폐쇄망에서 게임을 독립적으로 등록하고 운영하는 게임 플랫폼입니다. Go 모듈러 모놀리스, React 포털, PostgreSQL, Keycloak OIDC, Game Runtime/SDK, 랭킹·시즌·업적·이벤트, 선택형 검토/승인 흐름, 개인 키와 MCP/API를 한 개의 Docker 이미지로 제공합니다.

## 기술 선택

- Backend: Go, PostgreSQL, REST/SSE, MCP Streamable HTTP
- Frontend: React + TypeScript + Vite
- UI: 접근성과 장기 유지보수가 검증된 Material UI(MUI)를 사용하고 애플리케이션 자산은 번들에 포함합니다. 본문 기준 16px, 100~125% 개인 글자 확대, 키보드 포커스와 대비를 유지합니다.
- Deployment: `igame:v<version>` 단일 이미지. 외부 CDN이나 실행 중 패키지 다운로드가 없습니다.

초기에는 PostgreSQL만 사용하는 모듈러 모놀리스로 운영하고, 동시 접속 규모가 커질 때에만 실시간 런타임이나 랭킹을 분리하는 것이 권장됩니다.

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
- [릴리스 절차](docs/release.md)

## 라이선스

Apache License 2.0. 자세한 내용은 [LICENSE](LICENSE)를 참조하세요.

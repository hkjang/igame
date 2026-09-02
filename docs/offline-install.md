# 오프라인 설치

## 배포 구성

릴리스 자산은 `igame-vX.Y.Z.tar.gz` 하나입니다. 이 파일은 `igame:vX.Y.Z`를 `docker save`한 스트림을 gzip으로 압축한 것이므로 `docker load` 후에도 이미지 이름과 태그가 그대로 유지됩니다. PostgreSQL과 사내 DNS, TLS 종단 프록시는 운영 환경의 공통 인프라로 준비하며 릴리스에 포함하지 않습니다.

지원 기준은 Linux x86-64 호스트, Docker Engine 24 이상, Compose plugin 2.20 이상, PostgreSQL 15 이상입니다. 컨테이너 호스트와 Keycloak/PostgreSQL의 시간이 동기화되어야 OIDC 토큰 검증이 정상 동작합니다.

## 반입 전 준비

인터넷 연결 구간에서 다음을 수행합니다.

1. GitHub Release에서 대상 버전의 `igame-vX.Y.Z.tar.gz`를 받습니다.
2. 릴리스 작업 로그 또는 릴리스 노트의 SHA-256과 로컬 계산값을 대조합니다.
3. 조직의 악성코드 검사, 이미지 취약점 검사, SBOM 검토 절차를 완료합니다.
4. 이미지 파일, 승인된 `docker-compose.yml`, 이 문서를 매체에 복사합니다.

```bash
sha256sum igame-vX.Y.Z.tar.gz
gzip -t igame-vX.Y.Z.tar.gz
```

GitHub 워크플로는 SPDX JSON SBOM을 생성해 검증 증적으로 보관하지만, “Docker 이미지만 릴리스” 원칙에 따라 SBOM과 checksum 파일을 별도 Release asset으로 게시하지 않습니다.

`v0.7.3` image의 RealmGuard와 Defense Series 세 게임의 Phaser runtime, 코드 생성 캐릭터·전장 graphic, 콘텐츠도 같은 image layer에 포함됩니다. 최종 runtime은 package manager와 shell이 없는 `scratch` 기반이며 정적 Go binary, CA 신뢰 번들과 license evidence만 포함합니다. 반입 검사에서는 `/licenses/web`과 `/licenses/sdk/gamehub-js`의 locked package metadata, `/licenses/phaser`의 package metadata/MIT license, workflow SBOM의 Phaser·SDK version을 함께 확인합니다. Workflow evidence에는 reachable Go 취약점 검사, High/Critical 최종 이미지 검사와 binary의 Go 1.26.6 build info 검증도 포함됩니다. 브라우저가 Phaser, map, sprite, 교육 문제 또는 balance 데이터를 인터넷에서 내려받지 않습니다.

## PostgreSQL 준비

전용 데이터베이스와 최소 권한 소유자를 생성하고 `UTF8` 인코딩을 사용합니다. 애플리케이션은 시작 시 포함된 마이그레이션을 적용하므로 해당 데이터베이스의 schema 생성·변경 권한이 필요합니다. 다른 데이터베이스에 대한 권한은 부여하지 않습니다.

운영에서는 TLS를 켜고 다음과 같이 명시적으로 검증합니다.

```text
postgres://igame:<password>@postgres.internal:5432/igame?sslmode=verify-full
```

폐쇄망 PostgreSQL CA는 이미지의 `/etc/ssl/certs/ca-certificates.crt` 신뢰 번들에 포함되어야 합니다. 사설 CA가 기본 번들에 없다면 연결 구간에서 공개 CA와 조직 승인 CA를 합친 단일 PEM bundle을 준비하고 `FROM igame:vX.Y.Z` 파생 이미지에서 같은 경로로 `COPY`하십시오. 파생 최종 stage도 원본의 `scratch` runtime을 계승하므로 package manager를 추가하지 않으며, 새 이미지에 대해 checksum·SBOM·취약점·TLS 연결 검증을 다시 수행합니다. `sslmode=disable`은 개발 환경에만 허용합니다.

## 이미지 로드와 설정

```bash
gzip -dc igame-vX.Y.Z.tar.gz | docker load
docker image inspect igame:vX.Y.Z --format '{{json .RepoTags}}'
```

작업 디렉터리에 버전과 일치하는 `docker-compose.yml`을 두고, 권한이 제한된 `.env`를 만듭니다.

```bash
umask 077
touch .env
chmod 600 .env
```

`.env`에는 아래 네 줄만 둡니다. 셸 특수문자를 포함하는 값은 Compose의 변수 치환 규칙에 맞게 이스케이프해야 하며, 가능하면 URL 구성 요소를 percent-encoding 합니다.

```dotenv
POSTGRES_DSN=postgres://igame:...@postgres.internal:5432/igame?sslmode=verify-full
BOOTSTRAP_ADMIN=admin
BOOTSTRAP_ADMIN_PASSWORD=<long-random-password>
ENCRYPTION_KEY=base64:<base64-of-exactly-32-random-bytes>
```

`BOOTSTRAP_ADMIN_PASSWORD`는 16자 이상의 고유한 임의 값으로 만들고 최초 로그인 직후 프로필에서 실제 계정 비밀번호를 변경합니다. 이후 `.env` 값도 빈 DB 복구에 사용할 별도 임의 값으로 교체해 비밀 관리소에 보관합니다. 기존 DB의 암호는 환경변수로 다시 덮어쓰지 않습니다. `ENCRYPTION_KEY`는 아래처럼 생성하고 서비스 데이터 백업과 분리된 비밀 관리소에 보관합니다.

```bash
openssl rand -base64 32 | tr -d '\n'
```

값 앞에 `base64:`를 붙여야 합니다. 평문 형식은 정확히 32바이트일 때만 허용됩니다.

## 기동과 확인

```bash
docker compose up -d
docker compose ps
docker compose logs --tail=100 igame
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

- 컨테이너 내부 healthcheck: `/app/igame healthcheck`. 환경변수나 DB 초기화를 다시 수행하지 않고 실행 중인 localhost endpoint만 확인합니다.
- `/healthz`: 프로세스 생존 여부. 외부 의존성 장애와 분리합니다.
- `/readyz`: PostgreSQL과 필수 초기화가 요청을 받을 수 있는 상태인지 확인합니다.

브라우저에서 로그인 화면과 프로필 컨텍스트 메뉴의 버전이 이미지 태그와 같은지 확인합니다. 이후 로컬 관리자로 로그인해 비밀번호를 변경하고, 공개 URL과 Keycloak을 설정해 private 창에서 시험 로그인한 다음 활성화합니다. SSO 복구 절차를 확보하기 전에는 Bootstrap 관리자 로그인을 끄지 않습니다.

## 네트워크 허용 목록

igame 컨테이너에서 필요한 통신은 PostgreSQL, Keycloak issuer/JWKS와 관리자가 등록한 AI API입니다. 별도 URL 게임은 사용자 브라우저가 허용된 game origin에 접속하므로 클라이언트 망에서도 해당 목적지를 허용해야 합니다. JavaScript, CSS와 아이콘은 이미지에 포함되고 글꼴은 운영체제의 한국어 system font stack을 사용하므로 외부 CDN 연결이 필요하지 않습니다. 인바운드는 TLS 프록시에서 애플리케이션 8080으로 전달하고 `/admin`, `/api`, `/mcp`에도 동일한 인증·속도 제한 정책을 적용합니다.

## 제거

컨테이너 제거는 데이터베이스를 삭제하지 않습니다. `docker compose down --volumes`는 로컬 볼륨의 업로드 자산을 삭제하므로 백업·복구 승인이 없는 상태에서는 실행하지 마십시오.

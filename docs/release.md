# 릴리스

## 불변 계약

- 서비스명: `igame`
- 이미지: `igame:v<VERSION>`
- 대상 platform: `linux/amd64`
- GitHub Release asset: `igame-v<VERSION>.tar.gz` 단 하나
- asset 내용: `docker save igame:v<VERSION> | gzip` 출력 자체
- `VERSION` 파일에는 앞의 `v` 없이 semantic version 기록
- 런타임 환경변수: `POSTGRES_DSN`, `BOOTSTRAP_ADMIN`, `BOOTSTRAP_ADMIN_PASSWORD`, `ENCRYPTION_KEY`만 사용

SBOM과 checksum은 CI에서 생성·검증하고 workflow evidence artifact와 job summary에 남깁니다. 별도 release asset으로 올리거나 이미지 archive 내부에 삽입하지 않습니다. 일반 CI와 Release workflow는 Web·SDK lockfile을 `npm audit --audit-level=low`로 검사하고, 정확히 고정한 `govulncheck v1.6.0`으로 reachable Go 취약점을 검사합니다. SDK build chain은 Windows 개발 서버 경로 탐색 advisory를 피하도록 esbuild `0.27.2`를 root override로 고정합니다. Builder는 `golang:1.26.6-alpine3.23`이며, 최종 이미지는 package manager와 shell이 없는 `scratch`입니다. 최종 이미지에는 정적 `/app/igame`, CA 신뢰 번들, 오프라인 검토용 license metadata만 포함합니다. 두 workflow 모두 고정된 Anchore scan action과 Grype `v0.117.0`으로 최종 이미지를 검사해 High/Critical 발견 시 중단하며, `io.igame.build.go-version` 라벨과 추출한 `/app/igame`의 `go version -m`이 모두 `go1.26.6`인지 확인합니다. 이미지 `User`는 `10001:10001`이며 Compose는 read-only filesystem, `cap_drop: ALL`, `no-new-privileges` 계약을 유지합니다. Docker healthcheck는 shell utility 대신 `/app/igame healthcheck`를 실행합니다.

Production build는 RealmGuard와 Defense Series source가 locked Phaser dependency를 실제로 import하는지, compiled asset에 네 게임 route/content가 존재하는지, `index.html`이 원격 script/stylesheet를 참조하지 않는지를 검사합니다. Web과 SDK package version은 root `VERSION`과 같아야 합니다. 최종 이미지에는 locked web package manifest/lock을 `/licenses/web`, SDK manifest/lock을 `/licenses/sdk/gamehub-js`, Phaser package metadata와 MIT license를 `/licenses/phaser`에 보관합니다. SPDX SBOM에서 정확한 Phaser·`@igame/gamehub-js` version이 식별되지 않으면 Release workflow를 중단합니다. `003_realmguard.sql`, `004_realmguard_attestation.sql`과 Defense Series schema/seed migration을 포함한 SQL migration은 Go binary에 embed됩니다. fresh DB에서 세 Defense content pack이 각각 하나의 published snapshot으로 생성되고, 기존 RealmGuard 경제 필드는 `bigint`를 유지해야 합니다.

서비스 릴리스와 게임 콘텐츠는 서로 다른 수명 주기를 가집니다. `igame:v0.4.1`에는 RealmGuard 콘텐츠 `0.3.0`과 Defense Series 콘텐츠 `0.3.0`이 함께 들어갑니다. Web·SDK·이미지 버전은 root `VERSION`을 따르지만, 기존 게임 콘텐츠 버전을 서비스 버전으로 덮어쓰지 않습니다. 공식 세션은 각 게임 public config에서 읽은 immutable snapshot UUID를 사용합니다.

## 매뉴얼 PDF 재생성

`docs/`의 세 PDF는 `docs/guide.md`, `docs/cru-manual.md`, `docs/architecture.md`에서 생성합니다. 이전에는 생성 파이프라인이 저장소에 없어 제품이 바뀌어도 다시 만들 수 없었습니다.

```bash
make docs-pdf
```

Release browser gate와 같은 `mcr.microsoft.com/playwright:v1.55.0-noble` 컨테이너에서 Chromium 인쇄로 만듭니다. Playwright는 여기서도 repository dependency가 아니라 컨테이너 안에서만 설치합니다. 컨테이너에는 한글 글꼴이 없어 빌드 시 `fonts-noto-cjk`를 설치하며, 렌더링된 page가 외부 자원을 참조하면 인쇄를 중단합니다. 표지의 서비스 버전은 root `VERSION`을 따릅니다. 발행일은 기본적으로 빌드 당일이며 `DOCS_PDF_DATE=YYYY-MM-DD`로 고정할 수 있습니다.

## 로컬 후보 생성

Docker daemon을 준비한 다음 실행합니다. Syft가 있으면 SPDX JSON도 만들고, 없으면 단일 archive와 그 checksum을 만들며 경고합니다. GitHub CI에서는 Syft가 필수로 설치됩니다.

```bash
make test
make check-offline-bundle
make release
ls -lh dist/
bash ./scripts/verify-release.sh dist/igame-v$(cat VERSION).tar.gz
```

검증기는 gzip 무결성, Docker `manifest.json`, 보존된 `igame:v<version>` RepoTag와 checksum을 확인합니다. 동일 태그를 먼저 제거한 깨끗한 검증 환경에서 다음 load/smoke test도 수행합니다.

```bash
docker image rm "igame:v$(cat VERSION)"
gzip -dc dist/igame-v$(cat VERSION).tar.gz | docker load
docker image inspect "igame:v$(cat VERSION)"
```

Release workflow는 clean-load한 후보 이미지의 API smoke 직후, 앱 컨테이너를 정리하기 전에 Playwright `v1.55.0-noble` 컨테이너로 브라우저 gate를 자동 실행합니다. npm package도 `playwright@1.55.0`으로 정확히 고정하며 설치 또는 browser smoke가 실패하면 Release 게시를 중단합니다. Playwright는 제품 이미지나 repository dependency에 들어가지 않습니다. 아래 명령은 같은 gate를 QA host에서 수동 재현하는 방법입니다. 서비스 페이지가 로그인부터 RealmGuard와 Defense Series 세 게임 canvas, 교육 선택, 각 Designer/Studio와 연습 preview의 deep refresh까지 외부 HTTP 요청·console/page/request 오류 없이 동작하는지 검사합니다. 편집 화면은 앞선 API smoke가 남긴 editable draft를 반드시 열어 유효한 JSON과 활성화된 저장 버튼을 확인하며, invalid JSON 또는 빈 데이터 상태가 렌더링되면 실패합니다.

```bash
export IGAME_BASE_URL='http://127.0.0.1:8080'
export IGAME_USERNAME='admin'
export IGAME_REQUIRE_DESIGNER_DRAFT=true
export IGAME_REQUIRE_DEFENSE_DRAFT=true
mkdir -p /tmp/igame-v03-screenshots
read -r -s -p 'Bootstrap password: ' IGAME_PASSWORD; export IGAME_PASSWORD
docker run --rm --network host \
  -e IGAME_BASE_URL -e IGAME_USERNAME -e IGAME_PASSWORD -e IGAME_REQUIRE_DESIGNER_DRAFT -e IGAME_REQUIRE_DEFENSE_DRAFT \
  -e IGAME_SCREENSHOT_DIR=/screenshots \
  -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
  -v /tmp/igame-v03-screenshots:/screenshots \
  -v "$PWD:/work:ro" -w /tmp \
  mcr.microsoft.com/playwright:v1.55.0-noble \
  bash -lc 'mkdir -p browser-smoke && cd browser-smoke && npm init -y >/dev/null && npm install --silent --no-save playwright@1.55.0 && node /work/scripts/browser-smoke.mjs'
unset IGAME_PASSWORD
```

`/tmp/igame-v03-screenshots`에는 홈, 세 Defense 전장/교육·AI HUD, Content Studio의 로컬 검수 이미지가 남습니다. 이 증적은 사람이 시각 검수할 때만 사용하며 Docker image archive나 GitHub Release asset에 포함하지 않습니다.

## GitHub

`.github/workflows/release.yml`은 다음 진입점을 지원합니다.

- `v*` tag push
- Actions의 수동 실행(`version` 입력)

tag는 반드시 `v$(cat VERSION)`과 같아야 합니다. workflow는 test, RealmGuard/Defense Series/Phaser offline bundle, image build, SBOM의 Phaser 식별, archive 구조, clean-daemon load, PostgreSQL readiness, Bootstrap 로그인·비밀번호 회전과 주요 관리자 API를 검증합니다. 이미지가 fresh DB에 migration을 적용한 직후 published seed validator도 다시 실행합니다. RealmGuard의 기존 권위 결과·telemetry·Designer 회귀 smoke를 그대로 수행한 뒤 Defense Series smoke를 추가로 실행합니다. Defense smoke는 세 slug의 published config/version/progress, `defense_content_version_id` 누락 428·stale 409·정상 pin, 전용 결과/랭킹과 일반 score/ranking 우회 차단, 4 KiB ledger/content budget, 세 난이도의 실제 검증 결과, 교육 이벤트 답변, AI 자원 소진, 업적과 학습 결과, published UUID별 progress/ranking/report 격리, Studio `If-Match` 428/409, policy/source rollback Draft, Test/연습 preview/승인/반려/게시, telemetry와 learning report를 fresh DB에서 확인합니다. 승인 정책을 켠 경우 Manager 검토는 작성자와 같은 비어 있지 않은 team에서만 허용되며, 설정이 꺼진 경우 검토 단계를 만들지 않는 것도 확인합니다. 이 검증을 마친 뒤 GitHub Release를 생성하거나 갱신하고 오직 `.tar.gz` 하나만 asset으로 게시합니다. archive와 SBOM SHA-256은 Release 본문과 job summary에, SBOM 및 checksum 파일은 보존 기간이 정해진 workflow evidence에 기록합니다.

권장 순서:

```bash
version="$(cat VERSION)"
git tag -s "v${version}" -m "igame v${version}"
git push origin "v${version}"
```

보호 규칙으로 signed tag, 승인된 environment, Actions 변경 review를 요구하는 것이 좋습니다. Release workflow에서 외부 registry로 이미지를 push하지 않습니다.

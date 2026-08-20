# 릴리스

## 불변 계약

- 서비스명: `igame`
- 이미지: `igame:v<VERSION>`
- 대상 platform: `linux/amd64`
- GitHub Release asset: `igame-v<VERSION>.tar.gz` 단 하나
- asset 내용: `docker save igame:v<VERSION> | gzip` 출력 자체
- `VERSION` 파일에는 앞의 `v` 없이 semantic version 기록
- 런타임 환경변수: `POSTGRES_DSN`, `BOOTSTRAP_ADMIN`, `BOOTSTRAP_ADMIN_PASSWORD`, `ENCRYPTION_KEY`만 사용

SBOM과 checksum은 CI에서 생성·검증하고 workflow evidence artifact와 job summary에 남깁니다. 별도 release asset으로 올리거나 이미지 archive 내부에 삽입하지 않습니다.

`v0.2.0`부터 production build는 RealmGuard source가 locked Phaser dependency를 실제로 import하는지, compiled asset에 RealmGuard route/content가 존재하는지, `index.html`이 원격 script/stylesheet를 참조하지 않는지를 검사합니다. Web과 SDK package version은 root `VERSION`과 같아야 합니다. 최종 이미지에는 locked web package manifest/lock을 `/licenses/web`, SDK manifest/lock을 `/licenses/sdk/gamehub-js`, Phaser package metadata와 MIT license를 `/licenses/phaser`에 보관합니다. SPDX SBOM에서 정확한 Phaser·`@igame/gamehub-js` version이 식별되지 않으면 Release workflow를 중단합니다. `003_realmguard.sql` 콘텐츠 schema/seed와 `004_realmguard_attestation.sql` 결과 attestation·ordered telemetry migration을 포함한 SQL migration은 Go binary에 embed됩니다. fresh DB의 `003` 정의와 기존 DB에 적용되는 `004 ALTER` 모두 remaining/earned/spent/sold gold를 `bigint`로 보장하는지 함께 확인합니다.

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

Release workflow는 clean-load한 후보 이미지의 API/RealmGuard smoke 직후, 앱 컨테이너를 정리하기 전에 Playwright `v1.55.0-noble` 컨테이너로 브라우저 gate를 자동 실행합니다. npm package도 `playwright@1.55.0`으로 정확히 고정하며 설치 또는 browser smoke가 실패하면 Release 게시를 중단합니다. Playwright는 제품 이미지나 repository dependency에 들어가지 않습니다. 아래 명령은 같은 gate를 QA host에서 수동 재현하는 방법입니다. 서비스 페이지가 로그인부터 RealmGuard canvas, Designer, preview와 deep refresh까지 외부 HTTP 요청·console/page/request 오류 없이 동작하는지 검사합니다. Designer는 앞선 RealmGuard API smoke가 남긴 editable draft를 반드시 열어 stages JSON array 11개 이상과 활성화된 저장 버튼을 확인하며, invalid JSON 또는 빈 데이터 상태가 렌더링되면 실패합니다.

```bash
export IGAME_BASE_URL='http://127.0.0.1:8080'
export IGAME_USERNAME='admin'
export IGAME_REQUIRE_DESIGNER_DRAFT=true
read -r -s -p 'Bootstrap password: ' IGAME_PASSWORD; export IGAME_PASSWORD
docker run --rm --network host \
  -e IGAME_BASE_URL -e IGAME_USERNAME -e IGAME_PASSWORD -e IGAME_REQUIRE_DESIGNER_DRAFT \
  -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
  -v "$PWD:/work:ro" -w /tmp \
  mcr.microsoft.com/playwright:v1.55.0-noble \
  bash -lc 'mkdir -p browser-smoke && cd browser-smoke && npm init -y >/dev/null && npm install --silent --no-save playwright@1.55.0 && node /work/scripts/browser-smoke.mjs'
unset IGAME_PASSWORD
```

## GitHub

`.github/workflows/release.yml`은 다음 진입점을 지원합니다.

- `v*` tag push
- Actions의 수동 실행(`version` 입력)

tag는 반드시 `v$(cat VERSION)`과 같아야 합니다. workflow는 test, RealmGuard/Phaser offline bundle, image build, SBOM의 Phaser 식별, archive 구조, clean-daemon load, PostgreSQL readiness, Bootstrap 로그인·비밀번호 회전과 주요 관리자 API를 검증합니다. RealmGuard smoke는 `/games/realmguard`, 게시 config/version/progress, 세션 `realmguard_version_id` 누락 428·stale 409·정상 pin, 전용 랭킹 filter와 일반 랭킹 `409`, 일반 score 차단, 조작된 0-wave 결과 거부, UUID/1-based telemetry의 duplicate·sequence conflict·4 KiB 및 class별 한도, optional 128개 포화 뒤의 ready 예약 슬롯, ready/wave/battle 누적 histogram, `server_received_telemetry_v1` attestation, client proof/events 무시, 도달 가능한 패배 transaction과 결과 idempotency, 콘텐츠 ID/적 roster/최악 histogram budget, wave entry·기본 spawn 상한과 endless 10,000-wave 32-bit counter budget, 예약 RealmGuard slug의 직접·workflow 변경 차단, 일반/Designer manager team fail-closed, Designer `If-Match` 428/409, Test/preview/승인/반려/telemetry를 fresh DB에서 확인합니다. 결과 smoke의 milestone·제출 duration은 게시된 `balance.min_wave_duration_ms`에서 계산해 서버 receipt 및 session wall-time 검증까지 통과해야 합니다. 이 검증을 마친 뒤 GitHub Release를 생성하거나 갱신하고 오직 `.tar.gz` 하나만 asset으로 게시합니다. archive와 SBOM SHA-256은 Release 본문과 job summary에, SBOM 및 checksum 파일은 보존 기간이 정해진 workflow evidence에 기록합니다.

권장 순서:

```bash
version="$(cat VERSION)"
git tag -s "v${version}" -m "igame v${version}"
git push origin "v${version}"
```

보호 규칙으로 signed tag, 승인된 environment, Actions 변경 review를 요구하는 것이 좋습니다. Release workflow에서 외부 registry로 이미지를 push하지 않습니다.

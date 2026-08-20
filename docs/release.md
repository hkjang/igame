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

## 로컬 후보 생성

Docker daemon을 준비한 다음 실행합니다. Syft가 있으면 SPDX JSON도 만들고, 없으면 단일 archive와 그 checksum을 만들며 경고합니다. GitHub CI에서는 Syft가 필수로 설치됩니다.

```bash
make test
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

## GitHub

`.github/workflows/release.yml`은 다음 진입점을 지원합니다.

- `v*` tag push
- Actions의 수동 실행(`version` 입력)

tag는 반드시 `v$(cat VERSION)`과 같아야 합니다. workflow는 test, image build, SBOM 생성, archive 구조, clean-daemon load, PostgreSQL readiness, Bootstrap 로그인·비밀번호 회전과 주요 관리자 API 검증을 수행한 후 GitHub Release를 생성하거나 갱신하고 오직 `.tar.gz` 하나만 asset으로 게시합니다. archive와 SBOM SHA-256은 Release 본문과 job summary에, SBOM 및 checksum 파일은 보존 기간이 정해진 workflow evidence에 기록합니다.

권장 순서:

```bash
version="$(cat VERSION)"
git tag -s "v${version}" -m "igame v${version}"
git push origin "v${version}"
```

보호 규칙으로 signed tag, 승인된 environment, Actions 변경 review를 요구하는 것이 좋습니다. Release workflow에서 외부 registry로 이미지를 push하지 않습니다.

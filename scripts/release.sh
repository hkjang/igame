#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
readonly IMAGE="igame:v${VERSION}"
readonly OUT_DIR="${REPO_DIR}/dist"
readonly ARCHIVE="${OUT_DIR}/igame-v${VERSION}.tar.gz"
readonly SBOM="${OUT_DIR}/igame-v${VERSION}.spdx.json"
readonly CHECKSUMS="${OUT_DIR}/SHA256SUMS"

if [[ ! "${VERSION}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
  printf 'VERSION must be a semantic version without a leading v: %s\n' "${VERSION}" >&2
  exit 1
fi

for command_name in docker gzip sha256sum; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "${command_name}" >&2
    exit 1
  fi
done

bash "${SCRIPT_DIR}/check-release-contract.sh"

mkdir -p "${OUT_DIR}"
tmp_archive="$(mktemp "${OUT_DIR}/.igame-image.XXXXXX")"
trap 'rm -f -- "${tmp_archive}"' EXIT

commit="$(git -C "${REPO_DIR}" rev-parse --short=12 HEAD 2>/dev/null || printf unknown)"
build_date="${SOURCE_DATE_EPOCH:-$(date -u +%s)}"
build_date="$(date -u -d "@${build_date}" '+%Y-%m-%dT%H:%M:%SZ')"

printf 'Building %s\n' "${IMAGE}"
docker build \
  --platform linux/amd64 \
  --build-arg "VERSION=${VERSION}" \
  --build-arg "COMMIT=${commit}" \
  --build-arg "BUILD_DATE=${build_date}" \
  --label "org.opencontainers.image.version=${VERSION}" \
  --label "org.opencontainers.image.revision=${commit}" \
  --label "org.opencontainers.image.created=${build_date}" \
  --tag "${IMAGE}" \
  "${REPO_DIR}"

printf 'Writing docker save stream to %s\n' "${ARCHIVE}"
docker save "${IMAGE}" | gzip -n -9 > "${tmp_archive}"
mv -f -- "${tmp_archive}" "${ARCHIVE}"

sbom_generated=false
rm -f -- "${SBOM}"
if command -v syft >/dev/null 2>&1; then
  syft "${IMAGE}" --output "spdx-json=${SBOM}"
  sbom_generated=true
elif docker help sbom >/dev/null 2>&1; then
  docker sbom --format spdx-json --output "${SBOM}" "${IMAGE}"
  sbom_generated=true
else
  printf '%s\n' 'Warning: Syft is unavailable; the archive is valid but no local SBOM was generated.' >&2
fi

(
  cd "${OUT_DIR}"
  checksum_files=("$(basename -- "${ARCHIVE}")")
  if [[ "${sbom_generated}" == true ]]; then
    checksum_files+=("$(basename -- "${SBOM}")")
  fi
  sha256sum "${checksum_files[@]}" > "${CHECKSUMS}"
)

bash "${SCRIPT_DIR}/verify-release.sh" "${ARCHIVE}"
printf 'Release archive and verification evidence created in %s\n' "${OUT_DIR}"

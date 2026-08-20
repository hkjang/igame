#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 3 ]]; then
  printf 'Usage: %s VERSION COMMIT BUILD_DATE\n' "$0" >&2
  exit 2
fi

readonly VERSION="$1"
readonly COMMIT="$2"
readonly BUILD_DATE="$3"
readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly BUILD_ROOT="$(mktemp -d)/source"

cleanup() {
  rm -rf -- "$(dirname -- "${BUILD_ROOT}")"
}
trap cleanup EXIT

mkdir -p "${BUILD_ROOT}" "${REPO_DIR}/bin"
cp "${REPO_DIR}/go.mod" "${REPO_DIR}/go.sum" "${BUILD_ROOT}/"
cp -R "${REPO_DIR}/cmd" "${REPO_DIR}/internal" "${REPO_DIR}/migrations" "${BUILD_ROOT}/"
rm -rf -- "${BUILD_ROOT}/internal/web/dist"
mkdir -p "${BUILD_ROOT}/internal/web/dist"
cp -R "${REPO_DIR}/web/dist/." "${BUILD_ROOT}/internal/web/dist/"

(
  cd "${BUILD_ROOT}"
  CGO_ENABLED=0 go build \
    -buildvcs=false \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/hkjang/igame/internal/version.Version=${VERSION} \
      -X github.com/hkjang/igame/internal/version.Commit=${COMMIT} \
      -X github.com/hkjang/igame/internal/version.BuildDate=${BUILD_DATE}" \
    -o "${REPO_DIR}/bin/igame" ./cmd/igame
)

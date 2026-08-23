#!/usr/bin/env bash
# Rebuilds the three enterprise manuals under docs/ from their Markdown sources.
#
# The published PDFs previously came from a pipeline that was never committed,
# so they could not be regenerated when the product changed. This script is that
# pipeline: Markdown in the repository, Chromium print in the same pinned
# container the release browser gate already uses, and no network asset in the
# rendered page.
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
readonly PLAYWRIGHT_IMAGE='mcr.microsoft.com/playwright:v1.55.0-noble'
# Reproducible unless the caller pins it: release evidence should not change
# just because the file was rebuilt on a different day.
BUILD_DATE="${DOCS_PDF_DATE:-$(date -u '+%Y-%m-%d')}"
readonly BUILD_DATE

# source markdown : output pdf : footer title
readonly MANUALS=(
  "guide.md:igame_User_Guide.pdf:사용자 가이드"
  "cru-manual.md:igame_CRU_Operations_Manual.pdf:CRU 운영 매뉴얼"
  "architecture.md:igame_Architecture_and_Security_Whitepaper.pdf:아키텍처 및 보안 백서"
)

command -v docker >/dev/null || { printf '%s\n' 'docker is required to run the pinned Chromium container' >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf -- "${work}"' EXIT

manifest="${work}/manifest.json"
printf '[' >"${manifest}"
first=1
for entry in "${MANUALS[@]}"; do
  IFS=':' read -r source pdf title <<<"${entry}"
  [[ -f "${REPO_DIR}/docs/${source}" ]] || { printf 'missing source: docs/%s\n' "${source}" >&2; exit 1; }
  node "${SCRIPT_DIR}/docs-pdf/render.mjs" \
    "${REPO_DIR}/docs/${source}" "${work}/${source%.md}.html" "${VERSION}" "${BUILD_DATE}"
  [[ ${first} -eq 1 ]] || printf ',' >>"${manifest}"
  first=0
  printf '{"html":"/work/%s.html","pdf":"/out/%s","title":"%s"}' "${source%.md}" "${pdf}" "${title}" >>"${manifest}"
done
printf ']' >>"${manifest}"

# The container ships no Korean font — its CJK coverage is Chinese and a bitmap
# fallback — so Hangul would print with the wrong glyphs without this.
docker run --rm \
  --volume "${work}:/work" \
  --volume "${REPO_DIR}/docs:/out" \
  --volume "${SCRIPT_DIR}/docs-pdf:/pipeline:ro" \
  --workdir /tmp \
  "${PLAYWRIGHT_IMAGE}" \
  bash -lc '
    set -Eeuo pipefail
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null
    apt-get install -y -qq fonts-noto-cjk fonts-noto-color-emoji >/dev/null
    fc-cache -f >/dev/null 2>&1 || true
    mkdir -p /tmp/print && cd /tmp/print
    npm init -y >/dev/null
    npm install --silent --no-save playwright@1.55.0
    node /pipeline/print.mjs /work/manifest.json '"${VERSION}"'
  '

printf '\n'
for entry in "${MANUALS[@]}"; do
  IFS=':' read -r _ pdf _ <<<"${entry}"
  printf '  %-52s %s\n' "${pdf}" "$(du -h "${REPO_DIR}/docs/${pdf}" | cut -f1)"
done
printf '\nManuals rebuilt for igame v%s (%s).\n' "${VERSION}" "${BUILD_DATE}"

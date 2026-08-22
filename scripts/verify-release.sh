#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
readonly EXPECTED_IMAGE="igame:v${VERSION}"
readonly ARCHIVE="${1:-${REPO_DIR}/dist/igame-v${VERSION}.tar.gz}"

for command_name in gzip tar; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "${command_name}" >&2
    exit 1
  fi
done

if [[ ! -f "${ARCHIVE}" ]]; then
  printf 'Release archive does not exist: %s\n' "${ARCHIVE}" >&2
  exit 1
fi

gzip -t "${ARCHIVE}"
manifest="$(gzip -dc -- "${ARCHIVE}" | tar -xOf - manifest.json)"

if [[ "${manifest}" != *"\"${EXPECTED_IMAGE}\""* ]]; then
  printf 'Docker archive does not retain expected image tag %s\n' "${EXPECTED_IMAGE}" >&2
  exit 1
fi

readonly ARCHIVE_DIR="$(dirname -- "${ARCHIVE}")"
readonly ARCHIVE_NAME="$(basename -- "${ARCHIVE}")"
readonly CHECKSUM_FILE="${ARCHIVE_DIR}/SHA256SUMS"

if [[ -f "${CHECKSUM_FILE}" ]]; then
  checksum_entry="$(
    awk -v target="${ARCHIVE_NAME}" '
      length($1) == 64 && ($2 == target || $2 == "*" target) {
        print
        matches++
      }
      END { if (matches != 1) exit 1 }
    ' "${CHECKSUM_FILE}"
  )" || {
    printf 'SHA256SUMS must contain exactly one entry for %s\n' "${ARCHIVE_NAME}" >&2
    exit 1
  }
  (
    cd "${ARCHIVE_DIR}"
    printf '%s\n' "${checksum_entry}" | sha256sum --check --strict -
  )
fi

printf 'Verified gzip-compressed docker save archive: %s (%s)\n' "${ARCHIVE}" "${EXPECTED_IMAGE}"

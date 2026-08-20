#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
readonly COMPOSE_FILE="${REPO_DIR}/docker-compose.yml"
readonly ENV_EXAMPLE="${REPO_DIR}/.env.example"

if [[ ! "${VERSION}" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
  printf 'Invalid VERSION: %s\n' "${VERSION}" >&2
  exit 1
fi

if ! grep -Fq "image: igame:v${VERSION}" "${COMPOSE_FILE}"; then
  printf 'docker-compose.yml image must be igame:v%s\n' "${VERSION}" >&2
  exit 1
fi

actual_keys="$({
  sed -n '/^[[:space:]]\{4\}environment:/,/^[[:space:]]\{4\}volumes:/p' "${COMPOSE_FILE}" \
    | sed -n 's/^[[:space:]]\{6\}\([A-Z][A-Z0-9_]*\):.*/\1/p'
} | sort)"
expected_keys="$(printf '%s\n' BOOTSTRAP_ADMIN BOOTSTRAP_ADMIN_PASSWORD ENCRYPTION_KEY POSTGRES_DSN | sort)"

if [[ "${actual_keys}" != "${expected_keys}" ]]; then
  printf '%s\n' 'Runtime environment contract mismatch.' >&2
  printf 'Expected:\n%s\nActual:\n%s\n' "${expected_keys}" "${actual_keys}" >&2
  exit 1
fi

example_keys="$(sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' "${ENV_EXAMPLE}" | sort)"
if [[ "${example_keys}" != "${expected_keys}" ]]; then
  printf '%s\n' '.env.example must contain exactly the four runtime settings.' >&2
  printf 'Expected:\n%s\nActual:\n%s\n' "${expected_keys}" "${example_keys}" >&2
  exit 1
fi

if grep -Eq '^[[:space:]]+env_file:' "${COMPOSE_FILE}"; then
  printf '%s\n' 'docker-compose.yml must not inject an additional env_file.' >&2
  exit 1
fi

if grep -Eq '^[[:space:]]*ENV[[:space:]]' "${REPO_DIR}/Dockerfile"; then
  printf '%s\n' 'Dockerfile must not define additional application environment settings.' >&2
  exit 1
fi

if ! grep -Fq 'docker save "${IMAGE}" | gzip' "${REPO_DIR}/scripts/release.sh"; then
  printf '%s\n' 'release.sh must gzip the docker save stream directly.' >&2
  exit 1
fi

if ! grep -Fq -- '--platform linux/amd64' "${REPO_DIR}/scripts/release.sh"; then
  printf '%s\n' 'release.sh must build the documented linux/amd64 image.' >&2
  exit 1
fi

printf 'Release contract verified for igame:v%s\n' "${VERSION}"

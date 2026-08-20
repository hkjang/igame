#!/usr/bin/env bash
set -Eeuo pipefail

readonly BASE_URL="${1:-http://127.0.0.1:8080}"

if command -v curl >/dev/null 2>&1; then
  http_get() { curl --fail --silent --show-error --max-time 10 "$1"; }
elif command -v wget >/dev/null 2>&1; then
  http_get() { wget --quiet --timeout=10 --output-document=- "$1"; }
else
  printf '%s\n' 'curl or wget is required.' >&2
  exit 1
fi

http_get "${BASE_URL}/healthz" >/dev/null
http_get "${BASE_URL}/readyz" >/dev/null
http_get "${BASE_URL}/" >/dev/null
printf 'Smoke test passed: %s\n' "${BASE_URL}"

#!/usr/bin/env bash
# Rebuilds docs/assets/guide/*.png — the screen captures docs/USER_GUIDE.md and
# docs/ADMIN_GUIDE.md embed.
#
# The captures have to show a populated portal, so this script fills the target
# service with invented users, scores, notices, events and a season before it
# starts the browser. That makes it a destructive tool, and it is guarded
# accordingly:
#
#   * the target comes from IGAME_GUIDE_CAPTURE_URL, a variable no other script
#     in this repository reads, so a value left over from a smoke run cannot
#     aim it somewhere else;
#   * it refuses to run unless IGAME_GUIDE_CAPTURE_DISPOSABLE=yes says the
#     service and its database may be thrown away afterwards;
#   * it never writes a system setting, so it cannot change how a service is
#     configured — the play policy, the privacy policy and the approval flow of
#     whatever it is pointed at come back untouched.
#
# The seeding and the browser run inside containers, so the URL and the DSN are
# resolved from there, not from this shell. Set IGAME_GUIDE_CAPTURE_NETWORK to
# the Docker network the service and its database are on; it defaults to `host`,
# which is what a Linux host with the service bound to 127.0.0.1 needs.
#
# Usage — service and database on a dedicated network, as docs/ADMIN_GUIDE.md
# describes for a throwaway install:
#   IGAME_GUIDE_CAPTURE_URL=http://igame:8080 \
#   IGAME_GUIDE_CAPTURE_USER=admin@example.internal \
#   IGAME_GUIDE_CAPTURE_PASSWORD=… \
#   IGAME_GUIDE_CAPTURE_DSN='postgres://igame:…@postgres:5432/igame?sslmode=disable' \
#   IGAME_GUIDE_CAPTURE_NETWORK=igame-guide \
#   IGAME_GUIDE_CAPTURE_DISPOSABLE=yes \
#   bash ./scripts/capture-guide-screenshots.sh
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly OUTPUT_DIR="${REPO_DIR}/docs/assets/guide"
readonly PLAYWRIGHT_IMAGE='mcr.microsoft.com/playwright:v1.55.0-noble'
readonly POSTGRES_IMAGE='postgres:17-alpine'

fail() { printf '%s\n' "$1" >&2; exit 1; }

[[ "${IGAME_GUIDE_CAPTURE_DISPOSABLE:-}" == 'yes' ]] || fail \
  'Refusing to run: this script seeds demo content into the target service and its database.
Set IGAME_GUIDE_CAPTURE_DISPOSABLE=yes only when both may be discarded afterwards.'
url="${IGAME_GUIDE_CAPTURE_URL:-}"
[[ -n "${url}" ]] || fail 'Set IGAME_GUIDE_CAPTURE_URL to the disposable service to capture.'
user="${IGAME_GUIDE_CAPTURE_USER:-}"
password="${IGAME_GUIDE_CAPTURE_PASSWORD:-}"
dsn="${IGAME_GUIDE_CAPTURE_DSN:-}"
[[ -n "${user}" && -n "${password}" ]] || fail 'Set IGAME_GUIDE_CAPTURE_USER and IGAME_GUIDE_CAPTURE_PASSWORD.'
[[ -n "${dsn}" ]] || fail 'Set IGAME_GUIDE_CAPTURE_DSN to the same disposable database the service uses.'
network="${IGAME_GUIDE_CAPTURE_NETWORK:-host}"
command -v docker >/dev/null 2>&1 || fail 'Required command not found: docker'

printf 'Seeding demo users and scores into the disposable database…\n'
docker run --rm --interactive --network "${network}" \
  --env "PGCONNECT_TIMEOUT=10" \
  "${POSTGRES_IMAGE}" psql "${dsn}" --set ON_ERROR_STOP=1 --quiet \
  <"${SCRIPT_DIR}/guide-capture/seed.sql"

printf 'Capturing screens at 1440x900…\n'
mkdir -p "${OUTPUT_DIR}"
docker run --rm --network "${network}" \
  --env "IGAME_GUIDE_CAPTURE_URL=${url%/}" \
  --env "IGAME_GUIDE_CAPTURE_USER=${user}" \
  --env "IGAME_GUIDE_CAPTURE_PASSWORD=${password}" \
  --env 'IGAME_GUIDE_CAPTURE_DIR=/out' \
  --env PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
  --env "IGAME_GUIDE_CAPTURE_OWNER=$(id -u):$(id -g)" \
  --volume "${REPO_DIR}/scripts:/work/scripts:ro" \
  --volume "${OUTPUT_DIR}:/out" \
  --workdir /tmp \
  "${PLAYWRIGHT_IMAGE}" \
  bash -lc '
    set -Eeuo pipefail
    export DEBIAN_FRONTEND=noninteractive
    # The image ships no Korean font — its CJK coverage is Chinese — so every
    # Hangul label in the portal would be captured with the wrong glyphs.
    # scripts/build-docs-pdf.sh installs the same font for the same reason.
    apt-get update -qq >/dev/null
    apt-get install -y -qq fonts-noto-cjk >/dev/null
    fc-cache -f >/dev/null 2>&1 || true
    mkdir -p /tmp/capture && cd /tmp/capture
    npm init -y >/dev/null
    npm install --silent --no-save playwright@1.55.0
    node /work/scripts/guide-capture/capture.mjs
    chown -R "${IGAME_GUIDE_CAPTURE_OWNER}" /out
  '

printf 'Screen captures written to docs/assets/guide.\n'

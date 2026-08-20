#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DEFAULT_REPO_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
REPO_DIR=${1:-${DEFAULT_REPO_DIR}}
WEB_DIR=${REPO_DIR}/web
SDK_DIR=${REPO_DIR}/sdk/gamehub-js
DIST_DIR=${WEB_DIR}/dist
VERSION=$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")

fail() {
  printf 'Offline bundle contract failed: %s\n' "$1" >&2
  exit 1
}

[ -f "${WEB_DIR}/package.json" ] || fail 'web/package.json is missing'
[ -f "${WEB_DIR}/package-lock.json" ] || fail 'web/package-lock.json is missing'
[ -f "${SDK_DIR}/package.json" ] || fail 'sdk/gamehub-js/package.json is missing'
[ -f "${SDK_DIR}/package-lock.json" ] || fail 'sdk/gamehub-js/package-lock.json is missing'
[ -f "${DIST_DIR}/index.html" ] || fail 'web/dist/index.html is missing; run the production build first'

WEB_VERSION=$(node -p "require('${WEB_DIR}/package.json').version")
SDK_VERSION=$(node -p "require('${SDK_DIR}/package.json').version")
[ "${WEB_VERSION}" = "${VERSION}" ] || fail "web version ${WEB_VERSION} does not match VERSION ${VERSION}"
[ "${SDK_VERSION}" = "${VERSION}" ] || fail "gamehub-js version ${SDK_VERSION} does not match VERSION ${VERSION}"

PHASER_RANGE=$(node -p "require('${WEB_DIR}/package.json').dependencies?.phaser || ''")
[ -n "${PHASER_RANGE}" ] || fail 'Phaser must be a production dependency'
grep -Fq '"node_modules/phaser"' "${WEB_DIR}/package-lock.json" || fail 'Phaser is not locked in web/package-lock.json'
npm --prefix "${WEB_DIR}" ls phaser --omit=dev --depth=0 >/dev/null 2>&1 || fail 'the locked Phaser production dependency is not installed'

grep -R -E -q "from[[:space:]]+['\"]phaser['\"]|import\([[:space:]]*['\"]phaser['\"]" "${WEB_DIR}/src/games/realmguard" \
  || fail 'RealmGuard source does not import the bundled Phaser runtime'
grep -R -F -q "REALMGUARD_VERSION = '${VERSION}'" "${WEB_DIR}/src/games/realmguard" \
  || fail 'RealmGuard content version does not match VERSION'

if grep -E -i -q "<(script|link)[^>]+(src|href)=['\"]https?://" "${DIST_DIR}/index.html"; then
  fail 'production HTML references a remote script or stylesheet'
fi

find "${DIST_DIR}/assets" -type f -name '*.js' -print -quit | grep -q . \
  || fail 'the production JavaScript bundle is missing'
grep -R -i -q 'realmguard' "${DIST_DIR}/assets" \
  || fail 'RealmGuard route/content is absent from the production bundle'

printf 'Offline RealmGuard bundle verified (igame %s, Phaser %s).\n' "${VERSION}" "${PHASER_RANGE}"

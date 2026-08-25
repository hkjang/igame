#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DEFAULT_REPO_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
REPO_DIR=${1:-${DEFAULT_REPO_DIR}}
WEB_DIR=${REPO_DIR}/web
SDK_DIR=${REPO_DIR}/sdk/gamehub-js
DIST_DIR=${WEB_DIR}/dist
VERSION=$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")
# Game content and the service have separate lifecycles, as docs/release.md
# states: the image carries whatever content pack was last published and the
# service version must not overwrite it. Both packs are therefore pinned here,
# and a content release moves its own pin alongside a new seed migration.
REALMGUARD_CONTENT_VERSION=0.3.1
DEFENSE_CONTENT_VERSION=0.4.0

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
grep -R -F -q "REALMGUARD_VERSION = '${REALMGUARD_CONTENT_VERSION}'" "${WEB_DIR}/src/games/realmguard" \
  || fail "RealmGuard preserved content version is not ${REALMGUARD_CONTENT_VERSION}"
grep -R -E -q "DEFENSE_SERIES_VERSION[[:space:]]*=[[:space:]]*['\"]${DEFENSE_CONTENT_VERSION}['\"]" "${WEB_DIR}/src/games/defense" \
  || fail "Defense Series preserved content version is not ${DEFENSE_CONTENT_VERSION}"

for slug in office-guardians cyber-fortress ai-nexus-defense; do
  grep -R -F -q "${slug}" "${WEB_DIR}/src/games/defense" \
    || fail "Defense Series source is missing ${slug}"

  for suffix in '' '-banner'; do
    svg="${WEB_DIR}/public/assets/games/${slug}${suffix}.svg"
    built_svg="${DIST_DIR}/assets/games/${slug}${suffix}.svg"
    [ -f "${svg}" ] || fail "Defense Series offline SVG is missing: ${slug}${suffix}.svg"
    [ -f "${built_svg}" ] || fail "Defense Series SVG was not copied to the production bundle: ${slug}${suffix}.svg"
    if grep -E -i -q '(href|xlink:href)[[:space:]]*=[[:space:]]*"[[:space:]]*(https?:)?//|url\([[:space:]]*"?[[:space:]]*(https?:)?//' "${svg}" \
      || grep -E -i -q "(href|xlink:href)[[:space:]]*=[[:space:]]*'[[:space:]]*(https?:)?//" "${svg}"; then
      fail "Defense Series SVG references a remote asset: ${slug}${suffix}.svg"
    fi
  done
done

if grep -E -i -q "<(script|link)[^>]+(src|href)=['\"]https?://" "${DIST_DIR}/index.html"; then
  fail 'production HTML references a remote script or stylesheet'
fi

find "${DIST_DIR}/assets" -type f -name '*.js' -print -quit | grep -q . \
  || fail 'the production JavaScript bundle is missing'
grep -R -i -q 'realmguard' "${DIST_DIR}/assets" \
  || fail 'RealmGuard route/content is absent from the production bundle'
for slug in office-guardians cyber-fortress ai-nexus-defense; do
  grep -R -F -q "${slug}" "${DIST_DIR}/assets" \
    || fail "Defense Series route/content is absent from the production bundle: ${slug}"
done

# Education answer keys live only in PostgreSQL and the answer handler. Keep
# this exact seeded answer map out of the browser bundle; interface/property
# names alone are not treated as leaked content.
for answer_id in A B C safe unsafe correct wrong; do
  if grep -R -E -q "correct_answer_id['\"]?[[:space:]]*:[[:space:]]*['\"]${answer_id}['\"]" "${DIST_DIR}/assets"; then
    fail "Defense education answer material is embedded in the production browser bundle: ${answer_id}"
  fi
done

printf 'Offline RealmGuard %s and Defense Series %s bundles verified (igame %s, Phaser %s).\n' \
  "${REALMGUARD_CONTENT_VERSION}" "${DEFENSE_CONTENT_VERSION}" "${VERSION}" "${PHASER_RANGE}"

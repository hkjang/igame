#!/usr/bin/env bash
# Checks the boundary that decides who may do what, and that taking a
# permission away takes effect on a session that is already open.
#
# These properties were verified by hand and by nothing else: the route table
# is unit-tested against its permission map, but nothing exercised a live
# session losing its role, an account being disabled under it, or a key being
# revoked while it was in use.
set -Eeuo pipefail
trap 'printf "Authorisation smoke failed at line %d.\n" "${LINENO}" >&2' ERR

readonly BASE_URL="${1:-http://127.0.0.1:8080}"
readonly USERNAME="${2:-}"
readonly PASSWORD="${3:-}"

if [[ -z "${USERNAME}" || -z "${PASSWORD}" ]]; then
  printf 'Usage: %s [base-url] <bootstrap-user> <bootstrap-password>\n' "$0" >&2
  exit 2
fi
for command_name in curl jq; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'Required command not found: %s\n' "${command_name}" >&2
    exit 1
  }
done

admin_jar="$(mktemp)"
response_file="$(mktemp)"
cleanup() { rm -f -- "${admin_jar}" "${response_file}"; }
trap cleanup EXIT

csrf_of() { awk '/igame_csrf/ { print $NF }' "$1"; }

# call <jar> <method> <path> <expected> [body] [extra-header...]
call() {
  local jar="$1" method="$2" path="$3" expected="$4" body="${5:-}"
  shift 5 2>/dev/null || shift 4
  local -a args=(
    --silent --show-error --max-time 20 --request "${method}"
    --cookie "${jar}" --cookie-jar "${jar}"
    --header "Origin: ${BASE_URL}"
    --output "${response_file}" --write-out '%{http_code}'
  )
  local token
  token="$(csrf_of "${jar}")"
  [[ -n "${token}" ]] && args+=(--header "X-CSRF-Token: ${token}")
  [[ -n "${body}" ]] && args+=(--header 'Content-Type: application/json' --data-binary "${body}")
  local header
  for header in "$@"; do args+=(--header "${header}"); done
  local status
  status="$(curl "${args[@]}" "${BASE_URL}${path}")"
  if [[ "${status}" != "${expected}" ]]; then
    printf '%s %s returned HTTP %s, expected %s\n' "${method}" "${path}" "${status}" "${expected}" >&2
    cat "${response_file}" >&2
    printf '\n' >&2
    return 1
  fi
  cat "${response_file}"
}

login() {
  local jar="$1" user="$2" secret="$3"
  : > "${jar}"
  jq --null-input --compact-output --arg username "${user}" --arg password "${secret}" \
    '{username:$username,password:$password}' \
    | curl --silent --show-error --max-time 20 --request POST \
        --cookie-jar "${jar}" --header 'Content-Type: application/json' \
        --header "Origin: ${BASE_URL}" --data-binary @- \
        --output "${response_file}" --write-out '%{http_code}' \
        "${BASE_URL}/api/v1/auth/login" \
    | grep -qx 200
}

login "${admin_jar}" "${USERNAME}" "${PASSWORD}"
admin_id="$(call "${admin_jar}" GET /api/v1/me 200 | jq --raw-output '.user.id')"

# Accounts arrive from SSO or the bootstrap, and there is no endpoint that
# creates one, so the parts that need a second human account — a role taken away
# from an open session, an account disabled under it — cannot be reached from
# here. What can be reached is the same refusal path, entered through an API key
# whose scope does not cover what it asks for.
narrow="$(call "${admin_jar}" POST /api/v1/me/api-keys 201 '{"name":"authz-smoke-narrow","permissions":["games:read"]}')"
narrow_id="$(jq --raw-output '.api_key.id' <<<"${narrow}")"
narrow_secret="$(jq --raw-output '.secret // .api_key.secret' <<<"${narrow}")"
[[ -n "${narrow_secret}" && "${narrow_secret}" != "null" ]]

# Its own scope answers; anything wider is refused, and a key may never manage
# keys however wide its scope.
bearer() {
  curl --silent --show-error --max-time 20 --output /dev/null --write-out '%{http_code}' \
    --request "$1" --header "Authorization: Bearer ${narrow_secret}" "${BASE_URL}$2"
}
bearer GET /api/v1/games | grep -qx 200
bearer GET /api/v1/admin/users | grep -qx 403
bearer GET /api/v1/me/api-keys | grep -qx 403

# Every one of those refusals is written down.
audit="$(call "${admin_jar}" GET '/api/v1/admin/audit?limit=200' 200)"
jq --exit-status '[.items[] | select(.action == "access.denied")] | length >= 2' <<<"${audit}" >/dev/null
jq --exit-status 'first(.items[] | select(.action == "access.denied"))
   | (.detail.path | type == "string") and (.detail.method | type == "string")
     and (.detail.auth_type == "api_key")' <<<"${audit}" >/dev/null
call "${admin_jar}" DELETE "/api/v1/me/api-keys/${narrow_id}" 204 >/dev/null

# A revoked key stops working on the next request.
key="$(call "${admin_jar}" POST /api/v1/me/api-keys 201 '{"name":"authz-smoke-probe","permissions":["games:read"]}')"
key_id="$(jq --raw-output '.api_key.id' <<<"${key}")"
key_secret="$(jq --raw-output '.secret // .api_key.secret' <<<"${key}")"
[[ -n "${key_secret}" && "${key_secret}" != "null" ]]
curl --silent --show-error --max-time 20 --output /dev/null --write-out '%{http_code}' \
  --header "Authorization: Bearer ${key_secret}" "${BASE_URL}/api/v1/games" | grep -qx 200
call "${admin_jar}" DELETE "/api/v1/me/api-keys/${key_id}" 204 >/dev/null
curl --silent --show-error --max-time 20 --output /dev/null --write-out '%{http_code}' \
  --header "Authorization: Bearer ${key_secret}" "${BASE_URL}/api/v1/games" | grep -qx 401

# A write carrying a session but a foreign origin is refused.
curl --silent --show-error --max-time 20 --request PATCH \
  --cookie "${admin_jar}" --header 'Content-Type: application/json' \
  --header "X-CSRF-Token: $(csrf_of "${admin_jar}")" --header 'Origin: https://evil.example' \
  --data-binary '{"status":"active"}' --output /dev/null --write-out '%{http_code}' \
  "${BASE_URL}/api/v1/admin/users/${admin_id}" | grep -qx 403

printf 'Authorisation smoke passed: API key scope, refusals audited, key revocation, foreign-origin writes (%s)\n' "${BASE_URL}"

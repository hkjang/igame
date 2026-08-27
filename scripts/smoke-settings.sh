#!/usr/bin/env bash
# Checks that the admin settings page can save what the admin settings page was
# given.
#
# Saving a Keycloak connection failed for everyone: the list endpoint wrote a
# derived `client_secret_configured` flag into the OIDC value, the page edits
# and returns the object it was handed, and the OIDC write refuses fields the
# setting does not have. The API was returning a value it would not accept
# back, and nothing noticed, because every fixture that had ever exercised the
# write built its body by hand instead of by reading one.
#
# So the invariant this checks is a round trip, not a shape: whatever GET hands
# out for a setting, PUT takes.
set -Eeuo pipefail
trap 'printf "Settings smoke failed at line %d.\n" "${LINENO}" >&2' ERR

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

jar="$(mktemp)"
response_file="$(mktemp)"
original_oidc="$(mktemp)"
restore() {
  # Put the realm's own configuration back before leaving, however this ends:
  # the gates that run after this one sign in through the same server.
  if [[ -s "${original_oidc}" ]]; then
    curl --silent --show-error --max-time 20 --request PUT \
      --cookie "${jar}" --header 'Content-Type: application/json' \
      --header "X-CSRF-Token: $(awk '/igame_csrf/ { print $NF }' "${jar}")" \
      --header "Origin: ${BASE_URL}" --data-binary "@${original_oidc}" \
      --output /dev/null "${BASE_URL}/api/v1/admin/settings/oidc" || true
  fi
  rm -f -- "${jar}" "${response_file}" "${original_oidc}"
}
trap restore EXIT

csrf_of() { awk '/igame_csrf/ { print $NF }' "$1"; }

# call <method> <path> <expected> [body-file]
call() {
  local method="$1" path="$2" expected="$3" body_file="${4:-}"
  local -a args=(
    --silent --show-error --max-time 20 --request "${method}"
    --cookie "${jar}" --cookie-jar "${jar}"
    --header "Origin: ${BASE_URL}"
    --output "${response_file}" --write-out '%{http_code}'
  )
  local token
  token="$(csrf_of "${jar}")"
  [[ -n "${token}" ]] && args+=(--header "X-CSRF-Token: ${token}")
  [[ -n "${body_file}" ]] && args+=(--header 'Content-Type: application/json' --data-binary "@${body_file}")
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

: > "${jar}"
jq --null-input --compact-output --arg username "${USERNAME}" --arg password "${PASSWORD}" \
  '{username:$username,password:$password}' \
  | curl --silent --show-error --max-time 20 --request POST \
      --cookie-jar "${jar}" --header 'Content-Type: application/json' \
      --header "Origin: ${BASE_URL}" --data-binary @- \
      --output "${response_file}" --write-out '%{http_code}' \
      "${BASE_URL}/api/v1/auth/login" \
  | grep -qx 200

settings="$(call GET /api/v1/admin/settings 200)"
jq --compact-output '.settings.oidc' <<<"${settings}" > "${original_oidc}"

# No setting value carries a field that is about the setting rather than in it.
# `_configured` is the flag that caused this; a value is not the place for any
# of them.
jq --exit-status '[.settings | to_entries[] | .value | select(type == "object")
    | keys[] | select(endswith("_configured"))] | length == 0' <<<"${settings}" >/dev/null

# The fact it used to smuggle is still reported, beside the settings.
jq --exit-status '.secrets | type == "object"' <<<"${settings}" >/dev/null

# The round trip itself, for the two settings whose writes decode strictly.
# The page sends the value bare for these; everything else is wrapped.
for key in oidc ai; do
  body="$(mktemp)"
  jq --compact-output --arg key "${key}" '.settings[$key] // {}' <<<"${settings}" > "${body}"
  call PUT "/api/v1/admin/settings/${key}" 200 "${body}" >/dev/null
  rm -f -- "${body}"
done

# And the case an operator actually reports: fill in a Keycloak realm, save,
# reload, save again without retyping the secret, and find the secret still
# there. The second save is the one that used to fail.
keycloak="$(mktemp)"
jq --null-input --compact-output '{
  enabled: false,
  issuer: "https://keycloak.settings-smoke.invalid/realms/igame",
  client_id: "igame-settings-smoke",
  client_secret: "settings-smoke-secret",
  scopes: ["openid", "profile", "email"],
  username_claim: "preferred_username",
  display_name_claim: "name",
  email_claim: "email",
  groups_claim: "groups",
  admin_groups: ["igame-admins"]
}' > "${keycloak}"
call PUT /api/v1/admin/settings/oidc 200 "${keycloak}" >/dev/null
rm -f -- "${keycloak}"

reloaded="$(call GET /api/v1/admin/settings 200)"
jq --exit-status '.settings.oidc.issuer == "https://keycloak.settings-smoke.invalid/realms/igame"
  and .settings.oidc.client_id == "igame-settings-smoke"
  and (.settings.oidc.admin_groups == ["igame-admins"])' <<<"${reloaded}" >/dev/null
# Written, never read back out.
jq --exit-status '.settings.oidc.client_secret == "" and .secrets.oidc.client_secret == true' <<<"${reloaded}" >/dev/null

again="$(mktemp)"
jq --compact-output '.settings.oidc' <<<"${reloaded}" > "${again}"
call PUT /api/v1/admin/settings/oidc 200 "${again}" >/dev/null
rm -f -- "${again}"
jq --exit-status '.secrets.oidc.client_secret == true' <<<"$(call GET /api/v1/admin/settings 200)" >/dev/null

# The strictness that made the round trip matter is still on: a field the
# setting does not have is still refused, so a typo in a claim name cannot be
# saved and silently ignored.
typo="$(mktemp)"
jq --compact-output '.settings.oidc + {username_clam: "preferred_username"}' <<<"${reloaded}" > "${typo}"
call PUT /api/v1/admin/settings/oidc 400 "${typo}" >/dev/null
rm -f -- "${typo}"

printf 'Settings smoke passed: values round-trip, secrets stay write-only, unknown fields still refused (%s)\n' "${BASE_URL}"

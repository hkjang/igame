#!/usr/bin/env bash
set -Eeuo pipefail
trap 'printf "Defense Series smoke failed at line %d.\n" "${LINENO}" >&2' ERR

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly BASE_URL="${1:-http://127.0.0.1:8080}"
readonly USERNAME="${2:-}"
readonly PASSWORD="${3:-}"
readonly EXPECTED_VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
# Game content and the service have separate lifecycles, so the published
# Defense pack keeps its own version independently from the service image.
readonly EXPECTED_DEFENSE_CONTENT_VERSION="0.4.0"
readonly MANAGER_SAME_USERNAME="${IGAME_MANAGER_SAME_USERNAME:-}"
readonly MANAGER_SAME_PASSWORD="${IGAME_MANAGER_SAME_PASSWORD:-}"
readonly MANAGER_EMPTY_USERNAME="${IGAME_MANAGER_EMPTY_USERNAME:-}"
readonly MANAGER_EMPTY_PASSWORD="${IGAME_MANAGER_EMPTY_PASSWORD:-}"
readonly MANAGER_OTHER_USERNAME="${IGAME_MANAGER_OTHER_USERNAME:-}"
readonly MANAGER_OTHER_PASSWORD="${IGAME_MANAGER_OTHER_PASSWORD:-}"
readonly SMOKE_POSTGRES_DSN="${IGAME_SMOKE_POSTGRES_DSN:-}"

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

readonly ADMIN_COOKIE="$(mktemp)"
readonly RESPONSE_FILE="$(mktemp)"
readonly TEMP_DIR="$(mktemp -d)"
approval_changed=false
approval_restore_body=''
cleanup() {
  if [[ "${approval_changed}" == true && -n "${approval_restore_body}" ]]; then
    curl --silent --show-error --max-time 10 --request PUT \
      --cookie "${ADMIN_COOKIE}" --header 'Content-Type: application/json' \
      --data-binary "${approval_restore_body}" \
      "${BASE_URL}/api/v1/admin/settings/approval" >/dev/null || \
      printf '%s\n' 'Warning: failed to restore approval settings after Defense Series smoke.' >&2
  fi
  rm -f -- "${ADMIN_COOKIE}" "${RESPONSE_FILE}"
  rm -rf -- "${TEMP_DIR}"
}
trap cleanup EXIT

request_with_cookie() {
  local cookie_jar="$1"
  local method="$2"
  local path="$3"
  local expected_status="$4"
  local body="${5:-}"
  local extra_header="${6:-}"
  local status
  local -a args=(
    --silent --show-error --max-time 30
    --request "${method}"
    --cookie "${cookie_jar}"
    --cookie-jar "${cookie_jar}"
    --output "${RESPONSE_FILE}"
    --write-out '%{http_code}'
  )
  if [[ -n "${body}" ]]; then
    args+=(--header 'Content-Type: application/json' --data-binary "${body}")
  fi
  if [[ -n "${extra_header}" ]]; then
    args+=(--header "${extra_header}")
  fi
  status="$(curl "${args[@]}" "${BASE_URL}${path}")"
  if [[ "${status}" != "${expected_status}" ]]; then
    printf '%s %s returned HTTP %s, expected %s\n' "${method}" "${path}" "${status}" "${expected_status}" >&2
    jq . "${RESPONSE_FILE}" >&2 2>/dev/null || sed -n '1,100p' "${RESPONSE_FILE}" >&2
    exit 1
  fi
  cat "${RESPONSE_FILE}"
}

request() {
  request_with_cookie "${ADMIN_COOKIE}" "$@"
}

login() {
  local cookie_jar="$1"
  local username="$2"
  local password="$3"
  local expected_role="$4"
  local body
  body="$(jq --null-input --compact-output --arg username "${username}" --arg password "${password}" '{username:$username,password:$password}')"
  request_with_cookie "${cookie_jar}" POST /api/v1/auth/login 200 "${body}" >/dev/null
  request_with_cookie "${cookie_jar}" GET /api/v1/me 200 \
    | jq --exit-status --arg username "${username}" --arg role "${expected_role}" '.user.username == $username and .user.role == $role' >/dev/null
}

new_uuid() {
  local value=''
  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    IFS= read -r value < /proc/sys/kernel/random/uuid
  elif command -v uuidgen >/dev/null 2>&1; then
    value="$(uuidgen)"
  else
    printf '%s\n' 'A UUID source is required for Defense Series telemetry smoke.' >&2
    exit 1
  fi
  printf '%s' "${value,,}"
}

telemetry_payload() {
  local slug="$1"
  local session_id="$2"
  local session_token="$3"
  local event="$4"
  local sequence="$5"
  local client_event_id="$6"
  local data="$7"
  jq --null-input --compact-output \
    --arg slug "${slug}" --arg session_id "${session_id}" --arg session_token "${session_token}" \
    --arg event "${event}" --arg client_event_id "${client_event_id}" \
    --argjson sequence "${sequence}" --argjson data "${data}" \
    '{game_id:$slug,session_id:$session_id,session_token:$session_token,event:$event,data:$data,client_event_id:$client_event_id,sequence:$sequence}'
}

post_telemetry() {
  local slug="$1"
  local session_id="$2"
  local session_token="$3"
  local event="$4"
  local sequence="$5"
  local data="$6"
  local expected_status="${7:-202}"
  local client_event_id="${8:-$(new_uuid)}"
  request POST /api/v1/telemetry "${expected_status}" \
    "$(telemetry_payload "${slug}" "${session_id}" "${session_token}" "${event}" "${sequence}" "${client_event_id}" "${data}")"
}

start_session() {
  local slug="$1"
  local version_id="$2"
  local purpose="$3"
  start_session_with_cookie "${ADMIN_COOKIE}" "${slug}" "${version_id}" "${purpose}"
}

start_session_with_cookie() {
  local cookie_jar="$1"
  local slug="$2"
  local version_id="$3"
  local purpose="$4"
  local body
  body="$(jq --null-input --compact-output --arg version_id "${version_id}" --arg purpose "${purpose}" --arg version "${EXPECTED_VERSION}" \
    '{metadata:{client:"release-smoke",client_version:$version,purpose:$purpose,defense_content_version_id:$version_id}}')"
  request_with_cookie "${cookie_jar}" POST "/api/v1/games/${slug}/sessions" 201 "${body}"
}

assert_public_redaction() {
  local file="$1"
  jq --exit-status '
    def answer_material:
      .. | objects
      | select(has("correct_answer_id") or has("correct") or has("explanation"));
    ([.content.education, .content.events] | [answer_material] | length) == 0
    and ([.content.education[]
      | ([.answers[].id] | sort) == ["A","B","C"]]
      | all)
    and ([.content.education[].answers[].id]
      | all(. as $id | (["safe","unsafe","correct","wrong"] | index($id | ascii_downcase)) == null))
  ' "${file}" >/dev/null
}

check_svg_asset() {
  local path="$1"
  local payload
  payload="$(curl --fail --silent --show-error --max-time 15 "${BASE_URL}${path}")"
  grep -Eiq '<svg([[:space:]>])' <<<"${payload}" || {
    printf 'Offline asset is not SVG: %s\n' "${path}" >&2
    exit 1
  }
  if grep -Eiq '(href|xlink:href)[[:space:]]*=[[:space:]]*"[[:space:]]*(https?:)?//|url\([[:space:]]*"?[[:space:]]*(https?:)?//' <<<"${payload}" \
    || grep -Eiq "(href|xlink:href)[[:space:]]*=[[:space:]]*'[[:space:]]*(https?:)?//" <<<"${payload}"; then
    printf 'SVG contains a remote href/url: %s\n' "${path}" >&2
    exit 1
  fi
}

version_json="$(request GET /api/v1/version 200)"
jq --exit-status --arg version "${EXPECTED_VERSION}" '.version == $version and (.commit | length) > 0 and (.build_date | length) > 0' <<<"${version_json}" >/dev/null
login "${ADMIN_COOKIE}" "${USERNAME}" "${PASSWORD}" admin

manager_fixture=false
if [[ -n "${MANAGER_SAME_USERNAME}${MANAGER_SAME_PASSWORD}${MANAGER_EMPTY_USERNAME}${MANAGER_EMPTY_PASSWORD}${MANAGER_OTHER_USERNAME}${MANAGER_OTHER_PASSWORD}" ]]; then
  if [[ -z "${MANAGER_SAME_USERNAME}" || -z "${MANAGER_SAME_PASSWORD}" || -z "${MANAGER_EMPTY_USERNAME}" || -z "${MANAGER_EMPTY_PASSWORD}" || -z "${MANAGER_OTHER_USERNAME}" || -z "${MANAGER_OTHER_PASSWORD}" ]]; then
    printf '%s\n' 'All three manager smoke credentials must be supplied together.' >&2
    exit 1
  fi
  manager_fixture=true
fi
same_cookie="${TEMP_DIR}/manager-same.cookie"
empty_cookie="${TEMP_DIR}/manager-empty.cookie"
other_cookie="${TEMP_DIR}/manager-other.cookie"
if [[ "${manager_fixture}" == true ]]; then
  login "${same_cookie}" "${MANAGER_SAME_USERNAME}" "${MANAGER_SAME_PASSWORD}" manager
  login "${empty_cookie}" "${MANAGER_EMPTY_USERNAME}" "${MANAGER_EMPTY_PASSWORD}" manager
  login "${other_cookie}" "${MANAGER_OTHER_USERNAME}" "${MANAGER_OTHER_PASSWORD}" manager
fi

mcp_tools="$(request POST /mcp 200 \
  '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  'MCP-Protocol-Version: 2025-11-25')"
jq --exit-status '
  [.result.tools[] | select(.name == "game_session_start")][0].inputSchema as $session
  | [.result.tools[] | select(.name == "defense_config_get")][0].inputSchema as $config
  | [.result.tools[] | select(.name == "defense_rankings_get")][0].inputSchema as $rankings
  | ($session.required == ["game_id"])
    and ($session.additionalProperties == false)
    and ($session.properties.defense_content_version_id.format == "uuid")
    and ($session.properties.realmguard_version_id.format == "uuid")
    and ($config.required == ["slug"]) and ($config.additionalProperties == false)
    and (($config.properties.slug.enum | sort) == (["office-guardians","cyber-fortress","ai-nexus-defense"] | sort))
    and ($rankings.required == ["slug"]) and ($rankings.additionalProperties == false)
    and (($rankings.properties.period.enum | sort) == (["daily","weekly","monthly","season","all_time"] | sort))
    and (($rankings.properties.group.enum | sort) == (["individual","department","team"] | sort))
    and ($rankings.properties.limit.maximum == 200)
' <<<"${mcp_tools}" >/dev/null

declare -a DEFENSE_SLUGS=(office-guardians cyber-fortress ai-nexus-defense)
declare -A DEFENSE_NAMES=(
  [office-guardians]='Office Guardians'
  [cyber-fortress]='Cyber Fortress'
  [ai-nexus-defense]='AI Nexus Defense'
)
declare -A EXPECTED_STAGES=([office-guardians]=8 [cyber-fortress]=10 [ai-nexus-defense]=10)
declare -A EXPECTED_TOWERS=([office-guardians]=6 [cyber-fortress]=8 [ai-nexus-defense]=10)
declare -A EXPECTED_ENEMIES=([office-guardians]=10 [cyber-fortress]=15 [ai-nexus-defense]=15)
declare -A EXPECTED_BOSSES=([office-guardians]=2 [cyber-fortress]=3 [ai-nexus-defense]=4)
declare -A EXPECTED_HEROES=([office-guardians]=3 [cyber-fortress]=3 [ai-nexus-defense]=5)
declare -A EXPECTED_EVENTS=([office-guardians]=0 [cyber-fortress]=30 [ai-nexus-defense]=30)
declare -A EXPECTED_EDUCATION=([office-guardians]=0 [cyber-fortress]=50 [ai-nexus-defense]=50)
declare -A SMOKE_DIFFICULTIES=([office-guardians]=casual [cyber-fortress]=normal [ai-nexus-defense]=veteran)
declare -A VERSION_IDS=()
declare -A VERSION_CHECKSUMS=()
declare -A ACTIVE_POLICY_VERSIONS=()
LAST_VERIFIED_RESULT_ID=''
LAST_VERIFIED_RESULT_SCORE=''

for slug in "${DEFENSE_SLUGS[@]}"; do
  game_json="$(request GET "/api/v1/games/${slug}" 200)"
  jq --exit-status --arg slug "${slug}" --arg name "${DEFENSE_NAMES[${slug}]}" --arg version "${EXPECTED_DEFENSE_CONTENT_VERSION}" \
    '.game.slug == $slug and .game.name == $name and .game.game_url == ("/games/"+$slug) and .game.version == $version and .game.status == "active"' \
    <<<"${game_json}" >/dev/null
  curl --fail --silent --show-error --max-time 15 "${BASE_URL}/games/${slug}" | grep -Eiq '<!doctype html|<html'
  check_svg_asset "/assets/games/${slug}.svg"
  check_svg_asset "/assets/games/${slug}-banner.svg"

  config_file="${TEMP_DIR}/${slug}.json"
  request GET "/api/v1/defense/${slug}/config" 200 >"${config_file}"
  jq --exit-status \
    --arg slug "${slug}" --arg version "${EXPECTED_DEFENSE_CONTENT_VERSION}" \
    --argjson stages "${EXPECTED_STAGES[${slug}]}" --argjson towers "${EXPECTED_TOWERS[${slug}]}" \
    --argjson enemies "${EXPECTED_ENEMIES[${slug}]}" --argjson bosses "${EXPECTED_BOSSES[${slug}]}" \
    --argjson heroes "${EXPECTED_HEROES[${slug}]}" --argjson events "${EXPECTED_EVENTS[${slug}]}" \
    --argjson education "${EXPECTED_EDUCATION[${slug}]}" '
      .game.slug == $slug and .version.content_version == $version
      and .version.asset_version == "procedural-defense-2"
      and (.version.id | test("^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"))
      and (.version.checksum | test("^[0-9a-f]{64}$"))
      and (.content.stages | length) == $stages
      and (.content.waves | length) == ($stages * 8)
      and (.content.towers | length) == $towers
      and (.content.enemies | length) == $enemies
      and (.content.bosses | length) == $bosses
      and (.content.heroes | length) == $heroes
      and (.content.events | length) == $events
      and (.content.education | length) == $education
      and ([.content.stages[] | select((.path | length) >= 2 and ((.paths // [.path]) | length) >= 1 and (.tower_spots | length) >= 8 and (.map_style | length) > 0)] | length) == $stages
      and ([.content.stages[] | ((.paths // [.path]) | tostring)] | unique | length) == $stages
      and ([.content.stages[] | ((.paths // [.path]) | length)] | any(. > 1))
      and ([.content.stages[] as $stage
        | .content.waves[] | select(.stage_id == $stage.id) | .entries[]
        | ((.path_index // 0) >= 0 and (.path_index // 0) < (($stage.paths // [$stage.path]) | length))] | all)
      and ([.content.stages[].id,.content.waves[].id,.content.towers[].id,.content.enemies[].id,.content.bosses[].id,.content.heroes[].id]
        | all(test("^[a-z][a-z0-9_-]{0,31}$")))
    ' "${config_file}" >/dev/null
  assert_public_redaction "${config_file}"
  VERSION_IDS[${slug}]="$(jq --raw-output '.version.id' "${config_file}")"
  VERSION_CHECKSUMS[${slug}]="$(jq --raw-output '.version.checksum' "${config_file}")"
  ACTIVE_POLICY_VERSIONS[${slug}]="$(jq --raw-output '.version.policy_version' "${config_file}")"

  mcp_config_call="$(jq --null-input --compact-output --arg slug "${slug}" \
    '{jsonrpc:"2.0",id:10,method:"tools/call",params:{name:"defense_config_get",arguments:{slug:$slug}}}')"
  mcp_config_file="${TEMP_DIR}/${slug}-mcp-config.json"
  request POST /mcp 200 "${mcp_config_call}" 'MCP-Protocol-Version: 2025-11-25' \
    | jq --exit-status --arg slug "${slug}" --arg id "${VERSION_IDS[${slug}]}" '
        select(.result.isError == false and .result.structuredContent.game.slug == $slug
          and .result.structuredContent.version.id == $id)
        | .result.structuredContent
      ' >"${mcp_config_file}"
  assert_public_redaction "${mcp_config_file}"

  request GET "/api/v1/defense/${slug}/version" 200 \
    | jq --exit-status --arg id "${VERSION_IDS[${slug}]}" --arg version "${EXPECTED_DEFENSE_CONTENT_VERSION}" \
      '.version.id == $id and .version.content_version == $version and (.version.checksum | test("^[0-9a-f]{64}$"))' >/dev/null
done

# Defense telemetry uses the same exact 4 KiB event-data boundary as the
# authoritative RealmGuard ledger. Prove the boundary with a real pinned
# session rather than relying only on source inspection.
payload_limit_session="$(start_session office-guardians "${VERSION_IDS[office-guardians]}" telemetry-payload-limit)"
payload_limit_session_id="$(jq --raw-output '.session.id' <<<"${payload_limit_session}")"
payload_limit_session_token="$(jq --raw-output '.session.session_token' <<<"${payload_limit_session}")"
printf -v payload_at_limit_value '%*s' 4088 ''
payload_at_limit_value="${payload_at_limit_value// /x}"
payload_at_limit_data="$(jq --null-input --compact-output --arg value "${payload_at_limit_value}" '{x:$value}')"
[[ "${#payload_at_limit_data}" == 4096 ]] || {
  printf 'Internal smoke error: expected a 4096-byte telemetry document, got %d.\n' "${#payload_at_limit_data}" >&2
  exit 1
}
post_telemetry office-guardians "${payload_limit_session_id}" "${payload_limit_session_token}" game.pause 1 "${payload_at_limit_data}" \
  | jq --exit-status '.accepted == true and .sequence == 1' >/dev/null
printf -v payload_over_limit_value '%*s' 4089 ''
payload_over_limit_value="${payload_over_limit_value// /x}"
payload_over_limit_data="$(jq --null-input --compact-output --arg value "${payload_over_limit_value}" '{x:$value}')"
[[ "${#payload_over_limit_data}" == 4097 ]] || {
  printf 'Internal smoke error: expected a 4097-byte telemetry document, got %d.\n' "${#payload_over_limit_data}" >&2
  exit 1
}
post_telemetry office-guardians "${payload_limit_session_id}" "${payload_limit_session_token}" game.pause 2 "${payload_over_limit_data}" 400 \
  | jq --exit-status '.error.code == "invalid_telemetry" and (.error.message | contains("4 KiB"))' >/dev/null

jq --exit-status '
  (.content.model_profiles | length) == 5
  and ([.content.model_profiles[].id] | sort) == (["large","medium","reasoning","small","vision"] | sort)
  and ([.content.model_profiles[].tower_id as $tower | [.content.towers[].id] | index($tower) != null] | all)
  and ([.content.model_profiles[] | ((.name | type) == "string" and (.name | length) > 0 and .compute_cost > 0 and .token_cost > 0 and .latency_cost >= 0 and .accuracy > 0 and .damage_multiplier > 0)] | all)
  and (.content.resource_rules.compute_start == .content.balance.resource_state_limits.compute)
  and (.content.resource_rules.token_start == .content.balance.resource_state_limits.token)
  and (.content.resource_rules.trust_start == .content.balance.resource_state_limits.trust)
  and (.content.resource_rules.latency_max == .content.balance.resource_state_limits.latency)
  and (.content.resource_rules.wave_compute_cost > 0)
  and (.content.resource_rules.wave_token_cost > 0)
  and (.content.resource_rules.escaped_trust_cost > 0)
  and (.content.resource_rules.escaped_latency_cost > 0)
  and ((.content.balance.ai_resource_score_factors | keys | sort) == (["compute","token","trust","latency"] | sort))
' "${TEMP_DIR}/ai-nexus-defense.json" >/dev/null

# Generic game CRUD/workflow must not create or replace any authoritative
# Defense Series identity, regardless of whether approval is enabled.
for slug in "${DEFENSE_SLUGS[@]}"; do
  reserved_workflow="$(jq --null-input --compact-output --arg slug "${slug}" \
    '{action:"create",resource_type:"game",payload:{slug:$slug,name:"forged authoritative game",game_url:("/games/"+$slug),game_type:"embedded",status:"active"}}')"
  request POST /api/v1/workflow/requests 409 "${reserved_workflow}" \
    | jq --exit-status '.error.code == "protected_game_identity" and (.error.message | contains("authoritative"))' >/dev/null
done

# The answer key is inspected only through the fresh PostgreSQL fixture. Public
# HTTP responses above must remain redacted and use neutral A/B/C answer IDs.
if [[ -n "${SMOKE_POSTGRES_DSN}" ]]; then
  command -v docker >/dev/null 2>&1 || {
    printf '%s\n' 'docker is required for the DB-only answer distribution gate.' >&2
    exit 1
  }
  answer_distribution="$(docker run --rm --network host postgres:17-alpine \
    psql "${SMOKE_POSTGRES_DSN}" --no-psqlrc --tuples-only --no-align --command "
      SELECT g.slug||'|'||count(*)||'|'||count(DISTINCT q.item->>'correct_answer_id')||'|'||count(DISTINCT a.position)
      FROM defense_content_versions v
      JOIN games g ON g.id=v.game_id
      CROSS JOIN LATERAL jsonb_array_elements(v.content->'education') q(item)
      CROSS JOIN LATERAL jsonb_array_elements(q.item->'answers') WITH ORDINALITY a(item,position)
      WHERE v.status='published'
        AND g.slug IN ('cyber-fortress','ai-nexus-defense')
        AND a.item->>'id'=q.item->>'correct_answer_id'
        AND q.item->>'correct_answer_id' IN ('A','B','C')
      GROUP BY g.slug
      ORDER BY g.slug;")"
  [[ "${answer_distribution}" == *'cyber-fortress|50|'* && "${answer_distribution}" == *'ai-nexus-defense|50|'* ]] || {
    printf 'DB-only answer distribution is incomplete:\n%s\n' "${answer_distribution}" >&2
    exit 1
  }
  while IFS='|' read -r answer_slug answer_count distinct_ids distinct_positions; do
    [[ -z "${answer_slug}" ]] && continue
    [[ "${answer_count}" == 50 && "${distinct_ids}" -ge 2 && "${distinct_positions}" -ge 2 ]] || {
      printf 'Answer keys must be neutral and distributed for %s: %s|%s|%s\n' "${answer_slug}" "${answer_count}" "${distinct_ids}" "${distinct_positions}" >&2
      exit 1
    }
  done <<<"${answer_distribution}"
fi

# Every new official session pins the exact UUID read from that slug's public
# config. Missing, stale, and cross-game pins are rejected.
for index in "${!DEFENSE_SLUGS[@]}"; do
  slug="${DEFENSE_SLUGS[${index}]}"
  mcp_missing="$(jq --null-input --compact-output --arg slug "${slug}" \
    '{jsonrpc:"2.0",id:2,method:"tools/call",params:{name:"game_session_start",arguments:{game_id:$slug}}}')"
  request POST /mcp 200 "${mcp_missing}" 'MCP-Protocol-Version: 2025-11-25' \
    | jq --exit-status '.result.isError == true and (.result.content[0].text | contains("defense_version_required"))' >/dev/null
  mcp_both_pins="$(jq --null-input --compact-output --arg slug "${slug}" --arg id "${VERSION_IDS[${slug}]}" \
    '{jsonrpc:"2.0",id:3,method:"tools/call",params:{name:"game_session_start",arguments:{game_id:$slug,realmguard_version_id:$id,defense_content_version_id:$id}}}')"
  request POST /mcp 200 "${mcp_both_pins}" 'MCP-Protocol-Version: 2025-11-25' \
    | jq --exit-status '.result.isError == true and (.result.content[0].text | contains("mutually exclusive"))' >/dev/null
  mcp_exact_pin="$(jq --null-input --compact-output --arg slug "${slug}" --arg id "${VERSION_IDS[${slug}]}" \
    '{jsonrpc:"2.0",id:4,method:"tools/call",params:{name:"game_session_start",arguments:{game_id:$slug,defense_content_version_id:$id}}}')"
  request POST /mcp 200 "${mcp_exact_pin}" 'MCP-Protocol-Version: 2025-11-25' \
    | jq --exit-status --arg id "${VERSION_IDS[${slug}]}" '.result.isError == false and .result.structuredContent.session.defense_content_version_id == $id' >/dev/null
  request POST "/api/v1/games/${slug}/sessions" 428 \
    '{"metadata":{"client":"release-smoke","purpose":"missing-defense-pin"}}' \
    | jq --exit-status '.error.code == "defense_version_required"' >/dev/null
  stale_id="$(new_uuid)"
  stale_body="$(jq --null-input --compact-output --arg id "${stale_id}" '{metadata:{client:"release-smoke",defense_content_version_id:$id}}')"
  request POST "/api/v1/games/${slug}/sessions" 409 "${stale_body}" \
    | jq --exit-status '.error.code == "defense_config_stale"' >/dev/null
  other_slug="${DEFENSE_SLUGS[$(((index + 1) % ${#DEFENSE_SLUGS[@]}))]}"
  cross_body="$(jq --null-input --compact-output --arg id "${VERSION_IDS[${other_slug}]}" '{metadata:{client:"release-smoke",defense_content_version_id:$id}}')"
  request POST "/api/v1/games/${slug}/sessions" 409 "${cross_body}" \
    | jq --exit-status '.error.code == "defense_config_stale"' >/dev/null

  generic_session="$(start_session "${slug}" "${VERSION_IDS[${slug}]}" generic-bypass)"
  jq --exit-status --arg id "${VERSION_IDS[${slug}]}" '.session.defense_content_version_id == $id' <<<"${generic_session}" >/dev/null
  generic_session_id="$(jq --raw-output '.session.id' <<<"${generic_session}")"
  generic_session_token="$(jq --raw-output '.session.session_token' <<<"${generic_session}")"
  generic_score_body="$(jq --null-input --compact-output --arg slug "${slug}" --arg session_id "${generic_session_id}" --arg token "${generic_session_token}" \
    '{game_id:$slug,session_id:$session_id,session_token:$token,score:999999999,metadata:{forged:true}}')"
  request POST /api/v1/scores 409 "${generic_score_body}" \
    | jq --exit-status '.error.code == "defense_authoritative_result_required"' >/dev/null
  request GET "/api/v1/rankings/${slug}?period=weekly" 409 \
    | jq --exit-status '.error.code == "defense_ranking_required"' >/dev/null
  generic_mcp_ranking="$(jq --null-input --compact-output --arg slug "${slug}" \
    '{jsonrpc:"2.0",id:5,method:"tools/call",params:{name:"leaderboard_get",arguments:{game_id:$slug,period:"season"}}}')"
  request POST /mcp 200 "${generic_mcp_ranking}" 'MCP-Protocol-Version: 2025-11-25' \
    | jq --exit-status '.result.isError == true and (.result.content[0].text | contains("defense_ranking_required"))' >/dev/null
done

# Optional telemetry cannot consume the class-specific battle-ready reserve.
limit_slug=office-guardians
limit_session="$(start_session "${limit_slug}" "${VERSION_IDS[${limit_slug}]}" telemetry-limit)"
limit_session_id="$(jq --raw-output '.session.id' <<<"${limit_session}")"
limit_session_token="$(jq --raw-output '.session.session_token' <<<"${limit_session}")"
for ((sequence = 1; sequence <= 128; sequence++)); do
  post_telemetry "${limit_slug}" "${limit_session_id}" "${limit_session_token}" game.pause "${sequence}" '{}' >/dev/null
done
post_telemetry "${limit_slug}" "${limit_session_id}" "${limit_session_token}" game.pause 129 '{}' 429 \
  | jq --exit-status '.error.code == "telemetry_limit"' >/dev/null
limit_ready="$(jq --null-input --compact-output \
  --arg content "$(jq --raw-output '.version.content_version' "${TEMP_DIR}/${limit_slug}.json")" \
  --arg policy "$(jq --raw-output '.version.policy_version' "${TEMP_DIR}/${limit_slug}.json")" \
  --arg stage "$(jq --raw-output '[.content.stages[] | select(.number == 1)][0].id' "${TEMP_DIR}/${limit_slug}.json")" \
  --arg hero "$(jq --raw-output '.content.heroes[0].id' "${TEMP_DIR}/${limit_slug}.json")" \
  '{stage_id:$stage,difficulty:"normal",hero_id:$hero,content_version:$content,policy_version:$policy}')"
post_telemetry "${limit_slug}" "${limit_session_id}" "${limit_session_token}" defense.battle.ready 129 "${limit_ready}" \
  | jq --exit-status '.accepted == true and .duplicate == false and .sequence == 129' >/dev/null
post_telemetry "${limit_slug}" "${limit_session_id}" "${limit_session_token}" defense.battle.ready 130 "${limit_ready}" 429 \
  | jq --exit-status '.error.code == "telemetry_limit"' >/dev/null

assert_unattested_result_rejected() {
  local cookie_jar="$1"
  local slug="$2"
  local session_id="$3"
  local session_token="$4"
  local starting_resource="$5"
  local education_earned="$6"
  local education_spent="$7"
  local initial_resource_state="$8"
  local result_body="$9"
  local zero_wave_body body_only_body
  zero_wave_body="$(jq --compact-output --arg session_id "${session_id}" --arg token "${session_token}" --argjson initial "${starting_resource}" --argjson initial_resource_state "${initial_resource_state}" '
    .session_id=$session_id | .session_token=$token | .duration_ms=0 | .remaining_health=0 | .remaining_resource=$initial
    | .kills=0 | .escaped=0 | .spawned=0 | .waves_completed=0 | .victory=false
    | .battle.earned_resource=0 | .battle.spent_resource=0 | .battle.recovered_resource=0
    | .answers=[] | .resource_state=$initial_resource_state
    | .defeated_by_enemy={} | .escaped_by_enemy={} | .spawned_by_enemy={}
  ' <<<"${result_body}")"
  request_with_cookie "${cookie_jar}" POST "/api/v1/defense/${slug}/results" 422 "${zero_wave_body}" \
    | jq --exit-status '.error.code == "telemetry_attestation_failed" or .error.code == "invalid_duration"' >/dev/null

  body_only_body="$(jq --compact-output \
    --arg session_id "${session_id}" --arg token "${session_token}" \
    --argjson education_earned "${education_earned}" --argjson education_spent "${education_spent}" \
    --argjson initial_resource_state "${initial_resource_state}" '
      .session_id=$session_id | .session_token=$token | .answers=[]
      | .battle.earned_resource -= $education_earned
      | .battle.spent_resource -= $education_spent
      | .remaining_resource = (.remaining_resource - $education_earned + $education_spent)
      | .resource_state=$initial_resource_state
    ' <<<"${result_body}")"
  request_with_cookie "${cookie_jar}" POST "/api/v1/defense/${slug}/results" 422 "${body_only_body}" \
    | jq --exit-status '.error.code == "telemetry_attestation_failed"' >/dev/null
}

run_verified_battle() {
  local slug="$1"
  local difficulty="${2:-normal}"
  local config_file="${TEMP_DIR}/${slug}.json"
  local version_id="${VERSION_IDS[${slug}]}"
  local stage_id hero_id content_version policy_version
  stage_id="$(jq --raw-output '[.content.stages[] | select(.number == 1)][0].id' "${config_file}")"
  hero_id="$(jq --raw-output '.content.heroes[0].id' "${config_file}")"
  content_version="$(jq --raw-output '.version.content_version' "${config_file}")"
  policy_version="$(jq --raw-output '.version.policy_version' "${config_file}")"
  local total_waves min_wave_duration_ms minimum_duration_ms minimum_milestone_ms milestone_sleep_seconds starting_health starting_resource
  total_waves="$(jq --arg stage "${stage_id}" '[.content.waves[] | select(.stage_id == $stage)] | length' "${config_file}")"
  min_wave_duration_ms="$(jq '.content.balance.min_wave_duration_ms | ceil' "${config_file}")"
  minimum_duration_ms=$((total_waves * min_wave_duration_ms))
  minimum_milestone_ms=$((min_wave_duration_ms / 5))
  ((minimum_milestone_ms < 250)) && minimum_milestone_ms=250
  ((minimum_milestone_ms > 1000)) && minimum_milestone_ms=1000
  # The server compares wall-clock receipt timestamps. Keep three whole seconds
  # of margin so scheduler, database, and host clock resynchronization jitter
  # cannot make the security assertion flaky.
  milestone_sleep_seconds=$(((minimum_milestone_ms + 999) / 1000 + 3))
  starting_health="$(jq --arg stage "${stage_id}" '[.content.stages[] | select(.id == $stage)][0].starting_health' "${config_file}")"
  starting_resource="$(jq --arg stage "${stage_id}" --arg difficulty "${difficulty}" '
    (([.content.stages[] | select(.id == $stage)][0].starting_resource) * .content.balance.difficulties[$difficulty].gold) | round
  ' "${config_file}")"
  local resource_state='{}'
  if [[ "${slug}" == ai-nexus-defense ]]; then
    resource_state="$(jq --compact-output '.content.balance.resource_state_limits | to_entries | map({key:.key,value:{start:.value,spent:0,remaining:.value}}) | from_entries' "${config_file}")"
  fi
  local initial_resource_state="${resource_state}"

  # This concurrent session has plausible perfect counters but never sends a
  # ledger. It is submitted only after the valid battle has supplied enough
  # real wall time, so rejection must be the missing attestation, not duration.
  local no_ledger_session='' no_ledger_id='' no_ledger_token='' no_ledger_cookie="${ADMIN_COOKIE}"
  if [[ "${manager_fixture}" == true ]]; then
    no_ledger_cookie="${same_cookie}"
    no_ledger_session="$(start_session_with_cookie "${no_ledger_cookie}" "${slug}" "${version_id}" body-only-forgery)"
    no_ledger_id="$(jq --raw-output '.session.id' <<<"${no_ledger_session}")"
    no_ledger_token="$(jq --raw-output '.session.session_token' <<<"${no_ledger_session}")"
  fi

  local session session_id session_token
  session="$(start_session "${slug}" "${version_id}" "verified-ledger-${difficulty}")"
  jq --exit-status --arg id "${version_id}" '.session.defense_content_version_id == $id' <<<"${session}" >/dev/null
  session_id="$(jq --raw-output '.session.id' <<<"${session}")"
  session_token="$(jq --raw-output '.session.session_token' <<<"${session}")"
  local ready_data ready_uuid ready_payload
  ready_data="$(jq --null-input --compact-output --arg stage "${stage_id}" --arg hero "${hero_id}" --arg content "${content_version}" --arg policy "${policy_version}" --argjson resource_state "${resource_state}" \
    --arg difficulty "${difficulty}" \
    '{stage_id:$stage,difficulty:$difficulty,hero_id:$hero,content_version:$content,policy_version:$policy,resource_state:$resource_state}')"
  ready_uuid="$(new_uuid)"
  ready_payload="$(telemetry_payload "${slug}" "${session_id}" "${session_token}" defense.battle.ready 1 "${ready_uuid}" "${ready_data}")"
  request POST /api/v1/telemetry 202 "${ready_payload}" \
    | jq --exit-status '.accepted == true and .duplicate == false and .sequence == 1' >/dev/null
  request POST /api/v1/telemetry 202 "${ready_payload}" \
    | jq --exit-status '.accepted == true and .duplicate == true and .sequence == 1' >/dev/null
  conflicting_ready="$(jq --compact-output '.data.hero_id="forged-hero"' <<<"${ready_payload}")"
  request POST /api/v1/telemetry 409 "${conflicting_ready}" \
    | jq --exit-status '.error.code == "telemetry_event_conflict"' >/dev/null
  post_telemetry "${slug}" "${session_id}" "${session_token}" game.pause 3 '{}' 409 \
    | jq --exit-status '.error.code == "telemetry_sequence_conflict"' >/dev/null

  local sequence=2 battle_started_epoch
  battle_started_epoch="$(date +%s)"
  local answers='[]'
  local education_earned=0 education_spent=0 education_apply_count=0
  local tower_spent=0 model_build_count=0
  local cumulative_hist='{}' cumulative_kills=0 earned_resource=0 remaining_resource="${starting_resource}"
  if [[ "${slug}" == ai-nexus-defense ]]; then
    profile_id="$(jq --raw-output '.content.model_profiles[0].id' "${config_file}")"
    profile_tower="$(jq --raw-output '.content.model_profiles[0].tower_id' "${config_file}")"
    profile_spot="$(jq --raw-output --arg stage "${stage_id}" '[.content.stages[] | select(.id == $stage)][0].tower_spots[0].id' "${config_file}")"
    profile_compute="$(jq '.content.model_profiles[0].compute_cost' "${config_file}")"
    profile_token="$(jq '.content.model_profiles[0].token_cost' "${config_file}")"
    profile_latency="$(jq '.content.model_profiles[0].latency_cost' "${config_file}")"
    tower_spent="$(jq --arg tower "${profile_tower}" '[.content.towers[] | select(.id == $tower)][0].cost' "${config_file}")"
    resource_state="$(jq --compact-output --argjson compute "${profile_compute}" --argjson token "${profile_token}" --argjson latency "${profile_latency}" '
      .compute.remaining = ([0, (.compute.remaining - $compute)] | max)
      | .compute.spent = (.compute.start - .compute.remaining)
      | .token.remaining = ([0, (.token.remaining - $token)] | max)
      | .token.spent = (.token.start - .token.remaining)
      | .latency.remaining = ([0, (.latency.remaining - $latency)] | max)
      | .latency.spent = (.latency.start - .latency.remaining)
    ' <<<"${resource_state}")"
    model_build="$(jq --null-input --compact-output --arg tower "${profile_tower}" --arg spot "${profile_spot}" --arg profile "${profile_id}" --argjson state "${resource_state}" \
      '{tower:$tower,spot:$spot,profile_id:$profile,resource_state:$state}')"
    post_telemetry "${slug}" "${session_id}" "${session_token}" defense.tower.build "${sequence}" "${model_build}" >/dev/null
    sequence=$((sequence + 1))
    model_build_count=1
  fi
  for ((wave = 1; wave <= total_waves; wave++)); do
    if [[ "${slug}" == ai-nexus-defense ]]; then
      wave_compute_cost="$(jq '.content.resource_rules.wave_compute_cost' "${config_file}")"
      wave_token_cost="$(jq '.content.resource_rules.wave_token_cost' "${config_file}")"
      resource_state="$(jq --compact-output --argjson compute "${wave_compute_cost}" --argjson token "${wave_token_cost}" '
        .compute.remaining = ([0, (.compute.remaining - $compute)] | max)
        | .compute.spent = (.compute.start - .compute.remaining)
        | .token.remaining = ([0, (.token.remaining - $token)] | max)
        | .token.spent = (.token.start - .token.remaining)
      ' <<<"${resource_state}")"
    fi
    wave_start="$(jq --null-input --compact-output --arg stage "${stage_id}" --argjson wave "${wave}" --argjson resource_state "${resource_state}" '{stage_id:$stage,wave:$wave,resource_state:$resource_state}')"
    post_telemetry "${slug}" "${session_id}" "${session_token}" defense.wave.start "${sequence}" "${wave_start}" >/dev/null
    sequence=$((sequence + 1))

    while IFS= read -r answer_event_id; do
      [[ -n "${answer_event_id}" ]] || continue
      question_id="$(jq --raw-output --arg event "${answer_event_id}" '[.content.events[] | select(.id == $event)][0].education_id' "${config_file}")"
      answer_id="$(jq --raw-output --arg question "${question_id}" '[.content.education[] | select(.id == $question)][0].answers[0].id' "${config_file}")"
      [[ -n "${answer_event_id}" && -n "${answer_id}" ]] || {
        printf 'Published education event could not be resolved for %s\n' "${slug}" >&2
        exit 1
      }
      answer_body="$(jq --null-input --compact-output --arg session_id "${session_id}" --arg token "${session_token}" --arg answer "${answer_id}" \
        '{session_id:$session_id,session_token:$token,answer_id:$answer}')"
      answer_response="$(request POST "/api/v1/defense/${slug}/education/events/${answer_event_id}/answer" 200 "${answer_body}")"
      jq --exit-status --arg event "${answer_event_id}" --arg answer "${answer_id}" '
        .answer.event_id == $event and .answer.answer_id == $answer
        and (.answer.correct | type) == "boolean"
        and (.answer.explanation | type) == "string" and (.answer.explanation | length) > 0
        and (.answer.effect | type) == "object"
        and (.answer.effect.resource_delta | type) == "number"
        and .duplicate == false
      ' <<<"${answer_response}" >/dev/null
      request POST "/api/v1/defense/${slug}/education/events/${answer_event_id}/answer" 200 "${answer_body}" \
        | jq --exit-status '.duplicate == true and (.answer.correct | type) == "boolean" and (.answer.explanation | length) > 0' >/dev/null
      alternate_answer="$(jq --raw-output --arg question "${question_id}" --arg answer "${answer_id}" 'first([.content.education[] | select(.id == $question)][0].answers[] | select(.id != $answer) | .id)' "${config_file}")"
      conflicting_answer_body="$(jq --null-input --compact-output --arg session_id "${session_id}" --arg token "${session_token}" --arg answer "${alternate_answer}" \
        '{session_id:$session_id,session_token:$token,answer_id:$answer}')"
      request POST "/api/v1/defense/${slug}/education/events/${answer_event_id}/answer" 409 "${conflicting_answer_body}" \
        | jq --exit-status '.error.code == "answer_conflict"' >/dev/null

      education_resource_delta="$(jq '.answer.effect.resource_delta // 0' <<<"${answer_response}")"
      education_trust_delta="$(jq '.answer.effect.trust_delta // 0' <<<"${answer_response}")"
      education_latency_delta="$(jq '.answer.effect.latency_headroom_delta // 0' <<<"${answer_response}")"
      if ((education_resource_delta >= 0)); then
        education_earned=$((education_earned + education_resource_delta))
      else
        education_spent=$((education_spent - education_resource_delta))
      fi
      if [[ "${slug}" == ai-nexus-defense ]]; then
        resource_state="$(jq --compact-output --argjson trust "${education_trust_delta}" --argjson latency "${education_latency_delta}" '
          .trust.remaining = ([.trust.start, ([0, (.trust.remaining + $trust)] | max)] | min)
          | .trust.spent = (.trust.start - .trust.remaining)
          | .latency.remaining = ([.latency.start, ([0, (.latency.remaining + $latency)] | max)] | min)
          | .latency.spent = (.latency.start - .latency.remaining)
        ' <<<"${resource_state}")"
      fi
      education_apply="$(jq --null-input --compact-output \
        --arg event "${answer_event_id}" --argjson resource "${education_resource_delta}" \
        --argjson trust "${education_trust_delta}" --argjson latency "${education_latency_delta}" --argjson resource_state "${resource_state}" \
        '{event_id:$event,resource_delta:$resource,trust_delta:$trust,latency_headroom_delta:$latency,resource_state:$resource_state}')"
      post_telemetry "${slug}" "${session_id}" "${session_token}" defense.education.apply "${sequence}" "${education_apply}" >/dev/null
      sequence=$((sequence + 1))
      education_apply_count=$((education_apply_count + 1))
      answers="$(jq --compact-output --arg event "${answer_event_id}" --arg answer "${answer_id}" \
        '. + [{event_id:$event,answer_id:$answer}]' <<<"${answers}")"
    done < <(jq --raw-output --arg stage "${stage_id}" --argjson wave "${wave}" '
      .content.events[]
      | select(.stage_id == $stage and ((.trigger | gsub("_";"-")) == ("wave-" + ($wave | tostring))))
      | .id
    ' "${config_file}")

    sleep "${milestone_sleep_seconds}"
    cumulative_hist="$(jq --compact-output --arg stage "${stage_id}" --argjson wave "${wave}" '
      [.content.waves[] | select(.stage_id == $stage and .number <= $wave) | .entries[]]
      | reduce .[] as $entry ({}; .[$entry.enemy] = ((.[$entry.enemy] // 0) + $entry.count))
    ' "${config_file}")"
    cumulative_kills="$(jq 'add // 0' <<<"$(jq --compact-output '[.[]]' <<<"${cumulative_hist}")")"
    earned_resource="$(jq --arg stage "${stage_id}" --argjson wave "${wave}" --argjson hist "${cumulative_hist}" --argjson education "${education_earned}" '
      ([.content.enemies[],.content.bosses[]] | map({key:.id,value:.reward}) | from_entries) as $reward
      | ([.content.waves[] | select(.stage_id == $stage and .number <= $wave) | .reward] | add // 0)
        + ($hist | to_entries | map(.value * $reward[.key]) | add // 0) + $education
    ' "${config_file}")"
    remaining_resource=$((starting_resource + earned_resource - tower_spent - education_spent))
    snapshot="$(jq --null-input --compact-output \
      --arg stage "${stage_id}" --argjson wave "${wave}" --argjson health "${starting_health}" \
      --argjson resource "${remaining_resource}" --argjson earned "${earned_resource}" --argjson kills "${cumulative_kills}" \
      --argjson histogram "${cumulative_hist}" --argjson resource_state "${resource_state}" \
      --argjson spent "$((tower_spent + education_spent))" \
      '{stage_id:$stage,wave:$wave,health:$health,resource:$resource,earned_resource:$earned,spent_resource:$spent,sold_resource:0,kills:$kills,escaped:0,spawned:$kills,defeated_by_enemy:$histogram,escaped_by_enemy:{},spawned_by_enemy:$histogram,resource_state:$resource_state}')"
    post_telemetry "${slug}" "${session_id}" "${session_token}" defense.wave.complete "${sequence}" "${snapshot}" >/dev/null
    sequence=$((sequence + 1))
  done

  local duration_tolerance_ms required_wall_ms required_wall_seconds elapsed remaining_sleep
  duration_tolerance_ms="$(jq '.content.balance.duration_tolerance_ms' "${config_file}")"
  # Stay comfortably inside the server's upper duration bound. Using a fixed
  # subtraction made this assertion depend on second-boundary rounding.
  required_wall_ms=$((minimum_duration_ms - duration_tolerance_ms + 1500))
  ((required_wall_ms < 1000)) && required_wall_ms=1000
  required_wall_seconds=$(((required_wall_ms + 999) / 1000))
  ((required_wall_seconds < 1)) && required_wall_seconds=1
  elapsed=$(($(date +%s) - battle_started_epoch))
  remaining_sleep=$((required_wall_seconds - elapsed))
  if ((remaining_sleep > 0)); then sleep "${remaining_sleep}"; fi

  battle_complete="$(jq --null-input --compact-output \
    --arg stage "${stage_id}" --arg hero "${hero_id}" --arg content "${content_version}" --arg policy "${policy_version}" \
    --arg difficulty "${difficulty}" \
    --argjson duration "${minimum_duration_ms}" --argjson health "${starting_health}" --argjson resource "${remaining_resource}" \
    --argjson earned "${earned_resource}" --argjson spent "$((tower_spent + education_spent))" --argjson kills "${cumulative_kills}" --argjson waves "${total_waves}" \
    --argjson histogram "${cumulative_hist}" --argjson resource_state "${resource_state}" \
    '{stage_id:$stage,difficulty:$difficulty,duration_ms:$duration,health:$health,resource:$resource,earned_resource:$earned,spent_resource:$spent,sold_resource:0,kills:$kills,escaped:0,spawned:$kills,waves_completed:$waves,victory:true,hero_id:$hero,hero_level:1,content_version:$content,policy_version:$policy,defeated_by_enemy:$histogram,escaped_by_enemy:{},spawned_by_enemy:$histogram,resource_state:$resource_state}')"
  post_telemetry "${slug}" "${session_id}" "${session_token}" defense.battle.complete "${sequence}" "${battle_complete}" >/dev/null

  result_body="$(jq --null-input --compact-output \
    --arg session_id "${session_id}" --arg token "${session_token}" --arg stage "${stage_id}" --arg hero "${hero_id}" \
    --arg content "${content_version}" --arg policy "${policy_version}" \
    --arg difficulty "${difficulty}" \
    --argjson duration "${minimum_duration_ms}" --argjson health "${starting_health}" --argjson resource "${remaining_resource}" \
    --argjson earned "${earned_resource}" --argjson spent "$((tower_spent + education_spent))" --argjson kills "${cumulative_kills}" --argjson waves "${total_waves}" \
    --argjson histogram "${cumulative_hist}" --argjson resource_state "${resource_state}" --argjson answers "${answers}" \
    '{session_id:$session_id,session_token:$token,stage_id:$stage,difficulty:$difficulty,duration_ms:$duration,remaining_health:$health,remaining_resource:$resource,kills:$kills,escaped:0,spawned:$kills,waves_completed:$waves,victory:true,content_version:$content,policy_version:$policy,answers:$answers,battle:{earned_resource:$earned,spent_resource:$spent,recovered_resource:0,hero_id:$hero,hero_level:1},resource_state:$resource_state,defeated_by_enemy:$histogram,escaped_by_enemy:{},spawned_by_enemy:$histogram,score:999999999,stars:3}')"

  if [[ "${manager_fixture}" == true ]]; then
    assert_unattested_result_rejected "${no_ledger_cookie}" "${slug}" "${no_ledger_id}" "${no_ledger_token}" \
      "${starting_resource}" "${education_earned}" "${education_spent}" "${initial_resource_state}" "${result_body}"
  fi

  tampered_body="$(jq --compact-output '.remaining_resource += 1' <<<"${result_body}")"
  tampered_response="$(request POST "/api/v1/defense/${slug}/results" 422 "${tampered_body}")"
  if ! jq --exit-status '.error.code == "telemetry_attestation_failed"' <<<"${tampered_response}" >/dev/null; then
    printf 'Tampered %s result returned an unexpected error contract: %s\n' "${slug}" "${tampered_response}" >&2
    return 1
  fi
  result_response="$(request POST "/api/v1/defense/${slug}/results" 201 "${result_body}")"
  jq --exit-status --argjson waves "${total_waves}" --argjson education_events "${education_apply_count}" --argjson model_builds "${model_build_count}" '
    .duplicate == false and .result.verified == true and .result.stars == 3
    and .result.verification_method == "server_received_telemetry_v1"
    and .result.attestation.method == "server_received_telemetry_v1"
    and (.result.attestation.digest | test("^[0-9a-f]{64}$"))
    and .result.attestation.waves_started == $waves
    and .result.attestation.waves_completed == $waves
    and .result.attestation.event_count == (2 + ($waves * 2) + $education_events + $model_builds)
    and .result.attestation.model_profile_builds == $model_builds
  ' <<<"${result_response}" >/dev/null
  result_id="$(jq --raw-output '.result.id' <<<"${result_response}")"
  LAST_VERIFIED_RESULT_ID="${result_id}"
  LAST_VERIFIED_RESULT_SCORE="$(jq --raw-output '.result.score' <<<"${result_response}")"
  request POST "/api/v1/defense/${slug}/results" 200 "${result_body}" \
    | jq --exit-status --arg id "${result_id}" '
      (.duplicate == true or .idempotent == true) and .result.id == $id
      and .result.verification_method == "server_received_telemetry_v1"
      and (.result.attestation.digest | test("^[0-9a-f]{64}$"))
    ' >/dev/null
  if [[ "${manager_fixture}" != true ]]; then
    no_ledger_session="$(start_session "${slug}" "${version_id}" body-only-forgery)"
    no_ledger_id="$(jq --raw-output '.session.id' <<<"${no_ledger_session}")"
    no_ledger_token="$(jq --raw-output '.session.session_token' <<<"${no_ledger_session}")"
    local server_tolerance_ms wait_ms
    server_tolerance_ms="$(jq '.content.balance.duration_tolerance_ms' "${config_file}")"
    wait_ms=$((minimum_duration_ms - server_tolerance_ms + 1000))
    ((wait_ms > 0)) && sleep "$(((wait_ms + 999) / 1000))"
    assert_unattested_result_rejected "${ADMIN_COOKIE}" "${slug}" "${no_ledger_id}" "${no_ledger_token}" \
      "${starting_resource}" "${education_earned}" "${education_spent}" "${initial_resource_state}" "${result_body}"
  fi
}

for slug in "${DEFENSE_SLUGS[@]}"; do
  # Every game must attest all three published difficulty profiles. The
  # cross-game SMOKE_DIFFICULTIES mapping remains the first representative
  # pass so a swapped/default difficulty cannot accidentally satisfy the gate.
  representative_difficulty="${SMOKE_DIFFICULTIES[${slug}]}"
  run_verified_battle "${slug}" "${representative_difficulty}"
  for difficulty in casual normal veteran; do
    [[ "${difficulty}" == "${representative_difficulty}" ]] && continue
    run_verified_battle "${slug}" "${difficulty}"
  done
  request GET "/api/v1/defense/${slug}/progress" 200 \
    | jq --exit-status --arg id "${VERSION_IDS[${slug}]}" --argjson stages "${EXPECTED_STAGES[${slug}]}" '
      .version.id == $id and (.items | length) == ($stages * 3)
      and ([.items[] | select(.stage_id == "stage-1" and .attempts == 1 and .completions == 1 and .completed == true and .stars == 3)] | length) == 3
      and ([.items[] | select(.stage_id == "stage-1") | .difficulty] | sort) == (["casual","normal","veteran"] | sort)
      and ([.items[] | select(.stage_id == "stage-2" and .unlocked == true)] | length) == 3
    ' >/dev/null
  request GET "/api/v1/defense/${slug}/rankings?period=all_time&group=individual" 200 \
    | jq --exit-status --arg id "${VERSION_IDS[${slug}]}" '.version.id == $id and .period == "all_time" and .group == "individual" and (.items | length) >= 1' >/dev/null
  mcp_rankings_call="$(jq --null-input --compact-output --arg slug "${slug}" \
    '{jsonrpc:"2.0",id:20,method:"tools/call",params:{name:"defense_rankings_get",arguments:{slug:$slug,period:"season",group:"individual",limit:25}}}')"
  request POST /mcp 200 "${mcp_rankings_call}" 'MCP-Protocol-Version: 2025-11-25' \
    | jq --exit-status --arg id "${VERSION_IDS[${slug}]}" '
      .result.isError == false and .result.structuredContent.version.id == $id
      and .result.structuredContent.period == "season"
      and .result.structuredContent.group == "individual"
      and (.result.structuredContent.items | type) == "array"
      and (.result.structuredContent.items | length) == 0
    ' >/dev/null
  learning_response="$(request GET "/api/v1/defense/${slug}/learning" 200)"
  if [[ "${slug}" == office-guardians ]]; then
    jq --exit-status '.overall_score == 0 and (.topics | length) == 0' <<<"${learning_response}" >/dev/null
  else
    stage_one_id="$(jq --raw-output '[.content.stages[] | select(.number == 1)][0].id' "${TEMP_DIR}/${slug}.json")"
    expected_learning_attempts="$(jq --arg stage "${stage_one_id}" '[.content.events[] | select(.stage_id == $stage)] | length * 3' "${TEMP_DIR}/${slug}.json")"
    jq --exit-status --argjson attempts "${expected_learning_attempts}" '
      .overall_score >= 0 and (.topics | length) >= 1 and ([.topics[].total] | add) == $attempts
    ' <<<"${learning_response}" >/dev/null
  fi
  request GET /api/v1/me/achievements 200 \
    | jq --exit-status --arg code "${slug}-first-defense" '
      ([.items[].code] | index("defender")) != null
      and ([.items[].code] | index($code)) != null
    ' >/dev/null
done

# Fresh installations have no active season, which the loop above proves as an
# exact empty MCP result for every slug. Create one and record one additional
# attested battle so the populated season query cannot regress to all-time or
# remain permanently empty.
pre_season_office_top_score="$(
  request GET '/api/v1/defense/office-guardians/rankings?period=all_time&group=individual' 200 \
    | jq --raw-output '.items[0].score'
)"
active_season_start="$(date -u -d '1 hour ago' '+%Y-%m-%dT%H:%M:%SZ')"
active_season_end="$(date -u -d '1 day' '+%Y-%m-%dT%H:%M:%SZ')"
active_season_body="$(jq --null-input --compact-output \
  --arg start "${active_season_start}" --arg end "${active_season_end}" \
  '{name:"Defense release smoke",description:"Temporary active season coverage",starts_at:$start,ends_at:$end,status:"active"}')"
request POST /api/v1/admin/seasons 201 "${active_season_body}" \
  | jq --exit-status '.season.status == "active" and .season.name == "Defense release smoke"' >/dev/null
run_verified_battle office-guardians normal
[[ -n "${LAST_VERIFIED_RESULT_ID}" && "${LAST_VERIFIED_RESULT_SCORE}" =~ ^[0-9]+$ \
  && "${pre_season_office_top_score}" =~ ^[0-9]+$ \
  && "${LAST_VERIFIED_RESULT_SCORE}" != "${pre_season_office_top_score}" ]] || {
  printf 'Active-season result must be identifiable and differ from the pre-season all-time maximum: result=%s all-time=%s\n' \
    "${LAST_VERIFIED_RESULT_SCORE}" "${pre_season_office_top_score}" >&2
  exit 1
}
request GET '/api/v1/defense/office-guardians/rankings?period=season&group=individual' 200 \
  | jq --exit-status --arg id "${VERSION_IDS[office-guardians]}" --arg username "${USERNAME}" --argjson score "${LAST_VERIFIED_RESULT_SCORE}" '
    .version.id == $id and .period == "season" and .group == "individual"
    and (.items | length) == 1 and .items[0].name == $username and .items[0].score == $score
  ' >/dev/null
populated_season_mcp="$(jq --null-input --compact-output \
  '{jsonrpc:"2.0",id:21,method:"tools/call",params:{name:"defense_rankings_get",arguments:{slug:"office-guardians",period:"season",group:"individual",limit:25}}}')"
request POST /mcp 200 "${populated_season_mcp}" 'MCP-Protocol-Version: 2025-11-25' \
  | jq --exit-status --arg id "${VERSION_IDS[office-guardians]}" --arg username "${USERNAME}" --argjson score "${LAST_VERIFIED_RESULT_SCORE}" '
    .result.isError == false and .result.structuredContent.version.id == $id
    and .result.structuredContent.period == "season"
    and .result.structuredContent.group == "individual"
    and (.result.structuredContent.items | length) == 1
    and .result.structuredContent.items[0].name == $username
    and .result.structuredContent.items[0].score == $score
  ' >/dev/null

request GET /api/v1/me/achievements 200 \
  | jq --exit-status '
    ([.items[].code] | index("triple-guardian")) != null
    and ([.items[].code] | index("security-guardian")) == null
    and ([.items[].code] | index("ai-guardian")) == null
    and ([.items[].code] | index("defense-master")) == null
  ' >/dev/null

# AI Nexus must also accept a real, attested resource-depletion defeat while
# keeping ordinary health above zero. The first wave is fully escaped and only
# three prompt-injection threats are escaped in wave two, which exhausts Trust
# under the published enemy resource_effect + escaped_* rules.
run_ai_depletion_defeat() {
  local slug=ai-nexus-defense
  local config_file="${TEMP_DIR}/${slug}.json"
  local version_id="${VERSION_IDS[${slug}]}"
  local stage_id hero_id content_version policy_version starting_health starting_resource minimum_duration_ms
  local minimum_milestone_ms milestone_sleep_seconds
  stage_id="$(jq --raw-output '[.content.stages[] | select(.number == 1)][0].id' "${config_file}")"
  hero_id="$(jq --raw-output '.content.heroes[0].id' "${config_file}")"
  content_version="$(jq --raw-output '.version.content_version' "${config_file}")"
  policy_version="$(jq --raw-output '.version.policy_version' "${config_file}")"
  starting_health="$(jq --arg stage "${stage_id}" '[.content.stages[] | select(.id == $stage)][0].starting_health' "${config_file}")"
  starting_resource="$(jq --arg stage "${stage_id}" '[.content.stages[] | select(.id == $stage)][0].starting_resource' "${config_file}")"
  minimum_duration_ms="$(jq '.content.balance.min_wave_duration_ms * 2 | ceil' "${config_file}")"
  # The server compares receipt times, so a bare one-second pause can land just
  # under the milestone floor on a loaded host and fail an honest fixture.
  minimum_milestone_ms="$(jq '.content.balance.min_wave_duration_ms / 5 | ceil' "${config_file}")"
  ((minimum_milestone_ms < 250)) && minimum_milestone_ms=250
  ((minimum_milestone_ms > 1000)) && minimum_milestone_ms=1000
  milestone_sleep_seconds=$(((minimum_milestone_ms + 999) / 1000 + 1))

  local resource_state
  resource_state="$(jq --compact-output '.content.balance.resource_state_limits | to_entries | map({key:.key,value:{start:.value,spent:0,remaining:.value}}) | from_entries' "${config_file}")"
  local session session_id session_token sequence=1
  session="$(start_session "${slug}" "${version_id}" ai-resource-depletion)"
  session_id="$(jq --raw-output '.session.id' <<<"${session}")"
  session_token="$(jq --raw-output '.session.session_token' <<<"${session}")"
  ready_data="$(jq --null-input --compact-output --arg stage "${stage_id}" --arg hero "${hero_id}" --arg content "${content_version}" --arg policy "${policy_version}" --argjson state "${resource_state}" \
    '{stage_id:$stage,difficulty:"normal",hero_id:$hero,content_version:$content,policy_version:$policy,resource_state:$state}')"
  post_telemetry "${slug}" "${session_id}" "${session_token}" defense.battle.ready "${sequence}" "${ready_data}" >/dev/null
  sequence=$((sequence + 1))

  local education_event_id question_id answer_id answer_response education_resource_delta education_trust_delta education_latency_delta
  local education_earned=0 education_spent=0
  local cumulative_spawned='{}' cumulative_escaped='{}' cumulative_defeated='{}'
  local cumulative_kills=0 cumulative_escaped_count=0 cumulative_spawned_count=0
  local remaining_health="${starting_health}" earned_resource=0 remaining_resource="${starting_resource}"
  local wave_compute_cost wave_token_cost
  wave_compute_cost="$(jq '.content.resource_rules.wave_compute_cost' "${config_file}")"
  wave_token_cost="$(jq '.content.resource_rules.wave_token_cost' "${config_file}")"

  for wave in 1 2; do
    resource_state="$(jq --compact-output --argjson compute "${wave_compute_cost}" --argjson token "${wave_token_cost}" '
      .compute.remaining = ([0, (.compute.remaining - $compute)] | max)
      | .compute.spent = (.compute.start - .compute.remaining)
      | .token.remaining = ([0, (.token.remaining - $token)] | max)
      | .token.spent = (.token.start - .token.remaining)
    ' <<<"${resource_state}")"
    wave_start="$(jq --null-input --compact-output --arg stage "${stage_id}" --argjson wave "${wave}" --argjson state "${resource_state}" '{stage_id:$stage,wave:$wave,resource_state:$state}')"
    post_telemetry "${slug}" "${session_id}" "${session_token}" defense.wave.start "${sequence}" "${wave_start}" >/dev/null
    sequence=$((sequence + 1))

    if [[ "${wave}" == 1 ]]; then
      education_event_id="$(jq --raw-output --arg stage "${stage_id}" '[.content.events[] | select(.stage_id == $stage and ((.trigger | gsub("_";"-")) == "wave-1"))][0].id' "${config_file}")"
      question_id="$(jq --raw-output --arg event "${education_event_id}" '[.content.events[] | select(.id == $event)][0].education_id' "${config_file}")"
      answer_id="$(jq --raw-output --arg question "${question_id}" '[.content.education[] | select(.id == $question)][0].answers[0].id' "${config_file}")"
      answer_response="$(request POST "/api/v1/defense/${slug}/education/events/${education_event_id}/answer" 200 \
        "$(jq --null-input --compact-output --arg session_id "${session_id}" --arg token "${session_token}" --arg answer "${answer_id}" '{session_id:$session_id,session_token:$token,answer_id:$answer}')")"
      education_resource_delta="$(jq '.answer.effect.resource_delta // 0' <<<"${answer_response}")"
      education_trust_delta="$(jq '.answer.effect.trust_delta // 0' <<<"${answer_response}")"
      education_latency_delta="$(jq '.answer.effect.latency_headroom_delta // 0' <<<"${answer_response}")"
      if ((education_resource_delta >= 0)); then
        education_earned="${education_resource_delta}"
      else
        education_spent="$((-education_resource_delta))"
      fi
      resource_state="$(jq --compact-output --argjson trust "${education_trust_delta}" --argjson latency "${education_latency_delta}" '
        .trust.remaining = ([.trust.start, ([0, (.trust.remaining + $trust)] | max)] | min)
        | .trust.spent = (.trust.start - .trust.remaining)
        | .latency.remaining = ([.latency.start, ([0, (.latency.remaining + $latency)] | max)] | min)
        | .latency.spent = (.latency.start - .latency.remaining)
      ' <<<"${resource_state}")"
      education_apply="$(jq --null-input --compact-output --arg event "${education_event_id}" --argjson resource "${education_resource_delta}" --argjson trust "${education_trust_delta}" --argjson latency "${education_latency_delta}" --argjson state "${resource_state}" \
        '{event_id:$event,resource_delta:$resource,trust_delta:$trust,latency_headroom_delta:$latency,resource_state:$state}')"
      post_telemetry "${slug}" "${session_id}" "${session_token}" defense.education.apply "${sequence}" "${education_apply}" >/dev/null
      sequence=$((sequence + 1))
    fi

    sleep "${milestone_sleep_seconds}"
    cumulative_spawned="$(jq --compact-output --arg stage "${stage_id}" --argjson wave "${wave}" '
      [.content.waves[] | select(.stage_id == $stage and .number <= $wave) | .entries[]]
      | reduce .[] as $entry ({}; .[$entry.enemy] = ((.[$entry.enemy] // 0) + $entry.count))
    ' "${config_file}")"
    if [[ "${wave}" == 1 ]]; then
      cumulative_escaped="${cumulative_spawned}"
    else
      wave_two_first_enemy="$(jq --raw-output --arg stage "${stage_id}" '[.content.waves[] | select(.stage_id == $stage and .number == 2)][0].entries[0].enemy' "${config_file}")"
      cumulative_escaped="$(jq --compact-output --arg enemy "${wave_two_first_enemy}" '.[$enemy] = ((.[$enemy] // 0) + 3)' <<<"${cumulative_escaped}")"
    fi
    cumulative_defeated="$(jq --null-input --compact-output --argjson spawned "${cumulative_spawned}" --argjson escaped "${cumulative_escaped}" '
      $spawned | to_entries | reduce .[] as $entry ({}; .[$entry.key] = ($entry.value - ($escaped[$entry.key] // 0)))
      | with_entries(select(.value > 0))
    ')"
    cumulative_spawned_count="$(jq '[.[]] | add // 0' <<<"${cumulative_spawned}")"
    cumulative_escaped_count="$(jq '[.[]] | add // 0' <<<"${cumulative_escaped}")"
    cumulative_kills="$(jq '[.[]] | add // 0' <<<"${cumulative_defeated}")"
    remaining_health="$(jq --argjson escaped "${cumulative_escaped}" --argjson start "${starting_health}" '
      ([.content.enemies[],.content.bosses[]] | map({key:.id,value:.health_damage}) | from_entries) as $damage
      | [$start - ($escaped | to_entries | map(.value * $damage[.key]) | add // 0), 0] | max
    ' "${config_file}")"
    earned_resource="$(jq --arg stage "${stage_id}" --argjson wave "${wave}" --argjson defeated "${cumulative_defeated}" --argjson education "${education_earned}" '
      ([.content.enemies[],.content.bosses[]] | map({key:.id,value:.reward}) | from_entries) as $reward
      | ([.content.waves[] | select(.stage_id == $stage and .number <= $wave) | .reward] | add // 0)
        + ($defeated | to_entries | map(.value * $reward[.key]) | add // 0) + $education
    ' "${config_file}")"
    remaining_resource=$((starting_resource + earned_resource - education_spent))
    newly_escaped="${cumulative_escaped}"
    if [[ "${wave}" == 2 ]]; then
      first_wave_spawned="$(jq --compact-output --arg stage "${stage_id}" '
        [.content.waves[] | select(.stage_id == $stage and .number == 1) | .entries[]]
        | reduce .[] as $entry ({}; .[$entry.enemy] = ((.[$entry.enemy] // 0) + $entry.count))
      ' "${config_file}")"
      newly_escaped="$(jq --null-input --compact-output --argjson current "${cumulative_escaped}" --argjson previous "${first_wave_spawned}" '
        $current | to_entries | reduce .[] as $entry ({}; .[$entry.key] = ($entry.value - ($previous[$entry.key] // 0)))
        | with_entries(select(.value > 0))
      ')"
    fi
    resource_state="$(jq --null-input --compact-output --slurpfile config "${config_file}" --argjson state "${resource_state}" --argjson escaped "${newly_escaped}" '
      $config[0] as $c
      | ([ $c.content.enemies[], $c.content.bosses[] ] | map({key:.id,value:(.resource_effect // {})}) | from_entries) as $effects
      | ($escaped | to_entries | reduce .[] as $entry ({compute:0,token:0,trust:0,latency:0};
          .compute += (($effects[$entry.key].compute // 0) * $entry.value)
          | .token += (($effects[$entry.key].token // 0) * $entry.value)
          | .trust += ((($effects[$entry.key].trust // 0) + $c.content.resource_rules.escaped_trust_cost) * $entry.value)
          | .latency += ((($effects[$entry.key].latency // 0) + $c.content.resource_rules.escaped_latency_cost) * $entry.value)
        )) as $cost
      | $state
      | .compute.remaining = ([0, (.compute.remaining - $cost.compute)] | max)
      | .token.remaining = ([0, (.token.remaining - $cost.token)] | max)
      | .trust.remaining = ([0, (.trust.remaining - $cost.trust)] | max)
      | .latency.remaining = ([0, (.latency.remaining - $cost.latency)] | max)
      | .compute.spent = (.compute.start - .compute.remaining)
      | .token.spent = (.token.start - .token.remaining)
      | .trust.spent = (.trust.start - .trust.remaining)
      | .latency.spent = (.latency.start - .latency.remaining)
    ')"
    snapshot="$(jq --null-input --compact-output --arg stage "${stage_id}" --argjson wave "${wave}" --argjson health "${remaining_health}" --argjson resource "${remaining_resource}" --argjson earned "${earned_resource}" --argjson spent "${education_spent}" --argjson kills "${cumulative_kills}" --argjson escaped "${cumulative_escaped_count}" --argjson spawned "${cumulative_spawned_count}" --argjson defeated_hist "${cumulative_defeated}" --argjson escaped_hist "${cumulative_escaped}" --argjson spawned_hist "${cumulative_spawned}" --argjson state "${resource_state}" \
      '{stage_id:$stage,wave:$wave,health:$health,resource:$resource,earned_resource:$earned,spent_resource:$spent,sold_resource:0,kills:$kills,escaped:$escaped,spawned:$spawned,defeated_by_enemy:$defeated_hist,escaped_by_enemy:$escaped_hist,spawned_by_enemy:$spawned_hist,resource_state:$state}')"
    post_telemetry "${slug}" "${session_id}" "${session_token}" defense.wave.complete "${sequence}" "${snapshot}" >/dev/null
    sequence=$((sequence + 1))
  done

  [[ "${remaining_health}" -gt 0 ]] || {
    printf 'AI depletion fixture unexpectedly exhausted health instead of an AI resource.\n' >&2
    exit 1
  }
  jq --exit-status '.trust.remaining == 0 and .compute.remaining > 0 and .token.remaining > 0 and .latency.remaining > 0' <<<"${resource_state}" >/dev/null

  sleep "${milestone_sleep_seconds}"
  battle_complete="$(jq --null-input --compact-output --arg stage "${stage_id}" --arg hero "${hero_id}" --arg content "${content_version}" --arg policy "${policy_version}" --argjson duration "${minimum_duration_ms}" --argjson health "${remaining_health}" --argjson resource "${remaining_resource}" --argjson earned "${earned_resource}" --argjson spent "${education_spent}" --argjson kills "${cumulative_kills}" --argjson escaped "${cumulative_escaped_count}" --argjson spawned "${cumulative_spawned_count}" --argjson defeated_hist "${cumulative_defeated}" --argjson escaped_hist "${cumulative_escaped}" --argjson spawned_hist "${cumulative_spawned}" --argjson state "${resource_state}" \
    '{stage_id:$stage,difficulty:"normal",duration_ms:$duration,health:$health,resource:$resource,earned_resource:$earned,spent_resource:$spent,sold_resource:0,kills:$kills,escaped:$escaped,spawned:$spawned,waves_completed:2,victory:false,hero_id:$hero,hero_level:1,content_version:$content,policy_version:$policy,defeated_by_enemy:$defeated_hist,escaped_by_enemy:$escaped_hist,spawned_by_enemy:$spawned_hist,resource_state:$state}')"
  post_telemetry "${slug}" "${session_id}" "${session_token}" defense.battle.complete "${sequence}" "${battle_complete}" >/dev/null
  result_body="$(jq --null-input --compact-output --arg session_id "${session_id}" --arg token "${session_token}" --arg stage "${stage_id}" --arg hero "${hero_id}" --arg content "${content_version}" --arg policy "${policy_version}" --arg event "${education_event_id}" --arg answer "${answer_id}" --argjson duration "${minimum_duration_ms}" --argjson health "${remaining_health}" --argjson resource "${remaining_resource}" --argjson earned "${earned_resource}" --argjson spent "${education_spent}" --argjson kills "${cumulative_kills}" --argjson escaped "${cumulative_escaped_count}" --argjson spawned "${cumulative_spawned_count}" --argjson defeated_hist "${cumulative_defeated}" --argjson escaped_hist "${cumulative_escaped}" --argjson spawned_hist "${cumulative_spawned}" --argjson state "${resource_state}" \
    '{session_id:$session_id,session_token:$token,stage_id:$stage,difficulty:"normal",duration_ms:$duration,remaining_health:$health,remaining_resource:$resource,kills:$kills,escaped:$escaped,spawned:$spawned,waves_completed:2,victory:false,content_version:$content,policy_version:$policy,answers:[{event_id:$event,answer_id:$answer}],battle:{earned_resource:$earned,spent_resource:$spent,recovered_resource:0,hero_id:$hero,hero_level:1},resource_state:$state,defeated_by_enemy:$defeated_hist,escaped_by_enemy:$escaped_hist,spawned_by_enemy:$spawned_hist,score:999999999,stars:3}')"
  depletion_result="$(request POST "/api/v1/defense/${slug}/results" 201 "${result_body}")"
  jq --exit-status --argjson health "${remaining_health}" '
    .result.verified == true and .result.stars == 0
    and .result.verification_method == "server_received_telemetry_v1"
    and .result.attestation.waves_started == 2 and .result.attestation.waves_completed == 2
    and .result.attestation.resource_state.trust.remaining == 0
    and .result.resource_state.trust.remaining == 0
    and $health > 0
  ' <<<"${depletion_result}" >/dev/null
}

run_ai_depletion_defeat

# Content Studio uses immutable version snapshots and optimistic concurrency.
# Exercise every game's editor/test/preview contract before the configurable
# approval paths, then leave a fresh draft for the browser gate.
approval_restore_body="$(request GET /api/v1/admin/settings/approval 200 | jq --compact-output '{value:.value}')"
approval_changed=true
request PUT /api/v1/admin/settings/approval 200 \
  '{"value":{"enabled":false,"manager_required":false,"separation_of_duties":false}}' >/dev/null

declare -A STUDIO_VERSION_IDS=()
declare -A STUDIO_POLICY_VERSIONS=()
for slug in "${DEFENSE_SLUGS[@]}"; do
  studio_policy="smoke-policy-${EXPECTED_VERSION}-${slug}"
  create_body="$(jq --null-input --compact-output --arg label "release-smoke-${slug}" --arg policy "${studio_policy}" \
    '{label:$label,notes:"v0.4.0 Defense Content Studio smoke",policy_version:$policy}')"
  created="$(request POST "/api/v1/admin/defense/${slug}/versions" 201 "${create_body}")"
  version_id="$(jq --raw-output '.version.id' <<<"${created}")"
  STUDIO_VERSION_IDS[${slug}]="${version_id}"
  STUDIO_POLICY_VERSIONS[${slug}]="${studio_policy}"
  jq --exit-status --arg policy "${studio_policy}" '.version.policy_version == $policy' <<<"${created}" >/dev/null
  section_response="$(request GET "/api/v1/admin/defense/${slug}/drafts/stages?version_id=${version_id}" 200)"
  jq --exit-status --arg id "${version_id}" --arg policy "${studio_policy}" --argjson count "${EXPECTED_STAGES[${slug}]}" '
    .version.id == $id and .version.status == "draft" and .section == "stages"
    and .version.policy_version == $policy
    and (.version.checksum | test("^[0-9a-f]{64}$")) and (.data | length) == $count
  ' <<<"${section_response}" >/dev/null
  checksum="$(jq --raw-output '.version.checksum' <<<"${section_response}")"
  section_body="$(jq --compact-output '{data:.data}' <<<"${section_response}")"
  request PUT "/api/v1/admin/defense/${slug}/drafts/stages?version_id=${version_id}" 428 "${section_body}" \
    | jq --exit-status '.error.code == "precondition_required"' >/dev/null
  request PUT "/api/v1/admin/defense/${slug}/drafts/stages?version_id=${version_id}" 409 "${section_body}" \
    'If-Match: "0000000000000000000000000000000000000000000000000000000000000000"' \
    | jq --exit-status '.error.code == "stale_version"' >/dev/null
  saved="$(request PUT "/api/v1/admin/defense/${slug}/drafts/stages?version_id=${version_id}" 200 "${section_body}" "If-Match: \"${checksum}\"")"
  jq --exit-status --arg id "${version_id}" '.version.id == $id and .version.status == "draft" and (.version.checksum | test("^[0-9a-f]{64}$"))' <<<"${saved}" >/dev/null
  tested="$(request POST "/api/v1/admin/defense/${slug}/versions/${version_id}/test" 200)"
  jq --exit-status --arg id "${version_id}" --argjson stages "${EXPECTED_STAGES[${slug}]}" --argjson towers "${EXPECTED_TOWERS[${slug}]}" '
    .version.id == $id and .version.status == "testing" and .validation.valid == true
    and .validation.stages == $stages and .validation.towers == $towers
  ' <<<"${tested}" >/dev/null
  preview_file="${TEMP_DIR}/${slug}-preview.json"
  request GET "/api/v1/defense/${slug}/versions/${version_id}/preview" 200 >"${preview_file}"
  jq --exit-status --arg id "${version_id}" --arg slug "${slug}" --arg policy "${studio_policy}" '
    .practice_only == true and .preview == true and .version.id == $id and .game.slug == $slug
    and .version.policy_version == $policy
  ' "${preview_file}" >/dev/null
  assert_public_redaction "${preview_file}"
done

request GET "/api/v1/defense/office-guardians/versions/${STUDIO_VERSION_IDS[cyber-fortress]}/preview" 404 \
  | jq --exit-status '.error.code == "not_found"' >/dev/null
request GET "/api/v1/admin/defense/office-guardians/drafts/stages?version_id=${STUDIO_VERSION_IDS[cyber-fortress]}" 404 \
  | jq --exit-status '.error.code == "draft_not_found"' >/dev/null

# Content Studio must reject packs whose worst-case cumulative battle.complete
# document cannot cross the same 4 KiB transport boundary. Build a bounded but
# intentionally wide 64-enemy stage using valid 32-character IDs, then prove
# both Test and Publish fail closed without disturbing the valid draft.
payload_draft="$(request POST /api/v1/admin/defense/office-guardians/versions 201 \
  '{"label":"oversized-telemetry-payload","notes":"must fail the 4 KiB cumulative telemetry validator"}')"
payload_draft_id="$(jq --raw-output '.version.id' <<<"${payload_draft}")"
payload_enemies="$(request GET "/api/v1/admin/defense/office-guardians/drafts/enemies?version_id=${payload_draft_id}" 200)"
payload_checksum="$(jq --raw-output '.version.checksum' <<<"${payload_enemies}")"
payload_enemy_body="$(jq --compact-output '
  .data as $base
  | {data:($base + [range(1;65) as $index
      | ($index|tostring) as $number
      | ($base[0] + {
          id:("payload_enemy_" + (("000000000000000000"+$number)[-18:])),
          name:("Telemetry Payload Enemy " + $number)
        })
    ])}
' <<<"${payload_enemies}")"
payload_enemy_saved="$(request PUT "/api/v1/admin/defense/office-guardians/drafts/enemies?version_id=${payload_draft_id}" 200 "${payload_enemy_body}" "If-Match: \"${payload_checksum}\"")"
payload_checksum="$(jq --raw-output '.version.checksum' <<<"${payload_enemy_saved}")"
payload_waves="$(request GET "/api/v1/admin/defense/office-guardians/drafts/waves?version_id=${payload_draft_id}" 200)"
[[ "$(jq --raw-output '.version.checksum' <<<"${payload_waves}")" == "${payload_checksum}" ]]
payload_wave_body="$(jq --compact-output '
  {data:(.data | map(
    if .stage_id == "stage-1" then
      . as $wave
      | .entries = [range(0;8) as $slot
          | (((($wave.number - 1) * 8) + $slot + 1)|tostring) as $number
          | {enemy:("payload_enemy_" + (("000000000000000000"+$number)[-18:])),count:1,interval:0.5}
        ]
    else . end
  ))}
' <<<"${payload_waves}")"
request PUT "/api/v1/admin/defense/office-guardians/drafts/waves?version_id=${payload_draft_id}" 200 "${payload_wave_body}" "If-Match: \"${payload_checksum}\"" >/dev/null
request POST "/api/v1/admin/defense/office-guardians/versions/${payload_draft_id}/test" 422 \
  | jq --exit-status '.error.code == "content_validation_failed" and (.error.message | contains("4 KiB transport limit"))' >/dev/null
request POST "/api/v1/admin/defense/office-guardians/versions/${payload_draft_id}/publish" 422 '{}' \
  | jq --exit-status '.error.code == "content_validation_failed" and (.error.message | contains("4 KiB transport limit"))' >/dev/null

# Approval disabled: a tested version publishes immediately without creating a
# review request. Existing sessions remain pinned to their immutable UUID.
assert_fresh_published_boundary() {
  local slug="$1"
  local version_id="$2"
  local policy_version="$3"
  local stages="${EXPECTED_STAGES[${slug}]}"
  local fresh_config="${TEMP_DIR}/${slug}-fresh-published.json"
  request GET "/api/v1/defense/${slug}/config" 200 >"${fresh_config}"
  jq --exit-status --arg id "${version_id}" --arg policy "${policy_version}" \
    '.version.id == $id and .version.policy_version == $policy' "${fresh_config}" >/dev/null
  request GET "/api/v1/defense/${slug}/progress" 200 \
    | jq --exit-status --arg id "${version_id}" --argjson stages "${stages}" '
      .version.id == $id and (.items | length) == ($stages * 3)
      and ([.items[] | select(.attempts != 0 or .completions != 0 or .completed != false or .stars != 0 or .best_score != 0)] | length) == 0
      and ([.items[] | select(.stage_id == "stage-1" and .unlocked == true)] | length) == 3
      and ([.items[] | select(.stage_id != "stage-1" and .unlocked == true)] | length) == 0
    ' >/dev/null
  request GET "/api/v1/defense/${slug}/rankings?period=all_time&group=individual" 200 \
    | jq --exit-status --arg id "${version_id}" '.version.id == $id and (.items | length) == 0' >/dev/null
  request GET "/api/v1/defense/${slug}/learning" 200 \
    | jq --exit-status --arg id "${version_id}" --arg policy "${policy_version}" \
      '.version.id == $id and .policy_version == $policy and .overall_score == 0 and (.topics | length) == 0' >/dev/null
}

office_previous_id="${VERSION_IDS[office-guardians]}"
office_publish_id="${STUDIO_VERSION_IDS[office-guardians]}"
request POST "/api/v1/admin/defense/office-guardians/versions/${office_publish_id}/publish" 200 '{}' \
  | jq --exit-status --arg id "${office_publish_id}" '.published == true and .approval_required == false and .version.id == $id and .version.status == "published"' >/dev/null
request POST /api/v1/games/office-guardians/sessions 409 \
  "$(jq --null-input --compact-output --arg id "${office_previous_id}" '{metadata:{client:"release-smoke",defense_content_version_id:$id}}')" \
  | jq --exit-status '.error.code == "defense_config_stale"' >/dev/null
assert_fresh_published_boundary office-guardians "${office_publish_id}" "${STUDIO_POLICY_VERSIONS[office-guardians]}"

# A rollback is a new Draft copied from a selected immutable historical UUID;
# the old row is never reactivated in place. Its policy boundary is explicit
# and it must pass the same Test/preview gates before any later publication.
rollback_policy="rollback-policy-${EXPECTED_VERSION}"
rollback_created="$(request POST /api/v1/admin/defense/office-guardians/versions 201 \
  "$(jq --null-input --compact-output --arg source "${office_previous_id}" --arg policy "${rollback_policy}" \
    '{label:"rollback-source-smoke",notes:"historical source rollback smoke",source_version_id:$source,policy_version:$policy}')")"
rollback_id="$(jq --raw-output '.version.id' <<<"${rollback_created}")"
jq --exit-status --arg checksum "${VERSION_CHECKSUMS[office-guardians]}" --arg policy "${rollback_policy}" --arg source "${office_previous_id}" '
  .version.status == "draft" and .version.checksum == $checksum and .version.policy_version == $policy
  and .version.source_version_id == $source
' <<<"${rollback_created}" >/dev/null
request POST "/api/v1/admin/defense/office-guardians/versions/${rollback_id}/test" 200 \
  | jq --exit-status --arg id "${rollback_id}" '.version.id == $id and .version.status == "testing" and .validation.valid == true' >/dev/null
rollback_preview="${TEMP_DIR}/office-rollback-preview.json"
request GET "/api/v1/defense/office-guardians/versions/${rollback_id}/preview" 200 >"${rollback_preview}"
jq --exit-status --arg id "${rollback_id}" --arg policy "${rollback_policy}" '
  .practice_only == true and .version.id == $id and .version.policy_version == $policy
' "${rollback_preview}" >/dev/null
assert_public_redaction "${rollback_preview}"

# Record one battle against the newly published boundary so the browser gate
# can prove the result response unlocks stage 2 immediately without reload.
cp -- "${TEMP_DIR}/office-guardians-fresh-published.json" "${TEMP_DIR}/office-guardians.json"
VERSION_IDS[office-guardians]="${office_publish_id}"
ACTIVE_POLICY_VERSIONS[office-guardians]="${STUDIO_POLICY_VERSIONS[office-guardians]}"
run_verified_battle office-guardians casual
request GET /api/v1/defense/office-guardians/progress 200 \
  | jq --exit-status --arg id "${office_publish_id}" '.version.id == $id and ([.items[] | select(.stage_id == "stage-2" and .unlocked == true)] | length) == 3' >/dev/null

if [[ "${manager_fixture}" == true ]]; then
  request PUT /api/v1/admin/settings/approval 200 \
    '{"value":{"enabled":true,"manager_required":true,"separation_of_duties":true}}' >/dev/null
else
  request PUT /api/v1/admin/settings/approval 200 \
    '{"value":{"enabled":true,"manager_required":false,"separation_of_duties":false}}' >/dev/null
fi

cyber_review_id="${STUDIO_VERSION_IDS[cyber-fortress]}"
request POST "/api/v1/admin/defense/cyber-fortress/versions/${cyber_review_id}/publish" 202 '{}' \
  | jq --exit-status --arg id "${cyber_review_id}" '.approval_required == true and .published == false and .version.id == $id and .version.status == "pending_approval"' >/dev/null

if [[ "${manager_fixture}" == true ]]; then
  request_with_cookie "${empty_cookie}" GET /api/v1/defense/versions/pending 403 \
    | jq --exit-status '.error.code == "team_required"' >/dev/null
  request_with_cookie "${empty_cookie}" GET "/api/v1/defense/cyber-fortress/versions/${cyber_review_id}/preview" 403 \
    | jq --exit-status '.error.code == "team_required"' >/dev/null
  request_with_cookie "${empty_cookie}" POST "/api/v1/defense/versions/${cyber_review_id}/review" 403 \
    '{"decision":"rejected","comment":"unassigned manager review must fail"}' \
    | jq --exit-status '.error.code == "team_required"' >/dev/null
  request_with_cookie "${other_cookie}" GET /api/v1/defense/versions/pending 200 \
    | jq --exit-status --arg id "${cyber_review_id}" '([.items[] | select(.id == $id)] | length) == 0' >/dev/null
  request_with_cookie "${other_cookie}" GET "/api/v1/defense/cyber-fortress/versions/${cyber_review_id}/preview" 403 \
    | jq --exit-status '.error.code == "different_team"' >/dev/null
  request_with_cookie "${other_cookie}" POST "/api/v1/defense/versions/${cyber_review_id}/review" 403 \
    '{"decision":"rejected","comment":"cross-team review must fail"}' \
    | jq --exit-status '.error.code == "different_team"' >/dev/null
  request_with_cookie "${same_cookie}" GET /api/v1/defense/versions/pending 200 \
    | jq --exit-status --arg id "${cyber_review_id}" '([.items[] | select(.id == $id and .game_slug == "cyber-fortress" and .status == "pending_approval")] | length) == 1' >/dev/null
  same_preview="${TEMP_DIR}/cyber-manager-preview.json"
  request_with_cookie "${same_cookie}" GET "/api/v1/defense/cyber-fortress/versions/${cyber_review_id}/preview" 200 >"${same_preview}"
  assert_public_redaction "${same_preview}"
  request_with_cookie "${same_cookie}" POST "/api/v1/defense/versions/${cyber_review_id}/review" 200 \
    '{"decision":"rejected","comment":"release smoke rejection"}' \
    | jq --exit-status '.decision == "rejected" and .rejected == true and .version.status == "draft"' >/dev/null
else
  request POST "/api/v1/defense/versions/${cyber_review_id}/review" 200 \
    '{"decision":"rejected","comment":"release smoke rejection"}' \
    | jq --exit-status '.decision == "rejected" and .rejected == true and .version.status == "draft"' >/dev/null
fi
request GET /api/v1/defense/cyber-fortress/config 200 \
  | jq --exit-status --arg id "${VERSION_IDS[cyber-fortress]}" --arg policy "${ACTIVE_POLICY_VERSIONS[cyber-fortress]}" \
    '.version.id == $id and .version.policy_version == $policy' >/dev/null
request GET '/api/v1/defense/cyber-fortress/rankings?period=all_time&group=individual' 200 \
  | jq --exit-status --arg id "${VERSION_IDS[cyber-fortress]}" '.version.id == $id and (.items | length) >= 1' >/dev/null

ai_previous_id="${VERSION_IDS[ai-nexus-defense]}"
ai_review_id="${STUDIO_VERSION_IDS[ai-nexus-defense]}"
request POST "/api/v1/admin/defense/ai-nexus-defense/versions/${ai_review_id}/publish" 202 '{}' \
  | jq --exit-status '.approval_required == true and .version.status == "pending_approval"' >/dev/null
if [[ "${manager_fixture}" == true ]]; then
  request_with_cookie "${same_cookie}" POST "/api/v1/defense/versions/${ai_review_id}/review" 200 \
    '{"decision":"approved","comment":"release smoke approval"}' \
    | jq --exit-status '.decision == "approved" and .approved == true and .version.status == "approved"' >/dev/null
else
  request POST "/api/v1/defense/versions/${ai_review_id}/review" 200 \
    '{"decision":"approved","comment":"release smoke approval"}' \
    | jq --exit-status '.decision == "approved" and .approved == true and .version.status == "approved"' >/dev/null
fi
request POST "/api/v1/admin/defense/ai-nexus-defense/versions/${ai_review_id}/publish" 200 '{}' \
  | jq --exit-status '.published == true and .approval_required == true and .version.status == "published"' >/dev/null
request POST /api/v1/games/ai-nexus-defense/sessions 409 \
  "$(jq --null-input --compact-output --arg id "${ai_previous_id}" '{metadata:{client:"release-smoke",defense_content_version_id:$id}}')" \
  | jq --exit-status '.error.code == "defense_config_stale"' >/dev/null
assert_fresh_published_boundary ai-nexus-defense "${ai_review_id}" "${STUDIO_POLICY_VERSIONS[ai-nexus-defense]}"
VERSION_IDS[ai-nexus-defense]="${ai_review_id}"
ACTIVE_POLICY_VERSIONS[ai-nexus-defense]="${STUDIO_POLICY_VERSIONS[ai-nexus-defense]}"

request PUT /api/v1/admin/settings/approval 200 "${approval_restore_body}" >/dev/null
approval_changed=false

for slug in "${DEFENSE_SLUGS[@]}"; do
  telemetry_report="$(request GET "/api/v1/admin/defense/${slug}/telemetry?days=30" 200)"
  if [[ "${slug}" == ai-nexus-defense ]]; then
    jq --exit-status --arg slug "${slug}" --arg id "${VERSION_IDS[${slug}]}" '
      .game == $slug and .version.id == $id and .runs == 0 and .unique_users == 0
      and .average_game_score == 0 and .average_score == .average_game_score
      and .summary.average_game_score == 0 and .summary.average_score == .summary.average_game_score
      and .verification_method == "server_received_telemetry_v1"
    ' <<<"${telemetry_report}" >/dev/null
  else
    jq --exit-status --arg slug "${slug}" --arg id "${VERSION_IDS[${slug}]}" '
      .game == $slug and .version.id == $id and .runs >= 1 and .unique_users >= 1
      and .average_game_score > 0 and .average_score == .average_game_score
      and .summary.average_game_score > 0 and .summary.average_score == .summary.average_game_score
      and .verification_method == "server_received_telemetry_v1"
    ' <<<"${telemetry_report}" >/dev/null
  fi
  learning_report="$(request GET "/api/v1/admin/defense/${slug}/learning-report" 200)"
  if [[ "${slug}" == office-guardians ]]; then
    jq --exit-status --arg policy "${ACTIVE_POLICY_VERSIONS[${slug}]}" '.game == "office-guardians" and .policy_version == $policy and .education_enabled == false and (.topics | length) == 0 and (.departments | type) == "array"' <<<"${learning_report}" >/dev/null
  elif [[ "${slug}" == ai-nexus-defense ]]; then
    jq --exit-status --arg slug "${slug}" --arg policy "${ACTIVE_POLICY_VERSIONS[${slug}]}" '.game == $slug and .policy_version == $policy and .education_enabled == true and (.topics | length) == 0 and .participants == 0 and (.department_visible | type) == "boolean"' <<<"${learning_report}" >/dev/null
  else
    jq --exit-status --arg slug "${slug}" --arg policy "${ACTIVE_POLICY_VERSIONS[${slug}]}" '.game == $slug and .policy_version == $policy and .education_enabled == true and (.topics | length) >= 1 and ([.topics[].total] | add) >= 1 and (.department_visible | type) == "boolean"' <<<"${learning_report}" >/dev/null
  fi

  browser_draft="$(request POST "/api/v1/admin/defense/${slug}/versions" 201 \
    "$(jq --null-input --compact-output --arg label "browser-smoke-${slug}" '{label:$label,notes:"editable browser smoke draft"}')")"
  jq --exit-status '.version.status == "draft" and (.version.id | test("^[0-9a-f-]{36}$"))' <<<"${browser_draft}" >/dev/null
done

printf 'Defense Series release smoke passed: runtime, education, Studio, review, reports, and browser fixtures (%s, igame v%s)\n' "${BASE_URL}" "${EXPECTED_VERSION}"

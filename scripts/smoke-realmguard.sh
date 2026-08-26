#!/usr/bin/env bash
set -Eeuo pipefail
trap 'printf "RealmGuard smoke failed at line %d.\n" "${LINENO}" >&2' ERR

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly BASE_URL="${1:-http://127.0.0.1:8080}"
readonly USERNAME="${2:-}"
readonly PASSWORD="${3:-}"
readonly EXPECTED_VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
readonly EXPECTED_REALMGUARD_CONTENT_VERSION="0.3.1"

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

cookie_jar="$(mktemp)"
response_file="$(mktemp)"
approval_changed=false
approval_restore_body=''
cleanup() {
  if [[ "${approval_changed}" == true && -n "${approval_restore_body}" ]]; then
    curl --silent --show-error --max-time 10 --request PUT \
      --cookie "${cookie_jar}" --header 'Content-Type: application/json' \
      --data-binary "${approval_restore_body}" \
      "${BASE_URL}/api/v1/admin/settings/approval" >/dev/null || \
      printf '%s\n' 'Warning: failed to restore the approval setting after RealmGuard smoke.' >&2
  fi
  rm -f -- "${cookie_jar}" "${response_file}"
}
trap cleanup EXIT

request() {
  local method="$1"
  local path="$2"
  local expected_status="$3"
  local body="${4:-}"
  local extra_header="${5:-}"
  local status
  local -a args=(
    --silent --show-error --max-time 20
    --request "${method}"
    --cookie "${cookie_jar}"
    --cookie-jar "${cookie_jar}"
    --output "${response_file}"
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
    jq . "${response_file}" >&2 2>/dev/null || sed -n '1,80p' "${response_file}" >&2
    exit 1
  fi
  cat "${response_file}"
}

new_uuid() {
  local value=''
  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    IFS= read -r value < /proc/sys/kernel/random/uuid
  elif command -v uuidgen >/dev/null 2>&1; then
    value="$(uuidgen)"
  else
    printf '%s\n' 'A UUID source is required for RealmGuard telemetry smoke.' >&2
    exit 1
  fi
  printf '%s' "${value,,}"
}

telemetry_payload() {
  local session_id="$1"
  local session_token="$2"
  local event="$3"
  local sequence="$4"
  local client_event_id="$5"
  local data="$6"
  jq --null-input --compact-output \
    --arg session_id "${session_id}" --arg session_token "${session_token}" \
    --arg event "${event}" --arg client_event_id "${client_event_id}" \
    --argjson sequence "${sequence}" --argjson data "${data}" \
    '{game_id:"realmguard",session_id:$session_id,session_token:$session_token,event:$event,data:$data,client_event_id:$client_event_id,sequence:$sequence}'
}

post_telemetry() {
  local session_id="$1"
  local session_token="$2"
  local event="$3"
  local sequence="$4"
  local data="$5"
  local expected_status="${6:-202}"
  local client_event_id="${7:-$(new_uuid)}"
  request POST /api/v1/telemetry "${expected_status}" \
    "$(telemetry_payload "${session_id}" "${session_token}" "${event}" "${sequence}" "${client_event_id}" "${data}")"
}

version_json="$(request GET /api/v1/version 200)"
jq --exit-status --arg version "${EXPECTED_VERSION}" '.version == $version and (.commit | length) > 0 and (.build_date | length) > 0' <<<"${version_json}" >/dev/null

login_body="$(jq --null-input --compact-output --arg username "${USERNAME}" --arg password "${PASSWORD}" '{username:$username,password:$password}')"
request POST /api/v1/auth/login 200 "${login_body}" >/dev/null
request GET /api/v1/me 200 | jq --exit-status --arg username "${USERNAME}" '.user.username == $username and .user.role == "admin"' >/dev/null

game_json="$(request GET /api/v1/games/realmguard 200)"
jq --exit-status --arg version "${EXPECTED_REALMGUARD_CONTENT_VERSION}" '.game.slug == "realmguard" and .game.game_url == "/games/realmguard" and .game.version == $version and .game.status == "active"' <<<"${game_json}" >/dev/null
curl --fail --silent --show-error --max-time 10 "${BASE_URL}/games/realmguard" | grep -Eiq '<!doctype html|<html'

config_json="$(request GET /api/v1/realmguard/config 200)"
jq --exit-status --arg version "${EXPECTED_REALMGUARD_CONTENT_VERSION}" '
  .version.content_version == $version
  and .version.asset_version == "procedural-2"
  and (.version.id | test("^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"))
  and (.version.checksum | test("^[0-9a-f]{64}$"))
  and ([.stages[] | select(.mode == "campaign")] | length) >= 10
  and ([.stages[] | select(.mode == "endless")] | length) >= 1
  and (.towers | length) >= 4
  and ([.towers[].branches[]] | length) >= 8
  and (.enemies | length) >= 10
  and (.enemies | length) <= 16
  and (.bosses | length) >= 2
  and (.bosses | length) <= 4
  and (.heroes | length) >= 3
  and (.skills | length) >= 3
  and ([.stages[].waves[]
        | ((.entries | length) <= 8 and ([.entries[].count] | add) <= 500)]
       | all)
  and ([.stages[].id, .stages[].tower_spots[].id, .stages[].waves[].id,
        .enemies[].id, .bosses[].id, .towers[].id, .towers[].branches[].id,
        .heroes[].id, .skills[].id]
       | all(test("^[a-z][a-z0-9_-]{0,31}$")))
  and ([.balance.difficulties | has("casual"), has("normal"), has("veteran")] | all)
  and ([.stages[] | select(.id == "stage-1")][0] as $stage
    | ($stage.path | length) >= 2
    and ($stage.tower_spots | length) >= 8
    and ($stage.waves | length) >= 8
    and ($stage.waves | length) <= 15)
' <<<"${config_json}" >/dev/null
realmguard_version_id="$(jq --raw-output '.version.id' <<<"${config_json}")"

# Mirror the server's deliberately pessimistic battle.complete envelope. The
# published enemy/boss IDs must leave the complete cumulative histograms below
# the same 4 KiB event-data boundary enforced by the API.
worst_histogram="$(jq --compact-output '
  [.enemies[],.bosses[]]
  | reduce .[] as $enemy ({}; .[$enemy.id] = 2147483647)
' <<<"${config_json}")"
worst_battle_complete="$(jq --null-input --compact-output \
  --argjson histogram "${worst_histogram}" \
  '{stage_id:("s"*32),mode:"campaign",difficulty:"veteran",duration_ms:315360000000,lives:200,gold:9000000000000000,earned_gold:9000000000000000,spent_gold:9000000000000000,sold_gold:9000000000000000,kills:2147483647,escaped:2147483647,spawned:2147483647,waves:10000,waves_completed:10000,hero_id:("h"*32),hero_level:10,content_version:("c"*100),balance_version:("b"*100),stage_version:("s"*100),asset_version:("a"*100),victory:false,defeated_by_enemy:$histogram,escaped_by_enemy:$histogram,spawned_by_enemy:$histogram}')"
if (( ${#worst_battle_complete} > 4096 )); then
  printf 'Published RealmGuard roster exceeds the 4 KiB worst-case telemetry payload budget: %d bytes\n' "${#worst_battle_complete}" >&2
  exit 1
fi

# A RealmGuard session must pin the exact published config that the browser
# rendered. Missing pins fail with 428 and stale/unpublished UUIDs with 409.
request POST /api/v1/games/realmguard/sessions 428 \
  '{"metadata":{"client":"release-smoke","purpose":"missing-config-pin"}}' \
  | jq --exit-status '.error.code == "realmguard_version_required"' >/dev/null
stale_version_id="$(new_uuid)"
stale_session_body="$(jq --null-input --compact-output --arg version_id "${stale_version_id}" '{metadata:{client:"release-smoke",purpose:"stale-config-pin",realmguard_version_id:$version_id}}')"
request POST /api/v1/games/realmguard/sessions 409 "${stale_session_body}" \
  | jq --exit-status '.error.code == "realmguard_config_stale"' >/dev/null

request GET /api/v1/realmguard/version 200 \
  | jq --exit-status --arg version "${EXPECTED_REALMGUARD_CONTENT_VERSION}" '.version.content_version == $version and (.version.checksum | test("^[0-9a-f]{64}$"))' >/dev/null
request GET /api/v1/realmguard/progress 200 \
  | jq --exit-status '.total_stars == 0 and .unlocked_stage == 1 and ([.items[] | select(.stage_id == "stage-1" and .unlocked)] | length) == 3 and ([.heroes[] | select(.hero_id == "aerin" and .unlocked)] | length) == 1' >/dev/null

request GET '/api/v1/realmguard/rankings?group=individual&metric=score&period=weekly&mode=campaign&difficulty=normal&stage_id=stage-1' 200 \
  | jq --exit-status '.group == "individual" and .metric == "score" and .period == "weekly" and (.items | type) == "array"' >/dev/null
request GET '/api/v1/realmguard/rankings?group=hero&metric=score&period=season&mode=campaign&difficulty=normal&hero_id=aerin' 200 \
  | jq --exit-status '.group == "hero" and .metric == "score" and .period == "season"' >/dev/null
request GET '/api/v1/realmguard/rankings?group=department&metric=stars&period=all_time&mode=campaign&difficulty=normal&stage_id=stage-1' 200 \
  | jq --exit-status '.group == "department" and .metric == "stars" and .period == "all"' >/dev/null
request GET '/api/v1/rankings/realmguard?period=weekly' 409 \
  | jq --exit-status '.error.code == "realmguard_ranking_required"' >/dev/null

# The generic score endpoint must never accept a RealmGuard result.
generic_session_body="$(jq --null-input --compact-output --arg version_id "${realmguard_version_id}" '{metadata:{client:"release-smoke",purpose:"generic-score-rejection",realmguard_version_id:$version_id}}')"
generic_session="$(request POST /api/v1/games/realmguard/sessions 201 "${generic_session_body}")"
jq --exit-status --arg version_id "${realmguard_version_id}" '.session.realmguard_version_id == $version_id' <<<"${generic_session}" >/dev/null
generic_session_id="$(jq --raw-output '.session.id' <<<"${generic_session}")"
generic_session_token="$(jq --raw-output '.session.session_token' <<<"${generic_session}")"
generic_score_body="$(jq --null-input --compact-output \
  --arg session_id "${generic_session_id}" --arg session_token "${generic_session_token}" \
  '{game_id:"realmguard",session_id:$session_id,session_token:$session_token,score:1,metadata:{}}')"
request POST /api/v1/scores 409 "${generic_score_body}" \
  | jq --exit-status '.error.code == "authoritative_result_required"' >/dev/null

# A fabricated zero-wave "defeat" must be rejected and audited; it cannot be
# used to close a session or create progress.
invalid_duration_ms="$(jq '(.balance.min_wave_duration_ms | ceil)' <<<"${config_json}")"
invalid_sleep_seconds="$(((invalid_duration_ms + 999) / 1000))"
sleep "${invalid_sleep_seconds}"
invalid_defeat_body="$(jq --null-input --compact-output \
  --arg session_id "${generic_session_id}" --arg session_token "${generic_session_token}" \
  --arg content_version "$(jq --raw-output '.version.content_version' <<<"${config_json}")" \
  --arg stage_version "$(jq --raw-output '[.stages[] | select(.id == "stage-1")][0].version' <<<"${config_json}")" \
  --arg balance_version "$(jq --raw-output '.version.balance_version' <<<"${config_json}")" \
  --arg asset_version "$(jq --raw-output '.version.asset_version' <<<"${config_json}")" \
  --argjson initial_gold "$(jq '([.stages[] | select(.id == "stage-1")][0].starting_gold * .balance.difficulties.normal.gold | round)' <<<"${config_json}")" \
  --argjson duration_ms "${invalid_duration_ms}" \
  '{game_id:"realmguard",session_id:$session_id,session_token:$session_token,stage_id:"stage-1",mode:"campaign",difficulty:"normal",duration_ms:$duration_ms,remaining_lives:20,remaining_gold:$initial_gold,earned_gold:0,spent_gold:0,sold_gold:0,kills:0,escaped:0,spawned:0,waves_completed:0,hero_id:"aerin",hero_level:1,content_version:$content_version,stage_version:$stage_version,balance_version:$balance_version,asset_version:$asset_version,victory:false}')"
request POST /api/v1/realmguard/results 422 "${invalid_defeat_body}" \
  | jq --exit-status '.error.code == "missing_ledger"' >/dev/null

# A ledger the server cannot reproduce is refused before any battle numbers are
# considered: a forged content digest means the browser and the server were not
# playing the same rules.
forged_ledger_body="$(jq --compact-output --argjson body "${invalid_defeat_body}" --null-input \
  '$body + {ledger:{rules_version:"realmguard-kernel-1",config_digest:"0000000000000000",skill_ids:["meteor"],ticks:120,commands:[]}}')"
request POST /api/v1/realmguard/results 409 "${forged_ledger_body}" \
  | jq --exit-status '.error.code == "content_projection_mismatch"' >/dev/null
stale_rules_body="$(jq --compact-output --argjson body "${invalid_defeat_body}" --null-input \
  '$body + {ledger:{rules_version:"realmguard-kernel-0",config_digest:"0000000000000000",skill_ids:["meteor"],ticks:120,commands:[]}}')"
request POST /api/v1/realmguard/results 409 "${stale_rules_body}" \
  | jq --exit-status '.error.code == "ledger_rules_mismatch"' >/dev/null

# Optional RealmGuard events share a 128-event budget, while required battle
# milestone classes keep independent capacity. Fill only the optional class,
# prove another optional event is rejected, then prove the reserved ready slot
# remains usable (and is itself limited to one).
limit_session_body="$(jq --null-input --compact-output --arg version_id "${realmguard_version_id}" '{metadata:{client:"release-smoke",purpose:"telemetry-limit",realmguard_version_id:$version_id}}')"
limit_session="$(request POST /api/v1/games/realmguard/sessions 201 "${limit_session_body}")"
jq --exit-status --arg version_id "${realmguard_version_id}" '.session.realmguard_version_id == $version_id' <<<"${limit_session}" >/dev/null
limit_session_id="$(jq --raw-output '.session.id' <<<"${limit_session}")"
limit_session_token="$(jq --raw-output '.session.session_token' <<<"${limit_session}")"
for ((sequence = 1; sequence <= 128; sequence++)); do
  post_telemetry "${limit_session_id}" "${limit_session_token}" game.pause "${sequence}" '{}' >/dev/null
done
post_telemetry "${limit_session_id}" "${limit_session_token}" game.pause 129 '{}' 429 \
  | jq --exit-status '.error.code == "telemetry_limit"' >/dev/null
limit_ready_data='{"stage_id":"stage-1","difficulty":"normal","hero_id":"aerin"}'
post_telemetry "${limit_session_id}" "${limit_session_token}" realmguard.battle.ready 129 "${limit_ready_data}" \
  | jq --exit-status '.accepted == true and .duplicate == false and .sequence == 129' >/dev/null
post_telemetry "${limit_session_id}" "${limit_session_token}" realmguard.battle.ready 130 "${limit_ready_data}" 429 \
  | jq --exit-status '.error.code == "telemetry_limit"' >/dev/null

# The server replays this battle from the player's recorded inputs, so the smoke
# posts a committed kernel-generated ledger with the telemetry that battle
# streamed. Fabricated numbers cannot satisfy a replay, so the fixture is
# produced by the same kernel the browser runs.
smoke_fixture_path="${REPO_DIR}/scripts/testdata/realmguard-smoke.json"
if [[ ! -r "${smoke_fixture_path}" ]]; then
  printf 'Missing RealmGuard smoke battle fixture: %s\n' "${smoke_fixture_path}" >&2
  exit 1
fi
fixture_json="$(cat "${smoke_fixture_path}")"
jq --exit-status \
  --arg content_version "$(jq --raw-output '.version.content_version' <<<"${config_json}")" \
  --arg stage_version "$(jq --raw-output '[.stages[] | select(.id == "stage-1")][0].version' <<<"${config_json}")" \
  --arg balance_version "$(jq --raw-output '.version.balance_version' <<<"${config_json}")" \
  --arg asset_version "$(jq --raw-output '.version.asset_version' <<<"${config_json}")" \
  '.content_version == $content_version and .stage_version == $stage_version
   and .balance_version == $balance_version and .asset_version == $asset_version' \
  <<<"${fixture_json}" >/dev/null || {
  printf '%s\n' 'The committed RealmGuard smoke battle was recorded against different published content. Regenerate it with UPDATE_KERNEL_VECTORS=1 npx vitest run src/games/realmguard/kernel.' >&2
  exit 1
}

# Opening a session abandons the previous active one for the same game, so the
# battle session is the last RealmGuard session this smoke opens. A replayed
# battle only verifies if the session had the wall time to play it, and this
# smoke waits that out rather than pretending the battle was instant.
session_body="$(jq --null-input --compact-output --arg version_id "${realmguard_version_id}" --arg client_version "${EXPECTED_VERSION}" '{metadata:{client:"release-smoke",client_version:$client_version,scenario:"replayed-stage-1-defeat",realmguard_version_id:$version_id}}')"
session_json="$(request POST /api/v1/games/realmguard/sessions 201 "${session_body}")"
jq --exit-status --arg version_id "${realmguard_version_id}" '.session.realmguard_version_id == $version_id' <<<"${session_json}" >/dev/null
session_id="$(jq --raw-output '.session.id' <<<"${session_json}")"
session_token="$(jq --raw-output '.session.session_token' <<<"${session_json}")"
battle_started_ms="$(date +%s%3N)"

ready_data="$(jq --compact-output '.telemetry[0].data' <<<"${fixture_json}")"
ready_event_id="$(new_uuid)"
ready_payload="$(telemetry_payload "${session_id}" "${session_token}" realmguard.battle.ready 1 "${ready_event_id}" "${ready_data}")"
request POST /api/v1/telemetry 202 "${ready_payload}" \
  | jq --exit-status '.accepted == true and .duplicate == false and .sequence == 1' >/dev/null
request POST /api/v1/telemetry 202 "${ready_payload}" \
  | jq --exit-status '.accepted == true and .duplicate == true and .sequence == 1' >/dev/null

# A duplicate client_event_id is idempotent, but a gap in a new event's
# session-local sequence is rejected. Oversized RealmGuard event data is also
# rejected without consuming the expected next sequence.
oversized_data="$(jq --null-input --compact-output '{payload:("x" * 4097)}')"
post_telemetry "${session_id}" "${session_token}" game.pause 2 "${oversized_data}" 400 \
  | jq --exit-status '.error.code == "invalid_telemetry"' >/dev/null
post_telemetry "${session_id}" "${session_token}" game.pause 3 '{}' 409 \
  | jq --exit-status '.error.code == "telemetry_sequence_conflict"' >/dev/null


# Replay the recorded battle into the open session.
battle_duration_ms="$(jq '.outcome.duration_ms' <<<"${fixture_json}")"
telemetry_count="$(jq '.telemetry | length' <<<"${fixture_json}")"
min_wave_duration_ms="$(jq '(.balance.min_wave_duration_ms | ceil)' <<<"${config_json}")"
duration_tolerance_ms="$(jq '(.balance.duration_tolerance_ms | ceil)' <<<"${config_json}")"
minimum_milestone_ms="$((min_wave_duration_ms / 5))"
if ((minimum_milestone_ms < 250)); then minimum_milestone_ms=250; fi
if ((minimum_milestone_ms > 1000)); then minimum_milestone_ms=1000; fi
# The server measures the gap between wave.start and wave.complete as it
# received them, so this wait has to actually pass. `sleep` alone does not:
# a busy container host hands back short sleeps, and a run of this smoke put
# 711ms between a pair that needed 1000. Wait by the clock instead.
milestone_wait_ms="$((minimum_milestone_ms + 400))"
wait_at_least_ms() {
  local required="$1" start elapsed
  start="$(date +%s%3N)"
  while :; do
    elapsed="$(( $(date +%s%3N) - start ))"
    ((elapsed >= required)) && break
    sleep "$(awk -v milliseconds="$((required - elapsed))" 'BEGIN { printf "%.3f", milliseconds / 1000 }')"
  done
}

# Every milestone but the opening ready and the closing complete, in the order
# the battle produced them. A wave start is followed by a real pause so its
# completion cannot arrive faster than the server allows.
sequence=2
for ((index = 1; index < telemetry_count - 1; index++)); do
  milestone_event="$(jq --raw-output --argjson index "${index}" '.telemetry[$index].event' <<<"${fixture_json}")"
  milestone_data="$(jq --compact-output --argjson index "${index}" '.telemetry[$index].data' <<<"${fixture_json}")"
  post_telemetry "${session_id}" "${session_token}" "${milestone_event}" "${sequence}" "${milestone_data}" >/dev/null
  sequence=$((sequence + 1))
  if [[ "${milestone_event}" == "realmguard.wave.start" ]]; then
    wait_at_least_ms "${milestone_wait_ms}"
  fi
done

# A replayed battle is only accepted if the session had the wall time to play
# it. Most of that has already passed while the rest of this smoke ran; wait out
# whatever remains instead of assuming it.
required_elapsed_ms="$((battle_duration_ms / 2 - duration_tolerance_ms + 20000))"
announced_wait=false
# Re-measure after every sleep: a container host can hand back a short sleep,
# and arriving one second early would fail the run for no real reason.
while :; do
  elapsed_ms="$(( $(date +%s%3N) - battle_started_ms ))"
  ((elapsed_ms >= required_elapsed_ms)) && break
  remaining_ms="$((required_elapsed_ms - elapsed_ms))"
  if [[ "${announced_wait}" == false ]]; then
    announced_wait=true
    printf 'Replayed battle is %dms of simulation; waiting %ds so the session has honest wall time.\n' \
      "${battle_duration_ms}" "$(((remaining_ms + 999) / 1000))"
  fi
  sleep "$(awk -v milliseconds="${remaining_ms}" 'BEGIN { printf "%.3f", milliseconds / 1000 }')"
done

battle_complete_data="$(jq --compact-output '.telemetry[-1].data' <<<"${fixture_json}")"
post_telemetry "${session_id}" "${session_token}" realmguard.battle.complete "${sequence}" "${battle_complete_data}" >/dev/null

# The request still carries the legacy battle numbers; the server replaces every
# one of them with what its own replay produced.
result_body="$(jq --compact-output \
  --arg session_id "${session_id}" --arg session_token "${session_token}" \
  '{game_id:"realmguard",session_id:$session_id,session_token:$session_token,
    stage_id:.stage_id,mode:.mode,difficulty:.difficulty,hero_id:.hero_id,
    content_version:.content_version,stage_version:.stage_version,
    balance_version:.balance_version,asset_version:.asset_version,ledger:.ledger,
    duration_ms:.outcome.duration_ms,remaining_lives:.outcome.lives,remaining_gold:.outcome.gold,
    earned_gold:.outcome.earned_gold,spent_gold:.outcome.spent_gold,sold_gold:.outcome.sold_gold,
    kills:.outcome.kills,escaped:.outcome.escaped,spawned:.outcome.spawned,
    waves_completed:.outcome.waves_completed,hero_level:.outcome.hero_level,victory:.outcome.victory,
    defeated_by_enemy:.outcome.defeated_by_enemy,escaped_by_enemy:.outcome.escaped_by_enemy,
    spawned_by_enemy:.outcome.spawned_by_enemy,
    proof:"untrusted-client-proof",events:[{source:"client",trusted:false}]}' <<<"${fixture_json}")"
result_json="$(request POST /api/v1/realmguard/results 201 "${result_body}")"
jq --exit-status \
  --argjson expected "$(jq --compact-output '.outcome' <<<"${fixture_json}")" \
  --argjson events "${telemetry_count}" \
  '.result.verified == true
   and .result.victory == false
   and .result.stars == 0
   and (.result.score | type) == "number"
   and .result.battle_hero_level == $expected.hero_level
   and .result.verification_method == "server_replay_v1"
   and .result.attestation.method == "server_replay_v1"
   and .result.attestation.replay.rules_version == "realmguard-kernel-1"
   and .result.attestation.replay.ticks == $expected.ticks
   and .result.attestation.replay.commands == 3
   and (.result.attestation.replay.config_digest | test("^[0-9a-f]{16}$"))
   and .result.attestation.telemetry.method == "server_received_telemetry_v1"
   and .result.attestation.telemetry.event_count == $events
   and .result.attestation.telemetry.waves_started == ($expected.waves_completed + 1)
   and .result.attestation.telemetry.waves_completed == $expected.waves_completed
   and (.result.attestation.telemetry.digest | test("^[0-9a-f]{64}$"))
   and .progress.total_stars == 0
' <<<"${result_json}" >/dev/null
request POST /api/v1/realmguard/results 200 "${result_body}" \
  | jq --exit-status '.idempotent == true and .result.verified == true and .result.verification_method == "server_replay_v1"' >/dev/null

# The same ledger with one tower command removed is a different battle, so the
# server must refuse to close an already finished session with it.
result_id="$(jq --raw-output '.result.id' <<<"${result_json}")"
request GET '/api/v1/admin/audit?limit=200' 200 \
  | jq --exit-status --arg id "${result_id}" '([.items[] | select(.action == "realmguard.result.accept" and .resource_id == $id and .detail.client_evidence_ignored == true and .detail.verification_method == "server_replay_v1" and .detail.ledger_commands == 3)] | length) == 1' >/dev/null

request GET '/api/v1/realmguard/rankings?group=individual&metric=score&period=daily&mode=campaign&difficulty=veteran&stage_id=stage-1&hero_id=aerin' 200 \
  | jq --exit-status '.group == "individual" and .metric == "score" and .period == "daily" and (.items | length) == 0' >/dev/null
request GET /api/v1/realmguard/progress 200 \
  | jq --exit-status '([.items[] | select(.stage_id == "stage-1" and .difficulty == "veteran" and .attempts == 1 and .completed == false)] | length) == 1' >/dev/null


# Exercise the configurable Designer review path without changing the active
# published snapshot. Separation is disabled only for this single-admin smoke.
approval_restore_body="$(request GET /api/v1/admin/settings/approval 200 | jq --compact-output '{value:.value}')"
approval_changed=true
request PUT /api/v1/admin/settings/approval 200 '{"value":{"enabled":true,"manager_required":false,"separation_of_duties":false}}' >/dev/null

# RealmGuard owns a reserved authoritative slug. Neither the direct game
# update handler nor a workflow that reaches its apply phase may transition a
# normal game into that identity. Reject the failed request afterward so this
# fresh-DB smoke does not leave a pending review behind.
protected_game="$(request GET /api/v1/games/snake 200)"
protected_game_id="$(jq --raw-output '.game.id' <<<"${protected_game}")"
protected_game_payload="$(jq --compact-output '
  .game
  | {slug,name,description,category_id,tags,thumbnail_url,banner_url,game_url,
     game_type,multiplayer,ranking_enabled,achievement_enabled,season_enabled,
     min_players,max_players,status,version,developer,score_order,score_rules}
  | .slug = "realmguard"
' <<<"${protected_game}")"
request PUT "/api/v1/admin/games/${protected_game_id}" 409 "${protected_game_payload}" \
  | jq --exit-status '.error.code == "protected_game_identity"' >/dev/null
protected_workflow_body="$(jq --null-input --compact-output \
  --arg resource_id "${protected_game_id}" --argjson payload "${protected_game_payload}" \
  '{action:"update",resource_type:"game",resource_id:$resource_id,payload:$payload}')"
request POST /api/v1/workflow/requests 409 "${protected_workflow_body}" \
  | jq --exit-status '.error.code == "protected_game_identity" and (.error.message | contains("authoritative"))' >/dev/null
request GET /api/v1/games/snake 200 \
  | jq --exit-status '.game.slug == "snake"' >/dev/null

reject_version="$(request POST /api/v1/admin/realmguard/versions 201 '{"label":"release-smoke-reject","notes":"release smoke rejection"}')"
reject_id="$(jq --raw-output '.version.id' <<<"${reject_version}")"
reject_section="$(request GET "/api/v1/admin/realmguard/drafts/stages?version_id=${reject_id}" 200)"
jq --exit-status --arg id "${reject_id}" '.version.id == $id and .section == "stages" and (.data | length) >= 11 and (.version.checksum | test("^[0-9a-f]{64}$"))' <<<"${reject_section}" >/dev/null
reject_checksum="$(jq --raw-output '.version.checksum' <<<"${reject_section}")"
reject_section_body="$(jq --compact-output '{data:.data}' <<<"${reject_section}")"
request PUT "/api/v1/admin/realmguard/drafts/stages?version_id=${reject_id}" 428 "${reject_section_body}" \
  | jq --exit-status '.error.code == "precondition_required"' >/dev/null
request PUT "/api/v1/admin/realmguard/drafts/stages?version_id=${reject_id}" 409 "${reject_section_body}" \
  'If-Match: "0000000000000000000000000000000000000000000000000000000000000000"' \
  | jq --exit-status '.error.code == "stale_version"' >/dev/null
reject_section="$(request PUT "/api/v1/admin/realmguard/drafts/stages?version_id=${reject_id}" 200 "${reject_section_body}" "If-Match: \"${reject_checksum}\"")"
jq --exit-status --arg id "${reject_id}" '.version.id == $id and .version.status == "draft" and (.version.checksum | test("^[0-9a-f]{64}$"))' <<<"${reject_section}" >/dev/null
request POST "/api/v1/admin/realmguard/versions/${reject_id}/test" 200 \
  | jq --exit-status '.version.status == "testing" and .validation.campaign_stages >= 10 and .validation.endless_stages >= 1' >/dev/null
request GET "/api/v1/realmguard/versions/${reject_id}/preview" 200 \
  | jq --exit-status --arg id "${reject_id}" '.practice_only == true and .version.id == $id and (.stages | length) >= 11' >/dev/null
request POST "/api/v1/admin/realmguard/versions/${reject_id}/publish" 202 '{}' \
  | jq --exit-status '.approval_required == true and .published == false and .version.status == "pending_approval"' >/dev/null
request GET /api/v1/realmguard/versions/pending 200 \
  | jq --exit-status --arg id "${reject_id}" '([.items[] | select(.id == $id and .status == "pending_approval" and (.changed_sections | type) == "array")] | length) == 1' >/dev/null
request POST "/api/v1/realmguard/versions/${reject_id}/review" 200 '{"decision":"rejected","comment":"release smoke rejection"}' \
  | jq --exit-status '.decision == "rejected" and .rejected == true and .version.status == "draft" and .version.review_comment == "release smoke rejection"' >/dev/null

approve_version="$(request POST /api/v1/admin/realmguard/versions 201 '{"label":"release-smoke-approve","notes":"release smoke approval"}')"
approve_id="$(jq --raw-output '.version.id' <<<"${approve_version}")"
request POST "/api/v1/admin/realmguard/versions/${approve_id}/test" 200 \
  | jq --exit-status '.version.status == "testing" and .validation.base_towers >= 4 and .validation.advanced_towers >= 8' >/dev/null
request POST "/api/v1/admin/realmguard/versions/${approve_id}/publish" 202 '{}' \
  | jq --exit-status '.version.status == "pending_approval"' >/dev/null
request POST "/api/v1/realmguard/versions/${approve_id}/review" 200 '{"decision":"approved","comment":"release smoke approval"}' \
  | jq --exit-status '.decision == "approved" and .approved == true and .version.status == "approved"' >/dev/null
request PUT /api/v1/admin/settings/approval 200 "${approval_restore_body}" >/dev/null
approval_changed=false

request GET '/api/v1/admin/realmguard/telemetry?days=30' 200 \
  | jq --exit-status '.summary.runs >= 1 and .summary.rejected_results >= 1 and (.stages | type) == "array" and (.heroes | type) == "array" and (.difficulties | type) == "array"' >/dev/null

printf 'RealmGuard release smoke passed: %s (igame v%s)\n' "${BASE_URL}" "${EXPECTED_VERSION}"

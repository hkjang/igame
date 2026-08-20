#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly VERSION="$(tr -d '[:space:]' < "${REPO_DIR}/VERSION")"
readonly COMPOSE_FILE="${REPO_DIR}/docker-compose.yml"
readonly ENV_EXAMPLE="${REPO_DIR}/.env.example"
readonly WEB_PACKAGE="${REPO_DIR}/web/package.json"
readonly WEB_LOCK="${REPO_DIR}/web/package-lock.json"
readonly SDK_PACKAGE="${REPO_DIR}/sdk/gamehub-js/package.json"
readonly SDK_LOCK="${REPO_DIR}/sdk/gamehub-js/package-lock.json"

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

if ! grep -Eq '"phaser"[[:space:]]*:' "${WEB_PACKAGE}" || ! grep -Fq '"node_modules/phaser"' "${WEB_LOCK}"; then
  printf '%s\n' 'RealmGuard requires Phaser as a locked production dependency.' >&2
  exit 1
fi

for package_file in "${WEB_PACKAGE}" "${SDK_PACKAGE}"; do
  if ! grep -Fq "\"version\": \"${VERSION}\"" "${package_file}"; then
    printf 'Package version must match VERSION %s: %s\n' "${VERSION}" "${package_file}" >&2
    exit 1
  fi
done
for lock_file in "${WEB_LOCK}" "${SDK_LOCK}"; do
  if ! sed -n '1,12p' "${lock_file}" | grep -Fq "\"version\": \"${VERSION}\""; then
    printf 'Package lock root version must match VERSION %s: %s\n' "${VERSION}" "${lock_file}" >&2
    exit 1
  fi
done

if ! grep -Fq '/src/web/package-lock.json /licenses/web/package-lock.json' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq '/src/sdk/gamehub-js/package-lock.json /licenses/sdk/gamehub-js/package-lock.json' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq '/src/web/node_modules/phaser/package.json /licenses/phaser/package.json' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq '/src/web/node_modules/phaser/LICENSE.md /licenses/phaser/LICENSE.md' "${REPO_DIR}/Dockerfile"; then
  printf '%s\n' 'The runtime image must retain locked web metadata plus Phaser package and license metadata for offline review and SBOM discovery.' >&2
  exit 1
fi

if ! grep -Fq 'check-offline-bundle.sh /src' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq 'bash ./scripts/check-offline-bundle.sh' "${REPO_DIR}/Makefile"; then
  printf '%s\n' 'Local and container builds must enforce the RealmGuard offline bundle check.' >&2
  exit 1
fi

if ! grep -Fq '.name | ascii_downcase) == "phaser"' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq '.name == "@igame/gamehub-js"' "${REPO_DIR}/.github/workflows/release.yml"; then
  printf '%s\n' 'Release CI must prove that the SPDX SBOM identifies the bundled Phaser and gamehub-js packages.' >&2
  exit 1
fi

if ! grep -Fq '//go:embed *.sql' "${REPO_DIR}/migrations/migrations.go"; then
  printf '%s\n' 'SQL migrations must remain embedded in the igame binary.' >&2
  exit 1
fi

if [[ ! -f "${REPO_DIR}/migrations/003_realmguard.sql" ]] \
  || ! grep -Fq "'realmguard','RealmGuard'" "${REPO_DIR}/migrations/003_realmguard.sql" \
  || ! grep -Fq "'/games/realmguard'" "${REPO_DIR}/migrations/003_realmguard.sql"; then
  printf '%s\n' 'The embedded RealmGuard migration and canonical /games/realmguard seed are required.' >&2
  exit 1
fi
if [[ ! -f "${REPO_DIR}/migrations/004_realmguard_attestation.sql" ]] \
  || ! grep -Fq 'ADD COLUMN verification_method' "${REPO_DIR}/migrations/004_realmguard_attestation.sql" \
  || ! grep -Fq 'ADD COLUMN attestation' "${REPO_DIR}/migrations/004_realmguard_attestation.sql" \
  || ! grep -Fq 'ADD COLUMN client_event_id uuid' "${REPO_DIR}/migrations/004_realmguard_attestation.sql" \
  || ! grep -Fq 'ADD COLUMN sequence_no integer' "${REPO_DIR}/migrations/004_realmguard_attestation.sql" \
  || ! grep -Fq '["profile:write"]' "${REPO_DIR}/migrations/004_realmguard_attestation.sql"; then
  printf '%s\n' 'The embedded RealmGuard server-received telemetry attestation migration is required.' >&2
  exit 1
fi

for economy_column in remaining_gold earned_gold spent_gold sold_gold; do
  if ! grep -Eq "^[[:space:]]+${economy_column}[[:space:]]+bigint" "${REPO_DIR}/migrations/003_realmguard.sql" \
    || ! grep -Fq "ALTER COLUMN ${economy_column} TYPE bigint" "${REPO_DIR}/migrations/004_realmguard_attestation.sql"; then
    printf 'RealmGuard economy column must be bigint for fresh and upgraded databases: %s\n' "${economy_column}" >&2
    exit 1
  fi
done

for route_contract in \
  'a.Get("/api/v1/realmguard/config"' \
  'a.Get("/api/v1/realmguard/version"' \
  'a.Get("/api/v1/realmguard/progress"' \
  'a.Put("/api/v1/realmguard/progress"' \
  'a.Post("/api/v1/realmguard/results"' \
  'a.Get("/api/v1/realmguard/rankings"' \
  'Post("/api/v1/realmguard/versions/{id}/review"' \
  'Get("/api/v1/realmguard/versions/{id}/preview"' \
  'admin.Get("/realmguard/drafts/{section}"' \
  'admin.Post("/realmguard/versions/{id}/test"' \
  'admin.Post("/realmguard/versions/{id}/approve"' \
  'admin.Post("/realmguard/versions/{id}/review"' \
  'admin.Post("/realmguard/versions/{id}/publish"' \
  'admin.Get("/realmguard/telemetry"'; do
  if ! grep -Fq "${route_contract}" "${REPO_DIR}/internal/api/api.go"; then
    printf 'Missing RealmGuard route contract: %s\n' "${route_contract}" >&2
    exit 1
  fi
done

if ! grep -Fq 'authoritative_result_required' "${REPO_DIR}/internal/api/catalog.go" \
  || ! grep -Fq 'completeAuthoritatively' "${REPO_DIR}/sdk/gamehub-js/src/index.ts" \
  || ! grep -Fq 'in.Decision == "rejected"' "${REPO_DIR}/internal/api/realmguard_admin.go"; then
  printf '%s\n' 'RealmGuard requires authoritative completion and approve/reject review contracts.' >&2
  exit 1
fi

for telemetry_contract in \
  'len(in.Data) > 4<<10' \
  '"telemetry_event_conflict"' \
  '"telemetry_sequence_conflict"' \
  '"realmguard_version_required"' \
  '"realmguard_config_stale"' \
  '"realmguard_ranking_required"'; do
  if ! grep -Fq "${telemetry_contract}" "${REPO_DIR}/internal/api/catalog.go"; then
    printf 'Missing RealmGuard telemetry/ranking contract: %s\n' "${telemetry_contract}" >&2
    exit 1
  fi
done

if ! grep -Eq 'realmGuardOptionalTelemetryLimit[[:space:]]*=[[:space:]]*128' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Eq 'realmGuardTowerLedgerLimit[[:space:]]*=[[:space:]]*10000' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Eq 'realmGuardMaxEndlessWaves[[:space:]]*=[[:space:]]*10000' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'realmGuardTelemetryLimitReached(in.Event, eventCounts)' "${REPO_DIR}/internal/api/catalog.go"; then
  printf '%s\n' 'RealmGuard telemetry must reserve class-specific capacity for required battle milestones.' >&2
  exit 1
fi

if ! grep -Fq 'len(value) > 32' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'len(content.Enemies) > 16 || len(content.Bosses) > 4' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'len(wave.Entries) > 8' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'waveSpawnCount > 500' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'realmGuardTelemetryPayloadFits(enemyIDs)' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'realmGuardWaveCapacity(content, stage.ID, realmGuardMaxEndlessWaves, false)' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'int64(budget.BaseSpawns) > math.MaxInt32 || int64(budget.MaxSpawns) > math.MaxInt32' "${REPO_DIR}/internal/api/realmguard.go"; then
  printf '%s\n' 'RealmGuard content IDs, roster and wave cardinality, 4 KiB payload, and endless counter budgets must remain bounded.' >&2
  exit 1
fi

if ! grep -Fq '(currentSlug == realmGuardSlug) != (in.Slug == realmGuardSlug)' "${REPO_DIR}/internal/api/admin.go" \
  || ! grep -Fq '(currentSlug == realmGuardSlug) != (game.Slug == realmGuardSlug)' "${REPO_DIR}/internal/api/content.go"; then
  printf '%s\n' 'Direct admin updates and workflow apply updates must protect the built-in RealmGuard slug.' >&2
  exit 1
fi

if ! grep -Fq 'const realmGuardVerificationMethod = "server_received_telemetry_v1"' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'record.Sequence != index+1' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'realmguard.battle.ready' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'realmguard.wave.complete' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'realmguard.battle.complete' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'DefeatedByEnemy' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'SpawnedByEnemy' "${REPO_DIR}/internal/api/realmguard_attestation.go" \
  || ! grep -Fq 'client_evidence_ignored' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq '"server:"+serverProof' "${REPO_DIR}/internal/api/realmguard.go"; then
  printf '%s\n' 'RealmGuard results require ordered server-received milestones, cumulative enemy histograms, and a server-minted receipt.' >&2
  exit 1
fi

if ! grep -Fq 'required = "profile:write"' "${REPO_DIR}/internal/api/api.go" \
  || ! grep -Fq 'path == "/api/v1/me/achievements"' "${REPO_DIR}/internal/api/api.go" \
  || ! grep -Fq 'required = "scores:write"' "${REPO_DIR}/internal/api/api.go"; then
  printf '%s\n' 'Personal API keys must split profile reads, profile mutations, and achievement writes by scope.' >&2
  exit 1
fi

if ! grep -Fq 'RealmGuard requires realmguard_version_id from its published config' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq '"realmguard_version_id": map[string]any' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq 'metadata["realmguard_version_id"] = versionID' "${REPO_DIR}/internal/api/mcp.go"; then
  printf '%s\n' 'MCP game_session_start must forward the optional published RealmGuard version pin.' >&2
  exit 1
fi

if ! grep -Fq 'http.StatusPreconditionRequired' "${REPO_DIR}/internal/api/realmguard_admin.go" \
  || ! grep -Fq '"stale_version"' "${REPO_DIR}/internal/api/realmguard_admin.go" \
  || ! grep -Fq 'managerTeam == "" || creatorTeam == ""' "${REPO_DIR}/internal/api/realmguard_admin.go" \
  || ! grep -Fq "creator.team<>'' AND creator.team=\$2" "${REPO_DIR}/internal/api/realmguard_admin.go"; then
  printf '%s\n' 'RealmGuard Designer requires optimistic concurrency and fail-closed same-team manager review.' >&2
  exit 1
fi

if ! grep -Fq 'writeError(w, 403, "team_required", "managers require a team assignment to view review requests")' "${REPO_DIR}/internal/api/content.go" \
  || ! grep -Fq "u.team=\$2 AND u.team<>''" "${REPO_DIR}/internal/api/content.go" \
  || ! grep -Fq 'managerTeam == "" || creatorTeam == ""' "${REPO_DIR}/internal/api/content.go" \
  || ! grep -Fq 'managerTeam != creatorTeam' "${REPO_DIR}/internal/api/content.go"; then
  printf '%s\n' 'General workflow manager listing and review must fail closed on missing or mismatched teams.' >&2
  exit 1
fi

if ! grep -Fq 'scripts/smoke-realmguard.sh' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'mcr.microsoft.com/playwright:v1.55.0-noble' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'node /work/scripts/browser-smoke.mjs' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'IGAME_REQUIRE_DESIGNER_DRAFT=true' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq -- '--network host' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq '${GITHUB_WORKSPACE}:/work:ro' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'stages.length < 11' "${REPO_DIR}/scripts/browser-smoke.mjs" \
  || ! grep -Fq 'RealmGuard Designer rendered an invalid or empty editor state' "${REPO_DIR}/scripts/browser-smoke.mjs"; then
  printf '%s\n' 'Release CI must API-smoke RealmGuard and hard-fail on the pinned loaded-image Playwright browser gate.' >&2
  exit 1
fi

if ! grep -Fq '.balance.min_wave_duration_ms' "${REPO_DIR}/scripts/smoke-realmguard.sh" \
  || grep -Fq 'duration_ms:2000' "${REPO_DIR}/scripts/smoke-realmguard.sh"; then
  printf '%s\n' 'RealmGuard result smoke duration must follow the published minimum-wave balance and real session wall time.' >&2
  exit 1
fi

for smoke_contract in \
  'realmguard_version_id' \
  'realmguard_version_required' \
  'realmguard_config_stale' \
  'realmguard_ranking_required' \
  'telemetry_sequence_conflict' \
  'telemetry_limit' \
  'sequence <= 128' \
  'realmguard.battle.ready 129' \
  '"x" * 4097' \
  'realmguard.battle.ready' \
  'realmguard.wave.complete' \
  'realmguard.battle.complete' \
  'defeated_by_enemy' \
  'server_received_telemetry_v1' \
  'client_evidence_ignored' \
  'protected_game_identity' \
  'RealmGuard slug is reserved' \
  'precondition_required' \
  'stale_version'; do
  if ! grep -Fq "${smoke_contract}" "${REPO_DIR}/scripts/smoke-realmguard.sh"; then
    printf 'RealmGuard release smoke is missing contract coverage: %s\n' "${smoke_contract}" >&2
    exit 1
  fi
done

printf 'Release contract verified for igame:v%s\n' "${VERSION}"

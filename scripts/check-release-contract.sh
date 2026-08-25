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

if ! grep -Fqx 'go 1.26.6' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'github.com/coreos/go-oidc/v3 v3.20.0' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'github.com/go-chi/chi/v5 v5.3.2' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'github.com/jackc/pgx/v5 v5.10.0' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'golang.org/x/crypto v0.55.0' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'golang.org/x/oauth2 v0.36.0' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'github.com/go-jose/go-jose/v4 v4.1.4' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'golang.org/x/sync v0.22.0' "${REPO_DIR}/go.mod" \
  || ! grep -Fq 'golang.org/x/text v0.41.0' "${REPO_DIR}/go.mod"; then
  printf '%s\n' 'Go toolchain and security-sensitive module versions must match the audited v0.3.0 pins.' >&2
  exit 1
fi

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

if ! grep -Fqx 'USER 10001:10001' "${REPO_DIR}/Dockerfile" \
  || ! grep -Eq '^[[:space:]]{4}read_only:[[:space:]]+true[[:space:]]*$' "${COMPOSE_FILE}" \
  || ! grep -Eq '^[[:space:]]+-[[:space:]]+ALL[[:space:]]*$' "${COMPOSE_FILE}" \
  || ! grep -Eq '^[[:space:]]+-[[:space:]]+no-new-privileges:true[[:space:]]*$' "${COMPOSE_FILE}"; then
  printf '%s\n' 'The offline runtime must remain non-root, read-only, capability-free, and no-new-privileges.' >&2
  exit 1
fi

if ! grep -Fq 'FROM golang:1.26.6-alpine3.23 AS go-build' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fqx 'FROM scratch AS runtime' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq 'io.igame.build.go-version="1.26.6"' "${REPO_DIR}/Dockerfile"; then
  printf '%s\n' 'The release builder and OCI build metadata must pin Go 1.26.6 exactly.' >&2
  exit 1
fi

runtime_stage="$(sed -n '/^FROM scratch AS runtime$/,$p' "${REPO_DIR}/Dockerfile")"
if grep -Eq '(^|[[:space:]])(apk|apt-get|wget|curl)([[:space:]]|$)|/bin/(ba)?sh' <<<"${runtime_stage}"; then
  printf '%s\n' 'The package-free scratch runtime must not install or invoke a package manager, shell, or HTTP utility.' >&2
  exit 1
fi

if ! grep -Fq 'COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq 'mkdir -p /out/app/data' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq 'COPY --from=go-build --chown=10001:10001 /out/app /app' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq 'CMD ["/app/igame", "healthcheck"]' "${REPO_DIR}/Dockerfile" \
  || ! grep -Fq 'test: ["CMD", "/app/igame", "healthcheck"]' "${COMPOSE_FILE}" \
  || ! grep -Fq 'len(os.Args) == 2 && os.Args[1] == "healthcheck"' "${REPO_DIR}/cmd/igame/main.go"; then
  printf '%s\n' 'The scratch runtime must retain CA trust, a non-root data directory, and the shell-free binary healthcheck.' >&2
  exit 1
fi

for workflow in "${REPO_DIR}/.github/workflows/ci.yml" "${REPO_DIR}/.github/workflows/release.yml"; do
  action_refs="$(awk '$1 == "uses:" { print $2 }' "${workflow}")"
  unpinned_refs="$(grep -Ev '^[^@[:space:]]+@[0-9a-f]{40}$' <<<"${action_refs}" || true)"
  if [[ -n "${unpinned_refs}" ]]; then
    printf 'Every external GitHub Action must use an immutable full commit SHA in %s; found:\n%s\n' "${workflow}" "${unpinned_refs}" >&2
    exit 1
  fi
  if ! grep -Fq 'govulncheck@v1.6.0' "${workflow}" \
    || ! grep -Fq 'npm --prefix sdk/gamehub-js audit --audit-level=low' "${workflow}" \
    || ! grep -Fq 'npm --prefix web audit --audit-level=low' "${workflow}" \
    || ! grep -Fq "'{{.Config.User}}'" "${workflow}" \
    || ! grep -Fq '"10001:10001"' "${workflow}" \
    || ! grep -Fq 'go version -m "${binary_dir}/igame"' "${workflow}" \
    || ! grep -Fq "grep -Fq 'go1.26.6'" "${workflow}" \
    || ! grep -Fq 'io.igame.build.go-version' "${workflow}" \
    || ! grep -Fq 'anchore/scan-action@e1165082ffb1fe366ebaf02d8526e7c4989ea9d2 # v7.4.0' "${workflow}" \
    || ! grep -Fq 'severity-cutoff: high' "${workflow}" \
    || ! grep -Fq 'grype-version: v0.117.0' "${workflow}" \
    || ! grep -Fq 'fail-build: true' "${workflow}" \
    || ! grep -Fq 'docker cp "${container_id}:/licenses/." "${binary_dir}/licenses"' "${workflow}" \
    || grep -Fq -- '--entrypoint /bin/sh' "${workflow}"; then
    printf 'Workflow must audit dependencies, scan the final image, and inspect the shell-free non-root Go 1.26.6 image: %s\n' "${workflow}" >&2
    exit 1
  fi
done

if ! grep -Fq 'docker save "${IMAGE}" | gzip' "${REPO_DIR}/scripts/release.sh"; then
  printf '%s\n' 'release.sh must gzip the docker save stream directly.' >&2
  exit 1
fi

if ! grep -Fq 'END { if (matches != 1) exit 1 }' "${REPO_DIR}/scripts/verify-release.sh" \
  || ! grep -Fq 'sha256sum --check --strict -' "${REPO_DIR}/scripts/verify-release.sh"; then
  printf '%s\n' 'Release verification must select exactly one checksum entry for the requested archive.' >&2
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

if ! grep -Fq '"esbuild": "0.27.2"' "${SDK_PACKAGE}"; then
  printf '%s\n' 'The SDK must pin the audited pre-advisory esbuild 0.27.2 through a root override.' >&2
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

if ! grep -Fq 'isProtectedAuthoritativeGameSlug(in.Slug)' "${REPO_DIR}/internal/api/admin.go" \
  || ! grep -Fq 'isProtectedAuthoritativeGameSlug(currentSlug)' "${REPO_DIR}/internal/api/admin.go" \
  || ! grep -Fq 'isProtectedAuthoritativeGameSlug(currentSlug)' "${REPO_DIR}/internal/api/content.go" \
  || ! grep -Fq 'isProtectedAuthoritativeGameSlug(game.Slug)' "${REPO_DIR}/internal/api/content.go"; then
  printf '%s\n' 'Direct admin updates and workflow apply updates must protect every built-in authoritative game slug.' >&2
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

# The score has to come from the server's own replay of the player's inputs, not
# from numbers the browser reported, and the two kernels have to stay pinned to
# each other by committed vectors.
if ! grep -Fq 'const realmGuardReplayMethod = "server_replay_v1"' "${REPO_DIR}/internal/api/realmguard_replay.go" \
  || ! grep -Fq 'in = applyRealmGuardReplay(in, outcome)' "${REPO_DIR}/internal/api/realmguard.go" \
  || ! grep -Fq 'content_projection_mismatch' "${REPO_DIR}/internal/api/realmguard_replay.go" \
  || ! grep -Fq 'RulesVersion = "realmguard-kernel-1"' "${REPO_DIR}/internal/battle/realmguard/types.go" \
  || ! grep -Fq 'KERNEL_RULES_VERSION = "realmguard-kernel-1"' "${REPO_DIR}/web/src/games/realmguard/kernel/ledger.ts" \
  || [[ ! -s "${REPO_DIR}/internal/battle/realmguard/testdata/vectors.json" ]] \
  || [[ ! -s "${REPO_DIR}/internal/api/testdata/realmguard_projection.json" ]] \
  || [[ ! -s "${REPO_DIR}/internal/api/testdata/realmguard_published_config.json" ]] \
  || [[ ! -s "${REPO_DIR}/scripts/testdata/realmguard-smoke.json" ]]; then
  printf '%s\n' 'RealmGuard results must be derived by the server replay, with the browser and Go kernels pinned together by committed vectors.' >&2
  exit 1
fi

if ! grep -Fq 'required = "profile:write"' "${REPO_DIR}/internal/api/api.go" \
  || ! grep -Fq 'path == "/api/v1/me/achievements"' "${REPO_DIR}/internal/api/api.go" \
  || ! grep -Fq 'required = "scores:write"' "${REPO_DIR}/internal/api/api.go"; then
  printf '%s\n' 'Personal API keys must split profile reads, profile mutations, and achievement writes by scope.' >&2
  exit 1
fi

if ! grep -Fq '"realmguard_version_id": map[string]any' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq '"defense_content_version_id": map[string]any' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq 'metadata["realmguard_version_id"] = versionID' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq 'metadata["defense_content_version_id"] = versionID' "${REPO_DIR}/internal/api/mcp.go"; then
  printf '%s\n' 'MCP game_session_start must expose and forward both authoritative published-version pins.' >&2
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

# The smoke battle is generated by the browser kernel, so the milestones it
# streams live in the fixture rather than in the script.
for smoke_battle_contract in \
  'realmguard.battle.ready' \
  'realmguard.wave.start' \
  'realmguard.wave.complete' \
  'realmguard.tower.build' \
  'realmguard.tower.upgrade' \
  'realmguard.tower.sell' \
  'realmguard.battle.complete' \
  'defeated_by_enemy' \
  'escaped_by_enemy' \
  'spawned_by_enemy' \
  'realmguard-kernel-1' \
  'config_digest'; do
  if ! grep -Fq "${smoke_battle_contract}" "${REPO_DIR}/scripts/testdata/realmguard-smoke.json"; then
    printf 'RealmGuard smoke battle fixture is missing contract coverage: %s\n' "${smoke_battle_contract}" >&2
    exit 1
  fi
done

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
  'realmguard.battle.complete' \
  'server_replay_v1' \
  'content_projection_mismatch' \
  'client_evidence_ignored' \
  'protected_game_identity' \
  'contains("authoritative")' \
  'precondition_required' \
  'stale_version'; do
  if ! grep -Fq "${smoke_contract}" "${REPO_DIR}/scripts/smoke-realmguard.sh"; then
    printf 'RealmGuard release smoke is missing contract coverage: %s\n' "${smoke_contract}" >&2
    exit 1
  fi
done

if [[ ! -f "${REPO_DIR}/migrations/005_defense_series.sql" ]]; then
  printf '%s\n' 'The embedded Defense Series migration is required.' >&2
  exit 1
fi
for slug in office-guardians cyber-fortress ai-nexus-defense; do
  if ! grep -Fq "'${slug}'" "${REPO_DIR}/migrations/005_defense_series.sql" \
    || ! grep -Fq "'/games/${slug}'" "${REPO_DIR}/migrations/005_defense_series.sql" \
    || ! grep -Fq "'/assets/games/${slug}.svg'" "${REPO_DIR}/migrations/005_defense_series.sql" \
    || ! grep -Fq "'/assets/games/${slug}-banner.svg'" "${REPO_DIR}/migrations/005_defense_series.sql"; then
    printf 'Defense Series migration is missing canonical metadata for %s.\n' "${slug}" >&2
    exit 1
  fi
  for suffix in '' '-banner'; do
    if [[ ! -f "${REPO_DIR}/web/public/assets/games/${slug}${suffix}.svg" ]]; then
      printf 'Defense Series offline SVG is missing: %s%s.svg\n' "${slug}" "${suffix}" >&2
      exit 1
    fi
  done
done

for defense_column in defense_content_version_id source_version_id resource_state verification_method attestation server_proof; do
  if ! grep -Fq "${defense_column}" "${REPO_DIR}/migrations/005_defense_series.sql"; then
    printf 'Defense Series migration is missing authoritative column contract: %s\n' "${defense_column}" >&2
    exit 1
  fi
done

if grep -Fq "jsonb_build_object('id','safe'" "${REPO_DIR}/migrations/005_defense_series.sql" \
  || grep -Fq "jsonb_build_object('id','unsafe'" "${REPO_DIR}/migrations/005_defense_series.sql" \
  || grep -Fq "'correct_answer_id','safe'" "${REPO_DIR}/migrations/005_defense_series.sql"; then
  printf '%s\n' 'Defense education answer IDs must be neutral and answer keys must not use one semantic ID.' >&2
  exit 1
fi

for route_contract in \
  'a.Get("/api/v1/defense/{slug}/config"' \
  'a.Get("/api/v1/defense/{slug}/version"' \
  'a.Get("/api/v1/defense/{slug}/progress"' \
  'a.Post("/api/v1/defense/{slug}/results"' \
  'a.Get("/api/v1/defense/{slug}/rankings"' \
  'a.Get("/api/v1/defense/{slug}/learning"' \
  'a.Post("/api/v1/defense/{slug}/education/events/{eventID}/answer"' \
  'Get("/api/v1/defense/{slug}/versions/{id}/preview"' \
  'Get("/api/v1/defense/versions/pending"' \
  'Post("/api/v1/defense/versions/{id}/review"' \
  'admin.Get("/defense/{slug}/drafts/{section}"' \
  'admin.Get("/defense/{slug}/versions"' \
  'admin.Post("/defense/{slug}/versions"' \
  'admin.Post("/defense/{slug}/versions/{id}/test"' \
  'admin.Post("/defense/{slug}/versions/{id}/publish"' \
  'admin.Get("/defense/{slug}/telemetry"' \
  'admin.Get("/defense/{slug}/learning-report"'; do
  if ! grep -Fq "${route_contract}" "${REPO_DIR}/internal/api/api.go"; then
    printf 'Missing Defense Series route contract: %s\n' "${route_contract}" >&2
    exit 1
  fi
done

for catalog_contract in \
  'defense_version_required' \
  'defense_config_stale' \
  'defense_ranking_required' \
  'defense_content_version_id'; do
  if ! grep -Fq "${catalog_contract}" "${REPO_DIR}/internal/api/catalog.go"; then
    printf 'Missing Defense session/ranking isolation contract: %s\n' "${catalog_contract}" >&2
    exit 1
  fi
done

generic_rankings_handler="$(awk '
  /^func \(s \*Server\) rankings\(/ {inside=1}
  inside {print}
  inside && /^}/ {exit}
' "${REPO_DIR}/internal/api/catalog.go")"
if ! grep -Fq 'defense_ranking_required' <<<"${generic_rankings_handler}"; then
  printf '%s\n' 'The generic rankings handler must reject Defense Series and direct clients to the authoritative route.' >&2
  exit 1
fi

if ! grep -Fq 'const defenseVerificationMethod = "server_received_telemetry_v1"' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Eq 'defenseOptionalTelemetryLimit[[:space:]]*=[[:space:]]*128' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Eq 'defenseWaveTelemetryLimit[[:space:]]*=[[:space:]]*100' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Eq 'defenseTowerLedgerLimit[[:space:]]*=[[:space:]]*10000' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'defenseTelemetryLimitReached(in.Event, counts)' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'record.Sequence != index+1' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'defense.battle.ready' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'defense.wave.complete' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'case "defense.education.apply":' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'counts[event] >= 500' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'defense.battle.complete' "${REPO_DIR}/internal/api/defense_attestation.go"; then
  printf '%s\n' 'Defense results require bounded ordered server-received telemetry attestation.' >&2
  exit 1
fi

if ! grep -Fq 'func sanitizeDefenseContent' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'func defenseContentKeyIsSensitive' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'strings.HasPrefix(normalized, "correctanswer")' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'strings.HasPrefix(normalized, "explanation")' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq '"correctAnswerId"' "${REPO_DIR}/internal/api/defense_test.go" \
  || ! grep -Fq '"correctAnswers"' "${REPO_DIR}/internal/api/defense_test.go" \
  || ! grep -Fq '"answerKey"' "${REPO_DIR}/internal/api/defense_test.go" \
  || ! grep -Fq 'len(content.ModelProfiles) != 5' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'validateDefenseResourceState' "${REPO_DIR}/internal/api/defense_attestation.go" \
  || ! grep -Fq 'realmGuardManagerReviewTeamError(p.Team, creatorTeam)' "${REPO_DIR}/internal/api/defense_admin.go" \
  || ! grep -Fq 'realmGuardExpectedChecksum(w, r)' "${REPO_DIR}/internal/api/defense_admin.go"; then
  printf '%s\n' 'Defense public redaction, AI resource/model, optimistic concurrency, and fail-closed team review contracts are required.' >&2
  exit 1
fi

if ! grep -Fq 'cumulative telemetry snapshot exceeds the 4 KiB transport limit' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'len(sample)+512 > 4<<10' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'content_version_id=$3' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'func validDefenseAIResourceScoreFactors' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'if len(factors) != 4' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'strings.TrimSpace(profile.Name)' "${REPO_DIR}/internal/api/defense.go" \
  || ! grep -Fq 'source_version_id' "${REPO_DIR}/internal/api/defense_admin.go"; then
  printf '%s\n' 'Defense content validation must enforce the 4 KiB transport budget, version-isolated progress, and immutable rollback sources.' >&2
  exit 1
fi

if ! grep -Fq 'defense_content_version_id' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq 'realmguard_version_id' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq '"name": "defense_config_get"' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq '"name": "defense_rankings_get"' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq '[]string{"daily", "weekly", "monthly", "season", "all_time"}' "${REPO_DIR}/internal/api/mcp.go" \
  || ! grep -Fq 'defense_content_version_id' "${REPO_DIR}/docs/mcp.md" \
  || ! grep -Fq '`defense_config_get`' "${REPO_DIR}/docs/mcp.md" \
  || ! grep -Fq '`defense_rankings_get`' "${REPO_DIR}/docs/mcp.md"; then
  printf '%s\n' 'MCP game_session_start must expose and forward the mutually exclusive RealmGuard/Defense published version pins.' >&2
  exit 1
fi

if ! grep -Fq 'scripts/smoke-defense-series.sh' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'IGAME_REQUIRE_DEFENSE_DRAFT=true' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'IGAME_SMOKE_POSTGRES_DSN=' "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq "DEFENSE_TEST_DSN='postgres://" "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq "TestDefensePublishedSeedContract" "${REPO_DIR}/.github/workflows/release.yml" \
  || ! grep -Fq 'defense-smoke:' "${REPO_DIR}/Makefile"; then
  printf '%s\n' 'Release CI must run the fresh-DB Defense API and browser gates.' >&2
  exit 1
fi

for smoke_contract in \
  'defense_content_version_id' \
  'defense_version_required' \
  'defense_config_stale' \
  'defense_ranking_required' \
  'defense_config_get' \
  'defense_rankings_get' \
  'period:"season"' \
  'correct_answer_id' \
  'model_profiles' \
  'resource_state' \
  'telemetry_event_conflict' \
  'telemetry_sequence_conflict' \
  'sequence <= 128' \
  'defense.battle.ready' \
  'defense.wave.complete' \
  'defense.battle.complete' \
  'payload_at_limit_data' \
  '4 KiB transport limit' \
  'SMOKE_DIFFICULTIES' \
  'for difficulty in casual normal veteran' \
  'ai-resource-depletion' \
  'assert_fresh_published_boundary' \
  'source_version_id' \
  'policy_version' \
  'average_game_score' \
  '/api/v1/me/achievements' \
  'body-only-forgery' \
  'server_received_telemetry_v1' \
  'precondition_required' \
  'stale_version'; do
  if ! grep -Fq "${smoke_contract}" "${REPO_DIR}/scripts/smoke-defense-series.sh"; then
    printf 'Defense release smoke is missing contract coverage: %s\n' "${smoke_contract}" >&2
    exit 1
  fi
done

for browser_contract in \
  'office-guardians' \
  'cyber-fortress' \
  'ai-nexus-defense' \
  'defense_content_version_id' \
  'defense-choice-event' \
  'AI resource HUD' \
  'scene paused by education prompt' \
  'resource-depletion result' \
  'late education modal after terminal resource depletion' \
  'data-ai-depleted' \
  'IGAME_SCREENSHOT_DIR' \
  'stage 2 unlock' \
  'Defense Content Studio' \
  'browser-rollback-policy-v0.3.0' \
  'report metric definition' \
  'Education Report restored after deep refresh' \
  'IGAME_REQUIRE_DEFENSE_DRAFT'; do
  if ! grep -Fq "${browser_contract}" "${REPO_DIR}/scripts/browser-smoke.mjs"; then
    printf 'Defense browser smoke is missing gate: %s\n' "${browser_contract}" >&2
    exit 1
  fi
done

if ! grep -Fq 'TestDefenseTelemetryAllowsImmediateWaveStartDepletion' "${REPO_DIR}/internal/api/defense_test.go" \
  || ! grep -Fq 'defenseEducationTrigger("defense.wave.start", { wave: 3 }, true)' "${REPO_DIR}/web/src/games/defense/telemetry.test.ts"; then
  printf '%s\n' 'Release tests must preserve terminal wave-start depletion and suppress its late education prompt.' >&2
  exit 1
fi

printf 'Release contract verified for igame:v%s\n' "${VERSION}"

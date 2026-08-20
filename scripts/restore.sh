#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 2 || "$2" != "RESTORE" ]]; then
  printf 'Usage: %s BACKUP.dump RESTORE\n' "$0" >&2
  exit 2
fi
if [[ -z "${POSTGRES_DSN:-}" ]]; then
  printf '%s\n' 'POSTGRES_DSN is required.' >&2
  exit 1
fi
if ! command -v pg_restore >/dev/null 2>&1; then
  printf '%s\n' 'pg_restore is required. Use the PostgreSQL client version matching the server major version.' >&2
  exit 1
fi

readonly BACKUP="$1"
if [[ ! -f "${BACKUP}" ]]; then
  printf 'Backup does not exist: %s\n' "${BACKUP}" >&2
  exit 1
fi
if [[ -f "${BACKUP}.sha256" ]]; then
  (cd "$(dirname -- "${BACKUP}")" && sha256sum --check "$(basename -- "${BACKUP}").sha256")
else
  printf '%s\n' 'Refusing restore without the adjacent .sha256 checksum file.' >&2
  exit 1
fi

printf '%s\n' 'Restore will overwrite objects in the target igame database.'
pg_restore \
  --dbname="${POSTGRES_DSN}" \
  --clean \
  --if-exists \
  --no-owner \
  --no-acl \
  --exit-on-error \
  "${BACKUP}"
printf '%s\n' 'Restore completed. Restart igame and check /readyz.'

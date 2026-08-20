#!/usr/bin/env bash
set -Eeuo pipefail

if [[ -z "${POSTGRES_DSN:-}" ]]; then
  printf '%s\n' 'POSTGRES_DSN is required.' >&2
  exit 1
fi
if ! command -v pg_dump >/dev/null 2>&1; then
  printf '%s\n' 'pg_dump is required. Use the PostgreSQL client version matching the server major version.' >&2
  exit 1
fi

backup_dir="${1:-./backups}"
mkdir -p -- "${backup_dir}"
backup_dir="$(cd -- "${backup_dir}" && pwd)"
timestamp="$(date -u '+%Y%m%dT%H%M%SZ')"
target="${backup_dir%/}/igame-${timestamp}.dump"
tmp_target="$(mktemp "${backup_dir%/}/.igame-backup.XXXXXX")"
trap 'rm -f -- "${tmp_target}"' EXIT

if [[ -e "${target}" || -e "${target}.sha256" ]]; then
  printf 'Refusing to overwrite an existing backup: %s\n' "${target}" >&2
  exit 1
fi

pg_dump --dbname="${POSTGRES_DSN}" --format=custom --compress=9 --no-owner --no-acl --file="${tmp_target}"
mv -- "${tmp_target}" "${target}"
(
  cd -- "${backup_dir}"
  sha256sum "$(basename -- "${target}")" > "$(basename -- "${target}").sha256"
)
chmod 600 "${target}" "${target}.sha256"
printf 'Backup created: %s\n' "${target}"

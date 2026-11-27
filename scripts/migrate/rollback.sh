#!/usr/bin/env bash
# Reverts per-feature database migrations in reverse chronological order (D-02)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-idp_db}"
export PGPASSWORD="${DB_PASSWORD:-postgres_dev_password_123}"

mapfile -t ROLLBACK_FILES < <(
  find "${REPO_ROOT}/src/features" -path "*/database/migrations/*.down.sql" -type f \
    | awk -F/ '{print $NF "\t" $0}' \
    | sort -r -k1,1 \
    | cut -f2-
)

for sql_file in "${ROLLBACK_FILES[@]}"; do
    rel_path="${sql_file#"${REPO_ROOT}/"}"
    echo -e "\033[1;34m[ROLLBACK]\033[0m Reverting ${rel_path}..."
    psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -f "${sql_file}" > /dev/null
done

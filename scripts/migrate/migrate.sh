#!/usr/bin/env bash
# Applies declarative DB roles and chronological per-feature SQL migrations (D-01, D-02)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-idp_db}"
export PGPASSWORD="${DB_PASSWORD:-postgres_dev_password_123}"

echo -e "\033[1;34m[INFO]\033[0m Applying PostgreSQL roles from database/roles.sql..."
psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -f "${REPO_ROOT}/database/roles.sql" > /dev/null

echo -e "\033[1;34m[INFO]\033[0m Executing chronological per-feature migrations on ${DB_HOST}:${DB_PORT}/${DB_NAME}..."
mapfile -t MIGRATION_FILES < <(
  find "${REPO_ROOT}/src/features" -path "*/database/migrations/*.up.sql" -type f \
    | awk -F/ '{print $NF "\t" $0}' \
    | sort -k1,1 \
    | cut -f2-
)

for sql_file in "${MIGRATION_FILES[@]}"; do
    rel_path="${sql_file#"${REPO_ROOT}/"}"
    echo -e "\033[1;34m[MIGRATE]\033[0m Applying ${rel_path}..."
    psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -f "${sql_file}" > /dev/null
done

echo -e "\033[1;32m[SUCCESS]\033[0m All per-feature database migrations applied successfully."
psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "\dn"

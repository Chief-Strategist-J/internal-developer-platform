#!/usr/bin/env bash
# Applies pending DDL migrations from canonical database/migrations/sql/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}/../../database/migrations/sql"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-idp_db}"
export PGPASSWORD="${DB_PASSWORD:-postgres_dev_password_123}"

echo -e "\033[1;34m[INFO]\033[0m Executing PostgreSQL migrations on ${DB_HOST}:${DB_PORT}/${DB_NAME}..."

for sql_file in $(ls "${MIGRATIONS_DIR}"/*.sql | grep -v "rollback" | sort); do
    echo -e "\033[1;34m[MIGRATE]\033[0m Applying $(basename "${sql_file}")..."
    psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -f "${sql_file}" > /dev/null
done

echo -e "\033[1;32m[SUCCESS]\033[0m All database migrations applied successfully to Kubernetes PostgreSQL."
psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "\dt"

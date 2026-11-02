#!/usr/bin/env bash
# Applies pending DDL migrations against live PostgreSQL container
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}/migrations"

echo -e "\033[1;34m[INFO]\033[0m Executing PostgreSQL migrations against container idp-db-1..."

for sql_file in $(ls "${MIGRATIONS_DIR}"/*.sql | sort); do
    echo -e "\033[1;34m[MIGRATE]\033[0m Applying $(basename "${sql_file}")..."
    docker exec -i idp-db-1 psql -U postgres -d idp_db < "${sql_file}"
done

echo -e "\033[1;32m[SUCCESS]\033[0m All database migrations applied successfully."
docker exec idp-db-1 psql -U postgres -d idp_db -c "\dt"

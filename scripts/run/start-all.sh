#!/usr/bin/env bash
# Starts the Go Backend Platform API and Next.js Frontend Developer Portal
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo -e "\033[1;34m[INFO]\033[0m Starting Internal Developer Platform (Go Backend + Next.js Frontend)..."

cd "${PROJECT_ROOT}"

# Apply migrations first
echo -e "\033[1;34m[INFO]\033[0m Ensuring PostgreSQL schema migrations are up to date..."
"${SCRIPT_DIR}/../migrate/migrate.sh"

# Source environment
if [[ -f "${PROJECT_ROOT}/.env" ]]; then
    export $(grep -v '^#' "${PROJECT_ROOT}/.env" | xargs -d '\n')
fi

# Build Go Platform API binary
echo -e "\033[1;34m[INFO]\033[0m Building Go Platform API..."
go build -o /tmp/platform-api-live ./src/main.go

export DATABASE_URL="postgres://postgres:postgres_dev_password_123@127.0.0.1:5432/idp_db?sslmode=disable"
export REDIS_ADDR="127.0.0.1:6379"
export JWT_SECRET="${JWT_SECRET:-live-development-jwt-secret-key-32bytes}"
export PORT="${PORT:-8080}"
export ADMIN_PORT="${ADMIN_PORT:-8081}"

echo -e "\033[1;34m[INFO]\033[0m Booting Go Platform API on port ${PORT} (Admin: ${ADMIN_PORT})..."
/tmp/platform-api-live > /tmp/platform-api.log 2>&1 &
GO_PID=$!

cleanup() {
    echo -e "\033[1;34m[INFO]\033[0m Shutting down platform services..."
    kill -15 "${GO_PID}" 2>/dev/null || true
    rm -f /tmp/platform-api-live
}
trap cleanup EXIT INT TERM

# Wait for Go server readiness
echo -e "\033[1;34m[INFO]\033[0m Checking Go server readiness..."
for i in {1..20}; do
    if curl -s "http://127.0.0.1:${ADMIN_PORT}/readyz" | grep -q "healthy"; then
        echo -e "\033[1;32m[SUCCESS]\033[0m Go Backend API is healthy and accepting traffic on port ${PORT}."
        break
    fi
    sleep 0.5
done

# Start Next.js Frontend Developer Portal
echo -e "\033[1;34m[INFO]\033[0m Starting Next.js Developer Portal on port 7007..."
cd "${PROJECT_ROOT}/typescript-modules"
npm run build
npm start

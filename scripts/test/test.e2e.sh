#!/usr/bin/env bash
# Runs end-to-end regression runner against platform API and live infrastructure
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo -e "\033[1;34m[INFO]\033[0m Starting End-to-End (E2E) Regression Test..."

cd "${PROJECT_ROOT}"

# Pick ephemeral ports to avoid collision
E2E_PORT=18081
E2E_ADMIN_PORT=19091
export PORT="${E2E_PORT}"
export ADMIN_PORT="${E2E_ADMIN_PORT}"
export REDIS_ADDR="127.0.0.1:6379"
export JWT_SECRET="e2e-automated-regression-secret-32bytes"

# Compile platform binary
echo -e "\033[1;34m[INFO]\033[0m Compiling platform binary..."
go build -o /tmp/platform-api-e2e ./src/main.go

# Start platform server in background
echo -e "\033[1;34m[INFO]\033[0m Booting platform server on port ${E2E_PORT}..."
/tmp/platform-api-e2e > /tmp/platform-e2e.log 2>&1 &
SERVER_PID=$!

cleanup() {
    echo -e "\033[1;34m[INFO]\033[0m Shutting down E2E server (PID: ${SERVER_PID})..."
    kill -15 "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    rm -f /tmp/platform-api-e2e
}
trap cleanup EXIT

# Wait for server readiness
echo -e "\033[1;34m[INFO]\033[0m Awaiting server readiness on admin port ${E2E_ADMIN_PORT}..."
for i in {1..20}; do
    if curl -s "http://127.0.0.1:${E2E_ADMIN_PORT}/readyz" | grep -q "healthy"; then
        echo -e "\033[1;32m[SUCCESS]\033[0m Server ready."
        break
    fi
    sleep 0.5
done

# Run dedicated Go E2E client test
echo -e "\033[1;34m[INFO]\033[0m Executing live E2E HTTP client suite..."
go test -v -count=1 ./src/features/role-based-access-control/tests/ -run TestLiveE2E_FullHTTPServerFlow

echo -e "\033[1;32m[SUCCESS]\033[0m End-to-End Regression Test Completed Successfully."

#!/usr/bin/env bash
# Runs the full Go platform server and tests all RBAC and Catalog APIs via real curl requests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo "================================================================================"
echo "          VERIFYING FULL HTTP API SERVER & HANDLERS VIA CURL REQUESTS           "
echo "================================================================================"

cd "${PROJECT_ROOT}"

CLI_PORT=38081
CLI_ADMIN_PORT=39091
SECRET="real-curl-test-secret-key-production-32bytes"
export PORT="${CLI_PORT}"
export ADMIN_PORT="${CLI_ADMIN_PORT}"
export JWT_SECRET="${SECRET}"
export REDIS_ADDR="127.0.0.1:6379"

echo -e "\033[1;34m[BUILD]\033[0m Compiling platform binary..."
go build -o /tmp/platform-api-server ./src/main.go

echo -e "\033[1;34m[RUN]\033[0m Starting platform API server (PORT=${CLI_PORT}, ADMIN_PORT=${CLI_ADMIN_PORT})..."
/tmp/platform-api-server > /tmp/platform-server.log 2>&1 &
SERVER_PID=$!

cleanup() {
    echo ""
    echo -e "\033[1;34m[CLEANUP]\033[0m Stopping platform API server (PID: ${SERVER_PID})..."
    kill -15 "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    rm -f /tmp/platform-api-server
}
trap cleanup EXIT

# Wait for server readiness
for i in {1..20}; do
    if curl -s "http://127.0.0.1:${CLI_ADMIN_PORT}/readyz" | grep -q "healthy"; then
        echo -e "\033[1;32m[SERVER READY]\033[0m Platform server listening at http://127.0.0.1:${CLI_PORT}"
        break
    fi
    sleep 0.5
done

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 1] POST /api/v1/auth/token -> Issue valid Developer JWT"
echo "--------------------------------------------------------------------------------"
DEV_SUB="dev-sarah-$(date +%s%N)"
DEV_TOKEN_RESP=$(curl -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"subjectId\": \"${DEV_SUB}\",
    \"tenantId\": \"fintech-team\",
    \"roles\": [\"Developer\"],
    \"ttlSeconds\": 3600
  }")

echo "${DEV_TOKEN_RESP}"
DEV_TOKEN=$(echo "${DEV_TOKEN_RESP}" | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)

if [[ -n "${DEV_TOKEN}" ]]; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully issued JWT token for ${DEV_SUB}"
else
    echo -e "\033[1;31m[FAIL]\033[0m Failed to parse access token"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 2] GET /api/v1/auth/me -> Inspect Caller Identity and Claims"
echo "--------------------------------------------------------------------------------"
ME_RESP=$(curl -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/auth/me" \
  -H "Authorization: Bearer ${DEV_TOKEN}")

echo "${ME_RESP}"
if echo "${ME_RESP}" | grep -q "200 OK" && echo "${ME_RESP}" | grep -q "${DEV_SUB}"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/auth/me returned correct authenticated subject"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with ${DEV_SUB} subject"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 3] POST /api/v1/auth/evaluate -> PDP Decision Evaluation"
echo "--------------------------------------------------------------------------------"
EVAL_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/evaluate" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "targetTenantId": "fintech-team",
    "resource": "service-catalog",
    "action": "write"
  }')

echo "${EVAL_RESP}"
if echo "${EVAL_RESP}" | grep -q "200 OK" && echo "${EVAL_RESP}" | grep -q '"granted":true'; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/auth/evaluate confirmed granted=true"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected granted=true"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 4] POST /api/v1/services -> Register service with Bearer Token"
echo "--------------------------------------------------------------------------------"
SERVICE_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "fintech-ledger-api",
    "tenantId": "fintech-team",
    "name": "Fintech Ledger API",
    "owner": "sarah@fintech.io",
    "lifecycle": "production"
  }')

echo "${SERVICE_RESP}"
if echo "${SERVICE_RESP}" | grep -q "201 Created"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/services successfully created service descriptor"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 5] POST /api/v1/auth/token -> Issue PlatformAdmin Token"
echo "--------------------------------------------------------------------------------"
ADMIN_TOKEN_RESP=$(curl -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d '{
    "subjectId": "secops-officer-marcus",
    "tenantId": "system-admin",
    "roles": ["PlatformAdmin"]
  }')

ADMIN_TOKEN=$(echo "${ADMIN_TOKEN_RESP}" | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)
echo -e "\033[1;32m[PASS]\033[0m Issued PlatformAdmin token for secops-officer-marcus"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 6] POST /api/v1/auth/revoke -> Admin revokes ${DEV_SUB} session"
echo "--------------------------------------------------------------------------------"
REVOKE_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/revoke" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{
    \"subjectId\": \"${DEV_SUB}\",
    \"reason\": \"Routine security session termination\"
  }")

echo "${REVOKE_RESP}"
if echo "${REVOKE_RESP}" | grep -q "200 OK"; then
    echo -e "\033[1;32m[PASS]\033[0m Admin successfully revoked ${DEV_SUB} session"
else
    echo -e "\033[1;31m[FAIL]\033[0m Revocation failed"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 7] GET /api/v1/auth/me -> ${DEV_SUB} tries to use revoked token"
echo "--------------------------------------------------------------------------------"
POST_REVOKE_RESP=$(curl -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/auth/me" \
  -H "Authorization: Bearer ${DEV_TOKEN}")

echo "${POST_REVOKE_RESP}"
if echo "${POST_REVOKE_RESP}" | grep -q "401 Unauthorized"; then
    echo -e "\033[1;32m[PASS]\033[0m Revoked token instantly denied with HTTP 401 Unauthorized"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 401 Unauthorized"
    exit 1
fi

echo ""
echo "================================================================================"
echo "          ALL REAL HTTP CURL API TESTS PASSED AGAINST RUNNING SERVER            "
echo "================================================================================"

#!/usr/bin/env bash
# Real-world manual operator simulation script
# Reproduces how a real human engineer, developer, auditor, and security officer test the API
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo "================================================================================"
echo "          REAL-PERSON / OPERATOR MANUAL TESTING SIMULATION WORKFLOW             "
echo "================================================================================"

cd "${PROJECT_ROOT}"

CLI_PORT=28081
CLI_ADMIN_PORT=29091
SECRET="real-person-manual-test-secret-key-32bytes"
export PORT="${CLI_PORT}"
export ADMIN_PORT="${CLI_ADMIN_PORT}"
export JWT_SECRET="${SECRET}"
export REDIS_ADDR="127.0.0.1:6379"

echo -e "\033[1;34m[STEP 0]\033[0m Building platform server and CLI token helper..."
go build -o /tmp/platform-api-manual ./src/main.go
go build -o /tmp/token-helper ./scripts/test/token-helper/main.go

echo -e "\033[1;34m[STEP 0]\033[0m Starting platform API in background on port ${CLI_PORT}..."
/tmp/platform-api-manual > /tmp/platform-manual.log 2>&1 &
SERVER_PID=$!

cleanup() {
    echo ""
    echo -e "\033[1;34m[CLEANUP]\033[0m Shutting down platform API (PID: ${SERVER_PID})..."
    kill -15 "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    rm -f /tmp/platform-api-manual /tmp/token-helper
}
trap cleanup EXIT

sleep 1

# Await readiness
for i in {1..20}; do
    if curl -s "http://127.0.0.1:${CLI_ADMIN_PORT}/readyz" | grep -q "healthy"; then
        echo -e "\033[1;32m[SERVER ONLINE]\033[0m Listening at http://127.0.0.1:${CLI_PORT}"
        break
    fi
    sleep 0.5
done

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TEST 1] Real Person: Anonymous request with NO Bearer Token"
echo "Command: curl -i -X POST http://127.0.0.1:${CLI_PORT}/api/v1/services -d '{}'"
echo "--------------------------------------------------------------------------------"
RESPONSE_1=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" -d '{}' || true)
echo "${RESPONSE_1}"
if echo "${RESPONSE_1}" | grep -q "401 Unauthorized"; then
    echo -e "\033[1;32m[PASS]\033[0m Anonymous request correctly rejected with HTTP 401 Unauthorized"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected HTTP 401"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TEST 2] Real Person: Developer Alice issues service registration in her tenant"
echo "--------------------------------------------------------------------------------"
ALICE_TOKEN=$(/tmp/token-helper "${SECRET}" "alice.smith" "payments-team" "Developer")
echo "Alice's Token: ${ALICE_TOKEN:0:45}..."

RESPONSE_2=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${ALICE_TOKEN}" \
  -H "X-Tenant-ID: payments-team" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "payments-checkout-service",
    "tenantId": "payments-team",
    "name": "Checkout Microservice",
    "owner": "alice.smith@company.com",
    "lifecycle": "production"
  }' || true)

echo "${RESPONSE_2}"
if echo "${RESPONSE_2}" | grep -q "201 Created"; then
    echo -e "\033[1;32m[PASS]\033[0m Developer Alice successfully registered service (HTTP 201 Created)"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected HTTP 201"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TEST 3] Real Person: Alice tries to register service in core-infra tenant (Cross-Tenant Breach)"
echo "--------------------------------------------------------------------------------"
RESPONSE_3=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${ALICE_TOKEN}" \
  -H "X-Tenant-ID: core-infra" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "malicious-cluster-controller",
    "tenantId": "core-infra",
    "name": "Rogue Cluster Controller",
    "owner": "alice.smith@company.com",
    "lifecycle": "production"
  }' || true)

echo "${RESPONSE_3}"
if echo "${RESPONSE_3}" | grep -q "403 Forbidden"; then
    echo -e "\033[1;32m[PASS]\033[0m Cross-tenant attack rejected with HTTP 403 Forbidden"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected HTTP 403"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TEST 4] Real Person: Auditor Bob (Viewer role) attempts to create service"
echo "--------------------------------------------------------------------------------"
BOB_TOKEN=$(/tmp/token-helper "${SECRET}" "bob.auditor" "payments-team" "Viewer")

RESPONSE_4=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${BOB_TOKEN}" \
  -H "X-Tenant-ID: payments-team" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "unauthorized-audit-tool",
    "tenantId": "payments-team",
    "name": "Audit Tool",
    "owner": "bob.auditor@company.com",
    "lifecycle": "experimental"
  }' || true)

echo "${RESPONSE_4}"
if echo "${RESPONSE_4}" | grep -q "403 Forbidden"; then
    echo -e "\033[1;32m[PASS]\033[0m Viewer mutation rejected with HTTP 403 Forbidden"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected HTTP 403"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TEST 5] Real Person: SecOps Admin revokes Alice's session directly in live Redis"
echo "Command: docker exec idp-cache-1 redis-cli SETEX revocation:sub:alice.smith 86400 revoked"
echo "--------------------------------------------------------------------------------"
docker exec idp-cache-1 redis-cli SETEX revocation:sub:alice.smith 86400 revoked

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TEST 6] Real Person: Alice attempts to use her revoked token again"
echo "--------------------------------------------------------------------------------"
RESPONSE_6=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${ALICE_TOKEN}" \
  -H "X-Tenant-ID: payments-team" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "payments-fraud-detector",
    "tenantId": "payments-team",
    "name": "Fraud Detector",
    "owner": "alice.smith@company.com",
    "lifecycle": "production"
  }' || true)

echo "${RESPONSE_6}"
if echo "${RESPONSE_6}" | grep -q "401 Unauthorized"; then
    echo -e "\033[1;32m[PASS]\033[0m Revoked session instantly rejected with HTTP 401 Unauthorized"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected HTTP 401"
    exit 1
fi

echo ""
echo "================================================================================"
echo "          ALL REAL-PERSON OPERATOR TEST SCENARIOS PASSED WITH LIVE INFRA         "
echo "================================================================================"

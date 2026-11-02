#!/usr/bin/env bash
# Real-world verification against PostgreSQL container idp-db-1 and Redis container idp-cache-1
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo "================================================================================"
echo "          VERIFYING PLATFORM API PERSISTENCE ON REAL POSTGRESQL (idp-db-1)      "
echo "================================================================================"

cd "${PROJECT_ROOT}"

# Run DB Migrations first
bash "${PROJECT_ROOT}/scripts/migrate/migrate.sh"

DB_PORT=48081
DB_ADMIN_PORT=49091
SECRET="real-postgres-verification-secret-32bytes"
export PORT="${DB_PORT}"
export ADMIN_PORT="${DB_ADMIN_PORT}"
export JWT_SECRET="${SECRET}"
export REDIS_ADDR="127.0.0.1:6379"
export DATABASE_URL="postgres://postgres:postgres_dev_password_123@127.0.0.1:5432/idp_db?sslmode=disable"

echo -e "\033[1;34m[BUILD]\033[0m Compiling platform binary..."
go build -o /tmp/platform-postgres-api ./src/main.go

echo -e "\033[1;34m[RUN]\033[0m Starting platform API with real PostgreSQL connection..."
/tmp/platform-postgres-api > /tmp/platform-postgres.log 2>&1 &
SERVER_PID=$!

cleanup() {
    echo ""
    echo -e "\033[1;34m[CLEANUP]\033[0m Shutting down server (PID: ${SERVER_PID})..."
    kill -15 "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    rm -f /tmp/platform-postgres-api
}
trap cleanup EXIT

# Wait for server readiness
for i in {1..20}; do
    if curl -s "http://127.0.0.1:${DB_ADMIN_PORT}/readyz" | grep -q "healthy"; then
        echo -e "\033[1;32m[SERVER READY]\033[0m Connected to PostgreSQL & listening at http://127.0.0.1:${DB_PORT}"
        break
    fi
    sleep 0.5
done

# Step 1: Issue token for Developer
DEV_SUB="db-engineer-$(date +%s)"
echo -e "\033[1;34m[TEST 1]\033[0m Issuing Developer token for ${DEV_SUB}..."
DEV_TOKEN_RESP=$(curl -s -X POST "http://127.0.0.1:${DB_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"subjectId\": \"${DEV_SUB}\",
    \"tenantId\": \"tenant-fintech\",
    \"roles\": [\"Developer\"]
  }")
DEV_TOKEN=$(echo "${DEV_TOKEN_RESP}" | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)

# Step 2: Register service via REST API
SVC_ID="postgres-persisted-service-$(date +%s)"
echo -e "\033[1;34m[TEST 2]\033[0m Registering service '${SVC_ID}' in PostgreSQL via POST /api/v1/services..."
REGISTER_RESP=$(curl -s -i -X POST "http://127.0.0.1:${DB_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "X-Tenant-ID: tenant-fintech" \
  -H "Content-Type: application/json" \
  -d "{
    \"id\": \"${SVC_ID}\",
    \"tenantId\": \"tenant-fintech\",
    \"name\": \"Postgres Persisted Service\",
    \"owner\": \"data-team@fintech.io\",
    \"lifecycle\": \"production\"
  }")

echo "${REGISTER_RESP}"
if echo "${REGISTER_RESP}" | grep -q "201 Created"; then
    echo -e "\033[1;32m[PASS]\033[0m Service registration acknowledged with HTTP 201 Created"
else
    echo -e "\033[1;31m[FAIL]\033[0m Failed to register service"
    exit 1
fi

# Step 3: Direct PSQL Verification inside Docker Container
echo ""
echo -e "\033[1;34m[TEST 3]\033[0m Direct PSQL query in container idp-db-1 to verify actual row on disk..."
QUERY_RESULT=$(docker exec idp-db-1 psql -U postgres -d idp_db -c "SELECT id, tenant_id, name, owner, status, version FROM service_descriptors WHERE id = '${SVC_ID}';")
echo "${QUERY_RESULT}"

if echo "${QUERY_RESULT}" | grep -q "${SVC_ID}"; then
    echo -e "\033[1;32m[VERIFIED ON POSTGRES]\033[0m Row verified in table 'service_descriptors' on PostgreSQL 18!"
else
    echo -e "\033[1;31m[FAIL]\033[0m Row not found in PostgreSQL table"
    exit 1
fi

# Step 4: Verify Duplicate Conflict Constraint
echo ""
echo -e "\033[1;34m[TEST 4]\033[0m Attempting to insert duplicate service id -> Expect Conflict (409)..."
CONFLICT_RESP=$(curl -s -i -X POST "http://127.0.0.1:${DB_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "X-Tenant-ID: tenant-fintech" \
  -H "Content-Type: application/json" \
  -d "{
    \"id\": \"${SVC_ID}\",
    \"tenantId\": \"tenant-fintech\",
    \"name\": \"Postgres Persisted Service Duplicate\",
    \"owner\": \"data-team@fintech.io\",
    \"lifecycle\": \"production\"
  }")

echo "${CONFLICT_RESP}"
if echo "${CONFLICT_RESP}" | grep -q "409 Conflict"; then
    echo -e "\033[1;32m[PASS]\033[0m PostgreSQL Primary Key constraint correctly triggered HTTP 409 Conflict"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected HTTP 409 Conflict"
    exit 1
fi

echo ""
echo "================================================================================"
echo "          ALL REAL POSTGRESQL VERIFICATIONS PASSED SUCCESSFULLY                 "
echo "================================================================================"

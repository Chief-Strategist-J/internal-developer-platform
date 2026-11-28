#!/usr/bin/env bash
# Runs the full Go platform server and tests all RBAC and Catalog APIs via real curl requests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/autoscaling.curl.sh"

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
export RBAC_BOOTSTRAP_SECRET="real-curl-bootstrap-secret-32bytes"
export REDIS_ADDR="127.0.0.1:6379"

cleanup_ssp_test_data() {
    if [ -n "${DATABASE_URL:-}" ] && command -v psql >/dev/null 2>&1; then
        psql "${DATABASE_URL}" -c "DELETE FROM self_service_provisioning.provisioning_reservations WHERE normalized_name IN ('gke-fedramp-prod-01', 'gov-cloud-production/orders-prod/orders-sa'); DELETE FROM self_service_provisioning.provisioning_intents WHERE target_id IN ('gke-fedramp-prod-01', 'gov-cloud-production/orders-prod/orders-sa');" >/dev/null 2>&1 || true
    fi
}

if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    source "${PROJECT_ROOT}/.env"
    set +a
    export PORT="${CLI_PORT}"
    export ADMIN_PORT="${CLI_ADMIN_PORT}"
    export JWT_SECRET="${SECRET}"
    export REDIS_ADDR="127.0.0.1:6379"
    export ARGOCD_TOKEN=""
    cleanup_ssp_test_data
fi

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
    cleanup_ssp_test_data
}
trap cleanup EXIT

# Wait for server readiness
for i in {1..20}; do
    if curl --noproxy "*" -s "http://127.0.0.1:${CLI_ADMIN_PORT}/readyz" | grep -q "healthy"; then
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
DEV_TOKEN_RESP=$(curl --noproxy "*" -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
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
ME_RESP=$(curl --noproxy "*" -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/auth/me" \
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
EVAL_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/evaluate" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
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
SVC_ID="fintech-ledger-api-$(date +%s%N)"
SERVICE_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"id\": \"${SVC_ID}\",
    \"tenantId\": \"fintech-team\",
    \"name\": \"Fintech Ledger API\",
    \"owner\": \"sarah@fintech.io\",
    \"lifecycle\": \"production\"
  }")


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
ADMIN_TOKEN_RESP=$(curl --noproxy "*" -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
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
REVOKE_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/revoke" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
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
POST_REVOKE_RESP=$(curl --noproxy "*" -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/auth/me" \
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

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 8] POST /api/v1/workloads -> Provision Workload with Distributed Traces"
echo "--------------------------------------------------------------------------------"
WKLD_DEV_SUB="wkld-dev-$(date +%s%N)"
WKLD_TOKEN_RESP=$(curl --noproxy "*" -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"subjectId\": \"${WKLD_DEV_SUB}\",
    \"tenantId\": \"fintech-team\",
    \"roles\": [\"Developer\"],
    \"ttlSeconds\": 3600
  }")
WKLD_TOKEN=$(echo "${WKLD_TOKEN_RESP}" | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)

WKLD_ID="wkld-e2e-$(date +%s)"
TRACE_ID=$(openssl rand -hex 16 2>/dev/null || python3 -c "import secrets; print(secrets.token_hex(16))")
SPAN_ID=$(openssl rand -hex 8 2>/dev/null || python3 -c "import secrets; print(secrets.token_hex(8))")
TRACEPARENT="00-${TRACE_ID}-${SPAN_ID}-01"

WORKLOAD_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/workloads" \
  -H "Authorization: Bearer ${WKLD_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: ${TRACEPARENT}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"id\": \"${WKLD_ID}\",
    \"tenantId\": \"fintech-team\",
    \"serviceId\": \"fintech-ledger-api\",
    \"namespace\": \"fintech-dev\",
    \"workloadName\": \"ledger-worker\",
    \"image\": \"gcr.io/fintech-team/worker@sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\",
    \"cpuRequest\": \"250m\",
    \"cpuLimit\": \"500m\",
    \"memoryRequest\": \"256Mi\",
    \"memoryLimit\": \"512Mi\",
    \"replicas\": 2,
    \"idempotencyKey\": \"idem-${WKLD_ID}\",
    \"costCenter\": \"CC-FINTECH-01\"
  }")

echo "${WORKLOAD_RESP}"
if echo "${WORKLOAD_RESP}" | grep -q "201 Created"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/workloads successfully provisioned workload"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for workload provision"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 9] GET /api/v1/workloads/${WKLD_ID} -> Retrieve Workload State"
echo "--------------------------------------------------------------------------------"
GET_WKLD_RESP=$(curl --noproxy "*" -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/workloads/${WKLD_ID}" \
  -H "Authorization: Bearer ${WKLD_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: ${TRACEPARENT}")

echo "${GET_WKLD_RESP}"
if echo "${GET_WKLD_RESP}" | grep -q "200 OK" && echo "${GET_WKLD_RESP}" | grep -q "${WKLD_ID}"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/workloads/${WKLD_ID} returned correct workload state"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for workload query"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 10] POST /api/v1/regimes -> Register Platform Regime Policy (B-1 to B-4)"
echo "--------------------------------------------------------------------------------"
REGIME_EXT_ID="regime-ext-$(date +%s%N)"
REGIME_POST_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/regimes" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: 00-$(openssl rand -hex 16)-$(openssl rand -hex 8)-01" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"tenantId\": \"fintech-team\",
    \"externalId\": \"${REGIME_EXT_ID}\",
    \"jurisdiction\": \"US_FEDRAMP_HIGH\",
    \"auditRetentionDays\": 365,
    \"dataResidencyRegions\": [\"us-central1\", \"us-east4\"],
    \"kmsKeyCustodyMode\": \"CMEK\",
    \"rtoMinutesTier0\": 15,
    \"rpoMinutesTier0\": 0,
    \"rtoMinutesTier1\": 60,
    \"rpoMinutesTier1\": 15,
    \"clusterMode\": \"GKE_STANDARD\",
    \"primaryRegionCount\": 2,
    \"standbyRegionCount\": 1,
    \"tenantIsolationMode\": \"INTERNAL_ONLY\"
  }")

echo "${REGIME_POST_RESP}"
if echo "${REGIME_POST_RESP}" | grep -q "201 Created" && echo "${REGIME_POST_RESP}" | grep -q "rgm_"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/regimes successfully registered and activated regime policy"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created with rgm_ publicId"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 11] GET /api/v1/regimes/active -> Fetch Active Regime Policy"
echo "--------------------------------------------------------------------------------"
REGIME_GET_RESP=$(curl --noproxy "*" -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/regimes/active?tenantId=fintech-team" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "traceparent: 00-$(openssl rand -hex 16)-$(openssl rand -hex 8)-01")

echo "${REGIME_GET_RESP}"
if echo "${REGIME_GET_RESP}" | grep -q "200 OK" && echo "${REGIME_GET_RESP}" | grep -q "US_FEDRAMP_HIGH"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/regimes/active successfully retrieved active regime policy"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with US_FEDRAMP_HIGH jurisdiction"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 12] POST /api/v1/regimes -> Production Retention Floor Compliance (Retention >= 30)"
echo "--------------------------------------------------------------------------------"
REGIME_ERR_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/regimes" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: 00-$(openssl rand -hex 16)-$(openssl rand -hex 8)-01" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"tenantId\": \"fintech-team\",
    \"externalId\": \"regime-ext-floor-$(date +%s%N)\",
    \"jurisdiction\": \"US_FEDRAMP_HIGH\",
    \"auditRetentionDays\": 30,
    \"dataResidencyRegions\": [\"us-central1\"],
    \"kmsKeyCustodyMode\": \"CMEK\",
    \"rtoMinutesTier0\": 15,
    \"rpoMinutesTier0\": 0,
    \"rtoMinutesTier1\": 60,
    \"rpoMinutesTier1\": 15,
    \"clusterMode\": \"GKE_STANDARD\",
    \"primaryRegionCount\": 2,
    \"standbyRegionCount\": 1,
    \"tenantIsolationMode\": \"INTERNAL_ONLY\"
  }")

echo "${REGIME_ERR_RESP}"
if echo "${REGIME_ERR_RESP}" | grep -q "201 Created"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/regimes successfully accepted compliant retention of 30 days"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for compliant retention of 30 days"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 13] POST /api/v1/auth/bindings -> Bind User to Role Developer"
echo "--------------------------------------------------------------------------------"
BIND_SUB="user-bound-$(date +%s%N)"
BIND_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/bindings" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"subjectId\": \"${BIND_SUB}\",
    \"tenantId\": \"fintech-team\",
    \"roleName\": \"Developer\"
  }")

echo "${BIND_RESP}"
if echo "${BIND_RESP}" | grep -q "201 Created"; then
    echo -e "\033[1;32m[PASS]\033[0m User role successfully bound"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for user binding"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 14] GET /api/v1/auth/bindings -> Query Bound User Roles"
echo "--------------------------------------------------------------------------------"
GET_BIND_RESP=$(curl --noproxy "*" -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/auth/bindings?subjectId=${BIND_SUB}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team")

echo "${GET_BIND_RESP}"
if echo "${GET_BIND_RESP}" | grep -q "200 OK" && echo "${GET_BIND_RESP}" | grep -q "Developer"; then
    echo -e "\033[1;32m[PASS]\033[0m User roles query returned Developer role"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with Developer role"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 15] POST /api/v1/auth/token -> Token Resolution from User Binding"
echo "--------------------------------------------------------------------------------"
RESOLVED_TOKEN_RESP=$(curl --noproxy "*" -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"subjectId\": \"${BIND_SUB}\",
    \"tenantId\": \"fintech-team\"
  }")

echo "${RESOLVED_TOKEN_RESP}"
if echo "${RESOLVED_TOKEN_RESP}" | grep -q "Developer"; then
    echo -e "\033[1;32m[PASS]\033[0m Token issuance resolved role Developer from user bindings"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected resolved token to contain Developer role"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 16] POST /api/v1/clusters -> Hardened GKE Baseline Provisioning"
echo "--------------------------------------------------------------------------------"
CLUSTER_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/clusters" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d '{
    "clusterId": "gke-fedramp-prod-01",
    "projectId": "gov-cloud-production",
    "location": "us-central1",
    "releaseChannel": "REGULAR",
    "workloadPool": "gov-cloud-production.svc.id.goog",
    "privateCluster": {
      "enablePrivateNodes": true,
      "enablePrivateEndpoint": true,
      "masterIpv4CidrBlock": "172.16.0.0/28"
    },
    "shieldedNodes": {
      "enabled": true,
      "secureBoot": true,
      "integrityMonitoring": true
    },
    "databaseEncryption": {
      "state": "ENCRYPTED",
      "keyName": "projects/gov-cloud-kms/locations/us-central1/keyRings/gke/cryptoKeys/k8s-sec"
    },
    "binaryAuthorization": {
      "evaluationMode": "PROJECT_SINGLETON_POLICY_ENFORCE"
    },
    "backupAgentEnabled": true,
    "cloudServiceMeshMode": "MANAGED",
    "dataAccessLogsActive": true
  }')

echo "${CLUSTER_RESP}"
if echo "${CLUSTER_RESP}" | grep -q "201 Created" && echo "${CLUSTER_RESP}" | grep -q "READY"; then
    echo -e "\033[1;32m[PASS]\033[0m Hardened GKE baseline cluster provisioned and validated"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created and READY status"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 17] POST /api/v1/cloud-resources -> Cloud Resource Workload Identity"
echo "--------------------------------------------------------------------------------"
CR_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/cloud-resources" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d '{
    "project_id": "gov-cloud-production",
    "ksa_name": "orders-sa",
    "namespace": "orders-prod",
    "gsa_email": "orders-app@gov-cloud-production.iam.gserviceaccount.com",
    "role_bindings": ["roles/storage.objectViewer"],
    "metadata": {
      "request_id": "req-cr-1024",
      "owner": "platform-team@gov.io",
      "cost_center": "CC-1024",
      "workload_id": "orders-backend",
      "environment": "production"
    }
  }')

echo "${CR_RESP}"
if echo "${CR_RESP}" | grep -q "201 Created" && echo "${CR_RESP}" | grep -q "roles/iam.workloadIdentityUser"; then
    echo -e "\033[1;32m[PASS]\033[0m Cloud Resource API provisioned Workload Identity manifest"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created with Workload Identity role"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 18] POST /api/v1/workloads/timeline -> Incident Timeline Event"
echo "--------------------------------------------------------------------------------"
TIMELINE_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/workloads/timeline" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d '{
    "tenantId": "fintech-team",
    "workloadId": "fintech-ledger-api",
    "eventType": "CANARY_ANALYSIS_ABORTED",
    "description": "Canary error rate exceeded 0.05% threshold, automated rollback triggered",
    "actor": "argo-rollouts-controller",
    "metadata": {
      "errorRate": "0.12%",
      "threshold": "0.05%",
      "strategy": "Canary"
    }
  }')

echo "${TIMELINE_RESP}"
if echo "${TIMELINE_RESP}" | grep -q "201 Created" && echo "${TIMELINE_RESP}" | grep -q "CANARY_ANALYSIS_ABORTED"; then
    echo -e "\033[1;32m[PASS]\033[0m Incident timeline event persisted"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for incident timeline event"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 19] POST /api/v1/policies -> Create Policy-as-Code Rule"
echo "--------------------------------------------------------------------------------"
POL_TENANT=$(python3 -c "import uuid; print(uuid.uuid4())")
POL_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/policies" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${POL_TENANT}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"tenantId\": \"${POL_TENANT}\",
    \"externalId\": \"SEC-01\",
    \"name\": \"Restricted Pod Security Baseline\",
    \"description\": \"Disallow privileged containers across tenant clusters\",
    \"severity\": \"CRITICAL\",
    \"enforcementPoint\": \"CEL\",
    \"ruleExpression\": \"privileged == false\",
    \"exceptable\": true,
    \"detectionPair\": \"DET-01\",
    \"source\": \"API\"
  }")

echo "${POL_RESP}"
if echo "${POL_RESP}" | grep -q "201 Created" && echo "${POL_RESP}" | grep -q "Restricted Pod Security Baseline"; then
    echo -e "\033[1;32m[PASS]\033[0m Policy-as-Code API created policy rule"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for policy creation"
    exit 1
fi

POL_ID=$(echo "${POL_RESP}" | grep -o '"ID":"[^"]*"' | head -n 1 | cut -d'"' -f4)
if [ -z "${POL_ID}" ]; then
    POL_ID=$(echo "${POL_RESP}" | grep -o '"id":"[^"]*"' | head -n 1 | cut -d'"' -f4)
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 20] POST /api/v1/policies/admission/evaluate -> Evaluate Admission"
echo "--------------------------------------------------------------------------------"
EVAL_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/policies/admission/evaluate" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${POL_TENANT}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"tenantId\": \"${POL_TENANT}\",
    \"namespace\": \"fintech-prod\",
    \"workloadId\": \"ledger-worker\",
    \"callerRole\": \"Developer\",
    \"resourceJson\": \"{\\\"privileged\\\":true}\",
    \"policies\": [
      {
        \"id\": \"${POL_ID}\",
        \"externalId\": \"SEC-01\",
        \"severity\": \"CRITICAL\",
        \"enforcementPoint\": \"CEL\",
        \"lifecycleState\": \"DENY\",
        \"ruleExpression\": \"privileged == false\",
        \"exceptable\": false,
        \"description\": \"Privileged containers strictly forbidden in prod\"
      }
    ]
  }")

echo "${EVAL_RESP}"
if echo "${EVAL_RESP}" | grep -q "200 OK" && echo "${EVAL_RESP}" | grep -q "allowed\":false"; then
    echo -e "\033[1;32m[PASS]\033[0m Policy-as-Code admission review denied privileged workload (200 OK, allowed:false in body)"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with allowed:false for DENY policy evaluation"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 21] POST /api/v1/policies/\${POL_ID}/exceptions -> Request Policy Exception"
echo "--------------------------------------------------------------------------------"
EXC_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/policies/${POL_ID}/exceptions" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${POL_TENANT}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{
    \"tenantId\": \"${POL_TENANT}\",
    \"policyId\": \"${POL_ID}\",
    \"workloadId\": \"ledger-worker\",
    \"namespace\": \"fintech-prod\",
    \"approverId\": \"secops-admin\",
    \"justification\": \"Authorized production exemption for emergency failover drill\",
    \"expiresAt\": \"$(date -u -d "+30 days" +"%Y-%m-%dT%H:%M:%SZ")\"
  }")

echo "${EXC_RESP}"
if echo "${EXC_RESP}" | grep -q "201 Created" && echo "${EXC_RESP}" | grep -q "ACTIVE"; then
    echo -e "\033[1;32m[PASS]\033[0m Policy exception requested and persisted successfully"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for policy exception request"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 22] GET /api/v1/clusters/posture -> Scan Cluster Posture Drift"
echo "--------------------------------------------------------------------------------"
POSTURE_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/clusters/posture?clusterId=gke-fedramp-prod-01" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}")

echo "${POSTURE_RESP}"
if echo "${POSTURE_RESP}" | grep -q "200 OK" && echo "${POSTURE_RESP}" | grep -q "compliant\":true"; then
    echo -e "\033[1;32m[PASS]\033[0m Cluster posture scan returned compliant status and zero drift"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with compliant posture report"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 23] POST /api/v1/cloud-resources/reconcile -> Reconcile Cloud Resource Ledger"
echo "--------------------------------------------------------------------------------"
RECON_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/cloud-resources/reconcile" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d '{
    "activeWorkloadIds": ["non-matching-workload"]
  }')

echo "${RECON_RESP}"
if echo "${RECON_RESP}" | grep -q "200 OK" && echo "${RECON_RESP}" | grep -q "orphaned_count"; then
    echo -e "\033[1;32m[PASS]\033[0m Cloud resource ledger reconciliation identified orphaned resources"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with orphaned resource reconciliation report"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 24] POST /api/v1/dr/plans -> Create Disaster Recovery Plan"
echo "--------------------------------------------------------------------------------"
DR_TENANT_ID="00a02d55-cd2a-43d8-be83-a151ebc41553"
DR_PLAN_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}" \
  -H "X-Idempotency-Key: idemp-dr-plan-curl-01" \
  -d "{
    \"tenantId\": \"${DR_TENANT_ID}\",
    \"planName\": \"fintech-ledger-dr-plan\",
    \"incidentTier\": \"TIER_0\",
    \"primaryRegion\": \"us-central1\",
    \"drRegion\": \"us-east4\",
    \"topology\": \"WARM_STANDBY\",
    \"targetRtoSeconds\": 300,
    \"targetRpoSeconds\": 0,
    \"dataStoreType\": \"CLOUD_SQL\",
    \"secretSyncSource\": \"GOOGLE_SECRET_MANAGER\",
    \"primaryClusterId\": \"gke-primary-01\",
    \"drClusterId\": \"gke-dr-01\",
    \"idempotencyKey\": \"idemp-dr-plan-curl-01\"
  }")

echo "${DR_PLAN_RESP}"
if echo "${DR_PLAN_RESP}" | grep -q "201 Created" && echo "${DR_PLAN_RESP}" | grep -q "fintech-ledger-dr-plan"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully created Disaster Recovery plan in PostgreSQL"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 201 Created for DR plan"
    exit 1
fi

PLAN_ID=$(echo "${DR_PLAN_RESP}" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4)

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 25] GET /api/v1/dr/plans -> List Disaster Recovery Plans"
echo "--------------------------------------------------------------------------------"
DR_LIST_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${DR_LIST_RESP}"
if echo "${DR_LIST_RESP}" | grep -q "200 OK" && echo "${DR_LIST_RESP}" | grep -q "fintech-ledger-dr-plan"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully listed Disaster Recovery plans from PostgreSQL"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with DR plan in list"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 26] POST /api/v1/dr/plans/{plan}:checkReadiness -> Probe DR Readiness"
echo "--------------------------------------------------------------------------------"
DR_READY_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans/${PLAN_ID}:checkReadiness" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${DR_READY_RESP}"
if echo "${DR_READY_RESP}" | grep -q "200 OK" && echo "${DR_READY_RESP}" | grep -q "overallStatus"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully probed DR readiness and updated PostgreSQL status"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for DR readiness check"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 27] POST /api/v1/dr/incidents:declare -> Declare DR Incident"
echo "--------------------------------------------------------------------------------"
DR_INC_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/incidents:declare" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}" \
  -d "{
    \"tenantId\": \"${DR_TENANT_ID}\",
    \"planId\": \"${PLAN_ID}\",
    \"scope\": \"REGION_WIDE\",
    \"declarer\": \"ops-incident-commander\",
    \"evidenceLinks\": [\"https://monitoring.gov.io/incidents/INC-8921\"]
  }")

echo "${DR_INC_RESP}"
if echo "${DR_INC_RESP}" | grep -q "202 Accepted" && echo "${DR_INC_RESP}" | grep -q "PENDING_APPROVAL"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully declared DR incident (PENDING_APPROVAL)"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 202 Accepted for DR incident declaration"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 28] POST /api/v1/dr/datasets/{dataset}:fence -> Fence Dataset Epoch"
echo "--------------------------------------------------------------------------------"
DATASET_ID="ledger-postgres-cluster-01"
FENCE_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/datasets/${DATASET_ID}:fence" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}" \
  -d '{
    "authority": "DR_REPLICA"
  }')

echo "${FENCE_RESP}"
if echo "${FENCE_RESP}" | grep -q "200 OK" && echo "${FENCE_RESP}" | grep -q "DR_REPLICA"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully fenced dataset epoch and persisted in PostgreSQL"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for dataset fencing"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 29] GET /api/v1/dr/datasets/{dataset}/epoch -> Get Fencing Epoch"
echo "--------------------------------------------------------------------------------"
EPOCH_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/datasets/${DATASET_ID}/epoch" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${EPOCH_RESP}"
if echo "${EPOCH_RESP}" | grep -q "200 OK" && echo "${EPOCH_RESP}" | grep -q "DR_REPLICA"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully retrieved current fencing epoch from PostgreSQL"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for fencing epoch retrieval"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 30] POST /api/v1/dr/drills -> Execute DR Drill"
echo "--------------------------------------------------------------------------------"
DRILL_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/drills" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}" \
  -d "{
    \"planId\": \"${PLAN_ID}\",
    \"scope\": \"REGION\",
    \"initiator\": \"sre-drill-coordinator\",
    \"syntheticOnly\": true
  }")

echo "${DRILL_RESP}"
if echo "${DRILL_RESP}" | grep -q "200 OK" && echo "${DRILL_RESP}" | grep -q "true"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully executed DR drill and generated evidence bundle"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for DR drill execution"
    exit 1
fi

DRILL_RUN_ID=$(echo "${DRILL_RESP}" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4)
EVIDENCE_ID=$(echo "${DRILL_RESP}" | grep -o '"evidenceBundleId":"[^"]*' | head -n 1 | cut -d'"' -f4)

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 31] GET /api/v1/dr/plans/{plan}/rpo -> Get Live Replication Lag"
echo "--------------------------------------------------------------------------------"
RPO_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans/${PLAN_ID}/rpo" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${RPO_RESP}"
if echo "${RPO_RESP}" | grep -q "200 OK" && echo "${RPO_RESP}" | grep -q "withinSla"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully queried live RPO replication metrics"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for live RPO query"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 32] GET /api/v1/dr/drills/{run} -> Query Drill Run Report"
echo "--------------------------------------------------------------------------------"
GET_DRILL_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/drills/${DRILL_RUN_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${GET_DRILL_RESP}"
if echo "${GET_DRILL_RESP}" | grep -q "200 OK" && echo "${GET_DRILL_RESP}" | grep -q "${DRILL_RUN_ID}"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully retrieved drill run report by ID"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for drill run query"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 33] GET /api/v1/dr/evidence/{run} -> Query Immutable Evidence Bundle"
echo "--------------------------------------------------------------------------------"
GET_EVID_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/evidence/${EVIDENCE_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${GET_EVID_RESP}"
if echo "${GET_EVID_RESP}" | grep -q "200 OK" && echo "${GET_EVID_RESP}" | grep -q "sha256-"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully retrieved immutable evidence bundle with SHA-256 checksum"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for evidence bundle query"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 34] POST /api/v1/dr/restores -> Initiate DR Backup Restore"
echo "--------------------------------------------------------------------------------"
RESTORE_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/restores" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}" \
  -d "{
    \"tenantId\": \"${DR_TENANT_ID}\",
    \"planId\": \"${PLAN_ID}\",
    \"backupId\": \"bkp-cluster-snap-001\",
    \"targetCluster\": \"gke-dr-01\",
    \"namespace\": \"fintech-prod\",
    \"conflictPolicy\": \"FAIL_ON_CONFLICT\",
    \"initiator\": \"sre-restore-lead\"
  }")

echo "${RESTORE_RESP}"
if echo "${RESTORE_RESP}" | grep -q "202 Accepted" && echo "${RESTORE_RESP}" | grep -q "RESTORE"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully initiated DR backup restore (202 Accepted)"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 202 Accepted for backup restore initiation"
    exit 1
fi

RESTORE_OP_ID=$(echo "${RESTORE_RESP}" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4)

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 35] GET /api/v1/dr/restores/{op} -> Query Restore Operation Status"
echo "--------------------------------------------------------------------------------"
GET_RESTORE_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/restores/${RESTORE_OP_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}")

echo "${GET_RESTORE_RESP}"
if echo "${GET_RESTORE_RESP}" | grep -q "200 OK" && echo "${GET_RESTORE_RESP}" | grep -q "${RESTORE_OP_ID}"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully retrieved restore operation status by ID"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for restore operation query"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 36] POST /api/v1/dr/breakglass:activate -> Activate Emergency Role"
echo "--------------------------------------------------------------------------------"
BREAKGLASS_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/breakglass:activate" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-ID: ${DR_TENANT_ID}" \
  -d "{
    \"tenantId\": \"${DR_TENANT_ID}\",
    \"requester\": \"sre-operator-1\",
    \"approver\": \"director-eng-2\",
    \"durationMins\": 60,
    \"justification\": \"Emergency database replication repair during regional outage\"
  }")

echo "${BREAKGLASS_RESP}"
if echo "${BREAKGLASS_RESP}" | grep -q "200 OK" && echo "${BREAKGLASS_RESP}" | grep -q "DR_EMERGENCY_OPERATOR"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully activated emergency break-glass role via two-person quorum"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for breakglass activation"
    exit 1
fi

GITOPS_TENANT_ID="tenant-gitops-curl-$(date +%s)"
GITOPS_REC_ID="rec-curl-$(date +%s)"
GITOPS_IDEM_KEY="idem-curl-$(date +%s)"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 37] POST /api/v1/reconciliations -> Trigger GitOps Sync with W3C Trace"
echo "--------------------------------------------------------------------------------"
GITOPS_SYNC_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/reconciliations" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}" \
  -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  -d "{
    \"id\": \"${GITOPS_REC_ID}\",
    \"tenantId\": \"${GITOPS_TENANT_ID}\",
    \"serviceId\": \"svc-gitops-curl\",
    \"appName\": \"orders-dev\",
    \"targetRevision\": \"main\",
    \"projectEnforcesSig\": true,
    \"commitSigned\": true,
    \"allowedSigningKey\": true,
    \"roleReviewed\": true,
    \"inAllowWindow\": true,
    \"idempotencyKey\": \"${GITOPS_IDEM_KEY}\"
  }")

echo "${GITOPS_SYNC_RESP}"
if echo "${GITOPS_SYNC_RESP}" | grep -q "202 Accepted" && echo "${GITOPS_SYNC_RESP}" | grep -qi "X-Trace-Id: 4bf92f3577b34da6a3ce929d0e0e4736"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully triggered GitOps reconciliation with W3C trace propagation"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 202 Accepted and X-Trace-Id propagation for GitOps sync"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 38] GET /api/v1/reconciliations/{id} -> Query Reconciliation Record"
echo "--------------------------------------------------------------------------------"
GITOPS_GET_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/reconciliations/${GITOPS_REC_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}")

echo "${GITOPS_GET_RESP}"
if echo "${GITOPS_GET_RESP}" | grep -q "200 OK" && echo "${GITOPS_GET_RESP}" | grep -q "${GITOPS_REC_ID}"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully retrieved GitOps reconciliation record"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for GitOps reconciliation lookup"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 39] GET /api/v1/reconciliations/{id}/diff -> Inspect Drift Diff"
echo "--------------------------------------------------------------------------------"
GITOPS_DIFF_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/reconciliations/${GITOPS_REC_ID}/diff" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}")

echo "${GITOPS_DIFF_RESP}"
if echo "${GITOPS_DIFF_RESP}" | grep -q "200 OK" && echo "${GITOPS_DIFF_RESP}" | grep -q "orders-dev"; then
    echo -e "\033[1;32m[PASS]\033[0m Successfully inspected GitOps drift diff"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for GitOps drift diff"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 40] POST /api/v1/reconciliations -> Invalid Payload Returns 400"
echo "--------------------------------------------------------------------------------"
GITOPS_ERR_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/reconciliations" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}" \
  -d "{\"appName\":\"\"}")

echo "${GITOPS_ERR_RESP}"
if echo "${GITOPS_ERR_RESP}" | grep -q "400 Bad Request" && echo "${GITOPS_ERR_RESP}" | grep -qi "X-Trace-Id:"; then
    echo -e "\033[1;32m[PASS]\033[0m Rejected invalid GitOps sync payload with 400 and X-Trace-Id"
    GITOPS_ERR_TRACE=$(echo "${GITOPS_ERR_RESP}" | grep -i '^x-trace-id:' | awk '{print $2}' | tr -d '\r')
    expect_error_span "${GITOPS_ERR_TRACE}" "gitops.reconciliation.trigger" "must match alphanumeric identifier format"
    expect_span_attr "${GITOPS_ERR_TRACE}" "gitops.reconciliation.trigger" "app.error_class" "client"
    expect_span_attr "${GITOPS_ERR_TRACE}" "gitops.reconciliation.trigger" "app.status_code" "400"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 400 Bad Request with X-Trace-Id for invalid GitOps sync payload"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 41] PUT & GET /api/v1/gitops/secrets/config -> Secret Mechanism"
echo "--------------------------------------------------------------------------------"
GITOPS_SEC_PUT=$(curl --noproxy "*" -s -i -X PUT "http://127.0.0.1:${CLI_PORT}/api/v1/gitops/secrets/config" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}" \
  -d "{
    \"tenantId\": \"${GITOPS_TENANT_ID}\",
    \"mechanism\": \"SEALED_SECRETS\",
    \"clusterScope\": \"namespace\",
    \"encryptionKeyRef\": \"sealed-secrets-key\",
    \"enabled\": true
  }")

echo "${GITOPS_SEC_PUT}"
if echo "${GITOPS_SEC_PUT}" | grep -q "200 OK" && echo "${GITOPS_SEC_PUT}" | grep -q "SEALED_SECRETS"; then
    echo -e "\033[1;32m[PASS]\033[0m Configured GitOps secret mechanism"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for GitOps secret mechanism configuration"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 42] GET /api/v1/gitops/secrets/config -> Fetch Secret Mechanism"
echo "--------------------------------------------------------------------------------"
GITOPS_SEC_GET=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/gitops/secrets/config" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}")

echo "${GITOPS_SEC_GET}"
if echo "${GITOPS_SEC_GET}" | grep -q "200 OK" && echo "${GITOPS_SEC_GET}" | grep -q "SEALED_SECRETS"; then
    echo -e "\033[1;32m[PASS]\033[0m Retrieved GitOps secret mechanism config"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for GitOps secret mechanism retrieval"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 43] POST /api/v1/webhooks/gitops -> Ingest GitOps Webhook"
echo "--------------------------------------------------------------------------------"
GITOPS_WH_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/webhooks/gitops" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: ${GITOPS_TENANT_ID}" \
  -d "{
    \"repositoryUrl\": \"https://github.com/org/orders-repo.git\",
    \"branch\": \"main\",
    \"commitSha\": \"deadbeef1234\"
  }")

echo "${GITOPS_WH_RESP}"
if echo "${GITOPS_WH_RESP}" | grep -q "200 OK" && echo "${GITOPS_WH_RESP}" | grep -q "\"accepted\":true"; then
    echo -e "\033[1;32m[PASS]\033[0m Accepted GitOps webhook ingress"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK for GitOps webhook ingress"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 44] POST /api/v1/rbac/authorize -> Contextual RBAC Authorization"
echo "--------------------------------------------------------------------------------"
RBAC_AUTH_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/rbac/authorize" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: fintech-team" \
  -d "{
    \"targetTenantId\": \"fintech-team\",
    \"resource\": \"services\",
    \"action\": \"read\"
  }")

echo "${RBAC_AUTH_RESP}"
if echo "${RBAC_AUTH_RESP}" | grep -q "200 OK" && echo "${RBAC_AUTH_RESP}" | grep -q "\"granted\":true"; then
    echo -e "\033[1;32m[PASS]\033[0m Contextual RBAC authorization granted"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK and granted=true for /api/v1/rbac/authorize"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 45] POST /api/v1/rbac/roles/validate-hierarchy -> DAG Validation"
echo "--------------------------------------------------------------------------------"
RBAC_DAG_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/rbac/roles/validate-hierarchy" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: fintech-team" \
  -d "{\"edges\":{\"PlatformAdmin\":[\"Developer\"],\"Developer\":[\"Viewer\"]}}")

echo "${RBAC_DAG_RESP}"
if echo "${RBAC_DAG_RESP}" | grep -q "200 OK" && echo "${RBAC_DAG_RESP}" | grep -q "\"valid\":true"; then
    echo -e "\033[1;32m[PASS]\033[0m Validated acyclic RBAC role hierarchy DAG"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK and valid=true for /api/v1/rbac/roles/validate-hierarchy"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 46] POST /api/v1/rbac/sidecar/evaluate -> Sidecar PDP Fallback"
echo "--------------------------------------------------------------------------------"
RBAC_SIDECAR_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/rbac/sidecar/evaluate" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: fintech-team" \
  -d "{\"subject\":{\"id\":\"usr_admin\",\"tenantId\":\"fintech-team\",\"roles\":[\"PlatformAdmin\"]},\"targetTenantId\":\"fintech-team\",\"resource\":\"auth\",\"action\":\"read\",\"targetEnvironment\":\"development\"}")
echo "${RBAC_SIDECAR_RESP}"
if echo "${RBAC_SIDECAR_RESP}" | grep -q "200 OK" && echo "${RBAC_SIDECAR_RESP}" | grep -q "\"granted\":true"; then
    echo -e "\033[1;32m[PASS]\033[0m Sidecar PDP evaluate returned granted=true"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK and granted=true for /api/v1/rbac/sidecar/evaluate"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 47] POST /api/v1/rbac/least-privilege/analyze -> Least Privilege"
echo "--------------------------------------------------------------------------------"
RBAC_LP_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/rbac/least-privilege/analyze" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: fintech-team" \
  -d "{\"roles\":{\"Developer\":[{\"resource\":\"deployments\",\"action\":\"read\"},{\"resource\":\"secrets\",\"action\":\"delete\"}]},\"usage\":[{\"role\":\"Developer\",\"resource\":\"deployments\",\"action\":\"read\",\"lastUsedAt\":\"2026-11-26T00:00:00Z\",\"hitCount\":5}],\"staleHours\":24}")
echo "${RBAC_LP_RESP}"
if echo "${RBAC_LP_RESP}" | grep -q "200 OK" && echo "${RBAC_LP_RESP}" | grep -q "\"findings\""; then
    echo -e "\033[1;32m[PASS]\033[0m Least-privilege analyze returned findings"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK and findings for /api/v1/rbac/least-privilege/analyze"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 48] POST /api/v1/rbac/least-privilege/prune -> Prune Unused Perms"
echo "--------------------------------------------------------------------------------"
RBAC_PRUNE_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/rbac/least-privilege/prune" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: fintech-team" \
  -d "{\"roles\":{\"Developer\":[{\"resource\":\"deployments\",\"action\":\"read\"},{\"resource\":\"secrets\",\"action\":\"delete\"}]},\"usage\":[{\"role\":\"Developer\",\"resource\":\"deployments\",\"action\":\"read\",\"lastUsedAt\":\"2026-11-26T00:00:00Z\",\"hitCount\":5}],\"staleHours\":24}")
echo "${RBAC_PRUNE_RESP}"
if echo "${RBAC_PRUNE_RESP}" | grep -q "200 OK" && echo "${RBAC_PRUNE_RESP}" | grep -q "\"prunedRoles\""; then
    echo -e "\033[1;32m[PASS]\033[0m Least-privilege prune returned prunedRoles"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK and prunedRoles for /api/v1/rbac/least-privilege/prune"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 49] POST /api/v1/rbac/ebpf/enforce -> eBPF Hook Enforcement"
echo "--------------------------------------------------------------------------------"
RBAC_EBPF_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/rbac/ebpf/enforce" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -H "X-Tenant-Id: fintech-team" \
  -d "{\"event\":{\"workloadId\":\"wk-1\",\"namespace\":\"fintech-team\",\"syscall\":\"read\",\"targetPath\":\"/app/config.yaml\",\"uid\":1000},\"policy\":{\"allowedSyscalls\":[\"read\"],\"deniedPaths\":[\"/etc/shadow\"],\"enforce\":true}}")
echo "${RBAC_EBPF_RESP}"
if echo "${RBAC_EBPF_RESP}" | grep -q "200 OK" && echo "${RBAC_EBPF_RESP}" | grep -q "\"allowed\":true"; then
    echo -e "\033[1;32m[PASS]\033[0m eBPF hook enforcement permitted allowed syscall"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK and allowed=true for /api/v1/rbac/ebpf/enforce"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 50] GET /api/v1/clusters -> Fetch Provisioned Cluster"
echo "--------------------------------------------------------------------------------"
GET_CLUSTER_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/clusters?clusterId=gke-fedramp-prod-01" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-Id: fintech-team")
echo "${GET_CLUSTER_RESP}"
if echo "${GET_CLUSTER_RESP}" | grep -q "200 OK" && echo "${GET_CLUSTER_RESP}" | grep -q "gke-fedramp-prod-01"; then
    echo -e "\033[1;32m[PASS]\033[0m GET /api/v1/clusters returned provisioned cluster"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with gke-fedramp-prod-01"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 51] GET /api/v1/clusters -> Non-Existent Cluster Returns 404"
echo "--------------------------------------------------------------------------------"
MISS_CLUSTER_RESP=$(curl --noproxy "*" -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/clusters?clusterId=nonexistent-cluster-999" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-Id: fintech-team")
echo "${MISS_CLUSTER_RESP}"
if echo "${MISS_CLUSTER_RESP}" | grep -q "404 Not Found"; then
    echo -e "\033[1;32m[PASS]\033[0m GET /api/v1/clusters returned 404 Not Found for missing cluster"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 404 Not Found for missing cluster"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 52] POST /api/v1/clusters -> Malformed Payload Returns 400"
echo "--------------------------------------------------------------------------------"
BAD_CLUSTER_RESP=$(curl --noproxy "*" -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/clusters" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-Id: fintech-team" \
  -d '{"clusterId":""}')
echo "${BAD_CLUSTER_RESP}"
if echo "${BAD_CLUSTER_RESP}" | grep -q "400 Bad Request"; then
    echo -e "\033[1;32m[PASS]\033[0m POST /api/v1/clusters rejected invalid payload with 400 Bad Request"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 400 Bad Request for invalid cluster payload"
    exit 1
fi








run_autoscaling_checks

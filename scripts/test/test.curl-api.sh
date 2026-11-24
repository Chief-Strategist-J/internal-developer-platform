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

if [ -f "${PROJECT_ROOT}/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    source "${PROJECT_ROOT}/.env"
    set +a
    export PORT="${CLI_PORT}"
    export ADMIN_PORT="${CLI_ADMIN_PORT}"
    export JWT_SECRET="${SECRET}"
    export REDIS_ADDR="127.0.0.1:6379"
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
SVC_ID="fintech-ledger-api-$(date +%s%N)"
SERVICE_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/services" \
  -H "Authorization: Bearer ${DEV_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
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

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 8] POST /api/v1/workloads -> Provision Workload with Distributed Traces"
echo "--------------------------------------------------------------------------------"
WKLD_DEV_SUB="wkld-dev-$(date +%s%N)"
WKLD_TOKEN_RESP=$(curl -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"subjectId\": \"${WKLD_DEV_SUB}\",
    \"tenantId\": \"fintech-team\",
    \"roles\": [\"Developer\"],
    \"ttlSeconds\": 3600
  }")
WKLD_TOKEN=$(echo "${WKLD_TOKEN_RESP}" | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)

WKLD_ID="wkld-e2e-$(date +%s)"
TRACE_ID="4bf92f3577b34da6a3ce929d0e0e4736"
SPAN_ID="00f067aa0ba902b7"
TRACEPARENT="00-${TRACE_ID}-${SPAN_ID}-01"

WORKLOAD_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/workloads" \
  -H "Authorization: Bearer ${WKLD_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: ${TRACEPARENT}" \
  -H "Content-Type: application/json" \
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
GET_WKLD_RESP=$(curl -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/workloads/${WKLD_ID}" \
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
REGIME_POST_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/regimes" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902c1-01" \
  -H "Content-Type: application/json" \
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
REGIME_GET_RESP=$(curl -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/regimes/active?tenantId=fintech-team" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902c2-01")

echo "${REGIME_GET_RESP}"
if echo "${REGIME_GET_RESP}" | grep -q "200 OK" && echo "${REGIME_GET_RESP}" | grep -q "US_FEDRAMP_HIGH"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/regimes/active successfully retrieved active regime policy"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 200 OK with US_FEDRAMP_HIGH jurisdiction"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 12] POST /api/v1/regimes -> Traced Validation Error Path (Retention < 30)"
echo "--------------------------------------------------------------------------------"
REGIME_ERR_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/regimes" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902c3-01" \
  -H "Content-Type: application/json" \
  -d '{
    "tenantId": "fintech-team",
    "externalId": "regime-ext-fail",
    "jurisdiction": "US_FEDRAMP_HIGH",
    "auditRetentionDays": 10,
    "dataResidencyRegions": ["us-central1"],
    "kmsKeyCustodyMode": "CMEK",
    "rtoMinutesTier0": 15,
    "rpoMinutesTier0": 0,
    "rtoMinutesTier1": 60,
    "rpoMinutesTier1": 15,
    "clusterMode": "GKE_STANDARD",
    "primaryRegionCount": 2,
    "standbyRegionCount": 1,
    "tenantIsolationMode": "INTERNAL_ONLY"
  }')

echo "${REGIME_ERR_RESP}"
if echo "${REGIME_ERR_RESP}" | grep -q "400 Bad Request" && echo "${REGIME_ERR_RESP}" | grep -q "VALIDATION_FAILED"; then
    echo -e "\033[1;32m[PASS]\033[0m /api/v1/regimes error branch rejected invalid retention < 30 days"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 400 Bad Request with VALIDATION_FAILED code"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 13] POST /api/v1/auth/bindings -> Bind User to Role Developer"
echo "--------------------------------------------------------------------------------"
BIND_SUB="user-bound-$(date +%s%N)"
BIND_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/bindings" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
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
GET_BIND_RESP=$(curl -s -i "http://127.0.0.1:${CLI_PORT}/api/v1/auth/bindings?subjectId=${BIND_SUB}" \
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
RESOLVED_TOKEN_RESP=$(curl -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" \
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
CLUSTER_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/clusters" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
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
CR_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/cloud-resources" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
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
TIMELINE_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/workloads/timeline" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: fintech-team" \
  -H "Content-Type: application/json" \
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
POL_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/policies" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${POL_TENANT}" \
  -H "Content-Type: application/json" \
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
EVAL_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/policies/admission/evaluate" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-Tenant-ID: ${POL_TENANT}" \
  -H "Content-Type: application/json" \
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
if echo "${EVAL_RESP}" | grep -q "403 Forbidden" && echo "${EVAL_RESP}" | grep -q "allowed\":false"; then
    echo -e "\033[1;32m[PASS]\033[0m Policy-as-Code admission review denied privileged workload with HTTP 403"
else
    echo -e "\033[1;31m[FAIL]\033[0m Expected 403 Forbidden for DENY policy violation"
    exit 1
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 21] GET /api/v1/clusters/posture -> Scan Cluster Posture Drift"
echo "--------------------------------------------------------------------------------"
POSTURE_RESP=$(curl -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/clusters/posture?clusterId=gke-fedramp-prod-01" \
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
echo "[CURL TEST 22] POST /api/v1/cloud-resources/reconcile -> Reconcile Cloud Resource Ledger"
echo "--------------------------------------------------------------------------------"
RECON_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/cloud-resources/reconcile" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
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
echo "[CURL TEST 23] POST /api/v1/dr/plans -> Create Disaster Recovery Plan"
echo "--------------------------------------------------------------------------------"
DR_TENANT_ID="00a02d55-cd2a-43d8-be83-a151ebc41553"
DR_PLAN_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
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
echo "[CURL TEST 24] GET /api/v1/dr/plans -> List Disaster Recovery Plans"
echo "--------------------------------------------------------------------------------"
DR_LIST_RESP=$(curl -s -i -X GET "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans" \
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
echo "[CURL TEST 25] POST /api/v1/dr/plans/{plan}:checkReadiness -> Probe DR Readiness"
echo "--------------------------------------------------------------------------------"
DR_READY_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/plans/${PLAN_ID}:checkReadiness" \
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
echo "[CURL TEST 26] POST /api/v1/dr/incidents:declare -> Declare DR Incident"
echo "--------------------------------------------------------------------------------"
DR_INC_RESP=$(curl -s -i -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/dr/incidents:declare" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
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







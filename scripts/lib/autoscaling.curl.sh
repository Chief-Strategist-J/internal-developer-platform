#!/usr/bin/env bash
# Real-HTTP checks for the autoscaling feature: curl -> RBAC -> handler -> service -> Tempo trace -> Postgres audit.
# No mocks: every check hits the running server and the real Postgres, Redis and Tempo behind it.
# Required in the caller: CLI_PORT, ADMIN_TOKEN, RBAC_BOOTSTRAP_SECRET. Optional: DATABASE_URL, TEMPO_URL.
# AS_STRICT=1 turns every SKIP into a FAIL (CI sets it so nothing is silently skipped).

as_pass() { echo -e "\033[1;32m[PASS]\033[0m $1"; }
as_fail() { echo -e "\033[1;31m[FAIL]\033[0m $1"; exit 1; }

as_skip() {
    if [[ "${AS_STRICT:-0}" == "1" ]]; then
        echo -e "\033[1;31m[FAIL]\033[0m $1 (AS_STRICT=1 forbids skipping)"; exit 1
    fi
    echo -e "\033[1;33m[SKIP]\033[0m $1"
}

# expect_error_span <trace-id> <span-name> <message-fragment>
# Negative tests must fail loudly in the trace too (v3.0 VIII.1): the rejection span has to carry
# status ERROR and the expected message. This turns a red span in Tempo into a verified expectation.
expect_error_span() {
    local trace_id="$1" span_name="$2" fragment="$3" url="${TEMPO_URL:-http://127.0.0.1:3200}" body=""
    if ! curl --noproxy "*" -s -o /dev/null "${url}/ready" 2>/dev/null; then
        as_skip "Tempo not reachable at ${url}; cannot verify expected error span '${span_name}'"
        return 0
    fi
    for _ in {1..20}; do
        body=$(curl --noproxy "*" -s "${url}/api/traces/${trace_id}" || true)
        if echo "${body}" | jq -e --arg n "${span_name}" --arg m "${fragment}" \
            '[.batches[]?.scopeSpans[]?.spans[]? | select(.name==$n and .status.code=="STATUS_CODE_ERROR" and ((.status.message // "") | contains($m)))] | length > 0' >/dev/null 2>&1; then
            as_pass "Expected error span '${span_name}' (${fragment}) recorded in trace ${trace_id}"
            return 0
        fi
        sleep 1
    done
    as_fail "Expected error span '${span_name}' containing '${fragment}' not found in Tempo trace ${trace_id}"
}

# expect_span_attr <trace-id> <span-name> <attribute-key> <value-fragment>
# Asserts a span in Tempo carries an explanatory attribute (why it failed / what was decided).
expect_span_attr() {
    local trace_id="$1" span_name="$2" key="$3" fragment="$4" url="${TEMPO_URL:-http://127.0.0.1:3200}" body=""
    if ! curl --noproxy "*" -s -o /dev/null "${url}/ready" 2>/dev/null; then
        as_skip "Tempo not reachable at ${url}; cannot verify attribute ${key} on '${span_name}'"
        return 0
    fi
    for _ in {1..20}; do
        body=$(curl --noproxy "*" -s "${url}/api/traces/${trace_id}" || true)
        if echo "${body}" | jq -e --arg n "${span_name}" --arg k "${key}" --arg v "${fragment}" \
            '[.batches[]?.scopeSpans[]?.spans[]? | select(.name==$n) | .attributes[]? | select(.key==$k and ((.value | tostring) | contains($v)))] | length > 0' >/dev/null 2>&1; then
            as_pass "Span '${span_name}' carries ${key}=${fragment} in trace ${trace_id}"
            return 0
        fi
        sleep 1
    done
    as_fail "Span '${span_name}' lacks attribute ${key} containing '${fragment}' in Tempo trace ${trace_id}"
}

run_autoscaling_checks() {

AS_URL="http://127.0.0.1:${CLI_PORT}/api/v1/autoscaling/plans:evaluate"
AS_RUN="as$(date +%s%N)"
TEMPO_URL="${TEMPO_URL:-http://127.0.0.1:3200}"


AS_PLAN_OK='{"plan":{"workload":{"namespace":"app-prod","name":"api","kind":"Rollout"},"environment":"PRODUCTION","tier":"gold","replicaWriters":["HPA"],"minReplicas":3,"maxReplicas":10,"zones":3,"topologySpreadDeclared":true,"synchronousApi":true,"horizontalMetrics":["cpu"],"requests":{"cpuMilli":"250","memoryBytes":"268435456"},"vertical":{"mode":"RECOMMEND_ONLY","metrics":["memory"]},"rolloutSurgePods":2,"canaryPods":1},"policy":{"tier":"gold","minReplicasFloor":3,"minZones":3,"topologySpread":true,"tierCritical":true},"ceilings":{"quotaCpuMilli":"10000","quotaPods":"20","nodePoolMaxPods":"40","ipHeadroomPods":"60","budgetMaxPods":"80"},"signal":{"state":"AVAILABLE","currentReplicas":5,"proposedReplicas":4},"cluster":{"spot":{"standardNonGpuPoolPresent":true},"reservation":{"reserved":true,"fallbackClassesListed":true},"ip":{"nodePoolMaxNodes":"10","podsPerNodeRange":"32","podRangeSize":"1024"},"disruption":{"allowedDisruptions":1,"unhealthyPodEviction":"AlwaysAllow"},"canary":{"enabled":true,"analysisPerRevision":true,"trafficRouting":true},"budget":{"currency":"USD","projectedMaxSpendMinor":"1000","tenantBudgetMinor":"2000"},"scalingEventsRetained":true,"database":{},"ingestion":{}}}'
AS_PLAN_BAD=$(echo "${AS_PLAN_OK}" | sed 's/"replicaWriters":\["HPA"\]/"replicaWriters":["HPA","KEDA"]/; s/"maxReplicas":10/"maxReplicas":50/')

as_post() {
    local token="$1" key="$2" body="$3"; shift 3
    curl --noproxy "*" -s -i -X POST "${AS_URL}" \
      -H "Authorization: Bearer ${token}" \
      -H "Content-Type: application/json" \
      -H "x-tenant-id: fintech-team" \
      -H "x-request-id: ${AS_RUN}-req" \
      -H "x-correlation-id: ${AS_RUN}-corr" \
      -H "x-client-id: curl-autoscaling" \
      -H "x-idempotency-key: ${AS_RUN}-${key}" \
      "$@" -d "${body}"
}

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 53] POST /api/v1/autoscaling/plans:evaluate -> Compliant Plan Is Allowed (200)"
echo "--------------------------------------------------------------------------------"
AS_OK_RESP=$(as_post "${ADMIN_TOKEN}" "ok" "${AS_PLAN_OK}")
echo "${AS_OK_RESP}"
echo "${AS_OK_RESP}" | grep -q "200 OK" || as_fail "Expected 200 OK for compliant plan"
echo "${AS_OK_RESP}" | grep -q '"allowed":true' || as_fail "Expected allowed=true"
echo "${AS_OK_RESP}" | grep -q '"violations":\[\]' || as_fail "Expected violations to serialize as an empty array"
echo "${AS_OK_RESP}" | grep -qi "x-request-id: ${AS_RUN}-req" || as_fail "x-request-id was not echoed"
echo "${AS_OK_RESP}" | grep -q '"apiVersion":"v1"' || as_fail "Envelope meta.apiVersion missing"
as_pass "Compliant plan allowed with canonical envelope, echoed headers and empty violations array"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 54] POST /api/v1/autoscaling/plans:evaluate -> Two Writers And Over-Ceiling Plan Denied"
echo "--------------------------------------------------------------------------------"
AS_BAD_RESP=$(as_post "${ADMIN_TOKEN}" "bad" "${AS_PLAN_BAD}")
echo "${AS_BAD_RESP}"
echo "${AS_BAD_RESP}" | grep -q '"allowed":false' || as_fail "Expected allowed=false"
for code in MULTIPLE_REPLICA_WRITERS MAX_SCALE_EXCEEDS_POD_QUOTA MAX_SCALE_EXCEEDS_CPU_QUOTA; do
    echo "${AS_BAD_RESP}" | grep -q "\"${code}\"" || as_fail "Missing violation ${code}"
done
AS_BAD_TRACE=$(echo "${AS_BAD_RESP}" | grep -i '^x-trace-id:' | awk '{print $2}' | tr -d '\r')
expect_span_attr "${AS_BAD_TRACE}" "autoscaling.service.evaluate_plan" "autoscaling.violation.codes" "MULTIPLE_REPLICA_WRITERS"
expect_span_attr "${AS_BAD_TRACE}" "autoscaling.api.evaluate_plan" "autoscaling.outcome" "denied"
as_pass "AS-INV-01 and AS-INV-03 violations returned and explained in the trace (Phase 0 exit gate)"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 55] POST /api/v1/autoscaling/plans:evaluate -> Data & Ingestion Invariants Denied"
echo "--------------------------------------------------------------------------------"
AS_DATA_BODY=$(echo "${AS_PLAN_OK}" | sed 's/"database":{}/"database":{"talksToDatabase":true,"perPodPoolSize":20,"dbConnectionBudget":100,"storageAutoIncreaseLimit":"0","usesTransactionPooler":true,"requiresSessionState":true}/; s/"ingestion":{}/"ingestion":{"backbone":"KAFKA","topicPartitions":4,"consumerMaxReplicas":8}/')
AS_DATA_RESP=$(as_post "${ADMIN_TOKEN}" "data" "${AS_DATA_BODY}")
echo "${AS_DATA_RESP}"
for code in DB_CONNECTION_CEILING_EXCEEDED UNBOUNDED_STORAGE_GROWTH TRANSACTION_POOLING_INCOMPATIBLE KAFKA_REPLICAS_EXCEED_PARTITIONS; do
    echo "${AS_DATA_RESP}" | grep -q "\"${code}\"" || as_fail "Missing violation ${code}"
done
as_pass "AS-INV-16, 18, 19 and 21 violations returned"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 56] POST /api/v1/autoscaling/plans:evaluate -> No Token Returns 401"
echo "--------------------------------------------------------------------------------"
AS_NOAUTH_CODE=$(curl --noproxy "*" -s -o /dev/null -w "%{http_code}" -X POST "${AS_URL}" \
  -H "Content-Type: application/json" -H "x-tenant-id: fintech-team" \
  -H "x-idempotency-key: ${AS_RUN}-noauth" -d "${AS_PLAN_OK}")
[[ "${AS_NOAUTH_CODE}" == "401" ]] || as_fail "Expected 401, got ${AS_NOAUTH_CODE}"
as_pass "Unauthenticated request rejected with 401"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 57] POST /api/v1/autoscaling/plans:evaluate -> Developer Lacks infrastructure:scale (403)"
echo "--------------------------------------------------------------------------------"
AS_DEV_TOKEN=$(curl --noproxy "*" -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
  -H "Content-Type: application/json" -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
  -d "{\"subjectId\":\"${AS_RUN}-dev\",\"tenantId\":\"fintech-team\",\"roles\":[\"Developer\"],\"ttlSeconds\":3600}" \
  | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)
[[ -n "${AS_DEV_TOKEN}" ]] || as_fail "Could not issue a fresh Developer token"
AS_DEV_RESP=$(as_post "${AS_DEV_TOKEN}" "dev" "${AS_PLAN_OK}")
echo "${AS_DEV_RESP}"
echo "${AS_DEV_RESP}" | grep -q "403 Forbidden" || as_fail "Expected 403 Forbidden for Developer"
as_pass "RBAC denied the Developer role"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 58] POST /api/v1/autoscaling/plans:evaluate -> Closed Schema: Unknown Field Returns 400"
echo "--------------------------------------------------------------------------------"
AS_UNKNOWN_RESP=$(as_post "${ADMIN_TOKEN}" "unknown" '{"plan":{},"unexpected":true}')
echo "${AS_UNKNOWN_RESP}"
echo "${AS_UNKNOWN_RESP}" | grep -q "400 Bad Request" || as_fail "Expected 400 for unknown field"
echo "${AS_UNKNOWN_RESP}" | grep -q '"INVALID_PAYLOAD"' || as_fail "Expected INVALID_PAYLOAD error code"
as_pass "Unknown field rejected (additionalProperties: false)"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 59] POST /api/v1/autoscaling/plans:evaluate -> Tenant Only From Header (422 Without It)"
echo "--------------------------------------------------------------------------------"
AS_NOTENANT_RESP=$(curl --noproxy "*" -s -i -X POST "${AS_URL}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H "Content-Type: application/json" \
  -H "x-idempotency-key: ${AS_RUN}-notenant" -d "${AS_PLAN_OK}")
AS_NOTENANT_CODE=$(echo "${AS_NOTENANT_RESP}" | head -1 | awk '{print $2}')
[[ "${AS_NOTENANT_CODE}" == "422" ]] || as_fail "Expected 422 without x-tenant-id, got ${AS_NOTENANT_CODE}"
AS_NOTENANT_TRACE=$(echo "${AS_NOTENANT_RESP}" | grep -i '^x-trace-id:' | awk '{print $2}' | tr -d '\r')
as_pass "Missing tenant context rejected with 422"
expect_error_span "${AS_NOTENANT_TRACE}" "autoscaling.service.evaluate_plan" "tenant context is required"
expect_span_attr "${AS_NOTENANT_TRACE}" "autoscaling.service.evaluate_plan" "autoscaling.validation.reason" "tenant context is required"
expect_span_attr "${AS_NOTENANT_TRACE}" "autoscaling.service.evaluate_plan" "app.error_class" "client"
expect_span_attr "${AS_NOTENANT_TRACE}" "autoscaling.api.evaluate_plan" "autoscaling.outcome" "validation_failed"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 60] POST /api/v1/autoscaling/plans:evaluate -> Idempotency Replay And Key Reuse Conflict"
echo "--------------------------------------------------------------------------------"
AS_REPLAY_RESP=$(as_post "${ADMIN_TOKEN}" "bad" "${AS_PLAN_BAD}")
echo "${AS_REPLAY_RESP}" | grep -q "200 OK" || as_fail "Expected replay of the same key and body to return 200"
AS_REUSE_RESP=$(as_post "${ADMIN_TOKEN}" "bad" "${AS_PLAN_OK}")
echo "${AS_REUSE_RESP}"
echo "${AS_REUSE_RESP}" | grep -q "422" || as_fail "Expected 422 when a key is reused with a different body"
as_pass "Replay served; same key with different payload rejected"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 61] GET /api/v1/autoscaling/plans:evaluate -> Method Not Allowed (405)"
echo "--------------------------------------------------------------------------------"
AS_GET_CODE=$(curl --noproxy "*" -s -o /dev/null -w "%{http_code}" "${AS_URL}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H "x-tenant-id: fintech-team")
[[ "${AS_GET_CODE}" == "405" ]] || as_fail "Expected 405, got ${AS_GET_CODE}"
as_pass "GET rejected with 405"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[CURL TEST 61b] POST /api/v1/autoscaling/plans:evaluate -> Scale-to-zero, Ingestion Signal And Non-Production Floors"
echo "--------------------------------------------------------------------------------"
AS_Z_BODY=$(echo "${AS_PLAN_OK}" | sed 's/"database":{}/"database":{"talksToDatabase":true,"perPodPoolSize":1,"dbConnectionBudget":100,"storageAutoIncreaseLimit":"500"}/; s/"ingestion":{}/"ingestion":{"backbone":"KAFKA","topicPartitions":12,"consumerMaxReplicas":4,"signal":"ACK_RATE"}/; s/"database":{"talks/"serverless":{"httpScaleToZero":true},"database":{"talks/')
AS_Z_RESP=$(as_post "${ADMIN_TOKEN}" "zero" "${AS_Z_BODY}")
for code in HTTP_SCALE_TO_ZERO_IN_PRODUCTION HTTP_SCALE_TO_ZERO_NO_INTERCEPTOR INGEST_SIGNAL_INVALID INGEST_NO_DEAD_LETTER INGEST_NOT_LIMITED_TO_LAG INGEST_OFFSET_BEHAVIOR_UNDECIDED; do
    echo "${AS_Z_RESP}" | grep -q "\"${code}\"" || { echo "${AS_Z_RESP}"; as_fail "Missing violation ${code}"; }
done
AS_NONPROD_BODY=$(echo "${AS_PLAN_OK}" | sed 's/"environment":"PRODUCTION"/"environment":"NON_PRODUCTION"/; s/"minReplicas":3/"minReplicas":1/; s/"zones":3/"zones":1/')
AS_NONPROD_RESP=$(as_post "${ADMIN_TOKEN}" "nonprod" "${AS_NONPROD_BODY}")
echo "${AS_NONPROD_RESP}" | grep -q '"allowed":true' || { echo "${AS_NONPROD_RESP}"; as_fail "Non-production plan below the tier floor must be allowed"; }
as_pass "AS-INV-22/23 violations returned; tier floors apply only in production"

echo ""
echo "--------------------------------------------------------------------------------"
echo "[TRACE TEST 62] End-to-end W3C trace: curl -> RBAC -> autoscaling handler -> service -> Tempo"
echo "--------------------------------------------------------------------------------"
AS_TRACE_ID=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
AS_TRACE_RESP=$(as_post "${ADMIN_TOKEN}" "trace" "${AS_PLAN_OK}" -H "traceparent: 00-${AS_TRACE_ID}-00f067aa0ba902b7-01")
echo "${AS_TRACE_RESP}" | grep -qi "x-trace-id: ${AS_TRACE_ID}" || as_fail "Server did not continue the caller's trace id"
echo "${AS_TRACE_RESP}" | grep -qi "traceparent: 00-${AS_TRACE_ID}" || as_fail "traceparent was not echoed"
as_pass "Response carries the caller's trace id ${AS_TRACE_ID}"
if curl --noproxy "*" -s -o /dev/null "${TEMPO_URL}/ready" 2>/dev/null; then
    AS_SPANS=""
    for _ in {1..20}; do
        AS_SPANS=$(curl --noproxy "*" -s "${TEMPO_URL}/api/traces/${AS_TRACE_ID}" || true)
        echo "${AS_SPANS}" | grep -q "autoscaling.service.evaluate_plan" && break
        sleep 1
    done
    for span in "POST /api/v1/autoscaling/plans:evaluate" "autoscaling.api.evaluate_plan" "autoscaling.service.evaluate_plan" "rbac.service.authorize"; do
        echo "${AS_SPANS}" | grep -q "${span}" || as_fail "Span '${span}' missing from Tempo trace ${AS_TRACE_ID}"
    done
    as_pass "Tempo holds the full span tree for trace ${AS_TRACE_ID}"
    expect_span_attr "${AS_TRACE_ID}" "autoscaling.service.evaluate_plan" "autoscaling.allowed" "true"
    expect_span_attr "${AS_TRACE_ID}" "autoscaling.service.evaluate_plan" "autoscaling.tier" "gold"
else
    as_skip "Tempo not reachable at ${TEMPO_URL}; set TEMPO_URL (e.g. kubectl -n dev port-forward svc/tempo 3200)"
fi

echo ""
echo "--------------------------------------------------------------------------------"
echo "[DB TEST 63] Postgres audit trail: infrastructure:scale decisions recorded by RBAC outbox"
echo "--------------------------------------------------------------------------------"
if [ -n "${DATABASE_URL:-}" ] && command -v psql >/dev/null 2>&1; then
    AS_PERMITS=$(psql "${DATABASE_URL}" -At -c "SELECT count(*) FROM role_based_access_control.rbac_audit_outbox WHERE resource='infrastructure' AND action='scale' AND decision='PERMIT' AND occurred_at > now() - interval '10 minutes'")
    AS_DENIES=$(psql "${DATABASE_URL}" -At -c "SELECT count(*) FROM role_based_access_control.rbac_audit_outbox WHERE resource='infrastructure' AND action='scale' AND decision='DENY' AND occurred_at > now() - interval '10 minutes'")
    AS_SCHEMA=$(psql "${DATABASE_URL}" -At -c "SELECT count(*) FROM information_schema.schemata WHERE schema_name ILIKE '%autoscal%'")
    echo "permits=${AS_PERMITS} denies=${AS_DENIES} autoscaling_schemas=${AS_SCHEMA}"
    [[ "${AS_PERMITS}" -ge 1 ]] || as_fail "No PERMIT audit row for infrastructure:scale"
    [[ "${AS_DENIES}" -ge 1 ]] || as_fail "No DENY audit row for the Developer role"
    [[ "${AS_SCHEMA}" == "0" ]] || as_fail "Autoscaling must not own a schema while it persists nothing (D-01)"
    as_pass "Audit outbox holds PERMIT and DENY decisions; autoscaling owns no schema"
else
    as_skip "DATABASE_URL or psql unavailable"
fi
}

#!/usr/bin/env bash
# End-to-end verification of the autoscaling feature against REAL dependencies (no mocks):
#   build -> run the real server -> curl the real endpoint -> RBAC -> W3C trace in Tempo -> Postgres audit rows.
# Optional --argocd runs READ-ONLY GitOps checks (render, server dry-run, app status); it never syncs anything.
#
# Dependencies it expects to be reachable (CI starts them as service containers):
#   DATABASE_URL (migrated Postgres), REDIS_ADDR, OTEL_EXPORTER_OTLP_ENDPOINT, TEMPO_URL
# Env: AS_PORT, AS_ADMIN_PORT, AS_STRICT=1 (no SKIPs allowed), DEPLOYED_URL (probe a deployed API for the route)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."
cd "${PROJECT_ROOT}"

RUN_ARGOCD=0
for arg in "$@"; do
    case "${arg}" in
        --argocd) RUN_ARGOCD=1 ;;
        -h|--help) sed -n 2,9p "${BASH_SOURCE[0]}"; exit 0 ;;
        *) echo "unknown argument: ${arg}" >&2; exit 2 ;;
    esac
done

if [[ -z "${DATABASE_URL:-}" && -f "${PROJECT_ROOT}/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "${PROJECT_ROOT}/.env"
    set +a
fi

CLI_PORT="${AS_PORT:-38091}"
ADMIN_PORT="${AS_ADMIN_PORT:-39191}"
export PORT="${CLI_PORT}" ADMIN_PORT
export JWT_SECRET="${JWT_SECRET:-autoscaling-e2e-jwt-secret-key-32-bytes-min}"
export RBAC_BOOTSTRAP_SECRET="${RBAC_BOOTSTRAP_SECRET:-autoscaling-e2e-bootstrap-secret-32bytes}"
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export ARGOCD_TOKEN=""
export TEMPO_URL="${TEMPO_URL:-http://127.0.0.1:3200}"
export CLI_PORT RBAC_BOOTSTRAP_SECRET

WORK_DIR="$(mktemp -d)"
SERVER_BIN="${WORK_DIR}/platform-api"
SERVER_LOG="${WORK_DIR}/server.log"
SERVER_PID=""

cleanup() {
    local status=$?
    if [[ -n "${SERVER_PID}" ]]; then
        kill -15 "${SERVER_PID}" 2>/dev/null || true
        wait "${SERVER_PID}" 2>/dev/null || true
    fi
    if [[ ${status} -ne 0 && -f "${SERVER_LOG}" ]]; then
        echo "---------------- last server log lines ----------------"
        tail -25 "${SERVER_LOG}" | cut -c1-240
    fi
    rm -rf "${WORK_DIR}"
    exit ${status}
}
trap cleanup EXIT

step() { echo -e "\033[1;34m[$1]\033[0m $2"; }

step BUILD "go build ./src/main.go"
go build -o "${SERVER_BIN}" ./src/main.go

step RUN "server on :${CLI_PORT} (admin :${ADMIN_PORT}) with the real database, redis and tracing backend"
"${SERVER_BIN}" > "${SERVER_LOG}" 2>&1 &
SERVER_PID=$!

READY=0
for _ in $(seq 1 60); do
    if [[ "$(curl --noproxy '*' -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${ADMIN_PORT}/readyz" || true)" == "200" ]]; then
        READY=1
        break
    fi
    kill -0 "${SERVER_PID}" 2>/dev/null || { echo "server exited early"; exit 1; }
    sleep 0.5
done
[[ ${READY} -eq 1 ]] || { echo "server never became ready"; exit 1; }
step READY "http://127.0.0.1:${CLI_PORT}"

ADMIN_TOKEN=$(curl --noproxy '*' -s -X POST "http://127.0.0.1:${CLI_PORT}/api/v1/auth/token" \
    -H "Content-Type: application/json" -H "X-Bootstrap-Secret: ${RBAC_BOOTSTRAP_SECRET}" \
    -d '{"subjectId":"autoscaling-e2e-admin","tenantId":"fintech-team","roles":["PlatformAdmin"],"ttlSeconds":3600}' \
    | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)
[[ -n "${ADMIN_TOKEN}" ]] || { echo "could not issue PlatformAdmin token"; exit 1; }
export ADMIN_TOKEN

# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/autoscaling.curl.sh"
run_autoscaling_checks

if [[ ${RUN_ARGOCD} -eq 1 ]]; then
    echo ""
    echo "--------------------------------------------------------------------------------"
    echo "[GITOPS] Read-only Argo CD verification (no sync, no apply)"
    echo "--------------------------------------------------------------------------------"
    RENDER="${WORK_DIR}/render.yaml"
    kubectl kustomize deploy/k8s/overlays/dev > "${RENDER}" || as_fail "kustomize render failed"
    python3 - "${RENDER}" <<'PY' || as_fail "rendered prometheus-rules lacks the autoscaling alerts"
import sys, yaml
docs = [d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
cm = next(d for d in docs if d["kind"] == "ConfigMap" and d["metadata"]["name"] == "prometheus-rules")
rules = yaml.safe_load(cm["data"]["autoscaling.rules.yaml"])
alerts = {r["alert"] for g in rules["groups"] for r in g["rules"]}
need = {"DatabaseConnectionsExhaustionApproaching", "KafkaConsumerLagCritical", "AutoscalingThresholdConfigMissing"}
missing = need - alerts
print("rendered autoscaling alerts:", sorted(alerts))
sys.exit(1 if missing else 0)
PY
    as_pass "Dev overlay renders and carries the autoscaling alert rules"

    if kubectl -n argocd get application internal-developer-platform-dev >/dev/null 2>&1; then
        APP_REV=$(kubectl -n argocd get application internal-developer-platform-dev -o jsonpath='{.status.sync.revision}')
        APP_SYNC=$(kubectl -n argocd get application internal-developer-platform-dev -o jsonpath='{.status.sync.status}')
        APP_HEALTH=$(kubectl -n argocd get application internal-developer-platform-dev -o jsonpath='{.status.health.status}')
        echo "argocd: sync=${APP_SYNC} health=${APP_HEALTH} revision=${APP_REV:0:12} local-head=$(git rev-parse --short=12 HEAD)"
        kubectl apply --dry-run=server -f <(python3 - "${RENDER}" <<'PY'
import sys, yaml
docs = [d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
cm = next(d for d in docs if d["kind"] == "ConfigMap" and d["metadata"]["name"] == "prometheus-rules")
print(yaml.safe_dump(cm))
PY
) >/dev/null || as_fail "server-side dry-run of prometheus-rules was rejected"
        as_pass "Server-side dry-run accepted the rules ConfigMap"
        DIRTY=$(git status --porcelain -- src/features/autoscaling scripts config contracts src/main.go deploy | wc -l | tr -d ' ')
        if [[ "${DIRTY}" != "0" ]]; then
            echo -e "\033[1;33m[INFO]\033[0m ${DIRTY} uncommitted file(s) are invisible to Argo CD until committed, pushed and merged"
        elif git merge-base --is-ancestor HEAD "${APP_REV}" 2>/dev/null; then
            as_pass "Argo CD revision already contains local HEAD"
        else
            echo -e "\033[1;33m[INFO]\033[0m Argo CD has not synced local HEAD yet (push + merge, then it auto-syncs)"
        fi
    else
        as_skip "No Argo CD application reachable from the current kube context"
    fi

    if [[ -n "${DEPLOYED_URL:-}" ]]; then
        CODE=$(curl --noproxy '*' -s -o /dev/null -w '%{http_code}' -X POST "${DEPLOYED_URL}/api/v1/autoscaling/plans:evaluate" \
            -H 'Content-Type: application/json' -d '{}')
        if [[ "${CODE}" == "401" || "${CODE}" == "403" || "${CODE}" == "422" ]]; then
            as_pass "Deployed API serves the autoscaling route (HTTP ${CODE})"
        else
            as_fail "Deployed API at ${DEPLOYED_URL} does not serve the autoscaling route (HTTP ${CODE})"
        fi
    fi
fi

echo ""
echo "================================================================================"
echo "          AUTOSCALING END-TO-END VERIFICATION PASSED (REAL DEPENDENCIES)         "
echo "================================================================================"

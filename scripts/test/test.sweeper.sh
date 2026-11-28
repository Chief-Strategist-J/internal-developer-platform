#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Self-Service Provisioning (SSP) Section C Sweeper Verification Script
# ==============================================================================

NAMESPACE="dev"
JOB_NAME="ssp-sweeper-manual-test"
CRONJOB_NAME="ssp-daily-sweeper"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "=== [SSP SWEEPER TEST] Starting verification for Section C Operating Model ==="

echo "[1/4] Applying sweeper CronJob manifest..."
kubectl apply -f "${REPO_ROOT}/deploy/k8s/base/apps/sweeper-cronjob.yaml"

echo "[2/4] Cleaning previous test job instances if present..."
kubectl delete job "${JOB_NAME}" -n "${NAMESPACE}" --ignore-not-found=true

echo "[3/4] Triggering manual execution from ${CRONJOB_NAME}..."
kubectl create job --from="cronjob/${CRONJOB_NAME}" "${JOB_NAME}" -n "${NAMESPACE}"

echo "[4/4] Waiting for job completion (timeout: 45s)..."
if kubectl wait --for=condition=complete "job/${JOB_NAME}" -n "${NAMESPACE}" --timeout=45s; then
    echo "=== [SSP SWEEPER TEST] SUCCESS: Sweeper job completed successfully ==="
    echo "--- Job Pod Logs ---"
    kubectl logs -n "${NAMESPACE}" "job/${JOB_NAME}"
    echo "--------------------"
    kubectl delete job "${JOB_NAME}" -n "${NAMESPACE}"
    exit 0
else
    echo "=== [SSP SWEEPER TEST] FAILED: Sweeper job did not complete successfully ==="
    echo "--- Job Pod Logs / Status ---"
    kubectl describe job "${JOB_NAME}" -n "${NAMESPACE}"
    kubectl logs -n "${NAMESPACE}" "job/${JOB_NAME}" || true
    echo "-----------------------------"
    exit 1
fi

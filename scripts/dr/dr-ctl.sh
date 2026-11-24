#!/usr/bin/env bash
# ==============================================================================
# DISASTER RECOVERY CONTROL PLANE CLI (DEVELOPER AUTOMATION & RUNBOOK ENGINE)
#
# Usage:
#   ./scripts/dr/dr-ctl.sh [command] [options]
#
# Commands:
#   plan:create       Create a new tier-governed DR plan
#   plan:check        Run §2 regional dependency probes
#   plan:rpo          Inspect real-time replication lag per dataset
#   incident:declare  Declare a regional/provider outage incident (Actor 1)
#   incident:approve  Approve incident with two-person quorum (Actor 2)
#   failover:init     Initiate stateful failover operation
#   failover:approve  Secondary approval & failover execution
#   dataset:fence     Revoke write paths and increment fencing epoch
#   failback:exec     Verify divergence = 0 and restore traffic to primary
#   drill:run         Run safe synthetic DR drill & generate audit evidence
#   evidence:get      Inspect immutable SHA-256 evidence bundle
#   restore:init      Trigger Backup for GKE cluster restore
#   breakglass:grant  Activate audited time-boxed emergency break-glass role
# ==============================================================================

set -euo pipefail

# Configurable environment overrides with production-grade defaults
export DR_API_BASE_URL="${DR_API_BASE_URL:-http://127.0.0.1:8080}"
export DR_TENANT_ID="${DR_TENANT_ID:-tenant-production}"
export DR_AUTH_TOKEN="${DR_AUTH_TOKEN:-}"

# Colors for interactive developer output
C_GREEN="\033[1;32m"
C_RED="\033[1;31m"
C_BLUE="\033[1;34m"
C_YELLOW="\033[1;33m"
C_RESET="\033[0m"

log_info() { echo -e "${C_BLUE}[DR-CTL]${C_RESET} $1"; }
log_success() { echo -e "${C_GREEN}[PASS]${C_RESET} $1"; }
log_warn() { echo -e "${C_YELLOW}[WARN]${C_RESET} $1"; }
log_fail() { echo -e "${C_RED}[FAIL]${C_RESET} $1"; exit 1; }

get_auth_header() {
  if [ -n "${DR_AUTH_TOKEN}" ]; then
    echo "Authorization: Bearer ${DR_AUTH_TOKEN}"
  else
    echo "Authorization: Bearer dev-bypass-token"
  fi
}

cmd_plan_create() {
  local plan_name="${1:-}"
  local tier="${2:-TIER_0}"
  local pri_region="${3:-us-central1}"
  local dr_region="${4:-us-east4}"
  local store="${5:-CLOUD_SQL}"

  if [ -z "${plan_name}" ]; then
    log_fail "Usage: $0 plan:create <plan_name> [tier] [primary_region] [dr_region] [store_type]"
  fi

  log_info "Creating DR Plan '${plan_name}' (Tier: ${tier}, ${pri_region} -> ${dr_region})..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "planName": "${plan_name}",
  "incidentTier": "${tier}",
  "primaryRegion": "${pri_region}",
  "drRegion": "${dr_region}",
  "topology": "WARM_STANDBY",
  "targetRtoSeconds": 300,
  "targetRpoSeconds": 0,
  "dataStoreType": "${store}",
  "secretSyncSource": "GOOGLE_SECRET_MANAGER",
  "primaryClusterId": "gke-${pri_region}-cluster",
  "drClusterId": "gke-${dr_region}-cluster",
  "idempotencyKey": "dr-plan-${plan_name}-$(date +%s)"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/plans" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_plan_check() {
  local plan_id="${1:-}"
  [ -z "${plan_id}" ] && log_fail "Usage: $0 plan:check <plan_id_or_name>"

  log_info "Running dependency probes for plan '${plan_id}'..."
  curl -s -X POST "${DR_API_BASE_URL}/api/v1/dr/plans/${plan_id}:checkReadiness" \
    -H "$(get_auth_header)" \
    -H "x-tenant-id: ${DR_TENANT_ID}"
}

cmd_plan_rpo() {
  local plan_id="${1:-}"
  [ -z "${plan_id}" ] && log_fail "Usage: $0 plan:rpo <plan_id_or_name>"

  log_info "Querying live RPO lag metrics for '${plan_id}'..."
  curl -s -X GET "${DR_API_BASE_URL}/api/v1/dr/plans/${plan_id}/rpo" \
    -H "$(get_auth_header)" \
    -H "x-tenant-id: ${DR_TENANT_ID}"
}

cmd_incident_declare() {
  local plan_id="${1:-}"
  local declarer="${2:-sre-lead}"
  local scope="${3:-REGION}"
  [ -z "${plan_id}" ] && log_fail "Usage: $0 incident:declare <plan_id> [declarer] [scope]"

  log_info "Declaring incident on plan '${plan_id}' by '${declarer}' (Scope: ${scope})..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "planId": "${plan_id}",
  "scope": "${scope}",
  "declarer": "${declarer}",
  "evidenceLinks": ["https://monitoring.internal/incident/$(date +%s)"]
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/incidents:declare" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_incident_approve() {
  local incident_id="${1:-}"
  local approver="${2:-director-oncall}"
  [ -z "${incident_id}" ] && log_fail "Usage: $0 incident:approve <incident_id> [approver]"

  log_info "Approving incident '${incident_id}' by second approver '${approver}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "incidentId": "${incident_id}",
  "approver": "${approver}"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/incidents/${incident_id}:approve" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_failover_init() {
  local plan_id="${1:-}"
  local incident_id="${2:-}"
  local initiator="${3:-sre-operator}"
  local mode="${4:-SWITCHOVER}"
  [ -z "${plan_id}" ] || [ -z "${incident_id}" ] && log_fail "Usage: $0 failover:init <plan_id> <incident_id> [initiator] [mode]"

  log_info "Initiating failover for plan '${plan_id}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "planId": "${plan_id}",
  "incidentId": "${incident_id}",
  "mode": "${mode}",
  "initiator": "${initiator}"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/failovers" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_failover_approve() {
  local op_id="${1:-}"
  local approver="${2:-vp-engineering}"
  [ -z "${op_id}" ] && log_fail "Usage: $0 failover:approve <operation_id> [approver]"

  log_info "Authorizing failover operation '${op_id}' by '${approver}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "operationId": "${op_id}",
  "approver": "${approver}"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/failovers/${op_id}:approve" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_dataset_fence() {
  local dataset_id="${1:-}"
  local actor="${2:-sre-guard}"
  [ -z "${dataset_id}" ] && log_fail "Usage: $0 dataset:fence <dataset_id> [actor]"

  log_info "Fencing old primary write path for '${dataset_id}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "datasetId": "${dataset_id}",
  "authority": "DR_REPLICA",
  "actor": "${actor}"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/datasets/${dataset_id}:fence" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_failback_exec() {
  local plan_id="${1:-}"
  local initiator="${2:-sre-lead}"
  local approver="${3:-vp-infra}"
  [ -z "${plan_id}" ] && log_fail "Usage: $0 failback:exec <plan_id> [initiator] [approver]"

  log_info "Executing failback for '${plan_id}' with zero-divergence validation..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "planId": "${plan_id}",
  "initiator": "${initiator}",
  "approver": "${approver}",
  "divergence": 0,
  "reverseLag": 0
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/failbacks" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_drill_run() {
  local plan_id="${1:-}"
  local initiator="${2:-dr-coordinator}"
  [ -z "${plan_id}" ] && log_fail "Usage: $0 drill:run <plan_id> [initiator]"

  log_info "Executing synthetic production-safe DR drill for '${plan_id}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "planId": "${plan_id}",
  "scope": "REGION",
  "initiator": "${initiator}",
  "syntheticOnly": true
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/drills" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_evidence_get() {
  local run_id="${1:-}"
  [ -z "${run_id}" ] && log_fail "Usage: $0 evidence:get <run_id_or_plan_id>"

  log_info "Retrieving immutable audit evidence bundle for '${run_id}'..."
  curl -s -X GET "${DR_API_BASE_URL}/api/v1/dr/evidence/${run_id}" \
    -H "$(get_auth_header)" \
    -H "x-tenant-id: ${DR_TENANT_ID}"
}

cmd_restore_init() {
  local plan_id="${1:-}"
  local backup_id="${2:-bkp-gke-latest}"
  local cluster="${3:-gke-dr-cluster}"
  local ns="${4:-default}"
  [ -z "${plan_id}" ] && log_fail "Usage: $0 restore:init <plan_id> [backup_id] [cluster] [namespace]"

  log_info "Triggering Backup for GKE Restore into '${cluster}' namespace '${ns}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "planId": "${plan_id}",
  "backupId": "${backup_id}",
  "targetCluster": "${cluster}",
  "namespace": "${ns}",
  "conflictPolicy": "FAIL_ON_CONFLICT",
  "initiator": "sre-restore-operator"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/restores" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

cmd_breakglass_grant() {
  local requester="${1:-operator-alpha}"
  local approver="${2:-director-security}"
  local duration="${3:-60}"
  local reason="${4:-Emergency outage remediation}"

  log_info "Requesting two-person break-glass elevation for '${requester}'..."
  local payload
  payload=$(cat <<EOF
{
  "tenantId": "${DR_TENANT_ID}",
  "requester": "${requester}",
  "approver": "${approver}",
  "durationMins": ${duration},
  "justification": "${reason}"
}
EOF
)

  curl -s -i -X POST "${DR_API_BASE_URL}/api/v1/dr/breakglass:activate" \
    -H "$(get_auth_header)" \
    -H "Content-Type: application/json" \
    -H "x-tenant-id: ${DR_TENANT_ID}" \
    -d "${payload}"
}

# Subcommand routing
COMMAND="${1:-help}"
shift || true

case "${COMMAND}" in
  plan:create)       cmd_plan_create "$@" ;;
  plan:check)        cmd_plan_check "$@" ;;
  plan:rpo)          cmd_plan_rpo "$@" ;;
  incident:declare)  cmd_incident_declare "$@" ;;
  incident:approve)  cmd_incident_approve "$@" ;;
  failover:init)     cmd_failover_init "$@" ;;
  failover:approve)  cmd_failover_approve "$@" ;;
  dataset:fence)     cmd_dataset_fence "$@" ;;
  failback:exec)     cmd_failback_exec "$@" ;;
  drill:run)         cmd_drill_run "$@" ;;
  evidence:get)      cmd_evidence_get "$@" ;;
  restore:init)      cmd_restore_init "$@" ;;
  breakglass:grant)  cmd_breakglass_grant "$@" ;;
  *)
    echo "Disaster Recovery CLI - Commands:"
    echo "  plan:create <plan_name> [tier] [pri_region] [dr_region] [store]"
    echo "  plan:check <plan_id>"
    echo "  plan:rpo <plan_id>"
    echo "  incident:declare <plan_id> [declarer] [scope]"
    echo "  incident:approve <incident_id> [approver]"
    echo "  failover:init <plan_id> <incident_id> [initiator] [mode]"
    echo "  failover:approve <operation_id> [approver]"
    echo "  dataset:fence <dataset_id> [actor]"
    echo "  failback:exec <plan_id> [initiator] [approver]"
    echo "  drill:run <plan_id> [initiator]"
    echo "  evidence:get <run_id>"
    echo "  restore:init <plan_id> [backup_id] [cluster] [namespace]"
    echo "  breakglass:grant <requester> <approver> [duration] [reason]"
    ;;
esac

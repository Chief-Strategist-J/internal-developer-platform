#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

DEV_NAMESPACE="${DEV_NAMESPACE:-dev}"
ARGOCD_NAMESPACE="${ARGOCD_NAMESPACE:-argocd}"
RUN_DIR="${HOME}/.local/share/idp-platform"
LOG_DIR="${RUN_DIR}/logs"
PID_FILE="${RUN_DIR}/port-forward.pid"

mkdir -p "${LOG_DIR}"

declare -A FORWARD_TARGETS=(
    ["argocd"]="svc/argocd-server:${ARGOCD_NAMESPACE}:8085:443"
    ["grafana"]="svc/grafana:${DEV_NAMESPACE}:3000:3000"
    ["platform-api"]="svc/platform-api:${DEV_NAMESPACE}:8080:8080"
    ["otel-collector"]="svc/otel-collector:${DEV_NAMESPACE}:4318:4318"
    ["tempo"]="svc/tempo:${DEV_NAMESPACE}:3200:3200"
    ["postgres"]="svc/postgres:${DEV_NAMESPACE}:5432:5432"
    ["redis"]="svc/redis:${DEV_NAMESPACE}:6379:6379"
)

cleanup_existing_forwarders() {
    log_info "Terminating any stale port-forward instances..."
    if [[ -f "${PID_FILE}" ]]; then
        local supervisor_pid
        supervisor_pid=$(cat "${PID_FILE}" 2>/dev/null || true)
        if [[ -n "${supervisor_pid}" ]] && kill -0 "${supervisor_pid}" 2>/dev/null; then
            kill "${supervisor_pid}" 2>/dev/null || true
        fi
        rm -f "${PID_FILE}"
    fi

    for name in "${!FORWARD_TARGETS[@]}"; do
        local target_spec="${FORWARD_TARGETS[$name]}"
        IFS=':' read -r resource namespace local_port remote_port <<< "${target_spec}"
        pkill -f "kubectl port-forward ${resource} -n ${namespace}" 2>/dev/null || true
    done
}

run_supervisor() {
    log_info "Starting port-forward supervisor daemon..."
    echo $$ > "${PID_FILE}"

    trap 'log_info "Stopping supervisor..."; rm -f "${PID_FILE}"; exit 0' SIGINT SIGTERM

    while true; do
        for name in "${!FORWARD_TARGETS[@]}"; do
            local target_spec="${FORWARD_TARGETS[$name]}"
            IFS=':' read -r resource namespace local_port remote_port <<< "${target_spec}"

            if ! ss -tlpn 2>/dev/null | grep -q ":${local_port} "; then
                log_info "Port ${local_port} (${name}) not bound. Starting forwarder..."
                nohup kubectl port-forward "${resource}" -n "${namespace}" \
                    --address 0.0.0.0 "${local_port}:${remote_port}" \
                    > "${LOG_DIR}/${name}.log" 2>&1 &
            fi
        done
        sleep 3
    done
}

status_forwarders() {
    log_info "=== Active Port Forwarding Status ==="
    printf "%-18s %-12s %-25s %-10s\n" "SERVICE" "LOCAL PORT" "REMOTE TARGET" "STATUS"
    printf "%-18s %-12s %-25s %-10s\n" "-------" "----------" "-------------" "------"

    for name in "${!FORWARD_TARGETS[@]}"; do
        local target_spec="${FORWARD_TARGETS[$name]}"
        IFS=':' read -r resource namespace local_port remote_port <<< "${target_spec}"

        local status="DOWN"
        if ss -tlpn 2>/dev/null | grep -q ":${local_port} "; then
            status="BOUND"
        fi
        printf "%-18s %-12s %-25s %-10s\n" "${name}" "${local_port}" "${resource}:${remote_port}" "${status}"
    done

    if [[ -f "${PID_FILE}" ]]; then
        local pid
        pid=$(cat "${PID_FILE}" 2>/dev/null || true)
        if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
            log_success "Supervisor daemon is running (PID: ${pid})."
        else
            log_info "Supervisor daemon PID file exists but process is not running."
        fi
    else
        log_info "Supervisor daemon is stopped."
    fi
}

start_daemon() {
    require_binaries kubectl ss
    cleanup_existing_forwarders

    log_info "Starting supervisor daemon in background..."
    nohup "$0" supervise > "${LOG_DIR}/supervisor.log" 2>&1 &
    sleep 2
    status_forwarders
    log_success "All port forwards established. Logs available at: ${LOG_DIR}/"
}

stop_daemon() {
    cleanup_existing_forwarders
    log_success "All port forwarders and supervisor daemon terminated."
}

case "${1:-}" in
    start)
        start_daemon
        ;;
    stop)
        stop_daemon
        ;;
    restart)
        stop_daemon
        sleep 1
        start_daemon
        ;;
    status)
        status_forwarders
        ;;
    supervise)
        run_supervisor
        ;;
    *)
        echo "Usage: $(basename "$0") {start|stop|restart|status}"
        exit 1
        ;;
esac

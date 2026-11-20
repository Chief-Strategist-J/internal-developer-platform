#!/usr/bin/env bash
set -euo pipefail

export PATH="${PATH}:/snap/bin:/usr/local/go/bin:${HOME}/.local/bin:${HOME}/google-cloud-sdk/bin"

log_info() {
    echo -e "\033[1;34m[INFO]\033[0m $1"
}

log_success() {
    echo -e "\033[1;32m[SUCCESS]\033[0m $1"
}

log_error() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
}

require_binaries() {
    for bin in "$@"; do
        command -v "${bin}" >/dev/null 2>&1 || { log_error "Required binary '${bin}' not found in PATH"; exit 1; }
    done
}

diagnose_compose_failures() {
    local compose_file="$1"
    log_info "Diagnosing container logs for unhealthy or restarting services..."
    local services
    services=$(docker compose -f "${compose_file}" ps --format '{{.Service}}')
    for svc in ${services}; do
        log_info "=== Service: ${svc} (last 10 log lines) ==="
        docker compose -f "${compose_file}" logs --tail 10 "${svc}" || true
    done
}

prune_container_images() {
    local images=("$@")
    for img in "${images[@]}"; do
        if docker image inspect "${img}" >/dev/null 2>&1; then
            log_info "Removing superseded image: ${img}"
            docker rmi "${img}" || true
        fi
    done
}

#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/deploy/docker/compose/docker-compose.polyglot.yml"

log_info() {
    echo -e "\033[1;34m[INFO]\033[0m $1"
}

log_success() {
    echo -e "\033[1;32m[SUCCESS]\033[0m $1"
}

log_error() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
}

validate_production_environment() {
    log_info "Validating production deployment prerequisites..."
    if [[ ! -f "${COMPOSE_FILE}" ]]; then
        log_error "Compose manifest missing at ${COMPOSE_FILE}"
        exit 1
    fi

    if [[ -z "${SECRETS_DIR:-}" ]]; then
        log_error "SECRETS_DIR must be explicitly set in production."
        exit 1
    fi

    if [[ ! -f "${SECRETS_DIR}/db_password.txt" ]]; then
        log_error "Database password secret missing at ${SECRETS_DIR}/db_password.txt"
        exit 1
    fi
}

start_production() {
    log_info "Booting production containers in background..."
    docker compose -f "${COMPOSE_FILE}" up -d --wait --wait-timeout 180
    log_success "Production containers started and verified healthy."
}

stop_production() {
    log_info "Stopping production containers gracefully (without wiping persistent volumes)..."
    docker compose -f "${COMPOSE_FILE}" down
    log_success "Production containers stopped."
}

verify_production() {
    log_info "Production container status:"
    docker compose -f "${COMPOSE_FILE}" ps
}

main() {
    local action="${1:-up}"
    case "${action}" in
        up)
            validate_production_environment
            start_production
            verify_production
            ;;
        down)
            stop_production
            ;;
        status)
            verify_production
            ;;
        *)
            echo "Usage: $0 [up|down|status]"
            exit 1
            ;;
    esac
}

main "$@"

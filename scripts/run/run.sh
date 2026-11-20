#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${REPO_ROOT}/deploy/docker/compose/docker-compose.polyglot.yml"
INFRA_COMPOSE_FILE="${REPO_ROOT}/deploy/docker/compose/docker-compose.infra.yml"

validate_prerequisites() {
    require_binaries docker curl
    log_info "Validating compose file existence..."
    if [[ ! -f "${COMPOSE_FILE}" ]]; then
        log_error "Compose file not found at ${COMPOSE_FILE}"
        exit 1
    fi
    if [[ ! -f "${INFRA_COMPOSE_FILE}" ]]; then
        log_error "Infrastructure compose file not found at ${INFRA_COMPOSE_FILE}"
        exit 1
    fi

    local secrets_dir="${SECRETS_DIR:-/tmp}"
    if [[ ! -f "${secrets_dir}/db_password.txt" ]]; then
        log_info "Creating default development db_password secret..."
        echo "postgres_dev_password_123" > "${secrets_dir}/db_password.txt"
    fi
}

start_containers() {
    log_info "Starting Docker polyglot platform containers..."
    docker compose -f "${COMPOSE_FILE}" up -d
}

wait_for_services() {
    log_info "Waiting for containers to reach healthy status..."
    local timeout=90
    local elapsed=0

    while [[ ${elapsed} -lt ${timeout} ]]; do
        local unhealthy
        unhealthy=$(docker compose -f "${COMPOSE_FILE}" ps --format '{{.Service}}: {{.Health}}' | grep -v 'healthy' | grep -v '^$' || true)

        if [[ -z "${unhealthy}" ]]; then
            log_success "All application and backing infrastructure containers are healthy."
            return 0
        fi

        sleep 5
        elapsed=$((elapsed + 5))
    done

    log_error "Timeout reached waiting for containers to become healthy."
    docker compose -f "${COMPOSE_FILE}" ps
    diagnose_compose_failures "${COMPOSE_FILE}"
    exit 1
}

verify_status() {
    log_info "Container verification status:"
    docker compose -f "${COMPOSE_FILE}" ps
}

test_endpoints() {
    log_info "Verifying core service endpoints..."

    log_info "Checking PostgreSQL connection via docker exec..."
    if docker compose -f "${COMPOSE_FILE}" exec -T db pg_isready -U postgres >/dev/null 2>&1; then
        log_success "PostgreSQL is accepting connections."
    else
        log_error "PostgreSQL is not ready."
    fi

    log_info "Checking Redis cache ping..."
    if docker compose -f "${COMPOSE_FILE}" exec -T cache redis-cli ping | grep -q "PONG"; then
        log_success "Redis cache responded with PONG."
    else
        log_error "Redis cache ping failed."
    fi

    log_info "Checking Caddy edge proxy (http://127.0.0.1:80)..."
    local proxy_code
    proxy_code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:80/ || echo "000")
    if [[ "${proxy_code}" != "000" ]]; then
        log_success "Caddy ingress proxy reachable (HTTP ${proxy_code})."
    else
        log_error "Caddy ingress proxy unreachable."
    fi
}

down_containers() {
    log_info "Stopping containers gracefully..."
    docker compose -f "${COMPOSE_FILE}" down
}

clean_containers() {
    log_info "Stopping containers and removing named development volumes..."
    docker compose -f "${COMPOSE_FILE}" down -v
}

cleanup_obsolete_images() {
    prune_container_images \
        "postgres:16.4-bookworm" \
        "postgres:17-bookworm" \
        "redis:7.4-bookworm" \
        "golang:1.23.2-bookworm" \
        "node:22.11.0-slim"
}

main() {
    local action="${1:-up}"
    case "${action}" in
        down)
            down_containers
            ;;
        clean)
            clean_containers
            ;;
        prune)
            clean_containers
            cleanup_obsolete_images
            ;;
        up)
            validate_prerequisites
            start_containers
            wait_for_services
            verify_status
            test_endpoints
            ;;
        test)
            test_endpoints
            ;;
        all)
            clean_containers
            cleanup_obsolete_images
            validate_prerequisites
            start_containers
            wait_for_services
            verify_status
            test_endpoints
            ;;
        *)
            echo "Usage: $0 [down|clean|prune|up|test|all]"
            exit 1
            ;;
    esac
}

main "$@"

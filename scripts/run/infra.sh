#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/../../deploy/docker/compose/docker-compose.infra.yml"

log_info() {
    echo -e "\033[1;34m[INFO]\033[0m $1"
}

log_success() {
    echo -e "\033[1;32m[SUCCESS]\033[0m $1"
}

log_error() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
}

validate_prerequisites() {
    log_info "Validating compose file existence..."
    if [[ ! -f "${COMPOSE_FILE}" ]]; then
        log_error "Compose file not found at ${COMPOSE_FILE}"
        exit 1
    fi

    local compose_dir
    compose_dir="$(dirname "${COMPOSE_FILE}")"
    local secrets_dir="${compose_dir}/secrets"
    mkdir -p "${secrets_dir}"
    if [[ ! -f "${secrets_dir}/db_password.txt" ]]; then
        echo -n "postgres_dev_password_123" > "${secrets_dir}/db_password.txt"
        log_info "Generated default secrets/db_password.txt"
    fi
    mkdir -p /tmp
    echo -n "postgres_dev_password_123" > /tmp/db_password.txt
}

start_infrastructure() {
    log_info "Starting Docker infrastructure containers..."
    docker compose -f "${COMPOSE_FILE}" up -d
}

wait_for_services() {
    log_info "Waiting for all containers to reach healthy status..."
    local timeout=90
    local elapsed=0

    while [[ ${elapsed} -lt ${timeout} ]]; do
        local unhealthy
        unhealthy=$(docker compose -f "${COMPOSE_FILE}" ps --format '{{.Service}}: {{.Health}}' | grep -v 'healthy' | grep -v 'otel-collector' || true)

        if [[ -z "${unhealthy}" ]]; then
            log_success "All infrastructure containers are healthy."
            return 0
        fi

        sleep 5
        elapsed=$((elapsed + 5))
    done

    log_error "Timeout reached waiting for infrastructure to become healthy."
    docker compose -f "${COMPOSE_FILE}" ps
    diagnose_failures
    exit 1
}

diagnose_failures() {
    log_info "Diagnosing container logs for unhealthy/restarting services..."
    local services
    services=$(docker compose -f "${COMPOSE_FILE}" ps --format '{{.Service}}')
    for svc in ${services}; do
        log_info "=== Service: ${svc} (last 10 log lines) ==="
        docker compose -f "${COMPOSE_FILE}" logs --tail 10 "${svc}" || true
    done
}

verify_status() {
    log_info "Infrastructure container verification:"
    docker compose -f "${COMPOSE_FILE}" ps
}

test_trace_ingestion() {
    log_info "Testing trace ingestion via OTLP HTTP endpoint (127.0.0.1:4318)..."
    local now_s
    now_s=$(date +%s)
    local start_ns="${now_s}000000000"
    local end_ns="$((now_s + 1))000000000"
    local trace_id
    trace_id=$(head -c 16 /dev/urandom | xxd -p)
    local span_id
    span_id=$(head -c 8 /dev/urandom | xxd -p)

    local trace_payload=$(cat <<EOF
{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "internal-developer-platform" } },
          { "key": "environment", "value": { "stringValue": "dev" } }
        ]
      },
      "scopeSpans": [
        {
          "scope": { "name": "verification-script" },
          "spans": [
            {
              "traceId": "${trace_id}",
              "spanId": "${span_id}",
              "name": "verification-span",
              "kind": 1,
              "startTimeUnixNano": "${start_ns}",
              "endTimeUnixNano": "${end_ns}",
              "status": { "code": 1 }
            }
          ]
        }
      ]
    }
  ]
}
EOF
)

    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:4318/v1/traces \
      -H "Content-Type: application/json" \
      -d "${trace_payload}" || echo "000")

    if [[ "${response}" == "200" ]]; then
        log_success "OTLP Trace Ingestion OK (HTTP 200). Trace ID: ${trace_id}"
    else
        log_error "Trace Ingestion failed with HTTP status: ${response}"
    fi
}

test_endpoints() {
    log_info "Verifying core service endpoints..."

    # Prometheus
    local prom_code
    prom_code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:9090/-/ready || echo "000")
    if [[ "${prom_code}" == "200" ]]; then
        log_success "Prometheus endpoint healthy (HTTP 200) at http://127.0.0.1:9090"
    else
        log_error "Prometheus failed (HTTP ${prom_code})"
    fi

    # Grafana
    local grafana_code
    grafana_code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:3000/api/health || echo "000")
    if [[ "${grafana_code}" == "200" ]]; then
        log_success "Grafana endpoint healthy (HTTP 200) at http://127.0.0.1:3000"
    else
        log_error "Grafana failed (HTTP ${grafana_code})"
    fi

    # Caddy Ingress
    local caddy_code
    caddy_code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/ || echo "000")
    if [[ "${caddy_code}" == "200" ]]; then
        log_success "Caddy Ingress Proxy healthy (HTTP 200) at http://127.0.0.1:8080"
    else
        log_error "Caddy Proxy failed (HTTP ${caddy_code})"
    fi

    # OpenTelemetry Collector Extension Health
    local otel_code
    otel_code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:13133/ || echo "000")
    if [[ "${otel_code}" == "200" ]]; then
        log_success "OpenTelemetry Collector health extension OK (HTTP 200) at http://127.0.0.1:13133"
    else
        log_error "OpenTelemetry Collector health failed (HTTP ${otel_code})"
    fi
}

test_logs_ingestion() {
    log_info "Testing log ingestion via OTLP HTTP endpoint (127.0.0.1:4318/v1/logs)..."
    local now_s
    now_s=$(date +%s)
    local now_ns="${now_s}000000000"

    local log_payload=$(cat <<EOF
{
  "resourceLogs": [
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "internal-developer-platform" } },
          { "key": "environment", "value": { "stringValue": "dev" } }
        ]
      },
      "scopeLogs": [
        {
          "scope": { "name": "verification-script" },
          "logRecords": [
            {
              "timeUnixNano": "${now_ns}",
              "severityNumber": 9,
              "severityText": "INFO",
              "body": { "stringValue": "Platform health-check ping via OpenTelemetry Collector" }
            }
          ]
        }
      ]
    }
  ]
}
EOF
)

    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:4318/v1/logs \
      -H "Content-Type: application/json" \
      -d "${log_payload}" || echo "000")

    if [[ "${response}" == "200" ]]; then
        log_success "OTLP Log Ingestion OK (HTTP 200). Log sent to Loki via OpenTelemetry Collector."
    else
        log_error "Log Ingestion failed with HTTP status: ${response}"
    fi
}

down_infrastructure() {
    log_info "Stopping containers and wiping volumes for clean upgrade..."
    validate_prerequisites
    docker compose -f "${COMPOSE_FILE}" down -v
}

prune_obsolete_images() {
    log_info "Removing outdated container images to reclaim host memory and disk..."
    local old_images=(
        "postgres:16.4-bookworm"
        "apache/kafka:3.7.0"
    )
    for img in "${old_images[@]}"; do
        if docker image inspect "${img}" >/dev/null 2>&1; then
            log_info "Removing superseded image: ${img}"
            docker rmi "${img}" || true
        fi
    done
}

main() {
    local action="${1:-all}"
    case "${action}" in
        down)
            down_infrastructure
            ;;
        prune)
            down_infrastructure
            prune_obsolete_images
            ;;
        up)
            validate_prerequisites
            start_infrastructure
            wait_for_services
            verify_status
            ;;
        test)
            test_trace_ingestion
            test_logs_ingestion
            test_endpoints
            ;;
        all)
            down_infrastructure
            prune_obsolete_images
            validate_prerequisites
            start_infrastructure
            wait_for_services
            verify_status
            test_trace_ingestion
            test_logs_ingestion
            test_endpoints
            ;;
        *)
            echo "Usage: $0 [down|prune|up|test|all]"
            exit 1
            ;;
    esac
}

main "$@"

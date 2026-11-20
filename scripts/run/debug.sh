#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

DEV_NAMESPACE="${DEV_NAMESPACE:-dev}"

usage() {
    cat <<EOF
Usage: $(basename "$0") <service> <command> [args...]

Services:
  postgres | db         PostgreSQL Database (StatefulSet)
  redis | cache         Redis Cache (Deployment)
  app | platform-api    Platform API Workload (Deployment)
  otel | collector      OpenTelemetry Collector (Deployment)
  tempo | tracing       Tempo Tracing Engine (Deployment)
  loki | logs           Loki Log Aggregator (Deployment)
  prometheus | metrics  Prometheus Metrics Engine (Deployment)
  grafana               Grafana Dashboards (Deployment)
  all                   Target multi-component operations (logs, status)

Commands:
  sh | shell | exec     Open an interactive bash/sh shell inside the container
  query "<sql|cmd>"     Run a direct query (psql for postgres, redis-cli for redis)
  logs [-f] [--tail=N]  Stream or tail logs for the service
  follow-flow           Stream correlated database & application logs simultaneously
  top                   Display live CPU / Memory resource utilization
  status                Display pod health, endpoints, and probe status

Examples:
  ./scripts/run/debug.sh postgres query "SELECT tablename FROM pg_tables WHERE schemaname='public';"
  ./scripts/run/debug.sh redis query "PING"
  ./scripts/run/debug.sh redis query "KEYS *"
  ./scripts/run/debug.sh app shell
  ./scripts/run/debug.sh postgres shell
  ./scripts/run/debug.sh all follow-flow
  ./scripts/run/debug.sh tempo logs --tail=50
EOF
    exit 1
}

if [[ $# -lt 2 ]]; then
    usage
fi

SERVICE_ARG="$1"
CMD_ARG="$2"
shift 2

get_pod_name() {
    local label_selector="$1"
    local pod
    pod=$(kubectl get pods -n "${DEV_NAMESPACE}" -l "${label_selector}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [[ -z "${pod}" ]]; then
        log_error "No running pods found matching selector '${label_selector}' in namespace '${DEV_NAMESPACE}'"
        exit 1
    fi
    echo "${pod}"
}

case "${SERVICE_ARG}" in
    postgres|db)
        POD=$(get_pod_name "app.kubernetes.io/name=postgres")
        case "${CMD_ARG}" in
            shell|sh|exec)
                log_info "Opening interactive shell inside ${POD}..."
                kubectl exec -it -n "${DEV_NAMESPACE}" "${POD}" -- bash || kubectl exec -it -n "${DEV_NAMESPACE}" "${POD}" -- sh
                ;;
            query)
                if [[ $# -lt 1 ]]; then
                    log_error "Usage: debug.sh postgres query \"<sql_statement>\""
                    exit 1
                fi
                SQL="$1"
                log_info "Executing SQL against ${POD} (db: idp_db)..."
                kubectl exec -n "${DEV_NAMESPACE}" "${POD}" -- psql -U postgres -d idp_db -c "${SQL}"
                ;;
            logs)
                kubectl logs -n "${DEV_NAMESPACE}" "${POD}" "$@"
                ;;
            status)
                kubectl describe pod -n "${DEV_NAMESPACE}" "${POD}"
                ;;
            *)
                log_error "Unsupported command '${CMD_ARG}' for service '${SERVICE_ARG}'"
                usage
                ;;
        esac
        ;;

    redis|cache)
        POD=$(get_pod_name "app.kubernetes.io/name=redis")
        case "${CMD_ARG}" in
            shell|sh|exec)
                log_info "Opening interactive shell inside ${POD}..."
                kubectl exec -it -n "${DEV_NAMESPACE}" "${POD}" -- sh
                ;;
            query)
                if [[ $# -lt 1 ]]; then
                    log_error "Usage: debug.sh redis query \"<redis_command>\""
                    exit 1
                fi
                log_info "Executing Redis command against ${POD}..."
                kubectl exec -n "${DEV_NAMESPACE}" "${POD}" -- redis-cli "$@"
                ;;
            logs)
                kubectl logs -n "${DEV_NAMESPACE}" "${POD}" "$@"
                ;;
            status)
                kubectl describe pod -n "${DEV_NAMESPACE}" "${POD}"
                ;;
            *)
                log_error "Unsupported command '${CMD_ARG}' for service '${SERVICE_ARG}'"
                usage
                ;;
        esac
        ;;

    app|platform-api)
        POD=$(get_pod_name "app.kubernetes.io/name=platform-api")
        case "${CMD_ARG}" in
            shell|sh|exec)
                log_info "Opening interactive shell inside ${POD}..."
                kubectl exec -it -n "${DEV_NAMESPACE}" "${POD}" -- sh
                ;;
            logs)
                kubectl logs -n "${DEV_NAMESPACE}" "${POD}" "$@"
                ;;
            status)
                kubectl describe pod -n "${DEV_NAMESPACE}" "${POD}"
                ;;
            *)
                log_error "Unsupported command '${CMD_ARG}' for service '${SERVICE_ARG}'"
                usage
                ;;
        esac
        ;;

    otel|collector)
        POD=$(get_pod_name "app.kubernetes.io/name=otel-collector")
        case "${CMD_ARG}" in
            shell|sh|exec)
                kubectl exec -it -n "${DEV_NAMESPACE}" "${POD}" -- sh
                ;;
            logs)
                kubectl logs -n "${DEV_NAMESPACE}" "${POD}" "$@"
                ;;
            *)
                log_error "Unsupported command '${CMD_ARG}'"
                ;;
        esac
        ;;

    tempo|tracing)
        POD=$(get_pod_name "app.kubernetes.io/name=tempo")
        case "${CMD_ARG}" in
            shell|sh|exec)
                kubectl exec -it -n "${DEV_NAMESPACE}" "${POD}" -- sh
                ;;
            logs)
                kubectl logs -n "${DEV_NAMESPACE}" "${POD}" "$@"
                ;;
            *)
                log_error "Unsupported command '${CMD_ARG}'"
                ;;
        esac
        ;;

    all)
        case "${CMD_ARG}" in
            follow-flow)
                log_info "Streaming correlated multi-service execution flow (Platform API + Database + Collector)..."
                trap 'kill 0' SIGINT SIGTERM EXIT
                kubectl logs -n "${DEV_NAMESPACE}" -l app.kubernetes.io/name=platform-api -f --prefix=true &
                kubectl logs -n "${DEV_NAMESPACE}" -l app.kubernetes.io/name=postgres -f --prefix=true &
                kubectl logs -n "${DEV_NAMESPACE}" -l app.kubernetes.io/name=otel-collector -f --prefix=true &
                wait
                ;;
            status)
                log_info "=== Namespace: ${DEV_NAMESPACE} Pods ==="
                kubectl get pods -n "${DEV_NAMESPACE}" -o wide
                log_info "=== Namespace: ${DEV_NAMESPACE} Services ==="
                kubectl get svc -n "${DEV_NAMESPACE}"
                ;;
            *)
                usage
                ;;
        esac
        ;;

    *)
        log_error "Unknown service '${SERVICE_ARG}'"
        usage
        ;;
esac

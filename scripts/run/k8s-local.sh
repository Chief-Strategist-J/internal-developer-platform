#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

CLUSTER_NAME="idp-cluster"
ARGOCD_NAMESPACE="argocd"
ARGOCD_PORT=8085
GRAFANA_PORT=3000
PLATFORM_API_PORT=8080
OTEL_PORT=4318
POSTGRES_PORT=5432
REDIS_PORT=6379
DEV_NAMESPACE="dev"

create_cluster() {
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_info "kind cluster '${CLUSTER_NAME}' already exists. Ensuring control-plane container is running..."
        docker start "${CLUSTER_NAME}-control-plane" >/dev/null 2>&1 || true
        for _ in $(seq 1 30); do
            if kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1; then
                break
            fi
            sleep 2
        done
    else
        log_info "Creating kind cluster '${CLUSTER_NAME}' with DNS and host port forwarders..."
        cat <<'EOF_KIND' | kind create cluster --name "${CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 30080
    protocol: TCP
  - containerPort: 30443
    hostPort: 30443
    protocol: TCP
EOF_KIND
        log_success "kind cluster '${CLUSTER_NAME}' created."
    fi

    kubectl cluster-info --context "kind-${CLUSTER_NAME}"

    log_info "Configuring CoreDNS upstream DNS (1.1.1.1, 8.8.8.8) to prevent git clone timeouts..."
    kubectl get configmap coredns -n kube-system -o yaml | \
      sed 's/forward . \/etc\/resolv.conf/forward . 1.1.1.1 8.8.8.8/' | \
      kubectl apply -f - || true
    kubectl rollout restart -n kube-system deployment/coredns || true
}

install_crd_dependencies() {
    log_info "Installing External Secrets Operator CRDs..."
    kubectl apply -f https://raw.githubusercontent.com/external-secrets/external-secrets/v0.9.11/deploy/crds/bundle.yaml --server-side --force-conflicts || true
}

install_argocd() {
    log_info "Creating namespace '${ARGOCD_NAMESPACE}'..."
    kubectl get ns "${ARGOCD_NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${ARGOCD_NAMESPACE}"

    log_info "Applying official ArgoCD manifests using server-side apply..."
    kubectl apply -n "${ARGOCD_NAMESPACE}" -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml --server-side --force-conflicts

    log_info "Waiting for ArgoCD server deployment to become available (timeout: 300s)..."
    kubectl wait --for=condition=available deployment/argocd-server -n "${ARGOCD_NAMESPACE}" --timeout=300s

    log_success "ArgoCD server is healthy and running."
}

build_and_load_platform_api() {
    local root_dir="${SCRIPT_DIR}/../.."
    log_info "Building and packaging native Go platform-api container image..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /tmp/platform-svc "${root_dir}/src/main.go"
    
    local tmp_build="${root_dir}/deploy/docker/tmp-build"
    mkdir -p "${tmp_build}"
    cp /tmp/platform-svc "${tmp_build}/platform-svc"
    cat << 'EOF_DOCKER' > "${tmp_build}/Dockerfile"
FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tini \
 && rm -rf /var/lib/apt/lists/*
COPY platform-svc /app/platform-svc
EXPOSE 8080 8081 9090
ENTRYPOINT ["/usr/bin/tini", "--", "/app/platform-svc"]
EOF_DOCKER

    docker build -t ghcr.io/scaibu/platform-api:latest "${tmp_build}" > /dev/null
    rm -rf "${tmp_build}"
    rm -f /tmp/platform-svc

    log_info "Importing platform-api image into Kind cluster..."
    docker save ghcr.io/scaibu/platform-api:latest | docker exec -i "${CLUSTER_NAME}-control-plane" ctr --namespace=k8s.io images import - > /dev/null
    log_success "platform-api image imported into cluster."
}

apply_idp_project() {
    local root_dir="${SCRIPT_DIR}/../.."

    log_info "Applying modular IDP AppProject..."
    kubectl apply -f "${root_dir}/deploy/argocd/projects/platform.project.yaml"
    kubectl apply -f "${root_dir}/deploy/argocd/applications/app.dev.yaml"

    log_info "Ensuring namespace '${DEV_NAMESPACE}' exists..."
    kubectl get ns "${DEV_NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${DEV_NAMESPACE}"

    log_info "Deploying backing infrastructure & platform workloads directly via modular Kustomize dev overlay..."
    kubectl apply -k "${root_dir}/deploy/k8s/overlays/dev" --server-side --force-conflicts
    log_success "Dev environment manifests applied to cluster."
}

apply_migrations() {
    local root_dir="${SCRIPT_DIR}/../.."
    log_info "Applying database schema migrations..."
    "${root_dir}/scripts/migrate/migrate.sh"
}

wait_for_services() {
    log_info "Waiting for all cluster pods to reach Ready status..."
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=postgres -n "${DEV_NAMESPACE}" --timeout=120s
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=redis -n "${DEV_NAMESPACE}" --timeout=120s
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n "${DEV_NAMESPACE}" --timeout=120s
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=otel-collector -n "${DEV_NAMESPACE}" --timeout=120s
    kubectl wait --for=condition=available deployment/platform-api -n "${DEV_NAMESPACE}" --timeout=120s
    log_success "All application and backing services are ready."
}

start_port_forward() {
    log_info "Establishing persistent port-forwarding for all platform endpoints..."
    "${SCRIPT_DIR}/port-forward.sh" start
}

get_credentials() {
    log_info "Extracting credentials..."
    local argo_password
    argo_password=$(kubectl -n "${ARGOCD_NAMESPACE}" get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "")
    local env_file="${SCRIPT_DIR}/../../.env"
    if [ -n "${argo_password}" ] && [ -f "${env_file}" ]; then
        local argo_token
        argo_token=$(curl --noproxy "*" -sk -X POST "https://127.0.0.1:${ARGOCD_PORT}/api/v1/session" -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"${argo_password}\"}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
        if [ -n "${argo_token}" ]; then
            sed -i "s|^ARGOCD_SERVER_URL=.*|ARGOCD_SERVER_URL=https://127.0.0.1:${ARGOCD_PORT}|" "${env_file}"
            sed -i "s|^ARGOCD_TOKEN=.*|ARGOCD_TOKEN=${argo_token}|" "${env_file}"
        fi
    fi
    
    echo ""
    log_success "================================================================================"
    log_success " Enterprise Local Kubernetes & Observability Platform Ready! (Single Script)"
    log_success "================================================================================"
    log_success " Platform API Gateway : http://localhost:${PLATFORM_API_PORT} (Health: /healthz, /readyz)"
    log_success " Grafana Observability: http://localhost:${GRAFANA_PORT} (admin / admin)"
    log_success " ArgoCD GitOps Engine : https://127.0.0.1:${ARGOCD_PORT} (admin / ${argo_password})"
    log_success " OpenTelemetry OTLP   : http://localhost:${OTEL_PORT} (v1/traces, v1/logs)"
    log_success " PostgreSQL 18 DB     : 127.0.0.1:${POSTGRES_PORT} (postgres / postgres_dev_password_123)"
    log_success " Redis 7.4 In-Memory  : 127.0.0.1:${REDIS_PORT}"
    log_success "================================================================================"
    echo ""
    echo "To check pod status:"
    echo "  ./scripts/run/k8s-local.sh status"
}

down_cluster() {
    log_info "Deleting kind cluster '${CLUSTER_NAME}' and stopping port-forwards..."
    "${SCRIPT_DIR}/port-forward.sh" stop
    kind delete cluster --name "${CLUSTER_NAME}"
    log_success "kind cluster deleted."
}

main() {
    local action="${1:-up}"
    require_binaries kind kubectl docker go

    case "${action}" in
        up)
            create_cluster
            install_crd_dependencies
            install_argocd
            build_and_load_platform_api
            apply_idp_project
            wait_for_services
            start_port_forward
            apply_migrations
            get_credentials
            ;;
        down)
            down_cluster
            ;;
        status)
            echo "--- ArgoCD Pods ---"
            kubectl get pods -n "${ARGOCD_NAMESPACE}"
            echo ""
            echo "--- Platform & Backing Infrastructure Pods (dev namespace) ---"
            kubectl get pods -n "${DEV_NAMESPACE}"
            ;;
        port-forward)
            start_port_forward
            ;;
        reload|rebuild)
            build_and_load_platform_api
            kubectl rollout restart deployment/platform-api -n "${DEV_NAMESPACE}"
            kubectl rollout status deployment/platform-api -n "${DEV_NAMESPACE}" --timeout=60s
            log_success "platform-api reloaded successfully."
            ;;
        *)
            echo "Usage: $0 [up|down|status|port-forward|reload]"
            exit 1
            ;;
    esac
}

main "$@"

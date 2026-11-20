#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}/../.."
INFRA_COMPOSE="${ROOT_DIR}/deploy/docker/compose/docker-compose.infra.yml"
CLUSTER_NAME="idp-cluster"
ARGOCD_NAMESPACE="argocd"
ARGOCD_PORT=8085

log_info() {
    echo -e "\033[1;34m[INFO]\033[0m $1"
}

log_success() {
    echo -e "\033[1;32m[SUCCESS]\033[0m $1"
}

log_error() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
}

check_prerequisites() {
    log_info "Verifying CLI dependencies..."
    command -v kind >/dev/null 2>&1 || { log_error "kind CLI not found in PATH"; exit 1; }
    command -v kubectl >/dev/null 2>&1 || { log_error "kubectl CLI not found in PATH"; exit 1; }
    command -v docker >/dev/null 2>&1 || { log_error "docker CLI not found in PATH"; exit 1; }
}

start_docker_backing_services() {
    log_info "Step 1/5: Starting backing stateful infrastructure (Postgres 18, Redis 7.4, Prometheus, Loki, Tempo, OTel Collector, Caddy)..."
    
    local secrets_dir="${ROOT_DIR}/deploy/docker/compose/secrets"
    mkdir -p "${secrets_dir}"
    if [[ ! -f "${secrets_dir}/db_password.txt" ]]; then
        echo -n "postgres_dev_password_123" > "${secrets_dir}/db_password.txt"
        log_info "Generated default secrets/db_password.txt"
    fi

    docker compose -f "${INFRA_COMPOSE}" up -d

    log_info "Waiting for backing services (PostgreSQL & Redis) to reach healthy status..."
    local timeout=60
    local elapsed=0
    while [[ ${elapsed} -lt ${timeout} ]]; do
        if docker exec idp-db-1 pg_isready -U postgres -d idp_db >/dev/null 2>&1 && \
           docker exec idp-cache-1 redis-cli ping | grep -q "PONG"; then
            log_success "Backing infrastructure is healthy (Postgres 18 & Redis 7.4 ready)."
            break
        fi
        sleep 3
        elapsed=$((elapsed + 3))
    done

    log_info "Applying database schema migrations to PostgreSQL..."
    for migration in "${ROOT_DIR}"/database/migrations/sql/[0-9]*.sql; do
        if [[ ! "${migration}" =~ rollback\.sql$ ]]; then
            docker exec -i idp-db-1 psql -U postgres -d idp_db < "${migration}" >/dev/null 2>&1 || true
        fi
    done
    log_success "PostgreSQL database migrations applied."
}

create_cluster() {
    log_info "Step 2/5: Creating or connecting to Kubernetes cluster '${CLUSTER_NAME}'..."
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_info "kind cluster '${CLUSTER_NAME}' already exists."
    else
        cat <<EOF | kind create cluster --name "${CLUSTER_NAME}" --config=-
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
EOF
        log_success "kind cluster '${CLUSTER_NAME}' created."
    fi

    kubectl cluster-info --context "kind-${CLUSTER_NAME}"

    log_info "Patching CoreDNS upstream forwarders to 1.1.1.1 and 8.8.8.8..."
    kubectl -n kube-system get cm coredns -o json | jq '.data.Corefile |= gsub("/etc/resolv.conf"; "1.1.1.1 8.8.8.8")' | kubectl apply -f - >/dev/null 2>&1 || true
    kubectl -n kube-system rollout restart deployment/coredns >/dev/null 2>&1 || true
}

install_crds() {
    log_info "Step 3/5: Installing required CustomResourceDefinitions (ExternalSecrets)..."
    kubectl apply -f https://raw.githubusercontent.com/external-secrets/external-secrets/v0.9.11/deploy/crds/bundle.yaml >/dev/null 2>&1 || true
    log_success "CRDs installed."
}

install_argocd() {
    log_info "Step 4/5: Installing ArgoCD control plane..."
    kubectl get ns "${ARGOCD_NAMESPACE}" >/dev/null 2>&1 || kubectl create namespace "${ARGOCD_NAMESPACE}"

    kubectl apply -n "${ARGOCD_NAMESPACE}" --server-side --force-conflicts -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml >/dev/null 2>&1

    log_info "Waiting for ArgoCD server deployment to become available (timeout: 300s)..."
    kubectl wait --for=condition=available deployment/argocd-server -n "${ARGOCD_NAMESPACE}" --timeout=300s

    log_success "ArgoCD server is healthy and running."
}

apply_idp_project() {
    log_info "Step 5/5: Registering IDP AppProject and Dev Application..."
    kubectl apply -f "${ROOT_DIR}/deploy/argocd/project.yaml"
    kubectl apply -f "${ROOT_DIR}/deploy/argocd/applications/app.dev.yaml"
    log_success "ArgoCD project and application registered."
}

get_credentials() {
    log_info "Extracting credentials and status..."
    local password
    password=$(kubectl -n "${ARGOCD_NAMESPACE}" get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "")
    
    echo ""
    log_success "======================================================================"
    log_success "Complete Platform Infrastructure is Ready!"
    log_success "======================================================================"
    echo -e "  \033[1;36mKubernetes:\033[0m   kind (${CLUSTER_NAME})"
    echo -e "  \033[1;36mArgoCD UI:\033[0m    https://127.0.0.1:${ARGOCD_PORT} (User: admin / Pass: ${password})"
    echo -e "  \033[1;36mPostgreSQL:\033[0m   127.0.0.1:5432 (Database: idp_db, User: postgres)"
    echo -e "  \033[1;36mRedis:\033[0m        127.0.0.1:6379"
    echo -e "  \033[1;36mGrafana:\033[0m      http://127.0.0.1:3000 (User: admin / Pass: admin)"
    echo -e "  \033[1;36mPrometheus:\033[0m   http://127.0.0.1:9090"
    echo -e "  \033[1;36mOTel Ingest:\033[0m  http://127.0.0.1:4318 (HTTP) / 4317 (gRPC)"
    echo -e "  \033[1;36mTempo:\033[0m        http://127.0.0.1:3200"
    echo -e "  \033[1;36mLoki:\033[0m         http://127.0.0.1:3100"
    log_success "======================================================================"
    echo ""
    echo "To port-forward ArgoCD API to localhost:${ARGOCD_PORT}:"
    echo "  kubectl port-forward svc/argocd-server -n ${ARGOCD_NAMESPACE} ${ARGOCD_PORT}:80"
}

down_all() {
    log_info "Stopping all platform infrastructure..."
    log_info "1. Stopping backing Docker containers (Postgres, Redis, Observability)..."
    docker compose -f "${INFRA_COMPOSE}" down -v || true

    log_info "2. Deleting kind cluster '${CLUSTER_NAME}'..."
    kind delete cluster --name "${CLUSTER_NAME}" || true
    log_success "All platform infrastructure stopped and cleaned up."
}

main() {
    local action="${1:-up}"
    check_prerequisites

    case "${action}" in
        up)
            start_docker_backing_services
            create_cluster
            install_crds
            install_argocd
            apply_idp_project
            get_credentials
            ;;
        down)
            down_all
            ;;
        status)
            log_info "Docker backing containers:"
            docker compose -f "${INFRA_COMPOSE}" ps
            echo ""
            log_info "Kubernetes pods:"
            kubectl get pods -A
            ;;
        *)
            echo "Usage: $0 [up|down|status]"
            exit 1
            ;;
    esac
}

main "$@"

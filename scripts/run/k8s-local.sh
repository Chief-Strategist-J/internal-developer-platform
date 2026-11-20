#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../lib/common.sh"

CLUSTER_NAME="idp-cluster"
ARGOCD_NAMESPACE="argocd"
ARGOCD_PORT=8085
DEV_NAMESPACE="dev"

create_cluster() {
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_info "kind cluster '${CLUSTER_NAME}' already exists."
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

apply_idp_project() {
    local root_dir="${SCRIPT_DIR}/../.."

    log_info "Applying modular IDP AppProject..."
    kubectl apply -f "${root_dir}/deploy/argocd/projects/platform.project.yaml"

    log_info "Deploying backing infrastructure & platform workloads directly via modular Kustomize dev overlay..."
    kubectl apply -k "${root_dir}/deploy/k8s/overlays/dev" --server-side --force-conflicts
    log_success "Dev environment manifests applied to cluster."
}

start_port_forward() {
    log_info "Ensuring ArgoCD port-forward on port ${ARGOCD_PORT}..."
    pkill -f "kubectl port-forward svc/argocd-server -n ${ARGOCD_NAMESPACE}" 2>/dev/null || true
    nohup kubectl port-forward svc/argocd-server -n "${ARGOCD_NAMESPACE}" --address 0.0.0.0 "${ARGOCD_PORT}:443" > /tmp/argocd-port-forward.log 2>&1 &
    sleep 2
    log_success "ArgoCD port-forward active on https://127.0.0.1:${ARGOCD_PORT}"
}

get_credentials() {
    log_info "Extracting initial admin credentials..."
    local password
    password=$(kubectl -n "${ARGOCD_NAMESPACE}" get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "")
    
    echo ""
    log_success "=========================================================="
    log_success "Enterprise Local Kubernetes & ArgoCD Environment Ready!"
    log_success "Cluster:   ${CLUSTER_NAME}"
    log_success "ArgoCD:    https://127.0.0.1:${ARGOCD_PORT}"
    log_success "Username:  admin"
    log_success "Password:  ${password}"
    log_success "=========================================================="
    echo ""
    echo "To verify cluster workload status:"
    echo "  kubectl get pods -n ${DEV_NAMESPACE}"
    echo "  kubectl get pods -n ${ARGOCD_NAMESPACE}"
}

down_cluster() {
    log_info "Deleting kind cluster '${CLUSTER_NAME}'..."
    pkill -f "kubectl port-forward svc/argocd-server -n ${ARGOCD_NAMESPACE}" 2>/dev/null || true
    kind delete cluster --name "${CLUSTER_NAME}"
    log_success "kind cluster deleted."
}

main() {
    local action="${1:-up}"
    require_binaries kind kubectl docker

    case "${action}" in
        up)
            create_cluster
            install_crd_dependencies
            install_argocd
            apply_idp_project
            start_port_forward
            get_credentials
            ;;
        down)
            down_cluster
            ;;
        status)
            echo "--- ArgoCD Pods ---"
            kubectl get pods -n "${ARGOCD_NAMESPACE}"
            echo "--- Platform & Backing Infrastructure Pods ---"
            kubectl get pods -n "${DEV_NAMESPACE}"
            ;;
        port-forward)
            start_port_forward
            ;;
        *)
            echo "Usage: $0 [up|down|status|port-forward]"
            exit 1
            ;;
    esac
}

main "$@"

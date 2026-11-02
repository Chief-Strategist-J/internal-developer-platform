#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

REGISTRY="${REGISTRY:-ghcr.io/scaibu}"
TAG="${TAG:-latest}"
DOCKER_BUILDKIT=1
export DOCKER_BUILDKIT

echo "================================================================="
echo "Building Polyglot Microservices for Internal Developer Platform"
echo "Root: ${ROOT_DIR}"
echo "Registry: ${REGISTRY} | Tag: ${TAG}"
echo "================================================================="

build_service() {
    local name="$1"
    local dockerfile="$2"
    local image_name="${REGISTRY}/${name}:${TAG}"

    echo ">>> Building ${name} using ${dockerfile}..."
    docker build \
        -f "${ROOT_DIR}/${dockerfile}" \
        -t "${image_name}" \
        "${ROOT_DIR}"
}

build_service "platform-api" "deploy/docker/service/Dockerfile"
build_service "developer-portal" "deploy/docker/service/Dockerfile.typescript"
build_service "rust-worker" "deploy/docker/service/Dockerfile.rust"
build_service "cpp-engine" "deploy/docker/service/Dockerfile.cpp"

echo ""
echo "================================================================="
echo "Container Images Built Successfully - Measured Sizes:"
echo "================================================================="
printf "%-25s %-35s %-15s %-15s\n" "SERVICE" "IMAGE REPOSITORY" "SIZE (BYTES)" "HUMAN READABLE"
printf "%-25s %-35s %-15s %-15s\n" "-------" "----------------" "------------" "--------------"

for svc in "platform-api" "developer-portal" "rust-worker" "cpp-engine"; do
    img="${REGISTRY}/${svc}:${TAG}"
    size_bytes=$(docker image inspect "${img}" --format '{{.Size}}' 2>/dev/null || echo "0")
    size_mb=$(awk "BEGIN {printf \"%.2f MB\", ${size_bytes}/1048576}")
    printf "%-25s %-35s %-15s %-15s\n" "${svc}" "${img}" "${size_bytes}" "${size_mb}"
done
echo "================================================================="

#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="deploy/helm"
DIST_DIR="deploy/helm/dist"

echo "Linting Helm chart..."
helm lint "${CHART_DIR}"

echo "Packaging Helm chart..."
mkdir -p "${DIST_DIR}"
helm package "${CHART_DIR}" --destination "${DIST_DIR}"

echo "Chart successfully packaged into ${DIST_DIR}."

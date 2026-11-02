#!/usr/bin/env bash
set -euo pipefail

REGISTRY="${OCI_REGISTRY:-oci://ghcr.io/scaibu/charts}"
DIST_DIR="deploy/helm/dist"

PACKAGE_FILE=$(find "${DIST_DIR}" -name "*.tgz" | head -n 1)

if [[ -z "${PACKAGE_FILE}" ]]; then
  echo "No packaged chart found in ${DIST_DIR}. Run package.sh first." >&2
  exit 1
fi

echo "Publishing $(basename "${PACKAGE_FILE}") to OCI registry ${REGISTRY}..."
helm push "${PACKAGE_FILE}" "${REGISTRY}"

echo "Chart successfully published."

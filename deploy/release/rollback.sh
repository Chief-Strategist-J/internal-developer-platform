#!/usr/bin/env bash
set -euo pipefail

PREVIOUS_ENV_FILE="/var/lib/deploy/previous.env"

if [[ ! -f "${PREVIOUS_ENV_FILE}" ]]; then
  echo "No previous deployment recorded at ${PREVIOUS_ENV_FILE}. Cannot safely roll back." >&2
  exit 1
fi

# shellcheck source=/dev/null
source "${PREVIOUS_ENV_FILE}"
echo "Rolling back to previous digest: ${APP_DIGEST}"

export APP_DIGEST
docker compose -f deploy/docker/compose/docker-compose.polyglot.yml up -d --wait --wait-timeout 180

echo "Rollback completed. Database schema remains forward-compatible."

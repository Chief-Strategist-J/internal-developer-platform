#!/usr/bin/env bash
set -euo pipefail

APP_DIGEST="${APP_DIGEST:?APP_DIGEST must be set to a valid sha256 digest}"
REGISTRY="${REGISTRY:-ghcr.io/scaibu}"
PREVIOUS_ENV_FILE="/var/lib/deploy/previous.env"

echo "Verifying cryptographic image signature with Cosign..."
cosign verify --key "${COSIGN_KEY:-/etc/cosign/cosign.pub}" "${REGISTRY}/platform-api@${APP_DIGEST}" || {
  echo "Cosign signature verification failed!" >&2
  exit 1
}

echo "Saving current deployment state for rollback..."
mkdir -p "$(dirname "${PREVIOUS_ENV_FILE}")"
echo "APP_DIGEST=${APP_DIGEST}" > "${PREVIOUS_ENV_FILE}.new"

echo "Pulling pinned digest images..."
docker compose -f deploy/docker/compose/docker-compose.polyglot.yml pull

echo "Executing declarative schema migration..."
docker compose -f deploy/docker/compose/docker-compose.polyglot.yml run --rm migrate

echo "Rolling out polyglot containers..."
docker compose -f deploy/docker/compose/docker-compose.polyglot.yml up -d --wait --wait-timeout 180

echo "Performing smoke verification against proxy..."
if ! curl -sf http://127.0.0.1:8080/healthz >/dev/null; then
  echo "Smoke check failed! Triggering automatic rollback..." >&2
  bash deploy/release/rollback.sh
  exit 1
fi

mv "${PREVIOUS_ENV_FILE}.new" "${PREVIOUS_ENV_FILE}"
echo "Deployment successful."

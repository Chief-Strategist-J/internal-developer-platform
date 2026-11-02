#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo -e "\033[1;34m[INFO]\033[0m Executing platform integration test suites against live infrastructure..."

cd "${PROJECT_ROOT}"

# Run integration tests against real live containers
go test -v -count=1 ./src/features/role-based-access-control/tests/ -run TestLiveRedisRevocationAdapter

echo -e "\033[1;32m[SUCCESS]\033[0m All live infrastructure integration tests passed."

#!/usr/bin/env bash
# Runs business logic scenario test suite
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo -e "\033[1;34m[INFO]\033[0m Executing complete multi-tenant and privilege scenario test suite..."

cd "${PROJECT_ROOT}"

go test -v -count=1 ./src/features/role-based-access-control/tests/ -run "TestScenario"

echo -e "\033[1;32m[SUCCESS]\033[0m All scenario tests passed."

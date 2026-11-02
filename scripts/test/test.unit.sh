#!/usr/bin/env bash
# Runs unit tests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."

echo -e "\033[1;34m[INFO]\033[0m Executing complete unit test suite across all modules..."

cd "${PROJECT_ROOT}"

go test -v -count=1 ./src/features/role-based-access-control/tests/...
go test -v -count=1 ./src/features/service-catalog/tests/...

echo -e "\033[1;34m[INFO]\033[0m Executing TypeScript frontend unit tests (State-Controller-Model Triad)..."
(cd "${PROJECT_ROOT}/typescript-modules" && npm test)

echo -e "\033[1;32m[SUCCESS]\033[0m All unit tests passed cleanly."

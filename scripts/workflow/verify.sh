#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

run() {
  local name="$1"
  shift
  printf '\n==> %s\n' "$name"
  "$@"
}

run "go test" go test ./...
run "go vet" go vet ./...
run "mgsctl install wrapper fallback contract" ./scripts/test/install-wrapper-contract.sh
run "tagged release artifact contract" ./scripts/devops/release-contract-test.sh
run "mgsctl-only deployment documentation contract" ./scripts/workflow/deployment-docs-contract.sh
run "staged secret candidate path contract" ./scripts/workflow/staged-secret-candidates-contract-test.sh
run "API smoke BASE_URL safety contract" ./scripts/test/api_contract_smoke_contract_test.sh
run "shared local E2E runner safety contract" ./scripts/e2e/run-docker-e2e.contract.sh

run "frontend/shared contracts" ./scripts/workflow/verify-contracts.sh
run "docs web deployment contract" ./scripts/workflow/docs-web-contract-test.sh
run "frontend direct and prefixed path contract" ./scripts/workflow/frontend-path-contract.sh

if [ -f web/user/package.json ]; then
  run "web/user typecheck" npm --prefix web/user run typecheck
  run "web/user build" npm --prefix web/user run build
fi

if [ -f web/admin/package.json ]; then
  run "web/admin typecheck" npm --prefix web/admin run typecheck
  run "web/admin build" npm --prefix web/admin run build
fi

if [ -f web/docs/package.json ]; then
  run "web/docs typecheck" npm --prefix web/docs run typecheck
  run "web/docs build" npm --prefix web/docs run build
fi

run "Linux/Windows native package contract" ./scripts/workflow/native-package-contract.sh

printf '\nOK: verification passed\n'

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
run "local build service startup scripts" ./scripts/local/pgctl_contract_test.sh

run "frontend/shared contracts" ./scripts/workflow/verify-contracts.sh

if [ -f web/user/package.json ]; then
  run "web/user typecheck" npm --prefix web/user run typecheck
  run "web/user build" npm --prefix web/user run build
fi

if [ -f web/admin/package.json ]; then
  run "web/admin typecheck" npm --prefix web/admin run typecheck
  run "web/admin build" npm --prefix web/admin run build
fi

printf '\nOK: verification passed\n'

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

./scripts/workflow/verify.sh
./scripts/workflow/review-local.sh --scope committed
./scripts/workflow/check-review-gate.sh

changed="$(git diff --name-only HEAD~1..HEAD 2>/dev/null || git diff --name-only --cached)"
if printf '%s\n' "$changed" | grep -E '^(cmd/|internal/|pkg/|api/|configs/|deployments/|Dockerfile|Dockerfile\\.)' >/dev/null 2>&1; then
  ./scripts/workflow/api-smoke.sh
fi

echo "ship guard: OK"


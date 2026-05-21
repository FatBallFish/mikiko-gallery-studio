#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

BASE_URL="${BASE_URL:-http://localhost:8080}"

if [ -x scripts/test/smoke_readyz.sh ]; then
  scripts/test/smoke_readyz.sh "$BASE_URL"
else
  curl -fsS "$BASE_URL/readyz" >/dev/null
fi

echo "api smoke: OK ($BASE_URL)"


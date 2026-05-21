#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

git config core.hooksPath .githooks
chmod +x .githooks/* .hook-scripts/* scripts/workflow/*.sh 2>/dev/null || true

echo "OK: git core.hooksPath=.githooks"


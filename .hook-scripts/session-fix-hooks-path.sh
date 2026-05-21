#!/usr/bin/env bash
set -uo pipefail

HP="$(git config core.hooksPath 2>/dev/null || echo '')"
if [ -z "$HP" ]; then
  git config core.hooksPath .githooks >/dev/null 2>&1 \
    && echo "[auto-fix] core.hooksPath set to .githooks" >&2
elif [ "$HP" != ".githooks" ]; then
  echo "[WARN] core.hooksPath=$HP; expected .githooks. Run ./scripts/workflow/install-hooks.sh" >&2
fi

exit 0


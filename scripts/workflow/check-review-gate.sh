#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

GATE=".review/gate.json"
if [ ! -f "$GATE" ]; then
  echo "BLOCK: missing .review/gate.json. Run ./scripts/workflow/review-local.sh --scope committed" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "BLOCK: jq is required for review gate validation" >&2
  exit 1
fi

decision="$(jq -r '.decision // empty' "$GATE")"
scope="$(jq -r '.scope // empty' "$GATE")"
tree_sha="$(jq -r '.tree_sha // empty' "$GATE")"
current_tree="$(git rev-parse HEAD^{tree})"

if [ "$decision" != "PASS" ]; then
  echo "BLOCK: review gate decision is $decision" >&2
  exit 1
fi
if [ "$scope" != "committed" ]; then
  echo "BLOCK: review gate scope is $scope; expected committed" >&2
  exit 1
fi
if [ "$tree_sha" != "$current_tree" ]; then
  echo "BLOCK: review gate is stale. Run ./scripts/workflow/review-local.sh --scope committed" >&2
  exit 1
fi

echo "review gate: OK"


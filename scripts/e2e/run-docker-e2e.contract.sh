#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNNER="$ROOT_DIR/scripts/e2e/run-docker-e2e.sh"
LOCK_FILE="$ROOT_DIR/tmp/e2e/pic-gallery-local.lock"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pic-gallery-e2e-runner-contract.XXXXXX")"
LOCK_PID=""

cleanup() {
  if [[ -n "$LOCK_PID" ]]; then
    kill "$LOCK_PID" >/dev/null 2>&1 || true
    wait "$LOCK_PID" >/dev/null 2>&1 || true
  fi
  find "$TMP_DIR" -mindepth 1 -delete 2>/dev/null || true
  rmdir "$TMP_DIR" 2>/dev/null || true
}
trap cleanup EXIT

if BASE_URL=http://example.invalid "$RUNNER" >"$TMP_DIR/invalid-url.out" 2>&1; then
  echo "FAIL: shared E2E accepted a non-local API URL" >&2
  exit 1
fi
rg -q 'BASE_URL must be http://127.0.0.1:8088' "$TMP_DIR/invalid-url.out"

mkdir -p "$(dirname "$LOCK_FILE")"
if command -v flock >/dev/null 2>&1; then
  flock "$LOCK_FILE" sh -c 'touch "$1"; sleep 30' sh "$TMP_DIR/locked" &
elif command -v lockf >/dev/null 2>&1; then
  lockf "$LOCK_FILE" sh -c 'touch "$1"; sleep 30' sh "$TMP_DIR/locked" &
else
  echo "FAIL: flock or lockf is required for the runner contract" >&2
  exit 1
fi
LOCK_PID=$!

for _ in {1..40}; do
  [[ -f "$TMP_DIR/locked" ]] && break
  sleep 0.05
done
[[ -f "$TMP_DIR/locked" ]] || { echo "FAIL: test lock was not acquired" >&2; exit 1; }

if "$RUNNER" >"$TMP_DIR/concurrent.out" 2>&1; then
  echo "FAIL: shared E2E allowed a concurrent run" >&2
  exit 1
fi
rg -q 'another shared local E2E run is active' "$TMP_DIR/concurrent.out"

echo "OK: shared local E2E runner safety contract passed"

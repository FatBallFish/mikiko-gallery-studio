#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNNER="$ROOT_DIR/scripts/e2e/run-docker-e2e.sh"
RUNNER_STATE="$ROOT_DIR/scripts/e2e/local-runner-state.sh"
LOCK_FILE="$ROOT_DIR/tmp/e2e/pic-gallery-local.lock"
RECOVERY_MARKER="$ROOT_DIR/tmp/e2e/pic-gallery-local-recovery-required"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pic-gallery-e2e-runner-contract.XXXXXX")"
LOCK_CREATED=false
RECOVERY_MARKER_CREATED=false

cleanup() {
  if [[ "$LOCK_CREATED" == true && -L "$LOCK_FILE" ]]; then
    unlink "$LOCK_FILE"
  fi
  if [[ "$RECOVERY_MARKER_CREATED" == true && -f "$RECOVERY_MARKER" ]]; then
    unlink "$RECOVERY_MARKER"
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
if [[ -e "$LOCK_FILE" || -L "$LOCK_FILE" ]]; then
  owner_pid="$(readlink "$LOCK_FILE" 2>/dev/null || true)"
  if [[ "$owner_pid" =~ ^[0-9]+$ ]] && kill -0 "$owner_pid" 2>/dev/null; then
    echo "FAIL: a real shared local E2E run is already active" >&2
    exit 1
  fi
  unlink "$LOCK_FILE" >/dev/null 2>&1 || { echo "FAIL: stale E2E lock could not be removed" >&2; exit 1; }
fi
ln -s "$$" "$LOCK_FILE"
LOCK_CREATED=true

if "$RUNNER" >"$TMP_DIR/concurrent.out" 2>&1; then
  echo "FAIL: shared E2E allowed a concurrent run" >&2
  exit 1
fi
rg -q 'another shared local E2E run is active' "$TMP_DIR/concurrent.out"

if PIC_GALLERY_E2E_LOCKED=true "$RUNNER" >"$TMP_DIR/environment-bypass.out" 2>&1; then
  echo "FAIL: shared E2E lock was bypassed through the old environment sentinel" >&2
  exit 1
fi
rg -q 'another shared local E2E run is active' "$TMP_DIR/environment-bypass.out"

unlink "$LOCK_FILE"
LOCK_CREATED=false
printf 'snapshot_dir=%s\nphase=test-running\nowner_pid=999999\n' "$TMP_DIR/local-state-contract" >"$RECOVERY_MARKER"
RECOVERY_MARKER_CREATED=true
if "$RUNNER" >"$TMP_DIR/recovery-required.out" 2>&1; then
  echo "FAIL: shared E2E started while a recovery marker existed" >&2
  exit 1
fi
rg -q 'recovery required before another shared E2E run' "$TMP_DIR/recovery-required.out"
unlink "$RECOVERY_MARKER"
RECOVERY_MARKER_CREATED=false

source "$RUNNER_STATE"
COMPOSE=(fake_compose)
WRITERS_STOPPED=false
WRITER_CONTAINER_IDS=()

fake_compose() {
  local operation=$1
  local service="${*: -1}"
  if [[ "$operation" == "ps" ]]; then
    [[ "$WRITER_TEST_SCENARIO" != "ps-error" ]] || return 1
    [[ "$WRITER_TEST_SCENARIO" != "missing" ]] || return 0
    printf '%s-container\n' "$service"
    return 0
  fi
  [[ "$operation" == "stop" ]]
}

docker() {
  local container_id="${*: -1}"
  [[ "$1" == "inspect" ]] || return 2
  [[ "$WRITER_TEST_SCENARIO" != "inspect-error" ]] || return 1
  if [[ "$WRITER_TEST_SCENARIO" == "running" && "$container_id" == "worker-container" ]]; then
    printf 'true\n'
  else
    printf 'false\n'
  fi
}

for WRITER_TEST_SCENARIO in ps-error missing inspect-error running; do
  if stop_writers >/dev/null 2>&1; then
    echo "FAIL: writer stop accepted unsafe scenario: $WRITER_TEST_SCENARIO" >&2
    exit 1
  fi
done
WRITER_TEST_SCENARIO=stopped
stop_writers >/dev/null 2>&1 || { echo "FAIL: writer stop rejected three inspected stopped containers" >&2; exit 1; }

echo "OK: shared local E2E runner safety contract passed"

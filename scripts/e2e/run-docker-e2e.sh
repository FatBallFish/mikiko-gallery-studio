#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
STATE_HELPER="$ROOT_DIR/scripts/e2e/local-state.sh"
RUNNER_STATE="$ROOT_DIR/scripts/e2e/local-runner-state.sh"
SCRIPT_PATH="$ROOT_DIR/scripts/e2e/run-docker-e2e.sh"
PREPARE_RUNTIME="$ROOT_DIR/scripts/dev/prepare-local-runtime.sh"

source "$RUNNER_STATE"

fail() {
  echo "shared local E2E: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: scripts/e2e/run-docker-e2e.sh [--start]

Runs Docker E2E against the shared development API and database. Persistent
database and object state is restored when the run exits.

Options:
  --start   Start or update the E2E compose stack before testing.
  --recover Restore the retained snapshot from a failed or interrupted E2E run.

EOF
}

START_STACK=false
RECOVER_ONLY=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --start)
      START_STACK=true
      shift
      ;;
    --recover)
      RECOVER_ONLY=true
      shift
      ;;
    --clean)
      echo "--clean was removed because E2E shares persistent development data" >&2
      exit 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

cd "$ROOT_DIR"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE")
LOCAL_BASE_URL="http://127.0.0.1:8088"
BASE_URL="${BASE_URL:-$LOCAL_BASE_URL}"
USER_WEB_URL="${USER_WEB_URL:-$LOCAL_BASE_URL}"
ADMIN_WEB_URL="${ADMIN_WEB_URL:-$LOCAL_BASE_URL/admin}"
NGINX_URL="${NGINX_URL:-$LOCAL_BASE_URL}"
SNAPSHOT_DIR="$ROOT_DIR/tmp/e2e/local-state-$(date +%Y%m%d%H%M%S)-$$"
LOCK_FILE="$ROOT_DIR/tmp/e2e/pic-gallery-local.lock"
RECOVERY_MARKER="$ROOT_DIR/tmp/e2e/pic-gallery-local-recovery-required"
STATE_SNAPSHOTTED=false
WRITERS_STOPPED=false
LOCK_ACQUIRED=false
RECOVERY_IN_PROGRESS=false
E2E_CHILD_PID=""
E2E_CHILD_PGID=""
WRITER_CONTAINER_IDS=()

assert_local_url() {
  local name=$1
  local actual=$2
  local expected=$3
  [[ "$actual" == "$expected" ]] || fail "$name must be $expected because E2E restores the pic-gallery-local database"
}

assert_local_url BASE_URL "$BASE_URL" "$LOCAL_BASE_URL"
assert_local_url USER_WEB_URL "$USER_WEB_URL" "$LOCAL_BASE_URL"
assert_local_url ADMIN_WEB_URL "$ADMIN_WEB_URL" "$LOCAL_BASE_URL/admin"
assert_local_url NGINX_URL "$NGINX_URL" "$LOCAL_BASE_URL"

acquire_e2e_lock() {
  local owner_pid attempt
  mkdir -p "$(dirname "$LOCK_FILE")"
  for attempt in 1 2; do
    if ln -s "$$" "$LOCK_FILE" 2>/dev/null; then
      LOCK_ACQUIRED=true
      return 0
    fi
    owner_pid="$(readlink "$LOCK_FILE" 2>/dev/null || true)"
    if [[ "$owner_pid" =~ ^[0-9]+$ ]] && kill -0 "$owner_pid" 2>/dev/null; then
      return 1
    fi
    unlink "$LOCK_FILE" >/dev/null 2>&1 || return 1
  done
  return 1
}

release_e2e_lock() {
  local owner_pid
  [[ "$LOCK_ACQUIRED" == true ]] || return 0
  owner_pid="$(readlink "$LOCK_FILE" 2>/dev/null || true)"
  if [[ "$owner_pid" == "$$" ]]; then
    unlink "$LOCK_FILE" >/dev/null 2>&1 || true
  fi
  LOCK_ACQUIRED=false
}

write_recovery_marker() {
  local phase=$1
  local pending_marker="${RECOVERY_MARKER}.tmp.$$"
  printf 'snapshot_dir=%s\nphase=%s\nowner_pid=%s\nchild_pid=%s\nchild_pgid=%s\n' \
    "$SNAPSHOT_DIR" "$phase" "$$" "$E2E_CHILD_PID" "$E2E_CHILD_PGID" >"$pending_marker"
  mv "$pending_marker" "$RECOVERY_MARKER"
}

recovery_snapshot_dir() {
  sed -n 's/^snapshot_dir=//p' "$RECOVERY_MARKER" | head -n 1
}

clear_recovery_state() {
  # Retire the blocker before deleting the snapshot so an interruption cannot
  # leave a recovery marker pointing at a missing directory.
  unlink "$RECOVERY_MARKER"
  find "$SNAPSHOT_DIR" -mindepth 1 -delete
  rmdir "$SNAPSHOT_DIR"
}

wait_for_local_api() {
  for _ in {1..120}; do
    if curl --silent --fail --max-time 2 "$BASE_URL/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

start_writers() {
  "${COMPOSE[@]}" up -d minio api worker || return 1
  wait_for_local_api || return 1
  WRITERS_STOPPED=false
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if ! stop_e2e_children; then
    echo "E2E cleanup could not terminate the Docker E2E process group; restore skipped and recovery marker retained at $RECOVERY_MARKER" >&2
    release_e2e_lock
    exit 1
  fi
  if [[ "$RECOVERY_IN_PROGRESS" == true ]]; then
    stop_writers >/dev/null 2>&1 || true
    echo "E2E recovery did not complete; writers remain stopped where possible and recovery marker is retained at $RECOVERY_MARKER" >&2
    status=1
  elif [[ "$STATE_SNAPSHOTTED" == true ]]; then
    write_recovery_marker stopping-for-restore || true
    if stop_writers; then
      write_recovery_marker restoring || true
      if "$STATE_HELPER" restore "$SNAPSHOT_DIR"; then
        write_recovery_marker starting-writers || true
        if start_writers; then
          if ! clear_recovery_state; then
            echo "E2E state restored but recovery marker cleanup failed: $RECOVERY_MARKER" >&2
            status=1
          fi
        else
          echo "E2E restore succeeded but the local API did not become ready" >&2
          status=1
        fi
      else
        echo "E2E state restore failed; recovery snapshot retained at $SNAPSHOT_DIR" >&2
        status=1
      fi
    else
      echo "E2E writers could not be stopped; restore skipped and recovery snapshot retained at $SNAPSHOT_DIR" >&2
      status=1
      start_writers || true
    fi
  elif [[ "$WRITERS_STOPPED" == true ]]; then
    start_writers || status=1
  fi
  release_e2e_lock
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

acquire_e2e_lock || fail "another shared local E2E run is active"

if [[ "$RECOVER_ONLY" == true ]]; then
  [[ -f "$RECOVERY_MARKER" ]] || fail "no shared E2E recovery marker exists"
  recovery_child_pid="$(sed -n 's/^child_pid=//p' "$RECOVERY_MARKER" | head -n 1)"
  recovery_child_pgid="$(sed -n 's/^child_pgid=//p' "$RECOVERY_MARKER" | head -n 1)"
  if [[ "$recovery_child_pid" =~ ^[0-9]+$ ]] && kill -0 "$recovery_child_pid" 2>/dev/null; then
    fail "the interrupted E2E child process is still running: $recovery_child_pid"
  fi
  if [[ "$recovery_child_pgid" =~ ^[0-9]+$ ]] && kill -0 -- "-$recovery_child_pgid" 2>/dev/null; then
    fail "the interrupted E2E process group is still running: $recovery_child_pgid"
  fi
  SNAPSHOT_DIR="$(recovery_snapshot_dir)"
  case "$SNAPSHOT_DIR" in
    "$ROOT_DIR"/tmp/e2e/local-state-*) ;;
    *) fail "recovery marker contains an unsafe snapshot path" ;;
  esac
  [[ -d "$SNAPSHOT_DIR" ]] || fail "recovery snapshot directory is missing: $SNAPSHOT_DIR"
  RECOVERY_IN_PROGRESS=true
  write_recovery_marker recovery-stopping
  stop_writers || fail "writers could not be stopped for shared E2E recovery"
  write_recovery_marker recovering
  "$STATE_HELPER" restore "$SNAPSHOT_DIR" || fail "shared E2E recovery restore failed"
  write_recovery_marker recovery-starting-writers
  start_writers || fail "shared E2E recovery restored data but the API did not become ready"
  RECOVERY_IN_PROGRESS=false
  clear_recovery_state || fail "shared E2E recovery succeeded but marker cleanup failed"
  echo "shared local E2E: recovery completed"
  exit 0
fi

[[ ! -f "$RECOVERY_MARKER" ]] || fail "recovery required before another shared E2E run; run $SCRIPT_PATH --recover"

if [[ "$START_STACK" == true ]]; then
  "$PREPARE_RUNTIME"
  "${COMPOSE[@]}" config -q
  "${COMPOSE[@]}" up -d --build --remove-orphans
fi

wait_for_local_api || {
  echo "Shared local API is not ready at $BASE_URL; run with --start" >&2
  exit 1
}

stop_writers || fail "writers could not be stopped before the E2E snapshot"
write_recovery_marker snapshot-in-progress || fail "could not create the shared E2E recovery marker"
if ! "$STATE_HELPER" snapshot "$SNAPSHOT_DIR"; then
  unlink "$RECOVERY_MARKER" >/dev/null 2>&1 || true
  fail "shared local state snapshot failed"
fi
STATE_SNAPSHOTTED=true
write_recovery_marker test-running || fail "could not update the shared E2E recovery marker"
start_writers || {
  echo "Shared local API did not recover after the E2E snapshot" >&2
  exit 1
}

NODE_BIN="$(command -v node)" || fail "node is required for Docker E2E"
BASE_URL="$BASE_URL" \
USER_WEB_URL="$USER_WEB_URL" \
ADMIN_WEB_URL="$ADMIN_WEB_URL" \
NGINX_URL="$NGINX_URL" \
python3 -c 'import os, sys; os.setsid(); os.execv(sys.argv[1], sys.argv[1:])' \
  "$NODE_BIN" "$ROOT_DIR/scripts/e2e/docker-e2e.mjs" &
E2E_CHILD_PID=$!
E2E_CHILD_PGID=$E2E_CHILD_PID
write_recovery_marker test-running || {
  kill "$E2E_CHILD_PID" >/dev/null 2>&1 || true
  wait "$E2E_CHILD_PID" >/dev/null 2>&1 || true
  fail "could not record the Docker E2E process group"
}
set +e
wait "$E2E_CHILD_PID"
e2e_status=$?
set -e
E2E_CHILD_PID=""
E2E_CHILD_PGID=""
exit "$e2e_status"

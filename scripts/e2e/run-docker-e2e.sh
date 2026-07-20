#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
STATE_HELPER="$ROOT_DIR/scripts/e2e/local-state.sh"
RUNNER_STATE="$ROOT_DIR/scripts/e2e/local-runner-state.sh"

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

Environment:
  DEV_NGINX_PORT Shared local nginx port. Default: 8088
EOF
}

START_STACK=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --start)
      START_STACK=true
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
DEV_NGINX_PORT="${DEV_NGINX_PORT:-8088}"
LOCAL_BASE_URL="http://127.0.0.1:${DEV_NGINX_PORT}"
BASE_URL="${BASE_URL:-$LOCAL_BASE_URL}"
USER_WEB_URL="${USER_WEB_URL:-$LOCAL_BASE_URL}"
ADMIN_WEB_URL="${ADMIN_WEB_URL:-$LOCAL_BASE_URL/admin}"
NGINX_URL="${NGINX_URL:-$LOCAL_BASE_URL}"
SNAPSHOT_DIR="$ROOT_DIR/tmp/e2e/local-state-$(date +%Y%m%d%H%M%S)-$$"
LOCK_FILE="$ROOT_DIR/tmp/e2e/pic-gallery-local.lock"
STATE_SNAPSHOTTED=false
WRITERS_STOPPED=false
LOCK_ACQUIRED=false
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
  if [[ "$STATE_SNAPSHOTTED" == true ]]; then
    if stop_writers; then
      if "$STATE_HELPER" restore "$SNAPSHOT_DIR"; then
        if start_writers; then
          find "$SNAPSHOT_DIR" -mindepth 1 -delete
          rmdir "$SNAPSHOT_DIR"
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

if [[ "$START_STACK" == true ]]; then
  "${COMPOSE[@]}" config -q
  "${COMPOSE[@]}" up -d --build --remove-orphans
fi

wait_for_local_api || {
  echo "Shared local API is not ready at $BASE_URL; run with --start" >&2
  exit 1
}

stop_writers || fail "writers could not be stopped before the E2E snapshot"
"$STATE_HELPER" snapshot "$SNAPSHOT_DIR"
STATE_SNAPSHOTTED=true
start_writers || {
  echo "Shared local API did not recover after the E2E snapshot" >&2
  exit 1
}

BASE_URL="$BASE_URL" \
USER_WEB_URL="$USER_WEB_URL" \
ADMIN_WEB_URL="$ADMIN_WEB_URL" \
NGINX_URL="$NGINX_URL" \
node "$ROOT_DIR/scripts/e2e/docker-e2e.mjs"

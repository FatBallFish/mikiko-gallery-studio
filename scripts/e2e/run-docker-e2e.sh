#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
STATE_HELPER="$ROOT_DIR/scripts/e2e/local-state.sh"

usage() {
  cat <<'EOF'
Usage: scripts/e2e/run-docker-e2e.sh [--start]

Runs Docker E2E against the shared development API and database. Persistent
database and object state is restored when the run exits.

Options:
  --start   Start or update the E2E compose stack before testing.

Environment:
  BASE_URL       API base URL. Default: http://127.0.0.1:8088
  USER_WEB_URL   User frontend URL. Default: http://127.0.0.1:8088
  ADMIN_WEB_URL  Admin frontend URL. Default: http://127.0.0.1:8088/admin
  NGINX_URL      Nginx URL. Default: http://127.0.0.1:8088
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
BASE_URL="${BASE_URL:-http://127.0.0.1:8088}"
USER_WEB_URL="${USER_WEB_URL:-http://127.0.0.1:8088}"
ADMIN_WEB_URL="${ADMIN_WEB_URL:-http://127.0.0.1:8088/admin}"
NGINX_URL="${NGINX_URL:-http://127.0.0.1:8088}"
SNAPSHOT_DIR="$ROOT_DIR/tmp/e2e/local-state-$(date +%Y%m%d%H%M%S)-$$"
STATE_SNAPSHOTTED=false
WRITERS_STOPPED=false

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
  "${COMPOSE[@]}" up -d minio api worker
  WRITERS_STOPPED=false
  wait_for_local_api
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ "$STATE_SNAPSHOTTED" == true ]]; then
    "${COMPOSE[@]}" stop api worker minio >/dev/null || status=1
    WRITERS_STOPPED=true
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
  elif [[ "$WRITERS_STOPPED" == true ]]; then
    start_writers || status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

if [[ "$START_STACK" == true ]]; then
  "${COMPOSE[@]}" config -q
  "${COMPOSE[@]}" up -d --build --remove-orphans
fi

wait_for_local_api || {
  echo "Shared local API is not ready at $BASE_URL; run with --start" >&2
  exit 1
}

"${COMPOSE[@]}" stop api worker minio >/dev/null
WRITERS_STOPPED=true
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

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.e2e.yml"

usage() {
  cat <<'EOF'
Usage: scripts/e2e/run-docker-e2e.sh [--start] [--clean]

Runs the Docker/Postgres E2E suite against the local compose stack.

Options:
  --start   Start or update the E2E compose stack before testing.
  --clean   When used with --start, recreate E2E volumes first.

Environment:
  BASE_URL       API base URL. Default: http://127.0.0.1:18080
  USER_WEB_URL   User frontend URL. Default: http://127.0.0.1:5173
  ADMIN_WEB_URL  Admin frontend URL. Default: http://127.0.0.1:5174
  NGINX_URL      Nginx URL. Default: http://127.0.0.1:18081
EOF
}

START_STACK=false
CLEAN_STACK=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --start)
      START_STACK=true
      shift
      ;;
    --clean)
      CLEAN_STACK=true
      shift
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

if [[ "$START_STACK" == true ]]; then
  if [[ "$CLEAN_STACK" == true ]]; then
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans
  fi
  docker compose -f "$COMPOSE_FILE" up -d --remove-orphans
fi

BASE_URL="${BASE_URL:-http://127.0.0.1:18080}" \
USER_WEB_URL="${USER_WEB_URL:-http://127.0.0.1:5173}" \
ADMIN_WEB_URL="${ADMIN_WEB_URL:-http://127.0.0.1:5174}" \
NGINX_URL="${NGINX_URL:-http://127.0.0.1:18081}" \
node "$ROOT_DIR/scripts/e2e/docker-e2e.mjs"

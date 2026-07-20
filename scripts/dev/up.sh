#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"
MODE="${1:-fullstack}"

case "$MODE" in
  fullstack)
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --build --remove-orphans
    ;;
  middleware)
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d postgres redis minio mailpit
    ;;
  *)
    echo "Unknown dev compose mode: $MODE (expected fullstack or middleware)" >&2
    exit 2
    ;;
esac

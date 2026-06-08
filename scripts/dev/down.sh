#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.dev.yml"
MIDDLEWARE_COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose-middileware.yml"

if [[ "${1:-}" == "--volumes" || "${1:-}" == "-v" ]]; then
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down -v --remove-orphans
  docker compose --env-file "$ENV_FILE" -f "$MIDDLEWARE_COMPOSE_FILE" down -v --remove-orphans
else
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down --remove-orphans
  docker compose --env-file "$ENV_FILE" -f "$MIDDLEWARE_COMPOSE_FILE" down --remove-orphans
fi

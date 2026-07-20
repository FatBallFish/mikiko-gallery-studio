#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"

if [[ "${1:-}" == "--volumes" && "${2:-}" == "--confirm-destroy-local-data" ]]; then
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down -v --remove-orphans
elif [[ "${1:-}" == "--volumes" || "${1:-}" == "-v" ]]; then
  echo "Refusing to delete local data without: --volumes --confirm-destroy-local-data" >&2
  exit 2
else
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down --remove-orphans
fi

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.dev.yml"

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d

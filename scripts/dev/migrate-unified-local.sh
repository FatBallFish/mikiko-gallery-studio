#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
STATE_HELPER="$ROOT_DIR/scripts/e2e/local-state.sh"
BACKUP_ROOT="$ROOT_DIR/tmp/docker-migration"
BACKUP_DIR="$BACKUP_ROOT/$(date +%Y%m%d%H%M%S)-$$"

OLD_POSTGRES_CONTAINER=pic-gallery-dev-postgres-1
OLD_POSTGRES_VOLUME=pic-gallery-dev_postgres-data
OLD_MINIO_VOLUME=pic-gallery-dev_minio-data
OLD_STORAGE_VOLUME=pic-gallery-dev_shared-storage
TEMP_POSTGRES_CONTAINER=pic-gallery-local-migration-source-postgres
NEW_POSTGRES_VOLUME=pic-gallery-local_postgres-data
NEW_MINIO_VOLUME=pic-gallery-local_minio-data
NEW_STORAGE_VOLUME=pic-gallery-local_shared-storage
SOURCE_WAS_RUNNING=false
USING_TEMP_SOURCE=false
OLD_WRITER_IDS=()
DEV_NGINX_PORT="${DEV_NGINX_PORT:-8088}"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE")

fail() {
  echo "local migration: $*" >&2
  exit 1
}

volume_exists() {
  docker volume inspect "$1" >/dev/null 2>&1
}

container_running() {
  [[ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null || true)" == "true" ]]
}

wait_for_database() {
  local container=$1
  for _ in {1..80}; do
    if docker exec "$container" psql -U postgres -d pic_gallery -Atc 'select 1' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

wait_for_api() {
  for _ in {1..180}; do
    if curl --silent --fail --max-time 2 "http://127.0.0.1:${DEV_NGINX_PORT}/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

stop_old_dev_writers() {
  local service container_id
  for service in api worker minio; do
    while IFS= read -r container_id; do
      [[ -n "$container_id" ]] || continue
      if container_running "$container_id"; then
        OLD_WRITER_IDS+=("$container_id")
        docker stop "$container_id" >/dev/null
      fi
    done < <(docker ps -aq \
      --filter "label=com.docker.compose.project=pic-gallery-dev" \
      --filter "label=com.docker.compose.service=$service")
  done
  for container_id in "${OLD_WRITER_IDS[@]}"; do
    container_running "$container_id" && fail "old writer is still running after stop: $container_id"
  done
}

restart_old_dev_writers() {
  local container_id
  for container_id in "${OLD_WRITER_IDS[@]}"; do
    docker inspect "$container_id" >/dev/null 2>&1 || continue
    docker start "$container_id" >/dev/null 2>&1 || true
  done
}

stop_source_postgres() {
  if [[ "$USING_TEMP_SOURCE" == true ]]; then
    docker rm -f "$TEMP_POSTGRES_CONTAINER" >/dev/null 2>&1 || true
  elif [[ "$SOURCE_WAS_RUNNING" == false ]] && container_running "$OLD_POSTGRES_CONTAINER"; then
    docker stop "$OLD_POSTGRES_CONTAINER" >/dev/null
  fi
}

cleanup_source() {
  stop_source_postgres
  restart_old_dev_writers
}

remove_project_runtime() {
  local project=$1
  local ids
  ids="$(docker ps -aq --filter "label=com.docker.compose.project=$project")"
  if [[ -n "$ids" ]]; then
    docker rm -f $ids >/dev/null
  fi
  docker network rm "${project}_default" >/dev/null 2>&1 || true
}

[[ "${1:-}" == "--execute" && $# -eq 1 ]] || fail "usage: $0 --execute"
for volume in "$OLD_POSTGRES_VOLUME" "$OLD_MINIO_VOLUME" "$OLD_STORAGE_VOLUME"; do
  volume_exists "$volume" || fail "required old dev volume is missing: $volume"
done
for volume in "$NEW_POSTGRES_VOLUME" "$NEW_MINIO_VOLUME" "$NEW_STORAGE_VOLUME"; do
  if volume_exists "$volume"; then
    fail "new local volume already exists; refusing to overwrite it: $volume"
  fi
done

mkdir -p "$BACKUP_DIR"
trap cleanup_source EXIT
stop_old_dev_writers

if docker inspect "$OLD_POSTGRES_CONTAINER" >/dev/null 2>&1; then
  if container_running "$OLD_POSTGRES_CONTAINER"; then
    SOURCE_WAS_RUNNING=true
  else
    docker start "$OLD_POSTGRES_CONTAINER" >/dev/null
  fi
else
  docker run -d --name "$TEMP_POSTGRES_CONTAINER" \
    -e POSTGRES_DB=pic_gallery \
    -e POSTGRES_USER=postgres \
    -e POSTGRES_HOST_AUTH_METHOD=trust \
    -v "$OLD_POSTGRES_VOLUME:/var/lib/postgresql/data" \
    postgres:16-alpine >/dev/null
  OLD_POSTGRES_CONTAINER="$TEMP_POSTGRES_CONTAINER"
  USING_TEMP_SOURCE=true
fi
wait_for_database "$OLD_POSTGRES_CONTAINER" || fail "old dev database did not become ready"

PIC_GALLERY_LOCAL_POSTGRES_CONTAINER="$OLD_POSTGRES_CONTAINER" \
PIC_GALLERY_LOCAL_POSTGRES_USER=postgres \
PIC_GALLERY_LOCAL_POSTGRES_DB=pic_gallery \
PIC_GALLERY_LOCAL_REDIS_CONTAINER=missing-migration-redis \
PIC_GALLERY_LOCAL_MINIO_VOLUME="$OLD_MINIO_VOLUME" \
PIC_GALLERY_LOCAL_STORAGE_VOLUME="$OLD_STORAGE_VOLUME" \
  "$STATE_HELPER" snapshot "$BACKUP_DIR"

# Validate pg_dump output independently before changing any runtime resources.
docker run --rm -v "$BACKUP_DIR:/backup:ro" postgres:16-alpine pg_restore --list /backup/database.dump >/dev/null
[[ -f "$BACKUP_DIR/database-manifest.tsv" ]] || fail "database-manifest.tsv is missing"
[[ -f "$BACKUP_DIR/minio-manifest.sha256" ]] || fail "minio-manifest.sha256 is missing"
[[ -f "$BACKUP_DIR/shared-storage-manifest.sha256" ]] || fail "shared-storage-manifest.sha256 is missing"
stop_source_postgres
trap - EXIT

remove_project_runtime pic-gallery-dev
remove_project_runtime pic-gallery-e2e

docker volume create "$NEW_POSTGRES_VOLUME" >/dev/null
docker volume create "$NEW_MINIO_VOLUME" >/dev/null
docker volume create "$NEW_STORAGE_VOLUME" >/dev/null
"${COMPOSE[@]}" up -d postgres
wait_for_database pic-gallery-local-postgres-1 || fail "new local database did not become ready"

"$STATE_HELPER" restore "$BACKUP_DIR"
"${COMPOSE[@]}" up -d --build --remove-orphans
wait_for_api || fail "new local API did not become ready at http://127.0.0.1:${DEV_NGINX_PORT}"

echo "local migration: restored database-manifest.tsv, minio-manifest.sha256, and shared-storage-manifest.sha256"
echo "local migration: backup retained at $BACKUP_DIR"
echo "local migration: old volumes retained until API and E2E validation completes"
echo "local migration: after validation, remove only the old volumes with:"
echo "docker volume rm pic-gallery-dev_postgres-data pic-gallery-dev_minio-data pic-gallery-dev_redis-data pic-gallery-dev_shared-storage pic-gallery-e2e_postgres-data pic-gallery-e2e_minio-data pic-gallery-e2e_redis-data pic-gallery-e2e_shared-storage pic-gallery-e2e_admin-node-modules pic-gallery-e2e_docs-node-modules pic-gallery-e2e_go-cache pic-gallery-e2e_user-node-modules"
echo "local migration: new project pic-gallery-local is ready"

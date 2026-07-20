#!/usr/bin/env bash
set -euo pipefail

POSTGRES_CONTAINER="${PIC_GALLERY_LOCAL_POSTGRES_CONTAINER:-pic-gallery-local-postgres-1}"
POSTGRES_USER="${PIC_GALLERY_LOCAL_POSTGRES_USER:-postgres}"
POSTGRES_DB="${PIC_GALLERY_LOCAL_POSTGRES_DB:-pic_gallery}"
REDIS_CONTAINER="${PIC_GALLERY_LOCAL_REDIS_CONTAINER:-pic-gallery-local-redis-1}"
MINIO_VOLUME="${PIC_GALLERY_LOCAL_MINIO_VOLUME:-pic-gallery-local_minio-data}"
STORAGE_VOLUME="${PIC_GALLERY_LOCAL_STORAGE_VOLUME:-pic-gallery-local_shared-storage}"

fail() {
  echo "local state: $*" >&2
  exit 1
}

require_running_container() {
  local container=$1
  [[ "$(docker inspect -f '{{.State.Running}}' "$container" 2>/dev/null || true)" == "true" ]] \
    || fail "container is not running: $container"
}

require_volume() {
  docker volume inspect "$1" >/dev/null 2>&1 || fail "volume does not exist: $1"
}

database_manifest() {
  local output=$1
  local table count
  : >"$output"
  while IFS= read -r table; do
    [[ -n "$table" ]] || continue
    count="$(docker exec "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select count(*) from $table")"
    printf '%s\t%s\n' "$table" "$count" >>"$output"
  done < <(docker exec "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc \
    "select format('%I.%I', schemaname, relname) from pg_stat_user_tables order by 1")
}

volume_manifest() {
  local volume=$1
  local output=$2
  docker run --rm -v "$volume:/source:ro" alpine:3.21 sh -c \
    'cd /source && find . -type f -print | sort | while IFS= read -r file; do sha256sum "$file"; done' >"$output"
}

archive_volume() {
  local volume=$1
  local directory=$2
  local archive=$3
  docker run --rm -v "$volume:/source:ro" -v "$directory:/backup" alpine:3.21 \
    tar -C /source -czf "/backup/$archive" .
}

validate_snapshot() {
  local directory=$1
  [[ -f "$directory/snapshot.ready" ]] || fail "snapshot marker is missing: $directory"
  for file in database.dump database-manifest.tsv minio-data.tar.gz minio-manifest.sha256 shared-storage.tar.gz shared-storage-manifest.sha256; do
    [[ -f "$directory/$file" ]] || fail "snapshot artifact is missing: $file"
  done
  docker run --rm -v "$directory:/backup:ro" postgres:16-alpine \
    pg_restore --list /backup/database.dump >/dev/null
  docker run --rm -v "$directory:/backup:ro" alpine:3.21 tar -tzf /backup/minio-data.tar.gz >/dev/null
  docker run --rm -v "$directory:/backup:ro" alpine:3.21 tar -tzf /backup/shared-storage.tar.gz >/dev/null
}

snapshot() {
  local requested_directory=$1
  local directory
  mkdir -p "$requested_directory"
  directory="$(cd "$requested_directory" && pwd)"
  [[ -z "$(find "$directory" -mindepth 1 -maxdepth 1 -print -quit)" ]] || fail "snapshot directory must be empty: $directory"
  require_running_container "$POSTGRES_CONTAINER"
  require_volume "$MINIO_VOLUME"
  require_volume "$STORAGE_VOLUME"

  docker exec "$POSTGRES_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc >"$directory/database.dump"
  database_manifest "$directory/database-manifest.tsv"
  archive_volume "$MINIO_VOLUME" "$directory" minio-data.tar.gz
  volume_manifest "$MINIO_VOLUME" "$directory/minio-manifest.sha256"
  archive_volume "$STORAGE_VOLUME" "$directory" shared-storage.tar.gz
  volume_manifest "$STORAGE_VOLUME" "$directory/shared-storage-manifest.sha256"
  printf 'ready\n' >"$directory/snapshot.ready"
  validate_snapshot "$directory"
  echo "local state: snapshot ready at $directory"
}

restore_volume() {
  local volume=$1
  local directory=$2
  local archive=$3
  docker run --rm -v "$volume:/target" -v "$directory:/backup:ro" alpine:3.21 sh -c \
    "find /target -mindepth 1 -maxdepth 1 -exec rm -rf {} + && tar -C /target -xzf /backup/$archive"
}

assert_manifest() {
  local expected=$1
  local actual=$2
  local label=$3
  if ! cmp -s "$expected" "$actual"; then
    diff -u "$expected" "$actual" >&2 || true
    fail "$label manifest differs after restore"
  fi
}

restore() {
  local requested_directory=$1
  local directory database_actual minio_actual storage_actual
  [[ -d "$requested_directory" ]] || fail "snapshot directory does not exist: $requested_directory"
  directory="$(cd "$requested_directory" && pwd)"
  validate_snapshot "$directory"
  require_running_container "$POSTGRES_CONTAINER"
  require_volume "$MINIO_VOLUME"
  require_volume "$STORAGE_VOLUME"

  docker exec "$POSTGRES_CONTAINER" psql -U "$POSTGRES_USER" -d postgres -v ON_ERROR_STOP=1 -c \
    "select pg_terminate_backend(pid) from pg_stat_activity where datname = '$POSTGRES_DB' and pid <> pg_backend_pid();" >/dev/null
  docker exec "$POSTGRES_CONTAINER" dropdb -U "$POSTGRES_USER" --if-exists "$POSTGRES_DB"
  docker exec "$POSTGRES_CONTAINER" createdb -U "$POSTGRES_USER" "$POSTGRES_DB"
  docker exec -i "$POSTGRES_CONTAINER" pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner <"$directory/database.dump"
  restore_volume "$MINIO_VOLUME" "$directory" minio-data.tar.gz
  restore_volume "$STORAGE_VOLUME" "$directory" shared-storage.tar.gz

  if [[ "$(docker inspect -f '{{.State.Running}}' "$REDIS_CONTAINER" 2>/dev/null || true)" == "true" ]]; then
    docker exec "$REDIS_CONTAINER" redis-cli FLUSHDB >/dev/null
  fi

  database_actual="$(mktemp "${TMPDIR:-/tmp}/pic-gallery-database-manifest.XXXXXX")"
  minio_actual="$(mktemp "${TMPDIR:-/tmp}/pic-gallery-minio-manifest.XXXXXX")"
  storage_actual="$(mktemp "${TMPDIR:-/tmp}/pic-gallery-storage-manifest.XXXXXX")"
  database_manifest "$database_actual"
  volume_manifest "$MINIO_VOLUME" "$minio_actual"
  volume_manifest "$STORAGE_VOLUME" "$storage_actual"
  assert_manifest "$directory/database-manifest.tsv" "$database_actual" database
  assert_manifest "$directory/minio-manifest.sha256" "$minio_actual" minio
  assert_manifest "$directory/shared-storage-manifest.sha256" "$storage_actual" shared-storage
  find "$database_actual" "$minio_actual" "$storage_actual" -type f -delete
  echo "local state: restore verified from $directory"
}

case "${1:-}" in
  snapshot)
    [[ $# -eq 2 ]] || fail "usage: $0 snapshot <directory>"
    snapshot "$2"
    ;;
  restore)
    [[ $# -eq 2 ]] || fail "usage: $0 restore <directory>"
    restore "$2"
    ;;
  *)
    fail "usage: $0 <snapshot|restore> <directory>"
    ;;
esac

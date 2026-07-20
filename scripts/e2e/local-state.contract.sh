#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT_DIR/scripts/e2e/local-state.sh"
RUN_ID="$(date +%s)-$$"
POSTGRES_CONTAINER="pic-gallery-state-contract-postgres-$RUN_ID"
MINIO_VOLUME="pic-gallery-state-contract-minio-$RUN_ID"
STORAGE_VOLUME="pic-gallery-state-contract-storage-$RUN_ID"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pic-gallery-state-contract.XXXXXX")"

cleanup() {
  docker rm -f "$POSTGRES_CONTAINER" >/dev/null 2>&1 || true
  docker volume rm "$MINIO_VOLUME" "$STORAGE_VOLUME" >/dev/null 2>&1 || true
  find "$TMP_DIR" -mindepth 1 -delete 2>/dev/null || true
  rmdir "$TMP_DIR" 2>/dev/null || true
}
trap cleanup EXIT

if [[ ! -x "$HELPER" ]]; then
  echo "FAIL: local state helper is missing or not executable: $HELPER" >&2
  exit 1
fi

docker volume create "$MINIO_VOLUME" >/dev/null
docker volume create "$STORAGE_VOLUME" >/dev/null
docker run -d --name "$POSTGRES_CONTAINER" \
  -e POSTGRES_DB=pic_gallery \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_HOST_AUTH_METHOD=trust \
  postgres:16-alpine >/dev/null

for _ in {1..40}; do
  if docker exec "$POSTGRES_CONTAINER" psql -U postgres -d pic_gallery -Atc 'select 1' >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
docker exec "$POSTGRES_CONTAINER" psql -U postgres -d pic_gallery -Atc 'select 1' >/dev/null
docker exec "$POSTGRES_CONTAINER" psql -U postgres -d pic_gallery -v ON_ERROR_STOP=1 \
  -c "create table state_probe (id integer primary key, value text not null); insert into state_probe values (1, 'before');" >/dev/null
docker run --rm -v "$MINIO_VOLUME:/target" alpine:3.21 sh -c "mkdir -p /target/bucket && printf before >/target/bucket/object.txt"
docker run --rm -v "$STORAGE_VOLUME:/target" alpine:3.21 sh -c "mkdir -p /target/images && printf before >/target/images/object.txt"

export PIC_GALLERY_LOCAL_POSTGRES_CONTAINER="$POSTGRES_CONTAINER"
export PIC_GALLERY_LOCAL_POSTGRES_USER=postgres
export PIC_GALLERY_LOCAL_POSTGRES_DB=pic_gallery
export PIC_GALLERY_LOCAL_MINIO_VOLUME="$MINIO_VOLUME"
export PIC_GALLERY_LOCAL_STORAGE_VOLUME="$STORAGE_VOLUME"
export PIC_GALLERY_LOCAL_REDIS_CONTAINER=missing-state-contract-redis

"$HELPER" snapshot "$TMP_DIR/snapshot"

docker exec "$POSTGRES_CONTAINER" psql -U postgres -d pic_gallery -v ON_ERROR_STOP=1 \
  -c "update state_probe set value = 'after' where id = 1;" >/dev/null
docker run --rm -v "$MINIO_VOLUME:/target" alpine:3.21 sh -c "printf after >/target/bucket/object.txt"
docker run --rm -v "$STORAGE_VOLUME:/target" alpine:3.21 sh -c "printf after >/target/images/object.txt"

"$HELPER" restore "$TMP_DIR/snapshot"

database_value="$(docker exec "$POSTGRES_CONTAINER" psql -U postgres -d pic_gallery -Atc "select value from state_probe where id = 1")"
minio_value="$(docker run --rm -v "$MINIO_VOLUME:/target:ro" alpine:3.21 cat /target/bucket/object.txt)"
storage_value="$(docker run --rm -v "$STORAGE_VOLUME:/target:ro" alpine:3.21 cat /target/images/object.txt)"
[[ "$database_value" == "before" ]]
[[ "$minio_value" == "before" ]]
[[ "$storage_value" == "before" ]]

cp -R "$TMP_DIR/snapshot" "$TMP_DIR/corrupt-database"
printf 'not a PostgreSQL dump\n' >"$TMP_DIR/corrupt-database/database.dump"
if "$HELPER" restore "$TMP_DIR/corrupt-database" >/dev/null 2>&1; then
  echo "FAIL: restore accepted a corrupt database dump" >&2
  exit 1
fi

cp -R "$TMP_DIR/snapshot" "$TMP_DIR/corrupt-minio"
printf 'not a tar archive\n' >"$TMP_DIR/corrupt-minio/minio-data.tar.gz"
if "$HELPER" restore "$TMP_DIR/corrupt-minio" >/dev/null 2>&1; then
  echo "FAIL: restore accepted a corrupt MinIO archive" >&2
  exit 1
fi

database_value="$(docker exec "$POSTGRES_CONTAINER" psql -U postgres -d pic_gallery -Atc "select value from state_probe where id = 1")"
minio_value="$(docker run --rm -v "$MINIO_VOLUME:/target:ro" alpine:3.21 cat /target/bucket/object.txt)"
storage_value="$(docker run --rm -v "$STORAGE_VOLUME:/target:ro" alpine:3.21 cat /target/images/object.txt)"
[[ "$database_value" == "before" ]]
[[ "$minio_value" == "before" ]]
[[ "$storage_value" == "before" ]]

if "$HELPER" restore "$TMP_DIR/missing" >/dev/null 2>&1; then
  echo "FAIL: restore accepted a missing snapshot" >&2
  exit 1
fi

echo "OK: local state snapshot/restore contract passed"

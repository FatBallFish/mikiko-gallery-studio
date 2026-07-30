#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deployments/docker-compose/docker-compose.local.yml"
ENV_FILE="$ROOT_DIR/deployments/docker-compose/.env.example"
PROJECT="local-bootstrap-lifecycle-$$"
TEST_IMAGE_TAG="local-bootstrap-lifecycle-$$"
TEST_ROOT="$ROOT_DIR/tmp/$PROJECT"
CONFIG_DIR="$TEST_ROOT/config"

case "$PROJECT" in
  local-bootstrap-lifecycle-[0-9]*) ;;
  *) echo "unsafe lifecycle Compose project name" >&2; exit 2 ;;
esac

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ "$status" != "0" ]]; then
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" -p "$PROJECT" ps >&2 || true
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" -p "$PROJECT" logs --no-color bootstrap-local api >&2 || true
  fi
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || status=1
  docker image rm "pic-gallery-api:$TEST_IMAGE_TAG" >/dev/null 2>&1 || true
  if [[ -d "$TEST_ROOT" ]]; then
    find "$TEST_ROOT" -depth -mindepth 1 -delete >/dev/null 2>&1 || status=1
    rmdir "$TEST_ROOT" >/dev/null 2>&1 || status=1
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

mkdir -p "$CONFIG_DIR"
install -m 600 "$ROOT_DIR/config/runtime.local.env.example" "$CONFIG_DIR/runtime.env"
install -m 600 "$ROOT_DIR/config/install-state.local.json.example" "$CONFIG_DIR/install-state.json"
host_owner="$(if stat -f '%u:%g' "$CONFIG_DIR/runtime.env" >/dev/null 2>&1; then stat -f '%u:%g' "$CONFIG_DIR/runtime.env"; else stat -c '%u:%g' "$CONFIG_DIR/runtime.env"; fi)"

export PIC_GALLERY_LOCAL_CONFIG_DIR="$CONFIG_DIR"
export IMAGE_TAG="$TEST_IMAGE_TAG"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" -p "$PROJECT")

file_mode() {
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

wait_for_api() {
  local container status
  container="$("${COMPOSE[@]}" ps -q api)"
  [[ -n "$container" ]] || return 1
  for _ in {1..120}; do
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container")"
    [[ "$status" == "healthy" ]] && return 0
    [[ "$status" != "exited" && "$status" != "dead" ]] || return 1
    sleep 1
  done
  return 1
}

psql_value() {
  "${COMPOSE[@]}" exec -T postgres psql -X -qAt -U postgres -d pic_gallery -v ON_ERROR_STOP=1 -c "$1"
}

wait_for_database() {
  for _ in {1..80}; do
    if [[ "$(psql_value 'select 1' 2>/dev/null || true)" == "1" ]]; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

"${COMPOSE[@]}" config | grep -q 'service_completed_successfully'
"${COMPOSE[@]}" up -d --build api
wait_for_api

[[ "$(file_mode "$CONFIG_DIR/runtime.env")" == "600" ]]
[[ "$(file_mode "$CONFIG_DIR/install-state.json")" == "600" ]]
api_container="$("${COMPOSE[@]}" ps -q api)"
[[ "$(docker exec "$api_container" awk '/^Uid:/{print $2}' /proc/1/status)" != "0" ]]
[[ "$(docker exec "$api_container" stat -c '%a|%U' /app/config/runtime.env)" == "600|picgallery" ]]
[[ "$(docker exec "$api_container" stat -c '%a|%U' /app/config/install-state.json)" == "600|picgallery" ]]
[[ "$(if stat -f '%u:%g' "$CONFIG_DIR/install-state.json" >/dev/null 2>&1; then stat -f '%u:%g' "$CONFIG_DIR/install-state.json"; else stat -c '%u:%g' "$CONFIG_DIR/install-state.json"; fi)" == "$host_owner" ]]
[[ "$(psql_value "select email || '|' || role || '|' || status from admin_users order by id;")" == "admin@example.com|super_admin|active" ]]
[[ "$(psql_value "select setup_operation_id || '|' || setup_config_revision::text from installations;")" == "local-bootstrap|1" ]]

original_hash="$(psql_value "select password_hash from admin_users where email = 'admin@example.com';")"
original_digest="$(psql_value "select setup_request_digest from installations;")"
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null
install -m 600 "$ROOT_DIR/config/runtime.local.env.example" "$CONFIG_DIR/runtime.env"
install -m 600 "$ROOT_DIR/config/install-state.local.json.example" "$CONFIG_DIR/install-state.json"
"${COMPOSE[@]}" up -d postgres redis >/dev/null
wait_for_database
"${COMPOSE[@]}" run --rm --no-deps --entrypoint mikiko-gallery-studio-db-migrate bootstrap-local >/dev/null
"${COMPOSE[@]}" exec -T postgres psql -X -q -U postgres -d pic_gallery -v ON_ERROR_STOP=1 -v "password_hash=$original_hash" <<'SQL'
INSERT INTO admin_users (created_at, updated_at, email, password_hash, role, status)
VALUES (CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'alternate-root@example.com', :'password_hash', 'super_admin', 'active');
SQL
"${COMPOSE[@]}" up -d api >/dev/null
wait_for_api
[[ "$(psql_value "select email from admin_users where role = 'super_admin' and status = 'active';")" == "alternate-root@example.com" ]]
[[ "$(psql_value "select password_hash from admin_users where email = 'alternate-root@example.com';")" == "$original_hash" ]]
[[ "$(psql_value "select count(*) from admin_users;")" == "1" ]]
alternate_digest="$(psql_value "select setup_request_digest from installations;")"
[[ "$alternate_digest" != "$original_digest" ]]
grep -q "\"request_digest\": \"$alternate_digest\"" "$CONFIG_DIR/install-state.json"

state_checksum="$(cksum "$CONFIG_DIR/install-state.json")"
"${COMPOSE[@]}" up -d --force-recreate bootstrap-local api >/dev/null
wait_for_api
[[ "$(cksum "$CONFIG_DIR/install-state.json")" == "$state_checksum" ]]
PIC_GALLERY_LOCAL_CONFIG_DIR="$CONFIG_DIR" "$ROOT_DIR/scripts/dev/prepare-local-runtime.sh"

cp "$CONFIG_DIR/runtime.env" "$CONFIG_DIR/runtime.env.valid"
sed 's#postgres:5432/pic_gallery#postgres:5432/other#' "$CONFIG_DIR/runtime.env.valid" >"$CONFIG_DIR/runtime.env"
migration_marker="$(psql_value "select migrated_at::text || '|' || app_version from installations;")"
if "${COMPOSE[@]}" run --rm --no-deps bootstrap-local >/dev/null 2>&1; then
  echo "local bootstrap accepted a mismatched database name" >&2
  exit 1
fi
[[ "$(psql_value "select migrated_at::text || '|' || app_version from installations;")" == "$migration_marker" ]]
mv "$CONFIG_DIR/runtime.env.valid" "$CONFIG_DIR/runtime.env"
"${COMPOSE[@]}" run --rm --no-deps bootstrap-local >/dev/null

echo "OK: isolated local bootstrap lifecycle passed"

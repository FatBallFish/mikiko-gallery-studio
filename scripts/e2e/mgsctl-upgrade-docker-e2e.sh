#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/e2e/deployment-e2e-lib.sh"

assert_safe_runtime() {
  local candidate repository_runtime
  candidate="$(cd "$(dirname "$1")" 2>/dev/null && pwd -P)/$(basename "$1")"
  repository_runtime="$ROOT_DIR/runtime"
  case "$candidate" in
    "$repository_runtime"|"$repository_runtime"/*)
      deployment_e2e_fail "refusing to use the repository runtime/ directory: $candidate"
      ;;
  esac
}

if [[ "${1:-}" == "--contract-only" ]]; then
  if assert_safe_runtime "$ROOT_DIR/runtime" >/dev/null 2>&1; then
    deployment_e2e_fail "runtime safety guard accepted the repository runtime/ directory"
  fi
  assert_safe_runtime "$ROOT_DIR/tmp/e2e/contract-only-runtime"
  echo "OK: mgsctl Docker upgrade E2E contract passed"
  exit 0
fi
[[ $# -eq 0 ]] || deployment_e2e_fail "usage: $0 [--contract-only]"

for command in docker curl jq python3 go; do
  command -v "$command" >/dev/null 2>&1 || deployment_e2e_fail "$command is required"
done
docker version >/dev/null
docker compose version >/dev/null

RUN_ID="$(date +%Y%m%d%H%M%S)-$$"
E2E_PREFIX="mgsctl-upgrade-e2e-${RUN_ID}"
mkdir -p "$ROOT_DIR/tmp/e2e"
E2E_ROOT="$(mktemp -d "$ROOT_DIR/tmp/e2e/${E2E_PREFIX}.XXXXXX")"
RUNTIME_DIR="$E2E_ROOT/installation"
OUTSIDE_DIR="$E2E_ROOT/outside-runtime"
RELEASE_ROOT="$E2E_ROOT/release-server"
MGSCTL_HOME="$E2E_ROOT/home"
MGSCTL_CONFIG_HOME="$E2E_ROOT/config-home"
MGSCTL_DOCKER_CONFIG="${DOCKER_CONFIG:-${HOME:?HOME is required}/.docker}"
MGSCTL="$E2E_ROOT/mgsctl"
REGISTRY_CONTAINER="${E2E_PREFIX}-registry"
REGISTRY_PORT="$(deployment_e2e_port)"
RELEASE_PORT="$(deployment_e2e_port)"
IMAGE_REGISTRY="127.0.0.1:${REGISTRY_PORT}"
SOURCE_VERSION="v0.0.1"
TARGET_VERSION="v0.0.2"
API_PORT="$(deployment_e2e_port)"
GATEWAY_PORT="$(deployment_e2e_port)"
USER_WEB_PORT="$(deployment_e2e_port)"
ADMIN_WEB_PORT="$(deployment_e2e_port)"
DOCS_WEB_PORT="$(deployment_e2e_port)"
RELEASE_SERVER_PID=""
PROJECT=""
SUCCESS=false

assert_safe_runtime "$RUNTIME_DIR"
mkdir -p "$RUNTIME_DIR" "$OUTSIDE_DIR" "$RELEASE_ROOT" "$MGSCTL_HOME" "$MGSCTL_CONFIG_HOME"

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ -n "$PROJECT" && -f "$RUNTIME_DIR/compose.yml" && -f "$RUNTIME_DIR/config/runtime.env" ]]; then
    if [[ "$status" -ne 0 ]]; then
      deployment_e2e_redacted_logs "$RUNTIME_DIR" "$PROJECT" "$E2E_ROOT/upgrade.log"
    fi
    deployment_e2e_compose "$RUNTIME_DIR" "$PROJECT" down --volumes --remove-orphans >/dev/null 2>&1 || true
    deployment_e2e_remove_project "$PROJECT"
  fi
  if [[ -n "$RELEASE_SERVER_PID" ]]; then
    kill "$RELEASE_SERVER_PID" >/dev/null 2>&1 || true
    wait "$RELEASE_SERVER_PID" >/dev/null 2>&1 || true
  fi
  docker rm -f "$REGISTRY_CONTAINER" >/dev/null 2>&1 || true
  if [[ "$status" -eq 0 ]]; then
    find "$E2E_ROOT" -depth -delete
  else
    echo "mgsctl Docker upgrade E2E failed; redacted evidence retained at $E2E_ROOT" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

sha256_file() {
  local path=$1 directory name
  directory="$(dirname "$path")"
  name="$(basename "$path")"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$directory" && sha256sum "$name" > "$name.sha256")
  else
    (cd "$directory" && shasum -a 256 "$name" > "$name.sha256")
  fi
}

publish_target_version() {
  local source=$1 target=$2 component revision target_image actual_version
  revision="$(git -C "$ROOT_DIR" rev-parse HEAD)"
  for component in api worker user-web admin-web docs-web; do
    target_image="$IMAGE_REGISTRY/mikiko-gallery-studio-${component}:${target}"
    docker build \
      --label "org.opencontainers.image.version=$target" \
      --label "org.opencontainers.image.revision=$revision" \
      --label 'org.opencontainers.image.source=https://github.com/FatBallFish/mikiko-gallery-studio' \
      --tag "$target_image" - >/dev/null <<DOCKERFILE
FROM $IMAGE_REGISTRY/mikiko-gallery-studio-${component}:${source}
DOCKERFILE
    actual_version="$(docker image inspect "$target_image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')"
    [[ "$actual_version" == "$target" ]] || deployment_e2e_fail "target $component image label is $actual_version, want $target"
    docker push "$target_image" >/dev/null
  done
}

render_release() {
  local version=$1 asset_dir metadata_file component digest revision
  asset_dir="$RELEASE_ROOT/releases/download/$version"
  metadata_file="$E2E_ROOT/images-${version}.json"
  revision="$(git -C "$ROOT_DIR" rev-parse HEAD)"
  mkdir -p "$asset_dir" "$E2E_ROOT/image-rows-$version"
  cp "$MGSCTL" "$asset_dir/mgsctl-linux-amd64"
  sha256_file "$asset_dir/mgsctl-linux-amd64"
  for component in api worker user-web admin-web docs-web; do
    digest="$(deployment_e2e_registry_digest "$IMAGE_REGISTRY" "mikiko-gallery-studio-$component" "$version")"
    jq -n \
      --arg component "$component" \
      --arg repository "$IMAGE_REGISTRY/mikiko-gallery-studio-$component" \
      --arg tag "$version" \
      --arg digest "$digest" \
      --arg version "$version" \
      --arg revision "$revision" \
      '{component:$component,repository:$repository,tag:$tag,digest:$digest,version:$version,revision:$revision}' \
      > "$E2E_ROOT/image-rows-$version/$component.json"
  done
  jq -s '.' "$E2E_ROOT/image-rows-$version"/*.json > "$metadata_file"
  RELEASE_VERSION="$version" RELEASE_COMMIT="$revision" RELEASE_ASSET_DIR="$asset_dir" \
    RELEASE_IMAGE_METADATA="$metadata_file" RELEASE_MANIFEST_OUTPUT="$asset_dir/release-manifest.json" \
    "$ROOT_DIR/scripts/devops/render-release-manifest.sh"
}

runtime_json() {
  python3 - "$RUNTIME_DIR/config/runtime.env" <<'PY'
import json
import sys

values = {}
with open(sys.argv[1], encoding="utf-8") as source:
    for raw in source:
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key] = value.strip().strip('"')
keys = [
    "DATABASE_URL", "REDIS_URL", "REDIS_KEY_PREFIX", "STORAGE_DRIVER",
    "STORAGE_LOCAL_ROOT", "STORAGE_PUBLIC_BASE_URL", "STORAGE_SHARED_VOLUME",
    "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET",
    "STORAGE_S3_ACCESS_KEY_ID", "STORAGE_S3_SECRET_ACCESS_KEY",
    "STORAGE_S3_FORCE_PATH_STYLE", "STORAGE_S3_PREFIX",
]
print(json.dumps({key: values.get(key, "") for key in keys}, separators=(",", ":")))
PY
}

complete_setup() {
  local token cookie operation_id payload status
  token="$(env HOME="$MGSCTL_HOME" XDG_CONFIG_HOME="$MGSCTL_CONFIG_HOME" DOCKER_CONFIG="$MGSCTL_DOCKER_CONFIG" \
    "$MGSCTL" setup token show --runtime-dir "$RUNTIME_DIR" | sed -n 's/^Setup token: //p')"
  [[ -n "$token" ]] || deployment_e2e_fail "setup token was not returned"
  cookie="$E2E_ROOT/setup.cookie"
  status="$(curl --silent --show-error --output "$E2E_ROOT/setup-session.json" --write-out '%{http_code}' \
    --cookie-jar "$cookie" --header 'Content-Type: application/json' \
    --data "$(jq -n --arg token "$token" '{token:$token}')" \
    "http://127.0.0.1:${API_PORT}/api/setup/v1/session")"
  [[ "$status" == 200 ]] || deployment_e2e_fail "setup session returned HTTP $status"
  operation_id="$(python3 -c 'import uuid; print(uuid.uuid4())')"
  payload="$(jq -n --arg operation_id "$operation_id" --argjson runtime "$(runtime_json)" \
    --arg email 'upgrade-e2e@example.com' --arg password 'upgrade-e2e-password' \
    '{operation_id:$operation_id,runtime:$runtime,admin_email:$email,admin_password:$password}')"
  status="$(curl --silent --show-error --max-time 300 --output "$E2E_ROOT/setup-apply.json" --write-out '%{http_code}' \
    --cookie "$cookie" --header 'Content-Type: application/json' --data "$payload" \
    "http://127.0.0.1:${API_PORT}/api/setup/v1/apply")"
  [[ "$status" == 202 ]] || deployment_e2e_fail "setup apply returned HTTP $status"
  deployment_e2e_wait_status "http://127.0.0.1:${API_PORT}/readyz" 200 240
}

database_application_version() {
  local database
  database="$(deployment_e2e_env_value "$RUNTIME_DIR/config/runtime.env" POSTGRES_DATABASE)"
  deployment_e2e_compose "$RUNTIME_DIR" "$PROJECT" exec -T postgres \
    psql -X -qAt -U postgres -d "$database" -c 'SELECT app_version FROM installations LIMIT 1'
}

docker run --detach --name "$REGISTRY_CONTAINER" --publish "127.0.0.1:${REGISTRY_PORT}:5000" registry:2 >/dev/null
deployment_e2e_wait_status "http://127.0.0.1:${REGISTRY_PORT}/v2/" 200 60
deployment_e2e_build_images "$ROOT_DIR" "$IMAGE_REGISTRY" "$SOURCE_VERSION"
publish_target_version "$SOURCE_VERSION" "$TARGET_VERSION"
(cd "$ROOT_DIR" && go build -o "$MGSCTL" ./cmd/mgsctl)
render_release "$SOURCE_VERSION"
render_release "$TARGET_VERSION"
python3 -m http.server "$RELEASE_PORT" --bind 127.0.0.1 --directory "$RELEASE_ROOT" >"$E2E_ROOT/release-server.log" 2>&1 &
RELEASE_SERVER_PID=$!
deployment_e2e_wait_status "http://127.0.0.1:${RELEASE_PORT}/releases/download/${SOURCE_VERSION}/release-manifest.json" 200 30

env HOME="$MGSCTL_HOME" XDG_CONFIG_HOME="$MGSCTL_CONFIG_HOME" DOCKER_CONFIG="$MGSCTL_DOCKER_CONFIG" \
  MGSCTL_RELEASE_BASE_URL="http://127.0.0.1:${RELEASE_PORT}/releases" \
  "$MGSCTL" install --mode docker --profile full --topology single --yes \
    --runtime-dir "$RUNTIME_DIR" --image-tag "$SOURCE_VERSION" \
    --api-port "$API_PORT" --gateway-port "$GATEWAY_PORT" --user-web-port "$USER_WEB_PORT" \
    --admin-web-port "$ADMIN_WEB_PORT" --docs-web-port "$DOCS_WEB_PORT"

PROJECT="$(deployment_e2e_project_name "$RUNTIME_DIR/config/runtime.env")"
deployment_e2e_wait_status "http://127.0.0.1:${API_PORT}/healthz" 200 180
complete_setup
[[ "$(database_application_version)" == "$SOURCE_VERSION" ]] || deployment_e2e_fail "initial migration version is not $SOURCE_VERSION"

echo "==> mgsctl upgrade from outside-runtime"
upgrade_status=0
(
  cd "$OUTSIDE_DIR"
  env HOME="$MGSCTL_HOME" XDG_CONFIG_HOME="$MGSCTL_CONFIG_HOME" DOCKER_CONFIG="$MGSCTL_DOCKER_CONFIG" \
    MGSCTL_RELEASE_BASE_URL="http://127.0.0.1:${RELEASE_PORT}/releases" \
    "$MGSCTL" upgrade --image-tag "$TARGET_VERSION"
) > >(tee "$E2E_ROOT/upgrade-command.log") 2>&1 || upgrade_status=$?
if [[ "$upgrade_status" -ne 0 ]]; then
  deployment_e2e_redacted_logs "$RUNTIME_DIR" "$PROJECT" "$E2E_ROOT/upgrade.log"
  api_container="$(deployment_e2e_compose "$RUNTIME_DIR" "$PROJECT" ps -q api 2>/dev/null || true)"
  if [[ -n "$api_container" ]]; then
    docker inspect --format '{{json .State.Health}}' "$api_container" >"$E2E_ROOT/api-health.json" 2>&1 || true
  fi
  deployment_e2e_fail "mgsctl upgrade exited with status $upgrade_status"
fi

deployment_e2e_wait_status "http://127.0.0.1:${API_PORT}/readyz" 200 240
[[ "$(database_application_version)" == "$TARGET_VERSION" ]] || \
  deployment_e2e_fail "target mikiko-gallery-studio-db-migrate did not publish $TARGET_VERSION"
env HOME="$MGSCTL_HOME" XDG_CONFIG_HOME="$MGSCTL_CONFIG_HOME" DOCKER_CONFIG="$MGSCTL_DOCKER_CONFIG" \
  "$MGSCTL" doctor --runtime-dir "$RUNTIME_DIR" >/dev/null

SUCCESS=true
echo "OK: mgsctl Docker install, Setup, outside-runtime upgrade, migration, and readiness passed"

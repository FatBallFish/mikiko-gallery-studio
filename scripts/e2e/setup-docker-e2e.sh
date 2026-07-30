#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/e2e/deployment-e2e-lib.sh"

RUN_ID="$(date +%Y%m%d%H%M%S)-$$"
E2E_PROJECT_PREFIX="pic-gallery-setup-e2e-"
mkdir -p "$ROOT_DIR/tmp/e2e"
E2E_ROOT="$(mktemp -d "$ROOT_DIR/tmp/e2e/${E2E_PROJECT_PREFIX}${RUN_ID}.XXXXXX")"
E2E_EVIDENCE_DIR="${E2E_EVIDENCE_DIR:-$E2E_ROOT/evidence}"
REGISTRY_CONTAINER="${E2E_PROJECT_PREFIX}registry-${RUN_ID}"
REGISTRY_PORT="$(deployment_e2e_port)"
RELEASE_PORT="$(deployment_e2e_port)"
IMAGE_REGISTRY="127.0.0.1:${REGISTRY_PORT}"
IMAGE_TAG="v0.0.0-setup.${RUN_ID//-/.}"
MGSCTL="$E2E_ROOT/mgsctl"
RELEASE_ROOT="$E2E_ROOT/release-server"
RELEASE_SERVER_PID=""
SUCCESS=false
RUNTIMES=()
PROJECTS=()
MIDDLEWARE_CONTAINERS=()
MIDDLEWARE_NETWORKS=()
BACKGROUND_PIDS=()

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  for pid in "${BACKGROUND_PIDS[@]}"; do
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  done
  if [[ -n "$RELEASE_SERVER_PID" ]]; then
    kill "$RELEASE_SERVER_PID" >/dev/null 2>&1 || true
    wait "$RELEASE_SERVER_PID" >/dev/null 2>&1 || true
  fi
  for index in "${!RUNTIMES[@]}"; do
    runtime="${RUNTIMES[$index]}"
    project="${PROJECTS[$index]:-}"
    if [[ -z "$project" && -f "$runtime/config/runtime.env" ]]; then
      project="$(deployment_e2e_project_name "$runtime/config/runtime.env")"
    fi
    if [[ -n "$project" && -f "$runtime/compose.yml" && -f "$runtime/config/runtime.env" ]]; then
      if [[ "$status" -ne 0 ]]; then
        deployment_e2e_redacted_logs "$runtime" "$project" "$E2E_EVIDENCE_DIR/${project}.log"
      fi
      deployment_e2e_compose "$runtime" "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
      deployment_e2e_remove_project "$project"
    fi
  done
  for container in "${MIDDLEWARE_CONTAINERS[@]}"; do
    docker rm -fv "$container" >/dev/null 2>&1 || true
  done
  for network in "${MIDDLEWARE_NETWORKS[@]}"; do
    docker network rm "$network" >/dev/null 2>&1 || true
  done
  docker rm -f "$REGISTRY_CONTAINER" >/dev/null 2>&1 || true
  find "$E2E_ROOT" -type f \( -name 'runtime.env' -o -name '*.cookie' -o -name '*token*' \) -delete 2>/dev/null || true
  if [[ "$status" -eq 0 ]]; then
    find "$E2E_ROOT" -mindepth 1 -maxdepth 1 ! -name evidence -exec rm -rf {} +
  else
    echo "setup Docker E2E failed; redacted evidence retained at $E2E_EVIDENCE_DIR" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

mkdir -p "$E2E_EVIDENCE_DIR"
docker run --detach --name "$REGISTRY_CONTAINER" --publish "127.0.0.1:${REGISTRY_PORT}:5000" registry:2 >/dev/null
deployment_e2e_wait_status "http://127.0.0.1:${REGISTRY_PORT}/v2/" 200 60
deployment_e2e_build_images "$ROOT_DIR" "$IMAGE_REGISTRY" "$IMAGE_TAG"
(cd "$ROOT_DIR" && go build -o "$MGSCTL" ./cmd/mgsctl)
deployment_e2e_render_release "$ROOT_DIR" "$RELEASE_ROOT" "$MGSCTL" "$IMAGE_REGISTRY" "$IMAGE_TAG"
python3 -m http.server "$RELEASE_PORT" --bind 127.0.0.1 --directory "$RELEASE_ROOT" >"$E2E_ROOT/release-server.log" 2>&1 &
RELEASE_SERVER_PID=$!
deployment_e2e_wait_status "http://127.0.0.1:${RELEASE_PORT}/releases/download/${IMAGE_TAG}/release-manifest.json" 200 30
export MGSCTL_RELEASE_BASE_URL="http://127.0.0.1:${RELEASE_PORT}/releases"

start_core_middleware() {
  local suffix=$1
  CORE_NETWORK="${E2E_PROJECT_PREFIX}middleware-${suffix}"
  CORE_POSTGRES="${E2E_PROJECT_PREFIX}postgres-${suffix}"
  CORE_REDIS="${E2E_PROJECT_PREFIX}redis-${suffix}"
  CORE_MINIO="${E2E_PROJECT_PREFIX}minio-${suffix}"
  MIDDLEWARE_NETWORKS+=("$CORE_NETWORK")
  MIDDLEWARE_CONTAINERS+=("$CORE_POSTGRES" "$CORE_REDIS" "$CORE_MINIO")
  docker network create "$CORE_NETWORK" >/dev/null
  docker run -d --name "$CORE_POSTGRES" --network "$CORE_NETWORK" \
    -e POSTGRES_DB=app -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=e2e-postgres-bootstrap-password \
    -e APP_POSTGRES_USER=app -e APP_POSTGRES_PASSWORD=e2e-postgres-password \
    -v "$ROOT_DIR/deployments/docker-compose/postgres-init.sh:/opt/deploy/postgres-init.sh:ro" \
    --entrypoint /bin/sh postgres:16-alpine -ec \
    'cp /opt/deploy/postgres-init.sh /docker-entrypoint-initdb.d/10-app-role.sh; chown postgres:postgres /docker-entrypoint-initdb.d/10-app-role.sh; chmod 0700 /docker-entrypoint-initdb.d/10-app-role.sh; exec docker-entrypoint.sh postgres' >/dev/null
  docker run -d --name "$CORE_REDIS" --network "$CORE_NETWORK" redis:7-alpine \
    redis-server --appendonly yes --requirepass e2e-redis-password >/dev/null
  docker run -d --name "$CORE_MINIO" --network "$CORE_NETWORK" \
    -e MINIO_ROOT_USER=e2e-minio -e MINIO_ROOT_PASSWORD=e2e-minio-password minio/minio:RELEASE.2025-04-22T22-12-26Z \
    server /data --console-address :9001 >/dev/null
  for _ in {1..90}; do
    docker exec "$CORE_POSTGRES" pg_isready -U app -d app >/dev/null 2>&1 && \
      docker exec "$CORE_REDIS" redis-cli -a e2e-redis-password ping 2>/dev/null | rg -q PONG && \
      docker exec "$CORE_MINIO" curl -fsS http://127.0.0.1:9000/minio/health/live >/dev/null 2>&1 && break
    sleep 1
  done
  docker run --rm --network "$CORE_NETWORK" --entrypoint /bin/sh minio/mc:RELEASE.2025-04-16T18-13-26Z -ec \
    "mc alias set e2e http://${CORE_MINIO}:9000 e2e-minio e2e-minio-password >/dev/null && mc mb --ignore-existing e2e/app-assets >/dev/null"
}

configure_local_e2e_runtime() {
  local runtime=$1 project=$2 api_port=$3 env_file="$runtime/config/runtime.env"
  deployment_e2e_set_env_value "$env_file" PIC_GALLERY_ENV local
  deployment_e2e_set_env_value "$env_file" AUTH_FIXED_EMAIL_CODE 123456
  deployment_e2e_set_env_value "$env_file" AUTH_DEV_EMAIL_CODES true
  deployment_e2e_compose "$runtime" "$project" restart api worker gateway >/dev/null
  deployment_e2e_wait_status "http://127.0.0.1:${api_port}/healthz" 200 180
}

run_profile() {
  local profile=$1 ordinal=$2
  local runtime="$E2E_ROOT/$profile" api_port gateway_port user_port admin_port docs_port
  local attempt project env_file runtime_index
  local E2E_INSTALL_ATTEMPTS=3
  mkdir -p "$runtime"
  checkpoint="$E2E_EVIDENCE_DIR/${profile}-stage.txt"
  RUNTIMES+=("$runtime")
  PROJECTS+=("")
  runtime_index=$((${#RUNTIMES[@]} - 1))

  for ((attempt = 1; attempt <= E2E_INSTALL_ATTEMPTS; attempt++)); do
    api_port="$(deployment_e2e_port)"
    gateway_port="$(deployment_e2e_port)"
    user_port="$(deployment_e2e_port)"
    admin_port="$(deployment_e2e_port)"
    docs_port="$(deployment_e2e_port)"
    if "$MGSCTL" install --mode docker --profile "$profile" --topology single --yes \
      --runtime-dir "$runtime" --image-registry "$IMAGE_REGISTRY" --image-tag "$IMAGE_TAG" \
      --api-port "$api_port" --gateway-port "$gateway_port" --user-web-port "$user_port" --admin-web-port "$admin_port" --docs-web-port "$docs_port"; then
      break
    fi
    if [[ -f "$runtime/config/runtime.env" ]]; then
      project="$(deployment_e2e_project_name "$runtime/config/runtime.env")"
      PROJECTS[$runtime_index]="$project"
      deployment_e2e_compose "$runtime" "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
      deployment_e2e_remove_project "$project"
    fi
    if (( attempt == E2E_INSTALL_ATTEMPTS )); then
      deployment_e2e_fail "$profile deployment did not start after $E2E_INSTALL_ATTEMPTS attempts"
    fi
    rm -rf "$runtime"
    mkdir -p "$runtime"
    PROJECTS[$runtime_index]=""
    echo "$profile deployment start failed; retrying with fresh ports ($((attempt + 1))/$E2E_INSTALL_ATTEMPTS)" >&2
  done

  env_file="$runtime/config/runtime.env"
  project="$(deployment_e2e_project_name "$env_file")"
  PROJECTS[$runtime_index]="$project"
  if [[ "$profile" == core ]]; then
    configure_local_e2e_runtime "$runtime" "$project" "$api_port"
  else
    deployment_e2e_wait_status "http://127.0.0.1:${api_port}/healthz" 200 180
  fi

  if [[ "$profile" == core ]]; then
    start_core_middleware "${RUN_ID}-${ordinal}"
    api_container="$(deployment_e2e_compose "$runtime" "$project" ps -q api)"
    worker_container="$(deployment_e2e_compose "$runtime" "$project" ps -q worker)"
    docker network connect "$CORE_NETWORK" "$api_container"
    docker network connect "$CORE_NETWORK" "$worker_container"
  fi

  setup_status="$(curl -fsS "http://127.0.0.1:${api_port}/api/system/v1/bootstrap-status")"
  [[ "$(printf '%s' "$setup_status" | deployment_e2e_json_data_field phase)" == setup_required ]]
  [[ "$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${api_port}/api/agent/auth/v1/login/password")" == 404 ]]

  old_token="$($MGSCTL setup token show --runtime-dir "$runtime" | sed -n 's/^Setup token: //p')"
  [[ -n "$old_token" ]]
  deployment_e2e_wait_status "http://127.0.0.1:${gateway_port}/api/system/v1/bootstrap-status" 200 60
  gateway_status="$(curl -fsS "http://127.0.0.1:${gateway_port}/api/system/v1/bootstrap-status")"
  [[ "$(printf '%s' "$gateway_status" | deployment_e2e_json_data_field phase)" == setup_required ]] || \
    deployment_e2e_fail "$profile Gateway did not proxy setup bootstrap status"
  deployment_e2e_assert_frontend "http://127.0.0.1:${user_port}/" app
  deployment_e2e_assert_frontend "http://127.0.0.1:${admin_port}/" app
  deployment_e2e_assert_frontend "http://127.0.0.1:${docs_port}/" docs
  deployment_e2e_assert_frontend "http://127.0.0.1:${gateway_port}/" app
  deployment_e2e_assert_frontend "http://127.0.0.1:${gateway_port}/admin/" app
  deployment_e2e_assert_frontend "http://127.0.0.1:${gateway_port}/developer-docs/" docs
  BASE_URL="http://127.0.0.1:${api_port}" USER_WEB_URL="http://127.0.0.1:${gateway_port}/#/home" \
    ADMIN_WEB_URL="http://127.0.0.1:${gateway_port}/admin/#/overview" SETUP_TOKEN="$old_token" DEPLOYMENT_PROFILE="$profile" \
    DIRECT_USER_WEB_URL="http://127.0.0.1:${user_port}/#/home" DIRECT_ADMIN_WEB_URL="http://127.0.0.1:${admin_port}/#/overview" \
    DIRECT_DOCS_WEB_URL="http://127.0.0.1:${docs_port}/" GATEWAY_DOCS_WEB_URL="http://127.0.0.1:${gateway_port}/developer-docs/" \
    REDIRECT_SETUP_URL="http://127.0.0.1:${gateway_port}/setup" \
    E2E_EVIDENCE_DIR="$E2E_EVIDENCE_DIR/$profile" python3 "$ROOT_DIR/scripts/e2e/setup-browser.py"

  printf 'before-token-reset\n' >"$checkpoint"
  if ! reset_output="$($MGSCTL setup token reset --runtime-dir "$runtime")"; then
    deployment_e2e_fail "$profile setup token reset command failed"
  fi
  printf 'token-reset-command-complete\n' >"$checkpoint"
  new_token="$(printf '%s\n' "$reset_output" | sed -n 's/^Setup token reset. New token: //p')"
  [[ -n "$new_token" ]] || deployment_e2e_fail "$profile setup token reset output was not parseable"
  [[ "$new_token" != "$old_token" ]] || deployment_e2e_fail "$profile setup token reset reused the old token"
  printf 'token-reset-output-validated\n' >"$checkpoint"
  deployment_e2e_wait_status "http://127.0.0.1:${api_port}/healthz" 200 180 || \
    deployment_e2e_fail "$profile API did not recover after setup token reset"
  printf 'token-reset-health-ready\n' >"$checkpoint"
  old_status="$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    --data "{\"token\":\"$old_token\"}" "http://127.0.0.1:${api_port}/api/setup/v1/session")"
  [[ "$old_status" == 401 ]] || deployment_e2e_fail "$profile old setup token returned HTTP $old_status after reset"
  printf 'old-token-rejected\n' >"$checkpoint"

  if [[ "$profile" == full ]]; then
    setup_database_url=""
    setup_redis_url=""
    setup_redis_key_prefix=""
    setup_storage_driver=""
    setup_storage_endpoint=""
    setup_storage_region=""
    setup_storage_bucket=""
    setup_storage_access_key=""
    setup_storage_secret_key=""
    setup_storage_force_path_style=""
    setup_storage_prefix=""
  else
    setup_database_url="postgres://app:e2e-postgres-password@${CORE_POSTGRES}:5432/app?sslmode=disable"
    setup_redis_url="redis://:e2e-redis-password@${CORE_REDIS}:6379/0"
    setup_redis_key_prefix="app"
    setup_storage_driver="s3"
    setup_storage_endpoint="http://${CORE_MINIO}:9000"
    setup_storage_region="us-east-1"
    setup_storage_bucket="app-assets"
    setup_storage_access_key="e2e-minio"
    setup_storage_secret_key="e2e-minio-password"
    setup_storage_force_path_style="true"
    setup_storage_prefix="core"
  fi

  if [[ "$profile" == core ]]; then
    local migration_lock_key lock_pid interruption_marker interrupted_browser_pid recovery_operation_id
    migration_lock_key=$((0x5047434D49475231))
    interruption_marker="$E2E_ROOT/core-interruption-ready"
    docker exec "$CORE_POSTGRES" psql -U app -d app -v ON_ERROR_STOP=1 -qAt \
      -c "SELECT pg_advisory_lock(${migration_lock_key}); SELECT pg_sleep(300);" \
      >"$E2E_EVIDENCE_DIR/core-migration-lock.log" 2>&1 &
    lock_pid=$!
    BACKGROUND_PIDS+=("$lock_pid")
    for _ in {1..300}; do
      [[ "$(docker exec "$CORE_POSTGRES" psql -U app -d app -Atq -c "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted")" -ge 1 ]] && break
      sleep 0.1
    done
    [[ "$(docker exec "$CORE_POSTGRES" psql -U app -d app -Atq -c "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted")" -ge 1 ]] || \
      deployment_e2e_fail "core migration advisory lock was not acquired"

    BASE_URL="http://127.0.0.1:${api_port}" USER_WEB_URL="http://127.0.0.1:${gateway_port}/#/home" \
      ADMIN_WEB_URL="http://127.0.0.1:${gateway_port}/admin/#/overview" SETUP_TOKEN="$new_token" DEPLOYMENT_PROFILE="$profile" \
      REDIRECT_SETUP_URL="http://127.0.0.1:${gateway_port}/setup" E2E_APPLY_SETUP=true E2E_EXPECT_INTERRUPTION=true \
      E2E_INTERRUPTION_READY_FILE="$interruption_marker" \
      SETUP_DATABASE_URL="$setup_database_url" SETUP_REDIS_URL="$setup_redis_url" SETUP_REDIS_KEY_PREFIX="$setup_redis_key_prefix" \
      SETUP_STORAGE_DRIVER="$setup_storage_driver" SETUP_STORAGE_S3_ENDPOINT="$setup_storage_endpoint" \
      SETUP_STORAGE_S3_REGION="$setup_storage_region" SETUP_STORAGE_S3_BUCKET="$setup_storage_bucket" \
      SETUP_STORAGE_S3_ACCESS_KEY_ID="$setup_storage_access_key" SETUP_STORAGE_S3_SECRET_ACCESS_KEY="$setup_storage_secret_key" \
      SETUP_STORAGE_S3_FORCE_PATH_STYLE="$setup_storage_force_path_style" SETUP_STORAGE_S3_PREFIX="$setup_storage_prefix" \
      SETUP_CORS_ALLOWED_ORIGINS="http://127.0.0.1:${gateway_port},http://localhost:${gateway_port},http://127.0.0.1:${user_port},http://127.0.0.1:${admin_port}" SETUP_ADMIN_EMAIL="admin@example.com" \
      SETUP_ADMIN_PASSWORD="admin123456" E2E_EVIDENCE_DIR="$E2E_EVIDENCE_DIR/$profile-interrupted" \
      python3 "$ROOT_DIR/scripts/e2e/setup-browser.py" >"$E2E_EVIDENCE_DIR/core-interrupted-browser.log" 2>&1 &
    interrupted_browser_pid=$!
    BACKGROUND_PIDS+=("$interrupted_browser_pid")
    for _ in {1..600}; do
      [[ -f "$interruption_marker" ]] && break
      kill -0 "$interrupted_browser_pid" >/dev/null 2>&1 || break
      sleep 0.1
    done
    [[ -f "$interruption_marker" ]] || deployment_e2e_fail "core interrupted browser did not submit setup"
    for _ in {1..600}; do
      [[ "$(docker exec "$CORE_POSTGRES" psql -U app -d app -Atq -c "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted")" -ge 1 ]] && break
      kill -0 "$interrupted_browser_pid" >/dev/null 2>&1 || break
      sleep 0.1
    done
    [[ "$(docker exec "$CORE_POSTGRES" psql -U app -d app -Atq -c "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted")" -ge 1 ]] || \
      deployment_e2e_fail "core setup did not reach the blocked migration boundary"
    recovery_operation_id="$(python3 - "$runtime/config/install-state.json" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as source:
    state = json.load(source)
print((state.get("attempt") or {}).get("operation_id", ""))
PY
)"
    [[ -n "$recovery_operation_id" ]] || deployment_e2e_fail "core setup did not persist its interrupted operation ID"

    api_container="$(deployment_e2e_compose "$runtime" "$project" ps -q api)"
    docker kill "$api_container" >/dev/null
    docker exec "$CORE_POSTGRES" psql -U app -d app -v ON_ERROR_STOP=1 -qAt \
      -c "SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype = 'advisory' AND granted AND pid <> pg_backend_pid();" >/dev/null
    deployment_e2e_compose "$runtime" "$project" up -d --no-deps api gateway >/dev/null
	  api_container="$(deployment_e2e_compose "$runtime" "$project" ps -q api)"
	  docker network connect "$CORE_NETWORK" "$api_container"
    deployment_e2e_wait_status "http://127.0.0.1:${api_port}/healthz" 200 180
	  kill "$interrupted_browser_pid" >/dev/null 2>&1 || true
	  wait "$interrupted_browser_pid" >/dev/null 2>&1 || true
    printf 'operation_id=%s\napi_restarted=true\nnew_browser_required=true\n' "$recovery_operation_id" \
      >"$E2E_EVIDENCE_DIR/recovery-operation-id.txt"
  fi

  printf 'before-browser-apply\n' >"$checkpoint"
  BASE_URL="http://127.0.0.1:${api_port}" USER_WEB_URL="http://127.0.0.1:${gateway_port}/#/home" \
    ADMIN_WEB_URL="http://127.0.0.1:${gateway_port}/admin/#/overview" SETUP_TOKEN="$new_token" DEPLOYMENT_PROFILE="$profile" \
    DIRECT_USER_WEB_URL="http://127.0.0.1:${user_port}/#/home" DIRECT_ADMIN_WEB_URL="http://127.0.0.1:${admin_port}/#/overview" \
    DIRECT_DOCS_WEB_URL="http://127.0.0.1:${docs_port}/" GATEWAY_DOCS_WEB_URL="http://127.0.0.1:${gateway_port}/developer-docs/" \
    REDIRECT_SETUP_URL="http://127.0.0.1:${gateway_port}/setup" E2E_APPLY_SETUP=true \
    SETUP_DATABASE_URL="$setup_database_url" SETUP_REDIS_URL="$setup_redis_url" SETUP_REDIS_KEY_PREFIX="$setup_redis_key_prefix" \
    SETUP_STORAGE_DRIVER="$setup_storage_driver" SETUP_STORAGE_S3_ENDPOINT="$setup_storage_endpoint" \
    SETUP_STORAGE_S3_REGION="$setup_storage_region" SETUP_STORAGE_S3_BUCKET="$setup_storage_bucket" \
    SETUP_STORAGE_S3_ACCESS_KEY_ID="$setup_storage_access_key" SETUP_STORAGE_S3_SECRET_ACCESS_KEY="$setup_storage_secret_key" \
    SETUP_STORAGE_S3_FORCE_PATH_STYLE="$setup_storage_force_path_style" SETUP_STORAGE_S3_PREFIX="$setup_storage_prefix" \
    SETUP_CORS_ALLOWED_ORIGINS="http://127.0.0.1:${gateway_port},http://localhost:${gateway_port},http://127.0.0.1:${user_port},http://127.0.0.1:${admin_port}" SETUP_ADMIN_EMAIL="admin@example.com" \
    SETUP_ADMIN_PASSWORD="admin123456" E2E_EVIDENCE_DIR="$E2E_EVIDENCE_DIR/$profile" \
    python3 "$ROOT_DIR/scripts/e2e/setup-browser.py"
  printf 'browser-apply-returned\n' >"$checkpoint"

  deployment_e2e_wait_status "http://127.0.0.1:${api_port}/readyz" 200 240
  [[ "$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${api_port}/setup")" == 404 ]]
  [[ "$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' --data '{}' "http://127.0.0.1:${api_port}/api/setup/v1/apply")" == 404 ]]
  login="$(curl -fsS -H 'Content-Type: application/json' --data '{"email":"admin@example.com","password":"admin123456"}' \
    "http://127.0.0.1:${api_port}/api/ops/admin/v1/auth/login")"
  [[ -n "$(printf '%s' "$login" | deployment_e2e_json_data_field access_token)" ]]
  "$MGSCTL" doctor --runtime-dir "$runtime" >"$E2E_EVIDENCE_DIR/${profile}-doctor.txt"

  if [[ "$profile" == full && "${E2E_RUN_BUSINESS:-true}" == true ]]; then
    configure_local_e2e_runtime "$runtime" "$project" "$api_port"
    BASE_URL="http://127.0.0.1:${gateway_port}" USER_WEB_URL="http://127.0.0.1:${gateway_port}" \
      ADMIN_WEB_URL="http://127.0.0.1:${gateway_port}/admin" NGINX_URL="http://127.0.0.1:${gateway_port}" \
      MINIO_URL="http://127.0.0.1:${gateway_port}" MAILPIT_URL="http://127.0.0.1:${gateway_port}" \
      E2E_SKIP_MIDDLEWARE_HEALTH=true \
      node "$ROOT_DIR/scripts/e2e/docker-e2e.mjs"
  fi

  printf 'DEPLOYMENT_PROFILE=%s\nproject=%s\napi=http://127.0.0.1:%s\ngateway=http://127.0.0.1:%s\n' \
    "$profile" "$project" "$api_port" "$gateway_port" >"$E2E_EVIDENCE_DIR/${profile}-summary.txt"
}

case "${DEPLOYMENT_E2E_PROFILES:-full,core}" in
  full) run_profile full 1 ;;
  core) run_profile core 1 ;;
  full,core|core,full) run_profile full 1; run_profile core 2 ;;
  *) deployment_e2e_fail "DEPLOYMENT_E2E_PROFILES must be full, core, or full,core" ;;
esac
SUCCESS=true
echo "setup Docker E2E passed; evidence: $E2E_EVIDENCE_DIR"

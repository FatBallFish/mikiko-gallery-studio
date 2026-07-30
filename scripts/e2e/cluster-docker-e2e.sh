#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/e2e/deployment-e2e-lib.sh"

RUN_ID="$(date +%Y%m%d%H%M%S)-$$"
E2E_PROJECT_PREFIX="pic-gallery-cluster-e2e-"
mkdir -p "$ROOT_DIR/tmp/e2e"
E2E_ROOT="$(mktemp -d "$ROOT_DIR/tmp/e2e/${E2E_PROJECT_PREFIX}${RUN_ID}.XXXXXX")"
E2E_EVIDENCE_DIR="${E2E_EVIDENCE_DIR:-$E2E_ROOT/evidence}"
REGISTRY_CONTAINER="${E2E_PROJECT_PREFIX}registry-${RUN_ID}"
POSTGRES_CONTAINER="${E2E_PROJECT_PREFIX}postgres-${RUN_ID}"
REDIS_CONTAINER="${E2E_PROJECT_PREFIX}redis-${RUN_ID}"
MINIO_CONTAINER="${E2E_PROJECT_PREFIX}minio-${RUN_ID}"
REGISTRY_PORT="$(deployment_e2e_port)"
POSTGRES_PORT="$(deployment_e2e_port)"
REDIS_PORT="$(deployment_e2e_port)"
MINIO_PORT="$(deployment_e2e_port)"
IMAGE_REGISTRY="127.0.0.1:${REGISTRY_PORT}"
IMAGE_TAG="cluster-${RUN_ID}"
MGSCTL="$E2E_ROOT/mgsctl"
ENROLLMENT_CAPTURE="$E2E_ROOT/enrollment-capture.jsonl"
RUNTIMES=()
PROJECTS=()
PROXY_PIDS=()
JOIN_TOKENS=()

cleanup() {
  local status=$? index runtime project
  trap - EXIT INT TERM
  for pid in "${PROXY_PIDS[@]}"; do
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  done
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
  docker rm -fv "$POSTGRES_CONTAINER" "$REDIS_CONTAINER" "$MINIO_CONTAINER" "$REGISTRY_CONTAINER" >/dev/null 2>&1 || true
  rm -f "$ENROLLMENT_CAPTURE"
  find "$E2E_ROOT" -type f \( -name 'runtime.env' -o -name '*.cookie' -o -name '*token*' \) -delete 2>/dev/null || true
  if [[ "$status" -eq 0 ]]; then
    find "$E2E_ROOT" -mindepth 1 -maxdepth 1 ! -name evidence -exec rm -rf {} +
  else
    echo "cluster Docker E2E failed; redacted evidence retained at $E2E_EVIDENCE_DIR" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

register_pending_runtime() {
  local runtime=$1
  RUNTIMES+=("$runtime")
  PROJECTS+=("")
  REGISTERED_INDEX=$((${#RUNTIMES[@]} - 1))
}

complete_runtime_registration() {
  local index=$1 runtime=$2
  PROJECTS[$index]="$(deployment_e2e_project_name "$runtime/config/runtime.env")"
}

wait_for_file_lines() {
  local path=$1 minimum=$2 watched_pid=$3 timeout=${4:-240}
  local deadline=$((SECONDS + timeout)) lines status
  while (( SECONDS < deadline )); do
    lines=0
    [[ -f "$path" ]] && lines="$(wc -l <"$path" | tr -d ' ')"
    (( lines >= minimum )) && return 0
    if ! kill -0 "$watched_pid" 2>/dev/null; then
      if wait "$watched_pid"; then
        deployment_e2e_fail "business runner exited before writing ${minimum} lines to $path"
      else
        status=$?
        deployment_e2e_fail "business runner failed with status $status before writing ${minimum} lines to $path"
      fi
    fi
    sleep 1
  done
  deployment_e2e_fail "timed out waiting for ${minimum} lines in $path"
}

assert_env_empty() {
  local path=$1 key=$2
  [[ -z "$(deployment_e2e_env_value "$path" "$key")" ]] || deployment_e2e_fail "$key leaked into $path"
}

issue_token() {
  local role=$1 output token
  output="$($MGSCTL cluster token create --role "$role" --ttl 10m --runtime-dir "$TOKEN_RUNTIME")"
  token="$(printf '%s\n' "$output" | awk -F': ' '/^Cluster join token / {print $NF}')"
  [[ -n "$token" ]] || deployment_e2e_fail "mgsctl did not return a $role join token"
  printf '%s' "$token"
}

join_node() {
  local token=$1 runtime=$2 api_port=${3:-8080} runtime_index
  mkdir -p "$runtime"
  register_pending_runtime "$runtime"
  runtime_index=$REGISTERED_INDEX
  "$MGSCTL" cluster join --server "http://127.0.0.1:${ENROLLMENT_PORT}" --token "$token" \
    --runtime-dir "$runtime" --mode docker --application-version "$IMAGE_TAG" \
    --image-registry "$IMAGE_REGISTRY" --image-tag "$IMAGE_TAG" --api-port "$api_port"
  complete_runtime_registration "$runtime_index" "$runtime"
}

mkdir -p "$E2E_EVIDENCE_DIR"
docker run --detach --name "$REGISTRY_CONTAINER" --publish "127.0.0.1:${REGISTRY_PORT}:5000" registry:2 >/dev/null
deployment_e2e_wait_status "http://127.0.0.1:${REGISTRY_PORT}/v2/" 200 60
deployment_e2e_build_images "$ROOT_DIR" "$IMAGE_REGISTRY" "$IMAGE_TAG"
(cd "$ROOT_DIR" && go build -o "$MGSCTL" ./cmd/mgsctl)

docker run --detach --name "$POSTGRES_CONTAINER" --publish "127.0.0.1:${POSTGRES_PORT}:5432" \
  -e POSTGRES_DB=app -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=e2e-postgres-bootstrap-password \
  -e APP_POSTGRES_USER=app -e APP_POSTGRES_PASSWORD=e2e-postgres-password \
  -v "$ROOT_DIR/deployments/docker-compose/postgres-init.sh:/opt/deploy/postgres-init.sh:ro" \
  --entrypoint /bin/sh postgres:16-alpine -ec \
  'cp /opt/deploy/postgres-init.sh /docker-entrypoint-initdb.d/10-app-role.sh; chown postgres:postgres /docker-entrypoint-initdb.d/10-app-role.sh; chmod 0700 /docker-entrypoint-initdb.d/10-app-role.sh; exec docker-entrypoint.sh postgres' >/dev/null
docker run --detach --name "$REDIS_CONTAINER" --publish "127.0.0.1:${REDIS_PORT}:6379" redis:7-alpine \
  redis-server --appendonly yes --requirepass e2e-redis-password >/dev/null
docker run --detach --name "$MINIO_CONTAINER" --publish "127.0.0.1:${MINIO_PORT}:9000" \
  -e MINIO_ROOT_USER=e2e-minio -e MINIO_ROOT_PASSWORD=e2e-minio-password \
  minio/minio:RELEASE.2025-04-22T22-12-26Z server /data --console-address :9001 >/dev/null
for _ in {1..90}; do
  if docker exec "$POSTGRES_CONTAINER" pg_isready -U app -d app >/dev/null 2>&1 && \
    docker exec "$REDIS_CONTAINER" redis-cli --no-auth-warning -a e2e-redis-password ping 2>/dev/null | rg -q PONG && \
    docker exec "$MINIO_CONTAINER" curl -fsS http://127.0.0.1:9000/minio/health/live >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker run --rm --network "container:${MINIO_CONTAINER}" --entrypoint /bin/sh \
  minio/mc:RELEASE.2025-04-16T18-13-26Z -ec \
  'mc alias set e2e http://127.0.0.1:9000 e2e-minio e2e-minio-password >/dev/null && mc mb --ignore-existing e2e/app-assets >/dev/null'

CONTROL_RUNTIME="$E2E_ROOT/runtime-control"
CONTROL_API_PORT="$(deployment_e2e_port)"
CONTROL_GATEWAY_PORT="$(deployment_e2e_port)"
CONTROL_USER_PORT="$(deployment_e2e_port)"
CONTROL_ADMIN_PORT="$(deployment_e2e_port)"
CONTROL_DOCS_PORT="$(deployment_e2e_port)"
mkdir -p "$CONTROL_RUNTIME"
register_pending_runtime "$CONTROL_RUNTIME"
CONTROL_RUNTIME_INDEX=$REGISTERED_INDEX
"$MGSCTL" install --mode docker --profile core --topology cluster --role control --storage-driver s3 --yes \
  --runtime-dir "$CONTROL_RUNTIME" --application-version "$IMAGE_TAG" --image-registry "$IMAGE_REGISTRY" --image-tag "$IMAGE_TAG" \
  --api-port "$CONTROL_API_PORT" --gateway-port "$CONTROL_GATEWAY_PORT" --user-web-port "$CONTROL_USER_PORT" \
  --admin-web-port "$CONTROL_ADMIN_PORT" --docs-web-port "$CONTROL_DOCS_PORT"
CONTROL_ENV="$CONTROL_RUNTIME/config/runtime.env"
complete_runtime_registration "$CONTROL_RUNTIME_INDEX" "$CONTROL_RUNTIME"
CONTROL_PROJECT="${PROJECTS[$CONTROL_RUNTIME_INDEX]}"
printf '\n# E2E-only runtime settings / 仅用于 E2E 的运行配置\nPIC_GALLERY_ENV=local\nAUTH_FIXED_EMAIL_CODE=123456\nAUTH_DEV_EMAIL_CODES=true\n' >>"$CONTROL_ENV"
deployment_e2e_compose "$CONTROL_RUNTIME" "$CONTROL_PROJECT" restart api worker gateway >/dev/null
deployment_e2e_wait_status "http://127.0.0.1:${CONTROL_API_PORT}/healthz" 200 180

SETUP_TOKEN="$($MGSCTL setup token show --runtime-dir "$CONTROL_RUNTIME" | sed -n 's/^Setup token: //p')"
COOKIE="$E2E_ROOT/control.cookie"
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' --data "{\"token\":\"$SETUP_TOKEN\"}" \
  "http://127.0.0.1:${CONTROL_API_PORT}/api/setup/v1/session" -o /dev/null
DATABASE_URL="postgres://app:e2e-postgres-password@host.docker.internal:${POSTGRES_PORT}/app?sslmode=disable"
REDIS_URL="redis://:e2e-redis-password@host.docker.internal:${REDIS_PORT}/0"
S3_ENDPOINT="http://host.docker.internal:${MINIO_PORT}"
for probe in database redis storage; do
  case "$probe" in
    database) payload="{\"database_url\":\"$DATABASE_URL\"}" ;;
    redis) payload="{\"redis_url\":\"$REDIS_URL\",\"key_prefix\":\"app-cluster-${RUN_ID}\"}" ;;
    storage) payload="{\"driver\":\"s3\",\"endpoint\":\"$S3_ENDPOINT\",\"region\":\"us-east-1\",\"bucket\":\"app-assets\",\"access_key_id\":\"e2e-minio\",\"secret_access_key\":\"e2e-minio-password\",\"force_path_style\":true,\"prefix\":\"cluster-${RUN_ID}\"}" ;;
  esac
  response="$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' --data "$payload" \
    "http://127.0.0.1:${CONTROL_API_PORT}/api/setup/v1/probes/${probe}")"
  [[ "$(printf '%s' "$response" | deployment_e2e_json_data_field success)" == True ]]
done
RUNTIME_PAYLOAD="$(python3 - "$DATABASE_URL" "$REDIS_URL" "$S3_ENDPOINT" "$RUN_ID" "$CONTROL_GATEWAY_PORT" <<'PY'
import json
import sys
database_url, redis_url, endpoint, run_id, gateway_port = sys.argv[1:]
print(json.dumps({
    "DATABASE_URL": database_url,
    "REDIS_URL": redis_url,
    "REDIS_KEY_PREFIX": f"app-cluster-{run_id}",
    "STORAGE_DRIVER": "s3",
    "STORAGE_S3_ENDPOINT": endpoint,
    "STORAGE_S3_REGION": "us-east-1",
    "STORAGE_S3_BUCKET": "app-assets",
    "STORAGE_S3_ACCESS_KEY_ID": "e2e-minio",
    "STORAGE_S3_SECRET_ACCESS_KEY": "e2e-minio-password",
    "STORAGE_S3_FORCE_PATH_STYLE": "true",
    "STORAGE_S3_PREFIX": f"cluster-{run_id}",
    "CORS_ALLOWED_ORIGINS": f"http://127.0.0.1:{gateway_port},http://localhost:{gateway_port}",
}))
PY
)"
OPERATION_ID="$(python3 -c 'import uuid; print(uuid.uuid4())')"
APPLY_BODY="$(python3 - "$OPERATION_ID" "$RUNTIME_PAYLOAD" <<'PY'
import json
import sys
print(json.dumps({
    "operation_id": sys.argv[1],
    "runtime": json.loads(sys.argv[2]),
    "admin_email": "admin@example.com",
    "admin_password": "admin123456",
}))
PY
)"
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' --data "$APPLY_BODY" \
  "http://127.0.0.1:${CONTROL_API_PORT}/api/setup/v1/apply" >"$E2E_EVIDENCE_DIR/control-apply.json"
deployment_e2e_wait_status "http://127.0.0.1:${CONTROL_API_PORT}/readyz" 200 240
[[ "$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${CONTROL_API_PORT}/setup")" == 404 ]]

TOKEN_RUNTIME="$E2E_ROOT/token-runtime"
mkdir -p "$TOKEN_RUNTIME/config"
cp "$CONTROL_ENV" "$TOKEN_RUNTIME/config/runtime.env"
deployment_e2e_set_env_value "$TOKEN_RUNTIME/config/runtime.env" DATABASE_URL \
  "postgres://app:e2e-postgres-password@127.0.0.1:${POSTGRES_PORT}/app?sslmode=disable"

ENROLLMENT_PORT="$(deployment_e2e_port)"
python3 "$ROOT_DIR/scripts/e2e/cluster-http-proxy.py" --listen "127.0.0.1:${ENROLLMENT_PORT}" \
  --upstream "http://127.0.0.1:${CONTROL_API_PORT}" --capture-file "$ENROLLMENT_CAPTURE" \
  --upstream-log "$E2E_EVIDENCE_DIR/enrollment-upstreams.jsonl" &
PROXY_PIDS+=("$!")
deployment_e2e_wait_status "http://127.0.0.1:${ENROLLMENT_PORT}/healthz" 200 30

EXPIRED_TOKEN="$(issue_token worker)"
JOIN_TOKENS+=("$EXPIRED_TOKEN")
EXPIRED_TOKEN_ID="$(printf '%s' "$EXPIRED_TOKEN" | cut -d. -f3)"
docker exec "$POSTGRES_CONTAINER" psql -U app -d app -v ON_ERROR_STOP=1 -q \
  -c "UPDATE cluster_tokens SET expires_at = now() - interval '1 second' WHERE token_id = '${EXPIRED_TOKEN_ID}'" >/dev/null
if "$MGSCTL" cluster join --server "http://127.0.0.1:${ENROLLMENT_PORT}" --token "$EXPIRED_TOKEN" \
  --runtime-dir "$E2E_ROOT/runtime-expired" --mode docker --application-version "$IMAGE_TAG" \
  --image-registry "$IMAGE_REGISTRY" --image-tag "$IMAGE_TAG" >"$E2E_EVIDENCE_DIR/expired-token.out" 2>&1; then
  deployment_e2e_fail "expired cluster token was accepted"
fi
[[ ! -e "$E2E_ROOT/runtime-expired/config/runtime.env" ]]

MISMATCH_TOKEN="$(issue_token api)"
JOIN_TOKENS+=("$MISMATCH_TOKEN")
if "$MGSCTL" cluster join --server "http://127.0.0.1:${ENROLLMENT_PORT}" --token "$MISMATCH_TOKEN" \
  --runtime-dir "$E2E_ROOT/runtime-version-mismatch" --mode docker --application-version "incompatible-${RUN_ID}" \
  --image-registry "$IMAGE_REGISTRY" --image-tag "$IMAGE_TAG" >"$E2E_EVIDENCE_DIR/version-mismatch.out" 2>&1; then
  deployment_e2e_fail "application-version mismatch was accepted"
fi
[[ ! -e "$E2E_ROOT/runtime-version-mismatch/config/runtime.env" ]]

API_RUNTIME="$E2E_ROOT/runtime-api"
API_PORT="$(deployment_e2e_port)"
API_TOKEN="$(issue_token api)"
JOIN_TOKENS+=("$API_TOKEN")
join_node "$API_TOKEN" "$API_RUNTIME" "$API_PORT"
API_PROJECT="${PROJECTS[${#PROJECTS[@]}-1]}"
deployment_e2e_wait_status "http://127.0.0.1:${API_PORT}/readyz" 200 180

WORKER_ONE_RUNTIME="$E2E_ROOT/runtime-worker-1"
WORKER_ONE_TOKEN="$(issue_token worker)"
JOIN_TOKENS+=("$WORKER_ONE_TOKEN")
join_node "$WORKER_ONE_TOKEN" "$WORKER_ONE_RUNTIME"
WORKER_ONE_PROJECT="${PROJECTS[${#PROJECTS[@]}-1]}"
if "$MGSCTL" cluster join --server "http://127.0.0.1:${ENROLLMENT_PORT}" --token "$WORKER_ONE_TOKEN" \
  --runtime-dir "$E2E_ROOT/runtime-replay" --mode docker --application-version "$IMAGE_TAG" \
  --image-registry "$IMAGE_REGISTRY" --image-tag "$IMAGE_TAG" >"$E2E_EVIDENCE_DIR/replay.out" 2>&1; then
  deployment_e2e_fail "consumed cluster token was replayed successfully"
fi
[[ ! -e "$E2E_ROOT/runtime-replay/config/runtime.env" ]]

WORKER_TWO_RUNTIME="$E2E_ROOT/runtime-worker-2"
WORKER_TWO_TOKEN="$(issue_token worker)"
JOIN_TOKENS+=("$WORKER_TWO_TOKEN")
join_node "$WORKER_TWO_TOKEN" "$WORKER_TWO_RUNTIME"
WORKER_TWO_PROJECT="${PROJECTS[${#PROJECTS[@]}-1]}"

for key in SETUP_TOKEN CLUSTER_ENROLLMENT_SEAL_KEY POSTGRES_PASSWORD REDIS_PASSWORD MINIO_ROOT_PASSWORD; do
  assert_env_empty "$API_RUNTIME/config/runtime.env" "$key"
done
for runtime in "$WORKER_ONE_RUNTIME" "$WORKER_TWO_RUNTIME"; do
  [[ -n "$(deployment_e2e_env_value "$runtime/config/runtime.env" DATABASE_URL)" ]]
  for key in SETUP_TOKEN CLUSTER_ENROLLMENT_SEAL_KEY AUTH_ACCESS_TOKEN_SECRET API_KEY_SIGNING_SECRET_ENCRYPTION_KEY CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY; do
    assert_env_empty "$runtime/config/runtime.env" "$key"
  done
done

for secret in e2e-postgres-password e2e-redis-password e2e-minio-password \
  "$(deployment_e2e_env_value "$CONTROL_ENV" AUTH_ACCESS_TOKEN_SECRET)" \
  "$(deployment_e2e_env_value "$CONTROL_ENV" PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY)" \
  "$(deployment_e2e_env_value "$CONTROL_ENV" PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY)" \
  "${JOIN_TOKENS[@]}"; do
  if [[ -n "$secret" ]] && rg -F -q -- "$secret" "$ENROLLMENT_CAPTURE"; then
    rm -f "$ENROLLMENT_CAPTURE"
    deployment_e2e_fail "encrypted enrollment HTTP bodies exposed a plaintext credential"
  fi
done
printf 'encrypted enrollment request/response bodies checked=%s\nplaintext_credentials=false\n' \
  "$(wc -l <"$ENROLLMENT_CAPTURE" | tr -d ' ')" >"$E2E_EVIDENCE_DIR/enrollment-confidentiality.txt"
rm -f "$ENROLLMENT_CAPTURE"

ADMIN_LOGIN="$(curl -fsS -H 'Content-Type: application/json' --data '{"email":"admin@example.com","password":"admin123456"}' \
  "http://127.0.0.1:${CONTROL_API_PORT}/api/ops/admin/v1/auth/login")"
ADMIN_TOKEN="$(printf '%s' "$ADMIN_LOGIN" | deployment_e2e_json_data_field access_token)"
for _ in {1..60}; do
  NODES="$(curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
    "http://127.0.0.1:${CONTROL_API_PORT}/api/ops/admin/v1/cluster/nodes?page=1&page_size=20")"
  if printf '%s' "$NODES" | python3 -c '
import json,sys
items=json.load(sys.stdin)["data"]["items"]
roles=[item["role"] for item in items]
healthy=all(item.get("effective_health") == "healthy" for item in items)
drift=any(item.get("application_version_drift") or item.get("runtime_schema_drift") or item.get("config_revision_drift") for item in items)
raise SystemExit(0 if roles.count("control") == 1 and roles.count("api") == 1 and roles.count("worker") == 2 and healthy and not drift else 1)
'; then
    printf '%s\n' "$NODES" >"$E2E_EVIDENCE_DIR/cluster-nodes.json"
    break
  fi
  sleep 1
done
[[ -f "$E2E_EVIDENCE_DIR/cluster-nodes.json" ]] || deployment_e2e_fail "cluster nodes did not become healthy"

LB_PORT="$(deployment_e2e_port)"
LB_LOG="$E2E_EVIDENCE_DIR/load-balancer-upstreams.jsonl"
python3 "$ROOT_DIR/scripts/e2e/cluster-http-proxy.py" --listen "127.0.0.1:${LB_PORT}" \
  --upstream "http://127.0.0.1:${CONTROL_API_PORT}" --upstream "http://127.0.0.1:${API_PORT}" \
  --upstream-log "$LB_LOG" &
PROXY_PIDS+=("$!")
deployment_e2e_wait_status "http://127.0.0.1:${LB_PORT}/readyz" 200 30
for _ in {1..10}; do curl -fsS "http://127.0.0.1:${LB_PORT}/readyz" >/dev/null; done
rg -F -q "127.0.0.1:${CONTROL_API_PORT}" "$LB_LOG"
rg -F -q "127.0.0.1:${API_PORT}" "$LB_LOG"

deployment_e2e_compose "$CONTROL_RUNTIME" "$CONTROL_PROJECT" stop worker >/dev/null
MARKER="$E2E_EVIDENCE_DIR/image-provider-requests.jsonl"
BUSINESS_LOG="$E2E_EVIDENCE_DIR/business-e2e.log"
BASE_URL="http://127.0.0.1:${CONTROL_GATEWAY_PORT}" USER_WEB_URL="http://127.0.0.1:${CONTROL_GATEWAY_PORT}" \
  ADMIN_WEB_URL="http://127.0.0.1:${CONTROL_GATEWAY_PORT}/admin" NGINX_URL="http://127.0.0.1:${CONTROL_GATEWAY_PORT}" \
  E2E_SKIP_MIDDLEWARE_HEALTH=true E2E_IMAGE_PROVIDER_DELAY_MS=45000 E2E_IMAGE_PROVIDER_MARKER="$MARKER" \
  node "$ROOT_DIR/scripts/e2e/docker-e2e.mjs" >"$BUSINESS_LOG" 2>&1 &
BUSINESS_PID=$!
PROXY_PIDS+=("$BUSINESS_PID")
wait_for_file_lines "$MARKER" 1 "$BUSINESS_PID" 300

TASK_ROW="$(docker exec "$POSTGRES_CONTAINER" psql -U app -d app -At -v ON_ERROR_STOP=1 \
  -c "SELECT id::text || '|' || lease_owner FROM image_tasks WHERE prompt = 'docker e2e prompt' AND status = 'running' ORDER BY created_at DESC LIMIT 1")"
TASK_ID="${TASK_ROW%%|*}"
LEASE_OWNER="${TASK_ROW#*|}"
[[ -n "$TASK_ID" && -n "$LEASE_OWNER" && "$TASK_ROW" == *'|'* ]] || deployment_e2e_fail "no worker owned the delayed image task"

FAILED_WORKER=""
for runtime_project in "$WORKER_ONE_RUNTIME|$WORKER_ONE_PROJECT" "$WORKER_TWO_RUNTIME|$WORKER_TWO_PROJECT"; do
  runtime="${runtime_project%%|*}"
  project="${runtime_project#*|}"
  container="$(deployment_e2e_compose "$runtime" "$project" ps -q worker)"
  hostname="$(docker inspect --format '{{.Config.Hostname}}' "$container")"
  if [[ "$LEASE_OWNER" == "$hostname-"* ]]; then
    FAILED_WORKER="$container"
    break
  fi
done
[[ -n "$FAILED_WORKER" ]] || deployment_e2e_fail "could not map the task lease owner to a joined Worker"
docker stop "$FAILED_WORKER" >/dev/null
sleep 5
EARLY_RETRY_COUNT="$(rg -c '"prompt":"docker e2e prompt"' "$MARKER" || true)"
[[ "$EARLY_RETRY_COUNT" == 1 ]] || deployment_e2e_fail "another Worker claimed the task before the original lease expired"

wait "$BUSINESS_PID"
for index in "${!PROXY_PIDS[@]}"; do
  [[ "${PROXY_PIDS[$index]}" == "$BUSINESS_PID" ]] && unset 'PROXY_PIDS[index]'
done
RETRY_COUNT="$(rg -c '"prompt":"docker e2e prompt"' "$MARKER" || true)"
[[ "$RETRY_COUNT" == 2 ]] || deployment_e2e_fail "lease recovery provider attempts=$RETRY_COUNT, want 2"
TASK_ASSERTION="$(docker exec "$POSTGRES_CONTAINER" psql -U app -d app -At -v ON_ERROR_STOP=1 \
  -c "SELECT status || '|' || (SELECT count(*) FROM image_tasks WHERE prompt = 'docker e2e prompt') || '|' || (SELECT count(*) FROM point_ledgers WHERE task_id = '${TASK_ID}' AND ledger_type = 'consume') FROM image_tasks WHERE id = '${TASK_ID}'")"
[[ "$TASK_ASSERTION" == "succeeded|1|1" ]] || deployment_e2e_fail "exactly-once settlement assertion failed: $TASK_ASSERTION"

printf 'api_replica_ready=true\nload_balancer_distributed=true\njoined_workers=2\nlease_recovery_attempts=%s\nexactly_once_settlement=true\n' \
  "$RETRY_COUNT" >"$E2E_EVIDENCE_DIR/cluster-summary.txt"
echo "cluster Docker E2E passed; evidence: $E2E_EVIDENCE_DIR"

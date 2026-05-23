#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ -z "${BASE_URL:-}" ]]; then
  SMOKE_PORT="$(python3 - <<'PY'
import socket

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)"
  BASE_URL="http://127.0.0.1:${SMOKE_PORT}"
fi
API_ADDR="${BASE_URL#http://127.0.0.1}"
API_ADDR="${API_ADDR#http://localhost}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pic-gallery-api-smoke.XXXXXX")"
SMOKE_ID="$(basename "$TMP_DIR" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
DB_PATH="$TMP_DIR/smoke.db"
SERVER_LOG="$TMP_DIR/api.log"
COOKIE_JAR="$TMP_DIR/cookies.txt"
SMOKE_USER_EMAIL="smoke-user-${SMOKE_ID}@example.com"
SMOKE_ADMIN_EMAIL="admin-smoke-${SMOKE_ID}@example.com"
SERVER_PID=""
ADMIN_PASSWORD="$(python3 - <<'PY'
import secrets

print(secrets.token_urlsafe(24))
PY
)"
ACCESS_TOKEN_SECRET="$(python3 - <<'PY'
import secrets

print(secrets.token_urlsafe(32))
PY
)"
API_KEY_ENCRYPTION_KEY="$(python3 - <<'PY'
import secrets

print(secrets.token_urlsafe(32))
PY
)"

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

request() {
  curl --silent --show-error --fail "$@"
}

assert_json_field() {
  local json="$1"
  local expression="$2"
  JSON="$json" python3 - "$expression" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
expr = sys.argv[1]
value = data
for part in expr.split("."):
    if part:
        if isinstance(value, list):
            value = value[int(part)]
        else:
            value = value[part]
if value in ("", None):
    raise SystemExit(f"empty JSON field: {expr}")
print(value)
PY
}

body_sha256() {
  python3 - "$1" <<'PY'
import base64
import hashlib
import sys

body = sys.argv[1].encode()
print(base64.urlsafe_b64encode(hashlib.sha256(body).digest()).decode().rstrip("="))
PY
}

hmac_signature() {
  python3 - "$1" "$2" "$3" "$4" "$5" <<'PY'
import base64
import hmac
import hashlib
import sys

secret, method, path, timestamp, body_hash = sys.argv[1:]
payload = "\n".join([method.upper(), path, timestamp, body_hash]).encode()
sig = hmac.new(secret.encode(), payload, hashlib.sha256).digest()
print(base64.urlsafe_b64encode(sig).decode().rstrip("="))
PY
}

signed_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local body_hash timestamp signature
  body_hash="$(body_sha256 "$body")"
  timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  signature="$(hmac_signature "$API_SECRET" "$method" "$path" "$timestamp" "$body_hash")"
  if [[ -n "$body" ]]; then
    request -X "$method" "$BASE_URL$path" \
      -H "Content-Type: application/json" \
      -H "X-Access-Key: $ACCESS_KEY" \
      -H "X-Timestamp: $timestamp" \
      -H "X-Body-SHA256: $body_hash" \
      -H "X-Signature: $signature" \
      --data "$body"
  else
    request -X "$method" "$BASE_URL$path" \
      -H "X-Access-Key: $ACCESS_KEY" \
      -H "X-Timestamp: $timestamp" \
      -H "X-Body-SHA256: $body_hash" \
      -H "X-Signature: $signature"
  fi
}

cd "$ROOT_DIR"

APP_ADDR="$API_ADDR" \
DATABASE_URL="file:$DB_PATH?cache=shared&_fk=1" \
PIC_GALLERY_AUTH_DEV_EMAIL_CODES=true \
PIC_GALLERY_ADMIN_EMAIL="$SMOKE_ADMIN_EMAIL" \
PIC_GALLERY_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
AUTH_ACCESS_TOKEN_SECRET="$ACCESS_TOKEN_SECRET" \
API_KEY_SIGNING_SECRET_ENCRYPTION_KEY="$API_KEY_ENCRYPTION_KEY" \
REDIS_KEY_PREFIX="pic-gallery-smoke-${SMOKE_ID}" \
OPENAI_API_KEY="" \
OPENROUTER_API_KEY="" \
go run ./cmd/api >"$SERVER_LOG" 2>&1 &
SERVER_PID="$!"

for _ in {1..60}; do
  if request --max-time 2 "$BASE_URL/readyz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    echo "API server exited during startup. Log follows:" >&2
    cat "$SERVER_LOG" >&2
    exit 1
  fi
  sleep 0.5
done

ready_body="$(request "$BASE_URL/readyz")"
[[ "$(assert_json_field "$ready_body" "data.status")" == "ready" ]]

request -X POST "$BASE_URL/api/agent/auth/v1/email/send-code" \
  -H "Content-Type: application/json" \
  --data "$(SMOKE_USER_EMAIL="$SMOKE_USER_EMAIL" python3 - <<'PY'
import json
import os

print(json.dumps({
    "email": os.environ["SMOKE_USER_EMAIL"],
    "scene": "login",
}))
PY
)" >/dev/null

login_body="$(request -X POST "$BASE_URL/api/agent/auth/v1/login/email-code" \
  -H "Content-Type: application/json" \
  -c "$COOKIE_JAR" \
  --data "$(SMOKE_USER_EMAIL="$SMOKE_USER_EMAIL" python3 - <<'PY'
import json
import os

print(json.dumps({
    "email": os.environ["SMOKE_USER_EMAIL"],
    "code": "123456",
}))
PY
)")"
ACCESS_TOKEN="$(assert_json_field "$login_body" "data.access_token")"

profile_body="$(request "$BASE_URL/api/agent/user/v1/profile" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$profile_body" "data.email")" == "$SMOKE_USER_EMAIL" ]]
USER_ID="$(assert_json_field "$profile_body" "data.id")"

python3 - "$DB_PATH" "$USER_ID" <<'PY'
import sqlite3
import sys
from datetime import datetime, timezone

db_path, user_id = sys.argv[1], int(sys.argv[2])
now = datetime.now(timezone.utc).isoformat()
with sqlite3.connect(db_path) as conn:
    conn.execute(
        """
        INSERT INTO point_ledgers (
            created_at, updated_at, user_id, ledger_type, change_points,
            balance_after, frozen_after, reason, idempotency_key
        )
        VALUES (?, ?, ?, 'admin_adjust', '100.00000', '100.00000', '0.00000', 'api contract smoke seed', ?)
        """,
        (now, now, user_id, f"api-smoke-seed-{user_id}"),
    )
PY

estimate_body="$(request "$BASE_URL/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$estimate_body" "data.estimated_points")" == "2.00000" ]]

key_body="$(request -X POST "$BASE_URL/api/agent/account/v1/api-keys" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"name":"smoke-key","total_quota_points":"20.00000","daily_quota_points":"20.00000","rpm_limit":60}')"
ACCESS_KEY="$(assert_json_field "$key_body" "data.access_key")"
API_SECRET="$(assert_json_field "$key_body" "data.secret")"

open_estimate_path="/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0"
open_estimate_body="$(signed_request GET "$open_estimate_path")"
[[ "$(assert_json_field "$open_estimate_body" "data.estimated_points")" == "2.00000" ]]

open_task_body='{"task_type":"text_to_image","prompt":"smoke prompt","abstract_model":"basic","requested_quality":"auto","requested_size":"1024x1024","requested_output_image_count":1,"response_mode":"async"}'
open_task_resp="$(signed_request POST "/api/open/image/v1/tasks" "$open_task_body")"
[[ "$(assert_json_field "$open_task_resp" "data.status")" == "queued" ]]

models_body="$(request "$BASE_URL/v1/models" -H "Authorization: Bearer $API_SECRET")"
assert_json_field "$models_body" "data.0.id" >/dev/null

wrong_method_status="$(curl --silent --output "$TMP_DIR/wrong-method.json" --write-out "%{http_code}" -X POST "$BASE_URL/v1/models" -H "Authorization: Bearer $API_SECRET")"
[[ "$wrong_method_status" == "405" ]]
assert_json_field "$(cat "$TMP_DIR/wrong-method.json")" "error.code" >/dev/null

admin_login_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/auth/login" \
  -H "Content-Type: application/json" \
  --data "$(ADMIN_PASSWORD="$ADMIN_PASSWORD" SMOKE_ADMIN_EMAIL="$SMOKE_ADMIN_EMAIL" python3 - <<'PY'
import json
import os

print(json.dumps({
    "email": os.environ["SMOKE_ADMIN_EMAIL"],
    "password": os.environ["ADMIN_PASSWORD"],
}))
PY
)")"
ADMIN_TOKEN="$(assert_json_field "$admin_login_body" "data.access_token")"
tabs_body="$(request "$BASE_URL/api/ops/admin/v1/config-tabs" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_json_field "$tabs_body" "data.items.0.tab_key" >/dev/null

echo "API contract smoke passed: $BASE_URL"

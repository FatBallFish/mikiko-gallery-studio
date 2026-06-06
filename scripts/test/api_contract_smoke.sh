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
SMOKE_SUPER_ADMIN_EMAIL="super-admin-smoke-${SMOKE_ID}@example.com"
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

assert_json_array_contains() {
  local json="$1"
  local expression="$2"
  local expected="$3"
  JSON="$json" python3 - "$expression" "$expected" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
expr, expected = sys.argv[1], sys.argv[2]
value = data
for part in expr.split("."):
    if part:
        if isinstance(value, list):
            value = value[int(part)]
        else:
            value = value[part]
if not isinstance(value, list) or expected not in value:
    raise SystemExit(f"JSON array {expr} does not contain {expected}: {value!r}")
print(expected)
PY
}

assert_json_array_not_contains() {
  local json="$1"
  local expression="$2"
  local expected="$3"
  JSON="$json" python3 - "$expression" "$expected" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
expr, expected = sys.argv[1], sys.argv[2]
value = data
for part in expr.split("."):
    if part:
        if isinstance(value, list):
            value = value[int(part)]
        else:
            value = value[part]
if not isinstance(value, list):
    raise SystemExit(f"JSON field {expr} is not an array: {value!r}")
if expected in value:
    raise SystemExit(f"JSON array {expr} unexpectedly contains {expected}: {value!r}")
print(expected)
PY
}

assert_json_path_exists() {
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
        elif isinstance(value, dict) and part in value:
            value = value[part]
        else:
            raise SystemExit(f"missing JSON path: {expr}")
print(expr)
PY
}

assert_docs_examples_copyable() {
  local json="$1"
  JSON="$json" python3 - <<'PY'
import json
import os

data = json.loads(os.environ["JSON"])
items = data.get("data", {}).get("items", [])
if not items:
    raise SystemExit("docs examples are empty")
for item in items:
    code = str(item.get("code", "")).strip().lower()
    if not code:
        raise SystemExit(f"docs example code is empty: {item!r}")
    if code.startswith("<") or "<!doctype html" in code or ('id="root"' in code and "/src/main" in code):
        raise SystemExit(f"docs example contains HTML/bootstrap payload: {item!r}")
print("docs examples copyable")
PY
}

assert_docs_errors_include() {
  local json="$1"
  shift
  JSON="$json" python3 - "$@" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
items = data.get("data", {}).get("items", [])
codes = {str(item.get("code", "")) for item in items if isinstance(item, dict)}
missing = [code for code in sys.argv[1:] if code not in codes]
if missing:
    raise SystemExit(f"docs errors missing codes {missing!r}; got {sorted(codes)!r}")
print(",".join(sys.argv[1:]))
PY
}

assert_cashier_options_only_points_packages() {
  local json="$1"
  local forbidden_plan_code="$2"
  JSON="$json" python3 - "$forbidden_plan_code" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
forbidden_plan_code = sys.argv[1]
plans = data.get("data", {}).get("plans", [])
if not plans:
    raise SystemExit("cashier options returned no purchasable plans")
for plan in plans:
    if plan.get("plan_code") == forbidden_plan_code:
        raise SystemExit(f"cashier options exposed hidden subscription plan: {plan!r}")
    if plan.get("plan_type") != "points_package":
        raise SystemExit(f"cashier options exposed non-points plan: {plan!r}")
    if plan.get("purchase_enabled") is not True:
        raise SystemExit(f"cashier options exposed non-purchasable plan: {plan!r}")
    if plan.get("status") != "active":
        raise SystemExit(f"cashier options exposed inactive plan: {plan!r}")
print(len(plans))
PY
}

assert_provider_instance_secrets_redacted() {
  local json="$1"
  local name="$2"
  shift 2
  JSON="$json" python3 - "$name" "$@" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
name = sys.argv[1]
secret_values = [value for value in sys.argv[2:] if value]
body = json.dumps(data, ensure_ascii=False)
for secret in secret_values:
    if secret in body:
        raise SystemExit(f"provider instance response leaked secret value {secret!r}: {body}")

payload = data.get("data", {})
if isinstance(payload, dict) and isinstance(payload.get("items"), list):
    items = payload["items"]
elif isinstance(payload, dict):
    items = [payload]
else:
    items = []

target = None
for item in items:
    if item.get("name") == name:
        target = item
        break
if target is None:
    raise SystemExit(f"missing provider instance {name!r}: {items!r}")

config = target.get("config") or {}
for secret_key in ("app_private_key", "merchant_private_key", "api_v3_key", "key", "pkey", "merchant_key", "mch_key"):
    if secret_key in config:
        raise SystemExit(f"provider instance config exposed secret key {secret_key!r}: {config!r}")

credentials_status = target.get("credentials_status") or {}
if credentials_status.get("has_secret") is not True:
    raise SystemExit(f"provider instance should report redacted secret status: {credentials_status!r}")
if not str(credentials_status.get("fingerprint", "")).startswith("sha256:"):
    raise SystemExit(f"provider instance should expose secret fingerprint only: {credentials_status!r}")
print(target.get("id", "matched"))
PY
}

assert_ledger_entry() {
  local json="$1"
  local ledger_type="$2"
  local bucket_type="$3"
  local source_type="$4"
  JSON="$json" python3 - "$ledger_type" "$bucket_type" "$source_type" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
ledger_type, bucket_type, source_type = sys.argv[1:]
items = data.get("data", {}).get("items", [])
for item in items:
    if (
        item.get("ledger_type") == ledger_type
        and item.get("balance_bucket") == bucket_type
        and item.get("bucket_type") == bucket_type
        and item.get("source_type") == source_type
    ):
        print(item.get("id", "matched"))
        raise SystemExit(0)
raise SystemExit(f"missing ledger entry: {ledger_type}/{bucket_type}/{source_type}; got {items!r}")
PY
}

assert_ledger_entry_for_order() {
  local json="$1"
  local ledger_type="$2"
  local bucket_type="$3"
  local source_type="$4"
  local order_id="$5"
  JSON="$json" python3 - "$ledger_type" "$bucket_type" "$source_type" "$order_id" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
ledger_type, bucket_type, source_type, order_id = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
items = data.get("data", {}).get("items", [])
for item in items:
    if (
        item.get("ledger_type") == ledger_type
        and item.get("balance_bucket") == bucket_type
        and item.get("bucket_type") == bucket_type
        and item.get("source_type") == source_type
        and item.get("order_id") == order_id
    ):
        print(item.get("id", "matched"))
        raise SystemExit(0)
raise SystemExit(f"missing ledger entry for order {order_id}: {ledger_type}/{bucket_type}/{source_type}; got {items!r}")
PY
}

assert_readiness_check() {
  local json="$1"
  local key="$2"
  local status="$3"
  JSON="$json" python3 - "$key" "$status" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
key, status = sys.argv[1:]
items = data.get("data", {}).get("checks", [])
for item in items:
    if item.get("key") == key and item.get("status") == status and item.get("fix_route"):
        print(item.get("key"))
        raise SystemExit(0)
raise SystemExit(f"missing readiness check: {key}/{status}; got {items!r}")
PY
}

assert_webhook_event_in_list() {
  local json="$1"
  local event_id="$2"
  local order_id="$3"
  local status="$4"
  local event_type="$5"
  local failure_reason="$6"
  JSON="$json" python3 - "$event_id" "$order_id" "$status" "$event_type" "$failure_reason" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
event_id, order_id = int(sys.argv[1]), int(sys.argv[2])
status, event_type, failure_reason = sys.argv[3], sys.argv[4], sys.argv[5]
items = data.get("data", {}).get("items", [])
for item in items:
    if item.get("id") != event_id:
        continue
    if item.get("order_id") != order_id:
        raise SystemExit(f"webhook event order mismatch: want {order_id}, got {item!r}")
    if item.get("status") != status:
        raise SystemExit(f"webhook event status mismatch: want {status}, got {item!r}")
    if item.get("event_type") != event_type:
        raise SystemExit(f"webhook event type mismatch: want {event_type}, got {item!r}")
    if failure_reason and failure_reason not in (item.get("failure_reason") or ""):
        raise SystemExit(f"webhook event failure reason mismatch: want contains {failure_reason!r}, got {item!r}")
    if not item.get("order_no"):
        raise SystemExit(f"webhook event should include order_no: {item!r}")
    print(event_id)
    raise SystemExit(0)
raise SystemExit(f"missing webhook event {event_id}; got {items!r}")
PY
}

assert_webhook_event_retry_processed() {
  local json="$1"
  local event_id="$2"
  local order_id="$3"
  local event_type="$4"
  JSON="$json" python3 - "$event_id" "$order_id" "$event_type" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
event_id, order_id, event_type = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3]
item = data.get("data", {})
if item.get("id") != event_id:
    raise SystemExit(f"retry returned wrong event: want {event_id}, got {item!r}")
if item.get("order_id") != order_id:
    raise SystemExit(f"retry returned wrong order: want {order_id}, got {item!r}")
if item.get("status") != "processed":
    raise SystemExit(f"retry should mark event processed: {item!r}")
if item.get("event_type") != event_type:
    raise SystemExit(f"retry returned wrong event type: want {event_type}, got {item!r}")
if not item.get("processed_at"):
    raise SystemExit(f"retry should write processed_at: {item!r}")
if not item.get("order_no"):
    raise SystemExit(f"retry response should include order_no: {item!r}")
print(event_id)
PY
}

assert_call_record() {
  local json="$1"
  local user_id="$2"
  local error_code="$3"
  local source_channel="$4"
  JSON="$json" python3 - "$user_id" "$error_code" "$source_channel" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
user_id, error_code, source_channel = int(sys.argv[1]), sys.argv[2], sys.argv[3]
items = data.get("data", {}).get("items", [])
for item in items:
    if (
        item.get("user_id") == user_id
        and item.get("status") == "failed"
        and item.get("error_code") == error_code
        and item.get("source_channel") == source_channel
        and item.get("task_id")
    ):
        print(item["task_id"])
        raise SystemExit(0)
raise SystemExit(f"missing call record: user={user_id} error={error_code} source={source_channel}; got {items!r}")
PY
}

assert_admin_user_detail_core() {
  local json="$1"
  local user_id="$2"
  local email="$3"
  local expected_available="$4"
  local expected_recharge="$5"
  local expected_trial="$6"
  JSON="$json" python3 - "$user_id" "$email" "$expected_available" "$expected_recharge" "$expected_trial" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
user_id, email, expected_available, expected_recharge, expected_trial = int(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5]
payload = data.get("data", {})
user = payload.get("user", {})
balance = payload.get("balance", {})
if user.get("id") != user_id or user.get("email") != email:
    raise SystemExit(f"admin detail returned wrong user: {user!r}")
if balance.get("available_points") != expected_available:
    raise SystemExit(f"unexpected available balance: want {expected_available}, got {balance!r}")
if balance.get("recharge_points") != expected_recharge:
    raise SystemExit(f"unexpected recharge balance: want {expected_recharge}, got {balance!r}")
if balance.get("trial_points") != expected_trial:
    raise SystemExit(f"unexpected trial balance: want {expected_trial}, got {balance!r}")
buckets = {item.get("bucket"): item for item in balance.get("buckets", [])}
for bucket in ("trial", "recharge"):
    if bucket not in buckets:
        raise SystemExit(f"admin detail missing {bucket} bucket: {balance!r}")
print(user_id)
PY
}

assert_admin_user_detail_ledger() {
  local json="$1"
  local ledger_type="$2"
  local bucket_type="$3"
  local source_type="$4"
  JSON="$json" python3 - "$ledger_type" "$bucket_type" "$source_type" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
ledger_type, bucket_type, source_type = sys.argv[1:]
items = data.get("data", {}).get("recent_ledger", [])
for item in items:
    if (
        item.get("ledger_type") == ledger_type
        and item.get("bucket_type") == bucket_type
        and item.get("source_type") == source_type
    ):
        print(item.get("id", "matched"))
        raise SystemExit(0)
raise SystemExit(f"admin detail missing ledger entry: {ledger_type}/{bucket_type}/{source_type}; got {items!r}")
PY
}

assert_admin_user_detail_order() {
  local json="$1"
  local order_id="$2"
  local status="$3"
  JSON="$json" python3 - "$order_id" "$status" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
order_id, status = int(sys.argv[1]), sys.argv[2]
items = data.get("data", {}).get("recent_orders", [])
for item in items:
    if item.get("id") == order_id and item.get("status") == status and item.get("order_no"):
        print(order_id)
        raise SystemExit(0)
raise SystemExit(f"admin detail missing order {order_id}/{status}; got {items!r}")
PY
}

assert_admin_user_detail_task() {
  local json="$1"
  local task_id="$2"
  local status="$3"
  JSON="$json" python3 - "$task_id" "$status" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
task_id, status = sys.argv[1:]
items = data.get("data", {}).get("recent_tasks", [])
for item in items:
    if item.get("id") == task_id and item.get("status") == status:
        print(task_id)
        raise SystemExit(0)
raise SystemExit(f"admin detail missing task {task_id}/{status}; got {items!r}")
PY
}

assert_admin_user_detail_api_key() {
  local json="$1"
  local name="$2"
  local access_key="$3"
  JSON="$json" python3 - "$name" "$access_key" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
name, access_key = sys.argv[1:]
items = data.get("data", {}).get("api_keys", [])
for item in items:
    if item.get("name") == name and item.get("access_key") == access_key and item.get("status") == "active":
        print(access_key)
        raise SystemExit(0)
raise SystemExit(f"admin detail missing api key {name}/{access_key}; got {items!r}")
PY
}

assert_public_gallery_guest_list() {
  local json="$1"
  local image_id="$2"
  local full_prompt="$3"
  JSON="$json" python3 - "$image_id" "$full_prompt" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
image_id, full_prompt = sys.argv[1:]
items = data.get("data", {}).get("items", [])
for item in items:
    if item.get("id") != image_id:
        continue
    if item.get("prompt") not in (None, ""):
        raise SystemExit(f"guest list leaked prompt field: {item!r}")
    excerpt = item.get("prompt_excerpt") or ""
    if not excerpt:
        raise SystemExit(f"guest list missing prompt excerpt: {item!r}")
    if excerpt == full_prompt or len(excerpt) > 40:
        raise SystemExit(f"guest list excerpt should be redacted: {excerpt!r}")
    if full_prompt in json.dumps(item, ensure_ascii=False):
        raise SystemExit(f"guest list item contains full prompt: {item!r}")
    print(image_id)
    raise SystemExit(0)
raise SystemExit(f"missing public gallery image {image_id}; got {items!r}")
PY
}

assert_gallery_list_contains_status() {
  local json="$1"
  local image_id="$2"
  local status="$3"
  JSON="$json" python3 - "$image_id" "$status" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
image_id, status = sys.argv[1:]
payload = data.get("data", {})
if isinstance(payload, dict) and "items" in payload:
    items = payload.get("items", [])
else:
    items = [payload]
for item in items:
    if item.get("id") != image_id:
        continue
    if item.get("visibility_status") != status:
        raise SystemExit(f"unexpected visibility status: want {status}, got {item!r}")
    print(image_id)
    raise SystemExit(0)
raise SystemExit(f"missing image {image_id} with status {status}; got {items!r}")
PY
}

assert_gallery_list_excludes() {
  local json="$1"
  local image_id="$2"
  JSON="$json" python3 - "$image_id" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
image_id = sys.argv[1]
items = data.get("data", {}).get("items", [])
for item in items:
    if item.get("id") == image_id:
        raise SystemExit(f"image should not be visible yet: {item!r}")
print(image_id)
PY
}

assert_public_gallery_viewer_detail() {
  local json="$1"
  local image_id="$2"
  local full_prompt="$3"
  JSON="$json" python3 - "$image_id" "$full_prompt" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
image_id, full_prompt = sys.argv[1:]
item = data.get("data", {})
if item.get("id") != image_id:
    raise SystemExit(f"viewer detail returned wrong image: {item!r}")
if item.get("prompt") != full_prompt:
    raise SystemExit(f"viewer detail should expose full prompt: {item!r}")
if item.get("visibility_status") != "approved":
    raise SystemExit(f"viewer detail should expose approved image: {item!r}")
print(image_id)
PY
}

assert_public_gallery_viewer_list_state() {
  local json="$1"
  local image_id="$2"
  local field="$3"
  JSON="$json" python3 - "$image_id" "$field" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
image_id, field = sys.argv[1:]
items = data.get("data", {}).get("items", [])
for item in items:
    if item.get("id") != image_id:
        continue
    if item.get(field) is not True:
        raise SystemExit(f"viewer list missing {field}=true: {item!r}")
    print(image_id)
    raise SystemExit(0)
raise SystemExit(f"missing viewer list image {image_id}; got {items!r}")
PY
}

assert_cashier_order_state() {
  local json="$1"
  local status="$2"
  local amount_cny="$3"
  local points="$4"
  local expect_ledger="$5"
  JSON="$json" python3 - "$status" "$amount_cny" "$points" "$expect_ledger" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
status, amount_cny, points, expect_ledger = sys.argv[1:]
item = data.get("data", {})
if item.get("status") != status:
    raise SystemExit(f"unexpected order status: want {status}, got {item!r}")
if amount_cny and item.get("amount_cny") != amount_cny:
    raise SystemExit(f"unexpected order amount: want {amount_cny}, got {item!r}")
if points and item.get("points") != points:
    raise SystemExit(f"unexpected order points: want {points}, got {item!r}")
ledger_id = item.get("ledger_id") or 0
if expect_ledger == "yes" and not ledger_id:
    raise SystemExit(f"expected ledger id on order: {item!r}")
if expect_ledger == "no" and ledger_id:
    raise SystemExit(f"expected no ledger id on order: {item!r}")
print(item.get("id", "matched"))
PY
}

assert_cashier_manual_complete_state() {
  local json="$1"
  local order_id="$2"
  local provider="$3"
  local trade_no="$4"
  JSON="$json" python3 - "$order_id" "$provider" "$trade_no" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
order_id, provider, trade_no = int(sys.argv[1]), sys.argv[2], sys.argv[3]
item = data.get("data", {})
if item.get("id") != order_id:
    raise SystemExit(f"manual complete returned wrong order: want {order_id}, got {item!r}")
if item.get("status") != "completed":
    raise SystemExit(f"manual complete should return completed order: {item!r}")
if item.get("provider") != provider:
    raise SystemExit(f"manual complete returned wrong provider: want {provider}, got {item!r}")
if item.get("trade_no") != trade_no:
    raise SystemExit(f"manual complete returned wrong trade no: want {trade_no}, got {item!r}")
if not item.get("ledger_id"):
    raise SystemExit(f"manual complete should write ledger id: {item!r}")
if not item.get("completed_at"):
    raise SystemExit(f"manual complete should write completed_at: {item!r}")
print(order_id)
PY
}

assert_cashier_sync_state() {
  local json="$1"
  local order_id="$2"
  local trade_no="$3"
  local amount_cny="$4"
  local expected_completed="$5"
  JSON="$json" python3 - "$order_id" "$trade_no" "$amount_cny" "$expected_completed" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
order_id, trade_no, amount_cny, expected_completed = int(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4].lower() == "true"
payload = data.get("data", {})
order = payload.get("order") or {}
sync = payload.get("sync") or {}
if order.get("id") != order_id:
    raise SystemExit(f"sync returned wrong order: want {order_id}, got {payload!r}")
if order.get("status") != "completed":
    raise SystemExit(f"sync should complete paid order: {payload!r}")
if order.get("trade_no") != trade_no:
    raise SystemExit(f"sync returned wrong order trade no: want {trade_no}, got {payload!r}")
if not order.get("ledger_id"):
    raise SystemExit(f"sync should write ledger id: {payload!r}")
if sync.get("query_status") != "paid":
    raise SystemExit(f"sync query_status should be paid: {payload!r}")
if sync.get("paid") is not True:
    raise SystemExit(f"sync should mark paid=true: {payload!r}")
if sync.get("completed") is not expected_completed:
    raise SystemExit(f"sync completed flag mismatch: want {expected_completed}, got {payload!r}")
if sync.get("trade_no") != trade_no:
    raise SystemExit(f"sync returned wrong sync trade no: want {trade_no}, got {payload!r}")
if sync.get("amount_cny") != amount_cny:
    raise SystemExit(f"sync returned wrong amount: want {amount_cny}, got {payload!r}")
if not sync.get("synced_at"):
    raise SystemExit(f"sync should write synced_at: {payload!r}")
print(order_id)
PY
}

assert_cashier_sync_risk_state() {
  local json="$1"
  local order_id="$2"
  JSON="$json" python3 - "$order_id" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
order_id = int(sys.argv[1])
payload = data.get("data", {})
order = payload.get("order") or {}
sync = payload.get("sync") or {}
if order.get("id") != order_id:
    raise SystemExit(f"risk sync returned wrong order: want {order_id}, got {payload!r}")
if order.get("status") != "pending":
    raise SystemExit(f"risk sync should keep order pending for operator handling: {payload!r}")
if sync.get("query_status") != "failed":
    raise SystemExit(f"risk sync query_status should be failed: {payload!r}")
if sync.get("risk_category") != "risk_control":
    raise SystemExit(f"risk sync should classify risk_control: {payload!r}")
if "更换支付渠道" not in str(sync.get("action_hint", "")):
    raise SystemExit(f"risk sync should guide channel switching: {payload!r}")
print(order_id)
PY
}

assert_cashier_refund_state() {
  local json="$1"
  local status="$2"
  local refund_trade_no="$3"
  local refunded_amount_cny="$4"
  local refunded_points="$5"
  JSON="$json" python3 - "$status" "$refund_trade_no" "$refunded_amount_cny" "$refunded_points" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
status, refund_trade_no, refunded_amount_cny, refunded_points = sys.argv[1:]
item = data.get("data", {})
if item.get("status") != status:
    raise SystemExit(f"unexpected refund order status: want {status}, got {item!r}")
if item.get("refund_trade_no") != refund_trade_no:
    raise SystemExit(f"unexpected refund trade no: want {refund_trade_no}, got {item!r}")
if refunded_amount_cny and item.get("refunded_amount_cny") != refunded_amount_cny:
    raise SystemExit(f"unexpected refunded amount: want {refunded_amount_cny}, got {item!r}")
if refunded_points and item.get("refunded_points") != refunded_points:
    raise SystemExit(f"unexpected refunded points: want {refunded_points}, got {item!r}")
if status == "refunded" and not item.get("refunded_at"):
    raise SystemExit(f"expected refunded_at on refund order: {item!r}")
print(item.get("id", "matched"))
PY
}

assert_cashier_chargeback_state() {
  local json="$1"
  local order_id="$2"
  local expected_available="$3"
  local expected_recharge="$4"
  local expected_points="${5:-}"
  local expected_reason="${6:-}"
  local expected_key="${7:-}"
  JSON="$json" python3 - "$order_id" "$expected_available" "$expected_recharge" "$expected_points" "$expected_reason" "$expected_key" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["JSON"])
order_id, expected_available, expected_recharge = int(sys.argv[1]), sys.argv[2], sys.argv[3]
expected_points, expected_reason, expected_key = sys.argv[4], sys.argv[5], sys.argv[6]
payload = data.get("data", {})
order = payload.get("order") or {}
balance = payload.get("balance") or {}
if order.get("id") != order_id or not order.get("order_no"):
    raise SystemExit(f"chargeback response returned wrong order: {payload!r}")
if balance.get("available_points") != expected_available:
    raise SystemExit(f"unexpected chargeback available balance: want {expected_available}, got {balance!r}")
if balance.get("recharge_points") != expected_recharge:
    raise SystemExit(f"unexpected chargeback recharge balance: want {expected_recharge}, got {balance!r}")
if expected_points and order.get("chargeback_points") != expected_points:
    raise SystemExit(f"unexpected chargeback points: want {expected_points}, got {order!r}")
if expected_reason and order.get("chargeback_reason") != expected_reason:
    raise SystemExit(f"unexpected chargeback reason: want {expected_reason}, got {order!r}")
if expected_key and order.get("chargeback_idempotency_key") != expected_key:
    raise SystemExit(f"unexpected chargeback idempotency key: want {expected_key}, got {order!r}")
if expected_points and not order.get("chargeback_at"):
    raise SystemExit(f"expected chargeback_at on order: {order!r}")
print(order_id)
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

docs_body="$(request "$BASE_URL/docs/openapi.json")"
assert_json_field "$docs_body" "openapi" >/dev/null
assert_json_path_exists "$docs_body" "paths./api/agent/auth/v1/login/email-code.post" >/dev/null
assert_json_path_exists "$docs_body" "paths./api/agent/cashier/v1/orders.post" >/dev/null
assert_json_path_exists "$docs_body" "paths./api/open/image/v1/gallery/images.get" >/dev/null
assert_json_path_exists "$docs_body" "paths./api/ops/admin/v1/readiness.get" >/dev/null
assert_json_path_exists "$docs_body" "paths./api/ops/admin/v1/cashier/orders/{order_id}/refund.post" >/dev/null
examples_body="$(request "$BASE_URL/docs/examples")"
assert_json_field "$examples_body" "data.items.0.id" >/dev/null
assert_json_field "$examples_body" "data.items.0.title" >/dev/null
assert_json_field "$examples_body" "data.items.0.language" >/dev/null
assert_json_field "$examples_body" "data.items.0.code" >/dev/null
assert_docs_examples_copyable "$examples_body" >/dev/null
errors_body="$(request "$BASE_URL/docs/errors")"
assert_json_field "$errors_body" "data.items.0.code" >/dev/null
assert_docs_errors_include "$errors_body" \
  "MODEL_ROUTE_NOT_FOUND" \
  "MODEL_ROUTE_NO_CANDIDATE" \
  "ROUTE_MODEL_PRICE_MISSING" \
  "PAYMENT_METHOD_UNAVAILABLE" \
  "PAYMENT_PROVIDER_UNAVAILABLE" \
  "PAYMENT_TOO_MANY_PENDING_ORDERS" \
  "PAYMENT_SIGNATURE_INVALID" \
  "PAYMENT_AMOUNT_MISMATCH" >/dev/null

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
[[ "$(assert_json_field "$login_body" "data.signup_grant.granted")" == "True" || "$(assert_json_field "$login_body" "data.signup_grant.granted")" == "true" ]]
[[ "$(assert_json_field "$login_body" "data.signup_grant.balance.trial_points")" == "20.00000" ]]
[[ "$(assert_json_field "$login_body" "data.signup_grant.balance.buckets.0.bucket")" == "trial" ]]

profile_body="$(request "$BASE_URL/api/agent/user/v1/profile" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$profile_body" "data.email")" == "$SMOKE_USER_EMAIL" ]]
USER_ID="$(assert_json_field "$profile_body" "data.id")"

balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$balance_body" "data.trial_points")" == "20.00000" ]]
[[ "$(assert_json_field "$balance_body" "data.buckets.0.bucket")" == "trial" ]]
signup_ledger_body="$(request "$BASE_URL/api/agent/billing/v1/ledger?page=1&page_size=10" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_ledger_entry "$signup_ledger_body" "trial_grant" "trial" "signup" >/dev/null

cashier_options_body="$(request "$BASE_URL/api/agent/cashier/v1/options" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$cashier_options_body" "data.visible_methods.0.method")" == "mock" ]]
assert_json_field "$cashier_options_body" "data.plans.0.plan_code" >/dev/null
assert_cashier_options_only_points_packages "$cashier_options_body" "future-subscription-${SMOKE_ID}" >/dev/null

FUTURE_SUBSCRIPTION_PLAN_CODE="future-subscription-${SMOKE_ID}"
python3 - "$DB_PATH" "$FUTURE_SUBSCRIPTION_PLAN_CODE" <<'PY'
import sqlite3
import sys
from datetime import datetime, timezone

db_path, plan_code = sys.argv[1], sys.argv[2]
now = datetime.now(timezone.utc).isoformat()
with sqlite3.connect(db_path) as conn:
    conn.execute(
        """
        INSERT INTO subscription_plans (
            created_at, updated_at, plan_code, plan_name, status, price_cny,
            points, bonus_points, duration_days, currency, description,
            sort_order, metadata
        )
        VALUES (
            ?, ?, ?, 'Future Subscription Smoke', 'active', '29.90000',
            '200.00000', '0.00000', 30, 'CNY', 'future subscription placeholder',
            99, '{"plan_type":"subscription","purchase_enabled":true}'
        )
        """,
        (now, now, plan_code),
    )
PY

cashier_options_after_subscription_body="$(request "$BASE_URL/api/agent/cashier/v1/options" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_options_only_points_packages "$cashier_options_after_subscription_body" "$FUTURE_SUBSCRIPTION_PLAN_CODE" >/dev/null

subscription_order_status="$(curl --silent --output "$TMP_DIR/subscription-order.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$(FUTURE_SUBSCRIPTION_PLAN_CODE="$FUTURE_SUBSCRIPTION_PLAN_CODE" python3 - <<'PY'
import json
import os

print(json.dumps({
    "purchase_type": "plan",
    "plan_code": os.environ["FUTURE_SUBSCRIPTION_PLAN_CODE"],
    "visible_method": "mock",
}))
PY
)")"
[[ "$subscription_order_status" == "404" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/subscription-order.json")" "error.code")" == "NOT_FOUND" ]]

cashier_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
ORDER_ID="$(assert_json_field "$cashier_order_body" "data.id")"
[[ "$(assert_json_field "$cashier_order_body" "data.status")" == "pending" ]]

mock_pay_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${ORDER_ID}/mock-pay" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$mock_pay_body" "data.status")" == "completed" ]]
assert_json_field "$mock_pay_body" "data.ledger_id" >/dev/null

recharged_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$recharged_balance_body" "data.recharge_points")" == "100.00000" ]]
[[ "$(assert_json_field "$recharged_balance_body" "data.trial_points")" == "20.00000" ]]
recharge_ledger_body="$(request "$BASE_URL/api/agent/billing/v1/ledger?page=1&page_size=10" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_ledger_entry "$recharge_ledger_body" "recharge" "recharge" "payment_order" >/dev/null

python3 - "$DB_PATH" "$USER_ID" "$SMOKE_SUPER_ADMIN_EMAIL" "$ADMIN_PASSWORD" <<'PY'
import base64
import hashlib
import sqlite3
import sys
from datetime import datetime, timezone

db_path, user_id, super_admin_email, admin_password = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
now = datetime.now(timezone.utc).isoformat()
salt = "api-smoke-super-admin"
digest = hashlib.sha256(f"{salt}:{admin_password}".encode()).digest()
password_hash = f"sha256${salt}${base64.urlsafe_b64encode(digest).decode().rstrip('=')}"
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
    conn.execute(
        """
        INSERT INTO admin_users (created_at, updated_at, email, password_hash, role, status)
        VALUES (?, ?, ?, ?, 'super_admin', 'active')
        """,
        (now, now, super_admin_email, password_hash),
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
OPEN_TASK_ID="$(assert_json_field "$open_task_resp" "data.id")"

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
[[ "$(assert_json_field "$admin_login_body" "data.role")" == "admin" ]]
assert_json_array_contains "$admin_login_body" "data.permissions" "manage:cashier" >/dev/null
assert_json_array_not_contains "$admin_login_body" "data.permissions" "manage:admins" >/dev/null
assert_json_array_not_contains "$admin_login_body" "data.permissions" "manage:dangerous_config" >/dev/null
tabs_body="$(request "$BASE_URL/api/ops/admin/v1/config-tabs" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_json_field "$tabs_body" "data.items.0.tab_key" >/dev/null

admin_dangerous_config_status="$(curl --silent --output "$TMP_DIR/admin-dangerous-config.json" --write-out "%{http_code}" \
  -X PUT "$BASE_URL/api/ops/admin/v1/config-tabs/payments" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"version":1,"items":[{"config_category":"payments","config_key":"enabled","config_value":{"value":true},"scope":"global"}]}')"
[[ "$admin_dangerous_config_status" == "403" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/admin-dangerous-config.json")" "error.code")" == "FORBIDDEN" ]]

super_admin_login_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/auth/login" \
  -H "Content-Type: application/json" \
  --data "$(ADMIN_PASSWORD="$ADMIN_PASSWORD" SMOKE_SUPER_ADMIN_EMAIL="$SMOKE_SUPER_ADMIN_EMAIL" python3 - <<'PY'
import json
import os

print(json.dumps({
    "email": os.environ["SMOKE_SUPER_ADMIN_EMAIL"],
    "password": os.environ["ADMIN_PASSWORD"],
}))
PY
)")"
SUPER_ADMIN_TOKEN="$(assert_json_field "$super_admin_login_body" "data.access_token")"
[[ "$(assert_json_field "$super_admin_login_body" "data.role")" == "super_admin" ]]
assert_json_array_contains "$super_admin_login_body" "data.permissions" "manage:admins" >/dev/null
assert_json_array_contains "$super_admin_login_body" "data.permissions" "manage:dangerous_config" >/dev/null

super_admin_dangerous_config_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/config-tabs/payments" \
  -H "Authorization: Bearer $SUPER_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"version":1,"items":[{"config_category":"payments","config_key":"enabled","config_value":{"value":true},"scope":"global"}]}')"
[[ "$(assert_json_field "$super_admin_dangerous_config_body" "data.tab_key")" == "payments" ]]

alipay_secret="smoke-alipay-private-key-${SMOKE_ID}"
provider_create_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$(ALIPAY_SECRET="$alipay_secret" python3 - <<'PY'
import json
import os

print(json.dumps({
    "provider_type": "alipay_direct",
    "name": "Smoke Alipay Redaction",
    "enabled": True,
    "supported_methods": ["alipay"],
    "sort_order": 80,
    "scheduler_weight": 100,
    "limits": {"min_amount_cny": "1.00000", "max_amount_cny": "500.00000"},
    "config": {
        "app_id": "smoke-app",
        "app_private_key": os.environ["ALIPAY_SECRET"],
        "alipay_public_key": "smoke-public-key",
        "gateway_url": "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
    },
}))
PY
)")"
PROVIDER_INSTANCE_ID="$(assert_json_field "$provider_create_body" "data.id")"
assert_provider_instance_secrets_redacted "$provider_create_body" "Smoke Alipay Redaction" "$alipay_secret" >/dev/null

provider_list_body="$(request "$BASE_URL/api/ops/admin/v1/cashier/provider-instances?page=1&page_size=20" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_provider_instance_secrets_redacted "$provider_list_body" "Smoke Alipay Redaction" "$alipay_secret" >/dev/null

provider_detail_body="$(request "$BASE_URL/api/ops/admin/v1/cashier/provider-instances/${PROVIDER_INSTANCE_ID}" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_provider_instance_secrets_redacted "$provider_detail_body" "Smoke Alipay Redaction" "$alipay_secret" >/dev/null

wx_api_v3_secret="smoke-wx-api-v3-key-${SMOKE_ID}"
wx_private_key="smoke-wx-private-key-${SMOKE_ID}"
wx_provider_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$(WX_API_V3_SECRET="$wx_api_v3_secret" WX_PRIVATE_KEY="$wx_private_key" python3 - <<'PY'
import json
import os

print(json.dumps({
    "provider_type": "wxpay_direct",
    "name": "Smoke WxPay Redaction",
    "enabled": True,
    "supported_methods": ["wxpay"],
    "sort_order": 81,
    "scheduler_weight": 100,
    "config": {
        "app_id": "wx-smoke-app",
        "mch_id": "mch-smoke",
        "api_v3_key": os.environ["WX_API_V3_SECRET"],
        "merchant_private_key": os.environ["WX_PRIVATE_KEY"],
        "merchant_certificate_serial": "SMOKESERIAL",
        "wechat_pay_public_key": "smoke-wx-public-key",
    },
}))
PY
)")"
WX_PROVIDER_INSTANCE_ID="$(assert_json_field "$wx_provider_body" "data.id")"
assert_provider_instance_secrets_redacted "$wx_provider_body" "Smoke WxPay Redaction" "$wx_api_v3_secret" "$wx_private_key" >/dev/null

admin_user_detail_body="$(request "$BASE_URL/api/ops/admin/v1/users/${USER_ID}" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_admin_user_detail_core "$admin_user_detail_body" "$USER_ID" "$SMOKE_USER_EMAIL" "118.00000" "100.00000" "18.00000" >/dev/null
assert_admin_user_detail_ledger "$admin_user_detail_body" "trial_grant" "trial" "signup" >/dev/null
assert_admin_user_detail_ledger "$admin_user_detail_body" "recharge" "recharge" "payment_order" >/dev/null
assert_admin_user_detail_order "$admin_user_detail_body" "$ORDER_ID" "completed" >/dev/null
assert_admin_user_detail_task "$admin_user_detail_body" "$OPEN_TASK_ID" "queued" >/dev/null
assert_admin_user_detail_api_key "$admin_user_detail_body" "smoke-key" "$ACCESS_KEY" >/dev/null

admin_adjust_body='{"change_points":"7.00000","reason":"api smoke admin adjustment"}'
admin_adjust_key="admin-smoke-adjust-${SMOKE_ID}"
admin_point_adjustment_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/users/${USER_ID}/points-adjustments" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${admin_adjust_key}" \
  --data "$admin_adjust_body")"
[[ "$(assert_json_field "$admin_point_adjustment_body" "data.available_points")" == "125.00000" ]]

admin_point_adjustment_replay_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/users/${USER_ID}/points-adjustments" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${admin_adjust_key}" \
  --data "$admin_adjust_body")"
[[ "$(assert_json_field "$admin_point_adjustment_replay_body" "data.available_points")" == "125.00000" ]]

admin_point_adjustment_conflict_status="$(curl --silent --output "$TMP_DIR/admin-adjust-conflict.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/ops/admin/v1/users/${USER_ID}/points-adjustments" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${admin_adjust_key}" \
  --data '{"change_points":"8.00000","reason":"api smoke admin adjustment"}')"
[[ "$admin_point_adjustment_conflict_status" == "409" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/admin-adjust-conflict.json")" "error.code")" == "CONFLICT" ]]

admin_user_detail_after_adjust_body="$(request "$BASE_URL/api/ops/admin/v1/users/${USER_ID}" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_admin_user_detail_core "$admin_user_detail_after_adjust_body" "$USER_ID" "$SMOKE_USER_EMAIL" "125.00000" "100.00000" "18.00000" >/dev/null
assert_admin_user_detail_ledger "$admin_user_detail_after_adjust_body" "admin_adjust" "recharge" "admin" >/dev/null

custom_amount_config_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/custom-amount-config" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"enabled":true,"min_amount_cny":"5.00000","max_amount_cny":"500.00000","cny_per_point":"0.50000"}')"
[[ "$(assert_json_field "$custom_amount_config_body" "data.enabled")" == "True" || "$(assert_json_field "$custom_amount_config_body" "data.enabled")" == "true" ]]
[[ "$(assert_json_field "$custom_amount_config_body" "data.cny_per_point")" == "0.50000" ]]

custom_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
CUSTOM_ORDER_ID="$(assert_json_field "$custom_order_body" "data.id")"
assert_cashier_order_state "$custom_order_body" "pending" "10.00000" "20.00000" "no" >/dev/null

custom_pay_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${CUSTOM_ORDER_ID}/mock-pay" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$custom_pay_body" "completed" "10.00000" "20.00000" "yes" >/dev/null

partial_refund_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
PARTIAL_REFUND_ORDER_ID="$(assert_json_field "$partial_refund_order_body" "data.id")"
assert_cashier_order_state "$partial_refund_order_body" "pending" "10.00000" "20.00000" "no" >/dev/null

partial_refund_pay_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${PARTIAL_REFUND_ORDER_ID}/mock-pay" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$partial_refund_pay_body" "completed" "10.00000" "20.00000" "yes" >/dev/null

partial_refund_trade_no="REFUND-PARTIAL-SMOKE-${SMOKE_ID}"
partial_refund_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${PARTIAL_REFUND_ORDER_ID}/refund" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"refund_trade_no\":\"${partial_refund_trade_no}\",\"refund_amount_cny\":\"5.00000\",\"reason\":\"api smoke partial refund\"}")"
assert_cashier_refund_state "$partial_refund_body" "partially_refunded" "$partial_refund_trade_no" "5.00000" "10.00000" >/dev/null

partial_refunded_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$partial_refunded_balance_body" "data.recharge_points")" == "130.00000" ]]
[[ "$(assert_json_field "$partial_refunded_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$partial_refunded_balance_body" "data.frozen_points")" == "2.00000" ]]

partial_refund_finish_trade_no="REFUND-PARTIAL-FINISH-SMOKE-${SMOKE_ID}"
partial_refund_finish_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${PARTIAL_REFUND_ORDER_ID}/refund" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"refund_trade_no\":\"${partial_refund_finish_trade_no}\",\"reason\":\"api smoke partial refund finish\"}")"
assert_cashier_refund_state "$partial_refund_finish_body" "refunded" "$partial_refund_finish_trade_no" "10.00000" "20.00000" >/dev/null

cancel_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
CANCEL_ORDER_ID="$(assert_json_field "$cancel_order_body" "data.id")"
assert_cashier_order_state "$cancel_order_body" "pending" "" "" "no" >/dev/null

canceled_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${CANCEL_ORDER_ID}/cancel" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$canceled_order_body" "canceled" "" "" "no" >/dev/null
assert_json_field "$canceled_order_body" "data.closed_at" >/dev/null

canceled_mock_pay_status="$(curl --silent --output "$TMP_DIR/canceled-mock-pay.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/agent/cashier/v1/orders/${CANCEL_ORDER_ID}/mock-pay" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$canceled_mock_pay_status" == "409" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/canceled-mock-pay.json")" "error.code")" == "CONFLICT" ]]

canceled_detail_body="$(request "$BASE_URL/api/agent/cashier/v1/orders/${CANCEL_ORDER_ID}" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$canceled_detail_body" "canceled" "" "" "no" >/dev/null

manual_complete_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
MANUAL_COMPLETE_ORDER_ID="$(assert_json_field "$manual_complete_order_body" "data.id")"
assert_cashier_order_state "$manual_complete_order_body" "pending" "10.00000" "20.00000" "no" >/dev/null

manual_trade_no="MANUAL-SMOKE-${SMOKE_ID}"
manual_complete_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${MANUAL_COMPLETE_ORDER_ID}/complete" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"provider\":\"manual_alipay\",\"trade_no\":\"${manual_trade_no}\",\"reason\":\"api smoke manual complete\"}")"
assert_cashier_manual_complete_state "$manual_complete_body" "$MANUAL_COMPLETE_ORDER_ID" "manual_alipay" "$manual_trade_no" >/dev/null

manual_completed_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$manual_completed_balance_body" "data.recharge_points")" == "140.00000" ]]
[[ "$(assert_json_field "$manual_completed_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$manual_completed_balance_body" "data.frozen_points")" == "2.00000" ]]

manual_complete_ledger_body="$(request "$BASE_URL/api/agent/billing/v1/ledger?page=1&page_size=20" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_ledger_entry_for_order "$manual_complete_ledger_body" "recharge" "recharge" "payment_order" "$MANUAL_COMPLETE_ORDER_ID" >/dev/null

manual_complete_replay_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${MANUAL_COMPLETE_ORDER_ID}/complete" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"provider\":\"manual_alipay\",\"trade_no\":\"${manual_trade_no}\",\"reason\":\"api smoke manual complete replay\"}")"
assert_cashier_manual_complete_state "$manual_complete_replay_body" "$MANUAL_COMPLETE_ORDER_ID" "manual_alipay" "$manual_trade_no" >/dev/null

manual_complete_replay_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$manual_complete_replay_balance_body" "data.recharge_points")" == "140.00000" ]]
[[ "$(assert_json_field "$manual_complete_replay_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$manual_complete_replay_balance_body" "data.frozen_points")" == "2.00000" ]]

manual_refund_trade_no="REFUND-MANUAL-SMOKE-${SMOKE_ID}"
manual_complete_refund_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${MANUAL_COMPLETE_ORDER_ID}/refund" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"refund_trade_no\":\"${manual_refund_trade_no}\",\"reason\":\"api smoke rollback manual complete\"}")"
assert_cashier_refund_state "$manual_complete_refund_body" "refunded" "$manual_refund_trade_no" "10.00000" "20.00000" >/dev/null

sync_trade_no="SYNC-SMOKE-${SMOKE_ID}"
sync_provider_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$(SYNC_TRADE_NO="$sync_trade_no" python3 - <<'PY'
import json
import os

print(json.dumps({
    "provider_type": "mock",
    "name": "Smoke Mock Sync Paid",
    "enabled": True,
    "supported_methods": ["mock"],
    "sort_order": 0,
    "scheduler_weight": 100,
    "limits": {"min_amount_cny": "1.00000", "max_amount_cny": "500.00000"},
    "config": {
        "mock": True,
        "query_status": "paid",
        "query_trade_no": os.environ["SYNC_TRADE_NO"],
        "query_amount_cny": "10.00000",
    },
}))
PY
)")"
SYNC_PROVIDER_INSTANCE_ID="$(assert_json_field "$sync_provider_body" "data.id")"

sync_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
SYNC_ORDER_ID="$(assert_json_field "$sync_order_body" "data.id")"
assert_cashier_order_state "$sync_order_body" "pending" "10.00000" "20.00000" "no" >/dev/null
[[ "$(assert_json_field "$sync_order_body" "data.provider_instance_id")" == "$SYNC_PROVIDER_INSTANCE_ID" ]]

sync_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${SYNC_ORDER_ID}/sync" \
  -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_cashier_sync_state "$sync_body" "$SYNC_ORDER_ID" "$sync_trade_no" "10.00000" "true" >/dev/null

sync_completed_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$sync_completed_balance_body" "data.recharge_points")" == "140.00000" ]]
[[ "$(assert_json_field "$sync_completed_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$sync_completed_balance_body" "data.frozen_points")" == "2.00000" ]]

sync_ledger_body="$(request "$BASE_URL/api/agent/billing/v1/ledger?page=1&page_size=20" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_ledger_entry_for_order "$sync_ledger_body" "recharge" "recharge" "payment_order" "$SYNC_ORDER_ID" >/dev/null

sync_replay_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${SYNC_ORDER_ID}/sync" \
  -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_cashier_sync_state "$sync_replay_body" "$SYNC_ORDER_ID" "$sync_trade_no" "10.00000" "false" >/dev/null

sync_replay_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$sync_replay_balance_body" "data.recharge_points")" == "140.00000" ]]
[[ "$(assert_json_field "$sync_replay_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$sync_replay_balance_body" "data.frozen_points")" == "2.00000" ]]

sync_refund_trade_no="REFUND-SYNC-SMOKE-${SMOKE_ID}"
sync_refund_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${SYNC_ORDER_ID}/refund" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"refund_trade_no\":\"${sync_refund_trade_no}\",\"reason\":\"api smoke rollback sync complete\"}")"
assert_cashier_refund_state "$sync_refund_body" "refunded" "$sync_refund_trade_no" "10.00000" "20.00000" >/dev/null

sync_provider_disabled_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/provider-instances/${SYNC_PROVIDER_INSTANCE_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$(SYNC_TRADE_NO="$sync_trade_no" python3 - <<'PY'
import json
import os

print(json.dumps({
    "provider_type": "mock",
    "name": "Smoke Mock Sync Paid Disabled",
    "enabled": False,
    "supported_methods": ["mock"],
    "sort_order": 0,
    "scheduler_weight": 100,
    "limits": {"min_amount_cny": "1.00000", "max_amount_cny": "500.00000"},
    "config": {
        "mock": True,
        "query_status": "paid",
        "query_trade_no": os.environ["SYNC_TRADE_NO"],
        "query_amount_cny": "10.00000",
    },
}))
PY
)")"
[[ "$(assert_json_field "$sync_provider_disabled_body" "data.enabled")" == "False" || "$(assert_json_field "$sync_provider_disabled_body" "data.enabled")" == "false" ]]

round_robin_provider_a_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"mock",
    "name":"Smoke Mock Round Robin A",
    "enabled":true,
    "supported_methods":["mock"],
    "sort_order":1,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},
    "config":{"mock":true}
  }')"
ROUND_ROBIN_PROVIDER_A_ID="$(assert_json_field "$round_robin_provider_a_body" "data.id")"

round_robin_provider_b_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"mock",
    "name":"Smoke Mock Round Robin B",
    "enabled":true,
    "supported_methods":["mock"],
    "sort_order":2,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},
    "config":{"mock":true}
  }')"
ROUND_ROBIN_PROVIDER_B_ID="$(assert_json_field "$round_robin_provider_b_body" "data.id")"

round_robin_order_a_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
ROUND_ROBIN_ORDER_A_ID="$(assert_json_field "$round_robin_order_a_body" "data.id")"
assert_cashier_order_state "$round_robin_order_a_body" "pending" "10.00000" "20.00000" "no" >/dev/null
[[ "$(assert_json_field "$round_robin_order_a_body" "data.provider_instance_id")" == "$ROUND_ROBIN_PROVIDER_A_ID" ]]

round_robin_order_b_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
ROUND_ROBIN_ORDER_B_ID="$(assert_json_field "$round_robin_order_b_body" "data.id")"
assert_cashier_order_state "$round_robin_order_b_body" "pending" "10.00000" "20.00000" "no" >/dev/null
[[ "$(assert_json_field "$round_robin_order_b_body" "data.provider_instance_id")" == "$ROUND_ROBIN_PROVIDER_B_ID" ]]

round_robin_cancel_a_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${ROUND_ROBIN_ORDER_A_ID}/cancel" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$round_robin_cancel_a_body" "canceled" "10.00000" "20.00000" "no" >/dev/null

round_robin_cancel_b_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${ROUND_ROBIN_ORDER_B_ID}/cancel" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$round_robin_cancel_b_body" "canceled" "10.00000" "20.00000" "no" >/dev/null

round_robin_provider_a_disabled_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/provider-instances/${ROUND_ROBIN_PROVIDER_A_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"mock",
    "name":"Smoke Mock Round Robin A Disabled",
    "enabled":false,
    "supported_methods":["mock"],
    "sort_order":1,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},
    "config":{"mock":true}
  }')"
[[ "$(assert_json_field "$round_robin_provider_a_disabled_body" "data.enabled")" == "False" || "$(assert_json_field "$round_robin_provider_a_disabled_body" "data.enabled")" == "false" ]]

round_robin_provider_b_disabled_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/provider-instances/${ROUND_ROBIN_PROVIDER_B_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"mock",
    "name":"Smoke Mock Round Robin B Disabled",
    "enabled":false,
    "supported_methods":["mock"],
    "sort_order":2,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},
    "config":{"mock":true}
  }')"
[[ "$(assert_json_field "$round_robin_provider_b_disabled_body" "data.enabled")" == "False" || "$(assert_json_field "$round_robin_provider_b_disabled_body" "data.enabled")" == "false" ]]

sync_risk_provider_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"mock",
    "name":"Smoke Mock Sync Risk",
    "enabled":true,
    "supported_methods":["mock"],
    "sort_order":0,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},
    "config":{
      "mock":true,
      "query_status":"risk_control",
      "query_trade_no":"SYNC-RISK-SMOKE",
      "query_amount_cny":"10.00000"
    }
  }')"
SYNC_RISK_PROVIDER_INSTANCE_ID="$(assert_json_field "$sync_risk_provider_body" "data.id")"

sync_risk_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}')"
SYNC_RISK_ORDER_ID="$(assert_json_field "$sync_risk_order_body" "data.id")"
assert_cashier_order_state "$sync_risk_order_body" "pending" "10.00000" "20.00000" "no" >/dev/null
[[ "$(assert_json_field "$sync_risk_order_body" "data.provider_instance_id")" == "$SYNC_RISK_PROVIDER_INSTANCE_ID" ]]

sync_risk_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${SYNC_RISK_ORDER_ID}/sync" \
  -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_cashier_sync_risk_state "$sync_risk_body" "$SYNC_RISK_ORDER_ID" >/dev/null

sync_risk_cancel_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${SYNC_RISK_ORDER_ID}/cancel" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$sync_risk_cancel_body" "canceled" "10.00000" "20.00000" "no" >/dev/null

sync_risk_provider_disabled_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/provider-instances/${SYNC_RISK_PROVIDER_INSTANCE_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"mock",
    "name":"Smoke Mock Sync Risk Disabled",
    "enabled":false,
    "supported_methods":["mock"],
    "sort_order":0,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},
    "config":{
      "mock":true,
      "query_status":"risk_control",
      "query_trade_no":"SYNC-RISK-SMOKE",
      "query_amount_cny":"10.00000"
    }
  }')"
[[ "$(assert_json_field "$sync_risk_provider_disabled_body" "data.enabled")" == "False" || "$(assert_json_field "$sync_risk_provider_disabled_body" "data.enabled")" == "false" ]]

custom_amount_config_for_provider_limits_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/custom-amount-config" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"enabled":true,"min_amount_cny":"1.00000","max_amount_cny":"500.00000","cny_per_point":"0.50000"}')"
[[ "$(assert_json_field "$custom_amount_config_for_provider_limits_body" "data.min_amount_cny")" == "1.00000" ]]

wx_redaction_provider_disabled_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/provider-instances/${WX_PROVIDER_INSTANCE_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"wxpay_direct",
    "name":"Smoke WxPay Redaction Disabled",
    "enabled":false,
    "supported_methods":["wxpay"],
    "sort_order":81,
    "scheduler_weight":100,
    "config":{"app_id":"wx-smoke-app-disabled"}
  }')"
[[ "$(assert_json_field "$wx_redaction_provider_disabled_body" "data.enabled")" == "False" || "$(assert_json_field "$wx_redaction_provider_disabled_body" "data.enabled")" == "false" ]]

visible_methods_with_wxpay_body="$(request -X PUT "$BASE_URL/api/ops/admin/v1/cashier/visible-methods" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "items":[
      {"method":"mock","label":"Mock 支付","enabled":true,"source_provider_type":"mock","scheduler_strategy":"round_robin","display_order":10},
      {"method":"wxpay","label":"微信支付","enabled":true,"source_provider_type":"wxpay_direct","scheduler_strategy":"round_robin","display_order":20}
    ]
  }')"
assert_json_field "$visible_methods_with_wxpay_body" "data.items.0.method" >/dev/null

wxpay_limited_provider_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/provider-instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "provider_type":"wxpay_direct",
    "name":"Smoke WxPay Limited",
    "enabled":true,
    "supported_methods":["wxpay"],
    "sort_order":0,
    "scheduler_weight":100,
    "limits":{"min_amount_cny":"5.00000","max_amount_cny":"500.00000"},
    "config":{"app_id":"wx-limit-app","mch_id":"wx-limit-mch","qr_code":"weixin://wxpay/bizpayurl?pr=limit-smoke"}
  }')"
WXPAY_LIMITED_PROVIDER_ID="$(assert_json_field "$wxpay_limited_provider_body" "data.id")"

wxpay_below_provider_limit_status="$(curl --silent --output "$TMP_DIR/wxpay-provider-limit.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"4.00000","visible_method":"wxpay"}')"
[[ "$wxpay_below_provider_limit_status" == "409" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/wxpay-provider-limit.json")" "error.code")" == "PAYMENT_PROVIDER_UNAVAILABLE" ]]

wxpay_limit_order_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"wxpay"}')"
WXPAY_LIMIT_ORDER_ID="$(assert_json_field "$wxpay_limit_order_body" "data.id")"
assert_cashier_order_state "$wxpay_limit_order_body" "pending" "10.00000" "20.00000" "no" >/dev/null
[[ "$(assert_json_field "$wxpay_limit_order_body" "data.provider_instance_id")" == "$WXPAY_LIMITED_PROVIDER_ID" ]]
[[ "$(assert_json_field "$wxpay_limit_order_body" "data.payment_display.type")" == "qr_code" ]]

wxpay_limit_cancel_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders/${WXPAY_LIMIT_ORDER_ID}/cancel" \
  -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_cashier_order_state "$wxpay_limit_cancel_body" "canceled" "10.00000" "20.00000" "no" >/dev/null

pending_limit_key="cashier-pending-limit-${SMOKE_ID}"
pending_limit_first_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${pending_limit_key}" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
PENDING_LIMIT_FIRST_ID="$(assert_json_field "$pending_limit_first_body" "data.id")"
PENDING_LIMIT_FIRST_ORDER_NO="$(assert_json_field "$pending_limit_first_body" "data.order_no")"
assert_cashier_order_state "$pending_limit_first_body" "pending" "" "" "no" >/dev/null

pending_limit_replay_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${pending_limit_key}" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
[[ "$(assert_json_field "$pending_limit_replay_body" "data.id")" == "$PENDING_LIMIT_FIRST_ID" ]]
[[ "$(assert_json_field "$pending_limit_replay_body" "data.order_no")" == "$PENDING_LIMIT_FIRST_ORDER_NO" ]]
assert_cashier_order_state "$pending_limit_replay_body" "pending" "" "" "no" >/dev/null

pending_limit_second_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
assert_cashier_order_state "$pending_limit_second_body" "pending" "" "" "no" >/dev/null

pending_limit_third_body="$(request -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
assert_cashier_order_state "$pending_limit_third_body" "pending" "" "" "no" >/dev/null

pending_limit_status="$(curl --silent --output "$TMP_DIR/pending-limit.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/agent/cashier/v1/orders" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}')"
[[ "$pending_limit_status" == "409" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/pending-limit.json")" "error.code")" == "PAYMENT_TOO_MANY_PENDING_ORDERS" ]]

custom_recharged_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$custom_recharged_balance_body" "data.recharge_points")" == "120.00000" ]]
[[ "$(assert_json_field "$custom_recharged_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$custom_recharged_balance_body" "data.frozen_points")" == "2.00000" ]]

refund_trade_no="REFUND-SMOKE-${SMOKE_ID}"
custom_refund_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${CUSTOM_ORDER_ID}/refund" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"refund_trade_no\":\"${refund_trade_no}\",\"reason\":\"api smoke custom amount refund\"}")"
assert_cashier_refund_state "$custom_refund_body" "refunded" "$refund_trade_no" "10.00000" "20.00000" >/dev/null

refunded_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$refunded_balance_body" "data.recharge_points")" == "100.00000" ]]
[[ "$(assert_json_field "$refunded_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$refunded_balance_body" "data.frozen_points")" == "2.00000" ]]

refund_replay_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${CUSTOM_ORDER_ID}/refund" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data "{\"refund_trade_no\":\"${refund_trade_no}\",\"reason\":\"api smoke custom amount refund replay\"}")"
assert_cashier_refund_state "$refund_replay_body" "refunded" "$refund_trade_no" "10.00000" "20.00000" >/dev/null

refund_replay_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$refund_replay_balance_body" "data.recharge_points")" == "100.00000" ]]
[[ "$(assert_json_field "$refund_replay_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$refund_replay_balance_body" "data.frozen_points")" == "2.00000" ]]

refund_ledger_body="$(request "$BASE_URL/api/agent/billing/v1/ledger?page=1&page_size=20" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_ledger_entry "$refund_ledger_body" "payment_refund" "recharge" "payment_order" >/dev/null

chargeback_key="cashier-chargeback-smoke-${SMOKE_ID}"
chargeback_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${ORDER_ID}/chargeback" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${chargeback_key}" \
  --data '{"charge_points":"5.00000","reason":"api smoke provider chargeback"}')"
assert_cashier_chargeback_state "$chargeback_body" "$ORDER_ID" "120.00000" "95.00000" "5.00000" "api smoke provider chargeback" "$chargeback_key" >/dev/null

chargeback_order_detail_body="$(request "$BASE_URL/api/ops/admin/v1/cashier/orders/${ORDER_ID}" -H "Authorization: Bearer $ADMIN_TOKEN")"
[[ "$(assert_json_field "$chargeback_order_detail_body" "data.chargeback_points")" == "5.00000" ]]
[[ "$(assert_json_field "$chargeback_order_detail_body" "data.chargeback_reason")" == "api smoke provider chargeback" ]]
[[ "$(assert_json_field "$chargeback_order_detail_body" "data.chargeback_idempotency_key")" == "$chargeback_key" ]]
assert_json_path_exists "$chargeback_order_detail_body" "data.chargeback_at" >/dev/null

chargeback_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$chargeback_balance_body" "data.available_points")" == "120.00000" ]]
[[ "$(assert_json_field "$chargeback_balance_body" "data.recharge_points")" == "95.00000" ]]
[[ "$(assert_json_field "$chargeback_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$chargeback_balance_body" "data.frozen_points")" == "2.00000" ]]

chargeback_replay_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${ORDER_ID}/chargeback" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${chargeback_key}" \
  --data '{"charge_points":"5.00000","reason":"api smoke provider chargeback"}')"
assert_cashier_chargeback_state "$chargeback_replay_body" "$ORDER_ID" "120.00000" "95.00000" "5.00000" "api smoke provider chargeback" "$chargeback_key" >/dev/null

chargeback_replay_balance_body="$(request "$BASE_URL/api/agent/billing/v1/balance" -H "Authorization: Bearer $ACCESS_TOKEN")"
[[ "$(assert_json_field "$chargeback_replay_balance_body" "data.available_points")" == "120.00000" ]]
[[ "$(assert_json_field "$chargeback_replay_balance_body" "data.recharge_points")" == "95.00000" ]]
[[ "$(assert_json_field "$chargeback_replay_balance_body" "data.trial_points")" == "18.00000" ]]
[[ "$(assert_json_field "$chargeback_replay_balance_body" "data.frozen_points")" == "2.00000" ]]

chargeback_conflict_status="$(curl --silent --output "$TMP_DIR/chargeback-conflict.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${ORDER_ID}/chargeback" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: ${chargeback_key}" \
  --data '{"charge_points":"6.00000","reason":"api smoke provider chargeback"}')"
[[ "$chargeback_conflict_status" == "409" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/chargeback-conflict.json")" "error.code")" == "CONFLICT" ]]

chargeback_missing_key_status="$(curl --silent --output "$TMP_DIR/chargeback-missing-key.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/ops/admin/v1/cashier/orders/${ORDER_ID}/chargeback" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"charge_points":"1.00000","reason":"api smoke missing key"}')"
[[ "$chargeback_missing_key_status" == "400" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/chargeback-missing-key.json")" "error.code")" == "BAD_REQUEST" ]]

chargeback_ledger_body="$(request "$BASE_URL/api/agent/billing/v1/ledger?page=1&page_size=20" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_ledger_entry "$chargeback_ledger_body" "admin_adjust" "recharge" "admin" >/dev/null

read -r WEBHOOK_RETRY_EVENT_ID WEBHOOK_RETRY_FAILURE_REASON <<EOF
$(python3 - "$DB_PATH" "$ORDER_ID" "$SMOKE_ID" <<'PY'
import json
import sqlite3
import sys
from datetime import datetime, timezone

db_path, order_id, smoke_id = sys.argv[1], int(sys.argv[2]), sys.argv[3]
now = datetime.now(timezone.utc).isoformat()
failure_reason = f"api smoke retryable webhook failure {smoke_id}"
payload = json.dumps({"failure_reason": failure_reason}, separators=(",", ":"))
headers = json.dumps({"x-smoke": smoke_id}, separators=(",", ":"))
with sqlite3.connect(db_path) as conn:
    cursor = conn.execute(
        """
        INSERT INTO payment_webhook_events (
            created_at, updated_at, provider, trade_no, event_type, status,
            payment_order_id, headers, payload
        )
        VALUES (?, ?, 'mock', ?, 'payment.retryable_failed', 'failed', ?, ?, ?)
        """,
        (now, now, f"SMOKE-WEBHOOK-RETRY-{smoke_id}", order_id, headers, payload),
    )
    print(cursor.lastrowid, failure_reason)
PY
)
EOF

webhook_events_body="$(request "$BASE_URL/api/ops/admin/v1/cashier/webhook-events?page=1&page_size=20" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_webhook_event_in_list "$webhook_events_body" "$WEBHOOK_RETRY_EVENT_ID" "$ORDER_ID" "failed" "payment.retryable_failed" "$WEBHOOK_RETRY_FAILURE_REASON" >/dev/null

webhook_retry_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/cashier/webhook-events/${WEBHOOK_RETRY_EVENT_ID}/retry" \
  -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_webhook_event_retry_processed "$webhook_retry_body" "$WEBHOOK_RETRY_EVENT_ID" "$ORDER_ID" "payment.retryable_failed" >/dev/null

webhook_events_after_retry_body="$(request "$BASE_URL/api/ops/admin/v1/cashier/webhook-events?page=1&page_size=20" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_webhook_event_in_list "$webhook_events_after_retry_body" "$WEBHOOK_RETRY_EVENT_ID" "$ORDER_ID" "processed" "payment.retryable_failed" "$WEBHOOK_RETRY_FAILURE_REASON" >/dev/null

read -r PUBLIC_GALLERY_TASK_ID PUBLIC_GALLERY_IMAGE_ID PUBLIC_GALLERY_PROMPT <<EOF
$(python3 - "$DB_PATH" "$USER_ID" "$SMOKE_ID" <<'PY'
import hashlib
import sqlite3
import sys
import uuid
from datetime import datetime, timezone

db_path, user_id, smoke_id = sys.argv[1], int(sys.argv[2]), sys.argv[3]
task_id = str(uuid.uuid4())
image_id = str(uuid.uuid4())
prompt = f"Smoke public gallery full prompt {smoke_id} with protected detail text"
now = datetime.now(timezone.utc).isoformat()
object_key = f"https://cdn.example.com/pic-gallery-smoke/{image_id}.png"
sha256 = hashlib.sha256(object_key.encode()).hexdigest()

with sqlite3.connect(db_path) as conn:
    conn.execute(
        """
        INSERT INTO image_tasks (
            id, user_id, source_channel, task_type, status, prompt,
            abstract_model, requested_quality, resolved_quality_bucket,
            requested_size, resolved_width, resolved_height, aspect_ratio,
            requested_output_image_count, success_output_image_count,
            reference_image_count, mask_present, response_mode, save_policy,
            estimated_points, actual_points, route_model_code,
            pricing_snapshot, routing_snapshot, error_policy_snapshot,
            provider_trace, started_at, finished_at, created_at, updated_at
        )
        VALUES (
            ?, ?, 'web', 'text_to_image', 'succeeded', ?,
            'basic', 'auto', '1k',
            '1024x1024', 1024, 1024, '1:1',
            1, 1,
            0, 0, 'async', 'private',
            '2.00000', '2.00000', 'basic',
            '{}', '{}', '{}',
            '{}', ?, ?, ?, ?
        )
        """,
        (task_id, user_id, prompt, now, now, now, now),
    )
    conn.execute(
        """
        INSERT INTO task_images (
            id, task_id, user_id, image_role, storage_driver, object_key,
            mime_type, file_size_bytes, width, height, sha256, image_group,
            visibility_status, created_at, updated_at
        )
        VALUES (
            ?, ?, ?, 'output', 'remote', ?,
            'image/png', 128, 1024, 1024, ?, 'smoke',
            'pending_review', ?, ?
        )
        """,
        (image_id, task_id, user_id, object_key, sha256, now, now),
    )

print(task_id, image_id, prompt)
PY
)
EOF

pending_review_body="$(request "$BASE_URL/api/ops/admin/v1/image-reviews?page=1&page_size=10&status=pending_review" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_gallery_list_contains_status "$pending_review_body" "$PUBLIC_GALLERY_IMAGE_ID" "pending_review" >/dev/null

pre_approve_public_gallery_body="$(request "$BASE_URL/api/open/image/v1/gallery/images?page=1&page_size=10")"
assert_gallery_list_excludes "$pre_approve_public_gallery_body" "$PUBLIC_GALLERY_IMAGE_ID" >/dev/null

approved_review_body="$(request -X POST "$BASE_URL/api/ops/admin/v1/image-reviews/${PUBLIC_GALLERY_IMAGE_ID}:approve" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{}')"
assert_gallery_list_contains_status "$approved_review_body" "$PUBLIC_GALLERY_IMAGE_ID" "approved" >/dev/null

public_gallery_list_body="$(request "$BASE_URL/api/open/image/v1/gallery/images?page=1&page_size=10")"
assert_public_gallery_guest_list "$public_gallery_list_body" "$PUBLIC_GALLERY_IMAGE_ID" "$PUBLIC_GALLERY_PROMPT" >/dev/null

guest_public_detail_status="$(curl --silent --output "$TMP_DIR/public-gallery-guest-detail.json" --write-out "%{http_code}" \
  "$BASE_URL/api/open/image/v1/gallery/images/${PUBLIC_GALLERY_IMAGE_ID}")"
[[ "$guest_public_detail_status" == "401" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/public-gallery-guest-detail.json")" "error.code")" == "UNAUTHORIZED" ]]

viewer_public_detail_body="$(request "$BASE_URL/api/open/image/v1/gallery/images/${PUBLIC_GALLERY_IMAGE_ID}" -H "Authorization: Bearer $ACCESS_TOKEN")"
assert_public_gallery_viewer_detail "$viewer_public_detail_body" "$PUBLIC_GALLERY_IMAGE_ID" "$PUBLIC_GALLERY_PROMPT" >/dev/null

like_public_image_body="$(request -X POST "$BASE_URL/api/agent/gallery/v1/images/${PUBLIC_GALLERY_IMAGE_ID}/like" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"active":true}')"
[[ "$(assert_json_field "$like_public_image_body" "data.like_count")" == "1" ]]
[[ "$(assert_json_field "$like_public_image_body" "data.liked_by_viewer")" == "True" || "$(assert_json_field "$like_public_image_body" "data.liked_by_viewer")" == "true" ]]

favorite_public_image_body="$(request -X POST "$BASE_URL/api/agent/gallery/v1/images/${PUBLIC_GALLERY_IMAGE_ID}/favorite" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"active":true}')"
[[ "$(assert_json_field "$favorite_public_image_body" "data.favorite_count")" == "1" ]]
[[ "$(assert_json_field "$favorite_public_image_body" "data.favorited_by_viewer")" == "True" || "$(assert_json_field "$favorite_public_image_body" "data.favorited_by_viewer")" == "true" ]]

liked_public_gallery_body="$(request "$BASE_URL/api/open/image/v1/gallery/images?page=1&page_size=10&sort=hot&liked=true&access_token=${ACCESS_TOKEN}")"
assert_public_gallery_viewer_list_state "$liked_public_gallery_body" "$PUBLIC_GALLERY_IMAGE_ID" "liked_by_viewer" >/dev/null

favorited_public_gallery_body="$(request "$BASE_URL/api/open/image/v1/gallery/images?page=1&page_size=10&favorited=true&access_token=${ACCESS_TOKEN}")"
assert_public_gallery_viewer_list_state "$favorited_public_gallery_body" "$PUBLIC_GALLERY_IMAGE_ID" "favorited_by_viewer" >/dev/null

missing_route_code="missing-smoke-${SMOKE_ID}"
missing_route_status="$(curl --silent --output "$TMP_DIR/missing-route-task.json" --write-out "%{http_code}" \
  -X POST "$BASE_URL/api/agent/image/v1/tasks" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$(MISSING_ROUTE_CODE="$missing_route_code" python3 - <<'PY'
import json
import os

print(json.dumps({
    "task_type": "text_to_image",
    "prompt": "smoke route preflight failure",
    "route_model_code": os.environ["MISSING_ROUTE_CODE"],
    "requested_quality": "auto",
    "requested_size": "1024x1024",
    "requested_output_image_count": 1,
    "response_mode": "async",
}))
PY
)")"
[[ "$missing_route_status" == "404" ]]
[[ "$(assert_json_field "$(cat "$TMP_DIR/missing-route-task.json")" "error.code")" == "MODEL_ROUTE_NOT_FOUND" ]]

call_records_body="$(request "$BASE_URL/api/ops/admin/v1/call-records?page=1&page_size=5&status=failed&error_code=MODEL_ROUTE_NOT_FOUND&user_id=${USER_ID}&source_channel=web" -H "Authorization: Bearer $ADMIN_TOKEN")"
assert_call_record "$call_records_body" "$USER_ID" "MODEL_ROUTE_NOT_FOUND" "web" >/dev/null

readiness_body="$(request "$BASE_URL/api/ops/admin/v1/readiness" -H "Authorization: Bearer $ADMIN_TOKEN")"
[[ "$(assert_json_field "$readiness_body" "data.status")" == "fail" ]]
assert_json_field "$readiness_body" "data.summary.fail" >/dev/null
assert_json_field "$readiness_body" "data.checks.0.key" >/dev/null
assert_readiness_check "$readiness_body" "model_accounts" "fail" >/dev/null
assert_readiness_check "$readiness_body" "route_prices" "fail" >/dev/null

cashier_overview_body="$(request "$BASE_URL/api/ops/admin/v1/cashier/overview" -H "Authorization: Bearer $ADMIN_TOKEN")"
[[ "$(assert_json_field "$cashier_overview_body" "data.mock_enabled")" == "True" || "$(assert_json_field "$cashier_overview_body" "data.mock_enabled")" == "true" ]]
[[ "$(assert_json_field "$cashier_overview_body" "data.today_completed_count")" == "1" ]]

echo "API contract smoke passed: $BASE_URL"

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SMOKE="$ROOT_DIR/scripts/test/api_contract_smoke.sh"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mikiko-gallery-studio-api-smoke-contract.XXXXXX")"
STUB_BIN="$TMP_DIR/stubs"
INVOCATIONS="$TMP_DIR/invocations.log"
LISTENER_PID=""

bash -n "$SMOKE"
for marker in \
  'PREFLIGHT_PID=""' \
  'ordinary startup did not exit within' \
  'kill "$PREFLIGHT_PID"' \
  'wait "$PREFLIGHT_PID"'; do
  if ! grep -Fq "$marker" "$SMOKE"; then
    echo "API smoke ordinary-startup migration guard is not bounded/cleaned up: $marker" >&2
    exit 1
  fi
done

for marker in \
  'run_setup_initialization' \
  '/api/setup/v1/session' \
  'if [[ "$session_status" != "200" ]]' \
  'assert_json_path_exists "$(cat "$TMP_DIR/setup-session.json")" "data"' \
  '/api/setup/v1/apply' \
  'want supervisor restart code 75'; do
  if ! grep -Fq "$marker" "$SMOKE"; then
    echo "API smoke does not initialize through the approved setup transaction: $marker" >&2
    exit 1
  fi
done
if grep -Eq 'PIC_GALLERY_ADMIN_(EMAIL|PASSWORD|ROLE)=' "$SMOKE"; then
  echo "API smoke still seeds an administrator from plaintext runtime extensions" >&2
  exit 1
fi

for marker in \
  'wait_for_postgres_final_server()' \
  'for _ in {1..80}' \
  'psql -X -qAt' \
  '-h 127.0.0.1' \
  '-p 5432' \
  "-c 'SELECT 1'" \
  'PostgreSQL final server did not become ready within the bounded startup window'; do
  if ! grep -Fq -- "$marker" "$SMOKE"; then
    echo "API smoke does not wait for the persistent PostgreSQL TCP server: $marker" >&2
    exit 1
  fi
done
if grep -Fq 'pg_isready -U "$POSTGRES_SUPERUSER" -d postgres' "$SMOKE"; then
  echo "API smoke still accepts the PostgreSQL image temporary init server as ready" >&2
  exit 1
fi
if grep -Eq 'PostgreSQL final server.*(PASSWORD|password|postgres://|psql:)' "$SMOKE"; then
  echo "API smoke PostgreSQL readiness diagnostic may expose connection details" >&2
  exit 1
fi

for marker in \
  'POSTGRES_TEST_URL=' \
  'PIC_GALLERY_TEST_POSTGRES_URL="$POSTGRES_TEST_URL"' \
  "-run '^TestTextModelStore.*Postgres'" \
  "-run '^TestBillingStore(Postgres(CancelAndPaidReconciliationEndsCompleted|ConcurrentDuplicatePaidCallbacksAreIdempotent)|UpdatePlanSerializesWithLifecycleTransitionsPostgres)$'" \
  'go test ./internal/repository/db' \
  "-run '^TestSchemaV2MigratesLegacyRefreshSessions$'"; do
  if ! grep -Fq -- "$marker" "$SMOKE"; then
    echo "API smoke does not execute required PostgreSQL integration coverage: $marker" >&2
    exit 1
  fi
done

for marker in \
  'wxpay_limit_cancel_status=' \
  '[[ "$wxpay_limit_cancel_status" == "409" ]]' \
  'wxpay_limit_after_cancel_body=' \
  'assert_cashier_order_state "$wxpay_limit_after_cancel_body" "pending"' \
  'UPDATE payment_orders' \
  "SET status = 'canceled', closed_at = CURRENT_TIMESTAMP"; do
  if ! grep -Fq -- "$marker" "$SMOKE"; then
    echo "API smoke does not preserve and clean up an uncertain WxPay cancellation: $marker" >&2
    exit 1
  fi
done

for marker in \
  'CONFIG_REVISION=1' \
  'CLUSTER_ENROLLMENT_SEAL_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' \
  'SMOKE_INSTALL_STATE_PATH="$TMP_DIR/install-state.json"' \
  '"phase": "completed"' \
  '"config_revision": 1'; do
  if ! grep -Fq "$marker" "$SMOKE"; then
    echo "API smoke completed runtime is missing matching install-state fixture: $marker" >&2
    exit 1
  fi
done

cleanup() {
  if [[ -n "$LISTENER_PID" ]] && kill -0 "$LISTENER_PID" >/dev/null 2>&1; then
    kill "$LISTENER_PID" >/dev/null 2>&1 || true
    wait "$LISTENER_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$STUB_BIN"
for command in docker curl go; do
  cat >"$STUB_BIN/$command" <<'SH'
#!/usr/bin/env sh
printf '%s\n' "$(basename "$0") $*" >>"${SMOKE_CONTRACT_INVOCATIONS:?}"
exit 97
SH
  chmod +x "$STUB_BIN/$command"
done

assert_rejected_before_side_effects() {
  local name=$1
  local base_url=$2
  local expected=$3
  local output="$TMP_DIR/$name.out"
  : >"$INVOCATIONS"
  if PATH="$STUB_BIN:$PATH" SMOKE_CONTRACT_INVOCATIONS="$INVOCATIONS" BASE_URL="$base_url" \
    "$SMOKE" >"$output" 2>&1; then
    echo "API smoke accepted invalid BASE_URL for $name: $base_url" >&2
    exit 1
  fi
  if [[ -s "$INVOCATIONS" ]]; then
    echo "API smoke performed side effects before rejecting $name:" >&2
    cat "$INVOCATIONS" >&2
    exit 1
  fi
  grep -Fq "$expected" "$output"
}

assert_rejected_before_side_effects remote "http://example.com:18081" "BASE_URL must be"
assert_rejected_before_side_effects https "https://127.0.0.1:18081" "BASE_URL must be"
assert_rejected_before_side_effects path "http://127.0.0.1:18081/readyz" "BASE_URL must be"
assert_rejected_before_side_effects query "http://127.0.0.1:18081?target=readyz" "BASE_URL must be"
assert_rejected_before_side_effects fragment "http://127.0.0.1:18081#readyz" "BASE_URL must be"
assert_rejected_before_side_effects userinfo "http://user@127.0.0.1:18081" "BASE_URL must be"
assert_rejected_before_side_effects missing-port "http://127.0.0.1" "BASE_URL must be"
assert_rejected_before_side_effects zero-port "http://127.0.0.1:0" "BASE_URL port must be between 1 and 65535"
assert_rejected_before_side_effects high-port "http://localhost:65536" "BASE_URL port must be between 1 and 65535"
assert_rejected_before_side_effects nonnumeric-port "http://localhost:http" "BASE_URL must be"

if ! grep -Fq 'base_url = f"http://127.0.0.1:{port}"' "$SMOKE"; then
  echo "API smoke does not canonicalize localhost requests to the owned IPv4 listener" >&2
  exit 1
fi
if grep -Fq 'base_url = raw' "$SMOKE"; then
  echo "API smoke may send localhost requests to an unowned IPv6 listener" >&2
  exit 1
fi

PORT_FILE="$TMP_DIR/listener.port"
python3 - "$PORT_FILE" <<'PY' &
import socket
import sys
import time

path = sys.argv[1]
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
    listener.bind(("127.0.0.1", 0))
    listener.listen()
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(str(listener.getsockname()[1]))
    time.sleep(30)
PY
LISTENER_PID="$!"
for _ in {1..40}; do
  [[ -s "$PORT_FILE" ]] && break
  kill -0 "$LISTENER_PID" >/dev/null 2>&1
  sleep 0.05
done
[[ -s "$PORT_FILE" ]]
OCCUPIED_PORT="$(cat "$PORT_FILE")"
assert_rejected_before_side_effects occupied-port \
  "http://127.0.0.1:$OCCUPIED_PORT" "BASE_URL port is already in use"

echo "OK: API contract smoke BASE_URL safety contract passed"

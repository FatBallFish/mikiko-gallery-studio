#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

assert_contains() {
  local file=$1
  local expected=$2
  grep -Fq "$expected" "$file"
}

assert_not_contains() {
  local file=$1
  local unexpected=$2
  ! grep -Fq "$unexpected" "$file"
}

DEVOPS_TARGET_ROOT="$TMP_ROOT/subpath" scripts/devops/package.sh admin-web
assert_contains "$TMP_ROOT/subpath/admin-web/dist/index.html" '/admin/assets/'
assert_contains "$TMP_ROOT/subpath/admin-web/dist/index.html" '/admin/env.js'
assert_contains "$TMP_ROOT/subpath/admin-web/nginx-subpath.conf" 'location /admin/'
assert_contains "$TMP_ROOT/subpath/admin-web/nginx-domain.conf" 'try_files $uri $uri/ /index.html'

DEVOPS_TARGET_ROOT="$TMP_ROOT/domain" PIC_GALLERY_ADMIN_BASE_PATH=/ scripts/devops/package.sh admin-web
assert_not_contains "$TMP_ROOT/domain/admin-web/dist/index.html" '/admin/assets/'
assert_contains "$TMP_ROOT/domain/admin-web/dist/index.html" '/assets/'
assert_contains "$TMP_ROOT/domain/admin-web/dist/index.html" '/env.js'

DEVOPS_TARGET_ROOT="$TMP_ROOT/backend" scripts/devops/package.sh api-server
DEVOPS_TARGET_ROOT="$TMP_ROOT/backend" scripts/devops/package.sh worker
assert_contains "$TMP_ROOT/backend/api-server/run-api-server.sh" 'PIC_GALLERY_API_BIN_PATH:-"$APP_DIR/pic-gallery-api"'
assert_contains "$TMP_ROOT/backend/api-server/run-api-server.sh" 'ExecStart=$BIN_PATH'
assert_contains "$TMP_ROOT/backend/worker/run-worker.sh" 'PIC_GALLERY_WORKER_BIN_PATH:-"$APP_DIR/pic-gallery-worker"'
assert_contains "$TMP_ROOT/backend/worker/run-worker.sh" 'ExecStart=$BIN_PATH'
test -x "$TMP_ROOT/backend/api-server/pic-gallery-api"
test -x "$TMP_ROOT/backend/worker/pic-gallery-worker"

printf 'OK: devops package contract verified\n'

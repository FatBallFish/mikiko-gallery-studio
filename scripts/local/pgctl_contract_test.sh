#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

./scripts/local/pgctl.sh build --components api,worker

API_SCRIPT="$ROOT_DIR/target/local/bin/start-pic-gallery-api.sh"
WORKER_SCRIPT="$ROOT_DIR/target/local/bin/start-pic-gallery-worker.sh"

test -x "$API_SCRIPT"
test -x "$WORKER_SCRIPT"
test "$API_SCRIPT" != "$WORKER_SCRIPT"

grep -Fq 'COMPONENT="api"' "$API_SCRIPT"
grep -Fq 'COMPONENT="worker"' "$WORKER_SCRIPT"
grep -Fq 'service_is_registered()' "$API_SCRIPT"
grep -Fq 'list-unit-files "$unit"' "$API_SCRIPT"
grep -Fq 'LaunchAgents/com.picgallery.$COMPONENT.plist' "$WORKER_SCRIPT"
grep -Fq 'scripts/service/manage.sh" install --components "$COMPONENT"' "$API_SCRIPT"
grep -Fq 'scripts/service/manage.sh" install --components "$COMPONENT"' "$WORKER_SCRIPT"
grep -Fq 'scripts/service/manage.sh" restart --components "$COMPONENT"' "$API_SCRIPT"
grep -Fq 'scripts/service/manage.sh" start --components "$COMPONENT"' "$WORKER_SCRIPT"

printf 'OK: pgctl local build service startup scripts verified\n'

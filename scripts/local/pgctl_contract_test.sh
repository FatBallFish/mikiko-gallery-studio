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
grep -Fq 'list-unit-files "$UNIT"' "$API_SCRIPT"
grep -Fq 'launchd_plist_path()' "$WORKER_SCRIPT"
grep -Fq 'install -m 0644 "$tmp_plist" "$plist"' "$WORKER_SCRIPT"
grep -Fq 'install_systemd_service()' "$API_SCRIPT"
grep -Fq 'install_launchd_service()' "$WORKER_SCRIPT"
grep -Fq 'systemctl "${SYSTEMCTL_ARGS[@]}" restart "$UNIT"' "$API_SCRIPT"
grep -Fq 'systemctl "${SYSTEMCTL_ARGS[@]}" start "$UNIT"' "$WORKER_SCRIPT"
grep -Fq 'launchctl bootstrap "$DOMAIN" "$PLIST"' "$API_SCRIPT"
! grep -Fq 'scripts/service/manage.sh' "$API_SCRIPT"
! grep -Fq 'scripts/service/manage.sh' "$WORKER_SCRIPT"

printf 'OK: pgctl local build service startup scripts verified\n'

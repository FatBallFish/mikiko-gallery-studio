#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

require_text() {
  local file=$1 text=$2
  rg -Fq -- "$text" "$file" || {
    echo "$file is missing deployment documentation: $text" >&2
    exit 1
  }
}

forbid_text() {
  local file=$1 text=$2
  if rg -Fq -- "$text" "$file"; then
    echo "$file still documents a retired deployment entrypoint: $text" >&2
    exit 1
  fi
}

for file in README.md README.zh-CN.md; do
  path="$ROOT/$file"
  for required in \
    "mgsctl" \
    "./scripts/install.sh install" \
    "--mode docker" \
    "--profile full" \
    "--topology single" \
    "make mgsctl" \
    'MGSCTL_INSTALL_DIR' \
    'MGSCTL_BIN' \
    'MGSCTL_VERSION' \
    'MGSCTL_RELEASE_BASE_URL' \
    'MGSCTL_DOWNLOAD_URL' \
    'MGSCTL_SHA256' \
    'MGSCTL_SOURCE_DIR' \
    '--overwrite' \
    '$HOME/.local/bin/mgsctl' \
    '%LOCALAPPDATA%\Programs\mgsctl\mgsctl.exe' \
    'mgsctl version' \
    'mgsctl version --json' \
    'mgsctl self-update' \
    'mgsctl upgrade' \
    'make dev' \
    'make worker' \
    './scripts/workflow/verify.sh'; do
    require_text "$path" "$required"
  done
  for retired in \
    'scripts/local/pgctl' \
    'scripts/service/manage' \
    'service-install' \
    'service-uninstall' \
    'local-build' \
    'local-up'; do
    forbid_text "$path" "$retired"
  done
done

require_text "$ROOT/README.md" "## Developer Local Workflow"
require_text "$ROOT/README.md" "Release artifact"
require_text "$ROOT/README.md" "checksum mismatch"
require_text "$ROOT/README.zh-CN.md" "## 开发者本地调试"
require_text "$ROOT/README.zh-CN.md" "Release 产物"
require_text "$ROOT/README.zh-CN.md" "校验和不一致"

for required in 'mgsctl --help' 'Arrow keys' 'Ctrl+C' 'http://127.0.0.1:5173' 'http://127.0.0.1:5174' 'http://127.0.0.1:5175' '/developer-docs/' 'redirected output'; do
  require_text "$ROOT/README.md" "$required"
done
for required in 'mgsctl --help' '方向键' 'Ctrl+C' 'http://127.0.0.1:5173' 'http://127.0.0.1:5174' 'http://127.0.0.1:5175' '/developer-docs/' '重定向输出'; do
  require_text "$ROOT/README.zh-CN.md" "$required"
done
for required in 'mgsctl --help' 'TUI' 'Direct ports' 'Gateway paths' 'API-hosted Setup' 'redirected output'; do
  require_text "$ROOT/docs/runbooks/backend-deployment.md" "$required"
done

echo "OK: bilingual mgsctl-only deployment documentation verified"

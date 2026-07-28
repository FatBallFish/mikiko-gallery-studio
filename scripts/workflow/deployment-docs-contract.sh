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
    "deployctl" \
    "./scripts/install.sh install" \
    "--mode docker" \
    "--profile full" \
    "--topology single" \
    "make deployctl" \
    'DEPLOYCTL_INSTALL_DIR' \
    'DEPLOYCTL_BIN' \
    'DEPLOYCTL_VERSION' \
    'DEPLOYCTL_RELEASE_BASE_URL' \
    'DEPLOYCTL_DOWNLOAD_URL' \
    'DEPLOYCTL_SHA256' \
    'DEPLOYCTL_SOURCE_DIR' \
    '--overwrite' \
    '$HOME/.local/bin/deployctl' \
    '%LOCALAPPDATA%\Programs\deployctl\deployctl.exe' \
    'deployctl version' \
    'deployctl version --json' \
    'deployctl self-update' \
    'deployctl upgrade' \
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

echo "OK: bilingual deployctl-only deployment documentation verified"

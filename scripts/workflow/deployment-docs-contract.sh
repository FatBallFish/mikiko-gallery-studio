#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
UPGRADE_E2E="$ROOT/scripts/e2e/mgsctl-upgrade-docker-e2e.sh"

[[ -x "$UPGRADE_E2E" ]] || {
  echo "missing executable Docker upgrade E2E: $UPGRADE_E2E" >&2
  exit 1
}

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
    'release-manifest.json' \
    'docker.io/fatballfish/mikiko-gallery-studio-api' \
    'mgsctl/config.json' \
    'zh-CN' \
    'en-US' \
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
  forbid_text "$path" '--application-version'
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
for required in 'latest' 'release-manifest.json' 'concrete' 'current directory' './runtime' 'saved runtime' 'self-update' 'upgrade' 'mikiko-gallery-studio-db-migrate'; do
  require_text "$ROOT/docs/runbooks/backend-deployment.md" "$required"
done
forbid_text "$ROOT/docs/runbooks/backend-deployment.md" '--application-version'

for required in 'mgsctl' 'release-manifest.json' 'docker.io/fatballfish/mikiko-gallery-studio-api' 'mgsctl upgrade'; do
  require_text "$ROOT/docs/deploy/backend-runbook.md" "$required"
done
forbid_text "$ROOT/docs/deploy/backend-runbook.md" 'PIC_GALLERY_IMAGE_TAG'

for required in 'DOCKERHUB_USERNAME' 'DOCKERHUB_TOKEN' 'release-manifest.json' 'mikiko-gallery-studio-api-linux-amd64.tar.gz' 'mikiko-gallery-studio-user-web.tar.gz' 'latest'; do
  require_text "$ROOT/deployments/devops/README.md" "$required"
done

for required in '--contract-only' 'mktemp -d' 'runtime/' 'MGSCTL_RELEASE_BASE_URL' 'DOCKER_CONFIG' 'mgsctl upgrade' 'mikiko-gallery-studio-db-migrate' '/readyz' 'docker compose' 'outside-runtime'; do
  require_text "$UPGRADE_E2E" "$required"
done
forbid_text "$UPGRADE_E2E" '--application-version'

echo "OK: bilingual mgsctl-only deployment documentation verified"

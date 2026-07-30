#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
WORKFLOW="$ROOT/.github/workflows/release.yml"
PACKAGER="$ROOT/scripts/devops/package-mgsctl.sh"
MAINTAINER_DOC="$ROOT/deployments/devops/README.md"

require_file() {
  [[ -f "$1" ]] || {
    echo "release contract is missing ${1#"$ROOT/"}" >&2
    exit 1
  }
}

require_text() {
  local file=$1
  local text=$2
  rg -Fq -- "$text" "$file" || {
    echo "${file#"$ROOT/"} is missing release contract text: $text" >&2
    exit 1
  }
}

forbid_text() {
  local file=$1
  local text=$2
  if rg -Fq -- "$text" "$file"; then
    echo "${file#"$ROOT/"} contains forbidden release behavior: $text" >&2
    exit 1
  fi
}

require_file "$WORKFLOW"
require_file "$PACKAGER"
require_file "$MAINTAINER_DOC"

for required in \
  "tags:" \
  "'v*'" \
  "workflow_dispatch:" \
  "startsWith(github.ref, 'refs/tags/v')" \
  "contents: read" \
  "contents: write" \
  "needs: verify" \
  "go test ./internal/mgsctl" \
  "./scripts/test/install-wrapper-contract.sh" \
  "os: [linux, darwin, windows]" \
  "arch: [amd64, arm64]" \
  "os: [linux, windows]" \
  "scripts/devops/package-mgsctl.sh" \
  "scripts/devops/package.sh native" \
  "actions/upload-artifact@v4" \
  "actions/download-artifact@v4" \
  "gh release view" \
  "gh release create" \
  "gh release upload" \
  "GH_REPO:" \
  ".sha256"; do
  require_text "$WORKFLOW" "$required"
done
forbid_text "$WORKFLOW" "--clobber"
forbid_text "$MAINTAINER_DOC" "scripts/local/pgctl.sh"
forbid_text "$MAINTAINER_DOC" "scripts/service/manage"

for required in \
  "RELEASE_GOOS" \
  "RELEASE_GOARCH" \
  "RELEASE_VERSION" \
  "MGSCTL_OUTPUT" \
  "MGSCTL_VERSION" \
  "MGSCTL_COMMIT" \
  "MGSCTL_BUILD_TIME" \
  "MGSCTL_DIRTY=false" \
  "mgsctl-\${target_os}-\${target_arch}" \
  "sha256sum" \
  "shasum -a 256"; do
  require_text "$PACKAGER" "$required"
done

echo "OK: tagged mgsctl and native release contract verified"

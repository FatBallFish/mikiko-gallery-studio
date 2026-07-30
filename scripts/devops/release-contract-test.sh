#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
WORKFLOW="$ROOT/.github/workflows/release.yml"
PACKAGER="$ROOT/scripts/devops/package-mgsctl.sh"
MANIFEST_RENDERER="$ROOT/scripts/devops/render-release-manifest.sh"
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
require_file "$MANIFEST_RENDERER"
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

for required in \
  "RELEASE_VERSION" \
  "RELEASE_COMMIT" \
  "RELEASE_ASSET_DIR" \
  "RELEASE_IMAGE_METADATA" \
  "release-manifest.json" \
  "schema_version" \
  "application_version" \
  "sha256:"; do
  require_text "$MANIFEST_RENDERER" "$required"
done

manifest_fixture=$(mktemp -d)
trap 'rm -rf "$manifest_fixture"' EXIT
printf 'mgsctl fixture\n' > "$manifest_fixture/mgsctl-linux-amd64"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$manifest_fixture" && sha256sum mgsctl-linux-amd64 > mgsctl-linux-amd64.sha256)
else
  (cd "$manifest_fixture" && shasum -a 256 mgsctl-linux-amd64 > mgsctl-linux-amd64.sha256)
fi
cat > "$manifest_fixture/images.json" <<'JSON'
[
  {"component":"api","repository":"docker.io/fatballfish/mikiko-gallery-studio-api","tag":"v1.2.3","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","version":"v1.2.3","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  {"component":"worker","repository":"docker.io/fatballfish/mikiko-gallery-studio-worker","tag":"v1.2.3","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","version":"v1.2.3","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  {"component":"user-web","repository":"docker.io/fatballfish/mikiko-gallery-studio-user-web","tag":"v1.2.3","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","version":"v1.2.3","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  {"component":"admin-web","repository":"docker.io/fatballfish/mikiko-gallery-studio-admin-web","tag":"v1.2.3","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444","version":"v1.2.3","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  {"component":"docs-web","repository":"docker.io/fatballfish/mikiko-gallery-studio-docs-web","tag":"v1.2.3","digest":"sha256:5555555555555555555555555555555555555555555555555555555555555555","version":"v1.2.3","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
]
JSON
RELEASE_VERSION=v1.2.3 \
RELEASE_COMMIT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
RELEASE_ASSET_DIR="$manifest_fixture" \
RELEASE_IMAGE_METADATA="$manifest_fixture/images.json" \
RELEASE_MANIFEST_OUTPUT="$manifest_fixture/release-manifest.json" \
  "$MANIFEST_RENDERER"
python3 - "$manifest_fixture/release-manifest.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    manifest = json.load(source)
assert manifest["schema_version"] == 1
assert manifest["application_version"] == "v1.2.3"
assert sorted(manifest["images"]) == ["admin-web", "api", "docs-web", "user-web", "worker"]
assert manifest["assets"]["mgsctl-linux-amd64"]["name"] == "mgsctl-linux-amd64"
PY
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

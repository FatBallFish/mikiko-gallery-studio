#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
WORKFLOW="$ROOT/.github/workflows/release.yml"
PACKAGER="$ROOT/scripts/devops/package-mgsctl.sh"
MANIFEST_RENDERER="$ROOT/scripts/devops/render-release-manifest.sh"
MAINTAINER_DOC="$ROOT/deployments/devops/README.md"
IMAGE_SCRIPT="$ROOT/scripts/docker/images.sh"
NATIVE_PACKAGER="$ROOT/scripts/devops/package.sh"
NATIVE_CONTRACT="$ROOT/scripts/workflow/native-package-contract.sh"
PROD_COMPOSE="$ROOT/deployments/docker-compose/docker-compose.prod.yml"
APP_PACKAGER="$ROOT/scripts/devops/package.sh"
ADMIN_DOCKERFILE="$ROOT/Dockerfile.admin-web"
RELEASE_NOTES_TEMPLATE="$ROOT/.github/release-notes-template.md"
RELEASE_NOTES_RENDERER="$ROOT/scripts/devops/render-release-notes.sh"
RELEASE_NOTES_CONTRACT="$ROOT/scripts/devops/release-notes-contract-test.sh"

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
require_file "$IMAGE_SCRIPT"
require_file "$NATIVE_PACKAGER"
require_file "$NATIVE_CONTRACT"
require_file "$PROD_COMPOSE"
require_file "$ADMIN_DOCKERFILE"
require_file "$RELEASE_NOTES_TEMPLATE"
require_file "$RELEASE_NOTES_RENDERER"
require_file "$RELEASE_NOTES_CONTRACT"

require_text "$ADMIN_DOCKERFILE" 'FROM --platform=$BUILDPLATFORM node:${NODE_VERSION} AS build'
"$RELEASE_NOTES_CONTRACT"

for heading in \
  "## 项目简介" \
  "## Feature 更新" \
  "## Bugfix" \
  "## 优化项" \
  "## 快速部署教程" \
  "## 快速升级教程"; do
  require_text "$RELEASE_NOTES_TEMPLATE" "$heading"
done

for image in api worker user-web admin-web docs-web; do
  require_text "$IMAGE_SCRIPT" "mikiko-gallery-studio-$image"
  require_text "$PROD_COMPOSE" "mikiko-gallery-studio-$image"
done
require_text "$IMAGE_SCRIPT" 'REGISTRY=${IMAGE_REGISTRY:-docker.io/fatballfish}'
for required in \
  'org.opencontainers.image.version' \
  'org.opencontainers.image.revision' \
  'org.opencontainers.image.source' \
  '--metadata-file' \
  'linux/amd64,linux/arm64' \
  'dirty-' ; do
  require_text "$IMAGE_SCRIPT" "$required"
done
forbid_text "$IMAGE_SCRIPT" 'docker tag'
for binary in api worker gateway db-migrate; do
  require_text "$NATIVE_PACKAGER" "mikiko-gallery-studio-$binary"
  require_text "$NATIVE_CONTRACT" "mikiko-gallery-studio-$binary"
done
require_text "$NATIVE_PACKAGER" 'mikiko-gallery-studio-native-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz'
require_text "$NATIVE_CONTRACT" 'mikiko-gallery-studio-native-${target_os}-amd64.tar.gz'

for current_surface in \
  "$ROOT/Dockerfile.api" \
  "$ROOT/Dockerfile.worker" \
  "$ROOT/deployments/docker-compose/docker-compose.local.yml" \
  "$ROOT/deployments/docker-compose/docker-compose.prod.yml" \
  "$ROOT/deployments/devops/run-api-server.sh" \
  "$ROOT/deployments/devops/run-worker.sh" \
  "$ROOT/scripts/docker/images.sh" \
  "$ROOT/scripts/devops/package.sh" \
  "$ROOT/scripts/workflow/native-package-contract.sh"; do
  for retired_component in api worker gateway service-host native; do
    forbid_text "$current_surface" "pic-gallery-${retired_component}"
  done
  forbid_text "$current_surface" "pic-gallery-local-bootstrap"
  forbid_text "$current_surface" "pic-gallery-local-entrypoint"
done

for required in \
  "tags:" \
  "'v*'" \
  "workflow_dispatch:" \
  "startsWith(github.ref, 'refs/tags/v')" \
  "contents: read" \
  "contents: write" \
  "needs: verify" \
  "Install verification tools" \
  "sudo apt-get install --yes ripgrep" \
  "./scripts/workflow/verify.sh" \
  "go test ./internal/mgsctl" \
  "./scripts/test/install-wrapper-contract.sh" \
  "os: [linux, darwin, windows]" \
  "arch: [amd64, arm64]" \
  "os: [linux, windows]" \
  "scripts/devops/package-mgsctl.sh" \
  "scripts/devops/package.sh native" \
  "actions/upload-artifact@v4" \
  "actions/download-artifact@v4" \
  "docker/setup-buildx-action@v3" \
  "docker/login-action@v3" \
  "docker/build-push-action@v6" \
  "DOCKERHUB_USERNAME" \
  "DOCKERHUB_TOKEN" \
  "platforms: linux/amd64,linux/arm64" \
  "scripts/devops/render-release-manifest.sh" \
  "release-manifest.json" \
  "imagetools create" \
  "needs: [release" \
  "gh release view" \
  "gh release create" \
  "gh release upload" \
  "fetch-depth: 0" \
  "Render release notes" \
  "./scripts/devops/render-release-notes.sh" \
  "RELEASE_NOTES_TAG:" \
  "RELEASE_NOTES_REPOSITORY:" \
  "--notes-file" \
  "gh release edit" \
  "GH_REPO:" \
  ".sha256"; do
  require_text "$WORKFLOW" "$required"
done

for image in api worker user-web admin-web docs-web; do
  require_text "$WORKFLOW" "mikiko-gallery-studio-$image"
done
for package_target in api-release worker-release user-web-release admin-web-release docs-web-release; do
  require_text "$WORKFLOW" "scripts/devops/package.sh $package_target"
done
for asset_name in \
  'mikiko-gallery-studio-api-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz' \
  'mikiko-gallery-studio-worker-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz' \
  'mikiko-gallery-studio-user-web.tar.gz' \
  'mikiko-gallery-studio-admin-web.tar.gz' \
  'mikiko-gallery-studio-docs-web.tar.gz'; do
  require_text "$APP_PACKAGER" "$asset_name"
done
require_text "$APP_PACKAGER" 'checksum_file'

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
forbid_text "$WORKFLOW" "docker tag"
forbid_text "$WORKFLOW" "--generate-notes"
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

echo "OK: complete tagged application release contract verified"

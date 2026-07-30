#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
target_root=${RELEASE_TARGET_ROOT:-"$ROOT/target/release"}
target_os=${RELEASE_GOOS:-$(go env GOOS)}
target_arch=${RELEASE_GOARCH:-$(go env GOARCH)}
version=${RELEASE_VERSION:-$(git -C "$ROOT" describe --tags --exact-match 2>/dev/null || echo dev)}
commit=${RELEASE_COMMIT:-$(git -C "$ROOT" rev-parse --verify HEAD 2>/dev/null || echo unknown)}
build_time=${RELEASE_BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}

case "$target_os" in
  linux|darwin|windows) ;;
  *) echo "unsupported mgsctl release OS: $target_os" >&2; exit 2 ;;
esac
case "$target_arch" in
  amd64|arm64) ;;
  *) echo "unsupported mgsctl release architecture: $target_arch" >&2; exit 2 ;;
esac

extension=""
[[ "$target_os" == windows ]] && extension=".exe"
artifact="mgsctl-${target_os}-${target_arch}${extension}"
mkdir -p "$target_root"

GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 \
  make -C "$ROOT" mgsctl \
    "MGSCTL_OUTPUT=$target_root/$artifact" \
    "MGSCTL_VERSION=$version" \
    "MGSCTL_COMMIT=$commit" \
    "MGSCTL_BUILD_TIME=$build_time" \
    "MGSCTL_DIRTY=false"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$target_root" && sha256sum "$artifact" > "$artifact.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "$target_root" && shasum -a 256 "$artifact" > "$artifact.sha256")
else
  echo "sha256 tool is required to package mgsctl" >&2
  exit 1
fi

echo "Packaged $artifact and $artifact.sha256 in $target_root"

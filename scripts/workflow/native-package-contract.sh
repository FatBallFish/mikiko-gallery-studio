#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
OUTPUT_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/pic-gallery-native-package.XXXXXX")"
trap 'rm -rf "$OUTPUT_ROOT"' EXIT INT TERM

assert_archive_entry() {
  local listing=$1 entry=$2
  rg -Fxq "$entry" "$listing" || {
    echo "native package is missing $entry" >&2
    return 1
  }
}

for target_os in linux windows; do
  target_root="$OUTPUT_ROOT/$target_os"
  DEVOPS_TARGET_ROOT="$target_root" DEVOPS_GOOS="$target_os" DEVOPS_GOARCH=amd64 DEVOPS_CGO_ENABLED=0 \
    "$ROOT/scripts/devops/package.sh" native

  archive="$target_root/pic-gallery-native-${target_os}-amd64.tar.gz"
  test -s "$archive"
  test -s "$archive.sha256"
  listing="$OUTPUT_ROOT/${target_os}.listing"
  tar -tzf "$archive" >"$listing"

  extension=""
  [[ "$target_os" == windows ]] && extension=".exe"
  for entry in \
    "bin/pic-gallery-api${extension}" \
    "bin/pic-gallery-worker${extension}" \
    "bin/pic-gallery-gateway${extension}" \
	"bin/mikiko-gallery-studio-db-migrate${extension}" \
    "web/user/index.html" \
    "web/admin/index.html" \
    "web/docs/index.html" \
    "api/openapi/openapi.yaml"; do
    assert_archive_entry "$listing" "$entry"
  done
  if [[ "$target_os" == windows ]]; then
    assert_archive_entry "$listing" "bin/pic-gallery-service-host.exe"
  fi
done

echo "OK: Linux and Windows native package contract passed"

#!/usr/bin/env sh
set -eu

if [ -n "${DEPLOYCTL_BIN:-}" ]; then
  exec "$DEPLOYCTL_BIN" "$@"
fi

if command -v deployctl >/dev/null 2>&1; then
  exec deployctl "$@"
fi

version="${DEPLOYCTL_VERSION:-latest}"
release_base="${DEPLOYCTL_RELEASE_BASE_URL:-https://github.com/fatballfish/pic-gallery/releases}"
case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "unsupported operating system; set DEPLOYCTL_BIN" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture; set DEPLOYCTL_BIN" >&2; exit 1 ;;
esac

if [ "$version" = "latest" ]; then
  release_path="latest/download"
else
  release_path="download/$version"
fi
artifact="deployctl-$os-$arch"
url="${DEPLOYCTL_DOWNLOAD_URL:-$release_base/$release_path/$artifact}"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
binary="$temporary_dir/$artifact"
checksum_file="$temporary_dir/$artifact.sha256"

curl --fail --location --silent --show-error "$url" --output "$binary"
if [ -n "${DEPLOYCTL_SHA256:-}" ]; then
  expected_sha256="$DEPLOYCTL_SHA256"
else
  curl --fail --location --silent --show-error "$url.sha256" --output "$checksum_file"
  expected_sha256="$(awk '{print $1; exit}' "$checksum_file")"
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_sha256="$(sha256sum "$binary" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_sha256="$(shasum -a 256 "$binary" | awk '{print $1}')"
else
  echo "sha256 verification tool was not found" >&2
  exit 1
fi
if [ "$actual_sha256" != "$expected_sha256" ]; then
  echo "deployctl checksum verification failed" >&2
  exit 1
fi
chmod 0700 "$binary"
set +e
"$binary" "$@"
status=$?
set -e
exit "$status"

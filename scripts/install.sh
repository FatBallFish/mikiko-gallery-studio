#!/usr/bin/env sh
set -eu

if [ -n "${DEPLOYCTL_BIN:-}" ]; then
  exec "$DEPLOYCTL_BIN" "$@"
fi

if command -v deployctl >/dev/null 2>&1; then
  exec deployctl "$@"
fi

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
root_dir="$(CDPATH= cd -- "$script_dir/.." && pwd)"
version="${DEPLOYCTL_VERSION:-latest}"
release_base="${DEPLOYCTL_RELEASE_BASE_URL:-https://github.com/fatballfish/pic-gallery/releases}"
install_dir="${DEPLOYCTL_INSTALL_DIR:-${HOME:?HOME is required}/.local/bin}"
go_command="${GO:-go}"
make_command="${MAKE:-make}"

case "$install_dir" in
  /*) ;;
  *) install_dir="$(pwd)/$install_dir" ;;
esac

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
downloaded_binary="$temporary_dir/$artifact"
checksum_file="$temporary_dir/$artifact.sha256"

calculate_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  return 1
}

download_release() {
  if ! curl --fail --location --silent --show-error --output "$downloaded_binary" "$url"; then
    echo "deployctl release download was unavailable: $url" >&2
    return 10
  fi

  explicit_checksum=false
  if [ -n "${DEPLOYCTL_SHA256:-}" ]; then
    expected_sha256="$DEPLOYCTL_SHA256"
    explicit_checksum=true
  else
    if ! curl --fail --location --silent --show-error --output "$checksum_file" "$url.sha256"; then
      echo "deployctl release checksum was unavailable: $url.sha256" >&2
      return 10
    fi
    expected_sha256="$(awk '{print $1; exit}' "$checksum_file")"
  fi

  case "$expected_sha256" in
    ''|*[!0-9a-fA-F]*)
      if [ "$explicit_checksum" = true ]; then
        echo "DEPLOYCTL_SHA256 must contain exactly 64 hexadecimal characters" >&2
        return 20
      fi
      echo "deployctl release checksum file is incomplete" >&2
      return 10
      ;;
  esac
  if [ "${#expected_sha256}" -ne 64 ]; then
    if [ "$explicit_checksum" = true ]; then
      echo "DEPLOYCTL_SHA256 must contain exactly 64 hexadecimal characters" >&2
      return 20
    fi
    echo "deployctl release checksum file is incomplete" >&2
    return 10
  fi

  if ! actual_sha256="$(calculate_sha256 "$downloaded_binary")"; then
    echo "SHA-256 verification tool was not found" >&2
    return 10
  fi
  if [ "$(printf '%s' "$actual_sha256" | tr 'A-F' 'a-f')" != "$(printf '%s' "$expected_sha256" | tr 'A-F' 'a-f')" ]; then
    echo "deployctl checksum verification failed; refusing local build fallback" >&2
    return 20
  fi
  return 0
}

build_from_source() {
  missing=""
  for relative_path in go.mod Makefile cmd/deployctl; do
    if [ ! -e "$root_dir/$relative_path" ]; then
      if [ -n "$missing" ]; then
        missing="$missing, $relative_path"
      else
        missing="$relative_path"
      fi
    fi
  done
  if [ -n "$missing" ]; then
    echo "local deployctl build requires a complete source checkout; missing: $missing" >&2
    echo "provide a trusted prebuilt binary with DEPLOYCTL_BIN" >&2
    return 1
  fi
  if ! command -v "$go_command" >/dev/null 2>&1; then
    echo "local deployctl build requires Go ($go_command was not found); install Go or set DEPLOYCTL_BIN" >&2
    return 1
  fi
  if ! command -v "$make_command" >/dev/null 2>&1; then
    echo "local deployctl build requires Make ($make_command was not found); install Make or set DEPLOYCTL_BIN" >&2
    return 1
  fi

  local_binary="$temporary_dir/deployctl-local"
  if ! "$make_command" -C "$root_dir" deployctl "DEPLOYCTL_OUTPUT=$local_binary" "GO=$go_command" >&2; then
    echo "local deployctl build failed; install Go and Make or set DEPLOYCTL_BIN" >&2
    return 1
  fi
  if [ ! -f "$local_binary" ]; then
    echo "local deployctl build did not produce $local_binary" >&2
    return 1
  fi
  printf '%s\n' "$local_binary"
}

install_candidate() {
  candidate=$1
  if ! mkdir -p "$install_dir"; then
    echo "cannot create deployctl install directory: $install_dir" >&2
    return 1
  fi
  staged="$install_dir/.deployctl.install.$$"
  rm -f "$staged"
  if ! cp "$candidate" "$staged"; then
    echo "cannot stage deployctl in install directory: $install_dir" >&2
    return 1
  fi
  if ! chmod 0755 "$staged"; then
    rm -f "$staged"
    echo "cannot mark staged deployctl executable" >&2
    return 1
  fi
  if ! mv -f "$staged" "$install_dir/deployctl"; then
    rm -f "$staged"
    echo "cannot replace deployctl in install directory: $install_dir" >&2
    return 1
  fi
  printf '%s\n' "$install_dir/deployctl"
}

download_status=0
download_release || download_status=$?
case "$download_status" in
  0) candidate="$downloaded_binary" ;;
  10)
    echo "Release artifact could not be verified; falling back to a local source build." >&2
    candidate="$(build_from_source)" || exit 1
    ;;
  *) exit "$download_status" ;;
esac

installed_binary="$(install_candidate "$candidate")" || exit 1
echo "Installed deployctl: $installed_binary"
case ":${PATH:-}:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to PATH to run deployctl directly in future shells." ;;
esac

exec "$installed_binary" "$@"

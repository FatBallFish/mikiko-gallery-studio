#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
root_dir="$(CDPATH= cd -- "$script_dir/.." && pwd)"
source_ready=true
for relative_path in go.mod Makefile cmd/mgsctl Dockerfile.api Dockerfile.worker Dockerfile.user-web Dockerfile.admin-web Dockerfile.docs-web; do
  if [ ! -e "$root_dir/$relative_path" ]; then
    source_ready=false
    break
  fi
done
if [ -z "${MGSCTL_SOURCE_DIR:-}" ] && [ "$source_ready" = true ]; then
    MGSCTL_SOURCE_DIR="$root_dir"
    export MGSCTL_SOURCE_DIR
fi

if [ -n "${MGSCTL_BIN:-}" ]; then
  exec "$MGSCTL_BIN" "$@"
fi

version="${MGSCTL_VERSION:-latest}"
release_base="${MGSCTL_RELEASE_BASE_URL:-https://github.com/fatballfish/mikiko-gallery-studio/releases}"
install_dir="${MGSCTL_INSTALL_DIR:-${HOME:?HOME is required}/.local/bin}"
go_command="${GO:-go}"
make_command="${MAKE:-make}"

case "$install_dir" in
  /*) ;;
  *) install_dir="$(pwd)/$install_dir" ;;
esac

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "unsupported operating system; set MGSCTL_BIN" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture; set MGSCTL_BIN" >&2; exit 1 ;;
esac

if [ "$version" = "latest" ]; then
  release_path="latest/download"
else
  release_path="download/$version"
fi
artifact="mgsctl-$os-$arch"
url="${MGSCTL_DOWNLOAD_URL:-$release_base/$release_path/$artifact}"
display_url="${url%%\?*}"

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
    echo "mgsctl release download was unavailable: $display_url" >&2
    return 10
  fi

  explicit_checksum=false
  if [ -n "${MGSCTL_SHA256:-}" ]; then
    expected_sha256="$MGSCTL_SHA256"
    explicit_checksum=true
  else
    if ! curl --fail --location --silent --show-error --output "$checksum_file" "$url.sha256"; then
      echo "mgsctl release checksum was unavailable: $display_url.sha256" >&2
      return 10
    fi
    expected_sha256="$(awk '{print $1; exit}' "$checksum_file")"
  fi

  case "$expected_sha256" in
    ''|*[!0-9a-fA-F]*)
      if [ "$explicit_checksum" = true ]; then
        echo "MGSCTL_SHA256 must contain exactly 64 hexadecimal characters" >&2
        return 20
      fi
      echo "mgsctl release checksum file is incomplete" >&2
      return 10
      ;;
  esac
  if [ "${#expected_sha256}" -ne 64 ]; then
    if [ "$explicit_checksum" = true ]; then
      echo "MGSCTL_SHA256 must contain exactly 64 hexadecimal characters" >&2
      return 20
    fi
    echo "mgsctl release checksum file is incomplete" >&2
    return 10
  fi

  if ! actual_sha256="$(calculate_sha256 "$downloaded_binary")"; then
    echo "SHA-256 verification tool was not found" >&2
    return 10
  fi
  if [ "$(printf '%s' "$actual_sha256" | tr 'A-F' 'a-f')" != "$(printf '%s' "$expected_sha256" | tr 'A-F' 'a-f')" ]; then
    echo "mgsctl checksum verification failed; refusing local build fallback" >&2
    return 20
  fi
  return 0
}

build_from_source() {
  missing=""
  for relative_path in go.mod Makefile cmd/mgsctl; do
    if [ ! -e "$root_dir/$relative_path" ]; then
      if [ -n "$missing" ]; then
        missing="$missing, $relative_path"
      else
        missing="$relative_path"
      fi
    fi
  done
  if [ -n "$missing" ]; then
    echo "local mgsctl build requires a complete source checkout; missing: $missing" >&2
    echo "provide a trusted prebuilt binary with MGSCTL_BIN" >&2
    return 1
  fi
  if ! command -v "$go_command" >/dev/null 2>&1; then
    echo "local mgsctl build requires Go ($go_command was not found); install Go or set MGSCTL_BIN" >&2
    return 1
  fi
  if ! command -v "$make_command" >/dev/null 2>&1; then
    echo "local mgsctl build requires Make ($make_command was not found); install Make or set MGSCTL_BIN" >&2
    return 1
  fi

  local_binary="$temporary_dir/mgsctl-local"
  if ! "$make_command" -C "$root_dir" mgsctl "MGSCTL_OUTPUT=$local_binary" "GO=$go_command" >&2; then
    echo "local mgsctl build failed; install Go and Make or set MGSCTL_BIN" >&2
    return 1
  fi
  if [ ! -f "$local_binary" ]; then
    echo "local mgsctl build did not produce $local_binary" >&2
    return 1
  fi
  printf '%s\n' "$local_binary"
}

install_candidate() {
  candidate=$1
  if ! mkdir -p "$install_dir"; then
    echo "cannot create mgsctl install directory: $install_dir" >&2
    return 1
  fi
  staged="$install_dir/.mgsctl.install.$$"
  rm -f "$staged"
  if ! cp "$candidate" "$staged"; then
    echo "cannot stage mgsctl in install directory: $install_dir" >&2
    return 1
  fi
  if ! chmod 0755 "$staged"; then
    rm -f "$staged"
    echo "cannot mark staged mgsctl executable" >&2
    return 1
  fi
  if ! mv -f "$staged" "$install_dir/mgsctl"; then
    rm -f "$staged"
    echo "cannot replace mgsctl in install directory: $install_dir" >&2
    return 1
  fi
  printf '%s\n' "$install_dir/mgsctl"
}

force_local_build=false
if path_binary="$(command -v mgsctl 2>/dev/null)"; then
  source_commit=""
  source_dirty=false
  source_git_prefix=""
  if [ "$source_ready" = true ] && command -v git >/dev/null 2>&1; then
    source_git_prefix="$(git -C "$root_dir" rev-parse --show-prefix 2>/dev/null || true)"
    source_commit="$(git -C "$root_dir" rev-parse HEAD 2>/dev/null || true)"
    if [ -z "$source_git_prefix" ] && [ -n "$source_commit" ]; then
      if [ -n "$(git -C "$root_dir" status --porcelain --untracked-files=normal 2>/dev/null)" ]; then
        source_dirty=true
      fi

      if [ "$source_dirty" = true ]; then
        echo "Source checkout has uncommitted changes; selecting a local source build instead of the PATH mgsctl." >&2
        force_local_build=true
      else
        if path_metadata="$("$path_binary" version --json 2>/dev/null)"; then
          path_commit="$(printf '%s\n' "$path_metadata" | sed -n 's/.*"commit"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
        else
          path_commit=""
        fi
        if [ "$path_commit" = "$source_commit" ]; then
          exec "$path_binary" "$@"
        fi
        if [ -n "$path_commit" ]; then
          echo "PATH mgsctl is stale for this source checkout (tool commit $path_commit, checkout $source_commit); selecting a local source build." >&2
        else
          echo "PATH mgsctl build metadata is unavailable; selecting a local source build for this checkout." >&2
        fi
        force_local_build=true
      fi
    fi
  fi

  if [ "$force_local_build" = false ]; then
    exec "$path_binary" "$@"
  fi
fi

temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
downloaded_binary="$temporary_dir/$artifact"
checksum_file="$temporary_dir/$artifact.sha256"

if [ "$force_local_build" = true ]; then
  candidate="$(build_from_source)" || exit 1
else
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
fi

installed_binary="$(install_candidate "$candidate")" || exit 1
echo "Installed mgsctl: $installed_binary"
case ":${PATH:-}:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to PATH to run mgsctl directly in future shells." ;;
esac

exec "$installed_binary" "$@"

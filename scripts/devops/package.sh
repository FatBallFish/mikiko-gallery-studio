#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TARGET_ROOT=${DEVOPS_TARGET_ROOT:-"$ROOT_DIR/target/devops"}
GOOS_TARGET=${DEVOPS_GOOS:-linux}
GOARCH_TARGET=${DEVOPS_GOARCH:-amd64}
CGO_TARGET=${DEVOPS_CGO_ENABLED:-0}

usage() {
  cat <<'USAGE'
Usage: scripts/devops/package.sh <user-web|admin-web|docs-web|api-server|worker|gateway|native|all>
       scripts/devops/package.sh <user-web-release|admin-web-release|docs-web-release|api-release|worker-release>

Environment overrides:
  DEVOPS_TARGET_ROOT   Output root, default target/devops
  DEVOPS_GOOS          Backend GOOS, default linux
  DEVOPS_GOARCH        Backend GOARCH, default amd64
  DEVOPS_CGO_ENABLED   Backend CGO_ENABLED, default 0
  APP_ENV_FILE         Optional runtime env path used by generated service scripts,
                       default ./config/runtime.env beside the backend package.
USAGE
}

copy_file() {
  local src=$1
  local dst=$2
  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
}

checksum_file() {
  local path=$1
  local directory name
  directory=$(dirname "$path")
  name=$(basename "$path")
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$directory" && sha256sum "$name" > "$name.sha256")
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$directory" && shasum -a 256 "$name" > "$name.sha256")
  else
    echo "sha256 tool is required to package a release" >&2
    exit 1
  fi
}

archive_directory() {
  local directory=$1
  local archive=$2
  rm -f "$archive" "$archive.sha256"
  COPYFILE_DISABLE=1 tar --format=ustar -C "$directory" -czf "$archive" .
  checksum_file "$archive"
}

package_frontend() {
  local app=$1
  local web_dir="$ROOT_DIR/web/$app"
  local out_dir="$TARGET_ROOT/$app-web"
  local base_path="/"
  if [[ "$app" == "admin" ]]; then
    base_path="/admin/"
  elif [[ "$app" == "docs" ]]; then
    base_path="/developer-docs/"
  fi

  echo "==> Building $app-web"
  rm -rf "$out_dir"
  (cd "$web_dir" && VITE_BASE_PATH="$base_path" VITE_API_BASE_URL="" npm run build)

  mkdir -p "$out_dir"
  cp -R "$web_dir/dist" "$out_dir/dist"
  copy_file "$ROOT_DIR/deployments/devops/nginx-$app-web.conf" "$out_dir/nginx.conf"
  if [[ "$app" != "docs" ]]; then
    mkdir -p "$out_dir/env"
    copy_file "$ROOT_DIR/deployments/devops/env/frontend.env.example" "$out_dir/env/frontend.env.example"
    copy_file "$ROOT_DIR/deployments/devops/frontend-env.template.js" "$out_dir/env.template.js"
    copy_file "$ROOT_DIR/deployments/devops/start-$app-web.sh" "$out_dir/start-$app-web.sh"
    chmod +x "$out_dir/start-$app-web.sh"
  fi
}

package_backend() {
  local target=$1
  local cmd_pkg
  local bin_name
  local run_script

  case "$target" in
    api-server)
      cmd_pkg="./cmd/api"
      bin_name="mikiko-gallery-studio-api"
      run_script="run-api-server.sh"
      ;;
    worker)
      cmd_pkg="./cmd/worker"
      bin_name="mikiko-gallery-studio-worker"
      run_script="run-worker.sh"
      ;;
    gateway)
      cmd_pkg="./cmd/gateway"
      bin_name="mikiko-gallery-studio-gateway"
      run_script=""
      ;;
    *)
      echo "unknown backend target: $target" >&2
      exit 2
      ;;
  esac

  local out_dir="$TARGET_ROOT/$target"
  echo "==> Building $target ($GOOS_TARGET/$GOARCH_TARGET, CGO_ENABLED=$CGO_TARGET)"
  rm -rf "$out_dir"
  mkdir -p "$out_dir/bin"

  (
    cd "$ROOT_DIR"
    GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" \
      go build -trimpath -ldflags="-s -w" -o "$out_dir/bin/$bin_name" "$cmd_pkg"
  )

  if [[ -n "$run_script" ]]; then
    mkdir -p "$out_dir/config"
    copy_file "$ROOT_DIR/config/runtime.env.example" "$out_dir/config/runtime.env.example"
    copy_file "$ROOT_DIR/deployments/devops/$run_script" "$out_dir/$run_script"
    chmod +x "$out_dir/$run_script"
  fi

  if [[ "$target" == "api-server" ]]; then
    mkdir -p "$out_dir/api"
    mkdir -p "$out_dir/api/openapi"
    cp "$ROOT_DIR/api/openapi/openapi.yaml" "$out_dir/api/openapi/openapi.yaml"
    cp -R "$ROOT_DIR/api/openapi/components" "$out_dir/api/openapi/components"
  fi
}

package_frontend_release() {
  local app=$1
  local archive
  package_frontend "$app"
  case "$app" in
    user) archive="$TARGET_ROOT/mikiko-gallery-studio-user-web.tar.gz" ;;
    admin) archive="$TARGET_ROOT/mikiko-gallery-studio-admin-web.tar.gz" ;;
    docs) archive="$TARGET_ROOT/mikiko-gallery-studio-docs-web.tar.gz" ;;
    *) echo "unknown frontend release target: $app" >&2; exit 2 ;;
  esac
  archive_directory "$TARGET_ROOT/$app-web" "$archive"
}

package_backend_release() {
  local target=$1
  local archive
  package_backend "$target"
  case "$target" in
    api-server) archive="$TARGET_ROOT/mikiko-gallery-studio-api-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz" ;;
    worker) archive="$TARGET_ROOT/mikiko-gallery-studio-worker-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz" ;;
    *) echo "unknown backend release target: $target" >&2; exit 2 ;;
  esac
  archive_directory "$TARGET_ROOT/$target" "$archive"
}

package_native() {
  local bundle="$TARGET_ROOT/native-$GOOS_TARGET-$GOARCH_TARGET"
  local extension=""
  [[ "$GOOS_TARGET" == "windows" ]] && extension=".exe"
  local archive="$TARGET_ROOT/mikiko-gallery-studio-native-${GOOS_TARGET}-${GOARCH_TARGET}.tar.gz"

  echo "==> Building native bundle ($GOOS_TARGET/$GOARCH_TARGET, CGO_ENABLED=$CGO_TARGET)"
  rm -rf "$bundle"
  mkdir -p "$bundle/bin" "$bundle/web/user" "$bundle/web/admin" "$bundle/web/docs" "$bundle/api/openapi"

  for app in user admin docs; do
    local base_path="/"
    [[ "$app" == "admin" ]] && base_path="/admin/"
    [[ "$app" == "docs" ]] && base_path="/developer-docs/"
    (cd "$ROOT_DIR/web/$app" && VITE_BASE_PATH="$base_path" VITE_API_BASE_URL="" npm run build)
    cp -R "$ROOT_DIR/web/$app/dist/." "$bundle/web/$app/"
  done

  (
    cd "$ROOT_DIR"
    GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" go build -trimpath -ldflags="-s -w" -o "$bundle/bin/mikiko-gallery-studio-api$extension" ./cmd/api
    GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" go build -trimpath -ldflags="-s -w" -o "$bundle/bin/mikiko-gallery-studio-worker$extension" ./cmd/worker
    GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" go build -trimpath -ldflags="-s -w" -o "$bundle/bin/mikiko-gallery-studio-gateway$extension" ./cmd/gateway
	GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" go build -trimpath -ldflags="-s -w" -o "$bundle/bin/mikiko-gallery-studio-db-migrate$extension" ./cmd/db-migrate
	GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" go build -trimpath -ldflags="-s -w" -o "$bundle/bin/mikiko-gallery-studio-media-backfill$extension" ./cmd/media-backfill
    if [[ "$GOOS_TARGET" == "windows" ]]; then
      GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" go build -trimpath -ldflags="-s -w" -o "$bundle/bin/mikiko-gallery-studio-service-host.exe" ./cmd/servicehost
    fi
  )
  cp "$ROOT_DIR/api/openapi/openapi.yaml" "$bundle/api/openapi/openapi.yaml"
  cp -R "$ROOT_DIR/api/openapi/components" "$bundle/api/openapi/components"

  COPYFILE_DISABLE=1 tar --format=ustar -C "$bundle" -czf "$archive" bin web api
  checksum_file "$archive"
}

package_all() {
  package_frontend user
  package_frontend admin
  package_frontend docs
  package_backend api-server
  package_backend worker
  package_native
  mkdir -p "$TARGET_ROOT"
  copy_file "$ROOT_DIR/deployments/devops/middleware-compose.yml" "$TARGET_ROOT/middleware-compose.yml"
  copy_file "$ROOT_DIR/deployments/devops/README.md" "$TARGET_ROOT/README.md"
}

main() {
  local target=${1:-}
  if [[ -z "$target" || "$target" == "-h" || "$target" == "--help" ]]; then
    usage
    exit 0
  fi

  case "$target" in
    user-web)
      package_frontend user
      ;;
    user-web-release)
      package_frontend_release user
      ;;
    admin-web)
      package_frontend admin
      ;;
    admin-web-release)
      package_frontend_release admin
      ;;
    docs-web)
      package_frontend docs
      ;;
    docs-web-release)
      package_frontend_release docs
      ;;
    api-release)
      package_backend_release api-server
      ;;
    worker-release)
      package_backend_release worker
      ;;
    api-server|worker|gateway)
      package_backend "$target"
      ;;
    native)
      package_native
      ;;
    all)
      package_all
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac

  echo "==> Packaged $target under $TARGET_ROOT"
}

main "$@"

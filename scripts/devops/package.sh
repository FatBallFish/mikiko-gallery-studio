#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TARGET_ROOT=${DEVOPS_TARGET_ROOT:-"$ROOT_DIR/target/devops"}
GOOS_TARGET=${DEVOPS_GOOS:-linux}
GOARCH_TARGET=${DEVOPS_GOARCH:-amd64}
CGO_TARGET=${DEVOPS_CGO_ENABLED:-0}
APP_ENV_TARGET=${APP_ENV:-dev}

usage() {
  cat <<'USAGE'
Usage: scripts/devops/package.sh <user-web|admin-web|api-server|worker|all>

Environment overrides:
  DEVOPS_TARGET_ROOT   Output root, default target/devops
  DEVOPS_GOOS          Backend GOOS, default linux
  DEVOPS_GOARCH        Backend GOARCH, default amd64
  DEVOPS_CGO_ENABLED   Backend CGO_ENABLED, default 0
  APP_ENV              Backend config selector, default dev. Uses configs/config.<APP_ENV>.yaml.
                       prod and production are aliases for pro.
USAGE
}

copy_file() {
  local src=$1
  local dst=$2
  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
}

backend_config_env() {
  case "$APP_ENV_TARGET" in
    prod|production)
      echo "pro"
      ;;
    *)
      echo "$APP_ENV_TARGET"
      ;;
  esac
}

backend_config_path() {
  local config_env
  config_env=$(backend_config_env)
  local config_path="$ROOT_DIR/configs/config.$config_env.yaml"
  if [[ ! -f "$config_path" ]]; then
    echo "missing backend config for APP_ENV=$APP_ENV_TARGET: $config_path" >&2
    exit 2
  fi
  echo "$config_path"
}

package_frontend() {
  local app=$1
  local web_dir="$ROOT_DIR/web/$app"
  local out_dir="$TARGET_ROOT/$app-web"
  local base_path="/"
  if [[ "$app" == "admin" ]]; then
    base_path="/admin/"
  fi

  echo "==> Building $app-web"
  rm -rf "$out_dir"
  (cd "$web_dir" && VITE_BASE_PATH="$base_path" VITE_API_BASE_URL="" npm run build)

  mkdir -p "$out_dir"
  cp -R "$web_dir/dist" "$out_dir/dist"
  mkdir -p "$out_dir/env"
  copy_file "$ROOT_DIR/deployments/devops/env/frontend.env.example" "$out_dir/env/frontend.env.example"
  copy_file "$ROOT_DIR/deployments/devops/frontend-env.template.js" "$out_dir/env.template.js"
  copy_file "$ROOT_DIR/deployments/devops/nginx-$app-web.conf" "$out_dir/nginx.conf"
  copy_file "$ROOT_DIR/deployments/devops/start-$app-web.sh" "$out_dir/start-$app-web.sh"
  chmod +x "$out_dir/start-$app-web.sh"
}

package_backend() {
  local target=$1
  local cmd_pkg
  local bin_name
  local run_script

  case "$target" in
    api-server)
      cmd_pkg="./cmd/api"
      bin_name="pic-gallery-api"
      run_script="run-api-server.sh"
      ;;
    worker)
      cmd_pkg="./cmd/worker"
      bin_name="pic-gallery-worker"
      run_script="run-worker.sh"
      ;;
    *)
      echo "unknown backend target: $target" >&2
      exit 2
      ;;
  esac

  local out_dir="$TARGET_ROOT/$target"
  local config_path
  config_path=$(backend_config_path)
  echo "==> Building $target ($GOOS_TARGET/$GOARCH_TARGET, CGO_ENABLED=$CGO_TARGET, APP_ENV=$(backend_config_env))"
  rm -rf "$out_dir"
  mkdir -p "$out_dir/bin"

  (
    cd "$ROOT_DIR"
    GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED="$CGO_TARGET" \
      go build -trimpath -ldflags="-s -w" -o "$out_dir/bin/$bin_name" "$cmd_pkg"
  )

  copy_file "$config_path" "$out_dir/config.yaml"
  copy_file "$ROOT_DIR/deployments/devops/$run_script" "$out_dir/$run_script"
  chmod +x "$out_dir/$run_script"

  if [[ "$target" == "api-server" ]]; then
    mkdir -p "$out_dir/api"
    mkdir -p "$out_dir/api/openapi"
    cp "$ROOT_DIR/api/openapi/openapi.yaml" "$out_dir/api/openapi/openapi.yaml"
    cp -R "$ROOT_DIR/api/openapi/components" "$out_dir/api/openapi/components"
  fi
}

package_all() {
  package_frontend user
  package_frontend admin
  package_backend api-server
  package_backend worker
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
    admin-web)
      package_frontend admin
      ;;
    api-server|worker)
      package_backend "$target"
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

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPONENTS="api,worker,user-web,admin-web"
ENV_FILE="${PIC_GALLERY_ENV_FILE:-$ROOT_DIR/.env}"
BACKGROUND=false

usage() {
  cat <<'USAGE'
Usage: scripts/local/pgctl.sh <build|run|up|install|uninstall|start|stop|restart|status|logs> [options]

Options:
  --components LIST   Comma-separated components. Default: api,worker,user-web,admin-web
  --env-file PATH     Env file for api and worker. Default: .env
  --background        For run/up, start processes in the background.
  --user              Forward to service manager for user-level service install.

Modes:
  build               Build selected components only.
  run                 Run selected components without building.
  up                  Build then run selected components.
  install             Build and install api/worker as local services.
  uninstall/start/stop/restart/status/logs
                      Manage api/worker services.
USAGE
}

ACTION="${1:-}"
if [[ -z "$ACTION" || "$ACTION" == "-h" || "$ACTION" == "--help" ]]; then
  usage
  exit 0
fi
shift

SERVICE_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --components)
      COMPONENTS="${2:?missing components}"
      SERVICE_ARGS+=(--components "$COMPONENTS")
      shift 2
      ;;
    --env-file)
      ENV_FILE="${2:?missing env file}"
      SERVICE_ARGS+=(--env-file "$ENV_FILE")
      shift 2
      ;;
    --background)
      BACKGROUND=true
      shift
      ;;
    --user)
      SERVICE_ARGS+=(--user)
      shift
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

has_component() {
  local name=$1
  IFS=',' read -ra selected <<< "$COMPONENTS"
  for item in "${selected[@]}"; do
    [[ "$(echo "$item" | xargs)" == "$name" ]] && return 0
  done
  return 1
}

for_selected_component() {
  local fn=$1
  local component
  for component in api worker user-web admin-web; do
    if has_component "$component"; then
      "$fn" "$component"
    fi
  done
}

build_component() {
  local component=$1
  mkdir -p "$ROOT_DIR/target/local/bin"
  case "$component" in
    api) (cd "$ROOT_DIR" && go build -o target/local/bin/pic-gallery-api ./cmd/api) ;;
    worker) (cd "$ROOT_DIR" && go build -o target/local/bin/pic-gallery-worker ./cmd/worker) ;;
    user-web) (cd "$ROOT_DIR/web/user" && npm run build) ;;
    admin-web) (cd "$ROOT_DIR/web/admin" && npm run build) ;;
    *) echo "Unknown component: $component" >&2; exit 2 ;;
  esac
}

run_component() {
  local component=$1
  local cmd
  case "$component" in
    api) cmd="PIC_GALLERY_ENV_FILE='$ENV_FILE' '$ROOT_DIR/target/local/bin/pic-gallery-api'" ;;
    worker) cmd="PIC_GALLERY_ENV_FILE='$ENV_FILE' '$ROOT_DIR/target/local/bin/pic-gallery-worker'" ;;
    user-web) cmd="npm --prefix '$ROOT_DIR/web/user' run dev -- --host 0.0.0.0 --port ${USER_WEB_PORT:-5173}" ;;
    admin-web) cmd="npm --prefix '$ROOT_DIR/web/admin' run dev -- --host 0.0.0.0 --port ${ADMIN_WEB_PORT:-5174}" ;;
    *) echo "Unknown component: $component" >&2; exit 2 ;;
  esac
  if [[ "$BACKGROUND" == true ]]; then
    mkdir -p "$ROOT_DIR/tmp"
    nohup bash -lc "$cmd" >"$ROOT_DIR/tmp/$component.out.log" 2>"$ROOT_DIR/tmp/$component.err.log" &
    echo "$!" > "$ROOT_DIR/tmp/$component.pid"
    echo "started $component pid=$(cat "$ROOT_DIR/tmp/$component.pid")"
  else
    bash -lc "$cmd"
  fi
}

case "$ACTION" in
  build)
    for_selected_component build_component
    ;;
  run)
    for_selected_component run_component
    ;;
  up)
    for_selected_component build_component
    for_selected_component run_component
    ;;
  install|uninstall|start|stop|restart|status|logs)
    "$ROOT_DIR/scripts/service/manage.sh" "$ACTION" "${SERVICE_ARGS[@]}"
    ;;
  *)
    echo "Unknown action: $ACTION" >&2
    usage >&2
    exit 2
    ;;
esac

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
    api)
      (cd "$ROOT_DIR" && go build -o target/local/bin/pic-gallery-api ./cmd/api)
      write_service_start_script api
      ;;
    worker)
      (cd "$ROOT_DIR" && go build -o target/local/bin/pic-gallery-worker ./cmd/worker)
      write_service_start_script worker
      ;;
    user-web) (cd "$ROOT_DIR/web/user" && npm run build) ;;
    admin-web) (cd "$ROOT_DIR/web/admin" && npm run build) ;;
    *) echo "Unknown component: $component" >&2; exit 2 ;;
  esac
}

write_service_start_script() {
  local component=$1
  local script="$ROOT_DIR/target/local/bin/start-pic-gallery-$component.sh"
  cat > "$script" <<EOF
#!/usr/bin/env sh
set -eu

SCRIPT_DIR="\$(CDPATH= cd -- "\$(dirname -- "\$0")" && pwd)"
ROOT_DIR="\$(cd "\$SCRIPT_DIR/../../.." && pwd)"
COMPONENT="$component"
MANAGE_SH="\$ROOT_DIR/scripts/service/manage.sh"

service_is_registered() {
  case "\$(uname -s)" in
    Linux)
      unit="pic-gallery-\$COMPONENT.service"
      unit_path="/etc/systemd/system/\$unit"
      if [ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" != "1" ]; then
        unit_path="\$HOME/.config/systemd/user/\$unit"
        [ -f "\$unit_path" ] || systemctl --user list-unit-files "\$unit" --no-legend 2>/dev/null | grep -q "\$unit"
      else
        [ -f "\$unit_path" ] || systemctl list-unit-files "\$unit" --no-legend 2>/dev/null | grep -q "\$unit"
      fi
      ;;
    Darwin)
      plist="\$HOME/Library/LaunchAgents/com.picgallery.\$COMPONENT.plist"
      if [ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" = "1" ]; then
        plist="/Library/LaunchDaemons/com.picgallery.\$COMPONENT.plist"
      fi
      [ -f "\$plist" ]
      ;;
    *)
      return 1
      ;;
  esac
}

service_is_running() {
  case "\$(uname -s)" in
    Linux)
      if [ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" = "1" ]; then
        systemctl is-active --quiet "pic-gallery-\$COMPONENT.service"
      else
        systemctl --user is-active --quiet "pic-gallery-\$COMPONENT.service"
      fi
      ;;
    Darwin)
      domain="gui/\$(id -u)"
      if [ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" = "1" ]; then
        domain="system"
      fi
      launchctl print "\$domain/com.picgallery.\$COMPONENT" 2>/dev/null | grep -Eq 'state = running|pid = [0-9]+'
      ;;
    *)
      return 1
      ;;
  esac
}

manage_service() {
  action="\$1"
  if [ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" = "1" ]; then
    if [ -n "\${PIC_GALLERY_ENV_FILE:-}" ]; then
      "\$MANAGE_SH" "\$action" --components "\$COMPONENT" --env-file "\$PIC_GALLERY_ENV_FILE"
    else
      "\$MANAGE_SH" "\$action" --components "\$COMPONENT"
    fi
  else
    if [ -n "\${PIC_GALLERY_ENV_FILE:-}" ]; then
      "\$MANAGE_SH" "\$action" --components "\$COMPONENT" --user --env-file "\$PIC_GALLERY_ENV_FILE"
    else
      "\$MANAGE_SH" "\$action" --components "\$COMPONENT" --user
    fi
  fi
}

restart_packaged_systemd_service() {
  unit="pic-gallery-\$COMPONENT.service"
  if [ "\$(uname -s)" != "Linux" ]; then
    echo "packaged fallback only supports Linux systemd; missing source manager: \$MANAGE_SH" >&2
    exit 1
  fi
  if [ -f "/etc/systemd/system/\$unit" ] || systemctl list-unit-files "\$unit" --no-legend 2>/dev/null | grep -q "\$unit"; then
    systemctl restart "\$unit"
    systemctl is-active --quiet "\$unit"
    echo "systemd service restarted: \$unit"
    exit 0
  fi
  echo "missing source manager and systemd unit: \$MANAGE_SH, \$unit" >&2
  exit 1
}

if [ ! -x "\$MANAGE_SH" ]; then
  restart_packaged_systemd_service
fi

if ! service_is_registered; then
  manage_service install
fi

if service_is_running; then
  manage_service restart
else
  manage_service start
fi
EOF
  chmod +x "$script"
  echo "wrote $script"
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

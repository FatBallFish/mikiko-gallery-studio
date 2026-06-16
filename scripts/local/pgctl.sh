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
  local bin_name="pic-gallery-$component"
  local description="Pic Gallery $component"
  if [[ "$component" == "api" ]]; then
    bin_name="pic-gallery-api"
    description="Pic Gallery API"
  elif [[ "$component" == "worker" ]]; then
    bin_name="pic-gallery-worker"
    description="Pic Gallery Worker"
  fi
  cat > "$script" <<EOF
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")/../../.." && pwd)"
COMPONENT="$component"
BIN_PATH="\$ROOT_DIR/target/local/bin/$bin_name"
DESCRIPTION="$description"
SERVICE_NAME="pic-gallery-\$COMPONENT"
UNIT="\$SERVICE_NAME.service"
LABEL="com.picgallery.\$COMPONENT"
ENV_FILE="\${PIC_GALLERY_ENV_FILE:-\$ROOT_DIR/.env}"
SYSTEMCTL_ARGS=(--user)
if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" == "1" ]]; then
  SYSTEMCTL_ARGS=()
fi

service_is_registered() {
  case "\$(uname -s)" in
    Linux)
      local unit_path="/etc/systemd/system/\$UNIT"
      if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" != "1" ]]; then
        unit_path="\$HOME/.config/systemd/user/\$UNIT"
      fi
      [[ -f "\$unit_path" ]] || systemctl "\${SYSTEMCTL_ARGS[@]}" list-unit-files "\$UNIT" --no-legend 2>/dev/null | grep -q "\$UNIT"
      ;;
    Darwin)
      [[ -f "\$(launchd_plist_path)" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

service_is_running() {
  case "\$(uname -s)" in
    Linux)
      systemctl "\${SYSTEMCTL_ARGS[@]}" is-active --quiet "\$UNIT"
      ;;
    Darwin)
      launchctl print "\$(launchd_domain)/\$LABEL" 2>/dev/null | grep -Eq 'state = running|pid = [0-9]+'
      ;;
    *)
      return 1
      ;;
  esac
}

run_as_root() {
  if [[ "\$(id -u)" -eq 0 ]]; then
    "\$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "\$@"
    return
  fi
  echo "root permission or sudo is required to manage system service \$SERVICE_NAME" >&2
  exit 1
}

systemd_unit_path() {
  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" == "1" ]]; then
    printf '/etc/systemd/system/%s\n' "\$UNIT"
  else
    printf '%s/.config/systemd/user/%s\n' "\$HOME" "\$UNIT"
  fi
}

launchd_domain() {
  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" == "1" ]]; then
    printf 'system\n'
  else
    printf 'gui/%s\n' "\$(id -u)"
  fi
}

launchd_plist_path() {
  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" == "1" ]]; then
    printf '/Library/LaunchDaemons/%s.plist\n' "\$LABEL"
  else
    printf '%s/Library/LaunchAgents/%s.plist\n' "\$HOME" "\$LABEL"
  fi
}

install_systemd_service() {
  [[ -x "\$BIN_PATH" ]] || { echo "missing executable: \$BIN_PATH" >&2; exit 2; }
  local unit_path
  unit_path="\$(systemd_unit_path)"
  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" != "1" ]]; then
    mkdir -p "\$(dirname "\$unit_path")"
  elif [[ "\$(id -u)" -ne 0 ]] && ! command -v sudo >/dev/null 2>&1; then
    echo "root permission or sudo is required to install \$UNIT" >&2
    exit 1
  fi

  local tmp_unit
  tmp_unit="\$(mktemp)"
  trap 'rm -f "\$tmp_unit"' RETURN
  cat >"\$tmp_unit" <<UNIT
[Unit]
Description=\$DESCRIPTION
After=network.target

[Service]
Type=simple
WorkingDirectory=\$ROOT_DIR
Environment=PIC_GALLERY_ENV_FILE=\$ENV_FILE
ExecStart=\$BIN_PATH
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
UNIT

  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" == "1" ]]; then
    run_as_root install -m 0644 "\$tmp_unit" "\$unit_path"
  else
    install -m 0644 "\$tmp_unit" "\$unit_path"
  fi
  systemctl "\${SYSTEMCTL_ARGS[@]}" daemon-reload
  systemctl "\${SYSTEMCTL_ARGS[@]}" enable "\$UNIT"
}

install_launchd_service() {
  [[ -x "\$BIN_PATH" ]] || { echo "missing executable: \$BIN_PATH" >&2; exit 2; }
  local plist
  plist="\$(launchd_plist_path)"
  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" != "1" ]]; then
    mkdir -p "\$(dirname "\$plist")" "\$ROOT_DIR/tmp"
  elif [[ "\$(id -u)" -ne 0 ]] && ! command -v sudo >/dev/null 2>&1; then
    echo "root permission or sudo is required to install \$LABEL" >&2
    exit 1
  fi

  local tmp_plist
  tmp_plist="\$(mktemp)"
  trap 'rm -f "\$tmp_plist"' RETURN
  cat >"\$tmp_plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>\$LABEL</string>
  <key>WorkingDirectory</key><string>\$ROOT_DIR</string>
  <key>ProgramArguments</key><array><string>\$BIN_PATH</string></array>
  <key>EnvironmentVariables</key><dict><key>PIC_GALLERY_ENV_FILE</key><string>\$ENV_FILE</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>\$ROOT_DIR/tmp/\$COMPONENT.out.log</string>
  <key>StandardErrorPath</key><string>\$ROOT_DIR/tmp/\$COMPONENT.err.log</string>
</dict></plist>
PLIST

  if [[ "\${PIC_GALLERY_SYSTEM_SERVICE:-}" == "1" ]]; then
    run_as_root install -m 0644 "\$tmp_plist" "\$plist"
  else
    install -m 0644 "\$tmp_plist" "\$plist"
  fi
}

if ! service_is_registered; then
  case "\$(uname -s)" in
    Linux) install_systemd_service ;;
    Darwin) install_launchd_service ;;
    *) echo "Unsupported OS. This script supports Linux systemd and macOS launchd." >&2; exit 1 ;;
  esac
fi

if service_is_running; then
  case "\$(uname -s)" in
    Linux) systemctl "\${SYSTEMCTL_ARGS[@]}" restart "\$UNIT" ;;
    Darwin)
      PLIST="\$(launchd_plist_path)"
      DOMAIN="\$(launchd_domain)"
      launchctl bootout "\$DOMAIN" "\$PLIST" >/dev/null 2>&1 || true
      launchctl bootstrap "\$DOMAIN" "\$PLIST"
      ;;
  esac
else
  case "\$(uname -s)" in
    Linux) systemctl "\${SYSTEMCTL_ARGS[@]}" start "\$UNIT" ;;
    Darwin)
      PLIST="\$(launchd_plist_path)"
      DOMAIN="\$(launchd_domain)"
      launchctl bootstrap "\$DOMAIN" "\$PLIST"
      ;;
  esac
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

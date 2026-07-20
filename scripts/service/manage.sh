#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPONENTS="api,worker"
USER_MODE=false
ENV_FILE="${APP_ENV_FILE:-$ROOT_DIR/config/runtime.env}"

usage() {
  cat <<'USAGE'
Usage: scripts/service/manage.sh <install|uninstall|start|stop|restart|status|logs> [--components api,worker] [--user] [--env-file PATH]

Manages local api and worker services.

Linux uses systemd. macOS uses launchd. Windows should use scripts/service/manage.ps1.
USAGE
}

ACTION="${1:-}"
if [[ -z "$ACTION" || "$ACTION" == "-h" || "$ACTION" == "--help" ]]; then
  usage
  exit 0
fi
shift

while [[ $# -gt 0 ]]; do
  case "$1" in
    --components)
      COMPONENTS="${2:?missing components}"
      shift 2
      ;;
    --user)
      USER_MODE=true
      shift
      ;;
    --env-file)
      ENV_FILE="${2:?missing env file}"
      shift 2
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

service_command() {
  local component=$1
  case "$component" in
    api) echo "$ROOT_DIR/target/local/bin/pic-gallery-api" ;;
    worker) echo "$ROOT_DIR/target/local/bin/pic-gallery-worker" ;;
    *) echo "unknown component: $component" >&2; exit 2 ;;
  esac
}

build_component() {
  local component=$1
  mkdir -p "$ROOT_DIR/target/local/bin"
  case "$component" in
    api) (cd "$ROOT_DIR" && go build -o target/local/bin/pic-gallery-api ./cmd/api) ;;
    worker) (cd "$ROOT_DIR" && go build -o target/local/bin/pic-gallery-worker ./cmd/worker) ;;
  esac
}

systemctl_args=()
if [[ "$USER_MODE" == true ]]; then
  systemctl_args=(--user)
fi

install_systemd() {
  local component=$1
  local name="pic-gallery-$component"
  local unit_dir="/etc/systemd/system"
  if [[ "$USER_MODE" == true ]]; then
    unit_dir="$HOME/.config/systemd/user"
    mkdir -p "$unit_dir"
  elif [[ "$(id -u)" -ne 0 ]]; then
    echo "System service install requires root. Use sudo or --user." >&2
    exit 1
  fi
  build_component "$component"
  local command
  command="$(service_command "$component")"
  cat > "$unit_dir/$name.service" <<EOF
[Unit]
Description=Pic Gallery $component
After=network.target

[Service]
Type=simple
WorkingDirectory=$ROOT_DIR
Environment=APP_ENV_FILE=$ENV_FILE
ExecStart=$command
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
EOF
  systemctl "${systemctl_args[@]}" daemon-reload
  systemctl "${systemctl_args[@]}" enable --now "$name.service"
}

manage_systemd() {
  local action=$1
  local component=$2
  local name="pic-gallery-$component.service"
  case "$action" in
    uninstall)
      systemctl "${systemctl_args[@]}" disable --now "$name" >/dev/null 2>&1 || true
      local unit_dir="/etc/systemd/system"
      [[ "$USER_MODE" == true ]] && unit_dir="$HOME/.config/systemd/user"
      rm -f "$unit_dir/$name"
      systemctl "${systemctl_args[@]}" daemon-reload
      ;;
    start|stop|restart|status)
      systemctl "${systemctl_args[@]}" "$action" "$name"
      ;;
    logs)
      journalctl "${systemctl_args[@]}" -u "$name" -f
      ;;
  esac
}

install_launchd() {
  local component=$1
  build_component "$component"
  local label="com.picgallery.$component"
  local plist_dir="$HOME/Library/LaunchAgents"
  local domain="gui/$(id -u)"
  if [[ "$USER_MODE" == false ]]; then
    plist_dir="/Library/LaunchDaemons"
    domain="system"
    [[ "$(id -u)" -eq 0 ]] || { echo "System service install requires root. Use sudo or --user." >&2; exit 1; }
  fi
  mkdir -p "$plist_dir" "$ROOT_DIR/tmp"
  local command
  command="$(service_command "$component")"
  local plist="$plist_dir/$label.plist"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$label</string>
  <key>WorkingDirectory</key><string>$ROOT_DIR</string>
  <key>ProgramArguments</key><array><string>$command</string></array>
  <key>EnvironmentVariables</key><dict><key>APP_ENV_FILE</key><string>$ENV_FILE</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$ROOT_DIR/tmp/$component.out.log</string>
  <key>StandardErrorPath</key><string>$ROOT_DIR/tmp/$component.err.log</string>
</dict></plist>
EOF
  launchctl bootout "$domain" "$plist" >/dev/null 2>&1 || true
  launchctl bootstrap "$domain" "$plist"
  launchctl enable "$domain/$label"
}

manage_launchd() {
  local action=$1
  local component=$2
  local label="com.picgallery.$component"
  local plist_dir="$HOME/Library/LaunchAgents"
  local domain="gui/$(id -u)"
  if [[ "$USER_MODE" == false ]]; then
    plist_dir="/Library/LaunchDaemons"
    domain="system"
  fi
  local plist="$plist_dir/$label.plist"
  case "$action" in
    uninstall)
      launchctl bootout "$domain" "$plist" >/dev/null 2>&1 || true
      rm -f "$plist"
      ;;
    start) launchctl bootstrap "$domain" "$plist" ;;
    stop) launchctl bootout "$domain" "$plist" ;;
    restart)
      launchctl bootout "$domain" "$plist" >/dev/null 2>&1 || true
      launchctl bootstrap "$domain" "$plist"
      ;;
    status) launchctl print "$domain/$label" ;;
    logs) tail -f "$ROOT_DIR/tmp/$component.out.log" "$ROOT_DIR/tmp/$component.err.log" ;;
  esac
}

for component in api worker; do
  has_component "$component" || continue
  case "$(uname -s)" in
    Linux)
      if [[ "$ACTION" == install ]]; then install_systemd "$component"; else manage_systemd "$ACTION" "$component"; fi
      ;;
    Darwin)
      if [[ "$ACTION" == install ]]; then install_launchd "$component"; else manage_launchd "$ACTION" "$component"; fi
      ;;
    *)
      echo "Unsupported OS. Use scripts/service/manage.ps1 on Windows." >&2
      exit 1
      ;;
  esac
done

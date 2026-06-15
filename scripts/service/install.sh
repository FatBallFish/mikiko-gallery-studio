#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPONENTS="api,worker,user-web,admin-web"
USER_MODE=false
APP_CONFIG_PATH="${APP_CONFIG_PATH:-$ROOT_DIR/configs/config.dev.yaml}"

usage() {
  cat <<'EOF'
Usage: scripts/service/install.sh [--components api,worker,user-web,admin-web] [--user]

Installs source-run services for Linux systemd or macOS launchd.

Options:
  --components LIST   Comma-separated components to install. Default: all.
  --user              Install user-level services instead of system-level services.

Environment:
  APP_CONFIG_PATH     Config path for API and worker. Default: configs/config.dev.yaml.
  USER_WEB_PORT       User web port. Default: 5173.
  ADMIN_WEB_PORT      Admin web port. Default: 5174.
  VITE_API_PROXY_TARGET Frontend dev proxy target. Default: http://127.0.0.1:8080.
EOF
}

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
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

has_component() {
  local name="$1"
  IFS=',' read -ra selected <<< "$COMPONENTS"
  for item in "${selected[@]}"; do
    [[ "$(echo "$item" | xargs)" == "$name" ]] && return 0
  done
  return 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

install_systemd_unit() {
  local name="$1"
  local description="$2"
  local command="$3"
  local unit_dir unit_path systemctl_args

  if [[ "$USER_MODE" == true ]]; then
    unit_dir="$HOME/.config/systemd/user"
    mkdir -p "$unit_dir"
    systemctl_args=(--user)
  else
    unit_dir="/etc/systemd/system"
    systemctl_args=()
  fi
  unit_path="$unit_dir/$name.service"

  if [[ "$USER_MODE" == false && "$(id -u)" -ne 0 ]]; then
    echo "System-level install requires root. Re-run with sudo or use --user." >&2
    exit 1
  fi

  cat > "$unit_path" <<EOF
[Unit]
Description=$description
After=network.target

[Service]
Type=simple
WorkingDirectory=$ROOT_DIR
Environment=APP_CONFIG_PATH=$APP_CONFIG_PATH
Environment=USER_WEB_PORT=${USER_WEB_PORT:-5173}
Environment=ADMIN_WEB_PORT=${ADMIN_WEB_PORT:-5174}
Environment=VITE_API_PROXY_TARGET=${VITE_API_PROXY_TARGET:-http://127.0.0.1:8080}
ExecStart=$command
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
EOF

  systemctl "${systemctl_args[@]}" daemon-reload
  systemctl "${systemctl_args[@]}" enable --now "$name.service"
}

install_launchd_plist() {
  local name="$1"
  local command="$2"
  local plist_dir plist_path domain

  if [[ "$USER_MODE" == true ]]; then
    plist_dir="$HOME/Library/LaunchAgents"
    domain="gui/$(id -u)"
  else
    plist_dir="/Library/LaunchDaemons"
    domain="system"
  fi

  if [[ "$USER_MODE" == false && "$(id -u)" -ne 0 ]]; then
    echo "System-level install requires root. Re-run with sudo or use --user." >&2
    exit 1
  fi

  mkdir -p "$plist_dir"
  plist_path="$plist_dir/com.picgallery.$name.plist"

  cat > "$plist_path" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.picgallery.$name</string>
  <key>WorkingDirectory</key><string>$ROOT_DIR</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>-lc</string>
    <string>$command</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>APP_CONFIG_PATH</key><string>$APP_CONFIG_PATH</string>
    <key>APP_ADDR</key><string>${APP_ADDR:-:8080}</string>
    <key>USER_WEB_PORT</key><string>${USER_WEB_PORT:-5173}</string>
    <key>ADMIN_WEB_PORT</key><string>${ADMIN_WEB_PORT:-5174}</string>
    <key>VITE_API_PROXY_TARGET</key><string>${VITE_API_PROXY_TARGET:-http://127.0.0.1:8080}</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$ROOT_DIR/tmp/$name.out.log</string>
  <key>StandardErrorPath</key><string>$ROOT_DIR/tmp/$name.err.log</string>
</dict>
</plist>
EOF

  mkdir -p "$ROOT_DIR/tmp"
  launchctl bootout "$domain" "$plist_path" >/dev/null 2>&1 || true
  launchctl bootstrap "$domain" "$plist_path"
  launchctl enable "$domain/com.picgallery.$name"
}

install_component() {
  local component="$1"
  local command description
  case "$component" in
    api)
      command="/usr/bin/env go run ./cmd/api"
      description="Pic Gallery API"
      ;;
    worker)
      command="/usr/bin/env go run ./cmd/worker"
      description="Pic Gallery Worker"
      ;;
    user-web)
      command="/usr/bin/env npm --prefix web/user run dev -- --host 0.0.0.0 --port ${USER_WEB_PORT:-5173}"
      description="Pic Gallery User Web"
      ;;
    admin-web)
      command="/usr/bin/env npm --prefix web/admin run dev -- --host 0.0.0.0 --port ${ADMIN_WEB_PORT:-5174}"
      description="Pic Gallery Admin Web"
      ;;
    *)
      echo "Unknown component: $component" >&2
      exit 2
      ;;
  esac

  case "$(uname -s)" in
    Linux)
      require_command systemctl
      install_systemd_unit "pic-gallery-$component" "$description" "$command"
      ;;
    Darwin)
      require_command launchctl
      install_launchd_plist "$component" "$command"
      ;;
    *)
      echo "Unsupported OS for this script. Use scripts/service/install.ps1 on Windows." >&2
      exit 1
      ;;
  esac
}

for component in api worker user-web admin-web; do
  if has_component "$component"; then
    install_component "$component"
  fi
done

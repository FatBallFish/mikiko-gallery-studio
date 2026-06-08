#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPONENTS="api,worker,user-web,admin-web"
USER_MODE=false

usage() {
  cat <<'EOF'
Usage: scripts/service/uninstall.sh [--components api,worker,user-web,admin-web] [--user]

Uninstalls source-run services for Linux systemd or macOS launchd.
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

uninstall_systemd_unit() {
  local name="$1"
  local unit_dir systemctl_args
  if [[ "$USER_MODE" == true ]]; then
    unit_dir="$HOME/.config/systemd/user"
    systemctl_args=(--user)
  else
    unit_dir="/etc/systemd/system"
    systemctl_args=()
  fi
  if [[ "$USER_MODE" == false && "$(id -u)" -ne 0 ]]; then
    echo "System-level uninstall requires root. Re-run with sudo or use --user." >&2
    exit 1
  fi
  systemctl "${systemctl_args[@]}" disable --now "$name.service" >/dev/null 2>&1 || true
  rm -f "$unit_dir/$name.service"
  systemctl "${systemctl_args[@]}" daemon-reload
}

uninstall_launchd_plist() {
  local name="$1"
  local plist_dir plist_path domain
  if [[ "$USER_MODE" == true ]]; then
    plist_dir="$HOME/Library/LaunchAgents"
    domain="gui/$(id -u)"
  else
    plist_dir="/Library/LaunchDaemons"
    domain="system"
  fi
  if [[ "$USER_MODE" == false && "$(id -u)" -ne 0 ]]; then
    echo "System-level uninstall requires root. Re-run with sudo or use --user." >&2
    exit 1
  fi
  plist_path="$plist_dir/com.picgallery.$name.plist"
  launchctl bootout "$domain" "$plist_path" >/dev/null 2>&1 || true
  rm -f "$plist_path"
}

for component in api worker user-web admin-web; do
  if ! has_component "$component"; then
    continue
  fi
  case "$(uname -s)" in
    Linux)
      uninstall_systemd_unit "pic-gallery-$component"
      ;;
    Darwin)
      uninstall_launchd_plist "$component"
      ;;
    *)
      echo "Unsupported OS for this script. Use scripts/service/uninstall.ps1 on Windows." >&2
      exit 1
      ;;
  esac
done

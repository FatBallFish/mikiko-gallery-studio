#!/usr/bin/env sh
set -eu

SERVICE_NAME=${PIC_GALLERY_WORKER_SERVICE_NAME:-mikiko-gallery-studio-worker}
APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
BIN_PATH="$APP_DIR/bin/mikiko-gallery-studio-worker"
ENV_FILE=${APP_ENV_FILE:-"$APP_DIR/config/runtime.env"}
UNIT_PATH="/etc/systemd/system/$SERVICE_NAME.service"

if [ ! -x "$BIN_PATH" ]; then
  echo "missing executable: $BIN_PATH" >&2
  exit 2
fi

if [ ! -f "$ENV_FILE" ]; then
  echo "missing runtime env file: $ENV_FILE" >&2
  echo "copy $APP_DIR/config/runtime.env.example to $ENV_FILE and complete setup before installing" >&2
  exit 2
fi

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "warning: ffmpeg is unavailable; media processing readiness will fail until it is installed" >&2
fi
if ! command -v ffprobe >/dev/null 2>&1; then
  echo "warning: ffprobe is unavailable; media processing readiness will fail until it is installed" >&2
fi

run_as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi
  echo "root permission or sudo is required to manage systemd service $SERVICE_NAME" >&2
  exit 1
}

systemd_quote() {
  value=$1
  carriage_return=$(printf '\r')
  case "$value" in
    *"
"*|*"$carriage_return"*) echo "systemd values must not contain line breaks" >&2; return 2 ;;
  esac
  escaped=$(printf '%s' "$value" | sed \
    -e 's/\\/\\\\/g' \
    -e 's/"/\\"/g' \
    -e 's/%/%%/g')
  printf '"%s"' "$escaped"
}

systemd_exec_quote() {
  value=$1
  carriage_return=$(printf '\r')
  case "$value" in
    *"
"*|*"$carriage_return"*) echo "systemd values must not contain line breaks" >&2; return 2 ;;
  esac
  escaped=$(printf '%s' "$value" | sed \
    -e 's/\\/\\\\/g' \
    -e 's/"/\\"/g' \
    -e 's/%/%%/g' \
    -e 's/\$/$$/g')
  printf '"%s"' "$escaped"
}

tmp_unit=$(mktemp)
trap 'rm -f "$tmp_unit"' EXIT
working_directory=$(systemd_quote "$APP_DIR")
environment=$(systemd_quote "APP_ENV_FILE=$ENV_FILE")
exec_start=$(systemd_exec_quote "$BIN_PATH")

cat >"$tmp_unit" <<UNIT
[Unit]
Description=Pic Gallery Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$working_directory
Environment=$environment
ExecStart=$exec_start
Restart=always
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=30
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
UNIT

run_as_root install -m 0644 "$tmp_unit" "$UNIT_PATH"
run_as_root systemctl daemon-reload
run_as_root systemctl enable "$SERVICE_NAME"
run_as_root systemctl restart "$SERVICE_NAME"

echo "systemd service restarted: $SERVICE_NAME"
echo "status: systemctl status $SERVICE_NAME"

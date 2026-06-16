#!/usr/bin/env sh
set -eu

SERVICE_NAME=${PIC_GALLERY_WORKER_SERVICE_NAME:-pic-gallery-worker}
APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
BIN_PATH="$APP_DIR/bin/pic-gallery-worker"
ENV_FILE=${PIC_GALLERY_ENV_FILE:-"$APP_DIR/env/backend.env"}
UNIT_PATH="/etc/systemd/system/$SERVICE_NAME.service"

if [ ! -x "$BIN_PATH" ]; then
  echo "missing executable: $BIN_PATH" >&2
  exit 2
fi

if [ ! -f "$ENV_FILE" ]; then
  echo "missing runtime env file: $ENV_FILE" >&2
  echo "copy $APP_DIR/env/backend.env.example to $ENV_FILE and edit it before installing" >&2
  exit 2
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

tmp_unit=$(mktemp)
trap 'rm -f "$tmp_unit"' EXIT

cat >"$tmp_unit" <<UNIT
[Unit]
Description=Pic Gallery Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$APP_DIR
Environment=PIC_GALLERY_ENV_FILE=$ENV_FILE
ExecStart=$BIN_PATH
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

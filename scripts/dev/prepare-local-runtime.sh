#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_TEMPLATE="$ROOT_DIR/config/runtime.local.env.example"
STATE_TEMPLATE="$ROOT_DIR/config/install-state.local.json.example"
CONFIG_DIR="${PIC_GALLERY_LOCAL_CONFIG_DIR:-$ROOT_DIR/config}"
RUNTIME_FILE="$CONFIG_DIR/runtime.env"
STATE_FILE="$CONFIG_DIR/install-state.json"

if [[ -L "$RUNTIME_FILE" || -L "$STATE_FILE" ]]; then
  echo "local runtime files must not be symbolic links" >&2
  exit 1
fi
if [[ -f "$RUNTIME_FILE" && -f "$STATE_FILE" ]]; then
  exit 0
fi
if [[ -e "$RUNTIME_FILE" || -e "$STATE_FILE" ]]; then
  echo "local runtime is incomplete; config/runtime.env and config/install-state.json must either both exist or both be absent" >&2
  exit 1
fi

umask 077
install -m 600 "$RUNTIME_TEMPLATE" "$RUNTIME_FILE"
if ! install -m 600 "$STATE_TEMPLATE" "$STATE_FILE"; then
  rm -f "$RUNTIME_FILE"
  exit 1
fi

echo "prepared shared local runtime configuration under $CONFIG_DIR"

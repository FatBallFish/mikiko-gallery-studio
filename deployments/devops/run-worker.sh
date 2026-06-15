#!/usr/bin/env sh
set -eu

APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$APP_DIR"

ENV_FILE=${PIC_GALLERY_ENV_FILE:-"$APP_DIR/env/backend.env"}
if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

export APP_CONFIG_PATH=${APP_CONFIG_PATH:-configs/config.example.yaml}
export APP_ENV=${APP_ENV:-prod}
export STORAGE_DRIVER=${STORAGE_DRIVER:-local}
export STORAGE_LOCAL_ROOT=${STORAGE_LOCAL_ROOT:-/var/lib/pic-gallery/storage}
export STORAGE_SHARED_VOLUME=${STORAGE_SHARED_VOLUME:-true}

if [ "$STORAGE_DRIVER" = "local" ]; then
  mkdir -p "$STORAGE_LOCAL_ROOT"
fi

exec "$APP_DIR/bin/pic-gallery-worker"


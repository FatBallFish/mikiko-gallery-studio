#!/usr/bin/env sh
set -eu

APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ENV_FILE=${FRONTEND_ENV_FILE:-"$APP_DIR/env/frontend.env"}
if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

: "${PIC_GALLERY_API_BASE_URL:=}"

escape_js() {
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
}

cat > "$APP_DIR/dist/env.js" <<EOF
window.__PIC_GALLERY_CONFIG__ = {
  apiBaseUrl: '$(escape_js "$PIC_GALLERY_API_BASE_URL")',
};
EOF

echo "Rendered $APP_DIR/dist/env.js. Serve $APP_DIR/dist at /admin/ with nginx."

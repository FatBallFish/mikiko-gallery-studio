#!/usr/bin/env sh
set -eu

APP_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ENV_FILE=${FRONTEND_ENV_FILE:-"$APP_DIR/env/frontend.env"}
if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

: "${PIC_GALLERY_API_BASE_URL:=${PUBLIC_API_URL:-}}"
: "${PIC_GALLERY_API_PORT:=${API_PORT:-8080}}"
: "${PIC_GALLERY_DIRECT_FRONTEND_PORT:=${ADMIN_WEB_PORT:-5174}}"

escape_js() {
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
}

cat > "$APP_DIR/dist/env.js" <<EOF
window.__PIC_GALLERY_CONFIG__ = {
  apiBaseUrl: '$(escape_js "$PIC_GALLERY_API_BASE_URL")',
  apiPort: '$(escape_js "$PIC_GALLERY_API_PORT")',
  directFrontendPort: '$(escape_js "$PIC_GALLERY_DIRECT_FRONTEND_PORT")',
};
EOF

echo "Rendered $APP_DIR/dist/env.js. Serve $APP_DIR/dist at /admin/ with nginx."

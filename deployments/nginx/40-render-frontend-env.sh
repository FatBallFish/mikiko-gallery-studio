#!/usr/bin/env sh
set -eu

: "${PIC_GALLERY_API_BASE_URL:=${PUBLIC_API_URL:-}}"
: "${PIC_GALLERY_API_PORT:=${API_PORT:-8080}}"
: "${PIC_GALLERY_DIRECT_FRONTEND_PORT:=}"
: "${PIC_GALLERY_DOCS_URL:=/developer-docs/}"

escape_js() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e "s/'/\\\\'/g"
}

cat > /usr/share/nginx/html/env.js <<EOF
window.__PIC_GALLERY_CONFIG__ = {
  apiBaseUrl: '$(escape_js "$PIC_GALLERY_API_BASE_URL")',
  apiPort: '$(escape_js "$PIC_GALLERY_API_PORT")',
  directFrontendPort: '$(escape_js "$PIC_GALLERY_DIRECT_FRONTEND_PORT")',
};

window.__PIC_GALLERY_ENV__ = {
  VITE_DOCS_URL: '$(escape_js "$PIC_GALLERY_DOCS_URL")',
};
EOF

#!/usr/bin/env sh
set -eu

: "${PIC_GALLERY_API_BASE_URL:=}"
: "${PIC_GALLERY_DOCS_URL:=/developer-docs/}"

escape_js() {
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
}

cat > /usr/share/nginx/html/env.js <<EOF
window.__PIC_GALLERY_CONFIG__ = {
  apiBaseUrl: '$(escape_js "$PIC_GALLERY_API_BASE_URL")',
};

window.__PIC_GALLERY_ENV__ = {
  VITE_DOCS_URL: '$(escape_js "$PIC_GALLERY_DOCS_URL")',
};
EOF

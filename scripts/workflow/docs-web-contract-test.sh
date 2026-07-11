#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

require_file_text() {
  local file=$1
  local text=$2
  local message=$3
  if [[ ! -f "$file" ]] || ! grep -F "$text" "$file" >/dev/null 2>&1; then
    echo "FAIL: $message" >&2
    exit 1
  fi
}

require_file_text scripts/workflow/verify.sh 'web/docs run typecheck' 'web/docs typecheck is not wired into verify.sh'
require_file_text scripts/workflow/verify.sh 'web/docs run build' 'web/docs build is not wired into verify.sh'

for compose in \
  deployments/docker-compose/docker-compose.dev.yml \
  deployments/docker-compose/docker-compose.e2e.yml \
  deployments/docker-compose/docker-compose.prod.yml; do
  require_file_text "$compose" 'docs-web:' "docs-web service is missing from $compose"
done

require_file_text deployments/nginx/default.conf 'location /developer-docs/' 'gateway does not route /developer-docs/'
require_file_text deployments/nginx/default.conf 'pic_gallery_docs_web' 'gateway does not route to docs-web'
require_file_text web/user/public/env.js 'VITE_DOCS_URL' 'user runtime config does not expose VITE_DOCS_URL'
require_file_text deployments/devops/frontend-env.template.js 'PIC_GALLERY_DOCS_URL' 'packaged frontend runtime template does not expose the docs URL'

echo 'OK: docs web deployment contract passed'

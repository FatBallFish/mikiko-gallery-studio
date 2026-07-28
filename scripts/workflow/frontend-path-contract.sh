#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

VITE_BASE_PATH=./ npm --prefix web/admin run build >/dev/null
VITE_BASE_PATH=./ npm --prefix web/docs run build >/dev/null

for app in admin docs; do
  html="web/$app/dist/index.html"
  grep -Eq '(src|href)="\./assets/' "$html" || { echo "$html does not use relative assets" >&2; exit 1; }
  if grep -Eq '(src|href)="/(admin|developer-docs)/assets/' "$html"; then
    echo "$html contains a deployment-prefix asset URL" >&2
    exit 1
  fi
done

grep -Fq 'src="./env.js"' web/admin/dist/index.html || { echo 'admin env.js is not relocatable' >&2; exit 1; }

for config in deployments/nginx/frontend.conf deployments/devops/nginx-admin-web.conf deployments/devops/nginx-docs-web.conf; do
  grep -Eq 'assets/' "$config" || { echo "$config has no strict assets route" >&2; exit 1; }
  grep -Eq '=404|= 404' "$config" || { echo "$config has no explicit static 404" >&2; exit 1; }
done

grep -Eq 'openapi/' deployments/devops/nginx-docs-web.conf || { echo 'docs nginx has no OpenAPI route' >&2; exit 1; }

echo 'frontend path contract passed'

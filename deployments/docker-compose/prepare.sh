#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() {
  echo "error: $*" >&2
  exit 1
}

generate_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  die "openssl is required to generate deployment secrets"
}

replace_env() {
  local key=$1
  local value=$2
  if sed --version >/dev/null 2>&1; then
    sed -i "s|^${key}=.*|${key}=${value}|" .env.prod
  else
    sed -i '' "s|^${key}=.*|${key}=${value}|" .env.prod
  fi
}

if [[ -f .env.prod && "${1:-}" != "--force" ]]; then
  die ".env.prod already exists; pass --force to overwrite"
fi

cp "$SCRIPT_DIR/.env.prod.example" .env.prod
cp "$SCRIPT_DIR/docker-compose.prod.yml" docker-compose.yml

replace_env POSTGRES_PASSWORD "$(generate_secret)"
replace_env AUTH_ACCESS_TOKEN_SECRET "$(generate_secret)"
replace_env API_KEY_SIGNING_SECRET_ENCRYPTION_KEY "$(generate_secret)"
replace_env CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY "$(generate_secret)"
replace_env PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY "$(generate_secret)"
replace_env PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY "$(generate_secret)"

mkdir -p data postgres-data redis-data storage
chmod 600 .env.prod

cat <<'NEXT'
Pic Gallery Docker deployment files are ready.

Next steps:
  1. Edit .env.prod and set PIC_GALLERY_IMAGE_REGISTRY, public URL, CORS, and admin bootstrap credentials.
  2. Pull images:
     docker compose --env-file .env.prod -f docker-compose.yml pull
  3. Start:
     docker compose --env-file .env.prod -f docker-compose.yml up -d
NEXT

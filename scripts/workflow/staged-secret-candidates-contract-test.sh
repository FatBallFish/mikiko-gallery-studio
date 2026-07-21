#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCANNER="$ROOT/.hook-scripts/staged-secret-candidates.sh"

allowed=(
  ".hook-scripts/staged-secret-candidates.sh"
  "scripts/workflow/staged-secret-candidates-contract-test.sh"
  "internal/repository/ent/schema/clustertoken.go"
  "internal/repository/ent/clustertoken.go"
  "internal/repository/ent/clustertoken_create.go"
  "internal/repository/ent/clustertoken_delete.go"
  "internal/repository/ent/clustertoken_query.go"
  "internal/repository/ent/clustertoken_update.go"
  "internal/repository/ent/clustertoken/clustertoken.go"
  "internal/repository/ent/clustertoken/where.go"
  "web/shared/tokens.css"
  ".env.example"
)
if candidates="$(printf '%s\n' "${allowed[@]}" | "$SCANNER")" && [[ -n "$candidates" ]]; then
  echo "legitimate generated/schema files were classified as secrets:" >&2
  printf '%s\n' "$candidates" >&2
  exit 1
fi

blocked=(
  "internal/repository/ent/clustertoken_secret.go"
  "internal/repository/ent/clustertoken/secret.go"
  "internal/repository/ent/schema/sessiontoken.go"
  "config/private-key.json"
  ".env.local"
)
candidates="$(printf '%s\n' "${blocked[@]}" | "$SCANNER")"
for path in "${blocked[@]}"; do
  if ! grep -Fxq "$path" <<<"$candidates"; then
    echo "secret candidate was not blocked: $path" >&2
    exit 1
  fi
done

echo "OK: staged secret candidate path contract passed"

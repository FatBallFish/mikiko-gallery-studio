#!/usr/bin/env bash
set -euo pipefail

# Checks the API readiness contract exposed by internal/http/handlers.Readyz.
BASE_URL="${1:-${BASE_URL:-http://localhost:8080}}"
READYZ_URL="${BASE_URL%/}/readyz"

if command -v curl >/dev/null 2>&1; then
  body="$(curl --fail --silent --show-error --max-time 5 "$READYZ_URL")"
elif command -v wget >/dev/null 2>&1; then
  body="$(wget -qO- --timeout=5 "$READYZ_URL")"
else
  echo "curl or wget is required to smoke test $READYZ_URL" >&2
  exit 127
fi

printf '%s\n' "$body"

case "$body" in
  *'"status":"ready"'*|*'"status": "ready"'*)
    echo "readyz smoke check passed: $READYZ_URL"
    ;;
  *)
    echo "readyz smoke check failed: expected JSON status=ready from $READYZ_URL" >&2
    exit 1
    ;;
esac

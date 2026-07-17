#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [ ! -d web ]; then
  echo "No web directory; skipping contract verification"
  exit 0
fi

LANDING_BUILD_TESTS=(
  "scripts/workflow/contracts/landing-build.test.mjs"
  "scripts/workflow/contracts/landing-build-wiring.test.mjs"
)
printf '\n==> contract tests %s\n' "${LANDING_BUILD_TESTS[*]}"
node --test "${LANDING_BUILD_TESTS[@]}"

CSS_CONTRACT="scripts/workflow/contracts/luminous-vault-css.mjs"
printf '\n==> contract %s\n' "$CSS_CONTRACT"
node "$CSS_CONTRACT"

LANDING_BUILD_WIRING_CONTRACT="scripts/workflow/contracts/landing-build-wiring.mjs"
printf '\n==> contract %s\n' "$LANDING_BUILD_WIRING_CONTRACT"
node "$LANDING_BUILD_WIRING_CONTRACT"

mapfile -t CONTRACTS < <(find web/admin/src web/user/src web/shared -name '*.contract.ts' -print 2>/dev/null | sort)

if [ "${#CONTRACTS[@]}" -eq 0 ]; then
  echo "No frontend/shared contract files found"
  exit 0
fi

for contract in "${CONTRACTS[@]}"; do
  prefix="web/user"
  if [[ "$contract" == web/admin/* ]]; then
    prefix="web/admin"
  fi

  printf '\n==> contract %s\n' "$contract"
  npm exec --prefix "$prefix" -- tsx "$contract"
done

printf '\nOK: contract verification passed\n'

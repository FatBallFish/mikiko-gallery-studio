#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [ ! -d web ]; then
  echo "No web directory; skipping contract verification"
  exit 0
fi

printf '\n==> contract scripts/workflow/contracts/luminous-vault-css.mjs\n'
node scripts/workflow/contracts/luminous-vault-css.mjs

printf '\n==> contract scripts/workflow/contracts/landing-contract-wiring.mjs\n'
node scripts/workflow/contracts/landing-contract-wiring.mjs

LANDING_CONTRACTS=(
  web/user/src/pages/landingContent.contract.ts
  web/user/src/pages/landingPage.contract.ts
  web/user/src/ui/useLandingMotion.contract.ts
)

AUTH_CONTRACTS=(
  web/user/src/pages/loginPage.contract.ts
  web/user/src/pages/loginPresentation.contract.ts
)

for contract in "${LANDING_CONTRACTS[@]}"; do
  printf '\n==> contract %s\n' "$contract"
  npm exec --prefix web/user -- tsx "$contract"
done

for contract in "${AUTH_CONTRACTS[@]}"; do
  printf '\n==> contract %s\n' "$contract"
  npm exec --prefix web/user -- tsx "$contract"
done

DOCS_CONTRACTS=(
  web/docs/src/openapiManifest.contract.ts
  web/docs/src/content/guides.contract.ts
  web/docs/src/search/searchIndex.contract.ts
)

for contract in "${DOCS_CONTRACTS[@]}"; do
  printf '\n==> contract %s\n' "$contract"
  npm exec --prefix web/user -- tsx "$contract"
done

mapfile -t CONTRACTS < <(find web/admin/src web/user/src web/shared -name '*.contract.ts' -print 2>/dev/null | sort)

if [ "${#CONTRACTS[@]}" -eq 0 ]; then
  echo "No frontend/shared contract files found"
  exit 0
fi

for contract in "${CONTRACTS[@]}"; do
  case " ${LANDING_CONTRACTS[*]} ${AUTH_CONTRACTS[*]} " in
    *" $contract "*) continue ;;
  esac

  prefix="web/user"
  if [[ "$contract" == web/admin/* ]]; then
    prefix="web/admin"
  fi

  printf '\n==> contract %s\n' "$contract"
  npm exec --prefix "$prefix" -- tsx "$contract"
done

printf '\nOK: contract verification passed\n'

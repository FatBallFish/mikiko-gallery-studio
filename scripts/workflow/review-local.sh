#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

SCOPE="all"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --scope)
      SCOPE="${2:-all}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

mkdir -p .review

TREE_SHA="$(git rev-parse HEAD^{tree} 2>/dev/null || git write-tree)"
DECISION="PASS"
FINDINGS=()

if [ ! -f .coding-context.json ]; then
  DECISION="BLOCK"
  FINDINGS+=("missing .coding-context.json; run scripts/workflow/start-coding.sh before coding")
fi

if [ -f .coding-context.json ] && command -v jq >/dev/null 2>&1; then
  req="$(jq -r '.requirement_source // empty' .coding-context.json)"
  design="$(jq -r '.technical_design_source // empty' .coding-context.json)"
  track="$(jq -r '.track // empty' .coding-context.json)"
  approval="$(jq -r '.approval.status // empty' .coding-context.json)"
  if [ -z "$req" ] || [ ! -f "$req" ]; then
    DECISION="BLOCK"
    FINDINGS+=("requirement source missing or unreadable: ${req:-<empty>}")
  fi
  if [ -z "$design" ] || [ ! -f "$design" ]; then
    DECISION="BLOCK"
    FINDINGS+=("technical-design source missing or unreadable: ${design:-<empty>}")
  fi
  if [ "$track" = "heavyweight" ] && [ "$approval" != "approved" ]; then
    DECISION="BLOCK"
    FINDINGS+=("heavyweight task requires approval.status=approved before push/PR")
  fi
fi

secret_candidates="$(
  git diff --cached --name-only | awk '
    /(^|\/)\.env(\.[^\/]*)?\.example$/ { next }
    /(^|\/)\.env($|\.|\/)/ { print; next }
    {
      name=$0
      sub(/^.*\//, "", name)
      lower=tolower(name)
      if (lower == "tokens.css") next
      if (lower ~ /id_rsa|private[._-]?key|secret|token/) print
    }
  '
)"
if [ -n "$secret_candidates" ]; then
  DECISION="BLOCK"
  FINDINGS+=("staged files look like secrets or local env files")
fi

if git diff --cached --name-only | grep -E '\.go$' >/dev/null 2>&1; then
  unformatted="$(gofmt -l $(git diff --cached --name-only | grep -E '\.go$') 2>/dev/null || true)"
  if [ -n "$unformatted" ]; then
    DECISION="BLOCK"
    FINDINGS+=("Go files need gofmt: $(printf '%s' "$unformatted" | tr '\n' ' ')")
  fi
fi

{
  printf '{\n'
  printf '  "schema_version": 1,\n'
  printf '  "decision": "%s",\n' "$DECISION"
  printf '  "scope": "%s",\n' "$SCOPE"
  printf '  "tree_sha": "%s",\n' "$TREE_SHA"
  printf '  "generated_at": "%s",\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')"
  printf '  "checks": {\n'
  printf '    "coding_context": "%s",\n' "$([ -f .coding-context.json ] && echo PASS || echo BLOCK)"
  printf '    "requirement_source": "%s",\n' "$([ -f .coding-context.json ] && command -v jq >/dev/null 2>&1 && req=$(jq -r '.requirement_source // empty' .coding-context.json) && [ -f "$req" ] && echo PASS || echo BLOCK)"
  printf '    "technical_design_source": "%s"\n' "$([ -f .coding-context.json ] && command -v jq >/dev/null 2>&1 && design=$(jq -r '.technical_design_source // empty' .coding-context.json) && [ -f "$design" ] && echo PASS || echo BLOCK)"
  printf '  },\n'
  printf '  "findings": [\n'
  count="${#FINDINGS[@]}"
  i=0
  for finding in "${FINDINGS[@]}"; do
    i=$((i+1))
    comma=","
    [ "$i" -eq "$count" ] && comma=""
    printf '    {"severity": "BLOCK", "message": "%s"}%s\n' "$(printf '%s' "$finding" | sed 's/\\/\\\\/g; s/"/\\"/g')" "$comma"
  done
  printf '  ]\n'
  printf '}\n'
} > .review/gate.json

if [ "$DECISION" = "PASS" ]; then
  echo "review gate: PASS"
  exit 0
fi

echo "review gate: BLOCK"
printf '%s\n' "${FINDINGS[@]}" | sed 's/^/- /'
exit 1

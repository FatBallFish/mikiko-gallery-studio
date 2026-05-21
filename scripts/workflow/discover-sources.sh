#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

TASK=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --task)
      TASK="${2:-}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

score_file() {
  local f="$1" kind="$2" score=0 lower
  lower="$(printf '%s' "$f" | tr '[:upper:]' '[:lower:]')"
  case "$kind:$lower" in
    requirement:*prd*|requirement:*requirement*|requirement:*需求*|requirement:*acceptance*|requirement:*story*|requirement:*issue*) score=$((score+20)) ;;
    design:*tech*|design:*design*|design:*architecture*|design:*方案*|design:*设计*|design:*plan*) score=$((score+20)) ;;
  esac
  case "$kind:$lower" in
    requirement:docs/prd/*) score=$((score+25)) ;;
    requirement:docs/reviews/*prd*) score=$((score+8)) ;;
    design:docs/tech/*) score=$((score+25)) ;;
    design:docs/design/*) score=$((score+18)) ;;
    design:docs/plans/*) score=$((score+10)) ;;
  esac
  case "$lower" in
    docs/template/*) score=$((score-25)) ;;
  esac
  if [ -n "$TASK" ]; then
    # Lightweight token matching. This is discovery aid, not semantic proof.
    for token in $TASK; do
      [ "${#token}" -lt 3 ] && continue
      if printf '%s' "$lower" | grep -qiF "$token"; then
        score=$((score+3))
      elif grep -qiF "$token" "$f" 2>/dev/null; then
        score=$((score+2))
      fi
    done
  fi
  printf '%s\t%s\n' "$score" "$f"
}

find_candidates() {
  local kind="$1"
  find docs README.md api -type f \( -name '*.md' -o -name '*.yaml' -o -name '*.yml' \) 2>/dev/null \
    | while IFS= read -r f; do
        score_file "$f" "$kind"
      done \
    | awk '$1 >= 10 {print}' \
    | sort -rn \
    | head -5
}

REQ="$(find_candidates requirement || true)"
DESIGN="$(find_candidates design || true)"

req_path="$(printf '%s\n' "$REQ" | awk -F '\t' 'NR==1 {print $2}')"
design_path="$(printf '%s\n' "$DESIGN" | awk -F '\t' 'NR==1 {print $2}')"

mkdir -p .workflow
write_candidates_json() {
  local input="$1" first=1
  printf '%s\n' "$input" | while IFS="$(printf '\t')" read -r score path; do
    [ -z "${score:-}" ] && continue
    [ "$first" -eq 0 ] && printf ',\n'
    first=0
    printf '    {"score": %s, "path": "%s"}' "$score" "$(printf '%s' "$path" | sed 's/\\/\\\\/g; s/"/\\"/g')"
  done
  printf '\n'
}

{
  printf '{\n'
  printf '  "task": "%s",\n' "$(printf '%s' "$TASK" | sed 's/\\/\\\\/g; s/"/\\"/g')"
  printf '  "requirement_candidates": [\n'
  write_candidates_json "$REQ"
  printf '  ],\n'
  printf '  "design_candidates": [\n'
  write_candidates_json "$DESIGN"
  printf '  ]\n'
  printf '}\n'
} > .workflow/source-discovery.json

if [ -z "$req_path" ] || [ -z "$design_path" ]; then
  echo "BLOCK: missing requirement or technical-design source." >&2
  echo "Discovery report: .workflow/source-discovery.json" >&2
  [ -z "$req_path" ] && echo "- missing requirement source" >&2
  [ -z "$design_path" ] && echo "- missing technical-design source" >&2
  exit 2
fi

printf 'requirement=%s\n' "$req_path"
printf 'technical_design=%s\n' "$design_path"

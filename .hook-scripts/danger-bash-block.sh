#!/usr/bin/env bash
set -uo pipefail

PAYLOAD="$(cat)"

read_json_field() {
  local expr="$1"
  if command -v jq >/dev/null 2>&1; then
    printf '%s' "$PAYLOAD" | jq -r "$expr // empty" 2>/dev/null
  else
    printf '%s' "$PAYLOAD" | sed -n 's/.*"command"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1
  fi
}

CMD="$(read_json_field '.tool_input.command')"
[ -z "$CMD" ] && exit 0

block() {
  echo "BLOCKED: $1" >&2
  exit 2
}

for pat in \
  "rm -rf /" \
  "rm -rf ~" \
  "git push --force" \
  "git push -f" \
  "git push --force-with-lease" \
  "git reset --hard" \
  "git clean -fd" \
  "git checkout -- ." \
  "mkfs." \
  "dd if="
do
  printf '%s' "$CMD" | grep -qF "$pat" && block "dangerous shell pattern detected: $pat"
done

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
BRANCH="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"

if printf '%s' "$CMD" | grep -Eq '(^|[;&|[:space:]])git[[:space:]]+(commit|merge|cherry-pick|revert)([[:space:]]|$)'; then
  case "$BRANCH" in
    main|master)
      block "direct git commit/merge/cherry-pick/revert on $BRANCH is not allowed"
      ;;
  esac
fi

if printf '%s' "$CMD" | grep -Eq '(^|[;&|[:space:]])git[[:space:]]+push([[:space:]]|$)'; then
  if printf '%s' "$CMD" | grep -Eq '(^|[[:space:]:])main($|[[:space:]])|(^|[[:space:]:])master($|[[:space:]])'; then
    block "direct push to main/master is not allowed"
  fi
  if [ -f "$ROOT/.review/gate.json" ]; then
    (cd "$ROOT" && ./scripts/workflow/check-review-gate.sh >/dev/null 2>&1) || block "review gate is missing, failed, or stale"
  fi
fi

if printf '%s' "$CMD" | grep -Eq 'gh[[:space:]]+pr[[:space:]]+create'; then
  (cd "$ROOT" && ./scripts/workflow/check-review-gate.sh >/dev/null 2>&1) || block "run ./scripts/workflow/review-local.sh --scope committed before gh pr create"
fi

exit 0


#!/usr/bin/env bash
set -uo pipefail

PAYLOAD="$(cat)"
[ -z "$PAYLOAD" ] && exit 0

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

extract_paths() {
  if command -v jq >/dev/null 2>&1; then
    file_path="$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
    [ -n "$file_path" ] && printf '%s\n' "$file_path"
    command_text="$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.command // empty' 2>/dev/null)"
    [ -n "$command_text" ] && printf '%s\n' "$command_text" | grep -oE '^\*\*\* (Update|Add) File: .+$' | sed -E 's/^\*\*\* (Update|Add) File: //'
    patch_text="$(printf '%s' "$PAYLOAD" | jq -r '.tool_input.patch // empty' 2>/dev/null)"
    [ -n "$patch_text" ] && printf '%s\n' "$patch_text" | grep -oE '^\*\*\* (Update|Add) File: .+$' | sed -E 's/^\*\*\* (Update|Add) File: //'
  fi
}

extract_paths | while IFS= read -r path; do
  [ -z "$path" ] && continue
  case "$path" in
    /*) full="$path" ;;
    *) full="$ROOT/$path" ;;
  esac
  [ -f "$full" ] || continue
  case "$full" in
    *.go)
      gofmt -w "$full" 2>/dev/null || true
      ;;
  esac
done

exit 0


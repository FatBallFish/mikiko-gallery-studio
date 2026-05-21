#!/usr/bin/env bash
set -euo pipefail

wf_root() {
  git rev-parse --show-toplevel 2>/dev/null || pwd
}

wf_now() {
  date '+%Y-%m-%dT%H:%M:%S%z'
}

wf_json_escape() {
  sed \
    -e 's/\\/\\\\/g' \
    -e 's/"/\\"/g' \
    -e ':a;N;$!ba;s/\n/\\n/g'
}

wf_has_staged_or_head_changes_matching() {
  local pattern="$1"
  git diff --cached --name-only 2>/dev/null | grep -Eq "$pattern" && return 0
  git diff --name-only HEAD 2>/dev/null | grep -Eq "$pattern" && return 0
  return 1
}


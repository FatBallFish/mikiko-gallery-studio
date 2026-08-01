#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RENDERER="$ROOT/scripts/devops/render-release-notes.sh"
TEMPLATE="$ROOT/.github/release-notes-template.md"

fail() {
  echo "release notes contract failed: $*" >&2
  exit 1
}

require_file() {
  [[ -f "$1" ]] || fail "missing ${1#"$ROOT/"}"
}

require_text() {
  local text=$1
  local file=$2
  rg -Fq -- "$text" "$file" || fail "${file#"$ROOT/"} is missing: $text"
}

require_value_text() {
  local text=$1
  local value=$2
  [[ "$value" == *"$text"* ]] || fail "rendered section is missing: $text"
}

section() {
  local start=$1
  local end=$2
  local file=$3
  awk -v start="$start" -v end="$end" '
    $0 == start { active = 1; next }
    $0 == end { exit }
    active { print }
  ' "$file"
}

new_fixture() {
  local fixture=$1
  git init -q "$fixture"
  git -C "$fixture" config user.name "Release Contract"
  git -C "$fixture" config user.email "release-contract@example.com"
}

commit_fixture() {
  local fixture=$1
  local name=$2
  local subject=$3
  printf '%s\n' "$subject" > "$fixture/$name"
  git -C "$fixture" add "$name"
  git -C "$fixture" commit -q -m "$subject"
}

render_fixture() {
  local fixture=$1
  local tag=$2
  local output=$3
  RELEASE_NOTES_GIT_ROOT="$fixture" \
  RELEASE_NOTES_TEMPLATE="$TEMPLATE" \
  RELEASE_NOTES_OUTPUT="$output" \
  RELEASE_NOTES_REPOSITORY="FatBallFish/mikiko-gallery-studio" \
  RELEASE_NOTES_TAG="$tag" \
    "$RENDERER"
}

require_file "$RENDERER"
require_file "$TEMPLATE"
[[ -x "$RENDERER" ]] || fail "renderer is not executable"

for heading in \
  "## 项目简介" \
  "## Feature 更新" \
  "## Bugfix" \
  "## 优化项" \
  "## 快速部署教程" \
  "## 快速升级教程"; do
  require_text "$heading" "$TEMPLATE"
done

fixture_root=$(mktemp -d)
trap 'rm -rf "$fixture_root"' EXIT

history_fixture="$fixture_root/history"
new_fixture "$history_fixture"
commit_fixture "$history_fixture" baseline.txt "chore: baseline"
git -C "$history_fixture" tag v1.0.0
commit_fixture "$history_fixture" feature.txt "feat(tui): add Chinese release notes"
feature_sha=$(git -C "$history_fixture" rev-parse HEAD)
commit_fixture "$history_fixture" fix.txt "fix(ci): preserve published body"
fix_sha=$(git -C "$history_fixture" rev-parse HEAD)
commit_fixture "$history_fixture" refactor.txt "refactor(release): simplify notes rendering"
commit_fixture "$history_fixture" docs.txt "docs: explain upgrades"
commit_fixture "$history_fixture" other.txt "release polish"
git -C "$history_fixture" tag v1.1.0

history_output="$fixture_root/history.md"
render_fixture "$history_fixture" v1.1.0 "$history_output"
require_text "# Mikiko Gallery Studio v1.1.0" "$history_output"
require_text "https://raw.githubusercontent.com/FatBallFish/mikiko-gallery-studio/v1.1.0/scripts/install.sh" "$history_output"
require_text "MGSCTL_VERSION=v1.1.0" "$history_output"
require_text "mgsctl upgrade" "$history_output"
require_text "mgsctl doctor" "$history_output"

features=$(section "## Feature 更新" "## Bugfix" "$history_output")
bugfixes=$(section "## Bugfix" "## 优化项" "$history_output")
optimizations=$(section "## 优化项" "## 快速部署教程" "$history_output")
require_value_text "feat(tui): add Chinese release notes" "$features"
require_value_text "https://github.com/FatBallFish/mikiko-gallery-studio/commit/$feature_sha" "$features"
require_value_text "fix(ci): preserve published body" "$bugfixes"
require_value_text "https://github.com/FatBallFish/mikiko-gallery-studio/commit/$fix_sha" "$bugfixes"
require_value_text "refactor(release): simplify notes rendering" "$optimizations"
require_value_text "docs: explain upgrades" "$optimizations"
require_value_text "release polish" "$optimizations"
[[ "$features" != *"fix(ci):"* ]] || fail "fix commit leaked into Feature section"
[[ "$bugfixes" != *"refactor(release):"* ]] || fail "optimization leaked into Bugfix section"

first_fixture="$fixture_root/first"
new_fixture "$first_fixture"
commit_fixture "$first_fixture" first.txt "feat: first release"
git -C "$first_fixture" tag v0.1.0
first_output="$fixture_root/first.md"
render_fixture "$first_fixture" v0.1.0 "$first_output"
require_value_text "feat: first release" "$(section "## Feature 更新" "## Bugfix" "$first_output")"
require_value_text "本版本暂无" "$(section "## Bugfix" "## 优化项" "$first_output")"
require_value_text "本版本暂无" "$(section "## 优化项" "## 快速部署教程" "$first_output")"

if RELEASE_NOTES_GIT_ROOT="$history_fixture" \
  RELEASE_NOTES_TEMPLATE="$TEMPLATE" \
  RELEASE_NOTES_OUTPUT="$fixture_root/invalid.md" \
  RELEASE_NOTES_REPOSITORY="FatBallFish/mikiko-gallery-studio" \
  RELEASE_NOTES_TAG="latest" \
    "$RENDERER" >/dev/null 2>&1; then
  fail "renderer accepted a non-SemVer tag"
fi

if RELEASE_NOTES_GIT_ROOT="$history_fixture" \
  RELEASE_NOTES_TEMPLATE="$fixture_root/missing-template.md" \
  RELEASE_NOTES_OUTPUT="$fixture_root/missing.md" \
  RELEASE_NOTES_REPOSITORY="FatBallFish/mikiko-gallery-studio" \
  RELEASE_NOTES_TAG="v1.1.0" \
    "$RENDERER" >/dev/null 2>&1; then
  fail "renderer accepted a missing template"
fi

if RELEASE_NOTES_GIT_ROOT="$history_fixture" \
  RELEASE_NOTES_TEMPLATE="$TEMPLATE" \
  RELEASE_NOTES_OUTPUT="$fixture_root/unknown.md" \
  RELEASE_NOTES_REPOSITORY="FatBallFish/mikiko-gallery-studio" \
  RELEASE_NOTES_TAG="v9.9.9" \
    "$RENDERER" >/dev/null 2>&1; then
  fail "renderer accepted an unknown tag"
fi

if rg -q '\{\{[A-Z_]+\}\}' "$history_output" "$first_output"; then
  fail "rendered notes contain unresolved placeholders"
fi

echo "OK: templated release notes contract verified"

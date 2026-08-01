#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tag=${RELEASE_NOTES_TAG:-${1:-}}
repository=${RELEASE_NOTES_REPOSITORY:-FatBallFish/mikiko-gallery-studio}
git_root=${RELEASE_NOTES_GIT_ROOT:-$ROOT}
template=${RELEASE_NOTES_TEMPLATE:-$ROOT/.github/release-notes-template.md}
output=${RELEASE_NOTES_OUTPUT:-$ROOT/target/release-notes.md}

fail() {
  echo "render release notes: $*" >&2
  exit 1
}

semver_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
[[ "$tag" =~ $semver_pattern ]] || fail "RELEASE_NOTES_TAG must be a v-prefixed SemVer"
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail "RELEASE_NOTES_REPOSITORY must be owner/name"
[[ -d "$git_root" ]] || fail "Git root does not exist: $git_root"
[[ -r "$template" ]] || fail "template is not readable: $template"
git -C "$git_root" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "Git root is not a repository: $git_root"
git -C "$git_root" rev-parse --verify "$tag^{commit}" >/dev/null 2>&1 || fail "tag does not resolve to a commit: $tag"

output_dir=$(dirname "$output")
mkdir -p "$output_dir"
stage_dir=$(mktemp -d "$output_dir/.release-notes.XXXXXX")
trap 'rm -rf "$stage_dir"' EXIT

features_file="$stage_dir/features.md"
bugfixes_file="$stage_dir/bugfixes.md"
optimizations_file="$stage_dir/optimizations.md"
: > "$features_file"
: > "$bugfixes_file"
: > "$optimizations_file"

previous_tag=$(git -C "$git_root" describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' "$tag^" 2>/dev/null || true)
if [[ -n "$previous_tag" ]]; then
  revision_range="$previous_tag..$tag"
else
  revision_range="$tag"
fi

feature_pattern='^feat(\([^)]*\))?!?:[[:space:]]*'
bugfix_pattern='^fix(\([^)]*\))?!?:[[:space:]]*'
optimization_pattern='^(perf|refactor|ci|chore|docs|build|test|style)(\([^)]*\))?!?:[[:space:]]*'
commit_count=0
while IFS=$'\t' read -r sha subject; do
  [[ -n "$sha" ]] || continue
  commit_count=$((commit_count + 1))
  short_sha=${sha:0:7}
  target_file=$optimizations_file
  if [[ "$subject" =~ $feature_pattern ]]; then
    target_file=$features_file
  elif [[ "$subject" =~ $bugfix_pattern ]]; then
    target_file=$bugfixes_file
  elif [[ "$subject" =~ $optimization_pattern ]]; then
    target_file=$optimizations_file
  fi
  printf -- '- %s ([`%s`](https://github.com/%s/commit/%s))\n' \
    "$subject" "$short_sha" "$repository" "$sha" >> "$target_file"
done < <(git -C "$git_root" log --reverse --no-merges --format='%H%x09%s' "$revision_range")

if [[ "$commit_count" -eq 0 ]]; then
  fail "release range contains no non-merge commits: $revision_range"
fi
for category_file in "$features_file" "$bugfixes_file" "$optimizations_file"; do
  if [[ ! -s "$category_file" ]]; then
    printf '%s\n' '本版本暂无。' > "$category_file"
  fi
done

rendered="$stage_dir/release-notes.md"
awk \
  -v version="$tag" \
  -v features="$features_file" \
  -v bugfixes="$bugfixes_file" \
  -v optimizations="$optimizations_file" '
  function emit(file, line) {
    while ((getline line < file) > 0) {
      print line
    }
    close(file)
  }
  $0 == "{{FEATURES}}" { emit(features); next }
  $0 == "{{BUGFIXES}}" { emit(bugfixes); next }
  $0 == "{{OPTIMIZATIONS}}" { emit(optimizations); next }
  {
    line = $0
    gsub(/\{\{VERSION\}\}/, version, line)
    print line
  }
' "$template" > "$rendered"

[[ -s "$rendered" ]] || fail "rendered notes are empty"
if grep -Eq '\{\{[A-Z_]+\}\}' "$rendered"; then
  fail "rendered notes contain unresolved placeholders"
fi
mv "$rendered" "$output"
echo "Rendered release notes for $tag to $output"

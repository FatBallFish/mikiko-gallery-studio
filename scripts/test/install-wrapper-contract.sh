#!/usr/bin/env bash
set -euo pipefail

# Git hooks export repository internals that would otherwise leak into fixture repositories.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TMP_ROOT=$(mktemp -d)
trap 'rm -rf "$TMP_ROOT"' EXIT

old_tool='deploy''ctl'
old_env='DEPLOY''CTL'

[[ -d "$ROOT/cmd/mgsctl" ]] || {
  echo "install wrapper contract: cmd/mgsctl is required" >&2
  exit 1
}
[[ ! -e "$ROOT/cmd/$old_tool" ]] || {
  echo "install wrapper contract: legacy command path is still present" >&2
  exit 1
}
[[ -d "$ROOT/internal/mgsctl" ]] || {
  echo "install wrapper contract: internal/mgsctl is required" >&2
  exit 1
}
[[ ! -e "$ROOT/internal/$old_tool" ]] || {
  echo "install wrapper contract: legacy internal package path is still present" >&2
  exit 1
}
[[ -x "$ROOT/scripts/devops/package-mgsctl.sh" ]] || {
  echo "install wrapper contract: package-mgsctl.sh is required" >&2
  exit 1
}
[[ ! -e "$ROOT/scripts/devops/package-$old_tool.sh" ]] || {
  echo "install wrapper contract: legacy package script is still present" >&2
  exit 1
}

if rg -n "$old_tool|$old_env" \
  "$ROOT/Makefile" "$ROOT/cmd" "$ROOT/internal" "$ROOT/deployments" "$ROOT/scripts" "$ROOT/.github" \
  "$ROOT/README.md" "$ROOT/README.zh-CN.md" "$ROOT/docs/runbooks" "$ROOT/docs/deploy" \
  --glob '!test/install-wrapper-contract.sh'; then
  echo "install wrapper contract: legacy deployment-tool brand remains on a current surface" >&2
  exit 1
fi

fail() {
  echo "install wrapper contract: $*" >&2
  exit 1
}

assert_contains() {
  local content=$1
  local expected=$2
  [[ "$content" == *"$expected"* ]] || fail "expected output to contain '$expected': $content"
}

make_checkout() {
  local checkout=$1
  mkdir -p "$checkout/scripts" "$checkout/cmd/mgsctl"
  cp "$ROOT/scripts/install.sh" "$checkout/scripts/install.sh"
  : > "$checkout/go.mod"
  : > "$checkout/Makefile"
  for dockerfile in Dockerfile.api Dockerfile.worker Dockerfile.user-web Dockerfile.admin-web Dockerfile.docs-web; do
    : > "$checkout/$dockerfile"
  done
}

initialize_git_checkout() {
  local checkout=$1
  git -C "$checkout" init -q
  git -C "$checkout" config user.name "Install Wrapper Contract"
  git -C "$checkout" config user.email "install-wrapper@example.test"
  git -C "$checkout" add .
  git -C "$checkout" commit -qm "fixture"
}

make_path_mgsctl() {
  local directory=$1
  mkdir -p "$directory"
  cat > "$directory/mgsctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "version" && "${2:-}" == "--json" ]]; then
  printf '{"version":"dev","commit":"%s","build_time":"test","dirty":false}\n' "$FAKE_PATH_COMMIT"
  exit 0
fi
printf '%s\n' "$*" > "$FAKE_PATH_EXEC_LOG"
SCRIPT
  chmod +x "$directory/mgsctl"
}

make_fake_toolchain() {
  local directory=$1
  mkdir -p "$directory"
  cat > "$directory/curl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output|-o) output=$2; shift 2 ;;
    --*) shift ;;
    *) url=$1; shift ;;
  esac
done
[[ -n "$output" && -n "$url" ]] || exit 64
if [[ -n "${FAKE_CURL_LOG:-}" ]]; then
  printf '%s\n' "$url" >> "$FAKE_CURL_LOG"
fi
if [[ "${FAKE_CURL_MODE:-success}" != "success" ]]; then
  exit 22
fi
if [[ "$url" == *.sha256 ]]; then
  printf '%s  %s\n' "$FAKE_RELEASE_SHA" "$(basename "${url%.sha256}")" > "$output"
else
  cp "$FAKE_RELEASE_BINARY" "$output"
fi
SCRIPT
  cat > "$directory/make" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
output=""
for argument in "$@"; do
  case "$argument" in
    MGSCTL_OUTPUT=*) output=${argument#MGSCTL_OUTPUT=} ;;
  esac
done
[[ -n "$output" ]] || exit 64
printf '%s|%s\n' "$PWD" "$*" > "$FAKE_MAKE_LOG"
echo "go build -o $output ./cmd/mgsctl"
cp "$FAKE_RELEASE_BINARY" "$output"
chmod 0755 "$output"
SCRIPT
  cat > "$directory/go" <<'SCRIPT'
#!/usr/bin/env bash
exit 0
SCRIPT
  chmod +x "$directory/curl" "$directory/make" "$directory/go"
}

RELEASE_BINARY="$TMP_ROOT/release-mgsctl"
cat > "$RELEASE_BINARY" <<'SCRIPT'
#!/usr/bin/env sh
printf '%s\n' "$*" > "$FAKE_EXEC_LOG"
printf '%s\n' "${MGSCTL_SOURCE_DIR:-}" > "$FAKE_SOURCE_LOG"
SCRIPT
chmod +x "$RELEASE_BINARY"
if command -v sha256sum >/dev/null 2>&1; then
  RELEASE_SHA=$(sha256sum "$RELEASE_BINARY" | awk '{print $1}')
else
  RELEASE_SHA=$(shasum -a 256 "$RELEASE_BINARY" | awk '{print $1}')
fi

FAKE_BIN="$TMP_ROOT/fake-bin"
make_fake_toolchain "$FAKE_BIN"
CHECKOUT="$TMP_ROOT/source checkout"
make_checkout "$CHECKOUT"
BASE_PATH="$FAKE_BIN:/usr/bin:/bin"

git_checkout="$TMP_ROOT/git-checkout"
make_checkout "$git_checkout"
initialize_git_checkout "$git_checkout"
checkout_commit=$(git -C "$git_checkout" rev-parse HEAD)

stale_bin="$TMP_ROOT/stale-path-bin"
make_path_mgsctl "$stale_bin"
stale_install="$TMP_ROOT/stale-install"
stale_make_log="$TMP_ROOT/stale-make.log"
stale_exec_log="$TMP_ROOT/stale-built-exec.log"
stale_source_log="$TMP_ROOT/stale-built-source.log"
stale_path_exec_log="$TMP_ROOT/stale-path-exec.log"
stale_curl_log="$TMP_ROOT/stale-curl.log"
stale_output=$(env \
  PATH="$stale_bin:$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$stale_install" \
  FAKE_PATH_COMMIT="stale-commit" FAKE_PATH_EXEC_LOG="$stale_path_exec_log" \
  FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_EXEC_LOG="$stale_exec_log" FAKE_SOURCE_LOG="$stale_source_log" FAKE_MAKE_LOG="$stale_make_log" FAKE_CURL_LOG="$stale_curl_log" \
  sh "$git_checkout/scripts/install.sh" version 2>&1)
[[ ! -e "$stale_path_exec_log" ]] || fail "stale PATH mgsctl received the final command"
[[ ! -e "$stale_curl_log" ]] || fail "stale PATH mgsctl fallback unexpectedly attempted a release download"
[[ -x "$stale_install/mgsctl" ]] || fail "stale PATH mgsctl was not replaced persistently"
[[ $(cat "$stale_exec_log") == "version" ]] || fail "rebuilt mgsctl did not receive original arguments"
[[ $(cat "$stale_source_log") == "$git_checkout" ]] || fail "rebuilt mgsctl did not receive source checkout path"
assert_contains "$(cat "$stale_make_log")" "mgsctl"
assert_contains "$stale_output" "stale"
assert_contains "$stale_output" "local source build"

matching_bin="$TMP_ROOT/matching-path-bin"
make_path_mgsctl "$matching_bin"
matching_path_exec_log="$TMP_ROOT/matching-path-exec.log"
matching_make_log="$TMP_ROOT/matching-make.log"
env \
  PATH="$matching_bin:$BASE_PATH" HOME="$TMP_ROOT/home" \
  FAKE_PATH_COMMIT="$checkout_commit" FAKE_PATH_EXEC_LOG="$matching_path_exec_log" FAKE_MAKE_LOG="$matching_make_log" \
  sh "$git_checkout/scripts/install.sh" status
[[ $(cat "$matching_path_exec_log") == "status" ]] || fail "matching PATH mgsctl was not reused"
[[ ! -e "$matching_make_log" ]] || fail "matching PATH mgsctl unexpectedly triggered a local build"

printf 'dirty\n' >> "$git_checkout/go.mod"
dirty_install="$TMP_ROOT/dirty-install"
dirty_make_log="$TMP_ROOT/dirty-make.log"
dirty_exec_log="$TMP_ROOT/dirty-built-exec.log"
dirty_output=$(env \
  PATH="$matching_bin:$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$dirty_install" \
  FAKE_PATH_COMMIT="$checkout_commit" FAKE_PATH_EXEC_LOG="$TMP_ROOT/dirty-path-exec.log" \
  FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_EXEC_LOG="$dirty_exec_log" FAKE_SOURCE_LOG="$TMP_ROOT/dirty-source.log" FAKE_MAKE_LOG="$dirty_make_log" \
  sh "$git_checkout/scripts/install.sh" doctor 2>&1)
[[ $(cat "$dirty_exec_log") == "doctor" ]] || fail "dirty source rebuild did not receive original arguments"
assert_contains "$dirty_output" "uncommitted changes"
assert_contains "$(cat "$dirty_make_log")" "mgsctl"

explicit_bin="$TMP_ROOT/explicit-bin"
cp "$RELEASE_BINARY" "$explicit_bin"
explicit_log="$TMP_ROOT/explicit-exec.log"
env \
  PATH="$stale_bin:$BASE_PATH" MGSCTL_BIN="$explicit_bin" \
  FAKE_EXEC_LOG="$explicit_log" FAKE_SOURCE_LOG="$TMP_ROOT/explicit-source.log" \
  sh "$git_checkout/scripts/install.sh" logs --follow
[[ $(cat "$explicit_log") == "logs --follow" ]] || fail "MGSCTL_BIN did not remain authoritative"

non_git_path_log="$TMP_ROOT/non-git-path-exec.log"
env \
  PATH="$stale_bin:$BASE_PATH" FAKE_PATH_COMMIT="stale-commit" FAKE_PATH_EXEC_LOG="$non_git_path_log" \
  sh "$CHECKOUT/scripts/install.sh" status
[[ $(cat "$non_git_path_log") == "status" ]] || fail "non-Git checkout did not preserve PATH-first behavior"

success_home="$TMP_ROOT/success-home"
mkdir -p "$success_home"
success_install="$TMP_ROOT/success bin"
success_log="$TMP_ROOT/success-exec.log"
success_source_log="$TMP_ROOT/success-source.log"
success_output=$(env \
  PATH="$BASE_PATH" HOME="$success_home" SHELL=/bin/zsh MGSCTL_INSTALL_DIR="$success_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$success_log" FAKE_SOURCE_LOG="$success_source_log" \
  sh "$CHECKOUT/scripts/install.sh" version --json 2>&1)
[[ -x "$success_install/mgsctl" ]] || fail "verified release was not installed persistently"
[[ $(cat "$success_log") == "version --json" ]] || fail "installed mgsctl did not receive original arguments"
[[ $(cat "$success_source_log") == "$CHECKOUT" ]] || fail "installed mgsctl did not receive the complete source checkout path"
assert_contains "$success_output" "$success_install/mgsctl"
path_marker='# >>> mikiko-gallery-studio mgsctl >>>'
path_export="export PATH=\"$success_install:\$PATH\""
for profile in "$success_home/.profile" "$success_home/.zshrc"; do
  assert_contains "$(cat "$profile")" "$path_marker"
  assert_contains "$(cat "$profile")" "$path_export"
done

env \
  PATH="$BASE_PATH" HOME="$success_home" SHELL=/bin/zsh MGSCTL_INSTALL_DIR="$success_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$success_log" FAKE_SOURCE_LOG="$success_source_log" \
  sh "$CHECKOUT/scripts/install.sh" status >/dev/null 2>&1
for profile in "$success_home/.profile" "$success_home/.zshrc"; do
  [[ $(grep -F -c "$path_marker" "$profile") -eq 1 ]] || fail "PATH marker was duplicated in $profile"
done
[[ $(cat "$success_log") == "status" ]] || fail "repeated install did not execute the installed binary by absolute path"

bash_home="$TMP_ROOT/bash-home"
mkdir -p "$bash_home"
env \
  PATH="$BASE_PATH" HOME="$bash_home" SHELL=/bin/bash MGSCTL_INSTALL_DIR="$TMP_ROOT/bash-bin" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$TMP_ROOT/bash-exec.log" FAKE_SOURCE_LOG="$TMP_ROOT/bash-source.log" \
  sh "$CHECKOUT/scripts/install.sh" version >/dev/null 2>&1
assert_contains "$(cat "$bash_home/.bashrc")" "$path_marker"

in_path_home="$TMP_ROOT/in-path-home"
in_path_install="$TMP_ROOT/in-path-bin"
mkdir -p "$in_path_home" "$in_path_install"
env \
  PATH="$in_path_install:$BASE_PATH" HOME="$in_path_home" SHELL=/bin/zsh MGSCTL_INSTALL_DIR="$in_path_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$TMP_ROOT/in-path-exec.log" FAKE_SOURCE_LOG="$TMP_ROOT/in-path-source.log" \
  sh "$CHECKOUT/scripts/install.sh" version >/dev/null 2>&1
[[ ! -e "$in_path_home/.profile" && ! -e "$in_path_home/.zshrc" ]] || fail "profiles changed when install directory was already in PATH"

symlink_home="$TMP_ROOT/symlink-home"
mkdir -p "$symlink_home"
symlink_target="$TMP_ROOT/profile-target"
printf 'preserve me\n' > "$symlink_target"
ln -s "$symlink_target" "$symlink_home/.profile"
set +e
env \
  PATH="$BASE_PATH" HOME="$symlink_home" SHELL=/bin/zsh MGSCTL_INSTALL_DIR="$TMP_ROOT/symlink-bin" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$TMP_ROOT/symlink-exec.log" FAKE_SOURCE_LOG="$TMP_ROOT/symlink-source.log" \
  sh "$CHECKOUT/scripts/install.sh" version >/dev/null 2>&1
symlink_status=$?
set -e
[[ $symlink_status -ne 0 ]] || fail "symlinked shell profile was accepted"
[[ $(cat "$symlink_target") == "preserve me" ]] || fail "symlinked shell profile target changed"

fallback_install="$TMP_ROOT/fallback bin"
fallback_log="$TMP_ROOT/fallback-exec.log"
fallback_source_log="$TMP_ROOT/fallback-source.log"
make_log="$TMP_ROOT/make.log"
fallback_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$fallback_install" \
  MGSCTL_DOWNLOAD_URL="https://downloads.example.test/mgsctl?token=wrapper-query-secret" \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$fallback_log" FAKE_SOURCE_LOG="$fallback_source_log" FAKE_MAKE_LOG="$make_log" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
[[ -x "$fallback_install/mgsctl" ]] || fail "release failure did not install a local build"
[[ $(cat "$fallback_log") == "version" ]] || fail "local build did not receive original arguments"
[[ $(cat "$fallback_source_log") == "$CHECKOUT" ]] || fail "local build did not receive the complete source checkout path"
assert_contains "$fallback_output" "falling back to a local source build"
[[ "$fallback_output" != *"wrapper-query-secret"* ]] || fail "release failure leaked a signed download URL query"
assert_contains "$(cat "$make_log")" "mgsctl"
assert_contains "$(cat "$make_log")" "MGSCTL_OUTPUT="

mismatch_install="$TMP_ROOT/mismatch-bin"
mkdir -p "$mismatch_install"
printf 'known-good\n' > "$mismatch_install/mgsctl"
chmod +x "$mismatch_install/mgsctl"
mismatch_make_log="$TMP_ROOT/mismatch-make.log"
set +e
mismatch_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$mismatch_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$(printf '0%.0s' {1..64})" \
  FAKE_EXEC_LOG="$TMP_ROOT/mismatch-exec.log" FAKE_MAKE_LOG="$mismatch_make_log" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
mismatch_status=$?
set -e
[[ $mismatch_status -ne 0 ]] || fail "checksum mismatch unexpectedly succeeded"
[[ $(cat "$mismatch_install/mgsctl") == "known-good" ]] || fail "checksum mismatch replaced the known-good mgsctl"
[[ ! -e "$mismatch_make_log" ]] || fail "checksum mismatch incorrectly invoked the local build fallback"
assert_contains "$mismatch_output" "checksum verification failed"

incomplete="$TMP_ROOT/incomplete"
mkdir -p "$incomplete/scripts"
cp "$ROOT/scripts/install.sh" "$incomplete/scripts/install.sh"
set +e
incomplete_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$TMP_ROOT/incomplete-bin" \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" \
  sh "$incomplete/scripts/install.sh" version 2>&1)
incomplete_status=$?
set -e
[[ $incomplete_status -ne 0 ]] || fail "incomplete source checkout unexpectedly succeeded"
for missing in go.mod Makefile cmd/mgsctl; do
  assert_contains "$incomplete_output" "$missing"
done

set +e
missing_go_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$TMP_ROOT/missing-go-bin" GO=missing-go \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
missing_go_status=$?
missing_make_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" MGSCTL_INSTALL_DIR="$TMP_ROOT/missing-make-bin" MAKE=missing-make \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
missing_make_status=$?
set -e
[[ $missing_go_status -ne 0 ]] || fail "missing Go unexpectedly succeeded"
[[ $missing_make_status -ne 0 ]] || fail "missing Make unexpectedly succeeded"
assert_contains "$missing_go_output" "Go"
assert_contains "$missing_make_output" "Make"
assert_contains "$missing_make_output" "MGSCTL_BIN"
assert_contains "$(cat "$ROOT/scripts/install.ps1")" 'MGSCTL_SOURCE_DIR'
assert_contains "$(cat "$ROOT/scripts/install.ps1")" 'ConvertFrom-Json'
assert_contains "$(cat "$ROOT/scripts/install.ps1")" 'PATH mgsctl is stale'

echo "OK: mgsctl install wrapper fallback contract verified"

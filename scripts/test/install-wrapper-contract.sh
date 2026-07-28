#!/usr/bin/env bash
set -euo pipefail

# Git hooks export repository internals that would otherwise leak into fixture repositories.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TMP_ROOT=$(mktemp -d)
trap 'rm -rf "$TMP_ROOT"' EXIT

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
  mkdir -p "$checkout/scripts" "$checkout/cmd/deployctl"
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

make_path_deployctl() {
  local directory=$1
  mkdir -p "$directory"
  cat > "$directory/deployctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "version" && "${2:-}" == "--json" ]]; then
  printf '{"version":"dev","commit":"%s","build_time":"test","dirty":false}\n' "$FAKE_PATH_COMMIT"
  exit 0
fi
printf '%s\n' "$*" > "$FAKE_PATH_EXEC_LOG"
SCRIPT
  chmod +x "$directory/deployctl"
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
    DEPLOYCTL_OUTPUT=*) output=${argument#DEPLOYCTL_OUTPUT=} ;;
  esac
done
[[ -n "$output" ]] || exit 64
printf '%s|%s\n' "$PWD" "$*" > "$FAKE_MAKE_LOG"
echo "go build -o $output ./cmd/deployctl"
cp "$FAKE_RELEASE_BINARY" "$output"
chmod 0755 "$output"
SCRIPT
  cat > "$directory/go" <<'SCRIPT'
#!/usr/bin/env bash
exit 0
SCRIPT
  chmod +x "$directory/curl" "$directory/make" "$directory/go"
}

RELEASE_BINARY="$TMP_ROOT/release-deployctl"
cat > "$RELEASE_BINARY" <<'SCRIPT'
#!/usr/bin/env sh
printf '%s\n' "$*" > "$FAKE_EXEC_LOG"
printf '%s\n' "${DEPLOYCTL_SOURCE_DIR:-}" > "$FAKE_SOURCE_LOG"
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
make_path_deployctl "$stale_bin"
stale_install="$TMP_ROOT/stale-install"
stale_make_log="$TMP_ROOT/stale-make.log"
stale_exec_log="$TMP_ROOT/stale-built-exec.log"
stale_source_log="$TMP_ROOT/stale-built-source.log"
stale_path_exec_log="$TMP_ROOT/stale-path-exec.log"
stale_curl_log="$TMP_ROOT/stale-curl.log"
stale_output=$(env \
  PATH="$stale_bin:$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$stale_install" \
  FAKE_PATH_COMMIT="stale-commit" FAKE_PATH_EXEC_LOG="$stale_path_exec_log" \
  FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_EXEC_LOG="$stale_exec_log" FAKE_SOURCE_LOG="$stale_source_log" FAKE_MAKE_LOG="$stale_make_log" FAKE_CURL_LOG="$stale_curl_log" \
  sh "$git_checkout/scripts/install.sh" version 2>&1)
[[ ! -e "$stale_path_exec_log" ]] || fail "stale PATH deployctl received the final command"
[[ ! -e "$stale_curl_log" ]] || fail "stale PATH deployctl fallback unexpectedly attempted a release download"
[[ -x "$stale_install/deployctl" ]] || fail "stale PATH deployctl was not replaced persistently"
[[ $(cat "$stale_exec_log") == "version" ]] || fail "rebuilt deployctl did not receive original arguments"
[[ $(cat "$stale_source_log") == "$git_checkout" ]] || fail "rebuilt deployctl did not receive source checkout path"
assert_contains "$(cat "$stale_make_log")" "deployctl"
assert_contains "$stale_output" "stale"
assert_contains "$stale_output" "local source build"

matching_bin="$TMP_ROOT/matching-path-bin"
make_path_deployctl "$matching_bin"
matching_path_exec_log="$TMP_ROOT/matching-path-exec.log"
matching_make_log="$TMP_ROOT/matching-make.log"
env \
  PATH="$matching_bin:$BASE_PATH" HOME="$TMP_ROOT/home" \
  FAKE_PATH_COMMIT="$checkout_commit" FAKE_PATH_EXEC_LOG="$matching_path_exec_log" FAKE_MAKE_LOG="$matching_make_log" \
  sh "$git_checkout/scripts/install.sh" status
[[ $(cat "$matching_path_exec_log") == "status" ]] || fail "matching PATH deployctl was not reused"
[[ ! -e "$matching_make_log" ]] || fail "matching PATH deployctl unexpectedly triggered a local build"

printf 'dirty\n' >> "$git_checkout/go.mod"
dirty_install="$TMP_ROOT/dirty-install"
dirty_make_log="$TMP_ROOT/dirty-make.log"
dirty_exec_log="$TMP_ROOT/dirty-built-exec.log"
dirty_output=$(env \
  PATH="$matching_bin:$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$dirty_install" \
  FAKE_PATH_COMMIT="$checkout_commit" FAKE_PATH_EXEC_LOG="$TMP_ROOT/dirty-path-exec.log" \
  FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_EXEC_LOG="$dirty_exec_log" FAKE_SOURCE_LOG="$TMP_ROOT/dirty-source.log" FAKE_MAKE_LOG="$dirty_make_log" \
  sh "$git_checkout/scripts/install.sh" doctor 2>&1)
[[ $(cat "$dirty_exec_log") == "doctor" ]] || fail "dirty source rebuild did not receive original arguments"
assert_contains "$dirty_output" "uncommitted changes"
assert_contains "$(cat "$dirty_make_log")" "deployctl"

explicit_bin="$TMP_ROOT/explicit-bin"
cp "$RELEASE_BINARY" "$explicit_bin"
explicit_log="$TMP_ROOT/explicit-exec.log"
env \
  PATH="$stale_bin:$BASE_PATH" DEPLOYCTL_BIN="$explicit_bin" \
  FAKE_EXEC_LOG="$explicit_log" FAKE_SOURCE_LOG="$TMP_ROOT/explicit-source.log" \
  sh "$git_checkout/scripts/install.sh" logs --follow
[[ $(cat "$explicit_log") == "logs --follow" ]] || fail "DEPLOYCTL_BIN did not remain authoritative"

non_git_path_log="$TMP_ROOT/non-git-path-exec.log"
env \
  PATH="$stale_bin:$BASE_PATH" FAKE_PATH_COMMIT="stale-commit" FAKE_PATH_EXEC_LOG="$non_git_path_log" \
  sh "$CHECKOUT/scripts/install.sh" status
[[ $(cat "$non_git_path_log") == "status" ]] || fail "non-Git checkout did not preserve PATH-first behavior"

success_install="$TMP_ROOT/success bin"
success_log="$TMP_ROOT/success-exec.log"
success_source_log="$TMP_ROOT/success-source.log"
success_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$success_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$success_log" FAKE_SOURCE_LOG="$success_source_log" \
  sh "$CHECKOUT/scripts/install.sh" version --json 2>&1)
[[ -x "$success_install/deployctl" ]] || fail "verified release was not installed persistently"
[[ $(cat "$success_log") == "version --json" ]] || fail "installed deployctl did not receive original arguments"
[[ $(cat "$success_source_log") == "$CHECKOUT" ]] || fail "installed deployctl did not receive the complete source checkout path"
assert_contains "$success_output" "$success_install/deployctl"

fallback_install="$TMP_ROOT/fallback bin"
fallback_log="$TMP_ROOT/fallback-exec.log"
fallback_source_log="$TMP_ROOT/fallback-source.log"
make_log="$TMP_ROOT/make.log"
fallback_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$fallback_install" \
  DEPLOYCTL_DOWNLOAD_URL="https://downloads.example.test/deployctl?token=wrapper-query-secret" \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$fallback_log" FAKE_SOURCE_LOG="$fallback_source_log" FAKE_MAKE_LOG="$make_log" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
[[ -x "$fallback_install/deployctl" ]] || fail "release failure did not install a local build"
[[ $(cat "$fallback_log") == "version" ]] || fail "local build did not receive original arguments"
[[ $(cat "$fallback_source_log") == "$CHECKOUT" ]] || fail "local build did not receive the complete source checkout path"
assert_contains "$fallback_output" "falling back to a local source build"
[[ "$fallback_output" != *"wrapper-query-secret"* ]] || fail "release failure leaked a signed download URL query"
assert_contains "$(cat "$make_log")" "deployctl"
assert_contains "$(cat "$make_log")" "DEPLOYCTL_OUTPUT="

mismatch_install="$TMP_ROOT/mismatch-bin"
mkdir -p "$mismatch_install"
printf 'known-good\n' > "$mismatch_install/deployctl"
chmod +x "$mismatch_install/deployctl"
mismatch_make_log="$TMP_ROOT/mismatch-make.log"
set +e
mismatch_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$mismatch_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$(printf '0%.0s' {1..64})" \
  FAKE_EXEC_LOG="$TMP_ROOT/mismatch-exec.log" FAKE_MAKE_LOG="$mismatch_make_log" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
mismatch_status=$?
set -e
[[ $mismatch_status -ne 0 ]] || fail "checksum mismatch unexpectedly succeeded"
[[ $(cat "$mismatch_install/deployctl") == "known-good" ]] || fail "checksum mismatch replaced the known-good deployctl"
[[ ! -e "$mismatch_make_log" ]] || fail "checksum mismatch incorrectly invoked the local build fallback"
assert_contains "$mismatch_output" "checksum verification failed"

incomplete="$TMP_ROOT/incomplete"
mkdir -p "$incomplete/scripts"
cp "$ROOT/scripts/install.sh" "$incomplete/scripts/install.sh"
set +e
incomplete_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$TMP_ROOT/incomplete-bin" \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" \
  sh "$incomplete/scripts/install.sh" version 2>&1)
incomplete_status=$?
set -e
[[ $incomplete_status -ne 0 ]] || fail "incomplete source checkout unexpectedly succeeded"
for missing in go.mod Makefile cmd/deployctl; do
  assert_contains "$incomplete_output" "$missing"
done

set +e
missing_go_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$TMP_ROOT/missing-go-bin" GO=missing-go \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
missing_go_status=$?
missing_make_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$TMP_ROOT/missing-make-bin" MAKE=missing-make \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
missing_make_status=$?
set -e
[[ $missing_go_status -ne 0 ]] || fail "missing Go unexpectedly succeeded"
[[ $missing_make_status -ne 0 ]] || fail "missing Make unexpectedly succeeded"
assert_contains "$missing_go_output" "Go"
assert_contains "$missing_make_output" "Make"
assert_contains "$missing_make_output" "DEPLOYCTL_BIN"
assert_contains "$(cat "$ROOT/scripts/install.ps1")" 'DEPLOYCTL_SOURCE_DIR'
assert_contains "$(cat "$ROOT/scripts/install.ps1")" 'ConvertFrom-Json'
assert_contains "$(cat "$ROOT/scripts/install.ps1")" 'PATH deployctl is stale'

echo "OK: deployctl install wrapper fallback contract verified"

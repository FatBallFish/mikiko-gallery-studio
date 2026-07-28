#!/usr/bin/env bash
set -euo pipefail

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

success_install="$TMP_ROOT/success bin"
success_log="$TMP_ROOT/success-exec.log"
success_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$success_install" \
  FAKE_CURL_MODE=success FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$success_log" \
  sh "$CHECKOUT/scripts/install.sh" version --json 2>&1)
[[ -x "$success_install/deployctl" ]] || fail "verified release was not installed persistently"
[[ $(cat "$success_log") == "version --json" ]] || fail "installed deployctl did not receive original arguments"
assert_contains "$success_output" "$success_install/deployctl"

fallback_install="$TMP_ROOT/fallback bin"
fallback_log="$TMP_ROOT/fallback-exec.log"
make_log="$TMP_ROOT/make.log"
fallback_output=$(env \
  PATH="$BASE_PATH" HOME="$TMP_ROOT/home" DEPLOYCTL_INSTALL_DIR="$fallback_install" \
  DEPLOYCTL_DOWNLOAD_URL="https://downloads.example.test/deployctl?token=wrapper-query-secret" \
  FAKE_CURL_MODE=unavailable FAKE_RELEASE_BINARY="$RELEASE_BINARY" FAKE_RELEASE_SHA="$RELEASE_SHA" FAKE_EXEC_LOG="$fallback_log" FAKE_MAKE_LOG="$make_log" \
  sh "$CHECKOUT/scripts/install.sh" version 2>&1)
[[ -x "$fallback_install/deployctl" ]] || fail "release failure did not install a local build"
[[ $(cat "$fallback_log") == "version" ]] || fail "local build did not receive original arguments"
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

echo "OK: deployctl install wrapper fallback contract verified"

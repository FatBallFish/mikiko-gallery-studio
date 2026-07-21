#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pic-gallery-service-config.XXXXXX")"
TMP_DIR="$(cd "$TMP_DIR" && pwd)"
SPECIAL_ROOT="$TMP_DIR/project space & < \"double\" 'single' % back\\slash \$dollar"
SPECIAL_HOME="$TMP_DIR/home space & < \"double\" 'single' % back\\slash \$dollar"
SPECIAL_ENV="$SPECIAL_ROOT/config/runtime space & < \"double\" 'single' % back\\slash \$dollar.env"
STUB_BIN="$TMP_DIR/stubs"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$SPECIAL_ROOT/scripts/service" "$SPECIAL_ROOT/deployments/devops/bin" \
  "$(dirname "$SPECIAL_ENV")" "$SPECIAL_HOME" "$STUB_BIN"
cp "$ROOT_DIR/scripts/service/manage.sh" "$SPECIAL_ROOT/scripts/service/manage.sh"
cp "$ROOT_DIR/deployments/devops/run-api-server.sh" "$SPECIAL_ROOT/deployments/devops/run-api-server.sh"
cp "$ROOT_DIR/deployments/devops/run-worker.sh" "$SPECIAL_ROOT/deployments/devops/run-worker.sh"
printf 'SETUP_COMPLETED=true\n' >"$SPECIAL_ENV"
printf '#!/usr/bin/env sh\nexit 0\n' >"$SPECIAL_ROOT/deployments/devops/bin/pic-gallery-api"
printf '#!/usr/bin/env sh\nexit 0\n' >"$SPECIAL_ROOT/deployments/devops/bin/pic-gallery-worker"
chmod +x "$SPECIAL_ROOT/scripts/service/manage.sh" \
  "$SPECIAL_ROOT/deployments/devops/run-api-server.sh" \
  "$SPECIAL_ROOT/deployments/devops/run-worker.sh" \
  "$SPECIAL_ROOT/deployments/devops/bin/pic-gallery-api" \
  "$SPECIAL_ROOT/deployments/devops/bin/pic-gallery-worker"

cat >"$STUB_BIN/go" <<'SH'
#!/usr/bin/env sh
set -eu
output=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    output=$2
    break
  fi
  shift
done
[ -n "$output" ]
mkdir -p "$(dirname "$output")"
printf '#!/usr/bin/env sh\nexit 0\n' >"$output"
chmod +x "$output"
SH
cat >"$STUB_BIN/systemctl" <<'SH'
#!/usr/bin/env sh
exit 0
SH
cat >"$STUB_BIN/launchctl" <<'SH'
#!/usr/bin/env sh
exit 0
SH
cat >"$STUB_BIN/uname" <<'SH'
#!/usr/bin/env sh
printf '%s\n' "${FAKE_UNAME:?}"
SH
cat >"$STUB_BIN/id" <<'SH'
#!/usr/bin/env sh
if [ "${1:-}" = "-u" ]; then
  printf '0\n'
  exit 0
fi
exec /usr/bin/id "$@"
SH
cat >"$STUB_BIN/install" <<'SH'
#!/usr/bin/env sh
set -eu
[ "$1" = "-m" ]
cp "$3" "${CAPTURE_UNIT:?}"
SH
chmod +x "$STUB_BIN"/*

assert_systemd_unit() {
  local unit=$1
  local expected_root=$2
  local expected_env=$3
  local expected_executable=$4
  if command -v systemd-analyze >/dev/null 2>&1; then
    systemd-analyze verify "$unit"
  fi
  python3 - "$unit" "$expected_root" "$expected_env" "$expected_executable" <<'PY'
import shlex
import sys

unit_path, expected_root, expected_env, expected_executable = sys.argv[1:]
directives = {}
with open(unit_path, encoding="utf-8") as handle:
    for raw_line in handle:
        line = raw_line.rstrip("\n")
        if not line or line.startswith("[") or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        directives[key] = value

def decode_one(key):
    raw = directives[key]
    if not (raw.startswith('"') and raw.endswith('"')):
        raise SystemExit(f"{key} must use a quoted systemd token: {raw!r}")
    try:
        values = shlex.split(raw, posix=True)
    except ValueError as exc:
        raise SystemExit(f"{key} is not valid quoted systemd syntax: {exc}") from exc
    if len(values) != 1:
        raise SystemExit(f"{key} decoded to {len(values)} values: {values!r}")
    value = values[0].replace("%%", "%")
    if key == "ExecStart":
        value = value.replace("$$", "$")
    return value

actual = {
    "WorkingDirectory": decode_one("WorkingDirectory"),
    "Environment": decode_one("Environment"),
    "ExecStart": decode_one("ExecStart"),
}
expected = {
    "WorkingDirectory": expected_root,
    "Environment": f"APP_ENV_FILE={expected_env}",
    "ExecStart": expected_executable,
}
if actual != expected:
    raise SystemExit(f"systemd values changed during rendering: {actual!r} != {expected!r}")
for key, expected_value in expected.items():
    raw = directives[key]
    if "%" in expected_value and "%%" not in raw:
        raise SystemExit(f"{key} did not escape systemd percent specifiers: {raw!r}")
    if "\\" in expected_value and "\\\\" not in raw:
        raise SystemExit(f"{key} did not escape backslashes: {raw!r}")
if "$" in expected_executable and "$$" not in directives["ExecStart"]:
    raise SystemExit(f"ExecStart did not escape systemd dollar expansion: {directives['ExecStart']!r}")
PY
}

PATH="$STUB_BIN:$PATH" HOME="$SPECIAL_HOME" FAKE_UNAME=Linux \
  "$SPECIAL_ROOT/scripts/service/manage.sh" install --components api --user --env-file "$SPECIAL_ENV"
assert_systemd_unit \
  "$SPECIAL_HOME/.config/systemd/user/pic-gallery-api.service" \
  "$SPECIAL_ROOT" "$SPECIAL_ENV" \
  "$SPECIAL_ROOT/target/local/bin/pic-gallery-api"

rm -rf "$SPECIAL_HOME/Library"
PATH="$STUB_BIN:$PATH" HOME="$SPECIAL_HOME" FAKE_UNAME=Darwin \
  "$SPECIAL_ROOT/scripts/service/manage.sh" install --components worker --user --env-file "$SPECIAL_ENV"
python3 - \
  "$SPECIAL_HOME/Library/LaunchAgents/com.picgallery.worker.plist" \
  "$SPECIAL_ROOT" "$SPECIAL_ENV" \
  "$SPECIAL_ROOT/target/local/bin/pic-gallery-worker" <<'PY'
import plistlib
import sys

plist_path, expected_root, expected_env, expected_executable = sys.argv[1:]
with open(plist_path, "rb") as handle:
    values = plistlib.load(handle)
actual = {
    "WorkingDirectory": values["WorkingDirectory"],
    "APP_ENV_FILE": values["EnvironmentVariables"]["APP_ENV_FILE"],
    "ProgramArguments": values["ProgramArguments"],
    "StandardOutPath": values["StandardOutPath"],
    "StandardErrorPath": values["StandardErrorPath"],
}
expected = {
    "WorkingDirectory": expected_root,
    "APP_ENV_FILE": expected_env,
    "ProgramArguments": [expected_executable],
    "StandardOutPath": f"{expected_root}/tmp/worker.out.log",
    "StandardErrorPath": f"{expected_root}/tmp/worker.err.log",
}
if actual != expected:
    raise SystemExit(f"launchd values changed during rendering: {actual!r} != {expected!r}")
PY

for component in api worker; do
  capture="$TMP_DIR/devops-$component.service"
  script="$SPECIAL_ROOT/deployments/devops/run-$component-server.sh"
  executable="$SPECIAL_ROOT/deployments/devops/bin/pic-gallery-$component"
  if [[ "$component" == worker ]]; then
    script="$SPECIAL_ROOT/deployments/devops/run-worker.sh"
  fi
  PATH="$STUB_BIN:$PATH" APP_ENV_FILE="$SPECIAL_ENV" CAPTURE_UNIT="$capture" "$script"
  assert_systemd_unit "$capture" \
    "$SPECIAL_ROOT/deployments/devops" "$SPECIAL_ENV" "$executable"
done

CR_ENV="$SPECIAL_ROOT/config/runtime-carriage"$'\r'"return.env"
printf 'SETUP_COMPLETED=true\n' >"$CR_ENV"
if PATH="$STUB_BIN:$PATH" HOME="$SPECIAL_HOME" FAKE_UNAME=Linux \
  "$SPECIAL_ROOT/scripts/service/manage.sh" install --components api --user --env-file "$CR_ENV" \
  >"$TMP_DIR/manage-cr.out" 2>&1; then
  echo "manage.sh accepted a carriage return in the runtime env path" >&2
  exit 1
fi
if PATH="$STUB_BIN:$PATH" APP_ENV_FILE="$CR_ENV" CAPTURE_UNIT="$TMP_DIR/devops-cr.service" \
  "$SPECIAL_ROOT/deployments/devops/run-api-server.sh" >"$TMP_DIR/devops-cr.out" 2>&1; then
  echo "devops systemd renderer accepted a carriage return in the runtime env path" >&2
  exit 1
fi
grep -Fq 'systemd values must not contain line breaks' "$TMP_DIR/manage-cr.out"
grep -Fq 'systemd values must not contain line breaks' "$TMP_DIR/devops-cr.out"

for control_name in carriage-return line-feed; do
  if [[ "$control_name" == carriage-return ]]; then
    CONTROL_ENV="$SPECIAL_ROOT/config/launchd"$'\r'"runtime.env"
  else
    CONTROL_ENV="$SPECIAL_ROOT/config/launchd"$'\n'"runtime.env"
  fi
  output="$TMP_DIR/launchd-$control_name.out"
  if PATH="$STUB_BIN:$PATH" HOME="$SPECIAL_HOME" FAKE_UNAME=Darwin \
    "$SPECIAL_ROOT/scripts/service/manage.sh" install --components api --user --env-file "$CONTROL_ENV" \
    >"$output" 2>&1; then
    echo "manage.sh launchd renderer accepted $control_name in the runtime env path" >&2
    exit 1
  fi
  grep -Fq 'launchd XML values must not contain line breaks' "$output"
done

POWERSHELL_SOURCE="$ROOT_DIR/scripts/service/manage.ps1"
WINDOWS_ROOT="C:\\Program Files\\Pic & Gallery's % \\\$root"
WINDOWS_ENV="C:\\Program Data\\Pic & Gallery's % \\\$env\\runtime.env"
WINDOWS_EXE="C:\\Program Files\\Pic & Gallery's % \\\$bin\\pic-gallery-api.exe"
if command -v pwsh >/dev/null 2>&1; then
  ENCODED_PAYLOAD="$(pwsh -NoProfile -File "$POWERSHELL_SOURCE" \
    -RenderCommandPayload \
    -PayloadRoot "$WINDOWS_ROOT" \
    -PayloadEnvFile "$WINDOWS_ENV" \
    -PayloadExecutable "$WINDOWS_EXE")"
  for control_name in carriage-return line-feed; do
    if [[ "$control_name" == carriage-return ]]; then
      CONTROL_VALUE="$WINDOWS_ENV"$'\r'"suffix"
    else
      CONTROL_VALUE="$WINDOWS_ENV"$'\n'"suffix"
    fi
    if pwsh -NoProfile -File "$POWERSHELL_SOURCE" \
      -RenderCommandPayload \
      -PayloadRoot "$WINDOWS_ROOT" \
      -PayloadEnvFile "$CONTROL_VALUE" \
      -PayloadExecutable "$WINDOWS_EXE" >"$TMP_DIR/pwsh-$control_name.out" 2>&1; then
      echo "PowerShell payload renderer accepted $control_name" >&2
      exit 1
    fi
  done
else
  ENCODED_PAYLOAD="$(python3 - "$POWERSHELL_SOURCE" "$WINDOWS_ROOT" "$WINDOWS_ENV" "$WINDOWS_EXE" <<'PY'
import base64
import sys

source_path, root, env_file, executable = sys.argv[1:]
source = open(source_path, encoding="utf-8").read()
required_source = [
    "function New-ServiceCommandPayload",
    'if ($Value.Contains("`r") -or $Value.Contains("`n"))',
    'return "\'" + $Value.Replace("\'", "\'\'") + "\'"',
    '$Command = "Set-Location -LiteralPath $RootLiteral; `$env:APP_ENV_FILE = $EnvFileLiteral; & $ExeLiteral"',
    '[Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($Command))',
    '-EncodedCommand $($Payload.EncodedCommand)',
]
for required in required_source:
    if required not in source:
        raise SystemExit(f"PowerShell payload implementation is missing: {required}")
if "-Command $Command" in source:
    raise SystemExit("PowerShell service action still reparses paths through -Command")

def literal(value):
    if "\r" in value or "\n" in value:
        raise ValueError("line break")
    return "'" + value.replace("'", "''") + "'"

def payload(root_value, env_value, executable_value):
    command = (
        f"Set-Location -LiteralPath {literal(root_value)}; "
        f"$env:APP_ENV_FILE = {literal(env_value)}; & {literal(executable_value)}"
    )
    return base64.b64encode(command.encode("utf-16le")).decode("ascii")

for invalid in (env_file + "\rcontrol", env_file + "\ncontrol"):
    try:
        payload(root, invalid, executable)
    except ValueError:
        pass
    else:
        raise SystemExit("independent PowerShell payload renderer accepted a line break")
print(payload(root, env_file, executable))
PY
)"
fi

python3 - "$ENCODED_PAYLOAD" "$WINDOWS_ROOT" "$WINDOWS_ENV" "$WINDOWS_EXE" <<'PY'
import base64
import re
import sys

encoded, expected_root, expected_env, expected_executable = sys.argv[1:]
try:
    payload = base64.b64decode(encoded, validate=True).decode("utf-16le")
except Exception as exc:
    raise SystemExit(f"PowerShell payload is not UTF-16LE Base64: {exc}") from exc
match = re.fullmatch(
    r"Set-Location -LiteralPath ('(?:[^']|'')*'); "
    r"\$env:APP_ENV_FILE = ('(?:[^']|'')*'); & ('(?:[^']|'')*')",
    payload,
)
if not match:
    raise SystemExit(f"unexpected PowerShell payload syntax: {payload!r}")

def decode_literal(value):
    return value[1:-1].replace("''", "'")

actual = tuple(decode_literal(value) for value in match.groups())
expected = (expected_root, expected_env, expected_executable)
if actual != expected:
    raise SystemExit(f"PowerShell payload changed path values: {actual!r} != {expected!r}")
PY

echo "OK: service configuration escaping contract passed"

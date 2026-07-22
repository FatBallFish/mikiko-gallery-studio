param(
  [Parameter(Mandatory=$true, Position=0)]
  [ValidateSet("install", "uninstall", "restart", "status", "doctor")]
  [string]$Action,
  [string]$RuntimeDir = ".",
  [Parameter(ValueFromRemainingArguments=$true)]
  [string[]]$DeployctlArgs
)

$ErrorActionPreference = "Stop"
$InstallScript = Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path "install.ps1"
$Arguments = @($Action, "--runtime-dir", $RuntimeDir) + $DeployctlArgs

if ($env:DEPLOYCTL_BIN) {
  & $env:DEPLOYCTL_BIN @Arguments
} else {
  & $InstallScript @Arguments
}
exit $LASTEXITCODE

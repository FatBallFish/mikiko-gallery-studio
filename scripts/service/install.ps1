param(
  [string]$Components = "api,worker",
  [string]$EnvFile = ""
)
$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$ArgsList = @("install", "-Components", $Components)
if ($EnvFile) { $ArgsList += @("-EnvFile", $EnvFile) }
& (Join-Path $Root "scripts/service/manage.ps1") @ArgsList

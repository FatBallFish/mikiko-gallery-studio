param(
  [string]$Components = "api,worker"
)
$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
& (Join-Path $Root "scripts/service/manage.ps1") uninstall -Components $Components

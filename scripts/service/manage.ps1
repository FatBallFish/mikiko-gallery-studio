param(
  [Parameter(Mandatory=$false)]
  [ValidateSet("install", "uninstall", "start", "stop", "restart", "status", "logs")]
  [string]$Action,
  [string]$Components = "api,worker",
  [string]$EnvFile = "",
  [switch]$RenderCommandPayload,
  [string]$PayloadRoot = "",
  [string]$PayloadEnvFile = "",
  [string]$PayloadExecutable = ""
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
if (-not $EnvFile) {
  $EnvFile = Join-Path $Root "config/runtime.env"
}

function Build-Component {
  param([string]$Component)
  $BinDir = Join-Path $Root "target/local/bin"
  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  switch ($Component) {
    "api" { & go build -o (Join-Path $BinDir "pic-gallery-api.exe") ./cmd/api }
    "worker" { & go build -o (Join-Path $BinDir "pic-gallery-worker.exe") ./cmd/worker }
    default { throw "Unknown component: $Component" }
  }
}

function Component-Command {
  param([string]$Component)
  switch ($Component) {
    "api" { return (Join-Path $Root "target/local/bin/pic-gallery-api.exe") }
    "worker" { return (Join-Path $Root "target/local/bin/pic-gallery-worker.exe") }
    default { throw "Unknown component: $Component" }
  }
}

function Task-Name {
  param([string]$Component)
  return "PicGallery-$Component"
}

function ConvertTo-SingleQuotedLiteral {
  param([AllowEmptyString()][string]$Value)
  if ($Value.Contains("`r") -or $Value.Contains("`n")) {
    throw "PowerShell command payload values must not contain line breaks"
  }
  return "'" + $Value.Replace("'", "''") + "'"
}

function New-ServiceCommandPayload {
  param(
    [AllowEmptyString()][string]$RootPath,
    [AllowEmptyString()][string]$RuntimeEnvFile,
    [AllowEmptyString()][string]$ExecutablePath
  )
  $RootLiteral = ConvertTo-SingleQuotedLiteral $RootPath
  $EnvFileLiteral = ConvertTo-SingleQuotedLiteral $RuntimeEnvFile
  $ExeLiteral = ConvertTo-SingleQuotedLiteral $ExecutablePath
  $Command = "Set-Location -LiteralPath $RootLiteral; `$env:APP_ENV_FILE = $EnvFileLiteral; & $ExeLiteral"
  return [PSCustomObject]@{
    Command = $Command
    EncodedCommand = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($Command))
  }
}

if ($RenderCommandPayload) {
  $Payload = New-ServiceCommandPayload $PayloadRoot $PayloadEnvFile $PayloadExecutable
  Write-Output $Payload.EncodedCommand
  exit 0
}
if (-not $Action) {
  throw "Action is required unless -RenderCommandPayload is used"
}

function Install-Component {
  param([string]$Component)
  Build-Component $Component
  $TaskName = Task-Name $Component
  $Exe = Component-Command $Component
  $Payload = New-ServiceCommandPayload $Root $EnvFile $Exe
  $TaskAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -EncodedCommand $($Payload.EncodedCommand)"
  $Trigger = New-ScheduledTaskTrigger -AtStartup
  $Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel LeastPrivilege
  $Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
  Register-ScheduledTask -TaskName $TaskName -Action $TaskAction -Trigger $Trigger -Principal $Principal -Settings $Settings -Force | Out-Null
  Start-ScheduledTask -TaskName $TaskName
}

function Manage-Component {
  param([string]$Component)
  $TaskName = Task-Name $Component
  switch ($Action) {
    "install" { Install-Component $Component }
    "uninstall" {
      Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
      Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
    }
    "start" { Start-ScheduledTask -TaskName $TaskName }
    "stop" { Stop-ScheduledTask -TaskName $TaskName }
    "restart" {
      Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
      Start-ScheduledTask -TaskName $TaskName
    }
    "status" { Get-ScheduledTask -TaskName $TaskName | Format-List * }
    "logs" {
      Write-Host "Windows scheduled-task mode does not capture logs by default. Run target/local/bin/pic-gallery-$Component.exe with APP_ENV_FILE=$EnvFile for foreground logs."
    }
  }
}

Set-Location $Root
$Components.Split(",") | ForEach-Object {
  $Component = $_.Trim()
  if ($Component) {
    Manage-Component $Component
  }
}

param(
  [Parameter(Mandatory=$true)]
  [ValidateSet("install", "uninstall", "start", "stop", "restart", "status", "logs")]
  [string]$Action,
  [string]$Components = "api,worker",
  [string]$EnvFile = ""
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

function Install-Component {
  param([string]$Component)
  Build-Component $Component
  $TaskName = Task-Name $Component
  $Exe = Component-Command $Component
  $Command = "cd `"$Root`"; `$env:APP_ENV_FILE=`"$EnvFile`"; & `"$Exe`""
  $TaskAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -Command $Command"
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

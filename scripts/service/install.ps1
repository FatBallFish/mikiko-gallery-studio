param(
  [string]$Components = "api,worker,user-web,admin-web",
  [string]$AppConfigPath = "",
  [string]$UserWebPort = "5173",
  [string]$AdminWebPort = "5174",
  [string]$ApiProxyTarget = "http://127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
if (-not $AppConfigPath) {
  $AppConfigPath = Join-Path $Root "configs/config.dev.yaml"
}

function Install-Component {
  param([string]$Component)
  $TaskName = "PicGallery-$Component"
  switch ($Component) {
    "api" {
      $Command = "cd `"$Root`"; `$env:APP_CONFIG_PATH=`"$AppConfigPath`"; go run ./cmd/api"
    }
    "worker" {
      $Command = "cd `"$Root`"; `$env:APP_CONFIG_PATH=`"$AppConfigPath`"; go run ./cmd/worker"
    }
    "user-web" {
      $Command = "cd `"$Root`"; `$env:VITE_API_PROXY_TARGET=`"$ApiProxyTarget`"; `$env:USER_WEB_PORT=`"$UserWebPort`"; npm --prefix web/user run dev -- --host 0.0.0.0 --port $UserWebPort"
    }
    "admin-web" {
      $Command = "cd `"$Root`"; `$env:VITE_API_PROXY_TARGET=`"$ApiProxyTarget`"; `$env:ADMIN_WEB_PORT=`"$AdminWebPort`"; npm --prefix web/admin run dev -- --host 0.0.0.0 --port $AdminWebPort"
    }
    default {
      throw "Unknown component: $Component"
    }
  }

  $Action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -Command $Command"
  $Trigger = New-ScheduledTaskTrigger -AtStartup
  $Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel LeastPrivilege
  $Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
  Register-ScheduledTask -TaskName $TaskName -Action $Action -Trigger $Trigger -Principal $Principal -Settings $Settings -Force | Out-Null
  Start-ScheduledTask -TaskName $TaskName
}

$Components.Split(",") | ForEach-Object {
  $Component = $_.Trim()
  if ($Component) {
    Install-Component $Component
  }
}

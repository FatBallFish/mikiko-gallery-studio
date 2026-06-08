param(
  [string]$Components = "api,worker,user-web,admin-web"
)

$ErrorActionPreference = "Stop"

$Components.Split(",") | ForEach-Object {
  $Component = $_.Trim()
  if (-not $Component) {
    return
  }
  $TaskName = "PicGallery-$Component"
  Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
  Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
}

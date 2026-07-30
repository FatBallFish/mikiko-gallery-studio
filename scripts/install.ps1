$ErrorActionPreference = "Stop"

$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$sourceRequirements = @("go.mod", "Makefile", "cmd/mgsctl", "Dockerfile.api", "Dockerfile.worker", "Dockerfile.user-web", "Dockerfile.admin-web", "Dockerfile.docs-web")
$sourceComplete = @($sourceRequirements | Where-Object { -not (Test-Path -LiteralPath (Join-Path $root $_)) }).Count -eq 0
if (-not $env:MGSCTL_SOURCE_DIR -and $sourceComplete) {
  $env:MGSCTL_SOURCE_DIR = $root
}

$binary = $env:MGSCTL_BIN
$forceLocalBuild = $false
if (-not $binary) {
  $command = Get-Command mgsctl -ErrorAction SilentlyContinue
  if ($command) {
    $binary = $command.Source
    $gitCommand = if ($sourceComplete) { Get-Command git -ErrorAction SilentlyContinue } else { $null }
    if ($gitCommand) {
      $sourcePrefixOutput = @(& $gitCommand.Source -C $root rev-parse --show-prefix 2>$null)
      $prefixStatus = $LASTEXITCODE
      $sourceCommitOutput = @(& $gitCommand.Source -C $root rev-parse HEAD 2>$null)
      $commitStatus = $LASTEXITCODE
      $sourcePrefix = ($sourcePrefixOutput -join "").Trim()
      $sourceCommit = ($sourceCommitOutput -join "").Trim()
      if ($prefixStatus -eq 0 -and $commitStatus -eq 0 -and -not $sourcePrefix -and $sourceCommit) {
        $sourceChanges = @(& $gitCommand.Source -C $root status --porcelain --untracked-files=normal 2>$null)
        if ($sourceChanges.Count -gt 0) {
          Write-Warning "Source checkout has uncommitted changes; selecting a local source build instead of the PATH mgsctl."
          $forceLocalBuild = $true
        } else {
          $pathCommit = $null
          try {
            $pathMetadataOutput = @(& $binary version --json 2>$null)
            if ($LASTEXITCODE -eq 0) {
              $pathMetadata = (($pathMetadataOutput -join "`n") | ConvertFrom-Json)
              $pathCommit = [string]$pathMetadata.commit
            }
          } catch {
            $pathCommit = $null
          }
          if ($pathCommit -ne $sourceCommit) {
            if ($pathCommit) {
              Write-Warning "PATH mgsctl is stale for this source checkout (tool commit $pathCommit, checkout $sourceCommit); selecting a local source build."
            } else {
              Write-Warning "PATH mgsctl build metadata is unavailable; selecting a local source build for this checkout."
            }
            $forceLocalBuild = $true
          }
        }
      }
    }
  }
}

if (-not $binary -or $forceLocalBuild) {
  $version = if ($env:MGSCTL_VERSION) { $env:MGSCTL_VERSION } else { "latest" }
  $releaseBase = if ($env:MGSCTL_RELEASE_BASE_URL) { $env:MGSCTL_RELEASE_BASE_URL.TrimEnd("/") } else { "https://github.com/fatballfish/mikiko-gallery-studio/releases" }
  $localAppData = if ($env:LOCALAPPDATA) { $env:LOCALAPPDATA } else { Join-Path $HOME "AppData\Local" }
  $installDirectory = if ($env:MGSCTL_INSTALL_DIR) { $env:MGSCTL_INSTALL_DIR } else { Join-Path $localAppData "Programs\mgsctl" }
  $goCommandName = if ($env:GO) { $env:GO } else { "go" }
  $makeCommandName = if ($env:MAKE) { $env:MAKE } else { "make" }
  $architecture = switch ($env:PROCESSOR_ARCHITECTURE.ToUpperInvariant()) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Unsupported architecture; set MGSCTL_BIN" }
  }
  $releasePath = if ($version -eq "latest") { "latest/download" } else { "download/$version" }
  $artifact = "mgsctl-windows-$architecture.exe"
  $url = if ($env:MGSCTL_DOWNLOAD_URL) { $env:MGSCTL_DOWNLOAD_URL } else { "$releaseBase/$releasePath/$artifact" }
  $displayUrl = ([Uri]$url).GetLeftPart([UriPartial]::Path)
  $temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("mgsctl-" + [Guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null

  function Install-MGSCTLCandidate([string]$Candidate) {
    New-Item -ItemType Directory -Force -Path $installDirectory | Out-Null
    $staged = Join-Path $installDirectory (".mgsctl.install." + [Guid]::NewGuid().ToString("N") + ".exe")
    try {
      Copy-Item -LiteralPath $Candidate -Destination $staged
      Move-Item -LiteralPath $staged -Destination (Join-Path $installDirectory "mgsctl.exe") -Force
    } catch {
      Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
      throw "Cannot replace mgsctl in install directory ${installDirectory}: $_"
    }
    return (Join-Path $installDirectory "mgsctl.exe")
  }

  function Build-LocalMGSCTL {
    $required = @("go.mod", "Makefile", "cmd/mgsctl")
    $missing = @($required | Where-Object { -not (Test-Path -LiteralPath (Join-Path $root $_)) })
    if ($missing.Count -gt 0) {
      throw "Local mgsctl build requires a complete source checkout; missing: $($missing -join ', '). Provide a trusted prebuilt binary with MGSCTL_BIN."
    }
    $goCommand = Get-Command $goCommandName -ErrorAction SilentlyContinue
    if (-not $goCommand) {
      throw "Local mgsctl build requires Go ($goCommandName was not found); install Go or set MGSCTL_BIN."
    }
    $makeCommand = Get-Command $makeCommandName -ErrorAction SilentlyContinue
    if (-not $makeCommand) {
      throw "Local mgsctl build requires Make ($makeCommandName was not found); install Make or set MGSCTL_BIN."
    }
    $localBinary = Join-Path $temporaryDirectory "mgsctl-local.exe"
    $makeOutput = @(& $makeCommand.Source -C $root mgsctl "MGSCTL_OUTPUT=$localBinary" "GO=$($goCommand.Source)" 2>&1)
    $makeStatus = $LASTEXITCODE
    $makeOutput | ForEach-Object { Write-Host $_ }
    if ($makeStatus -ne 0 -or -not (Test-Path -LiteralPath $localBinary -PathType Leaf)) {
      throw "Local mgsctl build failed; install Go and Make or set MGSCTL_BIN."
    }
    return $localBinary
  }

  try {
    if ($forceLocalBuild) {
      $candidate = Build-LocalMGSCTL
    } else {
      $downloadedBinary = Join-Path $temporaryDirectory $artifact
      $downloadFailure = $null
      try {
        Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $downloadedBinary
        if ($env:MGSCTL_SHA256) {
          $expectedSha256 = $env:MGSCTL_SHA256.Trim()
          if ($expectedSha256 -notmatch '^[0-9a-fA-F]{64}$') {
            throw "MGSCTL_SHA256 must contain exactly 64 hexadecimal characters"
          }
        } else {
          $checksumFile = "$downloadedBinary.sha256"
          Invoke-WebRequest -UseBasicParsing -Uri "$url.sha256" -OutFile $checksumFile
          $expectedSha256 = ((Get-Content -Raw $checksumFile).Trim() -split "\s+")[0]
          if ($expectedSha256 -notmatch '^[0-9a-fA-F]{64}$') {
            $downloadFailure = "mgsctl release checksum file is incomplete"
          }
        }
      } catch {
        if ($_.Exception.Message -like "MGSCTL_SHA256*") { throw }
        $downloadFailure = "release download failed for $displayUrl"
      }

      if ($downloadFailure) {
        Write-Warning "Release artifact could not be verified ($downloadFailure); falling back to a local source build."
        $candidate = Build-LocalMGSCTL
      } else {
        $actualSha256 = (Get-FileHash -Algorithm SHA256 -Path $downloadedBinary).Hash
        if (-not $actualSha256.Equals($expectedSha256, [StringComparison]::OrdinalIgnoreCase)) {
          throw "mgsctl checksum verification failed; refusing local build fallback"
        }
        $candidate = $downloadedBinary
      }
    }

    $binary = Install-MGSCTLCandidate $candidate
    Write-Output "Installed mgsctl: $binary"
    $pathEntries = @($env:PATH -split [IO.Path]::PathSeparator)
    if ($pathEntries -notcontains $installDirectory) {
      Write-Output "Add $installDirectory to PATH to run mgsctl directly in future shells."
    }
    & $binary @args
    exit $LASTEXITCODE
  } finally {
    Remove-Item -Recurse -Force $temporaryDirectory -ErrorAction SilentlyContinue
  }
}

& $binary @args
exit $LASTEXITCODE

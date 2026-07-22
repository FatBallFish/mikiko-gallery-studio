$ErrorActionPreference = "Stop"

$binary = $env:DEPLOYCTL_BIN
if (-not $binary) {
  $command = Get-Command deployctl -ErrorAction SilentlyContinue
  if ($command) { $binary = $command.Source }
}
if (-not $binary) {
	$version = if ($env:DEPLOYCTL_VERSION) { $env:DEPLOYCTL_VERSION } else { "latest" }
	$releaseBase = if ($env:DEPLOYCTL_RELEASE_BASE_URL) { $env:DEPLOYCTL_RELEASE_BASE_URL.TrimEnd("/") } else { "https://github.com/fatballfish/pic-gallery/releases" }
	$architecture = switch ($env:PROCESSOR_ARCHITECTURE.ToUpperInvariant()) {
		"AMD64" { "amd64" }
		"ARM64" { "arm64" }
		default { throw "Unsupported architecture; set DEPLOYCTL_BIN" }
	}
	$releasePath = if ($version -eq "latest") { "latest/download" } else { "download/$version" }
	$artifact = "deployctl-windows-$architecture.exe"
	$url = if ($env:DEPLOYCTL_DOWNLOAD_URL) { $env:DEPLOYCTL_DOWNLOAD_URL } else { "$releaseBase/$releasePath/$artifact" }
	$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("deployctl-" + [Guid]::NewGuid().ToString("N"))
	New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
	$binary = Join-Path $temporaryDirectory $artifact
	try {
		Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $binary
		if ($env:DEPLOYCTL_SHA256) {
			$expectedSha256 = $env:DEPLOYCTL_SHA256.Trim()
		} else {
			$checksumFile = "$binary.sha256"
			Invoke-WebRequest -UseBasicParsing -Uri "$url.sha256" -OutFile $checksumFile
			$expectedSha256 = ((Get-Content -Raw $checksumFile).Trim() -split "\s+")[0]
		}
		$actualSha256 = (Get-FileHash -Algorithm SHA256 -Path $binary).Hash
		if (-not $actualSha256.Equals($expectedSha256, [StringComparison]::OrdinalIgnoreCase)) {
			throw "deployctl checksum verification failed"
		}
		& $binary @args
		exit $LASTEXITCODE
	} finally {
		Remove-Item -Recurse -Force $temporaryDirectory -ErrorAction SilentlyContinue
	}
}

& $binary @args
exit $LASTEXITCODE

$ErrorActionPreference = "Stop"

$Repo = if ($env:AHM_REPO) { $env:AHM_REPO } else { "travisennis/ahm" }
$Version = if ($env:AHM_VERSION) { $env:AHM_VERSION } else { "latest" }
$InstallDir = if ($env:AHM_INSTALL_DIR) { $env:AHM_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\ahm\bin" }

if ($Version -eq "latest") {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $Release.tag_name
}

if ($Version.StartsWith("v")) {
    $Tag = $Version
    $AssetVersion = $Version.Substring(1)
} else {
    $Tag = "v$Version"
    $AssetVersion = $Version
}

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$Asset = "ahm_${AssetVersion}_windows_${Arch}.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/$Tag"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $Archive = Join-Path $TempDir $Asset
    $Checksums = Join-Path $TempDir "checksums.txt"

    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $Archive
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $Checksums

    $ChecksumLine = Get-Content $Checksums | Where-Object { $_ -match "\s+$([regex]::Escape($Asset))$" } | Select-Object -First 1
    if (-not $ChecksumLine) {
        throw "checksum entry not found for $Asset"
    }

    $Expected = ($ChecksumLine -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $Archive).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) {
        throw "checksum mismatch for $Asset"
    }

    Expand-Archive -Path $Archive -DestinationPath $TempDir -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $TempDir "ahm.exe") -Destination (Join-Path $InstallDir "ahm.exe") -Force

    Write-Host "ahm $Tag installed to $(Join-Path $InstallDir "ahm.exe")"
} finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}

# gpu-tools one-line installer for Windows PowerShell / PowerShell 7+.
#
#   irm https://raw.githubusercontent.com/sunerpy/gpu-tools/main/scripts/install.ps1 | iex
#
# Overrides:
#   $env:GPU_TOOLS_INSTALL_DIR = "C:\Users\you\bin"
#   $env:GPU_TOOLS_VERSION = "v1.2.3"

param(
  [string]$Version = $env:GPU_TOOLS_VERSION,
  [string]$Dir = $env:GPU_TOOLS_INSTALL_DIR,
  [switch]$DryRun,
  [switch]$Help
)

$ErrorActionPreference = "Stop"

$Repo = "sunerpy/gpu-tools"
$Bin = "gpu-tools"
$ChecksumFile = "checksums.txt"

function Show-Help {
  @"
Install gpu-tools from GitHub Releases.

Usage:
  .\install.ps1 [-Version vX.Y.Z] [-Dir DIR] [-DryRun]

Options:
  -Version VERSION  Install a specific release tag/version, for example v1.2.3.
                    Defaults to the latest GitHub release.
  -Dir DIR          Install directory. Defaults to:
                    `$env:LOCALAPPDATA\Programs\gpu-tools
  -DryRun           Print resolved asset URLs and exit without downloading.
  -Help             Show this help.

Environment:
  GPU_TOOLS_VERSION      Same as -Version.
  GPU_TOOLS_INSTALL_DIR  Same as -Dir.
"@
}

function Fail([string]$Message) {
  Write-Error "error: $Message"
  exit 1
}

if ($Help) {
  Show-Help
  exit 0
}

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($arch) {
  "X64" { $AssetArch = "amd64" }
  "Arm64" { $AssetArch = "arm64" }
  default { Fail "unsupported architecture: $arch (supported: amd64, arm64)" }
}

$AssetOS = "windows"

if ([string]::IsNullOrWhiteSpace($Version) -or $Version -eq "latest") {
  Write-Host "Resolving latest release for $Repo..."
  $ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"
  $Release = Invoke-RestMethod -Uri $ApiUrl -Headers @{ "User-Agent" = "gpu-tools-installer" }
  $ReleaseTag = [string]$Release.tag_name
  if ([string]::IsNullOrWhiteSpace($ReleaseTag)) {
    Fail "could not resolve latest release tag from $ApiUrl"
  }
} elseif ($Version.StartsWith("v")) {
  $ReleaseTag = $Version
} else {
  $ReleaseTag = "v$Version"
}

$ArchiveVersion = $ReleaseTag -replace '^v', ''
if ([string]::IsNullOrWhiteSpace($ArchiveVersion)) {
  Fail "empty release version"
}

# Must match .goreleaser.yaml archives.name_template:
#   {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
$AssetBase = "${Bin}_${ArchiveVersion}_${AssetOS}_${AssetArch}"
$Asset = "$AssetBase.zip"
$ReleaseBaseUrl = "https://github.com/$Repo/releases/download/$ReleaseTag"
$AssetUrl = "$ReleaseBaseUrl/$Asset"
$ChecksumUrl = "$ReleaseBaseUrl/$ChecksumFile"

if ([string]::IsNullOrWhiteSpace($Dir)) {
  $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\gpu-tools"
} else {
  $InstallDir = $Dir
}

Write-Host "Installing $Bin $ReleaseTag ($AssetOS/$AssetArch)"
Write-Host "  asset:     $Asset"
Write-Host "  from:      $AssetUrl"
Write-Host "  checksum:  $ChecksumUrl"
Write-Host "  to:        $(Join-Path $InstallDir "$Bin.exe")"

if ($DryRun) {
  Write-Host "dry-run: no download or install performed"
  exit 0
}

$TempRoot = New-Item -ItemType Directory -Path (Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString()))
try {
  $ArchivePath = Join-Path $TempRoot.FullName $Asset
  $ChecksumsPath = Join-Path $TempRoot.FullName $ChecksumFile
  $ExtractDir = Join-Path $TempRoot.FullName "extract"
  New-Item -ItemType Directory -Force -Path $ExtractDir | Out-Null

  Write-Host "Downloading release archive..."
  Invoke-WebRequest -Uri $AssetUrl -OutFile $ArchivePath -Headers @{ "User-Agent" = "gpu-tools-installer" }

  Write-Host "Downloading checksums..."
  Invoke-WebRequest -Uri $ChecksumUrl -OutFile $ChecksumsPath -Headers @{ "User-Agent" = "gpu-tools-installer" }

  $ChecksumLine = Get-Content -Path $ChecksumsPath |
    Where-Object { $_ -match "\s$([Regex]::Escape($Asset))$" } |
    Select-Object -First 1
  if ([string]::IsNullOrWhiteSpace($ChecksumLine)) {
    Fail "$ChecksumFile does not contain an entry for $Asset"
  }

  $ExpectedSha = (($ChecksumLine -split '\s+')[0]).ToLowerInvariant()
  $ActualSha = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($ExpectedSha -ne $ActualSha) {
    Fail "checksum mismatch for ${Asset}: expected $ExpectedSha, got $ActualSha"
  }
  Write-Host "Checksum verified."

  Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
  $ExtractedBinary = Join-Path $ExtractDir "$Bin.exe"
  if (-not (Test-Path -LiteralPath $ExtractedBinary -PathType Leaf)) {
    Fail "archive $Asset did not contain $Bin.exe"
  }

  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Copy-Item -Force -Path $ExtractedBinary -Destination (Join-Path $InstallDir "$Bin.exe")

  Write-Host "installed to $(Join-Path $InstallDir "$Bin.exe")"

  $PathEntries = ($env:Path -split ';') | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  if ($PathEntries -notcontains $InstallDir) {
    Write-Host "NOTE: $InstallDir is not on PATH. Add it for future shells:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', '$InstallDir;' + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
  }
} finally {
  Remove-Item -Recurse -Force -LiteralPath $TempRoot.FullName -ErrorAction SilentlyContinue
}

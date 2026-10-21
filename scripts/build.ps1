# Build Lodestar on Windows: WebUI first, then the Go binary with the UI embedded.
#
#   .\scripts\build.ps1
#   $env:VERSION = "1.4.2"; .\scripts\build.ps1
param(
    [string]$Out = "lodestar.exe",
    [string]$Version = "",
    [string]$Commit = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

if (-not $Version) {
    $Version = (git describe --tags --always --dirty 2>$null)
    if (-not $Version) { $Version = "dev" }
}
if (-not $Commit) {
    $Commit = (git rev-parse --short HEAD 2>$null)
    if (-not $Commit) { $Commit = "dev" }
}

Write-Host "==> WebUI" -ForegroundColor Cyan
if (-not (Test-Path "webui\node_modules")) {
    Push-Location webui; npm install --no-audit --no-fund; Pop-Location
}
Push-Location webui; npm run build; Pop-Location

Write-Host "==> Go (version $Version, commit $Commit)" -ForegroundColor Cyan
$ldflags = "-s -w -X lodestar/internal/buildinfo.Version=$Version -X lodestar/internal/buildinfo.Commit=$Commit"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags $ldflags -o $Out ./cmd/lodestar

Write-Host "==> done: $Out" -ForegroundColor Green

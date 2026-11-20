# Build portable Windows GUI executables and a SHA-256 manifest.
[CmdletBinding()]
param([string] $OutputDirectory = "dist")

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$env:GOOS = "windows"
$env:CGO_ENABLED = "0"
$assets = @()
foreach ($arch in @("amd64", "arm64")) {
    $env:GOARCH = $arch
    $name = "LinkSwitch-windows-$arch.exe"
    $path = Join-Path $OutputDirectory $name
    go build -trimpath -buildvcs=false -ldflags "-H windowsgui -s -w" -o $path ./cmd/linkswitch
    if ($LASTEXITCODE -ne 0) { throw "Build failed: $arch" }
    $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    $assets += "$hash  $name"
}
$assets | Set-Content -Encoding ascii (Join-Path $OutputDirectory "SHA256SUMS.txt")

#!/bin/sh
# Cross-build the same portable Windows assets from Linux/macOS.
set -eu
cd "$(dirname "$0")/.."
out=${1:-dist}
goexe=${GO:-go}
mkdir -p "$out"
for arch in amd64 arm64; do
    GOOS=windows GOARCH=$arch CGO_ENABLED=0 "$goexe" build -trimpath -buildvcs=false -ldflags '-H windowsgui -s -w' -o "$out/LinkSwitch-windows-$arch.exe" ./cmd/linkswitch
done
(
    cd "$out"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum LinkSwitch-windows-amd64.exe LinkSwitch-windows-arm64.exe
    else
        shasum -a 256 LinkSwitch-windows-amd64.exe LinkSwitch-windows-arm64.exe
    fi
) > "$out/SHA256SUMS.txt"

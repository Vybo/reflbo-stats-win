#!/bin/sh
# Builds dist/reflbo-stats.exe for Windows x64 from any system with Go 1.26 or later (spec §3).
#   scripts/build.sh [version]     # the version defaults to git describe
set -eu
cd "$(dirname "$0")/.."
version="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=$version" -o dist/reflbo-stats.exe ./cmd/reflbo-stats
echo "dist/reflbo-stats.exe ($version, $(wc -c < dist/reflbo-stats.exe | tr -d ' ') bytes)"

#!/bin/bash
# Cross-compile FileShare for Windows and Linux on amd64 and arm64.
# Produces four static binaries in dist/.

set -euo pipefail
cd "$(dirname "$0")"

LDFLAGS="-s -w"
mkdir -p dist

build_one() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  local out="dist/fileserver-${goos}-${goarch}${ext}"
  echo "→ building $out"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags="$LDFLAGS" -o "$out"
}

build_one linux  amd64
build_one linux  arm64
build_one windows amd64
build_one windows arm64

echo
echo "Done:"
ls -lh dist/
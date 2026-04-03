#!/bin/bash
# Build shieldlink-server for multiple platforms
set -e

cd "$(dirname "$0")/../server"
OUTDIR="../bin/server"
mkdir -p "$OUTDIR"

GOBUILD="CGO_ENABLED=0 go build -trimpath -ldflags '-w -s' -o"

echo "Building shieldlink-server..."

GOOS=linux   GOARCH=amd64 $GOBUILD "$OUTDIR/shieldlink-server-linux-amd64"       ./cmd/
GOOS=linux   GOARCH=arm64 $GOBUILD "$OUTDIR/shieldlink-server-linux-arm64"       ./cmd/
GOOS=windows GOARCH=amd64 $GOBUILD "$OUTDIR/shieldlink-server-windows-amd64.exe" ./cmd/
GOOS=darwin  GOARCH=amd64 $GOBUILD "$OUTDIR/shieldlink-server-darwin-amd64"      ./cmd/
GOOS=darwin  GOARCH=arm64 $GOBUILD "$OUTDIR/shieldlink-server-darwin-arm64"      ./cmd/

echo "Done. Binaries in $OUTDIR/"
ls -lh "$OUTDIR/"

#!/bin/bash
# Build mihomo-shieldlink (patched mihomo with ShieldLink support)
#
# Prerequisites:
#   1. Clone mihomo: git clone https://github.com/MetaCubeX/mihomo.git
#   2. Run this script from the shieldlink project root:
#      ./scripts/build-mihomo.sh /path/to/mihomo
#
set -e

MIHOMO_DIR="${1:?Usage: $0 <mihomo-source-dir>}"
PATCH_DIR="$(cd "$(dirname "$0")/../mihomo-patch" && pwd)"
OUTDIR="$(cd "$(dirname "$0")/.." && pwd)/bin/mihomo"
mkdir -p "$OUTDIR"

echo "=== Patching mihomo at $MIHOMO_DIR ==="

# 1. Copy ShieldLink transport files
mkdir -p "$MIHOMO_DIR/transport/shieldlink"
cp "$PATCH_DIR/auth.go"      "$MIHOMO_DIR/transport/shieldlink/"
cp "$PATCH_DIR/conn.go"      "$MIHOMO_DIR/transport/shieldlink/"
cp "$PATCH_DIR/pool.go"      "$MIHOMO_DIR/transport/shieldlink/"
cp "$PATCH_DIR/listener.go"  "$MIHOMO_DIR/transport/shieldlink/"
cp "$PATCH_DIR/aggregate.go" "$MIHOMO_DIR/transport/shieldlink/"

# 2. Replace parser.go (adds ShieldLink detection before proxy construction)
cp "$PATCH_DIR/parser.go" "$MIHOMO_DIR/adapter/parser.go"

echo "=== Patch applied ==="

# 3. Build for all platforms
cd "$MIHOMO_DIR"

GOBUILD="CGO_ENABLED=0 go build -tags with_gvisor -trimpath -ldflags '-w -s' -o"

echo "Building mihomo-shieldlink..."

GOOS=windows GOARCH=amd64 GOAMD64=v3 $GOBUILD "$OUTDIR/mihomo-windows-amd64.exe" .
echo "  windows-amd64 done"
GOOS=windows GOARCH=arm64             $GOBUILD "$OUTDIR/mihomo-windows-arm64.exe" .
echo "  windows-arm64 done"
GOOS=android GOARCH=arm64             $GOBUILD "$OUTDIR/mihomo-android-arm64" .
echo "  android-arm64 done"
GOOS=darwin  GOARCH=amd64 GOAMD64=v3  $GOBUILD "$OUTDIR/mihomo-darwin-amd64" .
echo "  darwin-amd64 done"
GOOS=darwin  GOARCH=arm64             $GOBUILD "$OUTDIR/mihomo-darwin-arm64" .
echo "  darwin-arm64 done"
GOOS=linux   GOARCH=amd64 GOAMD64=v3  $GOBUILD "$OUTDIR/mihomo-linux-amd64" .
echo "  linux-amd64 done"
GOOS=linux   GOARCH=arm64             $GOBUILD "$OUTDIR/mihomo-linux-arm64" .
echo "  linux-arm64 done"

echo ""
echo "=== All builds complete ==="
ls -lh "$OUTDIR/"

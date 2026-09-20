#!/usr/bin/env bash
# Cross-compile LinkPool for Apple Silicon (arm64).
# On Linux: produces the binary only.
# On macOS: also assembles LinkPool.app under build/macos/ and ad-hoc codesigns.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-0.1.1}"
OUT_DIR="${OUT_DIR:-$ROOT/build/macos}"
BIN_NAME="LinkPool"
mkdir -p "$OUT_DIR"

echo "==> go test"
go test ./...

echo "==> build darwin/arm64"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
  -o "$OUT_DIR/${BIN_NAME}" ./cmd/linkpool

echo "==> assemble .app bundle"
APP="$OUT_DIR/LinkPool.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$ROOT/packaging/macos/Info.plist" "$APP/Contents/Info.plist"
cp "$OUT_DIR/${BIN_NAME}" "$APP/Contents/MacOS/LinkPool"
chmod +x "$APP/Contents/MacOS/LinkPool"

# Optional icon
if [[ -f "$ROOT/packaging/macos/AppIcon.icns" ]]; then
  cp "$ROOT/packaging/macos/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
fi

# PkgInfo
echo -n "APPL????" > "$APP/Contents/PkgInfo"

# Ad-hoc codesign on Darwin (helps Gatekeeper accept unsigned local builds)
if [[ "$(uname -s)" == "Darwin" ]]; then
  echo "==> ad-hoc codesign"
  codesign --force --deep --sign - "$APP"
  codesign --force --sign - "$OUT_DIR/${BIN_NAME}"
  codesign -dv --verbose=2 "$APP" 2>&1 | head -20 || true
fi

# Convenience: also keep bare binary next to .app
echo "Built:"
echo "  $APP"
echo "  $OUT_DIR/${BIN_NAME}"
echo ""
echo "Run binary:  $OUT_DIR/${BIN_NAME} -open"
echo "Or open app (macOS only): open $APP"
echo "Then package DMG: ./scripts/package-dmg.sh"

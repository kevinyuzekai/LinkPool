#!/usr/bin/env bash
# Optional: convert AppIcon.svg → AppIcon.icns (macOS only, needs qlmanage/rsvg + iconutil)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/packaging/macos/AppIcon.svg"
OUT="$ROOT/packaging/macos/AppIcon.icns"
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "make-icns.sh requires macOS. SVG source is at packaging/macos/AppIcon.svg"
  exit 0
fi
ICONSET="$ROOT/build/AppIcon.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET" "$ROOT/build"
# Prefer rsvg-convert or magick if present; else skip
if command -v rsvg-convert >/dev/null; then
  for s in 16 32 64 128 256 512 1024; do
    rsvg-convert -w "$s" -h "$s" "$SRC" > "$ICONSET/icon_${s}x${s}.png"
  done
  cp "$ICONSET/icon_32x32.png" "$ICONSET/icon_16x16@2x.png"
  cp "$ICONSET/icon_64x64.png" "$ICONSET/icon_32x32@2x.png"
  cp "$ICONSET/icon_256x256.png" "$ICONSET/icon_128x128@2x.png"
  cp "$ICONSET/icon_512x512.png" "$ICONSET/icon_256x256@2x.png"
  cp "$ICONSET/icon_1024x1024.png" "$ICONSET/icon_512x512@2x.png"
  iconutil -c icns "$ICONSET" -o "$OUT"
  echo "Wrote $OUT"
else
  echo "Install librsvg (rsvg-convert) to generate icns; continuing without icon."
fi

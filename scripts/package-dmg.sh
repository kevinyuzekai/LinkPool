#!/usr/bin/env bash
# Create a distributable .dmg containing LinkPool.app
# MUST run on macOS (requires hdiutil).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT/build/macos}"
APP="$OUT_DIR/LinkPool.app"
VERSION="${VERSION:-0.1.1}"
DMG_NAME="LinkPool-${VERSION}-arm64.dmg"
DMG_PATH="$OUT_DIR/$DMG_NAME"
VOL_NAME="LinkPool"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: package-dmg.sh must run on macOS (hdiutil not available on $(uname -s))." >&2
  echo "On this Linux build box we only prepare the .app layout via build-macos-arm64.sh." >&2
  echo "On your MacBook Pro (Apple Silicon):" >&2
  echo "  git clone … && cd LinkPool && ./scripts/build-macos-arm64.sh && ./scripts/package-dmg.sh" >&2
  exit 1
fi

if [[ ! -d "$APP" ]]; then
  echo "error: $APP not found. Run ./scripts/build-macos-arm64.sh first." >&2
  exit 1
fi

STAGE="$OUT_DIR/dmg-stage"
rm -rf "$STAGE" "$DMG_PATH"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

STAGE_APP="$STAGE/LinkPool.app"
# Re-sign after copy so quarantine/copy doesn't leave an unsigned tree
echo "==> ad-hoc codesign (stage)"
codesign --force --deep --sign - "$STAGE_APP"

# Optional README in DMG
cat > "$STAGE/使用说明.txt" << TXT
LinkPool 网卡聚合助手 v${VERSION}

1. 将 LinkPool.app 拖到「应用程序」文件夹
2. 首次打开若被拦截：系统设置 → 隐私与安全性 → 仍要打开
   或在终端执行：xattr -cr /Applications/LinkPool.app
3. 本应用没有原生窗口；启动后请用浏览器打开 http://127.0.0.1:8787 控制面板
4. 勾选网卡、设权重，点击「启动」
5. 可选启用系统代理，或手动将浏览器/Steam 指向：
   HTTP  127.0.0.1:18080
   SOCKS5 127.0.0.1:11080

本工具按「新连接」跨网卡调度，不会拆分单条 TCP。
TXT

echo "==> hdiutil create $DMG_PATH"
hdiutil create -volname "$VOL_NAME" -srcfolder "$STAGE" -ov -format UDZO "$DMG_PATH"
rm -rf "$STAGE"
echo "DMG ready: $DMG_PATH"
ls -lh "$DMG_PATH"

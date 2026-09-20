#!/usr/bin/env bash
# Create a distributable .dmg containing LinkPool.app
# MUST run on macOS (requires hdiutil).
# Usage: ARCH=arm64|amd64 ./scripts/package-dmg.sh
# Outputs: build/macos/$ARCH/LinkPool-$VERSION-$ARCH.dmg
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

VERSION="${VERSION:-0.2.0}"
ARCH="${ARCH:-arm64}"
case "$ARCH" in
  arm64|aarch64) ARCH=arm64 ;;
  amd64|x86_64|intel) ARCH=amd64 ;;
  *)
    echo "error: ARCH must be arm64 or amd64 (got: $ARCH)" >&2
    exit 1
    ;;
esac

OUT_DIR="${OUT_DIR:-$ROOT/build/macos/$ARCH}"
APP="$OUT_DIR/LinkPool.app"
DMG_NAME="LinkPool-${VERSION}-${ARCH}.dmg"
DMG_PATH="$OUT_DIR/$DMG_NAME"
VOL_NAME="LinkPool"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: package-dmg.sh must run on macOS (hdiutil not available on $(uname -s))." >&2
  echo "On this Linux build box we only prepare the .app layout via build-macos.sh." >&2
  echo "On your Mac:" >&2
  echo "  # Apple Silicon:" >&2
  echo "  ARCH=arm64 ./scripts/build-macos.sh && ARCH=arm64 ./scripts/package-dmg.sh" >&2
  echo "  # Intel:" >&2
  echo "  ARCH=amd64 ./scripts/build-macos.sh && ARCH=amd64 ./scripts/package-dmg.sh" >&2
  exit 1
fi

if [[ ! -d "$APP" ]]; then
  echo "error: $APP not found. Run ARCH=${ARCH} ./scripts/build-macos.sh first." >&2
  exit 1
fi

STAGE="$OUT_DIR/dmg-stage"
rm -rf "$STAGE" "$DMG_PATH"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

STAGE_APP="$STAGE/LinkPool.app"
echo "==> ad-hoc codesign (stage, ${ARCH})"
codesign --force --deep --sign - "$STAGE_APP"

ARCH_HINT="Apple Silicon (arm64)"
if [[ "$ARCH" == "amd64" ]]; then
  ARCH_HINT="Intel (x86_64 / amd64)"
fi

cat > "$STAGE/使用说明.txt" << TXT
LinkPool 网卡聚合助手 v${VERSION} — ${ARCH_HINT}

1. 将 LinkPool.app 拖到「应用程序」文件夹
2. 首次打开若被拦截：系统设置 → 隐私与安全性 → 仍要打开
   或在终端执行：xattr -cr /Applications/LinkPool.app
3. 本应用没有原生窗口；启动后请用浏览器打开 http://127.0.0.1:8787 控制面板
4. 勾选 ≥2 张网卡，调度选「自适应加速」，点击「启动」
5. 可选启用系统代理，或手动将浏览器/Steam 指向：
   HTTP  127.0.0.1:18080
   SOCKS5 127.0.0.1:11080
6. 单文件可用控制面板「分段加速下载」尽量把多网卡带宽叠在一起

请下载与本机芯片匹配的 DMG：
  · Apple Silicon (M1/M2/M3/…) → LinkPool-*-arm64.dmg
  · Intel Mac → LinkPool-*-amd64.dmg

本工具按「新连接」跨网卡调度；单条 TCP 无法拆分（非 TUN/MPTCP）。
TXT

echo "==> hdiutil create $DMG_PATH"
hdiutil create -volname "$VOL_NAME" -srcfolder "$STAGE" -ov -format UDZO "$DMG_PATH"
rm -rf "$STAGE"
echo "DMG ready: $DMG_PATH"
ls -lh "$DMG_PATH"

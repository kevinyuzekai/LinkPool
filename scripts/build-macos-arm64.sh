#!/usr/bin/env bash
# Convenience wrapper — Apple Silicon (arm64).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARCH=arm64 exec "$ROOT/scripts/build-macos.sh" "$@"

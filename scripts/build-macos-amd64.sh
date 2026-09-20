#!/usr/bin/env bash
# Convenience wrapper — Intel Mac (amd64 / x86_64).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARCH=amd64 exec "$ROOT/scripts/build-macos.sh" "$@"

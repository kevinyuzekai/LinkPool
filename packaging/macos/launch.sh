#!/bin/bash
# Runs inside LinkPool.app/Contents/MacOS/ as the CFBundleExecutable wrapper alternative.
# Prefer compiled Go binary named LinkPool; this script is only used if packaging copies it.
DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$DIR/linkpool-bin" -ui 127.0.0.1:8787 -open

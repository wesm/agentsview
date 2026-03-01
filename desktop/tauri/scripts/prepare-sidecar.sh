#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAURI_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$TAURI_DIR/../.." && pwd)"

if ! command -v rustc >/dev/null 2>&1; then
  echo "error: rustc is required to determine the host target triple" >&2
  exit 1
fi

TARGET_TRIPLE="$(rustc -vV | awk '/^host: /{print $2}')"
if [ -z "$TARGET_TRIPLE" ]; then
  echo "error: could not determine target triple" >&2
  exit 1
fi

echo "Building agentsview backend for sidecar ($TARGET_TRIPLE)..."
(
  cd "$REPO_ROOT"
  make build
)

SRC_BIN="$REPO_ROOT/agentsview"
if [ ! -f "$SRC_BIN" ]; then
  SRC_BIN="$REPO_ROOT/agentsview.exe"
fi

if [ ! -f "$SRC_BIN" ]; then
  echo "error: built backend binary not found at $REPO_ROOT/agentsview or $REPO_ROOT/agentsview.exe" >&2
  exit 1
fi

EXT=""
if [[ "$TARGET_TRIPLE" == *"windows"* ]]; then
  EXT=".exe"
fi

OUT_DIR="$TAURI_DIR/src-tauri/binaries"
OUT_BIN="$OUT_DIR/agentsview-$TARGET_TRIPLE$EXT"

mkdir -p "$OUT_DIR"
cp "$SRC_BIN" "$OUT_BIN"
chmod +x "$OUT_BIN" || true

echo "Prepared sidecar binary: $OUT_BIN"

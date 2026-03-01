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
  cd "$REPO_ROOT/frontend"
  npm install
  npm run build
)

rm -rf "$REPO_ROOT/internal/web/dist"
cp -r "$REPO_ROOT/frontend/dist" "$REPO_ROOT/internal/web/dist"

EXT=""
if [[ "$TARGET_TRIPLE" == *"windows"* ]]; then
  EXT=".exe"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

BUILD_BIN="$TMP_DIR/agentsview$EXT"
(
  cd "$REPO_ROOT"
  CGO_ENABLED=1 go build -tags fts5 -o "$BUILD_BIN" ./cmd/agentsview
)

if [ ! -f "$BUILD_BIN" ]; then
  echo "error: built backend binary not found at $BUILD_BIN" >&2
  exit 1
fi

OUT_DIR="$TAURI_DIR/src-tauri/binaries"
OUT_BIN="$OUT_DIR/agentsview-$TARGET_TRIPLE$EXT"

mkdir -p "$OUT_DIR"
cp "$BUILD_BIN" "$OUT_BIN"
chmod +x "$OUT_BIN" || true

echo "Prepared sidecar binary: $OUT_BIN"

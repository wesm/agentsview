#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_FILE="$SCRIPT_DIR/../ui/index.html"

if [ ! -f "$UI_FILE" ]; then
  echo "missing startup UI file: $UI_FILE" >&2
  exit 1
fi

if ! grep -q "width: min(680px, 100%);" "$UI_FILE"; then
  echo "startup UI width rule missing responsive 100% clamp" >&2
  exit 1
fi

if ! grep -q "prefers-reduced-motion: reduce" "$UI_FILE"; then
  echo "startup UI missing reduced-motion fallback" >&2
  exit 1
fi

echo "startup UI checks passed"

#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=prepare-sidecar.sh
source "$SCRIPT_DIR/prepare-sidecar.sh"

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"
  if [ "$got" != "$want" ]; then
    echo "assertion failed: $msg (got='$got' want='$want')" >&2
    exit 1
  fi
}

assert_fails() {
  local msg="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "assertion failed: expected failure: $msg" >&2
    exit 1
  fi
}

assert_eq "$(map_go_target aarch64-apple-darwin)" "darwin arm64" "map darwin arm64"
assert_eq "$(map_go_target x86_64-apple-darwin)" "darwin amd64" "map darwin amd64"
assert_eq "$(map_go_target x86_64-pc-windows-msvc)" "windows amd64" "map windows amd64"
assert_eq "$(map_go_target x86_64-unknown-linux-gnu)" "linux amd64" "map linux amd64"
assert_fails "unsupported triple rejected" map_go_target "weird-target"

target="$(
  TAURI_ENV_TARGET_TRIPLE="tauri-priority-target" CARGO_BUILD_TARGET="cargo-target" \
    resolve_target_triple
)"
assert_eq "$target" "tauri-priority-target" "TAURI target precedence"

target="$(
  unset TAURI_ENV_TARGET_TRIPLE
  CARGO_BUILD_TARGET="cargo-target" resolve_target_triple
)"
assert_eq "$target" "cargo-target" "Cargo target fallback"

echo "prepare-sidecar target mapping checks passed"

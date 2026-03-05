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

resolved_version="$(resolve_version)"
if [ -z "$resolved_version" ] || [ "$resolved_version" = "dev" ]; then
  echo "assertion failed: resolve_version should use git metadata (got '$resolved_version')" >&2
  exit 1
fi

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

# version_to_semver tests
assert_eq "$(version_to_semver "v0.10.0")" "0.10.0" \
  "tagged release"
assert_eq "$(version_to_semver "0.10.0")" "0.10.0" \
  "tagged release without v prefix"
assert_eq "$(version_to_semver "v0.10.0-3-gabcdef")" "0.10.0-dev.3" \
  "git-describe with distance"
assert_eq "$(version_to_semver "v1.2.3-15-g1234567")" "1.2.3-dev.15" \
  "git-describe large distance"
assert_eq "$(version_to_semver "v0.10.0-dirty")" "0.10.0" \
  "dirty tag stripped"
assert_eq "$(version_to_semver "v0.10.0-3-gabcdef-dirty")" "0.10.0-dev.3" \
  "git-describe dirty with distance"
assert_eq "$(version_to_semver "abc1234")" "0.0.0-dev" \
  "short SHA fallback"

# patch_tauri_version test (uses a temp copy)
tmp_conf="$(mktemp -d)"
cp "$SCRIPT_DIR/../src-tauri/tauri.conf.json" "$tmp_conf/tauri.conf.json"
original_version="$(grep '"version"' "$tmp_conf/tauri.conf.json" | head -1)"
# Temporarily override TAURI_DIR to use our temp copy
saved_tauri_dir="$TAURI_DIR"
TAURI_DIR="$tmp_conf/.."
mkdir -p "$tmp_conf/../src-tauri"
cp "$saved_tauri_dir/src-tauri/tauri.conf.json" "$tmp_conf/../src-tauri/tauri.conf.json"
TAURI_DIR="$tmp_conf/.."
patch_tauri_version "v0.10.0" >/dev/null
patched="$(grep '"version"' "$tmp_conf/../src-tauri/tauri.conf.json" | head -1)"
assert_eq "$(echo "$patched" | tr -d ' ')" '"version":"0.10.0",' \
  "patch_tauri_version applies correct version"
TAURI_DIR="$saved_tauri_dir"
rm -rf "$tmp_conf"

echo "prepare-sidecar target mapping checks passed"

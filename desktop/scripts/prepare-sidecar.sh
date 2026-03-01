#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TAURI_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$TAURI_DIR/.." && pwd)"

detect_host_triple() {
  if ! command -v rustc >/dev/null 2>&1; then
    echo "error: rustc is required to determine the host target triple" >&2
    return 1
  fi
  local host
  host="$(rustc -vV | awk '/^host: /{print $2}')"
  if [ -z "$host" ]; then
    echo "error: could not determine host target triple" >&2
    return 1
  fi
  echo "$host"
}

resolve_target_triple() {
  if [ -n "${TAURI_ENV_TARGET_TRIPLE:-}" ]; then
    echo "$TAURI_ENV_TARGET_TRIPLE"
    return 0
  fi
  if [ -n "${CARGO_BUILD_TARGET:-}" ]; then
    echo "$CARGO_BUILD_TARGET"
    return 0
  fi
  detect_host_triple
}

map_go_target() {
  case "$1" in
    aarch64-apple-darwin) echo "darwin arm64" ;;
    x86_64-apple-darwin) echo "darwin amd64" ;;
    x86_64-pc-windows-msvc|x86_64-pc-windows-gnu) echo "windows amd64" ;;
    aarch64-pc-windows-msvc) echo "windows arm64" ;;
    x86_64-unknown-linux-gnu) echo "linux amd64" ;;
    aarch64-unknown-linux-gnu) echo "linux arm64" ;;
    *)
      echo "error: unsupported target triple for Go sidecar: $1" >&2
      return 1
      ;;
  esac
}

install_frontend_deps() {
  if [ -f "$REPO_ROOT/frontend/package-lock.json" ]; then
    npm ci
  else
    npm install
  fi
}

main() {
  local target_triple go_target goos goarch ext host_triple
  target_triple="$(resolve_target_triple)"
  if [ -z "$target_triple" ]; then
    echo "error: target triple is empty" >&2
    exit 1
  fi
  go_target="$(map_go_target "$target_triple")"
  read -r goos goarch <<<"$go_target"
  host_triple="$(detect_host_triple)"

  echo "Building agentsview backend for sidecar ($target_triple -> $goos/$goarch)..."
  if [ "$target_triple" != "$host_triple" ]; then
    echo "warning: cross-target sidecar build requested from host $host_triple" >&2
  fi

  (
    cd "$REPO_ROOT/frontend"
    install_frontend_deps
    npm run build
  )

  rm -rf "$REPO_ROOT/internal/web/dist"
  cp -r "$REPO_ROOT/frontend/dist" "$REPO_ROOT/internal/web/dist"

  ext=""
  if [[ "$target_triple" == *"windows"* ]]; then
    ext=".exe"
  fi

  local version commit build_date ldflags tmp_dir build_bin
  version="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo "dev")"
  commit="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
  build_date="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  ldflags="-X main.version=$version -X main.commit=$commit -X main.buildDate=$build_date -s -w"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT
  build_bin="$tmp_dir/agentsview$ext"

  (
    cd "$REPO_ROOT"
    CGO_ENABLED=1 GOOS="$goos" GOARCH="$goarch" \
      go build -tags fts5 -ldflags "$ldflags" -trimpath \
      -o "$build_bin" ./cmd/agentsview
  )

  if [ ! -f "$build_bin" ]; then
    echo "error: built backend binary not found at $build_bin" >&2
    exit 1
  fi

  local out_dir out_bin
  out_dir="$TAURI_DIR/src-tauri/binaries"
  out_bin="$out_dir/agentsview-$target_triple$ext"

  mkdir -p "$out_dir"
  cp "$build_bin" "$out_bin"
  chmod +x "$out_bin" || true

  echo "Prepared sidecar binary: $out_bin"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi

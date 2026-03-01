# AgentsView Desktop (Tauri)

This directory contains an experimental Tauri desktop wrapper for AgentsView.

The wrapper does not reimplement the web app. Instead, it:
1. Builds the existing Go `agentsview` binary.
2. Packages it as a Tauri sidecar.
3. Starts it with `serve -no-browser` on a local port.
4. Loads the local URL in a native webview.

## Requirements

- Rust toolchain (`rustc`, `cargo`)
- Node.js and npm
- Go (with CGO enabled; same requirements as the main project)

## Usage

```bash
npm install
npm run tauri:dev
npm run tauri:build
```

The `prepare-sidecar` step runs automatically for `tauri:dev` and `tauri:build`.
It builds `agentsview` and copies it to `src-tauri/binaries/agentsview-<target-triple>`.

---
phase: 02-sync-and-config-integration
plan: "01"
subsystem: config
tags: [config, pi-agent, env-var, multi-dir]
dependency_graph:
  requires: []
  provides: [ResolvePiDirs, PiDir-default, PI_DIR-env, pi_dirs-config]
  affects: [internal/sync/engine.go, cmd/agentsview/main.go]
tech_stack:
  added: []
  patterns: [multi-dir resolver pattern (resolveDirs)]
key_files:
  modified:
    - internal/config/config.go
decisions:
  - "PiDir/PiDirs placed after OpenCodeDirs in struct — maintains agent ordering consistency"
  - "PI_DIR env var sets both PiDir and PiDirs (single-element slice) — identical pattern to all other agents"
  - "pi_dirs config file key matches snake_case convention of all other agent dir arrays"
metrics:
  duration: "1 min"
  completed: "2026-02-28"
  tasks_completed: 2
  files_modified: 1
---

# Phase 02 Plan 01: Pi-Agent Config Fields Summary

**One-liner:** Added PiDir/PiDirs config fields with PI_DIR env var, pi_dirs config file support, default ~/.pi/agent/sessions/, and ResolvePiDirs() method following the existing multi-dir agent pattern.

## What Was Built

Config layer support for pi-agent directories, following the identical pattern used by Gemini, Codex, Copilot, and OpenCode. The config layer is the root dependency for the sync engine; without ResolvePiDirs() the engine cannot receive pi directories.

### Changes

**internal/config/config.go:**
- Added `PiDir string` and `PiDirs []string` fields to Config struct (after OpenCodeDirs)
- Added `PiDir: filepath.Join(home, ".pi", "agent", "sessions")` default in Default()
- Added PI_DIR env var handling in loadEnv() that sets both PiDir and PiDirs
- Added `PiDirs []string` to the loadFile anonymous struct with `json:"pi_dirs"` tag
- Added pi_dirs application block in loadFile() (only applies when PiDirs is nil — env var takes precedence)
- Added ResolvePiDirs() method delegating to resolveDirs(c.PiDirs, c.PiDir)

## Precedence Order

1. PI_DIR env var (sets both PiDir and PiDirs as single-element slice)
2. pi_dirs array in config.json (applied only when PiDirs is nil)
3. Default: ~/.pi/agent/sessions/ (via PiDir single-dir fallback)

## Verification

- `go vet ./internal/config/...` passed
- `CGO_ENABLED=1 go build -tags fts5 ./internal/config/...` passed
- `CGO_ENABLED=1 go test -tags fts5 ./internal/config/...` passed — 17 tests, no regressions

## Deviations from Plan

None - plan executed exactly as written.

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| 1 | ef574bb | feat(02-01): add PiDir/PiDirs fields, PI_DIR env var, and pi_dirs config file support |
| 2 | 11d2161 | feat(02-01): add ResolvePiDirs() method to Config |

## Self-Check: PASSED

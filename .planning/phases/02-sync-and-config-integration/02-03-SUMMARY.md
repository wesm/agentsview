---
phase: 02-sync-and-config-integration
plan: "03"
subsystem: sync
tags: [go, sqlite, pi, sync-engine, integration-test]

# Dependency graph
requires:
  - phase: 02-sync-and-config-integration/02-01
    provides: config.ResolvePiDirs(), AgentPi constant in taxonomy
  - phase: 02-sync-and-config-integration/02-02
    provides: DiscoverPiSessions function
  - phase: 01-parser-foundation/01-01
    provides: ParsePiSession function and ParsedSession/ParsedMessage types
provides:
  - piDirs field in Engine struct and NewEngine parameter
  - classifyOnePath classification for pi session paths
  - processPi method calling ParsePiSession
  - syncAllLocked loop for DiscoverPiSessions
  - main.go warnMissingDirs and file watcher for pi dirs
  - TestPiSessionIntegration proving end-to-end pi session persistence
affects:
  - 03-frontend-display (pi sessions now appear in DB with agent=pi)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "processPi follows same nil-check-then-return pattern as processGemini"
    - "piDirs inserted between opencodeDirs and cursorDir in NewEngine signature"
    - "pi in classifyOnePath: 2-part relative path under piDir ending in .jsonl"

key-files:
  created: []
  modified:
    - internal/sync/engine.go
    - cmd/agentsview/main.go
    - internal/sync/engine_integration_test.go

key-decisions:
  - "processPi does not hash the file (unlike processGemini) — consistent with plan; mtime set by processFile after switch"
  - "classifyOnePath pi block placed after Cursor block before final return — continues existing ordering pattern"
  - "pi watcher registration uses piDir directly (no subdirectory) — pi session files are in encoded-cwd subdir, no extra nesting"

patterns-established:
  - "New agent type wiring requires: struct field, NewEngine param, classifyOnePath block, processFile case, processX method, syncAllLocked discovery loop, main.go warnMissingDirs + watcher + NewEngine call"

requirements-completed: [SYNC-01, SYNC-02, TEST-02]

# Metrics
duration: 5min
completed: 2026-02-28
---

# Phase 2 Plan 03: Sync Engine Pi Wiring Summary

**Pi sessions wired through all four Engine touch-points and proven end-to-end with TestPiSessionIntegration: 1 session synced, agent="pi" persisted in SQLite**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-02-28T05:06:25Z
- **Completed:** 2026-02-28T05:11:35Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Added piDirs field to Engine struct and piDirs []string parameter to NewEngine (between opencodeDirs and cursorDir)
- Added classifyOnePath pi block: matches `<piDir>/<encoded-cwd>/<session>.jsonl` (2-part relative path ending in .jsonl)
- Added processPi method calling ParsePiSession, added case parser.AgentPi in processFile switch
- Added DiscoverPiSessions loop in syncAllLocked with pi count in verbose log
- Updated main.go: warnMissingDirs("pi"), ResolvePiDirs() passed to NewEngine, pi dirs registered in file watcher
- TestPiSessionIntegration passes: seeds temp pi dir from parser testdata fixture, SyncAll returns Synced=1, DB row has agent="pi"
- All 192 existing sync integration tests still pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire pi into Engine struct, NewEngine, classifyOnePath, processPi, and syncAllLocked** - `fb91678` (feat)
2. **Task 2: Update both NewEngine callers and add integration test** - `c242d44` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `internal/sync/engine.go` - piDirs struct field, NewEngine param, classifyOnePath pi block, case AgentPi, processPi method, syncAllLocked pi discovery loop
- `cmd/agentsview/main.go` - warnMissingDirs pi, ResolvePiDirs() in NewEngine call, pi watcher registration in startFileWatcher
- `internal/sync/engine_integration_test.go` - nil piDirs in setupTestEnv, TestPiSessionIntegration added at end

## Decisions Made
- processPi does not set mtime: processFile sets `res.mtime = mtime` after the switch, consistent with all other processX methods
- pi watcher uses piDir directly (not a subdirectory): pi sessions live in `<piDir>/<encoded-cwd>/`, watcher catches all changes recursively
- classifyOnePath pi block placed after Cursor block: maintains agent-type ordering (claude, codex, copilot, gemini, cursor, pi)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `go build ./...` fails due to missing frontend dist (internal/web/embed.go pattern `all:dist`). Pre-existing condition, not caused by this plan. Verified sync and cmd packages specifically via `go build ./internal/sync/... ./internal/parser/... ./internal/config/...`.

## Next Phase Readiness
- Pi sessions now fully integrated: discovered, classified, parsed, and persisted in SQLite with agent="pi"
- Frontend display (Phase 3) can rely on agent="pi" rows appearing in the sessions table
- No blockers

---
*Phase: 02-sync-and-config-integration*
*Completed: 2026-02-28*

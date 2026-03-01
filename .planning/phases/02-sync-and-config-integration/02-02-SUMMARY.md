---
phase: 02-sync-and-config-integration
plan: "02"
subsystem: sync
tags: [discovery, pi, gjson, bufio, jsonl]

# Dependency graph
requires:
  - phase: 01-parser-foundation
    provides: AgentPi constant and ParsePiSession parser
  - phase: 02-sync-and-config-integration
    plan: "01"
    provides: PiDir config field wired into Config struct
provides:
  - DiscoverPiSessions(piDir string) []DiscoveredFile
  - FindPiSourceFile(piDir, sessionID string) string
  - isPiSessionFile(path string) bool (unexported content validator)
affects:
  - 02-sync-and-config-integration (plan 03 wires these into sync engine)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Content-based pi session validation: bufio.Scanner reads first JSONL line, gjson checks type field equals 'session'"
    - "Two-level directory scan for pi: piDir/encoded-cwd/*.jsonl (encoded-cwd format agnostic)"

key-files:
  created: []
  modified:
    - internal/sync/discovery.go

key-decisions:
  - "Content validation over directory-name parsing for pi discovery - directory encoding format is ambiguous between pi versions"
  - "Project field left empty in DiscoveredFile for pi - ParsePiSession derives project from header cwd field"
  - "8 KiB scanner buffer for isPiSessionFile - handles long header lines safely without unbounded reads"

patterns-established:
  - "isPiSessionFile pattern: open file, bufio.Scanner with explicit buffer, read first line, gjson.Get type field"
  - "FindPiSourceFile mirrors FindClaudeSourceFile: isValidSessionID guard, ReadDir top level, stat candidate paths"

requirements-completed:
  - CONF-05

# Metrics
duration: 1min
completed: 2026-02-28
---

# Phase 02 Plan 02: Pi Session Discovery Summary

**Two-level pi session discovery with bufio+gjson content validation, plus FindPiSourceFile lookup by session ID**

## Performance

- **Duration:** 1 min
- **Started:** 2026-02-28T05:04:32Z
- **Completed:** 2026-02-28T05:05:39Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- Added `isPiSessionFile` helper that validates a JSONL file has `type="session"` on its first line using bufio.Scanner (8 KiB buffer) and gjson
- Added `DiscoverPiSessions` that scans `piDir/<encoded-cwd>/*.jsonl` in two levels, validates each file via `isPiSessionFile`, and returns sorted `DiscoveredFile` list with `Agent=AgentPi` and empty `Project`
- Added `FindPiSourceFile` that searches all encoded-cwd subdirectories under piDir for `<sessionID>.jsonl`, returning the first match
- Added `bufio` and `github.com/tidwall/gjson` imports to discovery.go (gjson already in go.mod from parser package)
- All 142 existing sync tests continue to pass

## Task Commits

Each task was committed atomically:

1. **Tasks 1+2: Implement isPiSessionFile, DiscoverPiSessions, FindPiSourceFile** - `f9b4a1f` (feat)

## Files Created/Modified
- `internal/sync/discovery.go` - Added 93 lines: bufio/gjson imports, isPiSessionFile, DiscoverPiSessions, FindPiSourceFile

## Decisions Made
- Tasks 1 and 2 were implemented in a single edit and committed together since FindPiSourceFile was logically part of the same change and required no separate intermediate verification step
- Content validation (reading first JSONL line) chosen over directory-name parsing because the `--path--` vs `-path/` encoding format is ambiguous between pi versions (locked decision from STATE.md)
- Project left empty in DiscoveredFile so ParsePiSession can derive it from the header `cwd` field

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- `DiscoverPiSessions` and `FindPiSourceFile` are ready for the sync engine to call in plan 02-03
- No blockers

---
*Phase: 02-sync-and-config-integration*
*Completed: 2026-02-28*

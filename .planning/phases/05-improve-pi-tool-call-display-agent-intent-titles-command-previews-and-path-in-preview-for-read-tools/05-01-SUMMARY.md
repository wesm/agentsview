---
phase: 05-improve-pi-tool-call-display-agent-intent-titles-command-previews-and-path-in-preview-for-read-tools
plan: "01"
subsystem: ui
tags: [svelte, typescript, content-parser, pi, tool-calls]

# Dependency graph
requires:
  - phase: 04-pi-tool-rendering
    provides: enrichSegments with run_command/Bash expansion and TOOL_ALIASES
provides:
  - Lowercase pi tool name alias resolution (bash->Bash, read->Read, etc.)
  - isBashTool/isReadTool helper functions in content-parser
  - Read tool file path preview in collapsed header (content = file path)
  - Bash command preview for lowercase "bash" tool calls (content = $ command)
affects:
  - 05-02 (agent__intent titles — uses same enrichSegments pipeline)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Helper function pattern: isBashTool/isReadTool centralizes multi-name tool matching instead of inline OR chains"
    - "Content extraction pattern: isReadTool branch populates content from input_json path/file_path in append loop"

key-files:
  created: []
  modified:
    - frontend/src/lib/utils/content-parser.ts
    - frontend/src/lib/utils/content-parser.test.ts

key-decisions:
  - "find aliased to Read (not its own category) — consistent with grep/glob which are read-type operations"
  - "isReadTool applies only in !hasTextBasedTools append loop — text-marker Read segments already have content from regex"
  - "path ?? file_path coalesce matches pi field name (path) first, falls back to Claude field name (file_path)"

patterns-established:
  - "Use named helper functions (isBashTool, isReadTool) for multi-variant tool name checks across enrichSegments code paths"

requirements-completed: []

# Metrics
duration: 3min
completed: 2026-02-28
---

# Phase 05 Plan 01: Content Parser — Lowercase Aliases, Bash Expansion, Read Path Preview Summary

**Lowercase pi tool aliases (bash/read/write/edit/grep/glob/find) resolve to canonical labels; pi bash calls show `$ command` preview and pi read calls show file path in collapsed header**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-02-28T07:30:00Z
- **Completed:** 2026-02-28T07:33:23Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments

- Added 7 lowercase pi tool name entries to TOOL_ALIASES (bash, read, write, edit, grep, glob, find)
- Extracted isBashTool() and isReadTool() helper functions to eliminate duplicated OR conditions
- Updated both enrichSegments code paths to use helpers; added isReadTool branch for file path extraction
- 81 tests pass including 13 new tests covering alias resolution, bash expansion, and read path preview

## Task Commits

Each task was committed atomically:

1. **Task 1: Add lowercase pi tool aliases and isBashTool/isReadTool helpers** - `9c684e5` (feat)
2. **Task 2: Update enrichSegments to use helpers and add read path expansion** - `ae52760` (feat)
3. **Task 3: Add tests for aliases, bash expansion, and read path preview** - `06d3be6` (test)

## Files Created/Modified

- `frontend/src/lib/utils/content-parser.ts` — TOOL_ALIASES extended with 7 lowercase entries; isBashTool/isReadTool helpers added; enrichSegments updated in both code paths
- `frontend/src/lib/utils/content-parser.test.ts` — Two new describe blocks (13 new tests); fixed 3 existing label expectations from lowercase to canonical casing

## Decisions Made

- `find` aliased to `"Read"` — consistent with grep/glob which are all read-type file system operations
- `isReadTool` branch added only in the `!hasTextBasedTools` append loop — text-marker Read segments already have content populated from regex match; no double-processing needed
- `input.path ?? input.file_path` coalesce order: pi uses `path` field; `file_path` is Claude's field name — prefer pi's native field, fallback for compatibility

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed existing test label expectations broken by new aliases**
- **Found during:** Task 3 (test addition)
- **Issue:** Existing tests at lines 619, 625 expected labels `"read"`, `"write"`, `"bash"` (raw tool_name) — these now resolve to `"Read"`, `"Write"`, `"Bash"` via the new TOOL_ALIASES entries
- **Fix:** Updated 3 label assertions in the existing `enrichSegments` describe block to match canonical casing
- **Files modified:** frontend/src/lib/utils/content-parser.test.ts
- **Verification:** All 81 tests pass
- **Committed in:** `06d3be6` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 - existing test expectations)
**Impact on plan:** Necessary correctness fix — new aliases intentionally changed observable behavior that existing tests were asserting.

## Issues Encountered

None.

## Next Phase Readiness

- Plan 05-01 complete: lowercase aliases and content expansion foundation in place
- Plan 05-02 (agent__intent titles) can proceed — uses same enrichSegments pipeline and TOOL_ALIASES

---
*Phase: 05-improve-pi-tool-call-display-agent-intent-titles-command-previews-and-path-in-preview-for-read-tools*
*Completed: 2026-02-28*

---
phase: 05-improve-pi-tool-call-display-agent-intent-titles-command-previews-and-path-in-preview-for-read-tools
plan: "02"
subsystem: ui
tags: [svelte, typescript, tool-params, pi, agent__intent]

# Dependency graph
requires:
  - phase: 05-01
    provides: pi tool aliases and bash/read content extraction
provides:
  - agent__intent shown in collapsed tool header as preview fallback
  - agent__intent filtered from expanded key-value content in generateFallbackContent
  - INTERNAL_PARAMS constant for extensible pi-internal metadata filtering
affects:
  - any future pi tool display improvements

# Tech tracking
tech-stack:
  added: []
  patterns:
    - INTERNAL_PARAMS Set for filtering pi-internal metadata from generic key-value display

key-files:
  created: []
  modified:
    - frontend/src/lib/components/content/ToolBlock.svelte
    - frontend/src/lib/utils/tool-params.ts
    - frontend/src/lib/utils/tool-params.test.ts

key-decisions:
  - "inputParams derived before previewLine in ToolBlock.svelte to allow dependency"
  - "agent__intent filtered via INTERNAL_PARAMS Set (extensible for future pi metadata fields)"
  - "previewLine falls back to agent__intent only when content is empty (content takes priority)"

patterns-established:
  - "INTERNAL_PARAMS: module-level Set for pi-internal metadata keys excluded from generic display"

requirements-completed: []

# Metrics
duration: 3min
completed: 2026-02-28
---

# Phase 5 Plan 02: ToolBlock `agent__intent` Display and Fallback Filter Summary

**pi tool calls show agent__intent as collapsed header preview when content is empty, and agent__intent is filtered from expanded key-value content via INTERNAL_PARAMS Set**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-28T07:31:55Z
- **Completed:** 2026-02-28T07:34:30Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- Reordered `inputParams` before `previewLine` in ToolBlock.svelte so previewLine can reference inputParams
- Added `agent__intent` fallback to `previewLine` when content is empty using `$derived.by()`
- Added `INTERNAL_PARAMS` Set in tool-params.ts and filtered those keys from the generic key-value loop in `generateFallbackContent`
- Added 5 new tests for `agent__intent` filtering, all passing (41 total in tool-params.test.ts)

## Task Commits

Each task was committed atomically:

1. **Tasks 1-3: agent__intent previewLine fallback + INTERNAL_PARAMS filter + tests** - `0fe9165` (feat)

## Files Created/Modified
- `frontend/src/lib/components/content/ToolBlock.svelte` - Swapped inputParams/previewLine order; previewLine uses $derived.by() with agent__intent fallback
- `frontend/src/lib/utils/tool-params.ts` - Added INTERNAL_PARAMS constant; generic loop now skips those keys
- `frontend/src/lib/utils/tool-params.test.ts` - Added 5 agent__intent filtering tests in new describe block

## Decisions Made
- inputParams is derived before previewLine so the reactive graph can use inputParams in the previewLine derivation
- INTERNAL_PARAMS uses a Set for O(1) lookup and extensibility (easy to add future pi metadata fields)
- agent__intent only shows as previewLine when content is empty (content always takes priority)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

Pre-existing test failure in `sessions.test.ts` (agent filter test) confirmed to be unrelated to this plan and was already failing before changes.

## Next Phase Readiness
- agent__intent is now visible in collapsed tool headers for pi tool calls that lack content
- INTERNAL_PARAMS is set up for easy extension with additional pi-internal metadata fields
- Ready for any additional phase 5 plans

## Self-Check: PASSED

All files verified present. Commit 0fe9165 verified in git log.

---
*Phase: 05-improve-pi-tool-call-display-agent-intent-titles-command-previews-and-path-in-preview-for-read-tools*
*Completed: 2026-02-28*

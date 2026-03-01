# Roadmap: agentsview — Pi Agent Support

## Overview

This milestone adds pi-agent (and oh-my-pi) session support to agentsview. The work follows the existing integration pattern established by Claude, Gemini, and other agents: parser first (root dependency), then sync/config wiring, then frontend surfacing. Three phases deliver a complete, end-to-end pi session viewing experience with full parity to Claude.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Parser Foundation** - Pi JSONL parser with full message/tool/thinking parity and taxonomy additions (completed 2026-02-28)
- [x] **Phase 2: Sync and Config Integration** - Config fields, discovery, and engine wiring so pi sessions land in the DB (completed 2026-02-28)
- [ ] **Phase 3: Frontend Wiring** - Pi appears in session list with teal badge and agent filter

## Phase Details

### Phase 1: Parser Foundation
**Goal**: Pi-agent JSONL session files are correctly parsed into the existing session/message model with full content parity to Claude
**Depends on**: Nothing (first phase)
**Requirements**: PRSR-01, PRSR-02, PRSR-03, PRSR-04, PRSR-05, PRSR-06, PRSR-07, PRSR-08, PRSR-09, PRSR-10, PRSR-11, TAXO-01, TAXO-02, TEST-01
**Success Criteria** (what must be TRUE):
  1. A real pi JSONL session file fed to `ParsePiSession` returns a `ParsedSession` with correct id, project, agent=AgentPi, timestamps, and message counts
  2. Assistant messages with tool calls have `HasToolUse=true` and populated `ToolCalls` using the `toolCall`/`arguments` block type (not Claude's `tool_use`/`input`)
  3. Thinking blocks in assistant messages set `HasThinking=true` on that message
  4. Tool result entries (role="toolResult") are parsed as `ParsedMessage` with `ToolResults` populated
  5. Compaction entries produce a synthetic user message whose text content is FTS-indexable, and `model_change` entries produce meta messages with descriptive content
**Plans**: 3 plans

Plans:
- [x] 01-02-PLAN.md — Add AgentPi constant to types.go and find->Read taxonomy mapping (Wave 1)
- [ ] 01-01-PLAN.md — Implement ParsePiSession in internal/parser/pi.go (Wave 2)
- [ ] 01-03-PLAN.md — Write table-driven unit tests in internal/parser/pi_test.go (Wave 3)

### Phase 2: Sync and Config Integration
**Goal**: Pi sessions are discovered on startup and periodic sync, reaching the database through the fully wired sync engine and config layer
**Depends on**: Phase 1
**Requirements**: CONF-01, CONF-02, CONF-03, CONF-04, CONF-05, SYNC-01, SYNC-02, TEST-02
**Success Criteria** (what must be TRUE):
  1. Setting `PI_DIR=~/.omp/agent/sessions/` and starting agentsview causes pi sessions from that directory to appear in the database after startup sync
  2. With no `PI_DIR` set, pi sessions in `~/.pi/agent/sessions/` are discovered automatically via the default config
  3. After a 15-minute periodic sync (or forced sync), newly written pi session files appear in the database without a restart
  4. The sync engine routes pi sessions through all four wiring points without producing "unknown agent type" errors in logs
**Plans**: 3 plans

Plans:
- [ ] 02-01-PLAN.md — Add PiDir/PiDirs fields, PI_DIR env var, pi_dirs config file, and ResolvePiDirs() to internal/config/config.go (Wave 1)
- [ ] 02-02-PLAN.md — Implement DiscoverPiSessions() and FindPiSourceFile() in internal/sync/discovery.go using file-content validation (Wave 1)
- [ ] 02-03-PLAN.md — Wire pi into internal/sync/engine.go at all wiring points, update both NewEngine callers, add pi watcher in main.go, write integration test (Wave 2)

### Phase 3: Frontend Wiring
**Goal**: Pi sessions surface in the UI with a distinct teal badge and a working agent filter
**Depends on**: Phase 2
**Requirements**: FRNT-01, FRNT-02, FRNT-03, FRNT-04
**Success Criteria** (what must be TRUE):
  1. Pi sessions appear in the session list with a teal-colored "Pi" badge that is visually distinct from all six existing agent badges
  2. Clicking the "Pi" filter button in the sidebar shows only pi sessions (and hides sessions from other agents)
  3. Removing the pi filter restores the full session list with no regressions to existing agent filters
**Plans**: 1 plan

Plans:
- [ ] 03-01-PLAN.md — Add pi to KNOWN_AGENTS, define --accent-teal CSS variable pair, add agent-pi badge class and agentLabel helper, update tests (Wave 1)

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Parser Foundation | 3/3 | Complete   | 2026-02-28 |
| 2. Sync and Config Integration | 3/3 | Complete   | 2026-02-28 |
| 3. Frontend Wiring | 0/1 | Not started | - |

### Phase 4: Add better parsing of tool calls out of pi -- currently they are very much in their raw JSONL stored format and not in a more human readable format like you see in Claude's output

**Goal:** Pi tool calls render with the same visual polish as Claude Code — correct display labels, file path meta tags, diff/command content formats, and toolResult message suppression
**Requirements**: TBD
**Depends on:** Phase 3
**Plans:** 1 plan

Plans:
- [ ] 04-01-PLAN.md — Extend tool aliases, fix enrichSegments label path, add pi field name fallbacks, suppress toolResult messages (Wave 1)

### Phase 5: Improve pi tool call display: agent__intent titles, command previews, and path-in-preview for Read tools

**Goal:** [To be planned]
**Requirements**: TBD
**Depends on:** Phase 4
**Plans:** 2/2 plans complete

Plans:
- [x] TBD (run /gsd:plan-phase 5 to break down) (completed 2026-02-28)

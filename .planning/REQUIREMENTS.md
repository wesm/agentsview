# Requirements: agentsview — Pi Agent Support

**Defined:** 2026-02-27
**Core Value:** Users of pi-agent (and variants like oh-my-pi) can view their sessions in agentsview with full parity to Claude — all message types, tool calls, thinking blocks, and FTS search.

## v1 Requirements

Requirements for pi-agent integration. Maps to roadmap phases.

### Parser

- [x] **PRSR-01**: Pi-agent JSONL session files are parsed into `ParsedSession` with correct `id`, `Project`, `Agent=AgentPi`, `StartedAt`, `EndedAt`, `MessageCount`, and `UserMessageCount`
- [x] **PRSR-02**: User messages (role="user") are extracted as `ParsedMessage` with text content and correct ordinal
- [x] **PRSR-03**: Assistant messages (role="assistant") are extracted with text content, `HasThinking`, `HasToolUse`, `ToolCalls`, and token usage reflected in `ContentLength`
- [x] **PRSR-04**: Tool calls inside assistant messages are extracted using camelCase block type `toolCall` with `arguments` field (not Claude's `tool_use`/`input`)
- [x] **PRSR-05**: Tool results (`role="toolResult"` standalone top-level message entries) are extracted as `ParsedMessage` with `ToolResults` populated
- [x] **PRSR-06**: Thinking blocks (type="thinking") set `HasThinking=true` on the containing assistant message
- [x] **PRSR-07**: `model_change` events are surfaced as meta messages (ordinal preserved, content describes model change)
- [x] **PRSR-08**: `compaction` events are surfaced as synthetic user messages so FTS indexes the summary and session reads remain continuous
- [x] **PRSR-09**: V1 pi sessions (no `id`/`parentId` fields, no `version` in header) are parsed using linear ordering by file position
- [x] **PRSR-10**: `branchedFrom` path in session header is stored as `ParentSessionID` (basename without extension)
- [x] **PRSR-11**: `AgentPi` constant added to `internal/parser/types.go`

### Taxonomy

- [x] **TAXO-01**: `find` tool name maps to "Read" category in `NormalizeToolCategory`
- [x] **TAXO-02**: `grep` (lowercase, pi-specific) maps to "Grep" category (currently falls to "Other") — already handled in Gemini section; confirmed no-op

### Config & Discovery

- [x] **CONF-01**: `PiDir` (single) and `PiDirs` (multi) fields added to `Config` struct following `GeminiDir`/`GeminiDirs` pattern
- [x] **CONF-02**: `PI_DIR` env var overrides `PiDir` (same as `GEMINI_DIR` pattern)
- [x] **CONF-03**: Default `PiDir` is `~/.pi/agent/sessions/`
- [x] **CONF-04**: `ResolvePiDirs()` method returns the effective directory list (env var > config > default)
- [x] **CONF-05**: Pi session files are discovered by scanning encoded-cwd subdirectories for `*.jsonl` files and validating them by reading the session header (type="session")

### Sync Engine

- [x] **SYNC-01**: Sync engine dispatches pi-agent sessions through all 4 wiring points: `processFile` switch, `classifyOnePath`, `syncAllLocked` discovery call, and `piDirs` field
- [x] **SYNC-02**: Pi sessions appear in the database after startup sync and after 15-min periodic sync

### Frontend

- [x] **FRNT-01**: `"pi"` entry added to `KNOWN_AGENTS` in `frontend/src/utils/agents.ts` with display label "Pi"
- [x] **FRNT-02**: `--accent-teal` CSS variable added to `frontend/src/app.css`
- [x] **FRNT-03**: Pi agent badge uses teal accent color in `App.svelte`
- [x] **FRNT-04**: Pi appears in the agent filter UI and sessions can be filtered by pi agent type

### Tests

- [x] **TEST-01**: Table-driven unit tests in `internal/parser/pi_test.go` covering: session header parsing, user messages, assistant messages with tool calls, tool results, thinking blocks, model_change, compaction, V1 sessions, branchedFrom
- [x] **TEST-02**: Integration test seeding a temp pi session directory and verifying sessions appear in DB via sync engine

## v2 Requirements

### Enhanced Pi Features

- **PI-V2-01**: `session_info.name` display — use human-readable session name in UI when available
- **PI-V2-02**: `branchedFrom` resolution against existing DB sessions — full parent linking in UI
- **PI-V2-03**: Cost display per session — sum `usage.cost.total` across assistant messages and expose in API
- **PI-V2-04**: `bashExecution` custom message type display in session detail view
- **PI-V2-05**: HTML export for pi sessions

## Out of Scope

| Feature | Reason |
|---------|--------|
| Image content blocks (base64) | Storage/display complexity; not core to text session viewing |
| Real-time file watching during active pi run | 15-min periodic sync is sufficient for v1 |
| DAG/fork detection | Pi uses linear id/parentId chain, not Claude's uuid branching |
| `thinking_level_change` as displayed message | Metadata event; relevant to pi internals, not session reading |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| PRSR-01 | Phase 1 | Complete |
| PRSR-02 | Phase 1 | Complete |
| PRSR-03 | Phase 1 | Complete |
| PRSR-04 | Phase 1 | Complete |
| PRSR-05 | Phase 1 | Complete |
| PRSR-06 | Phase 1 | Complete |
| PRSR-07 | Phase 1 | Complete |
| PRSR-08 | Phase 1 | Complete |
| PRSR-09 | Phase 1 | Complete |
| PRSR-10 | Phase 1 | Complete |
| PRSR-11 | Phase 1 | Complete |
| TAXO-01 | Phase 1 | Complete |
| TAXO-02 | Phase 1 | Complete |
| CONF-01 | Phase 2 | Complete |
| CONF-02 | Phase 2 | Complete |
| CONF-03 | Phase 2 | Complete |
| CONF-04 | Phase 2 | Complete |
| CONF-05 | Phase 2 | Complete |
| SYNC-01 | Phase 2 | Complete |
| SYNC-02 | Phase 2 | Complete |
| FRNT-01 | Phase 3 | Complete |
| FRNT-02 | Phase 3 | Complete |
| FRNT-03 | Phase 3 | Complete |
| FRNT-04 | Phase 3 | Complete |
| TEST-01 | Phase 1 | Complete |
| TEST-02 | Phase 2 | Complete |

**Coverage:**
- v1 requirements: 26 total
- Mapped to phases: 26
- Unmapped: 0 ✓

---
*Requirements defined: 2026-02-27*
*Last updated: 2026-02-27 after 01-02 completion (PRSR-11, TAXO-01, TAXO-02 complete)*

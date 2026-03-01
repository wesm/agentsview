# agentsview — Pi Agent Support

## What This Is

agentsview is a local web viewer for AI agent sessions. It already supports Claude Code, Codex, Copilot CLI, Gemini CLI, OpenCode, and Cursor. This milestone adds support for the pi-agent (and variants like oh-my-pi), syncing their JSONL session files into the existing SQLite store and surfacing them in the UI.

## Core Value

Users of pi-agent (including oh-my-pi) should see their sessions in agentsview with the same quality of experience as Claude — full message parsing, tool call rendering, FTS search, and session metadata.

## Requirements

### Validated

- ✓ Claude Code sessions parsed from JSONL with full message/tool support — existing
- ✓ Codex, Copilot, Gemini, OpenCode, Cursor session parsing — existing
- ✓ SQLite/FTS5 session store with sync engine — existing
- ✓ Svelte SPA with agent filter UI — existing
- ✓ SSE real-time updates — existing
- ✓ Multi-directory config support for all existing agents — existing

### Active

- [ ] Pi-agent JSONL parser with full parity to Claude (user/assistant text, thinking blocks, embedded tool calls in content arrays, tool results, model_change and thinking_level_change events, compaction entries)
- [ ] `PiDir`/`PiDirs` config fields with `PI_DIR` env var, defaulting to `~/.pi/agent/sessions/`
- [ ] Sync engine discovery for pi-agent session directories (encoded-cwd subdirectory pattern, same as Claude)
- [ ] `AgentPi` constant in parser types + taxonomy entries for pi-agent tools (read, write, edit, bash, grep, glob, etc.)
- [ ] Filter UI support for pi agent type in frontend
- [ ] Unit tests for parser (session header, message types, tool call extraction, compaction entries)

### Out of Scope

- Branch/subagent linking across pi sessions — pi uses `branchedFrom` but this is complex; defer
- HTML export for pi sessions — can reuse existing export infrastructure later
- Real-time file watching for pi sessions during active agent run — sync handles this via 15min periodic + startup scan

## Context

Pi-agent session format (JSONL, one JSON object per line):
- Line 1: `{"type":"session","version":3,"id":"...","timestamp":"...","cwd":"..."}`
- Subsequent lines: typed events with `id` and `parentId` fields forming a chain
- Event types: `message`, `model_change`, `thinking_level_change`, `compaction`, `branch_summary`
- Messages: `role` is "user" or "assistant"; content is an array of typed blocks:
  - `{"type":"text","text":"..."}` — text content
  - `{"type":"thinking","thinking":"...","thinkingSignature":"..."}` — thinking blocks
  - `{"type":"toolCall","id":"...","name":"...","arguments":{...}}` — tool invocations
  - `{"type":"toolResult","toolCallId":"...","toolName":"...","content":[...]}` — tool results (in user messages)
- Assistant messages include `usage`, `model`, `provider`, `stopReason`, `duration`, `ttft`

Session directory layout mirrors Claude's encoding:
- Root: `~/.pi/agent/sessions/` (or `PI_DIR` env var)
- Subdirs: encoded cwd path, e.g. `-Documents-personal-misc-agentsview/`
- Files: `2026-02-14T19-40-45-439Z_{id}.jsonl`

Oh-my-pi users point `PI_DIR` at `~/.omp/agent/sessions/`.

Pi-agent tool names (from pi-mono codebase): `read`, `write`, `edit`, `bash`, `grep`, `glob`, `find` — all lowercase, mapping to existing taxonomy categories.

## Constraints

- **Tech stack**: Go + CGO/sqlite3 — no new dependencies; follow existing patterns
- **Testing**: All new code needs table-driven tests; use `testDB(t)` helper
- **Build**: CGO_ENABLED=1, -tags fts5 required
- **No amend**: Always new commits per CLAUDE.md conventions

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Agent name "pi" not "omp" | Matches upstream project identity; omp is a local variant | — Pending |
| Default dir `~/.pi/agent/sessions` | Upstream standard; oh-my-pi users set PI_DIR=~/.omp/agent/sessions | — Pending |
| Single env var PI_DIR | Follows COPILOT_DIR/GEMINI_DIR pattern; multi-dir via PiDirs config | — Pending |
| Full content parity with Claude | User expectation; thinking blocks and tool calls are core to pi workflow | — Pending |

---
*Last updated: 2026-02-27 after initialization*

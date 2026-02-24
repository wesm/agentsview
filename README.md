# agentsview

A local web application for browsing, searching, and analyzing
AI agent coding sessions. Supports Claude Code, Codex,
Copilot CLI, Gemini CLI, and OpenCode. A next-generation rewrite of
[agent-session-viewer](https://github.com/wesm/agent-session-viewer)
in Go.

<p align="center">
  <img src="https://agentsview.io/screenshots/dashboard.png" alt="Analytics dashboard" width="720">
</p>

## Install

```bash
curl -fsSL https://agentsview.io/install.sh | bash
```

**Windows:**

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://agentsview.io/install.ps1 | iex"
```

The installer downloads the latest release, verifies the SHA-256
checksum, and installs the binary.

**Build from source** (requires Go 1.25+ with CGO and Node.js 22+):

```bash
git clone https://github.com/wesm/agentsview.git
cd agentsview
make build
make install  # installs to ~/.local/bin
```

## Why?

AI coding agents generate large volumes of session data across
projects. agentsview indexes these sessions into a local SQLite
database with full-text search, providing a web interface to
find past conversations, review agent behavior, and track usage
patterns over time.

## Features

- **Full-text search** across all message content, instantly
- **Analytics dashboard** with activity heatmaps, tool usage,
  velocity metrics, and project breakdowns
- **Multi-agent support** for Claude Code, Codex, Copilot CLI, Gemini CLI, and OpenCode
- **Live updates** via SSE as active sessions receive new messages
- **Keyboard-first** navigation (vim-style `j`/`k`/`[`/`]`)
- **Export and publish** sessions as HTML or to GitHub Gist
- **Local-first** -- all data stays on your machine, single binary,
  no accounts

## Usage

```bash
agentsview              # start server, open browser
agentsview -port 9090   # custom port
agentsview -no-browser  # headless mode
```

On startup, agentsview discovers sessions from Claude Code, Codex,
Copilot CLI, Gemini CLI, and OpenCode, syncs them into a local SQLite database
with FTS5 full-text search, and opens a web UI at
`http://127.0.0.1:8080`.

## Screenshots

| Dashboard | Session viewer |
|-----------|---------------|
| ![Dashboard](https://agentsview.io/screenshots/dashboard.png) | ![Session viewer](https://agentsview.io/screenshots/message-viewer.png) |

| Search | Activity heatmap |
|--------|-----------------|
| ![Search](https://agentsview.io/screenshots/search-results.png) | ![Heatmap](https://agentsview.io/screenshots/heatmap.png) |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Cmd+K` | Open search |
| `j` / `k` | Next / previous message |
| `]` / `[` | Next / previous session |
| `o` | Toggle sort order |
| `t` | Toggle thinking blocks |
| `e` | Export session as HTML |
| `p` | Publish to GitHub Gist |
| `r` | Sync sessions |
| `?` | Show all shortcuts |

## Documentation

Full documentation is available at
[agentsview.io](https://agentsview.io):

- [Quick Start](https://agentsview.io/quickstart/) --
  installation and first run
- [Usage Guide](https://agentsview.io/usage/) --
  dashboard, session browser, search, export
- [CLI Reference](https://agentsview.io/commands/) --
  commands, flags, and environment variables
- [Configuration](https://agentsview.io/configuration/) --
  data directory, config file, session discovery
- [Architecture](https://agentsview.io/architecture/) --
  how the sync engine, parsers, and server work

## Development

```bash
make dev            # run Go server in dev mode
make frontend-dev   # run Vite dev server (use alongside make dev)
make test           # Go tests (CGO_ENABLED=1 -tags fts5)
make lint           # golangci-lint
make e2e            # Playwright E2E tests
```

### Project Structure

```
cmd/agentsview/     CLI entrypoint
internal/config/    Configuration loading
internal/db/        SQLite operations (sessions, search, analytics)
internal/parser/    Session parsers (Claude, Codex, Copilot, Gemini, OpenCode)
internal/server/    HTTP handlers, SSE, middleware
internal/sync/      Sync engine, file watcher, discovery
frontend/           Svelte 5 SPA (Vite, TypeScript)
```

## Supported Agents

| Agent | Session Directory |
|-------|-------------------|
| Claude Code | `~/.claude/projects/` |
| Codex | `~/.codex/sessions/` |
| Copilot CLI | `~/.copilot/session-state/` |
| Gemini CLI | `~/.gemini/` |
| OpenCode | `~/.local/share/opencode/` |

Override with `CLAUDE_PROJECTS_DIR`, `CODEX_SESSIONS_DIR`,
`COPILOT_DIR`, `GEMINI_DIR`, or `OPENCODE_DIR` environment variables.

## Acknowledgements

Inspired by
[claude-history-tool](https://github.com/andyfischer/ai-coding-tools/tree/main/claude-history-tool)
by Andy Fischer and
[claude-code-transcripts](https://github.com/simonw/claude-code-transcripts)
by Simon Willison.

## License

MIT

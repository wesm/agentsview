# Claude Code Instructions

## Project Overview

agentsview is a local web viewer for AI agent sessions (Claude Code, Codex, Copilot CLI, Gemini CLI, OpenCode). It syncs session data from disk into SQLite (with FTS5 full-text search), serves a Svelte 5 SPA via an embedded Go HTTP server, and provides real-time updates via SSE.

## Architecture

```
CLI (agentsview) → Config → DB (SQLite/FTS5)
                  ↓
              File Watcher → Sync Engine → Parser (Claude, Codex, Copilot, Gemini, OpenCode)
                  ↓
              HTTP Server → REST API + SSE + Embedded SPA
```

- **Server**: HTTP server with auto-port discovery (default 8080)
- **Storage**: SQLite with WAL mode, FTS5 for full-text search
- **Sync**: File watcher + periodic sync (15min) for session directories
- **Frontend**: Svelte 5 SPA embedded in the Go binary at build time
- **Config**: Env vars (`AGENT_VIEWER_DATA_DIR`, `CLAUDE_PROJECTS_DIR`, `CODEX_SESSIONS_DIR`, `COPILOT_DIR`, `GEMINI_DIR`, `OPENCODE_DIR`) and CLI flags

## Project Structure

- `cmd/agentsview/` - Go server entrypoint
- `cmd/testfixture/` - Test data generator for E2E tests
- `internal/config/` - Config loading, flag registration, legacy migration
- `internal/db/` - SQLite operations (sessions, messages, search, analytics)
- `internal/parser/` - Session file parsers (Claude, Codex, content extraction)
- `internal/server/` - HTTP handlers, SSE, middleware, search, export
- `internal/sync/` - Sync engine, file watcher, discovery, hashing
- `internal/timeutil/` - Time parsing utilities
- `internal/web/` - Embedded frontend (dist/ copied at build time)
- `frontend/` - Svelte 5 SPA (Vite, TypeScript)
- `scripts/` - Utility scripts (E2E server, changelog)

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/agentsview/main.go` | CLI entry point, server startup, file watcher |
| `internal/server/server.go` | HTTP router and handler setup |
| `internal/server/sessions.go` | Session list/detail API handlers |
| `internal/server/search.go` | Full-text search API |
| `internal/server/events.go` | SSE event streaming |
| `internal/db/db.go` | Database open, migrations, schema |
| `internal/db/sessions.go` | Session CRUD queries |
| `internal/db/search.go` | FTS5 search queries |
| `internal/sync/engine.go` | Sync orchestration |
| `internal/parser/claude.go` | Claude Code session parser |
| `internal/parser/codex.go` | Codex session parser |
| `internal/parser/copilot.go` | Copilot CLI session parser |
| `internal/config/config.go` | Config loading, flag registration |

## Development

```bash
make build          # Build binary with embedded frontend
make dev            # Run Go server in dev mode
make frontend       # Build frontend SPA only
make frontend-dev   # Run Vite dev server (use alongside make dev)
make install        # Build and install to ~/.local/bin or GOPATH
make install-hooks  # Install pre-commit git hooks
```

After making Go code changes, always run `go fmt ./...` and `go vet ./...` before committing.

## Testing

**All new features and bug fixes must include unit tests.** Run tests before committing:

```bash
make test       # Go tests (CGO_ENABLED=1 -tags fts5)
make test-short # Fast tests only (-short flag)
make e2e        # Playwright E2E tests
make lint       # golangci-lint
make vet        # go vet
```

### Test Guidelines

- Table-driven tests for Go code
- Use `testDB(t)` helper for database tests
- Frontend: colocated `*.test.ts` files, Playwright specs in `frontend/e2e/`
- All tests use `t.TempDir()` for temp directories

## Build Requirements

- **CGO_ENABLED=1** required (sqlite3 driver)
- **Build tag**: `-tags fts5` required for full-text search
- **Frontend**: Node.js + npm for Svelte build, embedded via `internal/web/dist/`

## Conventions

- Prefer stdlib over external dependencies
- Tests should be fast and isolated
- No emojis in code or output

## Git Workflow

- **Commit every turn** - always commit your work at the end of each turn, no exceptions
- **Never amend commits** - always create new commits for fixes, never use `--amend`
- **Never change branches** - don't create, switch, or delete branches without explicit permission
- Use conventional commit messages
- Run tests before committing when applicable
- Never push or pull unless explicitly asked

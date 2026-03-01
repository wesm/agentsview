# Technology Stack

**Analysis Date:** 2026-02-27

## Languages

**Primary:**
- Go 1.25.5 - Backend server, sync engine, CLI, database operations
- TypeScript 5.9.3 - Frontend application logic and type safety
- JavaScript/Node.js 20.19+ - Frontend build tooling and scripts

**Secondary:**
- HTML/CSS (Svelte) - Frontend UI components and styling
- SQL - Database schema and queries with SQLite

## Runtime

**Environment:**
- Go 1.25.5 (minimum: 1.25+)
- Node.js 20.19, 22.12, or 24+ (frontend build)

**Package Managers:**
- Go modules (go.mod) - Go dependency management
- npm - Node.js package management
- Lockfiles: `go.sum`, `frontend/package-lock.json` (inferred from npm install)

## Frameworks

**Core:**
- Go stdlib (net/http) - HTTP server and routing
- Svelte 5.53.5 - Frontend reactive UI framework
- Vite 7.3.1 - Frontend build tool and dev server

**Database:**
- SQLite 3 (via mattn/go-sqlite3 v1.14.34) - Local data persistence with FTS5 full-text search
- SQLite FTS5 (built-in) - Full-text search indexes via virtual tables and triggers

**File Watching:**
- fsnotify v1.9.0 - Cross-platform file system event monitoring

## Key Dependencies

**Critical:**
- `github.com/mattn/go-sqlite3` v1.14.34 - SQLite C driver for Go (requires CGO_ENABLED=1)
- `github.com/tidwall/gjson` v1.18.0 - JSON parsing for session data extraction (used in Claude, Codex, Copilot, Gemini parsers)
- `github.com/fsnotify/fsnotify` v1.9.0 - File watcher for detecting session changes

**Testing & Development:**
- `github.com/stretchr/testify` v1.11.1 - Assertion and mocking framework for Go tests
- `github.com/google/go-cmp` v0.7.0 - Deep comparison utilities for testing
- `@playwright/test` 1.58.2 - E2E testing framework for frontend

**Frontend Build & Quality:**
- `@sveltejs/vite-plugin-svelte` 6.2.4 - Svelte integration with Vite
- `svelte-check` 4.4.3 - Type checking for Svelte components
- `vitest` 4.0.18 - Unit testing for frontend code
- `jsdom` 28.1.0 - DOM simulation for tests
- `dompurify` 3.3.1 - HTML sanitization in frontend
- `marked` 17.0.3 - Markdown rendering (used for message content)
- `@tanstack/virtual-core` 3.13.19 - Virtual scrolling for large message lists

**Build Tools:**
- `golangci-lint` v2.10.1 - Go linting (configured in .golangci.yml)
- `golang.org/x/mod` v0.33.0 - Module version handling for updates

## Configuration

**Environment:**
- Configured via environment variables (primary):
  - `CLAUDE_PROJECTS_DIR` - Claude Code session directory
  - `CODEX_SESSIONS_DIR` - Codex session directory
  - `COPILOT_DIR` - Copilot CLI directory
  - `GEMINI_DIR` - Gemini CLI directory
  - `OPENCODE_DIR` - OpenCode directory
  - `CURSOR_PROJECTS_DIR` - Cursor projects directory
  - `AGENT_VIEWER_DATA_DIR` - SQLite and config storage location
  - `GITHUB_TOKEN` (optional) - GitHub authentication for Gist publishing

- Config file: `~/.agentsview/config.json` (optional override for env vars)
  - Supports array fields for multiple directories per agent type
  - Cursor decryption secret (base64-encoded)

- Default data directory: `~/.agentsview/`
  - Contains: `sessions.db`, `sessions.db-wal`, `sessions.db-shm`, `debug.log`

**Build:**
- Makefile targets with build tags: `-tags fts5` (required for FTS5 support)
- Link flags: version, commit hash, build date injection at compile time
- CGO requirement: CGO_ENABLED=1 (for SQLite C bindings)
- Release builds use `-trimpath` and `-s -w` flags for optimization

## Platform Requirements

**Development:**
- Go 1.25+ with CGO enabled
- C compiler (for mattn/go-sqlite3)
- Node.js 20.19+ with npm
- Git (for version strings and commit hashing)

**Production:**
- Linux, macOS (arm64/amd64), or Windows
- Single-threaded HTTP server (no external process dependencies)
- Local filesystem access to agent session directories
- Optional: GitHub token for Gist publishing feature

## Database

**Schema:**
- Automatically migrated on startup via embedded SQL schema (`internal/db/schema.sql`)
- Tables: sessions, messages, skipped_sessions, analytics metadata
- FTS5 virtual table for full-text search with triggers for auto-indexing
- Indexes on common query paths (agent, project, timestamp)

**Features:**
- WAL mode (Write-Ahead Logging) for concurrent read access
- Memory-mapped I/O (268MB mmap window)
- Foreign keys enabled
- Cache: 64,000 pages (64MB)
- Read-only replica connection pool for queries
- Single writer connection for consistency

## Build Artifacts

**Output:**
- Single binary: `agentsview` (contains embedded Svelte SPA in `internal/web/dist/`)
- Platform-specific builds: `agentsview-darwin-arm64`, `agentsview-darwin-amd64`, `agentsview-linux-amd64`
- Installed to: `~/.local/bin/agentsview` or `$GOBIN`

---

*Stack analysis: 2026-02-27*

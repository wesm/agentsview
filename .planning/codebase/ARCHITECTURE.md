# Architecture

**Analysis Date:** 2026-02-27

## Pattern Overview

**Overall:** Layered monolith with producer-consumer sync pipeline

**Key Characteristics:**
- File-first architecture: reads JSONL session files from disk, parses into structured data, stores in SQLite
- Reactive sync: background file watcher detects changes, triggers incremental re-parsing and database updates
- REST API + embedded SPA: Go HTTP server serves API endpoints and embedded Svelte frontend
- Multi-agent support: pluggable parsers for Claude, Codex, Copilot, Gemini, OpenCode, and Cursor

## Layers

**Presentation (Frontend):**
- Purpose: Client-side UI for browsing sessions, searching, viewing analytics, managing insights
- Location: `frontend/src/`
- Contains: Svelte 5 components, stores, API client, utilities
- Depends on: REST API via `internal/server`
- Used by: Browser (embedded in Go binary via `internal/web`)

**HTTP API & Middleware:**
- Purpose: REST endpoints for sessions, analytics, search, insights; SSE for real-time updates; SPA fallback
- Location: `internal/server/`
- Contains: Handler functions, middleware (CORS, host check, logging, timeout), response serialization
- Depends on: Database (`internal/db`), sync engine (`internal/sync`), parser types (`internal/parser`)
- Used by: Frontend, external clients

**Sync Engine:**
- Purpose: Discovers session files, parses them, batches writes to database with deduplication
- Location: `internal/sync/`
- Contains: File discovery, watcher, batch orchestration, skip cache (for failed/non-interactive files)
- Depends on: Parsers (`internal/parser`), database (`internal/db`)
- Used by: Main CLI (`cmd/agentsview`), server handlers (for manual sync)

**Parsers:**
- Purpose: JSONL file format-specific extraction (messages, tool calls, metadata, fork detection)
- Location: `internal/parser/`
- Contains: Per-agent parsers (Claude, Codex, Copilot, Gemini, OpenCode, Cursor), type definitions, utilities
- Depends on: Third-party JSON libs (gjson, tidwall), time utilities (`internal/timeutil`)
- Used by: Sync engine

**Database:**
- Purpose: SQLite persistence with WAL mode, FTS5 full-text search, atomic batch writes
- Location: `internal/db/`
- Contains: Schema, CRUD operations, search queries, analytics aggregations, connection pooling
- Depends on: `github.com/mattn/go-sqlite3` (CGO)
- Used by: Sync engine, HTTP handlers

**Configuration & CLI:**
- Purpose: Flag parsing, environment variable loading, config file persistence
- Location: `internal/config/`, `cmd/agentsview/`
- Contains: Config struct, default paths, file/env/flag precedence, subcommand dispatch
- Depends on: Standard library only
- Used by: Main entry point

**Utilities:**
- Purpose: Time parsing, response formatting, insight generation (LLM-based summaries)
- Location: `internal/timeutil/`, `internal/insight/`, `internal/update/`
- Contains: Timestamp extraction, math utils, insight prompting, auto-update logic
- Depends on: Standard library, optional LLM services
- Used by: Parsers, database, server, main

## Data Flow

**Initial Startup:**

1. Load config (CLI flags → env vars → config file → defaults)
2. Open database (create if missing, validate schema, initialize FTS)
3. Create sync engine with configured directories
4. Run initial sync (discover files → parse → batch write)
5. Set up file watcher (goroutine per root, debounced)
6. Start periodic sync (15 min timer)
7. Create HTTP server, route setup, middleware binding
8. Listen and serve

**Session File Sync (on file change or timer):**

1. Discover files in all configured directories
2. Hash check (if mtime/hash unchanged, skip)
3. Check skip cache (parse errors or non-interactive sessions skipped until file changes)
4. Parse with appropriate agent-specific parser:
   - Extract messages, tool calls, timestamps
   - Detect DAG forks (Claude only)
   - Generate session IDs and metadata
5. Write to database in transaction:
   - Delete old messages for session
   - Insert new messages with tool calls
   - Upsert session record
   - Update FTS index
6. Cache skip status (if parse error or empty result)
7. Emit sync progress events

**Runtime API Request:**

1. Request arrives at HTTP server
2. Host check middleware validates Host header (DNS rebinding protection)
3. CORS middleware validates Origin (for mutating methods)
4. Log middleware records method + path
5. Timeout middleware wraps handler with 30s deadline
6. Handler function:
   - Parse query/body parameters
   - Call database method with context
   - Handle context cancellation/timeout
   - Serialize response to JSON
7. Response sent with middleware-set headers

**Real-time Session Watch (SSE):**

1. Client opens `/api/v1/sessions/{id}/watch` connection
2. Server starts file monitor goroutine (polls source file every 1.5s)
3. On file change detected: triggers `SyncSingleSession` in sync engine
4. Monitor sends event to client on each sync
5. Connection held open with heartbeat (every ~30s)
6. Client closes → context cancellation → monitor stops

**State Management:**

- **Sync state:** Engine tracks last sync time, skip cache (in-memory + persistent in DB), pending file changes
- **Server state:** HTTP server holds references to config, database, engine, embedded SPA filesystem
- **Session state:** Database is authoritative; sync engine polls filesystem for changes
- **UI state:** Frontend stores use Svelte 5 runes (reactive signals) for session filters, selected tab, search query, pagination cursor

## Key Abstractions

**Engine:**
- Purpose: Orchestrate discovery, parsing, and writing for all session sources
- Examples: `internal/sync/engine.go` (SyncAll, SyncPaths, ResyncAll, FindSourceFile)
- Pattern: Batch processing with worker pool (max 8 workers) and skip cache to avoid re-parsing

**Session Parser Interface:**
- Purpose: Pluggable format-specific extraction logic
- Examples: `internal/parser/claude.go`, `internal/parser/codex.go`, `internal/parser/copilot.go`, `internal/parser/cursor.go`, `internal/parser/gemini.go`, `internal/parser/opencode.go`
- Pattern: Each parser receives file path → returns `ParseResult` (session metadata + messages + tool calls)

**Database Connection Pool:**
- Purpose: Separate read/write with atomic swaps for resync operations
- Examples: `internal/db/db.go` (writer, reader pools with atomic.Pointer)
- Pattern: Write lock serializes all mutations; reader pool (4 conns) allows concurrent reads; retired pools hold connections during swap window

**Server Handler Options:**
- Purpose: Dependency injection for version info, LLM function, custom filesystem
- Examples: `internal/server/server.go` (WithVersion, WithGenerateFunc, option pattern)
- Pattern: Variadic functional options applied after struct creation

**Middleware Chain:**
- Purpose: Compose security, logging, and timeout concerns without routing logic
- Examples: `internal/server/server.go` (hostCheckMiddleware, corsMiddleware, logMiddleware)
- Pattern: Each middleware wraps the next, returns http.Handler

## Entry Points

**CLI Binary:**
- Location: `cmd/agentsview/main.go`
- Triggers: `agentsview [serve|prune|update]` commands or default to serve
- Responsibilities: Parse flags, load config, open DB, create sync engine, start file watcher and periodic sync, run HTTP server

**HTTP Handlers:**
- Location: `internal/server/sessions.go`, `internal/server/search.go`, `internal/server/analytics.go`, `internal/server/events.go`, `internal/server/insights.go`
- Triggers: HTTP requests matching routes in `internal/server/server.go`
- Responsibilities: Parse parameters, validate input, call database/engine, serialize responses

**Sync Goroutines:**
- Location: `cmd/agentsview/main.go` (startFileWatcher, startPeriodicSync, startUnwatchedPoll), `internal/server/events.go` (sessionMonitor)
- Triggers: Server startup (file watcher, periodic) or SSE client connect (session monitor)
- Responsibilities: Detect changes, trigger sync, emit events

**Frontend Routes:**
- Location: `frontend/src/routes/` (SvelteKit)
- Triggers: Browser navigation
- Responsibilities: Fetch data from API, render UI, manage stores

## Error Handling

**Strategy:** Errors are logged, returned to caller, or written to HTTP response depending on context

**Patterns:**

- **Parse errors:** Logged, session skipped and cached (marked for retry on file mtime change)
- **Database errors:** Returned as is; HTTP handlers write 500 with error message
- **Context errors:** Checked by `handleContextError()` (logs 504 on timeout, silent on cancel)
- **Validation errors:** HTTP handlers return 400 with message (invalid date format, cursor signature, parameter bounds)
- **Graceful degradation:** FTS optional (checked at init, non-fatal if fts5 module missing)

## Cross-Cutting Concerns

**Logging:**
- Main: `log.Printf()` to stdout (redirected to `~/.agentsview/debug.log` after setup)
- Rotation: Log file truncated on startup if >10MB
- Level: Informational (sync progress, warnings, errors)

**Validation:**
- Query parameters: clamped limits (0-500 sessions), date format check (YYYY-MM-DD), timestamp RFC3339
- Host/Origin headers: DNS rebinding protection (host/origin must match configured listener)
- Cursor: HMAC signature verification

**Authentication:**
- No user auth; local-only (127.0.0.1 or localhost default)
- Optional GitHub token for public session publishing (optional feature)
- Cursor secret: auto-generated 32-byte key, stored in config.json

**Concurrency:**
- Sync: All operations guarded by `Engine.syncMu` (serializes to one sync at a time)
- Database: Write lock (`db.mu`) serializes writes; atomic pointers allow lock-free reads
- File watcher: Platform-specific (fsnotify on Unix, WinFileNotify on Windows)
- File monitoring (SSE): Per-session goroutines polling file, closed on ctx cancel

---

*Architecture analysis: 2026-02-27*

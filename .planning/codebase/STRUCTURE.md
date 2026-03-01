# Codebase Structure

**Analysis Date:** 2026-02-27

## Directory Layout

```
agentsview/
├── cmd/
│   ├── agentsview/             # CLI entry point and subcommands
│   │   ├── main.go             # Server startup, file watcher setup
│   │   ├── prune.go            # Session deletion command
│   │   ├── prune_test.go
│   │   ├── update.go           # Auto-update command
│   │   └── main_test.go
│   └── testfixture/            # E2E test data generator
│       └── main.go
├── internal/
│   ├── config/                 # Configuration loading and persistence
│   │   ├── config.go           # Config struct, flag registration, precedence
│   │   ├── config_test.go
│   │   └── persistence_test.go
│   ├── db/                     # SQLite database operations
│   │   ├── db.go               # Connection pooling, schema init, WAL mode
│   │   ├── db_test.go
│   │   ├── schema.sql          # Database schema (embedded)
│   │   ├── sessions.go         # Session CRUD, pagination cursor
│   │   ├── messages.go         # Message operations
│   │   ├── search.go           # FTS5 full-text search
│   │   ├── analytics.go        # Analytics aggregations
│   │   ├── analytics_test.go
│   │   ├── insights.go         # Insight management
│   │   ├── insights_test.go
│   │   ├── skipped.go          # Skip cache persistence
│   │   ├── skipped_test.go
│   │   ├── stats.go            # Database statistics
│   │   └── filter_test.go
│   ├── parser/                 # JSONL session file parsers
│   │   ├── types.go            # Common types (Session, Message, ToolCall)
│   │   ├── types_test.go
│   │   ├── claude.go           # Claude Code parser with fork detection
│   │   ├── claude_parser_test.go
│   │   ├── claude_subagent_test.go
│   │   ├── codex.go            # Codex parser
│   │   ├── codex_parser_test.go
│   │   ├── copilot.go          # Copilot CLI parser
│   │   ├── copilot_test.go
│   │   ├── cursor.go           # Cursor parser
│   │   ├── cursor_test.go
│   │   ├── gemini.go           # Gemini CLI parser
│   │   ├── gemini_parser_test.go
│   │   ├── opencode.go         # OpenCode parser
│   │   ├── opencode_test.go
│   │   ├── content.go          # Message content extraction
│   │   ├── project.go          # Project metadata extraction
│   │   ├── project_git_test.go
│   │   ├── taxonomy.go         # Tool call categorization
│   │   ├── taxonomy_test.go
│   │   ├── timestamp.go        # Time parsing utilities
│   │   ├── linereader.go       # Large file line reading
│   │   ├── linereader_test.go
│   │   ├── open_nofollow_unix.go   # Secure file opening (Unix)
│   │   ├── open_nofollow_windows.go # Secure file opening (Windows)
│   │   ├── parser_test.go
│   │   ├── fork_test.go
│   │   └── test_helpers_test.go
│   ├── server/                 # HTTP API and middleware
│   │   ├── server.go           # Route setup, middleware chain, handler options
│   │   ├── server_test.go
│   │   ├── sessions.go         # Session list/detail handlers
│   │   ├── messages.go         # Message detail handler
│   │   ├── search.go           # Full-text search handler
│   │   ├── search_test.go
│   │   ├── analytics.go        # Analytics endpoint handlers
│   │   ├── analytics_test.go
│   │   ├── insights.go         # Insight CRUD and generation handlers
│   │   ├── insights_test.go
│   │   ├── events.go           # SSE session watch, file monitor
│   │   ├── events_test.go
│   │   ├── export.go           # Session export to JSON
│   │   ├── export_test.go
│   │   ├── upload.go           # Session upload endpoint
│   │   ├── upload_failure_test.go
│   │   ├── middleware.go       # CORS, host check, logging, security
│   │   ├── middleware_test.go
│   │   ├── params.go           # Query parameter parsing
│   │   ├── params_test.go
│   │   ├── response.go         # JSON response helpers
│   │   ├── sse.go              # Server-sent events utilities
│   │   ├── timeout_custom_test.go
│   │   ├── deadline_test.go
│   │   ├── deadline_internal_test.go
│   │   ├── timeout_test.go
│   │   └── helpers_internal_test.go
│   ├── sync/                   # File watching and sync orchestration
│   │   ├── engine.go           # Main sync orchestrator, batch processing
│   │   ├── engine_test.go
│   │   ├── engine_integration_test.go
│   │   ├── discovery.go        # File discovery for each agent type
│   │   ├── watcher.go          # File system watcher abstraction
│   │   ├── watcher_test.go
│   │   ├── hash.go             # File hashing for change detection
│   │   ├── hash_test.go
│   │   ├── progress.go         # Progress tracking and reporting
│   │   ├── progress_test.go
│   │   ├── sync_test.go
│   │   ├── common_helpers_test.go
│   │   └── test_helpers_test.go
│   ├── insight/                # LLM-based insight generation
│   │   ├── generate.go         # Insight generation logic
│   │   ├── generate_test.go
│   │   ├── prompt.go           # Prompt construction for LLMs
│   │   └── prompt_test.go
│   ├── web/                    # Embedded frontend assets
│   │   ├── embed.go            # Filesystem embedding
│   │   └── dist/               # Built SPA (generated at build time)
│   ├── update/                 # Auto-update logic
│   │   ├── update.go
│   │   └── update_test.go
│   ├── timeutil/               # Time parsing utilities
│   │   ├── timeutil.go
│   │   └── timeutil_test.go
│   ├── dbtest/                 # Test helpers for database operations
│   │   └── dbtest.go
│   ├── testjsonl/              # Test fixture generation
│   │   └── testjsonl.go
│   └── ... (other internal packages)
├── frontend/                   # Svelte 5 SPA
│   ├── src/
│   │   ├── lib/
│   │   │   ├── api/            # API client and type definitions
│   │   │   │   ├── client.ts   # REST API client
│   │   │   │   ├── client.test.ts
│   │   │   │   ├── types.ts    # Re-export of types/index.ts
│   │   │   │   └── types/      # API response types by feature
│   │   │   │       ├── core.ts       # Session, Message, Analytics
│   │   │   │       ├── analytics.ts
│   │   │   │       ├── insights.ts
│   │   │   │       ├── sync.ts
│   │   │   │       ├── github.ts
│   │   │   │       └── index.ts      # Central export
│   │   │   ├── stores/         # Svelte 5 reactive stores
│   │   │   │   ├── sessions.svelte.ts     # Session list, filtering
│   │   │   │   ├── sessions.test.ts
│   │   │   │   ├── analytics.svelte.ts    # Analytics data
│   │   │   │   ├── analytics.test.ts
│   │   │   │   ├── search.svelte.ts       # Search queries
│   │   │   │   ├── search.test.ts
│   │   │   │   ├── insights.svelte.ts     # Insights list
│   │   │   │   ├── insights.test.ts
│   │   │   │   ├── messages.svelte.ts     # Current message detail
│   │   │   │   ├── messages.test.ts
│   │   │   │   ├── router.svelte.ts       # Current route/params
│   │   │   │   ├── router.test.ts
│   │   │   │   ├── ui.svelte.ts           # UI state (sidebar open, etc)
│   │   │   │   ├── ui.test.ts
│   │   │   │   ├── sync.svelte.ts         # Sync status
│   │   │   │   └── sync.test.ts
│   │   │   ├── utils/          # Helper functions
│   │   │   │   ├── format.ts           # Date/number formatting
│   │   │   │   ├── format.test.ts
│   │   │   │   ├── agents.ts           # Agent type utilities
│   │   │   │   ├── agents.test.ts
│   │   │   │   ├── content-parser.ts   # Message content parsing
│   │   │   │   ├── content-parser.test.ts
│   │   │   │   ├── markdown.ts         # Markdown rendering
│   │   │   │   ├── markdown.test.ts
│   │   │   │   ├── csv-export.ts       # CSV export logic
│   │   │   │   ├── csv-export.test.ts
│   │   │   │   ├── clipboard.ts        # Clipboard operations
│   │   │   │   ├── clipboard.test.ts
│   │   │   │   ├── cache.ts            # Simple cache utility
│   │   │   │   ├── cache.test.ts
│   │   │   │   ├── poll.ts             # Polling utility
│   │   │   │   ├── poll.test.ts
│   │   │   │   ├── keyboard.ts         # Keyboard event handling
│   │   │   │   ├── keyboard.test.ts
│   │   │   │   ├── tool-params.ts      # Tool parameter formatting
│   │   │   │   ├── tool-params.test.ts
│   │   │   │   ├── display-items.ts    # Filtering/sorting utilities
│   │   │   │   ├── display-items.test.ts
│   │   │   │   └── debounce.ts
│   │   │   ├── virtual/        # Virtual scrolling
│   │   │   │   ├── createVirtualizer.svelte.ts
│   │   │   │   ├── createVirtualizer.test.ts
│   │   │   │   └── createVirtualizer.cache.test.ts
│   │   │   └── components/     # Svelte components (not listed, see routes/)
│   │   ├── routes/             # SvelteKit pages and layouts
│   │   │   ├── +page.svelte    # Session list page
│   │   │   ├── +layout.svelte  # Root layout
│   │   │   ├── sessions/       # Session detail routes
│   │   │   │   ├── [id]/       # Individual session
│   │   │   │   │   └── +page.svelte
│   │   │   │   └── components/ # Shared session components
│   │   │   ├── analytics/      # Analytics pages
│   │   │   │   ├── +page.svelte
│   │   │   │   └── components/
│   │   │   ├── search/         # Search results page
│   │   │   │   └── +page.svelte
│   │   │   └── insights/       # Insights pages
│   │   │       ├── +page.svelte
│   │   │       └── [id]/
│   │   ├── main.ts             # App entry point
│   │   ├── app.css             # Global styles
│   │   ├── vite-env.d.ts       # Vite type definitions
│   │   └── app.svelte          # Root component
│   ├── e2e/                    # Playwright end-to-end tests
│   │   └── *.spec.ts
│   ├── vite.config.ts          # Vite bundler config
│   ├── svelte.config.js        # SvelteKit config
│   ├── package.json
│   ├── tsconfig.json
│   └── ...
├── scripts/                    # Utility scripts
│   ├── build.sh               # Build script
│   ├── e2e-server.go          # E2E test server launcher
│   └── changelog.js           # Changelog generator
├── Makefile                   # Build targets (build, dev, test, install)
├── go.mod                     # Go dependencies
├── go.sum
├── go.work                    # Go workspace (if applicable)
├── package.json               # Root (might not exist; frontend has own)
└── .planning/                 # GSD planning documents
    └── codebase/              # This folder
        ├── ARCHITECTURE.md
        ├── STRUCTURE.md
        ├── CONVENTIONS.md
        ├── TESTING.md
        ├── STACK.md
        ├── INTEGRATIONS.md
        └── CONCERNS.md
```

## Directory Purposes

**`cmd/agentsview/`:**
- Purpose: CLI entry point and subcommands
- Contains: Flag parsing, config loading, server startup, file watcher initialization, subcommand dispatch (serve/prune/update)
- Key files: `main.go` (server lifecycle), `prune.go` (session deletion), `update.go` (version checking)

**`internal/config/`:**
- Purpose: Configuration management with three-layer precedence
- Contains: Config struct, environment variable loading, JSON file persistence, flag registration
- Key files: `config.go` (Config type, Load/LoadMinimal, flag helpers)

**`internal/db/`:**
- Purpose: SQLite database operations and schema
- Contains: Connection pooling (write + read), FTS5 full-text search, CRUD operations for sessions/messages, analytics aggregations
- Key files: `db.go` (connection management, WAL mode), `sessions.go` (session queries with pagination), `search.go` (FTS), `analytics.go` (aggregations)

**`internal/parser/`:**
- Purpose: Agent-specific JSONL file parsing
- Contains: Type definitions (Session, Message, ToolCall), per-agent parsers (Claude, Codex, Copilot, etc), utilities (content extraction, fork detection, timestamp parsing)
- Key files: `types.go` (core types), `claude.go` (fork-aware parser), `codex.go`, `copilot.go`, `cursor.go`, `gemini.go`, `opencode.go`

**`internal/server/`:**
- Purpose: HTTP API endpoints, middleware, real-time features
- Contains: Route handlers (sessions, search, analytics, insights), middleware chain (CORS, host check, logging, timeout), SSE event streaming
- Key files: `server.go` (route setup, middleware), `sessions.go` (list/detail), `search.go` (FTS API), `events.go` (SSE watch), `middleware.go` (security)

**`internal/sync/`:**
- Purpose: File discovery, parsing orchestration, database batch writes
- Contains: Engine (main orchestrator), discovery (per-agent file finding), watcher (file system monitoring), batching logic
- Key files: `engine.go` (SyncAll, SyncPaths, skip cache), `discovery.go` (file discovery), `watcher.go` (platform-specific monitoring)

**`internal/insight/`:**
- Purpose: LLM-based session summarization
- Contains: Insight generation (calls external LLM), prompt construction
- Key files: `generate.go` (generates insights from sessions), `prompt.go` (builds prompts)

**`internal/web/`:**
- Purpose: Embed frontend assets into binary
- Contains: Built SPA files (dist/), embedding logic
- Key files: `embed.go` (filesystem embedding), `dist/` (compiled Svelte app)

**`frontend/src/lib/api/`:**
- Purpose: API client and type definitions
- Contains: REST client, type definitions for all API responses (by feature)
- Key files: `client.ts` (REST calls), `types/index.ts` (all types)

**`frontend/src/lib/stores/`:**
- Purpose: Svelte 5 reactive state management
- Contains: One store per major feature (sessions, analytics, search, insights, messages, router, ui, sync)
- Key files: Each `.svelte.ts` file is a store module

**`frontend/src/lib/utils/`:**
- Purpose: Shared utility functions
- Contains: Formatting, parsing, DOM utilities, cache, polling
- Key files: `format.ts`, `content-parser.ts`, `markdown.ts`, `csv-export.ts`

**`frontend/src/routes/`:**
- Purpose: SvelteKit pages (mounted at URL paths)
- Contains: Page components, layouts, route-specific logic
- Typical structure: `+page.svelte` for page, `+layout.svelte` for layout, `components/` for local components

## Key File Locations

**Entry Points:**
- `cmd/agentsview/main.go`: CLI entry, server startup
- `frontend/src/main.ts`: Frontend bootstrap
- `frontend/src/routes/+page.svelte`: Root page (session list)

**Configuration:**
- `internal/config/config.go`: Config loading and persistence
- `Makefile`: Build targets
- `go.mod`: Go dependencies
- `frontend/package.json`: Frontend dependencies

**Core Logic:**
- `internal/sync/engine.go`: Sync orchestration
- `internal/parser/claude.go`: Claude parser with fork detection
- `internal/db/db.go`: Database connection management
- `internal/server/server.go`: HTTP routing and middleware

**Testing:**
- `*_test.go`: Unit tests (colocated with source)
- `internal/dbtest/dbtest.go`: Database test helpers
- `internal/testjsonl/testjsonl.go`: Test fixture generators
- `frontend/e2e/`: Playwright E2E tests
- `frontend/src/**/*.test.ts`: Frontend unit tests

## Naming Conventions

**Files:**
- Source: `lowercase_with_underscores.go` or `camelCase.ts`
- Tests: `{source}_test.go` or `{source}.test.ts`
- Packages: `snake_case` directories matching package names
- Embedded: `schema.sql`, `dist/` (generated)

**Directories:**
- Internal packages: `internal/{feature}/` (singular: `internal/parser`, `internal/db`, `internal/server`)
- Commands: `cmd/{binary_name}/`
- Frontend: `frontend/` (npm project), `src/lib/` (library code), `src/routes/` (pages)

**Functions:**
- Go: `camelCase` starting with lowercase, exported functions uppercase
- TypeScript: `camelCase` for functions, `PascalCase` for classes/components
- Handlers: `handleXxx` (e.g., `handleListSessions`)
- Store methods: `{action}` or `subscribe` pattern (Svelte 5)

**Types/Structs:**
- Go: `PascalCase` (e.g., `Session`, `ParsedMessage`)
- TypeScript: `PascalCase` interfaces/types (e.g., `Session`, `Analytics`)
- Internal types: Prefix with package name in docs, not in code (e.g., `parser.AgentType`)

## Where to Add New Code

**New Feature (Backend):**
- Primary logic: `internal/sync/engine.go` (if sync-related) or `internal/server/{feature}.go` (if API-related) or `internal/db/{feature}.go` (if data-related)
- Tests: `internal/{package}/{feature}_test.go` (colocated)
- Entry point: Add handler to `internal/server/server.go` routes; add CLI flag to `internal/config/config.go`

**New Agent Support:**
- Parser: `internal/parser/{agent_name}.go` (follow `claude.go` pattern)
- Discovery: Add `Discover{AgentName}` function to `internal/sync/discovery.go`, call from `internal/sync/engine.go`
- Config: Add directory flag to `internal/config/config.go` (single + array variants)
- Tests: `internal/parser/{agent_name}_test.go` with test fixtures

**New API Endpoint:**
- Handler: `internal/server/{feature}.go` with `func (s *Server) handle{EndpointName}(...)`
- Route: Add to `s.routes()` in `internal/server/server.go`
- Type: Add to `frontend/src/lib/api/types/{feature}.ts`
- Client: Add method to `APIClient` in `frontend/src/lib/api/client.ts`

**New Frontend Feature:**
- Store: `frontend/src/lib/stores/{feature}.svelte.ts`
- Page: `frontend/src/routes/{feature}/+page.svelte`
- Components: `frontend/src/routes/{feature}/components/` or `frontend/src/lib/components/`
- Utilities: `frontend/src/lib/utils/{feature}.ts`
- Tests: Colocated `.test.ts` files

**Utility Functions:**
- Shared helpers: `internal/timeutil/`, `internal/parser/` (content, project)
- Frontend utils: `frontend/src/lib/utils/`

## Special Directories

**`internal/dbtest/`:**
- Purpose: Database test helpers (fixtures, cleanup)
- Generated: No
- Committed: Yes

**`internal/testjsonl/`:**
- Purpose: Generate test JSONL fixtures for E2E tests
- Generated: No (code generates fixtures at test time)
- Committed: Yes

**`frontend/dist/`:**
- Purpose: Built SPA (compiled Svelte, JavaScript, CSS)
- Generated: Yes (by `npm run build` or `make frontend`)
- Committed: No (in `.gitignore`)

**`internal/web/dist/`:**
- Purpose: Symlink or copy of `frontend/dist/` at build time
- Generated: Yes (by build script)
- Committed: No

**`~/.agentsview/` (at runtime):**
- Purpose: Database, config, logs
- Files: `sessions.db`, `sessions.db-wal`, `config.json`, `debug.log`
- Created: On first run

---

*Structure analysis: 2026-02-27*

# External Integrations

**Analysis Date:** 2026-02-27

## APIs & External Services

**GitHub API:**
- GitHub Releases API (https://api.github.com/repos/wesm/agentsview/releases/latest) - Checking for application updates
  - SDK/Client: Go standard library (`net/http`)
  - Auth: Optional personal access token via `GITHUB_TOKEN` env var (for higher rate limits)
  - Files: `internal/update/update.go`

- GitHub Gist API (https://api.github.com/gists) - Publishing session exports as public gists
  - SDK/Client: Go standard library (`net/http`)
  - Auth: Required user GitHub token stored in config (`github_token` in `~/.agentsview/config.json`)
  - Files: `internal/server/export.go` (function `createGist()`)
  - Endpoint: POST to create gist, returns gist ID and HTML URL
  - Rate limits: Subject to GitHub API rate limits (60 unauthenticated, 5000 authenticated per hour)

## Data Storage

**Databases:**
- SQLite 3 (local file-based)
  - Connection: `~/.agentsview/sessions.db`
  - Client: `github.com/mattn/go-sqlite3` v1.14.34
  - Features: FTS5 full-text search, WAL mode, memory-mapped I/O
  - Related files: `internal/db/db.go`, `internal/db/sessions.go`, `internal/db/search.go`, `internal/db/schema.sql`

**File Storage:**
- Local filesystem only
  - Session data parsed from disk: Claude Code, Codex, Copilot CLI, Gemini CLI, OpenCode, Cursor
  - Data extracted from: `~/.claude/projects/`, `~/.codex/sessions/`, `~/.copilot/`, `~/.gemini/`, `~/.local/share/opencode/`, `~/.cursor/projects/`
  - File watcher monitors changes via `fsnotify` (15-minute periodic sync fallback)
  - Related files: `internal/parser/*.go`, `internal/sync/engine.go`, `internal/sync/watcher.go`

**Caching:**
- Update check cache: `~/.agentsview/update_check.json`
  - 1 hour TTL for production builds, 15 minutes for dev builds
  - Prevents excessive GitHub API calls
  - Files: `internal/update/update.go`

## Authentication & Identity

**GitHub:**
- Optional token for Gist publishing: `GITHUB_TOKEN` or `github_token` in config
- Token scopes required: `gist` (for create/edit/delete)
- Authentication method: Bearer token in Authorization header
- Files: `internal/server/export.go` (function `createGist()`)
- Fallback: Error response if token not configured

**Cursor Agent (Encrypted):**
- Cursor sessions are encrypted; decryption requires a secret key
- Secret: base64-encoded in config (`cursor_secret` in `~/.agentsview/config.json`) or CLI flag
- Decryption: Applied during session parsing
- Files: `internal/db/db.go` (SetCursorSecret), `internal/parser/cursor.go`

## Monitoring & Observability

**Error Tracking:**
- None detected (no Sentry, Rollbar, or similar integration)

**Logging:**
- Console output during startup/shutdown
- Debug log file: `~/.agentsview/debug.log` (max 10MB, truncated on startup)
- Log output: `log.SetOutput()` redirects stdlib logging to file
- Files: `cmd/agentsview/main.go` (setupLogFile, truncateLogFile), all packages use standard `log` package

## CI/CD & Deployment

**Hosting:**
- Self-hosted single binary, no cloud platform required
- Local HTTP server bound to `127.0.0.1:8080` (default, configurable via `-port` or `-host` flags)
- Auto-discovery of available port if default in use

**Update Mechanism:**
- Binary self-update feature via GitHub Releases
- Downloads from: `https://api.github.com/repos/wesm/agentsview/releases/latest`
- Checksums verified against SHA256SUMS asset
- Installation: Replace binary in place or use installer scripts
- Files: `internal/update/update.go` (CheckForUpdate, InstallUpdate)
- Installer scripts: `https://agentsview.io/install.sh` (Linux/macOS), `https://agentsview.io/install.ps1` (Windows)

**Build Tags:**
- `-tags fts5` required for SQLite full-text search compilation
- CGO_ENABLED=1 required (not cross-compilable to different OS without C compiler toolchain)

## Environment Configuration

**Required env vars:**
- None (all have defaults)

**Optional env vars (override defaults):**
- `CLAUDE_PROJECTS_DIR` - Claude Code directory (default: `~/.claude/projects`)
- `CODEX_SESSIONS_DIR` - Codex directory (default: `~/.codex/sessions`)
- `COPILOT_DIR` - Copilot directory (default: `~/.copilot`)
- `GEMINI_DIR` - Gemini directory (default: `~/.gemini`)
- `OPENCODE_DIR` - OpenCode directory (default: `~/.local/share/opencode`)
- `CURSOR_PROJECTS_DIR` - Cursor directory (default: `~/.cursor/projects`)
- `AGENT_VIEWER_DATA_DIR` - Data directory (default: `~/.agentsview`)
- `GITHUB_TOKEN` - GitHub personal access token (optional, for update checks with higher rate limits)

**Secrets location:**
- `~/.agentsview/config.json`:
  - `github_token` - GitHub Gist publishing (plaintext in config file)
  - `cursor_secret` - Cursor decryption key (base64-encoded)
- `.env` files: Not detected (no dotenv integration)
- Note: Secrets stored in plain config file; not encrypted at rest

**Config file location:**
- `~/.agentsview/config.json` (optional)
- Supports array fields for multiple directories: `claude_project_dirs`, `codex_sessions_dirs`, etc.
- Env vars override config file values

## Webhooks & Callbacks

**Incoming:**
- None detected (read-only application)

**Outgoing:**
- GitHub Gist API (one-way POST for session publishing)
- Optional: Custom Cursor decryption (no external service, local only)

## Session Data Parsers

**Supported Formats:**
- Claude Code (JSON/JSONL format) - Files: `internal/parser/claude.go`
- Codex (JSONL format) - Files: `internal/parser/codex.go`
- Copilot CLI (JSON state format) - Files: `internal/parser/copilot.go`
- Gemini CLI (JSONL format) - Files: `internal/parser/gemini.go`
- OpenCode (SQLite database format) - Files: `internal/parser/opencode.go`
- Cursor (encrypted sessions) - Files: `internal/parser/cursor.go`

**Content Extraction:**
- Markdown rendering with `marked` library (frontend: `frontend/src/...`)
- HTML sanitization with `dompurify` library (frontend)
- JSON parsing with `tidwall/gjson` (backend)
- Plain text and code block extraction

---

*Integration audit: 2026-02-27*

# Coding Conventions

**Analysis Date:** 2026-02-27

## Naming Patterns

**Files (Go):**
- Package files: `lowercase_with_underscores.go` (e.g., `messages.go`, `sessions.go`)
- Test files: `{subject}_test.go` (e.g., `db_test.go`, `claude_parser_test.go`)
- Internal test files: `{subject}_internal_test.go` (e.g., `helpers_internal_test.go`)
- Export files: `{function}_test.go` for focused test files (e.g., `fork_test.go`)

**Files (TypeScript/Frontend):**
- Components and utilities: `kebab-case.ts` or `kebab-case.svelte` (e.g., `content-parser.ts`, `clipboard.ts`)
- Test files: `{subject}.test.ts` colocated with source (e.g., `format.test.ts` next to `format.ts`)
- E2E test specs: `kebab-case.spec.ts` (e.g., `message-content.spec.ts`, `session-list.spec.ts`)

**Functions (Go):**
- Public: `PascalCase` (e.g., `ParseClaudeSession`, `ListSessions`)
- Private: `camelCase` (e.g., `getReader`, `makeJSON`, `parseIntParam`)
- Package-level helpers: `camelCase` (e.g., `requireCount`, `sessionSet`)
- Test helpers: `camelCase` with `t.Helper()` call (e.g., `assertSessionMeta`, `requireNoError`)

**Functions (TypeScript):**
- Public: `camelCase` (e.g., `formatRelativeTime`, `truncate`, `sanitizeSnippet`)
- Private/internal: `camelCase` with underscore prefix (e.g., `_resetNonceCounter`)

**Variables (Go):**
- Loop variables: single letters for short iterations (e.g., `for i := 0; i < len(slice); i++`)
- Receiver names: short (2-3 chars typically, e.g., `r *Request`, `w http.ResponseWriter`)
- Temporary stores in tests: uppercase (e.g., `session`, `message`, `config`)

**Variables (TypeScript):**
- Constants: `UPPER_SNAKE_CASE` (e.g., `SESSION_PAGE_SIZE`, `MINUTE`, `HOUR`)
- Configuration objects: PascalCase for type, camelCase for instances (e.g., `type Filters`, `filters = {...}`)

**Types (Go):**
- Exported: `PascalCase` (e.g., `Session`, `ParseResult`, `SessionFilter`)
- Interface types: `PascalCase` (e.g., `Reader`, `Writer`)
- Struct tags: `snake_case` (e.g., `json:"message_count"`)

**Types (TypeScript):**
- Exported: `PascalCase` (e.g., `Session`, `ProjectInfo`, `AgentInfo`)
- Internal types: `PascalCase` (e.g., `Filters`, `SessionGroup`)

## Code Style

**Formatting (Go):**
- Tool: `gofmt` (automatic via `make build` and testing)
- Line length: No enforced limit, but keep reasonable (typically <100 chars when practical)
- Indentation: Tabs (default Go style)
- Spacing: Single space between logical sections, blank lines between major blocks

**Formatting (TypeScript):**
- Tool: Vite's built-in (SvelteKit defaults, no .prettierrc file)
- Line length: ~100 chars practical limit
- Indentation: 2 spaces
- Semicolons: Required (enforced by TypeScript)

**Linting (Go):**
- Tool: `golangci-lint` (run with `make lint`)
- Vet: `go vet -tags fts5 ./...` (run with `make vet`)
- Pre-commit hook: Enforces `go fmt`, `go vet`, and `go mod tidy`

**Linting (TypeScript):**
- Tool: `svelte-check` (run with `npm run check`)
- No ESLint/Prettier config files - uses SvelteKit defaults

## Import Organization

**Go (stdlib → external → internal):**
```go
import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tidwall/gjson"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/config"
)
```

**TypeScript (side effects → default → named → types):**
```typescript
import * as api from "../api/client.js";
import type { Session, ProjectInfo } from "../api/types.js";
import { describe, it, expect, vi } from "vitest";
import type { ListSessionsParams } from "../api/client.js";
```

**Path Organization:**
- Go: No path aliases, uses full package paths (e.g., `github.com/wesm/agentsview/internal/db`)
- TypeScript: Relative imports with explicit `.js` extensions (e.g., `../api/client.js`)

## Error Handling

**Go Patterns:**
- Explicit error returns: `func() error` or `(T, error)`
- Error wrapping: `fmt.Errorf("context: %w", err)` for context-specific wrapping
- Error checking: `if err != nil { ... }` (not shortcuts like `must` or `panic`)
- Context errors: Use `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)`
- HTTP errors: Centralized `writeError(w, status, msg)` helper function
- Logging errors: `log.Printf("operation: %v", err)` for non-fatal errors

**Example from `internal/server/response.go`:**
```go
func handleContextError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "gateway timeout")
		return true
	}
	return false
}
```

**TypeScript Patterns:**
- Explicit error throwing: `throw new Error("message")`
- Promise rejection: Return rejected Promise in async functions
- Try-catch: Used for async operations in tests and main code
- API error class: `ApiError` with proper error details
- Graceful degradation: Mock-friendly error handling in client code

## Logging

**Framework:**
- Go: Standard `log` package (not `logrus` or other external loggers)
- TypeScript: `console.log` in stores/utilities; tests mock where needed

**Patterns (Go):**
- Startup/shutdown: `log.Printf("message: %v", info)`
- Warnings (non-fatal): `log.Printf("operation failed: %v", err)` for JSON encoding, file ops
- Info about sync: `log.Printf("loading skip cache: %v", err)`
- Never log before critical paths are initialized (database, config)

**Patterns (TypeScript):**
- No production logging in stores or utilities
- Tests use `vi.mock` for API logging
- User feedback via UI state, not console

## Comments

**When to Comment (Go):**
- Public functions/types: Always use comment above declaration (e.g., `// ParseClaudeSession parses...`)
- Complex logic: Explain "why", not "what" (code shows what, comment explains rationale)
- Special behaviors: Document non-obvious behavior (e.g., fork detection thresholds)
- Test helper comments: Explain what the helper does (e.g., `// sessionSet inserts 3 sessions...`)

**JSDoc/Comments (TypeScript):**
- Public functions: JSDoc-style comments (e.g., `/** Formats an ISO timestamp as relative time */`)
- Helper functions: Single-line comments or no comments if self-explanatory
- Complex algorithms: Explain the approach in comments

**Example from `internal/parser/claude.go`:**
```go
// ABOUTME: Parses Claude Code JSONL session files into structured session data.
// ABOUTME: Detects DAG forks in uuid/parentUuid trees and splits large-gap forks into separate sessions.
```

## Function Design

**Size (Go):**
- Small focused functions (typically <50 lines)
- Large parsing functions (100-200 lines acceptable for DAG processing)
- Helpers extracted to separate functions with test prefix (e.g., `parseClaudeTestFile`)

**Parameters (Go):**
- Receiver: Short name (e.g., `db *DB`, `s *Server`)
- Multiple string params: Use struct if >3 parameters
- Testing: Functions accept `*testing.T` as first param with `t.Helper()` call
- Functional options: Use `Option func(*Type)` for configuration (e.g., `WithVersion`)

**Return Values (Go):**
- Error always last: `(T, error)` or `(T1, T2, error)`
- Multiple return types: Avoid >2 non-error values without struct
- Simple getters: May return single value (e.g., `Path() string`)

**Parameters (TypeScript):**
- Object destructuring: Use for >2 parameters (e.g., `{ handle, progress }`)
- Optional: Use `?` on type (e.g., `overrides?: Partial<...>`)
- Variadic: Use rest params (e.g., `...substrs: string[]`)

## Module Design

**Go Exports:**
- Package-level constructor: `func New(...) *Type` (e.g., `func New(cfg Config) *Server`)
- Option pattern: `func WithX(x T) Option` for optional configuration
- Pointer receivers: Used for methods that modify state or are large structs
- Value receivers: Used for simple getters or immutable operations

**TypeScript Exports (Stores):**
- Factory function: `createXStore()` returns store instance (e.g., `createSessionsStore()`)
- Methods attached to class/object: `loadProjects()`, `setFilter()`, `reset()`
- State: `$state()` reactive declarations for Svelte 5 (e.g., `sessions: Session[] = $state([])`)
- Getters: Computed properties using `get` keyword (e.g., `get activeSession()`)

**Barrel Files:**
- Go: Package exports public types and functions
- TypeScript: `/api/types/index.ts` re-exports from submodules

**Example Store Pattern (`frontend/src/lib/stores/sessions.svelte.ts`):**
```typescript
class SessionsStore {
  sessions: Session[] = $state([]);
  filters: Filters = $state(defaultFilters());

  get activeSession(): Session | undefined {
    return this.sessions.find((s) => s.id === this.activeSessionId);
  }

  async loadMore() { ... }
}

export function createSessionsStore() {
  return new SessionsStore();
}
```

## Type Patterns

**Go Interfaces:**
- Small, focused interfaces (typically 1-3 methods)
- Reader/Writer style: `type Reader interface { ... }`
- No interface pollution: Only create if multiple implementations exist or needed for dependency injection

**TypeScript Interfaces:**
- API response types: `interface Session { ... }`
- Configuration: `interface Filters { ... }`
- Type imports: `import type { T }` not default imports

## Constants and Magic Numbers

**Go:**
- Named constants at package level for configuration (e.g., `const forkThreshold = 3`, `const batchSize = 100`)
- HTTP status codes: Use `http.StatusOK`, not hardcoded integers
- Time durations: Use `time.Duration` with constants (e.g., `30 * time.Second`)

**TypeScript:**
- Named constants at module level (e.g., `const SESSION_PAGE_SIZE = 500`)
- Magic numbers in tests extracted to descriptive constants
- Enum-like objects: `const AgentType = { CLAUDE: "claude", ... }`

## Code Organization Patterns

**Go Packages:**
- `cmd/agentsview/` - CLI entry points
- `internal/` - All private packages (not exported)
- Each logical area: Separate file per concern (e.g., `sessions.go`, `messages.go`, `search.go`)
- Tests: Colocated in `_test.go` files within same package

**TypeScript:**
- `lib/api/` - API client and types
- `lib/stores/` - Reactive stores with test files
- `lib/utils/` - Utility functions with colocated tests
- `e2e/` - End-to-end tests with helpers and page objects

---

*Convention analysis: 2026-02-27*

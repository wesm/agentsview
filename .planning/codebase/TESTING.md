# Testing Patterns

**Analysis Date:** 2026-02-27

## Test Framework

**Go Runner:**
- Framework: Standard `testing` package (built-in)
- Config: None (uses Makefile)
- Assertions: `github.com/google/go-cmp/cmp` for complex comparisons, testify for basic assertions
- Build tag: `-tags fts5` required for SQLite FTS5 support

**TypeScript Runner:**
- Framework: `vitest` 4.0.18
- Config: None required (uses defaults)
- E2E Framework: Playwright 1.58.2
- Assertion Library: Built-in `expect` from vitest

**Run Commands:**
```bash
make test              # Run all Go tests with CGO_ENABLED=1
make test-short        # Run fast Go tests only (-short flag)
make e2e               # Run Playwright E2E tests (chromium only)
npm test               # Frontend unit tests (runs vitest)
npm run check          # svelte-check for type validation
```

## Test File Organization

**Go Location:**
- Pattern: Colocated in same package as source code
- Files: `{module}_test.go` (e.g., `db_test.go`, `sessions.go` → `sessions_test.go`)
- Internal tests: `{module}_internal_test.go` for same-package internal testing
- Layout: All tests in single `*_test.go` file per module

**TypeScript Location:**
- Pattern: Colocated with source files
- Files: `{module}.test.ts` (e.g., `format.ts` → `format.test.ts`)
- E2E specs: `frontend/e2e/` directory with separate spec files
- Structure: Separate directory (`e2e/`) for integration/E2E tests
- Helpers: `e2e/helpers/` and `e2e/pages/` for test utilities

**Example Structure:**
```
frontend/src/lib/stores/
├── sessions.svelte.ts
├── sessions.test.ts      # Unit test, colocated
├── messages.svelte.ts
├── messages.test.ts

frontend/e2e/
├── helpers/              # Shared E2E utilities
├── pages/                # Page object models
├── session-list.spec.ts
├── message-content.spec.ts
```

## Test Structure

**Go Suite Organization:**

```go
package db

import (
    "testing"
    "github.com/google/go-cmp/cmp"
)

// Helper functions at package level
func filterWith(fn func(*SessionFilter)) SessionFilter {
    f := SessionFilter{Limit: 100}
    fn(&f)
    return f
}

// Table-driven test
func TestListSessions(t *testing.T) {
    tests := []struct {
        name string
        // test data fields
        wantCount int
    }{
        {"case 1", ..., 5},
        {"case 2", ..., 10},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test body
            if got := len(results); got != tt.wantCount {
                t.Errorf("count = %d, want %d", got, tt.wantCount)
            }
        })
    }
}

// Setup helpers
func sessionSet(t *testing.T, d *DB) {
    t.Helper()  // Mark as helper for cleaner stack traces
    // setup code
}

// Assertion helpers
func requireNoError(t *testing.T, err error, msg string) {
    t.Helper()
    if err != nil {
        t.Fatalf("%s: %v", msg, err)
    }
}
```

**Go Patterns:**
- Table-driven tests: `tests := []struct { ... }{}` for parametrized testing
- Subtests: `t.Run(tt.name, func(t *testing.T) { ... })`
- Helpers: Always call `t.Helper()` first in helper functions
- Error assertions: `if err != nil { t.Errorf(...) }` or `require.NoError(t, err)`
- Cleanup: `t.Cleanup(func() { ... })` for resource teardown
- Temporary files: `t.TempDir()` for test isolation

**TypeScript Suite Organization:**

```typescript
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createSessionsStore } from "./sessions.svelte.js";

describe("SessionsStore", () => {
  let store: ReturnType<typeof createSessionsStore>;

  beforeEach(() => {
    vi.clearAllMocks();
    store = createSessionsStore();
  });

  describe("initFromParams", () => {
    it("should parse project and date params", () => {
      store.initFromParams({ project: "myproj" });
      expect(store.filters.project).toBe("myproj");
    });

    it("should parse numeric min_messages", () => {
      store.initFromParams({ min_messages: "5" });
      expect(store.filters.minMessages).toBe(5);
    });
  });
});
```

**TypeScript Patterns:**
- Nested describes: Organize related tests under describe blocks
- Setup/teardown: `beforeEach`, `afterEach` for test isolation
- Mocking: `vi.mock()` at module level, `vi.clearAllMocks()` in `beforeEach`
- Parametrized: Use `it.each()` for multiple cases (less common than Go tables)
- Cleanup: `afterEach` to unbind mocks or cleanup resources

## Mocking

**Go Framework:**
- Package: No external mocking framework; use interfaces or dependency injection
- Pattern: Pass dependencies (often interfaces) to functions or constructors
- Test doubles: Create test-specific implementations of interfaces
- Failures: Inject `failingReader` type or similar for error scenarios

**Example from `internal/sync/common_helpers_test.go`:**
```go
// failingReader is an io.Reader that always returns an error.
type failingReader struct {
    err error
}

func (f failingReader) Read(p []byte) (n int, err error) {
    return 0, f.err
}
```

**TypeScript Framework:**
- Tool: `vi` from vitest (Vitest's mocking utility)
- Pattern: Mock at module level with `vi.mock("../module.js", () => ({ ... }))`
- Setup: Call `vi.clearAllMocks()` in `beforeEach`
- Return values: Use `vi.mocked()` and `.mockResolvedValue()` or `.mockReturnValue()`
- Global stubs: `vi.stubGlobal("fetch", ...)` for browser APIs
- Cleanup: `vi.unstubAllGlobals()` in `afterEach`

**Example from `frontend/src/lib/stores/sessions.test.ts`:**
```typescript
vi.mock("../api/client.js", () => ({
    listSessions: vi.fn(),
    getProjects: vi.fn(),
}));

function mockListSessions(overrides?: Partial<...>) {
    vi.mocked(api.listSessions).mockResolvedValue({
        sessions: [],
        total: 0,
        ...overrides,
    });
}

beforeEach(() => {
    vi.clearAllMocks();
    mockListSessions();
});
```

**What to Mock (Go):**
- Database connections: Use `t.TempDir()` with real SQLite for unit tests
- File operations: Use `t.TempDir()` and real filesystem
- HTTP: Mock interfaces, not http.Client directly
- External APIs: Inject interface parameter

**What to Mock (TypeScript):**
- API client: Always mock `../api/client.js` in unit tests
- fetch: Stub global fetch with `vi.stubGlobal()`
- Time-based functions: Mock in specific tests (not globally)
- Store dependencies: Mock API responses, keep store logic real

**What NOT to Mock (Go):**
- Database: Use real SQLite with `t.TempDir()` and `db.Open()`
- Filesystem: Use real temp filesystem
- HTTP handlers: Test with real `*http.Request` and `http.ResponseWriter`

**What NOT to Mock (TypeScript):**
- Svelte 5 reactivity: Keep store implementations real
- Utility functions: Test actual logic
- Core algorithms: Parse real data, don't stub

## Fixtures and Factories

**Go Test Data:**

Location: `internal/testjsonl/testjsonl.go` and `internal/dbtest/dbtest.go`

```go
// Factory functions in dbtest
func SeedSession(t *testing.T, d *db.DB, id, project string, opts ...func(*db.Session)) {
    t.Helper()
    s := db.Session{
        ID: id,
        Project: project,
        Machine: "local",
        Agent: "claude",
        MessageCount: 1,
    }
    for _, opt := range opts {
        opt(&s)
    }
    if err := d.UpsertSession(s); err != nil {
        t.Fatalf("SeedSession %s: %v", id, err)
    }
}

// Usage with options
sessionSet(t, d, "s1", "proj", func(s *Session) {
    s.StartedAt = Ptr("2024-06-01T10:00:00Z")
    s.EndedAt = Ptr("2024-06-01T11:00:00Z")
})
```

**Go Test Constants:**

```go
const (
    tsZero    = "2024-01-01T00:00:00Z"
    tsZeroS1  = "2024-01-01T00:00:01Z"
    tsEarly   = "2024-01-01T10:00:00Z"
    helloWorldHash = "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
)
```

**Test Fixtures:**

Location: `frontend/e2e/helpers/mock-sessions.ts`

```typescript
// Mock session data for E2E tests
function makeSSEStream(chunks: string[]): ReadableStream<Uint8Array> {
    const encoder = new TextEncoder();
    let i = 0;
    return new ReadableStream({
        pull(controller) {
            if (i < chunks.length) {
                controller.enqueue(encoder.encode(chunks[i]!));
                i++;
            } else {
                controller.close();
            }
        },
    });
}
```

## Coverage

**Requirements:** No enforced coverage targets (not configured)

**View Coverage (Go):**
```bash
go test -cover ./...              # Summary per package
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

**View Coverage (TypeScript):**
```bash
vitest run --coverage              # If coverage config exists (not currently set up)
npm test -- --coverage             # Alternative
```

## Test Types

**Unit Tests (Go):**
- Scope: Single function or method
- Location: `*_test.go` files
- Database: Real SQLite in `t.TempDir()` for integration-like tests
- Time: Fixed test constants (e.g., `tsZero = "2024-01-01T00:00:00Z"`)
- Approach: Table-driven with subtests

**Unit Tests (TypeScript):**
- Scope: Single utility, store, or component logic
- Location: `*.test.ts` colocated with source
- Mocking: Mock API client, keep store logic real
- Time: Controlled with fixed test data
- Approach: Describe/it blocks with beforeEach setup

**Integration Tests (Go):**
- Scope: Multiple packages working together
- Location: `*_integration_test.go` (marked with `-short` skip if needed)
- Database: Real SQLite, real filesystem
- Example: `internal/sync/engine_integration_test.go` tests sync engine with real parsers

**Integration Tests (TypeScript):**
- Scope: Full store with mocked API client
- Location: Colocated in `*.test.ts`
- Example: Store initialization, filter application, pagination

**E2E Tests:**
- Framework: Playwright 1.58.2
- Location: `frontend/e2e/` directory
- Base URL: `http://127.0.0.1:8090`
- Server: Auto-started by Playwright config (runs `scripts/e2e-server.sh`)
- Timeout: 20 seconds per test
- Retries: 0 (disabled for reproducibility)
- Examples: `session-list.spec.ts`, `message-content.spec.ts`, `virtual-list.spec.ts`

**E2E Page Objects:**

```typescript
// e2e/pages/sessions-page.ts
export class SessionsPage {
    constructor(private page: Page) {}

    async goto() {
        await this.page.goto("/");
    }

    async getSessions() {
        return this.page.locator("[data-testid=session]");
    }
}

// Usage in spec
const page = new SessionsPage(testPage);
await page.goto();
const sessions = await page.getSessions();
```

## Common Patterns

**Async Testing (Go):**
- Context: Pass `context.Background()` or `context.WithTimeout()` for deadline tests
- Goroutines: Don't test goroutines directly; test the synchronization primitives
- Channels: Use `select` with timeout for blocking operations

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
result, err := db.ListSessions(ctx, filter)
```

**Async Testing (TypeScript):**
- Promises: Use `await` or `.then()` to wait for resolution
- Mocks: `.mockResolvedValue()` for async returns, `.mockRejectedValue()` for errors
- Streams: Create `ReadableStream` with test data chunks

```typescript
const { handle } = startSync([...chunks...]);
const stats = await handle.done;
expect(stats.total_sessions).toBe(5);
```

**Error Testing (Go):**
- Pattern: Create scenarios that produce errors (invalid JSON, file not found, etc.)
- Assertion: Use `errors.Is()` for specific error types
- Example: Test with malformed UTF-8, missing files, invalid JSON lines

```go
t.Run("malformed UTF-8", func(t *testing.T) {
    badUTF8 := `{...invalid...}` + string([]byte{0xff, 0xfe})
    sess, _ := runClaudeParserTest(t, "test.jsonl", badUTF8)
    assert.GreaterOrEqual(t, sess.MessageCount, 1) // graceful handling
})
```

**Error Testing (TypeScript):**
- Pattern: Mock API responses with error conditions
- Assertion: Check error state in store or API client
- Example: Test invalid parameters, network failures, timeout scenarios

```typescript
vi.mocked(api.listSessions).mockRejectedValue(new Error("Network error"));
// Verify store handles error gracefully
```

## Test Helpers

**Go Helper Packages:**

`internal/dbtest/dbtest.go`:
- `OpenTestDB(t)` - Create isolated test database
- `SeedSession(t, d, id, project, opts...)` - Insert test session with options
- `SeedMessages(t, d, msgs...)` - Insert test messages
- `UserMsg()`, `AsstMsg()` - Factory for message objects
- `Ptr[T](v)` - Generic pointer helper

`internal/parser/test_helpers_test.go`:
- `runClaudeParserTest(t, fileName, content)` - Parse JSONL content
- `createTestFile(t, name, content)` - Write test file
- `loadFixture(t, path)` - Load test fixture file
- `assertSessionMeta()`, `assertMessage()` - Custom assertions
- `captureLog(t)` - Redirect log output for assertion
- `assertLogContains()`, `assertLogNotContains()` - Log assertions

`internal/sync/common_helpers_test.go`:
- `requirePathError(t, err)` - Assert PathError type
- `failingReader` - Mock reader that always fails

**Go Assertion Pattern:**
```go
func requireNoError(t *testing.T, err error, msg string) {
    t.Helper()
    if err != nil {
        t.Fatalf("%s: %v", msg, err)
    }
}

func requireCount(t *testing.T, d *DB, f SessionFilter, want int) {
    t.Helper()
    page, err := d.ListSessions(context.Background(), f)
    requireNoError(t, err, "ListSessions")
    if got := len(page.Sessions); got != want {
        t.Errorf("got %d sessions, want %d", got, want)
    }
}
```

**TypeScript Helper Pattern:**
```typescript
function mockListSessions(overrides?: Partial<{ next_cursor: string }>) {
    vi.mocked(api.listSessions).mockResolvedValue({
        sessions: [],
        total: 0,
        ...overrides,
    });
}

function expectListSessionsCalledWith(expected: Partial<ListSessionsParams>) {
    expect(api.listSessions).toHaveBeenLastCalledWith(
        expect.objectContaining(expected),
    );
}
```

## Test Guidelines

**Before Committing:**
1. Run `make test` for Go tests (requires `CGO_ENABLED=1`)
2. Run `npm test` in `frontend/` for TypeScript tests
3. Run `make lint` and `make vet` for Go code quality
4. Run `npm run check` for TypeScript type checking

**When Writing Tests:**
1. Add tests for new features and bug fixes
2. Keep tests fast and isolated (use `t.TempDir()`, test doubles)
3. Use table-driven tests for parametrized scenarios in Go
4. Mock external dependencies, test internal logic
5. Name test cases descriptively (GoLang: `"should handle X case"`, TypeScript: descriptive strings)
6. Use helpers for common setup and assertions

**Test Maintenance:**
- Update tests when behavior changes
- Keep fixtures current with schema changes
- Don't skip flaky tests - fix them or remove them
- Document non-obvious test setup in comments

---

*Testing analysis: 2026-02-27*

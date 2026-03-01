# Codebase Concerns

**Analysis Date:** 2026-02-27

## Tech Debt

**Schema Migration Complexity:**
- Issue: Database schema has undergone multiple migrations with manual column checks. The `needsRebuild()` function in `internal/db/db.go` performs 6 separate pragma_table_info checks to detect schema versions rather than using a schema version table.
- Files: `internal/db/db.go` (lines 127-225)
- Impact: Adding new schema columns requires adding new migration checks; full database rebuild is triggered on any schema mismatch rather than incremental migration.
- Fix approach: Introduce a schema_version table with a single version number. Create incremental migration functions that can update from v1→v2, v2→v3, etc. This reduces maintenance burden and allows targeted schema updates.

**Line Reader Memory Scaling:**
- Issue: The line reader in `internal/parser/linereader.go` uses a dynamically growing buffer for handling oversized lines (up to 64MB max line size). Large JSONL files with very long lines could cause significant memory allocation.
- Files: `internal/parser/linereader.go`, `internal/parser/claude.go` (line 24: `maxLineSize = 64 * 1024 * 1024`)
- Impact: Processing a file with many 64MB lines could exhaust memory. The buffer grows per-line rather than per-file, compounding the issue.
- Fix approach: Add metrics to track actual max line sizes observed. Consider implementing a streaming JSON parser that doesn't require buffering entire lines, or add configurable line size limits with user-friendly errors.

**DB Connection Pool Management:**
- Issue: Reader pool has hardcoded `SetMaxOpenConns(4)` in `internal/db/db.go` (line 251). This may be insufficient under load or wasteful in light traffic. No adaptive pooling strategy.
- Files: `internal/db/db.go` (line 251)
- Impact: Concurrent API requests exceeding 4 reads could queue, or connections may idle unnecessarily.
- Fix approach: Make pool size configurable via environment variable or config file with reasonable defaults based on expected concurrency.

## Known Bugs

**Insight Generation Env Filtering Edge Case:**
- Issue: The environment allowlist in `internal/insight/generate.go` (lines 68-87) uses prefix matching with trailing underscores. Case-insensitive uppercase comparison could theoretically allow typos like "path" to pass if any prefix matches, though current allowlist is safe.
- Files: `internal/insight/generate.go` (lines 89-102)
- Trigger: Only a concern if allowlist is modified incorrectly.
- Workaround: Keep tight control over the allowlist and add test coverage for prefix matching edge cases.

**Cursor Secret Regeneration on Every Open:**
- Issue: Every call to `db.Open()` generates a fresh random cursor secret in `internal/db/db.go` (line 258), invalidating all previously issued pagination cursors from older server instances. This affects multi-instance deployments or server restarts.
- Files: `internal/db/db.go` (lines 257-264)
- Impact: Cursor-based pagination breaks across server restarts; bookmarks from older sessions fail with "invalid cursor" errors.
- Workaround: Store cursor secret in database and load on startup instead of regenerating. This requires treating it as persistent state rather than ephemeral.

**Sync Engine File Handle Exhaustion:**
- Issue: In `internal/sync/engine.go`, worker goroutines parse session files concurrently (maxWorkers=8, batchSize=100) without explicit file handle management. If file handles aren't properly closed in all code paths, repeated syncs could exhaust OS limits.
- Files: `internal/sync/engine.go` (lines 20-23, 1350+)
- Trigger: Many sessions with parse errors or edge cases where defer isn't reached.
- Mitigation: Parser code (claude.go, codex.go) has proper defers, but error paths should be audited.

## Security Considerations

**CORS and Host Header Validation:**
- Risk: CORS middleware in `internal/server/server.go` (lines 467-525) restricts to localhost origins by default. However, the host check middleware (lines 257-279) only applies to `/api/` routes while SPA fallback (lines 161-178) is left accessible. An attacker who can control DNS or get a certificate could potentially access the SPA and perform CSRF.
- Files: `internal/server/server.go` (entire middleware section)
- Current mitigation: Host header validation on API routes, CORS origin checks, SPA-only routes have no state-changing operations. Mutating requests require proper Origin headers.
- Recommendations: Document that this security model assumes attackers cannot redirect the user's browser to the actual server IP (i.e., not suitable for servers accessible on the public internet). Add a warning in docs about running on 0.0.0.0/:: with proper network isolation.

**Subprocess Environment Filtering:**
- Risk: Insight generation in `internal/insight/generate.go` (lines 39-66) spawns subprocess CLIs (claude, codex, gemini) with a filtered environment. The allowlist is tight but must be carefully maintained.
- Files: `internal/insight/generate.go` (lines 68-102)
- Current mitigation: Allowlist approach rather than blocklist; only passes PATH, HOME, locale, shell, proxy, and SSL_CERT_ vars. Explicitly blocks API key prefixes.
- Recommendations: Log subprocess env filtering in debug mode; periodically audit against new CLI requirements for environment variables.

**File Upload Path Traversal:**
- Risk: File uploads in `internal/server/upload.go` (lines 28-69) validate filename safety with `filepath.Base()` and `isSafeName()` checks. But `isSafeName()` implementation should be verified to prevent traversal via encoded paths.
- Files: `internal/server/upload.go` (lines 56-62)
- Current mitigation: Uses `filepath.Base()` which strips directory components, then validates against alphanumeric + underscore/dash only via `isSafeName()`.
- Recommendations: Add unit tests for path traversal vectors (../../../etc/passwd, ..%2F..%2F, URL encoding, unicode normalization).

## Performance Bottlenecks

**Cursor Signature Verification Per Request:**
- Problem: Pagination cursor verification in `internal/db/sessions.go` (lines 95-112, 115-164) recomputes HMAC signatures for every paginated request. No caching of decoded cursors.
- Files: `internal/db/sessions.go`
- Cause: Cursor is stateless by design (signed rather than stored), but this means signature computation is unavoidable. Not a bottleneck for typical usage.
- Improvement path: Minor—pre-compute cursors for common offsets or cache recently validated cursors if profile data shows this matters.

**FTS5 Index Rebuilds:**
- Problem: `internal/db/db.go` DropFTS/RebuildFTS (lines 276-311) require full table scans. During ResyncAll when many messages are inserted, FTS index could become a bottleneck.
- Files: `internal/db/db.go` (lines 276-311)
- Cause: FTS5 is re-initialized after bulk inserts to optimize index structure, but this blocks writes.
- Improvement path: Batch message inserts before triggering FTS rebuild; consider incremental FTS updates rather than drop/rebuild.

**DAG Processing in Parser:**
- Problem: Claude session parser in `internal/parser/claude.go` (lines 166-179) performs full DAG traversal to detect forks. For sessions with thousands of messages, fork detection is O(n) with significant tree structure overhead.
- Files: `internal/parser/claude.go`
- Cause: Necessary to handle multi-branch Claude Code sessions correctly.
- Improvement path: Cache parent-child relationships incrementally; use DFS instead of redundant lookups.

## Fragile Areas

**Database Reopen During ResyncAll:**
- Files: `internal/sync/engine.go` (lines 400-516)
- Why fragile: ResyncAll performs an atomic file swap (create temp DB, close orig, rename temp, reopen orig). If any step fails partway, the database could be left in an inconsistent state where connections point to deallocated memory or old files.
- Safe modification: Never modify the 5-step reopen sequence without adding comprehensive error recovery. The "recovery reopen" fallback at lines 446-447, 466-467, 491-492 is critical—ensure rerr is always logged and service restoration is attempted even on cascading failures.
- Test coverage: `TestResyncAllConcurrentReads` and `TestMigrationRace` in `internal/db/db_test.go` exercise this, but edge cases around partial connection closure remain.

**Parser Fork Detection:**
- Files: `internal/parser/claude.go` (lines 166-175)
- Why fragile: Fork detection relies on UUID/parentUUID DAG structure. If Claude Code ever changes how it assigns UUIDs (e.g., adds random IDs to non-forking sessions), fork detection will incorrectly split sessions.
- Safe modification: Any change to fork detection logic should be guarded by comprehensive tests covering actual Claude Code session files. Add validation that split sessions have proper parentSessionID links.
- Test coverage: `TestDagFork`, `TestForkThreshold` in `internal/parser/claude_parser_test.go` cover basic cases but not all real-world UUID patterns.

**File Watcher Debounce:**
- Files: `internal/sync/watcher.go` (lines 16-88)
- Why fragile: The debounce mechanism (lines 143-150) collects pending changes and flushes after debounce period. If fsnotify drops events or timing races occur, changes could be missed or cause infinite debounce loops.
- Safe modification: The `flush()` method (lines 143-150) holds mutex while iterating; ensure no callback recursion. Test with rapid file modifications to verify debounce stability.
- Test coverage: `TestWatcherConcurrentStop` in `internal/sync/watcher_test.go` covers concurrent stops but not rapid event flooding.

**Insight Generation Context Cancellation:**
- Files: `internal/insight/generate.go` (lines 39-66)
- Why fragile: Subprocess is spawned with context deadline, but `cmd.Wait()` (called implicitly) doesn't guarantee timely process termination on context cancel. Process could hang or zombie if CLI doesn't handle SIGTERM.
- Safe modification: Wrap `cmd.Run()` with context-aware cancellation; use `cmd.Kill()` as fallback. Test with unresponsive subprocess simulation.
- Test coverage: `TestGenerateTimeout` missing—add test for context.DeadlineExceeded propagation.

## Scaling Limits

**Worker Pool Hardcoded Size:**
- Current capacity: maxWorkers=8 (line 23 in `internal/sync/engine.go`)
- Limit: On machines with <4 cores, 8 workers may oversubscribe; on 64+ core systems, throughput may be bottlenecked.
- Scaling path: Make workers configurable via environment variable; set default to `runtime.NumCPU()` or `min(8, NumCPU())`. Profile sync time with large session counts (1000+ files) to establish optimal worker count.

**Database Connection Pool:**
- Current capacity: Reader pool max 4 connections (line 251 in `internal/db/db.go`)
- Limit: Under load with 10+ concurrent API requests, read queries will queue.
- Scaling path: Increase default to 16, make configurable, and monitor connection pool saturation in logs.

**Watcher Directory Tracking:**
- Current capacity: fsnotify can handle several hundred watched directories on most systems.
- Limit: Users with >1000 subdirectories in session folders may hit OS limits (inotify limit on Linux is typically 8192 watches).
- Scaling path: Document watcher limits; add pagination or session directory filtering. Monitor watch count at startup.

**SSE Client Connections:**
- Current capacity: No explicit limit on concurrent SSE clients.
- Limit: Each client holds a goroutine (`sessionMonitor` in `internal/server/events.go` line 26) and timer. 1000+ concurrent watchers could exhaust goroutines or memory.
- Scaling path: Implement max concurrent watchers per session or global limit; document expected scale (assume <100 concurrent clients for now).

## Dependencies at Risk

**go-sqlite3 (CGO Dependency):**
- Risk: Project depends on `github.com/mattn/go-sqlite3` (line 17 in `internal/db/db.go`), which requires CGO. This complicates cross-platform builds and increases attack surface through C library dependencies.
- Impact: Prevents pure Go distribution; requires gcc/clang on all platforms. SQLite vulnerabilities could affect agentsview security.
- Migration plan: Consider `github.com/ncruces/go-sqlite3` (pure Go implementation) or `modernc.org/sqlite` as alternatives. Test compatibility with FTS5 module.

**fsnotify (Watcher):**
- Risk: `github.com/fsnotify/fsnotify` (line 12 in `internal/sync/watcher.go`) is the only file watching library; no fallback to polling.
- Impact: If fsnotify has bugs on specific OS/filesystem combinations, agentsview can't detect session updates.
- Migration plan: Keep, but add optional fallback to periodic polling (already done in server SSE with `sessionMonitor`). Document known issues with network filesystems.

## Missing Critical Features

**Incremental Database Sync:**
- Problem: ResyncAll rebuilds entire database from scratch. For large session collections, this is slow and I/O intensive.
- Blocks: Performance optimization for 10000+ sessions per machine.
- Suggested approach: Implement incremental sync that only re-parses files with changed mtime. Requires versioning schema changes carefully.

**Configuration Validation at Startup:**
- Problem: Config loading (`internal/config/config.go`) doesn't validate directory accessibility or warn about permission issues until first sync.
- Blocks: User confusion when session directories are unreadable.
- Suggested approach: Add startup check that validates all configured directories exist and are readable; warn about potential path issues early.

## Test Coverage Gaps

**Untested Error Paths in Database Recovery:**
- What's not tested: Full catastrophic failure scenarios (corrupted WAL file, disk full during reopen, partial file swap).
- Files: `internal/sync/engine.go` (lines 437-507)
- Risk: Unknown bugs in recovery logic could leave database in inconsistent state.
- Priority: High - recovery must be bulletproof.

**Parser Edge Cases with Malformed JSONL:**
- What's not tested: JSONL files with extremely long lines (hitting 64MB limit), recursive embedding, null bytes, invalid UTF-8 sequences.
- Files: `internal/parser/claude.go`, `internal/parser/linereader.go`
- Risk: Parser could panic or consume unbounded memory on adversarial input.
- Priority: Medium - unlikely in practice but security-relevant.

**Concurrent Watcher Events During Sync:**
- What's not tested: File modifications happening while sync is in progress; watcher events triggered during ResyncAll.
- Files: `internal/sync/engine.go`, `internal/sync/watcher.go`
- Risk: Sessions could be skipped or double-synced.
- Priority: Medium - difficult to reproduce but impacts data consistency.

**CORS and Host Header Bypass Scenarios:**
- What's not tested: Requests with malformed Host headers, IPv6 address variations, non-standard ports under CORS.
- Files: `internal/server/server.go` (middleware section)
- Risk: CSRF or DNS rebinding attacks could slip through.
- Priority: Medium - security-relevant, exercise thoroughly before exposing to untrusted networks.

**Insight Generation Subprocess Failures:**
- What's not tested: Context cancellation during subprocess startup, subprocess crash with partial output, missing CLI binaries in specific environments.
- Files: `internal/insight/generate.go`
- Risk: Insight generation could hang or return corrupted data.
- Priority: Low - affects non-critical feature, but should be resilient.

---

*Concerns audit: 2026-02-27*

package sync

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	gosync "sync"
	"time"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/parser"
	"github.com/wesm/agentsview/internal/timeutil"
)

const (
	batchSize  = 100
	maxWorkers = 8
)

// Engine orchestrates session file discovery and sync.
type Engine struct {
	db            *db.DB
	claudeDir     string
	codexDir      string
	geminiDir     string
	machine       string
	mu            gosync.RWMutex
	lastSync      time.Time
	lastSyncStats SyncStats
	// failedFiles tracks paths that errored during parsing,
	// keyed by path with the file mtime at time of failure.
	// The file is only retried when its mtime changes.
	failedMu    gosync.RWMutex
	failedFiles map[string]int64
}

// NewEngine creates a sync engine.
func NewEngine(
	database *db.DB,
	claudeDir, codexDir, geminiDir, machine string,
) *Engine {
	return &Engine{
		db:          database,
		claudeDir:   claudeDir,
		codexDir:    codexDir,
		geminiDir:   geminiDir,
		machine:     machine,
		failedFiles: make(map[string]int64),
	}
}

// LastSync returns the time of the last completed sync.
func (e *Engine) LastSync() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastSync
}

// LastSyncStats returns statistics from the last sync.
func (e *Engine) LastSyncStats() SyncStats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastSyncStats
}

type syncJob struct {
	processResult
	path string
}

// SyncPaths syncs only the specified changed file paths
// instead of discovering and hashing all session files.
// Paths that don't match known session file patterns are
// silently ignored.
func (e *Engine) SyncPaths(paths []string) {
	files := e.classifyPaths(paths)
	if len(files) == 0 {
		return
	}

	results := e.startWorkers(files)
	stats := e.collectAndBatch(results, len(files), nil)

	e.mu.Lock()
	e.lastSync = time.Now()
	e.lastSyncStats = stats
	e.mu.Unlock()

	if stats.Synced > 0 {
		log.Printf(
			"sync: %d file(s) updated", stats.Synced,
		)
	}
}

// classifyPaths maps changed file system paths to
// DiscoveredFile structs, filtering out paths that don't
// match known session file patterns.
func (e *Engine) classifyPaths(
	paths []string,
) []DiscoveredFile {
	var geminiProjects map[string]string
	var files []DiscoveredFile
	for _, p := range paths {
		if df, ok := e.classifyOnePath(
			p, &geminiProjects,
		); ok {
			files = append(files, df)
		}
	}
	return files
}

// isUnder checks whether path is strictly inside dir after
// cleaning both paths. Returns the relative path on success.
func isUnder(dir, path string) (string, bool) {
	dir = filepath.Clean(dir)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return "", false
	}
	sep := string(filepath.Separator)
	if rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+sep) {
		return "", false
	}
	return rel, true
}

func (e *Engine) classifyOnePath(
	path string,
	geminiProjects *map[string]string,
) (DiscoveredFile, bool) {
	sep := string(filepath.Separator)

	// Claude: <claudeDir>/<project>/<session>.jsonl
	if e.claudeDir != "" {
		if rel, ok := isUnder(e.claudeDir, path); ok {
			if !strings.HasSuffix(path, ".jsonl") {
				return DiscoveredFile{}, false
			}
			stem := strings.TrimSuffix(
				filepath.Base(path), ".jsonl",
			)
			if strings.HasPrefix(stem, "agent-") {
				return DiscoveredFile{}, false
			}
			parts := strings.Split(rel, sep)
			if len(parts) != 2 {
				return DiscoveredFile{}, false
			}
			return DiscoveredFile{
				Path:    path,
				Project: parts[0],
				Agent:   parser.AgentClaude,
			}, true
		}
	}

	// Codex: <codexDir>/<year>/<month>/<day>/<file>.jsonl
	if e.codexDir != "" {
		if rel, ok := isUnder(e.codexDir, path); ok {
			parts := strings.Split(rel, sep)
			if len(parts) != 4 {
				return DiscoveredFile{}, false
			}
			if !isDigits(parts[0]) ||
				!isDigits(parts[1]) ||
				!isDigits(parts[2]) {
				return DiscoveredFile{}, false
			}
			if !strings.HasSuffix(parts[3], ".jsonl") {
				return DiscoveredFile{}, false
			}
			return DiscoveredFile{
				Path:  path,
				Agent: parser.AgentCodex,
			}, true
		}
	}

	// Gemini: <geminiDir>/tmp/<hash>/chats/session-*.json
	if e.geminiDir != "" {
		if rel, ok := isUnder(e.geminiDir, path); ok {
			parts := strings.Split(rel, sep)
			if len(parts) != 4 ||
				parts[0] != "tmp" ||
				parts[2] != "chats" {
				return DiscoveredFile{}, false
			}
			name := parts[3]
			if !strings.HasPrefix(name, "session-") ||
				!strings.HasSuffix(name, ".json") {
				return DiscoveredFile{}, false
			}
			hash := parts[1]
			if *geminiProjects == nil {
				*geminiProjects = buildGeminiProjectMap(
					e.geminiDir,
				)
			}
			project := (*geminiProjects)[hash]
			if project == "" {
				project = "unknown"
			}
			return DiscoveredFile{
				Path:    path,
				Project: project,
				Agent:   parser.AgentGemini,
			}, true
		}
	}

	return DiscoveredFile{}, false
}

// SyncAll discovers and syncs all session files from all agents.
func (e *Engine) SyncAll(onProgress ProgressFunc) SyncStats {
	claude := DiscoverClaudeProjects(e.claudeDir)
	codex := DiscoverCodexSessions(e.codexDir)
	gemini := DiscoverGeminiSessions(e.geminiDir)

	all := make(
		[]DiscoveredFile, 0,
		len(claude)+len(codex)+len(gemini),
	)
	all = append(all, claude...)
	all = append(all, codex...)
	all = append(all, gemini...)

	if onProgress != nil {
		onProgress(Progress{
			Phase:         PhaseSyncing,
			SessionsTotal: len(all),
		})
	}

	results := e.startWorkers(all)
	stats := e.collectAndBatch(results, len(all), onProgress)

	e.mu.Lock()
	e.lastSync = time.Now()
	e.lastSyncStats = stats
	e.mu.Unlock()
	return stats
}

// startWorkers fans out file processing across a worker pool
// and returns a channel of results.
func (e *Engine) startWorkers(
	files []DiscoveredFile,
) <-chan syncJob {
	workers := min(max(runtime.NumCPU(), 2), maxWorkers)

	jobs := make(chan DiscoveredFile, len(files))
	results := make(chan syncJob, len(files))

	for range workers {
		go func() {
			for file := range jobs {
				results <- syncJob{
					processResult: e.processFile(file),
					path:          file.Path,
				}
			}
		}()
	}

	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	return results
}

// collectAndBatch drains the results channel, batches
// successful parses, and writes them to the database.
func (e *Engine) collectAndBatch(
	results <-chan syncJob, total int,
	onProgress ProgressFunc,
) SyncStats {
	var stats SyncStats
	stats.TotalSessions = total

	progress := Progress{
		Phase:         PhaseSyncing,
		SessionsTotal: total,
	}

	var pending []pendingWrite

	for range total {
		r := <-results

		if r.err != nil {
			e.tombstoneFromPath(r.path)
			log.Printf("sync error: %v", r.err)
			continue
		}
		if r.skip {
			stats.RecordSkip()
			progress.SessionsDone++
			if onProgress != nil {
				onProgress(progress)
			}
			continue
		}
		e.clearTombstone(r.path)
		if r.sess == nil {
			progress.SessionsDone++
			if onProgress != nil {
				onProgress(progress)
			}
			continue
		}

		pending = append(pending, pendingWrite{
			sess: *r.sess,
			msgs: r.msgs,
		})

		if len(pending) >= batchSize {
			stats.RecordSynced(len(pending))
			progress.MessagesIndexed += countMessages(pending)
			e.writeBatch(pending)
			pending = pending[:0]
		}

		progress.SessionsDone++
		if onProgress != nil {
			onProgress(progress)
		}
	}

	if len(pending) > 0 {
		stats.RecordSynced(len(pending))
		progress.MessagesIndexed += countMessages(pending)
		e.writeBatch(pending)
	}

	progress.Phase = PhaseDone
	if onProgress != nil {
		onProgress(progress)
	}
	return stats
}

type processResult struct {
	sess *parser.ParsedSession
	msgs []parser.ParsedMessage
	skip bool
	err  error
}

func (e *Engine) processFile(
	file DiscoveredFile,
) processResult {

	info, err := os.Stat(file.Path)
	if err != nil {
		return processResult{err: fmt.Errorf("stat %s: %w", file.Path, err)}
	}

	// Skip files that previously failed and haven't changed
	mtime := info.ModTime().UnixNano()
	e.failedMu.RLock()
	failedMtime, failed := e.failedFiles[file.Path]
	e.failedMu.RUnlock()
	if failed && failedMtime == mtime {
		return processResult{skip: true}
	}

	switch file.Agent {
	case parser.AgentClaude:
		return e.processClaude(file, info)
	case parser.AgentCodex:
		return e.processCodex(file, info)
	case parser.AgentGemini:
		return e.processGemini(file, info)
	default:
		return processResult{
			err: fmt.Errorf("unknown agent type: %s", file.Agent),
		}
	}
}

// tombstone records a failed file so it won't be retried
// until its mtime changes.
func (e *Engine) tombstone(path string, mtime int64) {
	e.failedMu.Lock()
	e.failedFiles[path] = mtime
	e.failedMu.Unlock()
}

// clearTombstone removes a tombstone when a file succeeds.
func (e *Engine) clearTombstone(path string) {
	e.failedMu.Lock()
	delete(e.failedFiles, path)
	e.failedMu.Unlock()
}

// shouldSkipFile returns true when the file's size and hash
// match what is already stored in the database.
func (e *Engine) shouldSkipFile(
	sessionID, path string, info os.FileInfo,
) bool {
	storedSize, storedHash, ok := e.db.GetSessionFileInfo(
		sessionID,
	)
	if !ok || storedSize != info.Size() {
		return false
	}
	hash, err := ComputeFileHash(path)
	return err == nil && hash == storedHash
}

func (e *Engine) processClaude(
	file DiscoveredFile, info os.FileInfo,
) processResult {

	sessionID := strings.TrimSuffix(info.Name(), ".jsonl")

	if e.shouldSkipFile(sessionID, file.Path, info) {
		sess, _ := e.db.GetSession(
			context.Background(), sessionID,
		)
		if sess != nil &&
			sess.Project != "" &&
			!parser.NeedsProjectReparse(sess.Project) {
			return processResult{skip: true}
		}
	}

	// Determine project name from cwd if possible
	project := parser.GetProjectName(file.Project)
	cwd, gitBranch := parser.ExtractClaudeProjectHints(
		file.Path,
	)
	if cwd != "" {
		if p := parser.ExtractProjectFromCwdWithBranch(
			cwd, gitBranch,
		); p != "" {
			project = p
		}
	}

	sess, msgs, err := parser.ParseClaudeSession(
		file.Path, project, e.machine,
	)
	if err != nil {
		return processResult{err: err}
	}

	hash, err := ComputeFileHash(file.Path)
	if err == nil {
		sess.File.Hash = hash
	}

	return processResult{sess: &sess, msgs: msgs}
}

func (e *Engine) processCodex(
	file DiscoveredFile, info os.FileInfo,
) processResult {

	// For codex, we need to parse to get session_id
	sess, msgs, err := parser.ParseCodexSession(
		file.Path, e.machine, false,
	)
	if err != nil {
		return processResult{err: err}
	}
	if sess == nil {
		return processResult{} // non-interactive
	}

	if e.shouldSkipFile(sess.ID, file.Path, info) {
		return processResult{skip: true}
	}

	hash, err := ComputeFileHash(file.Path)
	if err == nil {
		sess.File.Hash = hash
	}

	return processResult{sess: sess, msgs: msgs}
}

func (e *Engine) processGemini(
	file DiscoveredFile, info os.FileInfo,
) processResult {
	sess, msgs, err := parser.ParseGeminiSession(
		file.Path, file.Project, e.machine,
	)
	if err != nil {
		return processResult{err: err}
	}
	if sess == nil {
		return processResult{}
	}

	if e.shouldSkipFile(sess.ID, file.Path, info) {
		return processResult{skip: true}
	}

	hash, err := ComputeFileHash(file.Path)
	if err == nil {
		sess.File.Hash = hash
	}

	return processResult{sess: sess, msgs: msgs}
}

type pendingWrite struct {
	sess parser.ParsedSession
	msgs []parser.ParsedMessage
}

func (e *Engine) writeBatch(batch []pendingWrite) {
	for _, pw := range batch {
		s := db.Session{
			ID:              pw.sess.ID,
			Project:         pw.sess.Project,
			Machine:         pw.sess.Machine,
			Agent:           string(pw.sess.Agent),
			MessageCount:    pw.sess.MessageCount,
			ParentSessionID: strPtr(pw.sess.ParentSessionID),
			FilePath:        strPtr(pw.sess.File.Path),
			FileSize:        int64Ptr(pw.sess.File.Size),
			FileMtime:       int64Ptr(pw.sess.File.Mtime),
			FileHash:        strPtr(pw.sess.File.Hash),
		}
		if pw.sess.FirstMessage != "" {
			s.FirstMessage = &pw.sess.FirstMessage
		}
		if !pw.sess.StartedAt.IsZero() {
			s.StartedAt = timeutil.Ptr(pw.sess.StartedAt)
		}
		if !pw.sess.EndedAt.IsZero() {
			s.EndedAt = timeutil.Ptr(pw.sess.EndedAt)
		}

		if err := e.db.UpsertSession(s); err != nil {
			log.Printf("upsert session %s: %v", s.ID, err)
			continue
		}

		msgs := make([]db.Message, len(pw.msgs))
		for i, m := range pw.msgs {
			msgs[i] = db.Message{
				SessionID:     pw.sess.ID,
				Ordinal:       m.Ordinal,
				Role:          string(m.Role),
				Content:       m.Content,
				Timestamp:     timeutil.Format(m.Timestamp),
				HasThinking:   m.HasThinking,
				HasToolUse:    m.HasToolUse,
				ContentLength: m.ContentLength,
				ToolCalls: convertToolCalls(
					pw.sess.ID, m.ToolCalls,
				),
				ToolResults: convertToolResults(m.ToolResults),
			}
		}
		msgs = pairAndFilter(msgs)

		if err := e.db.ReplaceSessionMessages(
			pw.sess.ID, msgs,
		); err != nil {
			log.Printf(
				"replace messages for %s: %v", pw.sess.ID, err,
			)
		}
	}
}

func countMessages(batch []pendingWrite) int {
	n := 0
	for _, pw := range batch {
		n += len(pw.msgs)
	}
	return n
}

// FindSourceFile locates the original source file for a
// session ID.
func (e *Engine) FindSourceFile(sessionID string) string {
	switch {
	case strings.HasPrefix(sessionID, "codex:"):
		return FindCodexSourceFile(e.codexDir, sessionID[6:])
	case strings.HasPrefix(sessionID, "gemini:"):
		return FindGeminiSourceFile(
			e.geminiDir, sessionID[7:],
		)
	default:
		return FindClaudeSourceFile(e.claudeDir, sessionID)
	}
}

// SyncSingleSession re-syncs a single session by its ID.
// Unlike the bulk SyncAll path, this includes exec-originated
// Codex sessions and uses the existing DB project as fallback.
func (e *Engine) SyncSingleSession(sessionID string) error {
	path := e.FindSourceFile(sessionID)
	if path == "" {
		return fmt.Errorf(
			"source file not found for %s", sessionID,
		)
	}

	var agent parser.AgentType
	switch {
	case strings.HasPrefix(sessionID, "codex:"):
		agent = parser.AgentCodex
	case strings.HasPrefix(sessionID, "gemini:"):
		agent = parser.AgentGemini
	default:
		agent = parser.AgentClaude
	}

	// Reuse processFile for tombstone check, stat, and hash
	// skip logic. For Claude this is the full pipeline; for
	// Codex we need includeExec=true so we call the parser
	// directly.
	file := DiscoveredFile{
		Path:  path,
		Agent: agent,
	}
	if agent == parser.AgentClaude {
		// Try to preserve existing project from DB first
		if sess, _ := e.db.GetSession(context.Background(), sessionID); sess != nil &&
			sess.Project != "" &&
			!parser.NeedsProjectReparse(sess.Project) {
			file.Project = sess.Project
		} else {
			file.Project = filepath.Base(filepath.Dir(path))
		}
	}

	res := e.processFile(file)
	if res.err != nil {
		e.tombstoneFromPath(path)
		return res.err
	}
	if res.skip {
		return nil
	}

	// For Codex, processFile uses includeExec=false which may
	// return nil sess for exec-originated sessions. Re-parse
	// with includeExec=true when that happens.
	if res.sess == nil && agent == parser.AgentCodex {
		res = e.processCodexIncludeExec(file)
		if res.err != nil {
			e.tombstoneFromPath(path)
			return res.err
		}
		if res.sess == nil {
			return nil
		}
	}

	if res.sess == nil {
		return nil
	}

	e.clearTombstone(path)
	e.writeBatch([]pendingWrite{
		{sess: *res.sess, msgs: res.msgs},
	})
	return nil
}

// processCodexIncludeExec re-parses a Codex session with
// exec-originated sessions included.
func (e *Engine) processCodexIncludeExec(
	file DiscoveredFile,
) processResult {
	sess, msgs, err := parser.ParseCodexSession(
		file.Path, e.machine, true,
	)
	if err != nil {
		return processResult{err: err}
	}
	if sess == nil {
		return processResult{}
	}
	if h, herr := ComputeFileHash(file.Path); herr == nil {
		sess.File.Hash = h
	}
	return processResult{sess: sess, msgs: msgs}
}

func (e *Engine) tombstoneFromPath(path string) {
	if info, err := os.Stat(path); err == nil {
		e.tombstone(path, info.ModTime().UnixNano())
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int64Ptr(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}

// convertToolCalls maps parsed tool calls to db.ToolCall
// structs. MessageID is resolved later during insert.
func convertToolCalls(
	sessionID string, parsed []parser.ParsedToolCall,
) []db.ToolCall {
	if len(parsed) == 0 {
		return nil
	}
	calls := make([]db.ToolCall, len(parsed))
	for i, tc := range parsed {
		calls[i] = db.ToolCall{
			SessionID: sessionID,
			ToolName:  tc.ToolName,
			Category:  tc.Category,
			ToolUseID: tc.ToolUseID,
			InputJSON: tc.InputJSON,
			SkillName: tc.SkillName,
		}
	}
	return calls
}

// convertToolResults maps parsed tool results to db.ToolResult
// structs for use in pairing before DB insert.
func convertToolResults(
	parsed []parser.ParsedToolResult,
) []db.ToolResult {
	if len(parsed) == 0 {
		return nil
	}
	results := make([]db.ToolResult, len(parsed))
	for i, tr := range parsed {
		results[i] = db.ToolResult{
			ToolUseID:     tr.ToolUseID,
			ContentLength: tr.ContentLength,
		}
	}
	return results
}

// pairAndFilter pairs tool results with their corresponding
// tool calls, then removes user messages that carried only
// tool_result blocks (no displayable text).
func pairAndFilter(msgs []db.Message) []db.Message {
	pairToolResults(msgs)
	filtered := msgs[:0]
	for _, m := range msgs {
		if m.Role == "user" &&
			len(m.ToolResults) > 0 &&
			strings.TrimSpace(m.Content) == "" {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// pairToolResults matches tool_result content lengths to their
// corresponding tool_calls across message boundaries using
// tool_use_id.
func pairToolResults(msgs []db.Message) {
	idx := make(map[string]*db.ToolCall)
	for i := range msgs {
		for j := range msgs[i].ToolCalls {
			tc := &msgs[i].ToolCalls[j]
			if tc.ToolUseID != "" {
				idx[tc.ToolUseID] = tc
			}
		}
	}
	if len(idx) == 0 {
		return
	}
	for _, m := range msgs {
		for _, tr := range m.ToolResults {
			if tc, ok := idx[tr.ToolUseID]; ok {
				tc.ResultContentLength = tr.ContentLength
			}
		}
	}
}

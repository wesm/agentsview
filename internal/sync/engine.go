package sync

import (
	"context"
	"fmt"
	"log"
	"maps"
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
	claudeDirs    []string
	codexDirs     []string
	copilotDirs   []string
	geminiDirs    []string
	opencodeDirs  []string
	machine       string
	syncMu        gosync.Mutex // serializes full sync runs
	mu            gosync.RWMutex
	lastSync      time.Time
	lastSyncStats SyncStats
	// skipCache tracks paths that should be skipped on
	// subsequent syncs, keyed by path with the file mtime
	// at time of caching. Covers parse errors and
	// non-interactive sessions (nil result). The file is
	// retried when its mtime changes.
	skipMu    gosync.RWMutex
	skipCache map[string]int64
}

// NewEngine creates a sync engine. It pre-populates the
// in-memory skip cache from the database so that files
// skipped in a prior run are not re-parsed on startup.
func NewEngine(
	database *db.DB,
	claudeDirs, codexDirs, copilotDirs,
	geminiDirs, opencodeDirs []string, machine string,
) *Engine {
	skipCache := make(map[string]int64)
	if loaded, err := database.LoadSkippedFiles(); err == nil {
		skipCache = loaded
	} else {
		log.Printf("loading skip cache: %v", err)
	}

	return &Engine{
		db:           database,
		claudeDirs:   claudeDirs,
		codexDirs:    codexDirs,
		copilotDirs:  copilotDirs,
		geminiDirs:   geminiDirs,
		opencodeDirs: opencodeDirs,
		machine:      machine,
		skipCache:    skipCache,
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
	e.persistSkipCache()

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
	geminiProjectsByDir := make(map[string]map[string]string)
	var files []DiscoveredFile
	for _, p := range paths {
		if df, ok := e.classifyOnePath(
			p, geminiProjectsByDir,
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
	geminiProjectsByDir map[string]map[string]string,
) (DiscoveredFile, bool) {
	sep := string(filepath.Separator)

	// Claude: <claudeDir>/<project>/<session>.jsonl
	for _, claudeDir := range e.claudeDirs {
		if claudeDir == "" {
			continue
		}
		if rel, ok := isUnder(claudeDir, path); ok {
			if !strings.HasSuffix(path, ".jsonl") {
				continue
			}
			stem := strings.TrimSuffix(
				filepath.Base(path), ".jsonl",
			)
			if strings.HasPrefix(stem, "agent-") {
				continue
			}
			parts := strings.Split(rel, sep)
			if len(parts) != 2 {
				continue
			}
			return DiscoveredFile{
				Path:    path,
				Project: parts[0],
				Agent:   parser.AgentClaude,
			}, true
		}
	}

	// Codex: <codexDir>/<year>/<month>/<day>/<file>.jsonl
	for _, codexDir := range e.codexDirs {
		if codexDir == "" {
			continue
		}
		if rel, ok := isUnder(codexDir, path); ok {
			parts := strings.Split(rel, sep)
			if len(parts) != 4 {
				continue
			}
			if !isDigits(parts[0]) ||
				!isDigits(parts[1]) ||
				!isDigits(parts[2]) {
				continue
			}
			if !strings.HasSuffix(parts[3], ".jsonl") {
				continue
			}
			return DiscoveredFile{
				Path:  path,
				Agent: parser.AgentCodex,
			}, true
		}
	}

	// Copilot: <copilotDir>/session-state/<uuid>.jsonl
	//      or: <copilotDir>/session-state/<uuid>/events.jsonl
	for _, copilotDir := range e.copilotDirs {
		if copilotDir == "" {
			continue
		}
		stateDir := filepath.Join(
			copilotDir, "session-state",
		)
		if rel, ok := isUnder(stateDir, path); ok {
			parts := strings.Split(rel, sep)
			switch len(parts) {
			case 1:
				stem, ok := strings.CutSuffix(
					parts[0], ".jsonl",
				)
				if !ok {
					continue
				}
				dirEvents := filepath.Join(
					stateDir, stem, "events.jsonl",
				)
				if _, err := os.Stat(dirEvents); err == nil {
					continue
				}
				return DiscoveredFile{
					Path:  path,
					Agent: parser.AgentCopilot,
				}, true
			case 2:
				if parts[1] != "events.jsonl" {
					continue
				}
				return DiscoveredFile{
					Path:  path,
					Agent: parser.AgentCopilot,
				}, true
			default:
				continue
			}
		}
	}

	// Gemini: <geminiDir>/tmp/<dir>/chats/session-*.json
	// <dir> is either a SHA-256 hash (old) or project name (new).
	for _, geminiDir := range e.geminiDirs {
		if geminiDir == "" {
			continue
		}
		if rel, ok := isUnder(geminiDir, path); ok {
			parts := strings.Split(rel, sep)
			if len(parts) != 4 ||
				parts[0] != "tmp" ||
				parts[2] != "chats" {
				continue
			}
			name := parts[3]
			if !strings.HasPrefix(name, "session-") ||
				!strings.HasSuffix(name, ".json") {
				continue
			}
			dirName := parts[1]
			if _, ok := geminiProjectsByDir[geminiDir]; !ok {
				geminiProjectsByDir[geminiDir] =
					buildGeminiProjectMap(geminiDir)
			}
			project := resolveGeminiProject(
				dirName, geminiProjectsByDir[geminiDir],
			)
			return DiscoveredFile{
				Path:    path,
				Project: project,
				Agent:   parser.AgentGemini,
			}, true
		}
	}

	return DiscoveredFile{}, false
}

// ResyncAll clears all skip caches and resets stored mtimes so
// that the subsequent SyncAll re-parses every file. This is the
// "full resync" path triggered from the UI when schema changes
// or parser fixes require re-processing without deleting the DB.
func (e *Engine) ResyncAll(
	onProgress ProgressFunc,
) SyncStats {
	// Serialize with SyncAll so pre-steps and the sync
	// itself run atomically.
	e.syncMu.Lock()
	defer e.syncMu.Unlock()

	// 1. Clear in-memory skip cache.
	e.skipMu.Lock()
	e.skipCache = make(map[string]int64)
	e.skipMu.Unlock()

	// 2. Clear persisted skip cache.
	if err := e.db.ReplaceSkippedFiles(
		map[string]int64{},
	); err != nil {
		log.Printf("resync: clear skipped files: %v", err)
	}

	// 3. Zero all stored mtimes so shouldSkipFile returns false.
	if err := e.db.ResetAllMtimes(); err != nil {
		log.Printf("resync: reset mtimes: %v", err)
	}

	return e.syncAllLocked(onProgress)
}

// SyncAll discovers and syncs all session files from all agents.
func (e *Engine) SyncAll(onProgress ProgressFunc) SyncStats {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	return e.syncAllLocked(onProgress)
}

func (e *Engine) syncAllLocked(
	onProgress ProgressFunc,
) SyncStats {
	t0 := time.Now()

	var claude, codex, copilot, gemini []DiscoveredFile
	for _, d := range e.claudeDirs {
		claude = append(claude, DiscoverClaudeProjects(d)...)
	}
	for _, d := range e.codexDirs {
		codex = append(codex, DiscoverCodexSessions(d)...)
	}
	for _, d := range e.copilotDirs {
		copilot = append(copilot, DiscoverCopilotSessions(d)...)
	}
	for _, d := range e.geminiDirs {
		gemini = append(gemini, DiscoverGeminiSessions(d)...)
	}

	all := make(
		[]DiscoveredFile, 0,
		len(claude)+len(codex)+len(copilot)+len(gemini),
	)
	all = append(all, claude...)
	all = append(all, codex...)
	all = append(all, copilot...)
	all = append(all, gemini...)

	verbose := onProgress == nil

	if verbose {
		log.Printf(
			"discovered %d files (%d claude, %d codex, %d copilot, %d gemini) in %s",
			len(all), len(claude), len(codex), len(copilot), len(gemini),
			time.Since(t0).Round(time.Millisecond),
		)
	}

	if onProgress != nil {
		onProgress(Progress{
			Phase:         PhaseSyncing,
			SessionsTotal: len(all),
		})
	}

	tWorkers := time.Now()
	results := e.startWorkers(all)
	stats := e.collectAndBatch(results, len(all), onProgress)
	if verbose {
		log.Printf(
			"file sync: %d synced, %d skipped in %s",
			stats.Synced, stats.Skipped,
			time.Since(tWorkers).Round(time.Millisecond),
		)
	}

	// Sync OpenCode sessions (DB-backed, not file-based).
	// Uses full replace because OpenCode messages can change
	// in place (streaming updates, tool result pairing).
	tOC := time.Now()
	ocPending := e.syncOpenCode()
	if len(ocPending) > 0 {
		stats.TotalSessions += len(ocPending)
		stats.RecordSynced(len(ocPending))
		tWrite := time.Now()
		for _, pw := range ocPending {
			e.writeSessionFull(pw)
		}
		if verbose {
			log.Printf(
				"opencode write: %d sessions in %s",
				len(ocPending),
				time.Since(tWrite).Round(time.Millisecond),
			)
		}
	}
	if verbose {
		log.Printf(
			"opencode sync: %s",
			time.Since(tOC).Round(time.Millisecond),
		)
	}

	tPersist := time.Now()
	skipCount := e.persistSkipCache()
	if verbose {
		log.Printf(
			"persist skip cache (%d entries): %s",
			skipCount,
			time.Since(tPersist).Round(time.Millisecond),
		)
	}

	e.mu.Lock()
	e.lastSync = time.Now()
	e.lastSyncStats = stats
	e.mu.Unlock()
	return stats
}

// syncOpenCode syncs sessions from OpenCode SQLite databases.
// Uses per-session time_updated to detect changes, so only
// modified sessions are fully parsed. Returns pending writes.
func (e *Engine) syncOpenCode() []pendingWrite {
	var allPending []pendingWrite
	for _, dir := range e.opencodeDirs {
		if dir == "" {
			continue
		}
		allPending = append(allPending, e.syncOneOpenCode(dir)...)
	}
	return allPending
}

// syncOneOpenCode handles a single OpenCode directory.
func (e *Engine) syncOneOpenCode(dir string) []pendingWrite {
	dbPath := filepath.Join(dir, "opencode.db")

	metas, err := parser.ListOpenCodeSessionMeta(dbPath)
	if err != nil {
		log.Printf("sync opencode: %v", err)
		return nil
	}
	if len(metas) == 0 {
		return nil
	}

	var changed []string
	for _, m := range metas {
		_, storedMtime, ok :=
			e.db.GetFileInfoByPath(m.VirtualPath)
		if ok && storedMtime == m.FileMtime {
			continue
		}
		changed = append(changed, m.SessionID)
	}
	if len(changed) == 0 {
		return nil
	}

	var pending []pendingWrite
	for _, sid := range changed {
		sess, msgs, err := parser.ParseOpenCodeSession(
			dbPath, sid, e.machine,
		)
		if err != nil {
			log.Printf(
				"opencode session %s: %v", sid, err,
			)
			continue
		}
		if sess == nil {
			continue
		}
		pending = append(pending, pendingWrite{
			sess: *sess,
			msgs: msgs,
		})
	}

	if len(pending) > 0 {
		log.Printf(
			"sync: %d opencode session(s) updated",
			len(pending),
		)
	}
	return pending
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
			if r.mtime != 0 {
				e.cacheSkip(r.path, r.mtime)
			}
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
		if r.sess == nil {
			e.cacheSkip(r.path, r.mtime)
			progress.SessionsDone++
			if onProgress != nil {
				onProgress(progress)
			}
			continue
		}
		e.clearSkip(r.path)

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
	sess  *parser.ParsedSession
	msgs  []parser.ParsedMessage
	skip  bool
	mtime int64
	err   error
}

func (e *Engine) processFile(
	file DiscoveredFile,
) processResult {

	info, err := os.Stat(file.Path)
	if err != nil {
		return processResult{
			err: fmt.Errorf("stat %s: %w", file.Path, err),
		}
	}

	// Capture mtime once from the initial stat so all
	// downstream cache operations use a consistent value.
	mtime := info.ModTime().UnixNano()

	// Skip files cached from a previous sync (parse errors
	// or non-interactive sessions) whose mtime is unchanged.
	e.skipMu.RLock()
	cachedMtime, cached := e.skipCache[file.Path]
	e.skipMu.RUnlock()
	if cached && cachedMtime == mtime {
		return processResult{skip: true, mtime: mtime}
	}

	var res processResult
	switch file.Agent {
	case parser.AgentClaude:
		res = e.processClaude(file, info)
	case parser.AgentCodex:
		res = e.processCodex(file, info)
	case parser.AgentCopilot:
		res = e.processCopilot(file, info)
	case parser.AgentGemini:
		res = e.processGemini(file, info)
	default:
		res = processResult{
			err: fmt.Errorf(
				"unknown agent type: %s", file.Agent,
			),
		}
	}
	res.mtime = mtime
	return res
}

// cacheSkip records a file so it won't be retried until
// its mtime changes.
func (e *Engine) cacheSkip(path string, mtime int64) {
	e.skipMu.Lock()
	e.skipCache[path] = mtime
	e.skipMu.Unlock()
}

// clearSkip removes a skip-cache entry when a file
// produces a valid session.
func (e *Engine) clearSkip(path string) {
	e.skipMu.Lock()
	delete(e.skipCache, path)
	e.skipMu.Unlock()
	_ = e.db.DeleteSkippedFile(path)
}

// persistSkipCache writes the in-memory skip cache to the
// database so skipped files survive process restarts.
// Returns the number of entries persisted.
func (e *Engine) persistSkipCache() int {
	e.skipMu.RLock()
	snapshot := make(map[string]int64, len(e.skipCache))
	maps.Copy(snapshot, e.skipCache)
	e.skipMu.RUnlock()

	if err := e.db.ReplaceSkippedFiles(snapshot); err != nil {
		log.Printf("persisting skip cache: %v", err)
	}
	return len(snapshot)
}

// shouldSkipFile returns true when the file's size and mtime
// match what is already stored in the database (by session ID).
// This relies on mtime changing on any write, which holds for
// append-only session files under normal filesystem behavior.
// The file hash is still computed and stored on successful sync
// for integrity; mtime is purely a skip-check optimization.
func (e *Engine) shouldSkipFile(
	sessionID string, info os.FileInfo,
) bool {
	storedSize, storedMtime, ok := e.db.GetSessionFileInfo(
		sessionID,
	)
	if !ok {
		return false
	}
	return storedSize == info.Size() &&
		storedMtime == info.ModTime().UnixNano()
}

// shouldSkipByPath checks file size and mtime against what is
// stored in the database by file_path. Used for codex/gemini
// files where the session ID requires parsing.
func (e *Engine) shouldSkipByPath(
	path string, info os.FileInfo,
) bool {
	storedSize, storedMtime, ok := e.db.GetFileInfoByPath(
		path,
	)
	if !ok {
		return false
	}
	return storedSize == info.Size() &&
		storedMtime == info.ModTime().UnixNano()
}

func (e *Engine) processClaude(
	file DiscoveredFile, info os.FileInfo,
) processResult {

	sessionID := strings.TrimSuffix(info.Name(), ".jsonl")

	if e.shouldSkipFile(sessionID, info) {
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

	// Fast path: skip by file_path + mtime before parsing.
	if e.shouldSkipByPath(file.Path, info) {
		return processResult{skip: true}
	}

	sess, msgs, err := parser.ParseCodexSession(
		file.Path, e.machine, false,
	)
	if err != nil {
		return processResult{err: err}
	}
	if sess == nil {
		return processResult{} // non-interactive
	}

	hash, err := ComputeFileHash(file.Path)
	if err == nil {
		sess.File.Hash = hash
	}

	return processResult{sess: sess, msgs: msgs}
}

func (e *Engine) processCopilot(
	file DiscoveredFile, info os.FileInfo,
) processResult {
	if e.shouldSkipByPath(file.Path, info) {
		return processResult{skip: true}
	}

	sess, msgs, err := parser.ParseCopilotSession(
		file.Path, e.machine,
	)
	if err != nil {
		return processResult{err: err}
	}
	if sess == nil {
		return processResult{}
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
	// Fast path: skip by file_path + mtime before parsing.
	if e.shouldSkipByPath(file.Path, info) {
		return processResult{skip: true}
	}

	sess, msgs, err := parser.ParseGeminiSession(
		file.Path, file.Project, e.machine,
	)
	if err != nil {
		return processResult{err: err}
	}
	if sess == nil {
		return processResult{}
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
		s := toDBSession(pw)
		if err := e.db.UpsertSession(s); err != nil {
			log.Printf("upsert session %s: %v", s.ID, err)
			continue
		}
		msgs := toDBMessages(pw)
		e.writeMessages(pw.sess.ID, msgs)
	}
}

// writeMessages uses an incremental append when possible.
// Session files are append-only, so if the DB already has
// messages for this session and the new set is larger, we
// only insert the new messages (avoiding expensive FTS5
// delete+reinsert of existing content).
func (e *Engine) writeMessages(
	sessionID string, msgs []db.Message,
) {
	maxOrd := e.db.MaxOrdinal(sessionID)

	// No existing messages — insert all.
	if maxOrd < 0 {
		if err := e.db.InsertMessages(msgs); err != nil {
			log.Printf(
				"insert messages for %s: %v",
				sessionID, err,
			)
		}
		return
	}

	// Find new messages (ordinal > maxOrd).
	delta := 0
	for i, m := range msgs {
		if m.Ordinal > maxOrd {
			delta = len(msgs) - i
			msgs = msgs[i:]
			break
		}
	}

	if delta == 0 {
		return
	}

	if err := e.db.InsertMessages(msgs); err != nil {
		log.Printf(
			"append messages for %s: %v",
			sessionID, err,
		)
	}
}

// writeSessionFull upserts a session and does a full
// delete+reinsert of its messages. Used by explicit
// single-session re-syncs where existing content may have
// changed (not just appended).
func (e *Engine) writeSessionFull(pw pendingWrite) {
	s := toDBSession(pw)
	if err := e.db.UpsertSession(s); err != nil {
		log.Printf("upsert session %s: %v", s.ID, err)
		return
	}
	msgs := toDBMessages(pw)
	if err := e.db.ReplaceSessionMessages(
		pw.sess.ID, msgs,
	); err != nil {
		log.Printf(
			"replace messages for %s: %v",
			pw.sess.ID, err,
		)
	}
}

// toDBSession converts a pendingWrite to a db.Session.
func toDBSession(pw pendingWrite) db.Session {
	s := db.Session{
		ID:               pw.sess.ID,
		Project:          pw.sess.Project,
		Machine:          pw.sess.Machine,
		Agent:            string(pw.sess.Agent),
		MessageCount:     pw.sess.MessageCount,
		UserMessageCount: pw.sess.UserMessageCount,
		ParentSessionID:  strPtr(pw.sess.ParentSessionID),
		FilePath:         strPtr(pw.sess.File.Path),
		FileSize:         int64Ptr(pw.sess.File.Size),
		FileMtime:        int64Ptr(pw.sess.File.Mtime),
		FileHash:         strPtr(pw.sess.File.Hash),
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
	return s
}

// toDBMessages converts parsed messages to db.Message rows
// with tool-result pairing and filtering applied.
func toDBMessages(pw pendingWrite) []db.Message {
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
	return pairAndFilter(msgs)
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
	case strings.HasPrefix(sessionID, "opencode:"):
		return ""
	case strings.HasPrefix(sessionID, "codex:"):
		for _, d := range e.codexDirs {
			if f := FindCodexSourceFile(d, sessionID[6:]); f != "" {
				return f
			}
		}
		return ""
	case strings.HasPrefix(sessionID, "copilot:"):
		for _, d := range e.copilotDirs {
			if f := FindCopilotSourceFile(d, sessionID[8:]); f != "" {
				return f
			}
		}
		return ""
	case strings.HasPrefix(sessionID, "gemini:"):
		for _, d := range e.geminiDirs {
			if f := FindGeminiSourceFile(d, sessionID[7:]); f != "" {
				return f
			}
		}
		return ""
	default:
		for _, d := range e.claudeDirs {
			if f := FindClaudeSourceFile(d, sessionID); f != "" {
				return f
			}
		}
		return ""
	}
}

// SyncSingleSession re-syncs a single session by its ID.
// Unlike the bulk SyncAll path, this includes exec-originated
// Codex sessions and uses the existing DB project as fallback.
func (e *Engine) SyncSingleSession(sessionID string) error {
	if strings.HasPrefix(sessionID, "opencode:") {
		return e.syncSingleOpenCode(sessionID)
	}

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
	case strings.HasPrefix(sessionID, "copilot:"):
		agent = parser.AgentCopilot
	case strings.HasPrefix(sessionID, "gemini:"):
		agent = parser.AgentGemini
	default:
		agent = parser.AgentClaude
	}

	// Clear skip cache so explicit re-sync always processes
	// the file, even if it was cached as non-interactive
	// during a bulk SyncAll.
	e.clearSkip(path)

	// Reuse processFile for stat and DB-skip logic. For
	// Claude this is the full pipeline; for Codex we need
	// includeExec=true so we call the parser directly.
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
		if res.mtime != 0 {
			e.cacheSkip(path, res.mtime)
		}
		return res.err
	}
	if res.skip {
		return nil
	}

	// For Codex, processFile uses includeExec=false which may
	// return nil sess for exec-originated sessions. Re-parse
	// with includeExec=true when that happens.
	if res.sess == nil && agent == parser.AgentCodex {
		execRes := e.processCodexIncludeExec(file)
		if execRes.err != nil {
			if res.mtime != 0 {
				e.cacheSkip(path, res.mtime)
			}
			return execRes.err
		}
		if execRes.sess == nil {
			return nil
		}
		res.sess = execRes.sess
		res.msgs = execRes.msgs
	}

	if res.sess == nil {
		return nil
	}

	e.writeSessionFull(
		pendingWrite{sess: *res.sess, msgs: res.msgs},
	)
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

// syncSingleOpenCode re-syncs a single OpenCode session.
func (e *Engine) syncSingleOpenCode(
	sessionID string,
) error {
	rawID := strings.TrimPrefix(sessionID, "opencode:")

	var lastErr error
	for _, dir := range e.opencodeDirs {
		if dir == "" {
			continue
		}
		dbPath := filepath.Join(dir, "opencode.db")
		sess, msgs, err := parser.ParseOpenCodeSession(
			dbPath, rawID, e.machine,
		)
		if err != nil {
			lastErr = err
			continue
		}
		if sess == nil {
			continue
		}
		e.writeSessionFull(
			pendingWrite{sess: *sess, msgs: msgs},
		)
		return nil
	}

	if len(e.opencodeDirs) == 0 {
		return fmt.Errorf("opencode dir not configured")
	}
	if lastErr != nil {
		return fmt.Errorf(
			"opencode session %s: %w", sessionID, lastErr,
		)
	}
	return fmt.Errorf("opencode session %s not found", sessionID)
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

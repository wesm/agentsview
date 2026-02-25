package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/wesm/agentsview/internal/parser"
)

// uuidRe matches a standard UUID (8-4-4-4-12 hex) at the end of a rollout filename stem.
var uuidRe = regexp.MustCompile(
	`^rollout-.*-([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-` +
		`[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`,
)

// isDirOrSymlink reports whether the entry is a directory or a
// symlink that resolves to a directory. parentDir is needed to
// build the full path for symlink resolution.
func isDirOrSymlink(
	entry os.DirEntry, parentDir string,
) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	fi, err := os.Stat(
		filepath.Join(parentDir, entry.Name()),
	)
	return err == nil && fi.IsDir()
}

// DiscoveredFile holds a discovered session JSONL file.
type DiscoveredFile struct {
	Path    string
	Project string           // pre-extracted project name
	Agent   parser.AgentType // AgentClaude or AgentCodex
}

// DiscoverClaudeProjects finds all project directories under the
// Claude projects dir and returns their JSONL session files.
func DiscoverClaudeProjects(projectsDir string) []DiscoveredFile {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	var files []DiscoveredFile
	for _, entry := range entries {
		if !isDirOrSymlink(entry, projectsDir) {
			continue
		}

		projDir := filepath.Join(projectsDir, entry.Name())
		sessionFiles, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}

		for _, sf := range sessionFiles {
			if sf.IsDir() {
				continue
			}
			name := sf.Name()
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			stem := strings.TrimSuffix(name, ".jsonl")
			if strings.HasPrefix(stem, "agent-") {
				continue
			}
			files = append(files, DiscoveredFile{
				Path:    filepath.Join(projDir, name),
				Project: entry.Name(),
				Agent:   parser.AgentClaude,
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// DiscoverCodexSessions finds all JSONL files under the Codex
// sessions dir (year/month/day structure).
func DiscoverCodexSessions(sessionsDir string) []DiscoveredFile {
	var files []DiscoveredFile

	walkCodexDayDirs(sessionsDir, func(dayPath string) bool {
		entries, err := os.ReadDir(dayPath)
		if err != nil {
			return true
		}
		for _, sf := range entries {
			if sf.IsDir() {
				continue
			}
			if !strings.HasSuffix(sf.Name(), ".jsonl") {
				continue
			}
			files = append(files, DiscoveredFile{
				Path:  filepath.Join(dayPath, sf.Name()),
				Agent: parser.AgentCodex,
			})
		}
		return true
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// FindClaudeSourceFile finds the original JSONL file for a Claude
// session ID by searching all project directories.
func FindClaudeSourceFile(
	projectsDir, sessionID string,
) string {
	if !isValidSessionID(sessionID) {
		return ""
	}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	target := sessionID + ".jsonl"
	for _, entry := range entries {
		if !isDirOrSymlink(entry, projectsDir) {
			continue
		}
		candidate := filepath.Join(
			projectsDir, entry.Name(), target,
		)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// FindCodexSourceFile finds a Codex session file by UUID.
// Searches the year/month/day directory structure for files matching
// rollout-{timestamp}-{uuid}.jsonl.
func FindCodexSourceFile(sessionsDir, sessionID string) string {
	if !isValidSessionID(sessionID) {
		return ""
	}

	var result string
	walkCodexDayDirs(sessionsDir, func(dayPath string) bool {
		if result != "" {
			return false
		}
		entries, err := os.ReadDir(dayPath)
		if err != nil {
			return true
		}
		for _, f := range entries {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if !strings.HasPrefix(name, "rollout-") ||
				!strings.HasSuffix(name, ".jsonl") {
				continue
			}
			if extractUUIDFromRollout(name) == sessionID {
				result = filepath.Join(dayPath, name)
				return false
			}
		}
		return true
	})
	return result
}

// walkCodexDayDirs traverses a Codex sessions directory with
// year/month/day structure, calling fn for each valid day directory.
// fn returns false to stop traversal.
func walkCodexDayDirs(
	root string, fn func(dayPath string) bool,
) {
	years, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, year := range years {
		if !year.IsDir() || !isDigits(year.Name()) {
			continue
		}
		yearPath := filepath.Join(root, year.Name())
		months, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		for _, month := range months {
			if !month.IsDir() || !isDigits(month.Name()) {
				continue
			}
			monthPath := filepath.Join(yearPath, month.Name())
			days, err := os.ReadDir(monthPath)
			if err != nil {
				continue
			}
			for _, day := range days {
				if !day.IsDir() || !isDigits(day.Name()) {
					continue
				}
				if !fn(filepath.Join(monthPath, day.Name())) {
					return
				}
			}
		}
	}
}

// extractUUIDFromRollout extracts the UUID from a Codex filename
// like rollout-{timestamp}-{uuid}.jsonl using regex matching on the
// standard 8-4-4-4-12 hex format.
func extractUUIDFromRollout(filename string) string {
	stem := strings.TrimSuffix(filename, ".jsonl")
	match := uuidRe.FindStringSubmatch(stem)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isValidSessionID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !isAlphanumOrDashUnderscore(c) {
			return false
		}
	}
	return true
}

func isAlphanumOrDashUnderscore(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

// DiscoverGeminiSessions finds all session JSON files under
// the Gemini directory (~/.gemini/tmp/*/chats/session-*.json).
func DiscoverGeminiSessions(
	geminiDir string,
) []DiscoveredFile {
	if geminiDir == "" {
		return nil
	}

	tmpDir := filepath.Join(geminiDir, "tmp")
	hashDirs, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}

	projectMap := buildGeminiProjectMap(geminiDir)

	var files []DiscoveredFile
	for _, hd := range hashDirs {
		if !isDirOrSymlink(hd, tmpDir) {
			continue
		}
		hash := hd.Name()
		chatsDir := filepath.Join(tmpDir, hash, "chats")
		entries, err := os.ReadDir(chatsDir)
		if err != nil {
			continue
		}

		project := resolveGeminiProject(hash, projectMap)

		for _, sf := range entries {
			if sf.IsDir() {
				continue
			}
			name := sf.Name()
			if !strings.HasPrefix(name, "session-") ||
				!strings.HasSuffix(name, ".json") {
				continue
			}
			files = append(files, DiscoveredFile{
				Path:    filepath.Join(chatsDir, name),
				Project: project,
				Agent:   parser.AgentGemini,
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// FindGeminiSourceFile locates a Gemini session file by its
// session UUID. Searches all project hash directories.
func FindGeminiSourceFile(
	geminiDir, sessionID string,
) string {
	if geminiDir == "" || !isValidSessionID(sessionID) ||
		len(sessionID) < 8 {
		return ""
	}

	tmpDir := filepath.Join(geminiDir, "tmp")
	hashDirs, err := os.ReadDir(tmpDir)
	if err != nil {
		return ""
	}

	for _, hd := range hashDirs {
		if !isDirOrSymlink(hd, tmpDir) {
			continue
		}
		chatsDir := filepath.Join(tmpDir, hd.Name(), "chats")
		entries, err := os.ReadDir(chatsDir)
		if err != nil {
			continue
		}
		for _, sf := range entries {
			if sf.IsDir() {
				continue
			}
			name := sf.Name()
			if !strings.HasPrefix(name, "session-") ||
				!strings.HasSuffix(name, ".json") {
				continue
			}
			// The UUID prefix appears in the filename:
			// session-<timestamp>-<uuid-prefix>.json
			if strings.Contains(name, sessionID[:8]) {
				path := filepath.Join(chatsDir, name)
				if confirmGeminiSessionID(
					path, sessionID,
				) {
					return path
				}
			}
		}
	}
	return ""
}

// confirmGeminiSessionID reads the sessionId field from a
// Gemini file to confirm it matches the expected ID.
func confirmGeminiSessionID(
	path, sessionID string,
) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return parser.GeminiSessionID(data) == sessionID
}

// geminiProjectsFile holds the structure of
// ~/.gemini/projects.json.
type geminiProjectsFile struct {
	Projects map[string]string `json:"projects"`
}

// geminiTrustedFoldersFile holds the structure of
// ~/.gemini/trustedFolders.json.
type geminiTrustedFoldersFile struct {
	TrustedFolders []string `json:"trustedFolders"`
}

// buildGeminiProjectMap reads ~/.gemini/projects.json and
// ~/.gemini/trustedFolders.json to build a map from directory
// name to resolved project name. Entries are keyed by both the
// SHA-256 hash of the absolute path (old Gemini format) and
// the short project name from the JSON value (new format).
// trustedFolders.json provides additional path-to-hash
// mappings for directories that projects.json has lost.
// ExtractProjectFromCwd resolves worktrees to the main repo.
func buildGeminiProjectMap(
	geminiDir string,
) map[string]string {
	result := make(map[string]string)

	data, err := os.ReadFile(
		filepath.Join(geminiDir, "projects.json"),
	)
	if err == nil {
		var pf geminiProjectsFile
		if err := json.Unmarshal(data, &pf); err == nil {
			addProjectPaths(result, pf.Projects)
		}
	}

	// trustedFolders.json lists additional project paths
	// that may not be in projects.json (Gemini CLI cleans
	// up projects.json but trustedFolders.json persists
	// longer). Format: {"trustedFolders": ["/abs/path",...]}
	tfData, err := os.ReadFile(
		filepath.Join(geminiDir, "trustedFolders.json"),
	)
	if err == nil {
		var tf geminiTrustedFoldersFile
		if err := json.Unmarshal(tfData, &tf); err == nil {
			paths := make(
				map[string]string, len(tf.TrustedFolders),
			)
			for _, p := range tf.TrustedFolders {
				paths[p] = ""
			}
			addProjectPaths(result, paths)
		}
	}

	return result
}

// addProjectPaths adds hash and name entries for the given
// absolute paths. name is the short project name from
// projects.json (empty for trustedFolders.json entries).
// Existing entries are not overwritten.
func addProjectPaths(
	result map[string]string,
	paths map[string]string,
) {
	// Sort keys for deterministic first-seen-wins on
	// duplicate short names.
	sorted := make([]string, 0, len(paths))
	for absPath := range paths {
		sorted = append(sorted, absPath)
	}
	sort.Strings(sorted)

	for _, absPath := range sorted {
		name := paths[absPath]
		project := parser.ExtractProjectFromCwd(absPath)
		if project == "" {
			project = "unknown"
		}
		hash := geminiPathHash(absPath)
		if _, exists := result[hash]; !exists {
			result[hash] = project
		}
		if name != "" {
			if _, exists := result[name]; !exists {
				result[name] = project
			}
		}
	}
}

// geminiPathHash computes the SHA-256 hex hash of a path,
// matching Gemini CLI's project hash algorithm.
func geminiPathHash(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h)
}

// isHexHash reports whether s is a 64-character lowercase hex
// string (i.e. a SHA-256 hash).
func isHexHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// resolveGeminiProject maps a tmp/ subdirectory name to a
// project name. Newer Gemini CLI versions use the project name
// directly; older versions use a SHA-256 hash. Both are looked
// up in the project map (built from projects.json) which
// resolves worktrees to the main repository via
// ExtractProjectFromCwd. For named dirs not in the map, the
// directory name itself is used.
func resolveGeminiProject(
	dirName string,
	projectMap map[string]string,
) string {
	if p := projectMap[dirName]; p != "" {
		return p
	}
	// Old-format dirs use a SHA-256 hash of the project path.
	// If the hash isn't in the project map (e.g. projects.json
	// was cleaned up), we can't resolve it. A project name
	// that coincidentally matches the 64-char hex pattern
	// would also hit this path, but that's vanishingly
	// unlikely compared to orphaned hashes.
	if isHexHash(dirName) {
		return "unknown"
	}
	return parser.NormalizeName(dirName)
}

// DiscoverCopilotSessions finds all JSONL files under
// <copilotDir>/session-state/. Supports both bare format
// (<uuid>.jsonl) and directory format (<uuid>/events.jsonl).
func DiscoverCopilotSessions(
	copilotDir string,
) []DiscoveredFile {
	if copilotDir == "" {
		return nil
	}

	stateDir := filepath.Join(copilotDir, "session-state")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil
	}

	// Collect directories that actually contain events.jsonl
	// so we can skip bare files that have a valid directory
	// counterpart.
	dirs := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		eventsPath := filepath.Join(
			stateDir, entry.Name(), "events.jsonl",
		)
		if _, err := os.Stat(eventsPath); err == nil {
			dirs[entry.Name()] = struct{}{}
		}
	}

	var files []DiscoveredFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			// Directory format: <uuid>/events.jsonl
			candidate := filepath.Join(
				stateDir, name, "events.jsonl",
			)
			if _, err := os.Stat(candidate); err == nil {
				files = append(files, DiscoveredFile{
					Path:  candidate,
					Agent: parser.AgentCopilot,
				})
			}
			continue
		}
		// Bare format: <uuid>.jsonl — skip if a directory
		// with the same stem exists (prefer directory format).
		if stem, ok := strings.CutSuffix(name, ".jsonl"); ok {
			if _, dup := dirs[stem]; dup {
				continue
			}
			files = append(files, DiscoveredFile{
				Path:  filepath.Join(stateDir, name),
				Agent: parser.AgentCopilot,
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// FindCopilotSourceFile locates a Copilot session file by
// UUID. Checks both bare (<uuid>.jsonl) and directory
// (<uuid>/events.jsonl) layouts.
func FindCopilotSourceFile(
	copilotDir, rawID string,
) string {
	if copilotDir == "" || !isValidSessionID(rawID) {
		return ""
	}

	stateDir := filepath.Join(copilotDir, "session-state")

	// Check directory format first (matches discovery
	// precedence which prefers directory over bare).
	dirFmt := filepath.Join(stateDir, rawID, "events.jsonl")
	if _, err := os.Stat(dirFmt); err == nil {
		return dirFmt
	}

	// Fall back to bare format.
	bare := filepath.Join(stateDir, rawID+".jsonl")
	if _, err := os.Stat(bare); err == nil {
		return bare
	}

	return ""
}

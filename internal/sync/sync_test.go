package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wesm/agentsview/internal/dbtest"
	"github.com/wesm/agentsview/internal/parser"
)

// setupTestDir creates a temporary directory and populates
// it with the given relative file paths (each containing
// "{}").
func setupTestDir(
	t *testing.T, relativePaths []string,
) string {
	t.Helper()
	dir := t.TempDir()
	for _, p := range relativePaths {
		dbtest.WriteTestFile(
			t, filepath.Join(dir, p), []byte("{}"),
		)
	}
	return dir
}

// assertDiscoveredFiles verifies that the discovered files match the expected filenames and agent type.
func assertDiscoveredFiles(t *testing.T, got []DiscoveredFile, wantFilenames []string, wantAgent parser.AgentType) {
	t.Helper()

	want := make(map[string]bool)
	for _, f := range wantFilenames {
		want[f] = true
	}

	gotMap := make(map[string]bool)
	for _, f := range got {
		base := filepath.Base(f.Path)
		gotMap[base] = true
		if f.Agent != wantAgent {
			t.Errorf("file %q: agent = %q, want %q", base, f.Agent, wantAgent)
		}
	}

	if len(got) != len(want) {
		t.Errorf("got %d files total, want %d", len(got), len(want))
	}

	for file := range want {
		if !gotMap[file] {
			t.Errorf("missing expected file: %q", file)
		}
	}

	// Check for unexpected files
	for file := range gotMap {
		if !want[file] {
			t.Errorf("got unexpected file: %q", file)
		}
	}
}

func TestDiscoverClaudeProjects(t *testing.T) {
	dir := setupTestDir(t, []string{
		filepath.Join("project-a", "abc.jsonl"),
		filepath.Join("project-a", "def.jsonl"),
		filepath.Join("project-a", "agent-123.jsonl"), // Should be ignored
		filepath.Join("project-b", "xyz.jsonl"),
	})

	files := DiscoverClaudeProjects(dir)

	assertDiscoveredFiles(t, files, []string{
		"abc.jsonl",
		"def.jsonl",
		"xyz.jsonl",
	}, parser.AgentClaude)
}

func TestDiscoverClaudeProjectsEmpty(t *testing.T) {
	dir := t.TempDir()
	files := DiscoverClaudeProjects(dir)
	assertDiscoveredFiles(t, files, nil, parser.AgentClaude)
}

func TestDiscoverClaudeProjectsNonexistent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	files := DiscoverClaudeProjects(dir)
	if files != nil {
		t.Errorf("expected nil, got %d files", len(files))
	}
}

func TestDiscoverCodexSessions(t *testing.T) {
	file1 := "rollout-123-abc-def-ghi-jkl-mno.jsonl"
	file2 := "rollout-456-abc-def-ghi-jkl-mno.jsonl"

	dir := setupTestDir(t, []string{
		filepath.Join("2024", "01", "15", file1),
		filepath.Join("2024", "02", "01", file2),
	})

	files := DiscoverCodexSessions(dir)

	assertDiscoveredFiles(t, files, []string{
		file1,
		file2,
	}, parser.AgentCodex)
}

func TestDiscoverCodexSessionsSkipsNonDigit(t *testing.T) {
	// Non-digit directory should be ignored
	dir := setupTestDir(t, []string{
		filepath.Join("notes", "01", "01", "x.jsonl"),
	})

	files := DiscoverCodexSessions(dir)
	assertDiscoveredFiles(t, files, nil, parser.AgentCodex)
}

func TestFindClaudeSourceFile(t *testing.T) {
	relPath := filepath.Join("project-a", "session-abc.jsonl")
	dir := setupTestDir(t, []string{relPath})

	expected := filepath.Join(dir, relPath)

	got := FindClaudeSourceFile(dir, "session-abc")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}

	// Nonexistent
	got = FindClaudeSourceFile(dir, "nonexistent")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFindClaudeSourceFileValidation(t *testing.T) {
	dir := t.TempDir()

	// Invalid session IDs should return empty
	tests := []string{"", "../etc/passwd", "a/b", "a b"}
	for _, id := range tests {
		got := FindClaudeSourceFile(dir, id)
		if got != "" {
			t.Errorf("FindClaudeSourceFile(%q) = %q, want empty",
				id, got)
		}
	}
}

func TestFindCodexSourceFile(t *testing.T) {
	uuid := "abc12345-1234-5678-9abc-def012345678"
	filename := "rollout-20240115-" + uuid + ".jsonl"
	relPath := filepath.Join("2024", "01", "15", filename)

	dir := setupTestDir(t, []string{relPath})
	expected := filepath.Join(dir, relPath)

	got := FindCodexSourceFile(dir, uuid)
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestExtractUUIDFromRollout(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{
			"rollout-20240115-abc12345-1234-5678-9abc-def012345678.jsonl",
			"abc12345-1234-5678-9abc-def012345678",
		},
		{
			"rollout-20240115T100000-abc12345-1234-5678-9abc-def012345678.jsonl",
			"abc12345-1234-5678-9abc-def012345678",
		},
		{
			"short.jsonl",
			"",
		},
		{
			"rollout-20240115-12345678-1234-1234-1234-1234567890ab-abc12345-1234-5678-9abc-def012345678.jsonl",
			"abc12345-1234-5678-9abc-def012345678",
		},
		{
			"rollout-20240115-abc12345-1234-5678-9abc-def012345678-suffix.jsonl",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractUUIDFromRollout(tt.filename)
			if got != tt.want {
				t.Errorf("extractUUID(%q) = %q, want %q",
					tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsValidSessionID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"abc-123", true},
		{"session_1", true},
		{"abc123", true},
		{"", false},
		{"../etc", false},
		{"a b", false},
		{"a/b", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isValidSessionID(tt.id)
			if got != tt.want {
				t.Errorf("isValidSessionID(%q) = %v, want %v",
					tt.id, got, tt.want)
			}
		})
	}
}

func TestIsDigits(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"123", true},
		{"0", true},
		{"", false},
		{"12a", false},
		{"abc", false},
		{"１２３", true}, // Fullwidth digits are supported
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := isDigits(tt.s)
			if got != tt.want {
				t.Errorf("isDigits(%q) = %v, want %v",
					tt.s, got, tt.want)
			}
		})
	}
}

func TestDiscoverGeminiSessions(t *testing.T) {
	dir := setupTestDir(t, []string{
		filepath.Join("tmp", "hash1", "chats", "session-2026-01-01T10-00-abc123.json"),
		filepath.Join("tmp", "hash1", "chats", "session-2026-01-02T10-00-def456.json"),
		filepath.Join("tmp", "hash2", "chats", "session-2026-01-03T10-00-ghi789.json"),
	})

	files := DiscoverGeminiSessions(dir)

	assertDiscoveredFiles(t, files, []string{
		"session-2026-01-01T10-00-abc123.json",
		"session-2026-01-02T10-00-def456.json",
		"session-2026-01-03T10-00-ghi789.json",
	}, parser.AgentGemini)
}

func TestDiscoverGeminiSessionsNoChatDir(t *testing.T) {
	// Hash dir exists but has no chats/ subdirectory
	dir := setupTestDir(t, []string{
		filepath.Join("tmp", "hash1", "other.txt"),
	})

	files := DiscoverGeminiSessions(dir)
	assertDiscoveredFiles(t, files, nil, parser.AgentGemini)
}

func TestDiscoverGeminiSessionsEmptyChatDir(t *testing.T) {
	dir := t.TempDir()
	chatsDir := filepath.Join(dir, "tmp", "hash1", "chats")
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	files := DiscoverGeminiSessions(dir)
	assertDiscoveredFiles(t, files, nil, parser.AgentGemini)
}

func TestDiscoverGeminiSessionsNonexistent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	files := DiscoverGeminiSessions(dir)
	if files != nil {
		t.Errorf("expected nil, got %d files", len(files))
	}
}

func TestDiscoverGeminiSessionsSkipsNonSessionFiles(t *testing.T) {
	dir := setupTestDir(t, []string{
		filepath.Join("tmp", "hash1", "chats", "session-abc.json"),
		filepath.Join("tmp", "hash1", "chats", "other.json"),
		filepath.Join("tmp", "hash1", "chats", "session-def.txt"),
	})

	files := DiscoverGeminiSessions(dir)
	assertDiscoveredFiles(t, files, []string{
		"session-abc.json",
	}, parser.AgentGemini)
}

func TestFindGeminiSourceFile(t *testing.T) {
	sessionID := "b0a4eadd-cb99-4165-94d9-64cad5a66d24"
	sessionContent := `{"sessionId":"` + sessionID + `","messages":[]}`
	filename := "session-2026-01-19T18-21-b0a4eadd.json"

	dir := t.TempDir()
	chatsDir := filepath.Join(dir, "tmp", "hash1", "chats")
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(chatsDir, filename)
	if err := os.WriteFile(
		path, []byte(sessionContent), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := FindGeminiSourceFile(dir, sessionID)
	if got != path {
		t.Errorf("got %q, want %q", got, path)
	}

	// Nonexistent session
	got = FindGeminiSourceFile(dir, "nonexistent-uuid-1234")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestGeminiPathHash(t *testing.T) {
	// Known SHA-256 of "/Users/alice/code/sample-repo"
	hash := geminiPathHash("/Users/alice/code/sample-repo")
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
	// Hash should be deterministic
	if geminiPathHash("/Users/alice/code/sample-repo") != hash {
		t.Error("hash not deterministic")
	}
}

func TestBuildGeminiProjectMap(t *testing.T) {
	dir := t.TempDir()
	projectsJSON := `{"projects":{"/Users/alice/code/my-app":"my-app"}}`
	if err := os.WriteFile(
		filepath.Join(dir, "projects.json"),
		[]byte(projectsJSON), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := buildGeminiProjectMap(dir)
	hash := geminiPathHash("/Users/alice/code/my-app")
	if m[hash] != "my_app" {
		t.Errorf("project for hash = %q, want %q",
			m[hash], "my_app")
	}
}

func TestBuildGeminiProjectMapMissingFile(t *testing.T) {
	m := buildGeminiProjectMap(t.TempDir())
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestFindGeminiSourceFileShortID(t *testing.T) {
	dir := t.TempDir()
	// IDs shorter than 8 chars should return empty, not panic
	for _, id := range []string{"", "a", "abc", "1234567"} {
		got := FindGeminiSourceFile(dir, id)
		if got != "" {
			t.Errorf(
				"FindGeminiSourceFile(%q) = %q, want empty",
				id, got,
			)
		}
	}
}

func TestDiscoverGeminiSessionsEmptyDir(t *testing.T) {
	files := DiscoverGeminiSessions("")
	if files != nil {
		t.Errorf("expected nil, got %d files", len(files))
	}
}

func TestFindGeminiSourceFileEmptyDir(t *testing.T) {
	got := FindGeminiSourceFile(
		"", "b0a4eadd-cb99-4165-94d9-64cad5a66d24",
	)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- Copilot discovery tests ---

func TestDiscoverCopilotSessions_BareFormat(t *testing.T) {
	dir := setupTestDir(t, []string{
		filepath.Join("session-state", "abc-123.jsonl"),
		filepath.Join("session-state", "def-456.jsonl"),
	})

	files := DiscoverCopilotSessions(dir)
	assertDiscoveredFiles(t, files, []string{
		"abc-123.jsonl",
		"def-456.jsonl",
	}, parser.AgentCopilot)
}

func TestDiscoverCopilotSessions_DirFormat(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "session-state")

	// Create directory-format sessions with events.jsonl
	for _, id := range []string{"sess-1", "sess-2"} {
		sessDir := filepath.Join(stateDir, id)
		if err := os.MkdirAll(sessDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(sessDir, "events.jsonl")
		if err := os.WriteFile(
			path, []byte("{}"), 0o644,
		); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	files := DiscoverCopilotSessions(dir)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	for _, f := range files {
		if f.Agent != parser.AgentCopilot {
			t.Errorf("agent = %q, want %q",
				f.Agent, parser.AgentCopilot)
		}
		if filepath.Base(f.Path) != "events.jsonl" {
			t.Errorf("base = %q, want events.jsonl",
				filepath.Base(f.Path))
		}
	}
}

func TestDiscoverCopilotSessions_Mixed(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "session-state")

	// Bare format
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stateDir, "bare-1.jsonl"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Directory format
	sessDir := filepath.Join(stateDir, "dir-1")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(sessDir, "events.jsonl"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	files := DiscoverCopilotSessions(dir)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	for _, f := range files {
		if f.Agent != parser.AgentCopilot {
			t.Errorf("agent = %q, want %q",
				f.Agent, parser.AgentCopilot)
		}
	}
}

func TestDiscoverCopilotSessions_BareWithInvalidDir(
	t *testing.T,
) {
	// A directory without events.jsonl should not suppress
	// the bare file with the same stem.
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "session-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	uuid := "invalid-dir-uuid"
	if err := os.WriteFile(
		filepath.Join(stateDir, uuid+".jsonl"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write bare: %v", err)
	}
	// Directory exists but has no events.jsonl.
	if err := os.MkdirAll(
		filepath.Join(stateDir, uuid), 0o755,
	); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}

	files := DiscoverCopilotSessions(dir)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	wantPath := filepath.Join(stateDir, uuid+".jsonl")
	if files[0].Path != wantPath {
		t.Errorf("path = %q, want %q",
			files[0].Path, wantPath)
	}
}

func TestDiscoverCopilotSessions_DedupBareAndDir(
	t *testing.T,
) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "session-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Both bare and directory format for the same UUID.
	uuid := "dup-uuid-1234"
	if err := os.WriteFile(
		filepath.Join(stateDir, uuid+".jsonl"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write bare: %v", err)
	}
	sessDir := filepath.Join(stateDir, uuid)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(sessDir, "events.jsonl"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write dir: %v", err)
	}

	files := DiscoverCopilotSessions(dir)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1 (dedup)", len(files))
	}
	// Directory format should win.
	want := filepath.Join(sessDir, "events.jsonl")
	if files[0].Path != want {
		t.Errorf("path = %q, want %q", files[0].Path, want)
	}
}

func TestDiscoverCopilotSessions_DirWithoutEvents(
	t *testing.T,
) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "session-state")
	// Directory exists but has no events.jsonl
	sessDir := filepath.Join(stateDir, "no-events")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(sessDir, "other.txt"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	files := DiscoverCopilotSessions(dir)
	assertDiscoveredFiles(
		t, files, nil, parser.AgentCopilot,
	)
}

func TestDiscoverCopilotSessions_EmptyDir(t *testing.T) {
	files := DiscoverCopilotSessions("")
	if files != nil {
		t.Errorf("expected nil, got %d files", len(files))
	}
}

func TestDiscoverCopilotSessions_Nonexistent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	files := DiscoverCopilotSessions(dir)
	if files != nil {
		t.Errorf("expected nil, got %d files", len(files))
	}
}

func TestFindCopilotSourceFile_Bare(t *testing.T) {
	dir := setupTestDir(t, []string{
		filepath.Join("session-state", "abc-123.jsonl"),
	})
	expected := filepath.Join(
		dir, "session-state", "abc-123.jsonl",
	)

	got := FindCopilotSourceFile(dir, "abc-123")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFindCopilotSourceFile_DirFormat(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(
		dir, "session-state", "sess-42",
	)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	expected := filepath.Join(sessDir, "events.jsonl")
	if err := os.WriteFile(
		expected, []byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := FindCopilotSourceFile(dir, "sess-42")
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFindCopilotSourceFile_Nonexistent(t *testing.T) {
	dir := setupTestDir(t, []string{
		filepath.Join("session-state", "abc-123.jsonl"),
	})

	got := FindCopilotSourceFile(dir, "nonexistent")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFindCopilotSourceFile_InvalidID(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{
		"", "../etc/passwd", "a/b", "a b",
	} {
		got := FindCopilotSourceFile(dir, id)
		if got != "" {
			t.Errorf(
				"FindCopilotSourceFile(%q) = %q, want empty",
				id, got,
			)
		}
	}
}

func TestFindCopilotSourceFile_EmptyDir(t *testing.T) {
	got := FindCopilotSourceFile("", "abc-123")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFindCopilotSourceFile_DirPreferred(t *testing.T) {
	// When both bare and directory format exist, directory is
	// preferred (matching discovery precedence).
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "session-state")

	// Create bare file
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stateDir, "dual-1.jsonl"),
		[]byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Create directory format too
	sessDir := filepath.Join(stateDir, "dual-1")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dirPath := filepath.Join(sessDir, "events.jsonl")
	if err := os.WriteFile(
		dirPath, []byte("{}"), 0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := FindCopilotSourceFile(dir, "dual-1")
	if got != dirPath {
		t.Errorf("got %q, want dir path %q", got, dirPath)
	}
}

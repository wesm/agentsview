package parser

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openCodeSchema matches the real OpenCode database schema.
// Role and part type live inside the JSON data columns.
const openCodeSchema = `
CREATE TABLE project (
	id TEXT PRIMARY KEY,
	worktree TEXT NOT NULL,
	time_created INTEGER NOT NULL DEFAULT 0,
	time_updated INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE session (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	parent_id TEXT,
	title TEXT,
	time_created INTEGER NOT NULL,
	time_updated INTEGER NOT NULL,
	FOREIGN KEY (project_id) REFERENCES project(id)
);

CREATE TABLE message (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	time_created INTEGER NOT NULL,
	time_updated INTEGER NOT NULL,
	data TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES session(id)
);

CREATE TABLE part (
	id TEXT PRIMARY KEY,
	message_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	time_created INTEGER NOT NULL,
	time_updated INTEGER NOT NULL,
	data TEXT NOT NULL,
	FOREIGN KEY (message_id) REFERENCES message(id)
);
`

func createTestOpenCodeDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(openCodeSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return dbPath
}

func seedStandardSession(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`INSERT INTO project (id, worktree)
		 VALUES ('prj_1', '/home/user/code/myapp')`,
		`INSERT INTO session
		 (id, project_id, title, time_created, time_updated)
		 VALUES ('ses_abc', 'prj_1', 'Test Session',
		         1700000000000, 1700000060000)`,
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_1', 'ses_abc', 1700000000000,
		         1700000000000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_1', 'msg_1', 'ses_abc',
		         1700000000000, 1700000000000,
		         '{"type":"text","text":"Hello, help me with Go"}')`,
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_2', 'ses_abc', 1700000010000,
		         1700000010000,
		         '{"role":"assistant"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_2', 'msg_2', 'ses_abc',
		         1700000010000, 1700000010000,
		         '{"type":"text","text":"Sure, I can help with Go."}')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestParseOpenCodeDB_StandardSession(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	seedStandardSession(t, dbPath)

	sessions, err := ParseOpenCodeDB(dbPath, "testmachine")
	if err != nil {
		t.Fatalf("ParseOpenCodeDB: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	s := sessions[0]
	if s.Session.ID != "opencode:ses_abc" {
		t.Errorf("ID = %q, want %q",
			s.Session.ID, "opencode:ses_abc")
	}
	if s.Session.Agent != AgentOpenCode {
		t.Errorf("Agent = %q, want %q",
			s.Session.Agent, AgentOpenCode)
	}
	if s.Session.Machine != "testmachine" {
		t.Errorf("Machine = %q, want %q",
			s.Session.Machine, "testmachine")
	}
	if s.Session.Project != "myapp" {
		t.Errorf("Project = %q, want %q",
			s.Session.Project, "myapp")
	}
	if s.Session.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2",
			s.Session.MessageCount)
	}
	if s.Session.FirstMessage != "Hello, help me with Go" {
		t.Errorf("FirstMessage = %q, want %q",
			s.Session.FirstMessage,
			"Hello, help me with Go")
	}

	wantPath := dbPath + "#ses_abc"
	if s.Session.File.Path != wantPath {
		t.Errorf("File.Path = %q, want %q",
			s.Session.File.Path, wantPath)
	}

	// Mtime = time_updated * 1_000_000
	wantMtime := int64(1700000060000) * 1_000_000
	if s.Session.File.Mtime != wantMtime {
		t.Errorf("File.Mtime = %d, want %d",
			s.Session.File.Mtime, wantMtime)
	}

	if len(s.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(s.Messages))
	}
	if s.Messages[0].Role != RoleUser {
		t.Errorf("msg[0].Role = %q, want user",
			s.Messages[0].Role)
	}
	if s.Messages[1].Role != RoleAssistant {
		t.Errorf("msg[1].Role = %q, want assistant",
			s.Messages[1].Role)
	}
	if s.Messages[1].Content != "Sure, I can help with Go." {
		t.Errorf("msg[1].Content = %q",
			s.Messages[1].Content)
	}
}

func TestParseOpenCodeDB_ToolParts(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	stmts := []string{
		`INSERT INTO project (id, worktree)
		 VALUES ('prj_1', '/tmp/proj')`,
		`INSERT INTO session
		 (id, project_id, time_created, time_updated)
		 VALUES ('ses_tools', 'prj_1',
		         1700000000000, 1700000030000)`,
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_u', 'ses_tools', 1700000000000,
		         1700000000000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_u', 'msg_u', 'ses_tools',
		         1700000000000, 1700000000000,
		         '{"type":"text","text":"read my file"}')`,
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_a', 'ses_tools', 1700000010000,
		         1700000012000,
		         '{"role":"assistant"}')`,
		// reasoning part
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_r', 'msg_a', 'ses_tools',
		         1700000010000, 1700000010000,
		         '{"type":"reasoning","text":"Let me think about this..."}')`,
		// tool part
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_t', 'msg_a', 'ses_tools',
		         1700000011000, 1700000011000,
		         '{"type":"tool","tool":"read","callID":"call_1","state":{"input":{"file_path":"main.go"}}}')`,
		// text part
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_txt', 'msg_a', 'ses_tools',
		         1700000012000, 1700000012000,
		         '{"type":"text","text":"Here is the file content."}')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()

	sessions, err := ParseOpenCodeDB(dbPath, "m")
	if err != nil {
		t.Fatalf("ParseOpenCodeDB: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	msgs := sessions[0].Messages
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	ast := msgs[1]
	if !ast.HasThinking {
		t.Error("expected HasThinking=true")
	}
	if !ast.HasToolUse {
		t.Error("expected HasToolUse=true")
	}
	if len(ast.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1",
			len(ast.ToolCalls))
	}

	tc := ast.ToolCalls[0]
	if tc.ToolName != "read" {
		t.Errorf("ToolName = %q, want %q",
			tc.ToolName, "read")
	}
	if tc.Category != "Read" {
		t.Errorf("Category = %q, want %q",
			tc.Category, "Read")
	}
	if tc.ToolUseID != "call_1" {
		t.Errorf("ToolUseID = %q, want %q",
			tc.ToolUseID, "call_1")
	}
	if tc.InputJSON == "" {
		t.Error("expected non-empty InputJSON")
	}
}

func TestParseOpenCodeDB_EmptySession(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	stmts := []string{
		`INSERT INTO project (id, worktree)
		 VALUES ('prj_1', '/tmp/proj')`,
		`INSERT INTO session
		 (id, project_id, time_created, time_updated)
		 VALUES ('ses_empty', 'prj_1',
		         1700000000000, 1700000000000)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()

	sessions, err := ParseOpenCodeDB(dbPath, "m")
	if err != nil {
		t.Fatalf("ParseOpenCodeDB: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("got %d sessions, want 0 (empty skipped)",
			len(sessions))
	}
}

func TestParseOpenCodeDB_NonexistentDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nonexistent.db")

	sessions, err := ParseOpenCodeDB(dbPath, "m")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %d",
			len(sessions))
	}
}

func TestParseOpenCodeDB_ProjectFromWorktree(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)

	// Create a temp dir that looks like a git repo so
	// ExtractProjectFromCwd resolves it.
	repoDir := filepath.Join(t.TempDir(), "my-project")
	if err := os.MkdirAll(
		filepath.Join(repoDir, ".git"), 0o755,
	); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	stmts := []string{
		`INSERT INTO project (id, worktree)
		 VALUES ('prj_git', '` + repoDir + `')`,
		`INSERT INTO session
		 (id, project_id, time_created, time_updated)
		 VALUES ('ses_git', 'prj_git',
		         1700000000000, 1700000010000)`,
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_1', 'ses_git', 1700000000000,
		         1700000000000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_1', 'msg_1', 'ses_git',
		         1700000000000, 1700000000000,
		         '{"type":"text","text":"hello"}')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()

	sessions, err := ParseOpenCodeDB(dbPath, "m")
	if err != nil {
		t.Fatalf("ParseOpenCodeDB: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	if sessions[0].Session.Project != "my_project" {
		t.Errorf("Project = %q, want %q",
			sessions[0].Session.Project, "my_project")
	}
}

func TestParseOpenCodeSession_SingleSession(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	seedStandardSession(t, dbPath)

	sess, msgs, err := ParseOpenCodeSession(
		dbPath, "ses_abc", "testmachine",
	)
	if err != nil {
		t.Fatalf("ParseOpenCodeSession: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}

	if sess.ID != "opencode:ses_abc" {
		t.Errorf("ID = %q, want %q",
			sess.ID, "opencode:ses_abc")
	}
	if len(msgs) != 2 {
		t.Errorf("got %d messages, want 2", len(msgs))
	}
}

func TestParseOpenCodeDB_OrdinalContinuity(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// Insert messages with mixed roles including "system"
	// (which should be skipped) and an empty-content user
	// message (also skipped). Ordinals of the remaining
	// messages must be contiguous 0,1,2.
	stmts := []string{
		`INSERT INTO project (id, worktree)
		 VALUES ('prj_1', '/tmp/proj')`,
		`INSERT INTO session
		 (id, project_id, time_created, time_updated)
		 VALUES ('ses_ord', 'prj_1',
		         1700000000000, 1700000050000)`,
		// msg 0: user (kept, ordinal 0)
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_1', 'ses_ord', 1700000000000,
		         1700000000000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_1', 'msg_1', 'ses_ord',
		         1700000000000, 1700000000000,
		         '{"type":"text","text":"first"}')`,
		// msg 1: system (skipped role)
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_2', 'ses_ord', 1700000010000,
		         1700000010000, '{"role":"system"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_2', 'msg_2', 'ses_ord',
		         1700000010000, 1700000010000,
		         '{"type":"text","text":"system msg"}')`,
		// msg 2: user with empty content (skipped)
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_3', 'ses_ord', 1700000020000,
		         1700000020000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_3', 'msg_3', 'ses_ord',
		         1700000020000, 1700000020000,
		         '{"type":"text","text":""}')`,
		// msg 3: assistant (kept, ordinal 1)
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_4', 'ses_ord', 1700000030000,
		         1700000030000,
		         '{"role":"assistant"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_4', 'msg_4', 'ses_ord',
		         1700000030000, 1700000030000,
		         '{"type":"text","text":"response"}')`,
		// msg 4: user (kept, ordinal 2)
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_5', 'ses_ord', 1700000040000,
		         1700000040000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_5', 'msg_5', 'ses_ord',
		         1700000040000, 1700000040000,
		         '{"type":"text","text":"follow up"}')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()

	sessions, err := ParseOpenCodeDB(dbPath, "m")
	if err != nil {
		t.Fatalf("ParseOpenCodeDB: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}

	msgs := sessions[0].Messages
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}

	for i, m := range msgs {
		if m.Ordinal != i {
			t.Errorf("msg[%d].Ordinal = %d, want %d",
				i, m.Ordinal, i)
		}
	}

	if msgs[0].Content != "first" {
		t.Errorf("msgs[0].Content = %q", msgs[0].Content)
	}
	if msgs[1].Content != "response" {
		t.Errorf("msgs[1].Content = %q", msgs[1].Content)
	}
	if msgs[2].Content != "follow up" {
		t.Errorf("msgs[2].Content = %q", msgs[2].Content)
	}
}

func TestParseOpenCodeDB_ParentSession(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	stmts := []string{
		`INSERT INTO project (id, worktree)
		 VALUES ('prj_1', '/tmp/proj')`,
		`INSERT INTO session
		 (id, project_id, time_created, time_updated)
		 VALUES ('ses_parent', 'prj_1',
		         1700000000000, 1700000010000)`,
		`INSERT INTO session
		 (id, project_id, parent_id,
		  time_created, time_updated)
		 VALUES ('ses_child', 'prj_1', 'ses_parent',
		         1700000020000, 1700000030000)`,
		// Add messages to both so they aren't skipped
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_p', 'ses_parent', 1700000000000,
		         1700000000000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_p', 'msg_p', 'ses_parent',
		         1700000000000, 1700000000000,
		         '{"type":"text","text":"parent msg"}')`,
		`INSERT INTO message
		 (id, session_id, time_created, time_updated, data)
		 VALUES ('msg_c', 'ses_child', 1700000020000,
		         1700000020000, '{"role":"user"}')`,
		`INSERT INTO part
		 (id, message_id, session_id,
		  time_created, time_updated, data)
		 VALUES ('prt_c', 'msg_c', 'ses_child',
		         1700000020000, 1700000020000,
		         '{"type":"text","text":"child msg"}')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()

	sessions, err := ParseOpenCodeDB(dbPath, "m")
	if err != nil {
		t.Fatalf("ParseOpenCodeDB: %v", err)
	}

	var child *OpenCodeSession
	for i := range sessions {
		if sessions[i].Session.ID == "opencode:ses_child" {
			child = &sessions[i]
		}
	}
	if child == nil {
		t.Fatal("child session not found")
	}
	if child.Session.ParentSessionID != "opencode:ses_parent" {
		t.Errorf("ParentSessionID = %q, want %q",
			child.Session.ParentSessionID,
			"opencode:ses_parent")
	}
}

func TestListOpenCodeSessionMeta(t *testing.T) {
	dbPath := createTestOpenCodeDB(t)
	seedStandardSession(t, dbPath)

	metas, err := ListOpenCodeSessionMeta(dbPath)
	if err != nil {
		t.Fatalf("ListOpenCodeSessionMeta: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("got %d metas, want 1", len(metas))
	}

	m := metas[0]
	if m.SessionID != "ses_abc" {
		t.Errorf("SessionID = %q, want %q",
			m.SessionID, "ses_abc")
	}
	wantPath := dbPath + "#ses_abc"
	if m.VirtualPath != wantPath {
		t.Errorf("VirtualPath = %q, want %q",
			m.VirtualPath, wantPath)
	}
	// time_updated = 1700000060000 ms → nanos
	wantMtime := int64(1700000060000) * 1_000_000
	if m.FileMtime != wantMtime {
		t.Errorf("FileMtime = %d, want %d",
			m.FileMtime, wantMtime)
	}
}

func TestListOpenCodeSessionMeta_NonexistentDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nope.db")
	metas, err := ListOpenCodeSessionMeta(dbPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("got %d metas, want 0", len(metas))
	}
}

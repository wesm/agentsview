package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/dbtest"
	"github.com/wesm/agentsview/internal/sync"
	"github.com/wesm/agentsview/internal/testjsonl"
)

type testEnv struct {
	claudeDir   string
	codexDir    string
	geminiDir   string
	opencodeDir string
	db          *db.DB
	engine      *sync.Engine
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := &testEnv{
		claudeDir:   t.TempDir(),
		codexDir:    t.TempDir(),
		geminiDir:   t.TempDir(),
		opencodeDir: t.TempDir(),
		db:          dbtest.OpenTestDB(t),
	}

	env.engine = sync.NewEngine(
		env.db, env.claudeDir, env.codexDir, "",
		env.geminiDir, env.opencodeDir, "local",
	)
	return env
}

// writeSession creates a JSONL session file under baseDir at
// the given relative path, creating parent directories as
// needed. Returns the full file path.
func (e *testEnv) writeSession(
	t *testing.T, baseDir, relPath, content string,
) string {
	t.Helper()
	path := filepath.Join(baseDir, relPath)
	dbtest.WriteTestFile(t, path, []byte(content))
	return path
}

// writeClaudeSession creates a JSONL session file under the
// Claude projects directory.
func (e *testEnv) writeClaudeSession(
	t *testing.T, projName, filename, content string,
) string {
	t.Helper()
	return e.writeSession(
		t, e.claudeDir,
		filepath.Join(projName, filename), content,
	)
}

// writeCodexSession creates a JSONL session file under the
// Codex date-based directory.
func (e *testEnv) writeCodexSession(
	t *testing.T, dayPath, filename, content string,
) string {
	t.Helper()
	return e.writeSession(
		t, e.codexDir,
		filepath.Join(dayPath, filename), content,
	)
}

// writeGeminiSession creates a JSON session file under the
// Gemini directory at the given relative path.
func (e *testEnv) writeGeminiSession(
	t *testing.T, relPath, content string,
) string {
	t.Helper()
	return e.writeSession(t, e.geminiDir, relPath, content)
}

func TestSyncEngineIntegration(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsEarly, "Hello", "/Users/alice/code/my-app").
		AddClaudeAssistant(tsEarlyS5, "Hi there!").
		String()

	env.writeClaudeSession(
		t, "-Users-alice-code-my-app",
		"test-session.jsonl", content,
	)

	// First sync should parse
	stats := runSyncAndAssert(t, env.engine, 1, 0)
	if stats.TotalSessions != 1 {
		t.Errorf("total = %d, want 1", stats.TotalSessions)
	}

	// Verify session was stored
	assertSessionState(t, env.db, "test-session", func(sess *db.Session) {
		if sess.Project != "my_app" {
			t.Errorf("project = %q, want %q",
				sess.Project, "my_app")
		}
		if sess.MessageCount != 2 {
			t.Errorf("message_count = %d, want 2",
				sess.MessageCount)
		}
	})

	// Verify messages
	assertMessageRoles(
		t, env.db, "test-session", "user", "assistant",
	)

	// Second sync should skip (unchanged files)
	runSyncAndAssert(t, env.engine, 0, 1)

	// FindSourceFile
	src := env.engine.FindSourceFile("test-session")
	if src == "" {
		t.Error("FindSourceFile returned empty")
	}
}

func TestSyncEngineWorktreesShareProject(t *testing.T) {
	env := setupTestEnv(t)

	root := t.TempDir()
	mainRepo := filepath.Join(root, "agentsview")
	worktree := filepath.Join(root, "agentsview-worktree-tool-call-arguments")
	worktreeGitDir := filepath.Join(mainRepo, ".git", "worktrees", "feature")

	dbtest.WriteTestFile(t, filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+worktreeGitDir+"\n"))
	dbtest.WriteTestFile(t, filepath.Join(worktreeGitDir, "commondir"),
		[]byte("../..\n"))

	// Create a standard main repository marker.
	if err := os.MkdirAll(filepath.Join(mainRepo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir main .git: %v", err)
	}

	mainContent := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsEarly, "Main repo", mainRepo).
		AddClaudeAssistant(tsEarlyS5, "ok").
		String()
	worktreeContent := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsEarly, "Worktree", worktree).
		AddClaudeAssistant(tsEarlyS5, "ok").
		String()

	env.writeClaudeSession(
		t, "-Users-me-code-agentsview",
		"main-repo.jsonl", mainContent,
	)
	env.writeClaudeSession(
		t, "-Users-me-code-agentsview-worktree-tool-call-arguments",
		"worktree-repo.jsonl", worktreeContent,
	)

	runSyncAndAssert(t, env.engine, 2, 0)

	assertSessionState(t, env.db, "main-repo", func(sess *db.Session) {
		if sess.Project != "agentsview" {
			t.Errorf("main session project = %q, want agentsview", sess.Project)
		}
	})
	assertSessionState(t, env.db, "worktree-repo", func(sess *db.Session) {
		if sess.Project != "agentsview" {
			t.Errorf("worktree session project = %q, want agentsview", sess.Project)
		}
	})

	projects, err := env.db.GetProjects(context.Background())
	if err != nil {
		t.Fatalf("GetProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}
	if projects[0].Name != "agentsview" {
		t.Fatalf("project name = %q, want %q", projects[0].Name, "agentsview")
	}
	if projects[0].SessionCount != 2 {
		t.Fatalf("session_count = %d, want 2", projects[0].SessionCount)
	}
}

func TestSyncEngineWorktreeProjectWhenPathMissing(t *testing.T) {
	env := setupTestEnv(t)

	mainContent := `{"type":"user","timestamp":"2024-01-01T10:00:00Z","cwd":"/Users/wesm/code/agentsview","gitBranch":"main","message":{"content":"hello"}}` + "\n" +
		`{"type":"assistant","timestamp":"2024-01-01T10:00:05Z","message":{"content":"ok"}}` + "\n"

	worktreeContent := `{"type":"user","timestamp":"2024-01-01T10:00:00Z","cwd":"/Users/wesm/code/agentsview-worktree-tool-call-arguments","gitBranch":"worktree-tool-call-arguments","message":{"content":"hello"}}` + "\n" +
		`{"type":"assistant","timestamp":"2024-01-01T10:00:05Z","message":{"content":"ok"}}` + "\n"

	env.writeClaudeSession(
		t, "-Users-me-code-agentsview",
		"offline-main.jsonl", mainContent,
	)
	env.writeClaudeSession(
		t, "-Users-me-code-agentsview-worktree-tool-call-arguments",
		"offline-worktree.jsonl", worktreeContent,
	)

	runSyncAndAssert(t, env.engine, 2, 0)

	assertSessionState(t, env.db, "offline-main", func(sess *db.Session) {
		if sess.Project != "agentsview" {
			t.Errorf("main session project = %q, want agentsview", sess.Project)
		}
	})
	assertSessionState(t, env.db, "offline-worktree", func(sess *db.Session) {
		if sess.Project != "agentsview" {
			t.Errorf("worktree session project = %q, want agentsview", sess.Project)
		}
	})
}

func TestSyncEngineCodex(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddCodexMeta(tsEarly, "test-uuid", "/home/user/code/api", "user").
		AddCodexMessage(tsEarlyS1, "user", "Add tests").
		AddCodexMessage(tsEarlyS5, "assistant", "Adding test coverage.").
		String()

	env.writeCodexSession(
		t, filepath.Join("2024", "01", "15"),
		"rollout-20240115-test-uuid.jsonl", content,
	)

	stats := env.engine.SyncAll(nil)
	if stats.TotalSessions != 1 {
		t.Errorf("total = %d, want 1", stats.TotalSessions)
	}

	assertSessionState(t, env.db, "codex:test-uuid", func(sess *db.Session) {
		if sess.Agent != "codex" {
			t.Errorf("agent = %q", sess.Agent)
		}
		if sess.Project != "api" {
			t.Errorf("project = %q", sess.Project)
		}
	})
}

func TestSyncEngineProgress(t *testing.T) {
	env := setupTestEnv(t)

	msg := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "msg").
		String()

	for _, name := range []string{"a", "b", "c"} {
		env.writeClaudeSession(
			t, "test-proj", name+".jsonl", msg,
		)
	}

	var progressCalls int
	env.engine.SyncAll(func(p sync.Progress) {
		progressCalls++
	})

	if progressCalls == 0 {
		t.Error("expected progress callbacks")
	}
}

func TestSyncEngineHashSkip(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "msg1").
		String()

	path := env.writeClaudeSession(
		t, "test-proj", "hash-test.jsonl", content,
	)

	// First sync
	runSyncAndAssert(t, env.engine, 1, 0)

	// Verify file metadata was stored
	size, mtime, ok := env.db.GetSessionFileInfo("hash-test")
	if !ok {
		t.Fatal("file info not stored")
	}
	if mtime == 0 {
		t.Fatal("mtime not stored")
	}
	if size == 0 {
		t.Fatal("size not stored")
	}

	// Second sync — unchanged content → skipped
	runSyncAndAssert(t, env.engine, 0, 1)

	// Overwrite with different content (changes mtime).
	different := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "msg2").
		String()
	os.WriteFile(path, []byte(different), 0o644)

	// Third sync — mtime changed → re-synced
	runSyncAndAssert(t, env.engine, 1, 0)
}

func TestSyncEngineSkipCache(t *testing.T) {
	env := setupTestEnv(t)

	// Write malformed content that produces 0 valid messages
	path := env.writeClaudeSession(
		t, "test-proj", "skip-test.jsonl",
		"not json at all\x00\x01",
	)

	// First sync — file parsed (empty session stored)
	stats := env.engine.SyncAll(nil)
	if stats.TotalSessions != 1 {
		t.Fatalf("total = %d, want 1", stats.TotalSessions)
	}

	// Second sync — unchanged mtime, should be skipped
	runSyncAndAssert(t, env.engine, 0, 1)

	// Touch file (change mtime) but keep same content
	time.Sleep(10 * time.Millisecond)
	os.Chtimes(path, time.Now(), time.Now())

	// Third sync — mtime changed → re-synced (harmless)
	runSyncAndAssert(t, env.engine, 1, 0)
}

func TestSyncEngineFileAppend(t *testing.T) {
	env := setupTestEnv(t)

	initial := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "first").
		String()

	path := env.writeClaudeSession(
		t, "test-proj", "append-test.jsonl", initial,
	)

	// First sync
	runSyncAndAssert(t, env.engine, 1, 0)

	assertSessionState(t, env.db, "append-test", func(sess *db.Session) {
		if sess.MessageCount != 1 {
			t.Fatalf("initial message_count = %d, want 1",
				sess.MessageCount)
		}
	})

	// Append a new message (changes size and hash)
	appended := initial + testjsonl.NewSessionBuilder().
		AddClaudeAssistant(tsZeroS5, "reply").
		String()

	os.WriteFile(path, []byte(appended), 0o644)

	// Re-sync — different size → re-synced
	runSyncAndAssert(t, env.engine, 1, 0)

	assertSessionState(t, env.db, "append-test", func(sess *db.Session) {
		if sess.MessageCount != 2 {
			t.Errorf("updated message_count = %d, want 2",
				sess.MessageCount)
		}
	})
}

// TestSyncSingleSessionReplacesContent verifies that an
// explicit SyncSingleSession replaces existing message
// content (same ordinals, different text).
func TestSyncSingleSessionReplacesContent(
	t *testing.T,
) {
	env := setupTestEnv(t)

	original := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "original question").
		AddClaudeAssistant(tsZeroS5, "original answer").
		String()

	path := env.writeClaudeSession(
		t, "test-proj", "replace-test.jsonl", original,
	)

	env.engine.SyncAll(nil)
	assertMessageContent(
		t, env.db, "replace-test",
		"original question", "original answer",
	)

	// Rewrite the file with different content but same
	// number of messages (same ordinals 0 and 1).
	updated := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "updated question").
		AddClaudeAssistant(tsZeroS5, "updated answer").
		String()
	os.WriteFile(path, []byte(updated), 0o644)

	// SyncSingleSession should fully replace messages.
	if err := env.engine.SyncSingleSession(
		"replace-test",
	); err != nil {
		t.Fatalf("SyncSingleSession: %v", err)
	}

	assertMessageContent(
		t, env.db, "replace-test",
		"updated question", "updated answer",
	)
}

func TestSyncSingleSessionHash(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "hello").
		AddClaudeAssistant(tsZeroS5, "hi").
		String()

	env.writeClaudeSession(
		t, "test-proj", "single-hash.jsonl", content,
	)

	env.engine.SyncAll(nil)
	env.assertResyncRoundTrip(t, "single-hash")
}

func TestSyncSingleSessionHashCodex(t *testing.T) {
	env := setupTestEnv(t)

	uuid := "a1b2c3d4-1234-5678-9abc-def012345678"
	content := testjsonl.NewSessionBuilder().
		AddCodexMeta(tsEarly, uuid, "/home/user/code/api", "user").
		AddCodexMessage(tsEarlyS1, "user", "Add tests").
		AddCodexMessage(tsEarlyS5, "assistant", "Adding test coverage.").
		String()

	env.writeCodexSession(
		t, filepath.Join("2024", "01", "15"),
		"rollout-20240115-"+uuid+".jsonl", content,
	)

	sessionID := "codex:" + uuid

	env.engine.SyncAll(nil)
	assertSessionState(t, env.db, sessionID, nil)
	env.assertResyncRoundTrip(t, sessionID)
}

func TestSyncSingleSessionCodexExecBypassesCache(
	t *testing.T,
) {
	env := setupTestEnv(t)

	uuid := "e5f6a7b8-5678-9012-cdef-123456789012"
	// Exec-originated session: SyncAll skips these, but
	// SyncSingleSession should still find them.
	content := testjsonl.NewSessionBuilder().
		AddCodexMeta(
			tsEarly, uuid,
			"/home/user/code/api", "codex_exec",
		).
		AddCodexMessage(tsEarlyS1, "user", "run ls").
		AddCodexMessage(tsEarlyS5, "assistant", "done").
		String()

	env.writeCodexSession(
		t, filepath.Join("2024", "01", "15"),
		"rollout-20240115-"+uuid+".jsonl", content,
	)

	// SyncAll skips exec-originated sessions (nil result).
	env.engine.SyncAll(nil)
	sess, _ := env.db.GetSession(
		context.Background(), "codex:"+uuid,
	)
	if sess != nil {
		t.Fatal("exec session should not appear after SyncAll")
	}

	// SyncSingleSession should bypass the skip cache and
	// parse with includeExec=true.
	err := env.engine.SyncSingleSession("codex:" + uuid)
	if err != nil {
		t.Fatalf("SyncSingleSession: %v", err)
	}

	assertSessionState(
		t, env.db, "codex:"+uuid,
		func(sess *db.Session) {
			if sess.Agent != "codex" {
				t.Errorf("agent = %q, want codex",
					sess.Agent)
			}
		},
	)
}

func TestSyncEngineTombstoneClearOnMtimeChange(t *testing.T) {
	env := setupTestEnv(t)

	// Write something that produces 0 messages but parses OK
	path := env.writeClaudeSession(
		t, "test-proj", "tombstone-clear.jsonl", "garbage\n",
	)

	// First sync
	env.engine.SyncAll(nil)

	// Replace with valid content
	valid := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "hello").
		AddClaudeAssistant(tsZeroS5, "hi").
		String()

	os.WriteFile(path, []byte(valid), 0o644)

	// Re-sync — content changed (different size) → re-synced
	runSyncAndAssert(t, env.engine, 1, 0)

	assertSessionState(t, env.db, "tombstone-clear", func(sess *db.Session) {
		if sess.MessageCount != 2 {
			t.Errorf("message_count = %d, want 2",
				sess.MessageCount)
		}
	})
}

func TestSyncSingleSessionProjectFallback(t *testing.T) {
	env := setupTestEnv(t)

	// 1. Create a session in a directory "default-proj"
	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "hello").
		String()

	env.writeClaudeSession(
		t, "default-proj", "fallback-test.jsonl", content,
	)

	// 2. Initial sync - should get "default-proj"
	env.engine.SyncAll(nil)

	assertSessionState(t, env.db, "fallback-test", func(sess *db.Session) {
		if sess.Project != "default_proj" {
			t.Fatalf("initial project = %q, want %q", sess.Project, "default_proj")
		}
	})

	// 3. Manually update project to "custom_proj"
	// This simulates a user override we want to preserve
	env.updateSessionProject(t, "fallback-test", "custom_proj")

	assertSessionState(t, env.db, "fallback-test", func(sess *db.Session) {
		if sess.Project != "custom_proj" {
			t.Fatalf("manual update failed, project = %q", sess.Project)
		}
	})

	// 4. SyncSingleSession should NOT revert to "default_proj"
	err := env.engine.SyncSingleSession("fallback-test")
	if err != nil {
		t.Fatalf("SyncSingleSession: %v", err)
	}

	assertSessionState(t, env.db, "fallback-test", func(sess *db.Session) {
		if sess.Project != "custom_proj" {
			t.Errorf("regression: project reverted to %q, want %q", sess.Project, "custom_proj")
		}
	})

	// Case A: Empty project -> should fall back to directory
	env.updateSessionProject(t, "fallback-test", "")

	err = env.engine.SyncSingleSession("fallback-test")
	if err != nil {
		t.Fatalf("SyncSingleSession (empty): %v", err)
	}

	assertSessionState(t, env.db, "fallback-test", func(sess *db.Session) {
		if sess.Project != "default_proj" {
			t.Errorf("empty project fallback: got %q, want %q", sess.Project, "default_proj")
		}
	})

	// Case B: Bad project -> should fall back to directory
	env.updateSessionProject(t, "fallback-test", "_Users_alice_bad")

	err = env.engine.SyncSingleSession("fallback-test")
	if err != nil {
		t.Fatalf("SyncSingleSession (bad): %v", err)
	}

	assertSessionState(t, env.db, "fallback-test", func(sess *db.Session) {
		if sess.Project != "default_proj" {
			t.Errorf("bad project fallback: got %q, want %q", sess.Project, "default_proj")
		}
	})
}

func TestSyncEngineNoTrailingNewline(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsEarly, "Hello").
		StringNoTrailingNewline()

	env.writeClaudeSession(
		t, "test-proj", "no-newline.jsonl", content,
	)

	// Sync should succeed
	runSyncAndAssert(t, env.engine, 1, 0)

	assertSessionState(t, env.db, "no-newline", func(sess *db.Session) {
		if sess.MessageCount != 1 {
			t.Errorf("message_count = %d, want 1", sess.MessageCount)
		}
	})
}

func TestSyncPathsClaude(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "Hello").
		String()

	path := env.writeClaudeSession(
		t, "test-proj", "paths-test.jsonl", content,
	)

	// Initial full sync
	runSyncAndAssert(t, env.engine, 1, 0)

	assertSessionState(
		t, env.db, "paths-test",
		func(sess *db.Session) {
			if sess.MessageCount != 1 {
				t.Fatalf(
					"initial message_count = %d, want 1",
					sess.MessageCount,
				)
			}
		},
	)

	// Append a message (changes size and hash)
	appended := content + testjsonl.NewSessionBuilder().
		AddClaudeAssistant(tsZeroS5, "reply").
		String()
	os.WriteFile(path, []byte(appended), 0o644)

	// SyncPaths with just the changed file
	env.engine.SyncPaths([]string{path})

	assertSessionState(
		t, env.db, "paths-test",
		func(sess *db.Session) {
			if sess.MessageCount != 2 {
				t.Errorf(
					"message_count = %d, want 2",
					sess.MessageCount,
				)
			}
		},
	)
}

func TestSyncPathsOnlyProcessesChanged(t *testing.T) {
	env := setupTestEnv(t)

	content1 := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "msg1").
		String()
	content2 := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "msg2").
		String()

	path1 := env.writeClaudeSession(
		t, "proj", "session-1.jsonl", content1,
	)
	env.writeClaudeSession(
		t, "proj", "session-2.jsonl", content2,
	)

	// Initial full sync
	runSyncAndAssert(t, env.engine, 2, 0)

	// Only modify session-1
	appended := content1 + testjsonl.NewSessionBuilder().
		AddClaudeAssistant(tsZeroS5, "reply").
		String()
	os.WriteFile(path1, []byte(appended), 0o644)

	// SyncPaths with just session-1
	env.engine.SyncPaths([]string{path1})

	// session-1 should have 2 messages
	assertSessionState(
		t, env.db, "session-1",
		func(sess *db.Session) {
			if sess.MessageCount != 2 {
				t.Errorf(
					"session-1 message_count = %d, want 2",
					sess.MessageCount,
				)
			}
		},
	)
	// session-2 should still have 1 message (untouched)
	assertSessionState(
		t, env.db, "session-2",
		func(sess *db.Session) {
			if sess.MessageCount != 1 {
				t.Errorf(
					"session-2 message_count = %d, want 1",
					sess.MessageCount,
				)
			}
		},
	)
}

func TestSyncPathsIgnoresNonSessionFiles(t *testing.T) {
	env := setupTestEnv(t)

	// SyncPaths with non-session paths: no panic, no error
	env.engine.SyncPaths([]string{
		filepath.Join(env.claudeDir, "some-dir"),
		filepath.Join(env.claudeDir, "proj", "README.md"),
		"/tmp/random-file.txt",
	})
}

func TestSyncPathsCodex(t *testing.T) {
	env := setupTestEnv(t)

	uuid := "c3d4e5f6-3456-7890-abcd-ef1234567890"
	content := testjsonl.NewSessionBuilder().
		AddCodexMeta(
			tsEarly, uuid,
			"/home/user/code/api", "user",
		).
		AddCodexMessage(tsEarlyS1, "user", "Add tests").
		String()

	path := env.writeCodexSession(
		t, filepath.Join("2024", "01", "15"),
		"rollout-20240115-"+uuid+".jsonl", content,
	)

	// SyncPaths should process this Codex file
	env.engine.SyncPaths([]string{path})

	assertSessionState(
		t, env.db, "codex:"+uuid,
		func(sess *db.Session) {
			if sess.Agent != "codex" {
				t.Errorf("agent = %q, want codex",
					sess.Agent)
			}
		},
	)
}

func TestSyncPathsIgnoresAgentFiles(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "Hello").
		String()

	// Create an agent-* file (should be ignored)
	path := env.writeClaudeSession(
		t, "proj", "agent-abc.jsonl", content,
	)

	// SyncPaths should ignore agent-* files
	env.engine.SyncPaths([]string{path})

	// No session should exist for agent-abc
	sess, _ := env.db.GetSession(
		context.Background(), "agent-abc",
	)
	if sess != nil {
		t.Error("agent-* file should be ignored")
	}
}

func TestSyncEngineCodexNoTrailingNewline(t *testing.T) {
	env := setupTestEnv(t)

	uuid := "b2c3d4e5-2345-6789-0abc-def123456789"
	content := testjsonl.NewSessionBuilder().
		AddCodexMeta(tsEarly, uuid, "/home/user/code/api", "user").
		AddCodexMessage(tsEarlyS1, "user", "Hello").
		StringNoTrailingNewline()

	env.writeCodexSession(
		t, filepath.Join("2024", "01", "15"),
		"rollout-20240115-"+uuid+".jsonl", content,
	)

	// Sync should succeed
	runSyncAndAssert(t, env.engine, 1, 0)

	assertSessionState(t, env.db, "codex:"+uuid, func(sess *db.Session) {
		if sess.MessageCount != 1 {
			t.Errorf("message_count = %d, want 1", sess.MessageCount)
		}
	})
}

func TestSyncPathsTrailingSlashDirs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Dirs with trailing slashes should still work after
	// filepath.Clean normalisation in isUnder.
	claudeDir := t.TempDir() + "/"
	codexDir := t.TempDir() + "/"
	database := dbtest.OpenTestDB(t)
	engine := sync.NewEngine(
		database, claudeDir, codexDir, "", "", "", "local",
	)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "Hello").
		String()

	claudePath := filepath.Join(
		claudeDir, "proj", "trailing.jsonl",
	)
	dbtest.WriteTestFile(t, claudePath, []byte(content))

	engine.SyncPaths([]string{claudePath})

	assertSessionState(
		t, database, "trailing",
		func(sess *db.Session) {
			if sess.MessageCount != 1 {
				t.Errorf(
					"message_count = %d, want 1",
					sess.MessageCount,
				)
			}
		},
	)
}

func TestSyncPathsGemini(t *testing.T) {
	env := setupTestEnv(t)

	sessionID := "gem-test-uuid"
	hash := "abcdef1234567890"
	content := testjsonl.GeminiSessionJSON(
		sessionID, hash, tsEarly, tsEarlyS5,
		[]map[string]any{
			testjsonl.GeminiUserMsg(
				"m1", tsEarly, "Hello Gemini",
			),
			testjsonl.GeminiAssistantMsg(
				"m2", tsEarlyS5, "Hi there!", nil,
			),
		},
	)

	path := env.writeGeminiSession(
		t,
		filepath.Join(
			"tmp", hash, "chats",
			"session-001.json",
		),
		content,
	)

	env.engine.SyncPaths([]string{path})

	assertSessionState(
		t, env.db, "gemini:"+sessionID,
		func(sess *db.Session) {
			if sess.Agent != "gemini" {
				t.Errorf("agent = %q, want gemini",
					sess.Agent)
			}
			if sess.MessageCount != 2 {
				t.Errorf(
					"message_count = %d, want 2",
					sess.MessageCount,
				)
			}
		},
	)
}

func TestSyncPathsCodexRejectsFlat(t *testing.T) {
	env := setupTestEnv(t)

	uuid := "d4e5f6a7-4567-8901-bcde-f12345678901"
	content := testjsonl.NewSessionBuilder().
		AddCodexMeta(
			tsEarly, uuid,
			"/home/user/code/api", "user",
		).
		AddCodexMessage(tsEarlyS1, "user", "Add tests").
		String()

	// Write directly under codexDir (no year/month/day)
	path := env.writeSession(
		t, env.codexDir,
		"rollout-flat-"+uuid+".jsonl", content,
	)

	env.engine.SyncPaths([]string{path})

	sess, _ := env.db.GetSession(
		context.Background(), "codex:"+uuid,
	)
	if sess != nil {
		t.Error(
			"flat Codex file should be ignored " +
				"(no year/month/day structure)",
		)
	}
}

func TestSyncPathsGeminiRejectsWrongStructure(t *testing.T) {
	env := setupTestEnv(t)

	sessionID := "gem-wrong-struct"
	content := testjsonl.GeminiSessionJSON(
		sessionID, "somehash", tsEarly, tsEarlyS5,
		[]map[string]any{
			testjsonl.GeminiUserMsg(
				"m1", tsEarly, "Hello",
			),
		},
	)

	// Write session-*.json directly under geminiDir (wrong)
	path1 := env.writeGeminiSession(
		t, "session-wrong.json", content,
	)
	// Write under tmp/<hash> but without /chats/ dir
	path2 := env.writeGeminiSession(
		t,
		filepath.Join("tmp", "abc123", "session-bad.json"),
		content,
	)

	env.engine.SyncPaths([]string{path1, path2})

	sess, _ := env.db.GetSession(
		context.Background(), "gemini:"+sessionID,
	)
	if sess != nil {
		t.Error(
			"Gemini file outside tmp/<hash>/chats " +
				"should be ignored",
		)
	}
}

func TestSyncPathsStatsUpdated(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "Hello").
		String()

	path := env.writeClaudeSession(
		t, "proj", "stats-test.jsonl", content,
	)

	env.engine.SyncPaths([]string{path})

	stats := env.engine.LastSyncStats()
	if stats.Synced != 1 {
		t.Errorf("LastSyncStats.Synced = %d, want 1",
			stats.Synced)
	}
	lastSync := env.engine.LastSync()
	if lastSync.IsZero() {
		t.Error("LastSync should be set after SyncPaths")
	}
}

func TestSyncPathsClaudeParentSessionID(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUserWithSessionID(
			tsZero, "Hello", "parent-uuid",
		).
		AddClaudeAssistant(tsZeroS5, "Hi there!").
		String()

	path := env.writeClaudeSession(
		t, "test-proj", "child-test.jsonl", content,
	)

	env.engine.SyncPaths([]string{path})

	assertSessionState(
		t, env.db, "child-test",
		func(sess *db.Session) {
			if sess.ParentSessionID == nil ||
				*sess.ParentSessionID != "parent-uuid" {
				t.Errorf("parent_session_id = %v, want %q",
					sess.ParentSessionID, "parent-uuid")
			}
		},
	)
}

func TestSyncPathsClaudeNoParentSessionID(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "Hello").
		String()

	path := env.writeClaudeSession(
		t, "test-proj", "no-parent-test.jsonl", content,
	)

	env.engine.SyncPaths([]string{path})

	assertSessionState(
		t, env.db, "no-parent-test",
		func(sess *db.Session) {
			if sess.ParentSessionID != nil {
				t.Errorf("parent_session_id = %v, want nil",
					sess.ParentSessionID)
			}
		},
	)
}

func TestSyncPathsClaudeRejectsNested(t *testing.T) {
	env := setupTestEnv(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "Hello").
		String()

	// Write at proj/subdir/nested.jsonl — should be rejected
	// since Claude expects exactly <project>/<session>.jsonl.
	path := env.writeClaudeSession(
		t, filepath.Join("proj", "subdir"),
		"nested.jsonl", content,
	)

	env.engine.SyncPaths([]string{path})

	sess, _ := env.db.GetSession(
		context.Background(), "nested",
	)
	if sess != nil {
		t.Error(
			"nested Claude path should be rejected " +
				"(only <project>/<session>.jsonl allowed)",
		)
	}
}

// TestSyncEngineOpenCodeBulkSync verifies that SyncAll
// discovers OpenCode sessions and fully replaces messages
// when content changes in place (same ordinals, different
// text/tool data).
func TestSyncEngineOpenCodeBulkSync(t *testing.T) {
	env := setupTestEnv(t)

	oc := createOpenCodeDB(t, env.opencodeDir)
	oc.addProject(t, "proj-1", "/home/user/code/myapp")

	sessionID := "oc-sess-001"
	var timeCreated int64 = 1704067200000 // 2024-01-01T00:00:00Z
	var timeUpdated int64 = 1704067205000 // +5s

	oc.addSession(
		t, sessionID, "proj-1",
		timeCreated, timeUpdated,
	)
	oc.addMessage(
		t, "msg-u1", sessionID, "user", timeCreated,
	)
	oc.addMessage(
		t, "msg-a1", sessionID, "assistant",
		timeCreated+1,
	)
	oc.addTextPart(
		t, "part-u1", sessionID, "msg-u1",
		"original question", timeCreated,
	)
	oc.addTextPart(
		t, "part-a1", sessionID, "msg-a1",
		"original answer", timeCreated+1,
	)

	// First SyncAll should discover and store the session.
	env.engine.SyncAll(nil)

	agentviewID := "opencode:" + sessionID
	assertSessionState(t, env.db, agentviewID,
		func(sess *db.Session) {
			if sess.Agent != "opencode" {
				t.Errorf("agent = %q, want opencode",
					sess.Agent)
			}
			if sess.MessageCount != 2 {
				t.Errorf("message_count = %d, want 2",
					sess.MessageCount)
			}
		},
	)
	assertMessageContent(
		t, env.db, agentviewID,
		"original question", "original answer",
	)

	// Mutate the session in place: replace content but
	// keep the same number of messages (same ordinals).
	// Bump time_updated so the sync engine detects it.
	oc.replaceTextContent(
		t, sessionID,
		"updated question", "updated answer",
		timeCreated,
	)
	oc.updateSessionTime(t, sessionID, timeUpdated+1000)

	// Second SyncAll should fully replace messages.
	env.engine.SyncAll(nil)

	assertMessageContent(
		t, env.db, agentviewID,
		"updated question", "updated answer",
	)

	// Third SyncAll with no changes should be a no-op
	// (time_updated unchanged, so session is skipped).
	env.engine.SyncAll(nil)

	assertMessageContent(
		t, env.db, agentviewID,
		"updated question", "updated answer",
	)
}

// TestSyncEngineOpenCodeToolCallReplace verifies that tool
// call data is fully replaced during OpenCode bulk sync, not
// left stale from a previous sync.
func TestSyncEngineOpenCodeToolCallReplace(t *testing.T) {
	env := setupTestEnv(t)

	oc := createOpenCodeDB(t, env.opencodeDir)
	oc.addProject(t, "proj-1", "/home/user/code/myapp")

	sessionID := "oc-tool-sess"
	var timeCreated int64 = 1704067200000
	var timeUpdated int64 = 1704067205000

	oc.addSession(
		t, sessionID, "proj-1",
		timeCreated, timeUpdated,
	)

	// Assistant message with a tool call.
	oc.addMessage(
		t, "msg-u1", sessionID, "user", timeCreated,
	)
	oc.addMessage(
		t, "msg-a1", sessionID, "assistant",
		timeCreated+1,
	)
	oc.addTextPart(
		t, "part-u1", sessionID, "msg-u1",
		"run ls", timeCreated,
	)
	oc.addToolPart(
		t, "part-tool1", sessionID, "msg-a1",
		"bash", "call-1", timeCreated+1,
	)

	env.engine.SyncAll(nil)

	agentviewID := "opencode:" + sessionID
	assertSessionState(t, env.db, agentviewID, nil)
	assertToolCallCount(t, env.db, agentviewID, 1)

	// Replace: remove tool call, add text instead.
	oc.deleteMessages(t, sessionID)
	oc.deleteParts(t, sessionID)
	oc.addMessage(
		t, "msg-u1-v2", sessionID, "user", timeCreated,
	)
	oc.addMessage(
		t, "msg-a1-v2", sessionID, "assistant",
		timeCreated+1,
	)
	oc.addTextPart(
		t, "part-u1-v2", sessionID, "msg-u1-v2",
		"run ls", timeCreated,
	)
	oc.addTextPart(
		t, "part-a1-v2", sessionID, "msg-a1-v2",
		"here are the files", timeCreated+1,
	)
	oc.updateSessionTime(t, sessionID, timeUpdated+1000)

	env.engine.SyncAll(nil)

	assertMessageContent(
		t, env.db, agentviewID,
		"run ls", "here are the files",
	)
	assertToolCallCount(t, env.db, agentviewID, 0)
}

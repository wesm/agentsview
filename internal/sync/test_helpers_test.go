package sync_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/sync"
)

// Timestamp constants for test data.
const (
	tsZero    = "2024-01-01T00:00:00Z"
	tsZeroS5  = "2024-01-01T00:00:05Z"
	tsEarly   = "2024-01-01T10:00:00Z"
	tsEarlyS1 = "2024-01-01T10:00:01Z"
	tsEarlyS5 = "2024-01-01T10:00:05Z"
)

// --- Assertion Helpers ---

func assertSessionState(t *testing.T, database *db.DB, sessionID string, check func(*db.Session)) {
	t.Helper()
	sess, err := database.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession(%q): %v", sessionID, err)
	}
	if sess == nil {
		t.Fatalf("Session %q not found", sessionID)
	}
	if check != nil {
		check(sess)
	}
}

func runSyncAndAssert(t *testing.T, engine *sync.Engine, wantSynced, wantSkipped int) sync.SyncStats {
	t.Helper()
	stats := engine.SyncAll(nil)
	if stats.Synced != wantSynced {
		t.Fatalf("Synced: got %d, want %d", stats.Synced, wantSynced)
	}
	if stats.Skipped != wantSkipped {
		t.Fatalf("Skipped: got %d, want %d", stats.Skipped, wantSkipped)
	}
	return stats
}

// assertResyncRoundTrip clears file_mtime to force a resync,
// runs SyncSingleSession, and verifies the session is stored
// and a subsequent SyncAll skips.
func (e *testEnv) assertResyncRoundTrip(
	t *testing.T, sessionID string,
) {
	t.Helper()

	// Clear mtime to force resync on next check.
	err := e.db.Update(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"UPDATE sessions SET file_mtime = NULL"+
				" WHERE id = ?",
			sessionID,
		)
		return err
	})
	if err != nil {
		t.Fatalf(
			"clear mtime for %s: %v", sessionID, err,
		)
	}

	if err := e.engine.SyncSingleSession(sessionID); err != nil {
		t.Fatalf("SyncSingleSession: %v", err)
	}

	_, mtime, ok := e.db.GetSessionFileInfo(sessionID)
	if !ok {
		t.Fatal("session file info not found")
	}
	if mtime == 0 {
		t.Error("SyncSingleSession did not store mtime")
	}

	runSyncAndAssert(t, e.engine, 0, 1)
}

// assertMessageRoles verifies that a session's messages have
// the expected roles in order.
func assertMessageRoles(
	t *testing.T, database *db.DB,
	sessionID string, wantRoles ...string,
) {
	t.Helper()
	msgs, err := database.GetAllMessages(
		context.Background(), sessionID,
	)
	if err != nil {
		t.Fatalf("GetAllMessages(%q): %v", sessionID, err)
	}
	if len(msgs) != len(wantRoles) {
		t.Fatalf("got %d messages, want %d",
			len(msgs), len(wantRoles))
	}
	for i, want := range wantRoles {
		if msgs[i].Role != want {
			t.Errorf("msgs[%d].Role = %q, want %q",
				i, msgs[i].Role, want)
		}
	}
}

// assertMessageContent verifies that a session's messages
// have the expected content strings in ordinal order.
func assertMessageContent(
	t *testing.T, database *db.DB,
	sessionID string, wantContent ...string,
) {
	t.Helper()
	msgs, err := database.GetAllMessages(
		context.Background(), sessionID,
	)
	if err != nil {
		t.Fatalf("GetAllMessages(%q): %v",
			sessionID, err)
	}
	if len(msgs) != len(wantContent) {
		t.Fatalf("got %d messages, want %d",
			len(msgs), len(wantContent))
	}
	for i, want := range wantContent {
		if msgs[i].Content != want {
			t.Errorf("msgs[%d].Content = %q, want %q",
				i, msgs[i].Content, want)
		}
	}
}

// assertToolCallCount verifies that the total number of
// tool_calls rows for a session matches the expected count.
func assertToolCallCount(
	t *testing.T, database *db.DB,
	sessionID string, want int,
) {
	t.Helper()
	var got int
	err := database.Reader().QueryRow(
		"SELECT COUNT(*) FROM tool_calls"+
			" WHERE session_id = ?",
		sessionID,
	).Scan(&got)
	if err != nil {
		t.Fatalf("count tool_calls for %q: %v",
			sessionID, err)
	}
	if got != want {
		t.Errorf("tool_calls count for %q = %d, want %d",
			sessionID, got, want)
	}
}

// updateSessionProject fetches the session, updates its
// Project field, and upserts it back. Reduces boilerplate
// for tests that need to override a single field.
func (e *testEnv) updateSessionProject(
	t *testing.T, sessionID, project string,
) {
	t.Helper()
	sess, err := e.db.GetSessionFull(
		context.Background(), sessionID,
	)
	if err != nil {
		t.Fatalf("GetSessionFull: %v", err)
	}
	if sess == nil {
		t.Fatalf("session %q not found", sessionID)
	}
	sess.Project = project
	if err := e.db.UpsertSession(*sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
}

// openCodeTestDB manages an OpenCode SQLite database for tests.
type openCodeTestDB struct {
	path string
	db   *sql.DB
}

// createOpenCodeDB creates a minimal OpenCode SQLite database
// with the required schema (project, session, message, part
// tables). Returns a handle for inserting test data.
func createOpenCodeDB(t *testing.T, dir string) *openCodeTestDB {
	t.Helper()
	path := filepath.Join(dir, "opencode.db")
	d, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("opening opencode test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	schema := `
		CREATE TABLE project (
			id TEXT PRIMARY KEY,
			worktree TEXT NOT NULL
		);
		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			parent_id TEXT,
			title TEXT,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL
		);
		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			data TEXT NOT NULL,
			time_created INTEGER NOT NULL
		);
		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			message_id TEXT NOT NULL,
			data TEXT NOT NULL,
			time_created INTEGER NOT NULL
		);
	`
	if _, err := d.Exec(schema); err != nil {
		t.Fatalf("creating opencode schema: %v", err)
	}
	return &openCodeTestDB{path: path, db: d}
}

func (oc *openCodeTestDB) addProject(
	t *testing.T, id, worktree string,
) {
	t.Helper()
	_, err := oc.db.Exec(
		"INSERT INTO project (id, worktree) VALUES (?, ?)",
		id, worktree,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
}

func (oc *openCodeTestDB) addSession(
	t *testing.T,
	id, projectID string,
	timeCreated, timeUpdated int64,
) {
	t.Helper()
	_, err := oc.db.Exec(
		`INSERT INTO session
			(id, project_id, time_created, time_updated)
		 VALUES (?, ?, ?, ?)`,
		id, projectID, timeCreated, timeUpdated,
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func (oc *openCodeTestDB) updateSessionTime(
	t *testing.T, id string, timeUpdated int64,
) {
	t.Helper()
	_, err := oc.db.Exec(
		"UPDATE session SET time_updated = ? WHERE id = ?",
		timeUpdated, id,
	)
	if err != nil {
		t.Fatalf("update session time: %v", err)
	}
}

func (oc *openCodeTestDB) addMessage(
	t *testing.T,
	id, sessionID, role string,
	timeCreated int64,
) {
	t.Helper()
	data, _ := json.Marshal(map[string]string{
		"role": role,
	})
	_, err := oc.db.Exec(
		`INSERT INTO message
			(id, session_id, data, time_created)
		 VALUES (?, ?, ?, ?)`,
		id, sessionID, string(data), timeCreated,
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func (oc *openCodeTestDB) addTextPart(
	t *testing.T,
	id, sessionID, messageID, content string,
	timeCreated int64,
) {
	t.Helper()
	data, _ := json.Marshal(map[string]string{
		"type":    "text",
		"content": content,
	})
	_, err := oc.db.Exec(
		`INSERT INTO part
			(id, session_id, message_id, data, time_created)
		 VALUES (?, ?, ?, ?, ?)`,
		id, sessionID, messageID, string(data), timeCreated,
	)
	if err != nil {
		t.Fatalf("insert part: %v", err)
	}
}

func (oc *openCodeTestDB) addToolPart(
	t *testing.T,
	id, sessionID, messageID string,
	toolName, callID string,
	timeCreated int64,
) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{
		"type":   "tool",
		"tool":   toolName,
		"callID": callID,
	})
	_, err := oc.db.Exec(
		`INSERT INTO part
			(id, session_id, message_id, data, time_created)
		 VALUES (?, ?, ?, ?, ?)`,
		id, sessionID, messageID, string(data), timeCreated,
	)
	if err != nil {
		t.Fatalf("insert tool part: %v", err)
	}
}

func (oc *openCodeTestDB) deleteMessages(
	t *testing.T, sessionID string,
) {
	t.Helper()
	_, err := oc.db.Exec(
		"DELETE FROM message WHERE session_id = ?",
		sessionID,
	)
	if err != nil {
		t.Fatalf("delete messages: %v", err)
	}
}

func (oc *openCodeTestDB) deleteParts(
	t *testing.T, sessionID string,
) {
	t.Helper()
	_, err := oc.db.Exec(
		"DELETE FROM part WHERE session_id = ?",
		sessionID,
	)
	if err != nil {
		t.Fatalf("delete parts: %v", err)
	}
}

// replaceTextContent deletes all messages and parts for a
// session, then re-inserts them with new content but the same
// ordinal structure (user msg + assistant msg).
func (oc *openCodeTestDB) replaceTextContent(
	t *testing.T,
	sessionID string,
	userContent, assistantContent string,
	timeCreated int64,
) {
	t.Helper()
	oc.deleteMessages(t, sessionID)
	oc.deleteParts(t, sessionID)

	umID := fmt.Sprintf("%s-msg-user-v2", sessionID)
	amID := fmt.Sprintf("%s-msg-asst-v2", sessionID)
	oc.addMessage(t, umID, sessionID, "user", timeCreated)
	oc.addMessage(
		t, amID, sessionID, "assistant", timeCreated+1,
	)
	oc.addTextPart(
		t, umID+"-p", sessionID, umID,
		userContent, timeCreated,
	)
	oc.addTextPart(
		t, amID+"-p", sessionID, amID,
		assistantContent, timeCreated+1,
	)
}

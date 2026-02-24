package sync_test

import (
	"context"
	"database/sql"
	"testing"

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

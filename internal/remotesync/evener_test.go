package remotesync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/parser"
)

func TestEvenerRemoteFilesAndImport(t *testing.T) {
	root := t.TempDir()
	sessions := filepath.Join(root, "projects", "demo", "sessions")
	require.NoError(t, os.MkdirAll(sessions, 0o755))
	for _, name := range []string{"demo.transcript.jsonl", "demo.meta.json"} {
		data, err := os.ReadFile(filepath.Join("..", "sync", "testdata", "evener", name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(sessions, name), data, 0o600))
	}
	secret := filepath.Join(root, "auth-token")
	log := filepath.Join(sessions, "demo.api.jsonl")
	require.NoError(t, os.WriteFile(secret, []byte("synthetic-secret"), 0o600))
	require.NoError(t, os.WriteFile(log, []byte("synthetic-operation-log"), 0o600))
	targets, err := ResolveTargets(config.Config{AgentDirs: map[parser.AgentType][]string{parser.AgentType("evener"): {root}}})
	require.NoError(t, err)
	manifest, err := BuildManifest(targets)
	require.NoError(t, err)
	var files []string
	for _, f := range manifest.Files {
		files = append(files, f.Path)
	}
	assert.ElementsMatch(t, []string{filepath.Join(sessions, "demo.transcript.jsonl"), filepath.Join(sessions, "demo.meta.json")}, files)
	_, allowed := SelectAllowedFiles(targets, []string{secret, log})
	assert.False(t, allowed)

	database := dbtest.OpenTestDB(t)
	// Repeat extraction into different directories to exercise canonical identity.
	for _, title := range []string{"Orchard investigation", "Remote diagnosis"} {
		extracted := t.TempDir()
		local := filepath.Join(extracted, "remote", "evener", "projects", "demo", "sessions")
		require.NoError(t, os.MkdirAll(local, 0o755))
		data, err := os.ReadFile(filepath.Join(sessions, "demo.transcript.jsonl"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(local, "demo.transcript.jsonl"), data, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(local, "demo.meta.json"), []byte(`{"id":"demo","name":"`+title+`"}`), 0o600))
		stats, err := (Importer{Host: "test-host", DB: database}).ImportExtracted(t.Context(), TargetSet{Dirs: map[parser.AgentType][]string{parser.AgentType("evener"): {"/remote/evener"}}}, extracted)
		require.NoError(t, err)
		require.Zero(t, stats.Failed)
		page, err := database.ListSessions(t.Context(), db.SessionFilter{Limit: 10})
		require.NoError(t, err)
		require.Len(t, page.Sessions, 1)
		session, err := database.GetSessionFull(t.Context(), page.Sessions[0].ID)
		require.NoError(t, err)
		require.NotNil(t, session)
		require.NotNil(t, session.FilePath)
		assert.Equal(t, "test-host:/remote/evener/projects/demo/sessions/demo.transcript.jsonl", *session.FilePath)
		require.NotNil(t, session.SessionName)
		assert.Equal(t, title, *session.SessionName)
		messages, err := database.GetMessages(t.Context(), session.ID, 0, 100, true)
		require.NoError(t, err)
		require.Len(t, messages, 3)
		assert.Equal(t, 20, messages[1].OutputTokens)
	}
}

func TestEvenerEmptyRemoteRootDoesNotExportUnrelatedState(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "auth-token")
	require.NoError(t, os.WriteFile(secret, []byte("synthetic-secret"), 0o600))
	targets, err := ResolveTargets(config.Config{AgentDirs: map[parser.AgentType][]string{parser.AgentType("evener"): {root}}})
	require.NoError(t, err)
	manifest, err := BuildManifest(targets)
	require.NoError(t, err)
	assert.Empty(t, manifest.Files)
	_, allowed := SelectAllowedFiles(targets, []string{secret})
	assert.False(t, allowed)
}

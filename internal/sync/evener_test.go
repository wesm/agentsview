package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/parser"
)

func TestEvenerArchiveLifecycle(t *testing.T) {
	database := openTestDB(t)
	root := t.TempDir()
	sessions := filepath.Join(root, "projects", "project-demo", "sessions")
	require.NoError(t, os.MkdirAll(sessions, 0o755))
	source, err := os.ReadFile("testdata/evener/demo.transcript.jsonl")
	require.NoError(t, err)
	path := filepath.Join(sessions, "demo.transcript.jsonl")
	metaPath := filepath.Join(sessions, "demo.meta.json")
	require.NoError(t, os.WriteFile(path, source, 0o600))
	require.NoError(t, os.WriteFile(metaPath, []byte(`{"id":"demo","name":"Orchard investigation"}`), 0o600))
	engine := NewEngine(database, EngineConfig{AgentDirs: map[parser.AgentType][]string{parser.AgentType("evener"): {root}}, Machine: "local"})
	t.Cleanup(engine.Close)
	ctx := t.Context()
	first := engine.SyncAll(ctx, nil)
	require.Equal(t, 1, first.Synced)
	require.Zero(t, first.Failed)
	session, err := database.GetSessionFull(ctx, "evener:demo")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "evener", session.Agent)
	require.NotNil(t, session.SessionName)
	assert.Equal(t, "Orchard investigation", *session.SessionName)
	messages, err := database.GetMessages(ctx, session.ID, 0, 100, true)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	assert.Equal(t, "Check the state before changing it.", messages[1].ThinkingText)
	assert.Equal(t, 20, messages[1].OutputTokens)
	assert.Equal(t, 130, messages[1].ContextTokens)
	require.Len(t, messages[1].ToolCalls, 1)
	assert.Equal(t, "call-1", messages[1].ToolCalls[0].ToolUseID)
	assert.Contains(t, messages[1].ToolCalls[0].ResultContent, "/workspace/demo")
	found, err := database.Search(ctx, db.SearchFilter{Query: "orchard"})
	require.NoError(t, err)
	require.NotEmpty(t, found.Results)
	assert.Equal(t, session.ID, found.Results[0].SessionID)

	exported, err := database.LoadArtifactExportData(ctx, session.ID, db.ArtifactExportLoadLimits{
		Messages: 100, UsageEvents: 100, MessageToolCalls: 100, ToolResultEvents: 100,
		SessionToolCalls: 100, SessionResultEvents: 100, MessageBytes: 1 << 20, UsageBytes: 1 << 20,
	})
	require.NoError(t, err)
	require.Len(t, exported.Messages, len(messages))
	assert.Equal(t, messages[1].ThinkingText, exported.Messages[1].ThinkingText)
	assert.Equal(t, messages[1].TokenUsage, exported.Messages[1].TokenUsage)
	require.Len(t, exported.Messages[1].ToolCalls, 1)
	require.Len(t, exported.Messages[1].ToolCalls[0].ResultEvents, 1)
	assert.Equal(t, "/workspace/demo", exported.Messages[1].ToolCalls[0].ResultEvents[0].Content)

	unchanged := engine.SyncAll(ctx, nil)
	require.Zero(t, unchanged.Failed)
	assert.Zero(t, unchanged.Synced)

	// Only a sibling metadata write changes; the transcript remains untouched.
	beforeMetadata := engine.SourceMtime(session.ID)
	require.NoError(t, os.WriteFile(metaPath, []byte(`{"id":"demo","name":"Orchard diagnosis"}`), 0o600))
	assert.NotEqual(t, beforeMetadata, engine.SourceMtime(session.ID), "session polling must notice metadata-only updates")
	renamed := engine.SyncAll(ctx, nil)
	require.Zero(t, renamed.Failed)
	session, err = database.GetSessionFull(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, session.SessionName)
	assert.Equal(t, "Orchard diagnosis", *session.SessionName)

	tail := `{"kind":"entry","seq":4,"turn":{"kind":"ASSISTANT","timestamp":"2026-09-05T10:00:04Z","message":{"role":"assistant","content":[{"kind":"text","text":"The orchard is synchronized."}]},"response_model":"gpt-4.1","usage":{"input_tokens":10,"output_tokens":4}}}`
	require.NoError(t, os.WriteFile(path, append(append([]byte{}, source...), []byte(tail[:len(tail)/2])...), 0o600))
	partial := engine.SyncAll(ctx, nil)
	assert.Positive(t, partial.Failed, "incomplete sources stay eligible for retry")
	messages, err = database.GetMessages(ctx, session.ID, 0, 100, true)
	require.NoError(t, err)
	assert.Len(t, messages, 3)
	require.NoError(t, os.WriteFile(path, append(append([]byte{}, source...), []byte(tail+"\n")...), 0o600))
	complete := engine.SyncAll(ctx, nil)
	require.Zero(t, complete.Failed)
	messages, err = database.GetMessages(ctx, session.ID, 0, 100, true)
	require.NoError(t, err)
	require.Len(t, messages, 4)
	assert.Equal(t, "The orchard is synchronized.", messages[3].Content)

	// Replacing a source must remove obsolete messages instead of appending.
	lines := strings.Split(strings.TrimSpace(string(source)), "\n")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines[:2], "\n")+"\n"), 0o600))
	replaced := engine.SyncAll(ctx, nil)
	require.Zero(t, replaced.Failed)
	messages, err = database.GetMessages(ctx, session.ID, 0, 100, true)
	require.NoError(t, err)
	assert.Len(t, messages, 1)

	// Removing provider metadata clears its title without deleting the session.
	require.NoError(t, os.Remove(metaPath))
	removedMeta := engine.SyncAll(ctx, nil)
	require.Zero(t, removedMeta.Failed)
	session, err = database.GetSessionFull(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, session)
	if session.SessionName != nil {
		assert.Empty(t, *session.SessionName)
	}

	// Bad input must not replace the last good archive contents.
	require.NoError(t, os.WriteFile(path, []byte(lines[0]+"\n{bad}\n"), 0o600))
	corrupt := engine.SyncAll(ctx, nil)
	assert.Equal(t, 1, corrupt.Failed)
	messages, err = database.GetMessages(ctx, session.ID, 0, 100, true)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
}

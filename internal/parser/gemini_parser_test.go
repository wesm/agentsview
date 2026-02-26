package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wesm/agentsview/internal/testjsonl"
)

func runGeminiParserTest(t *testing.T, content string) (*ParsedSession, []ParsedMessage) {
	t.Helper()
	path := createTestFile(t, "session.json", content)
	sess, msgs, err := ParseGeminiSession(path, "my_project", "local")
	require.NoError(t, err)
	return sess, msgs
}

func TestParseGeminiSession_Basic(t *testing.T) {
	content := loadFixture(t, "gemini/standard_session.json")
	sess, msgs := runGeminiParserTest(t, content)
	
	require.NotNil(t, sess)
	assertMessageCount(t, len(msgs), 4)
	assertMessageCount(t, sess.MessageCount, 4)
	assertSessionMeta(t, sess, "gemini:sess-uuid-1", "my_project", AgentGemini)
	assert.Equal(t, "Fix the login bug", sess.FirstMessage)
	
	assertMessage(t, msgs[0], RoleUser, "Fix the login bug")
	assertMessage(t, msgs[1], RoleAssistant, "Looking at")
	assert.Equal(t, 0, msgs[0].Ordinal)
	assert.Equal(t, 1, msgs[1].Ordinal)
}

func TestParseGeminiSession_ToolCalls(t *testing.T) {
	t.Run("basic tool calls", func(t *testing.T) {
		content := loadFixture(t, "gemini/tool_calls.json")
		_, msgs := runGeminiParserTest(t, content)
		
		assert.Equal(t, 2, len(msgs))
		assert.True(t, msgs[1].HasToolUse)
		assert.True(t, msgs[1].HasThinking)
		assert.True(t, strings.Contains(msgs[1].Content, "[Thinking: Planning]"))
		assert.True(t, strings.Contains(msgs[1].Content, "[Read: main.go]"))
		assertToolCalls(t, msgs[1].ToolCalls, []ParsedToolCall{{ToolName: "read_file", Category: "Read"}})
	})

	t.Run("empty tool name skipped", func(t *testing.T) {
		content := testjsonl.GeminiSessionJSON("sess-uuid-empty-tc", "hash", tsEarly, tsEarlyS5, []map[string]any{
			testjsonl.GeminiUserMsg("u1", tsEarly, "do it"),
			testjsonl.GeminiAssistantMsg("a1", tsEarlyS5, "Using tool.", &testjsonl.GeminiMsgOpts{
				ToolCalls: []testjsonl.GeminiToolCall{{Name: "", DisplayName: "", Args: nil}},
			}),
		})
		_, msgs := runGeminiParserTest(t, content)
		assert.Equal(t, 2, len(msgs))
		assert.True(t, msgs[1].HasToolUse)
		assertToolCalls(t, msgs[1].ToolCalls, nil)
	})
}

func TestParseGeminiSession_EdgeCases(t *testing.T) {
	t.Run("only system messages", func(t *testing.T) {
		content := loadFixture(t, "gemini/system_messages.json")
		sess, msgs := runGeminiParserTest(t, content)
		require.NotNil(t, sess)
		assert.Equal(t, 0, len(msgs))
	})

	t.Run("first message truncation", func(t *testing.T) {
		content := testjsonl.GeminiSessionJSON(
			"sess-uuid-6", "hash", tsEarly, tsEarlyS5,
			[]map[string]any{
				testjsonl.GeminiUserMsg("u1", tsEarly, generateLargeString(400)),
			},
		)
		sess, _ := runGeminiParserTest(t, content)
		require.NotNil(t, sess)
		assert.Equal(t, 303, len(sess.FirstMessage))
	})
	
	t.Run("malformed JSON", func(t *testing.T) {
		path := createTestFile(t, "session.json", "not valid json {{{")
		_, _, err := ParseGeminiSession(path, "my_project", "local")
		assert.Error(t, err)
	})
	
	t.Run("missing file", func(t *testing.T) {
		_, _, err := ParseGeminiSession("/nonexistent.json", "my_project", "local")
		assert.Error(t, err)
	})

	t.Run("empty messages array", func(t *testing.T) {
		content := testjsonl.GeminiSessionJSON("sess-uuid-4", "hash", tsEarly, tsEarlyS5, []map[string]any{})
		sess, msgs := runGeminiParserTest(t, content)
		assert.Equal(t, 0, sess.MessageCount)
		assert.Equal(t, 0, len(msgs))
	})

	t.Run("content as Part array", func(t *testing.T) {
		content := testjsonl.GeminiSessionJSON("sess-uuid-5", "hash", tsEarly, tsEarlyS5, []map[string]any{
			{
				"id":        "u1",
				"timestamp": tsEarly,
				"type":      "user",
				"content": []map[string]string{
					{"text": "part one"},
					{"text": "part two"},
				},
			},
		})
		_, msgs := runGeminiParserTest(t, content)
		assert.Equal(t, 1, len(msgs))
		assert.True(t, strings.Contains(msgs[0].Content, "part one"))
		assert.True(t, strings.Contains(msgs[0].Content, "part two"))
	})

	t.Run("timestamps from startTime and lastUpdated", func(t *testing.T) {
		content := testjsonl.GeminiSessionJSON("sess-uuid-7", "hash", "2024-06-15T10:00:00Z", "2024-06-15T11:30:00Z", []map[string]any{
			testjsonl.GeminiUserMsg("u1", "2024-06-15T10:00:00Z", "hello"),
		})
		sess, _ := runGeminiParserTest(t, content)
		wantStart := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
		wantEnd := time.Date(2024, 6, 15, 11, 30, 0, 0, time.UTC)
		assertTimestamp(t, sess.StartedAt, wantStart)
		assertTimestamp(t, sess.EndedAt, wantEnd)
	})

	t.Run("missing sessionId", func(t *testing.T) {
		content := `{"projectHash":"abc","startTime":"2024-01-01T00:00:00Z","lastUpdated":"2024-01-01T00:00:00Z","messages":[]}`
		path := createTestFile(t, "session.json", content)
		_, _, err := ParseGeminiSession(path, "my_project", "local")
		assert.Error(t, err)
	})
}

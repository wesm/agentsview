package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wesm/agentsview/internal/testjsonl"
)

func runClaudeParserTest(t *testing.T, fileName, content string) (ParsedSession, []ParsedMessage) {
	t.Helper()
	if fileName == "" {
		fileName = "test.jsonl"
	}
	path := createTestFile(t, fileName, content)
	results, err := ParseClaudeSession(path, "my_app", "local")
	require.NoError(t, err)
	require.NotEmpty(t, results)
	return results[0].Session, results[0].Messages
}

func TestParseClaudeSession_Basic(t *testing.T) {
	content := loadFixture(t, "claude/valid_session.jsonl")
	sess, msgs := runClaudeParserTest(t, "test.jsonl", content)
	
	assertMessageCount(t, len(msgs), 4)
	assertMessageCount(t, sess.MessageCount, 4)
	assertSessionMeta(t, &sess, "test", "my_app", AgentClaude)
	assert.Equal(t, "Fix the login bug", sess.FirstMessage)
	
	assertMessage(t, msgs[0], RoleUser, "")
	assertMessage(t, msgs[1], RoleAssistant, "")
	assert.True(t, msgs[1].HasToolUse)
	assertToolCalls(t, msgs[1].ToolCalls, []ParsedToolCall{{ToolUseID: "toolu_1", ToolName: "Read", Category: "Read", InputJSON: `{"file_path":"src/auth.ts"}`}})
	assert.Equal(t, 0, msgs[0].Ordinal)
	assert.Equal(t, 1, msgs[1].Ordinal)
}

func TestParseClaudeSession_HyphenatedFilename(t *testing.T) {
	content := loadFixture(t, "claude/valid_session.jsonl")
	sess, _ := runClaudeParserTest(t, "my-test-session.jsonl", content)
	assert.Equal(t, "my-test-session", sess.ID)
}

func TestParseClaudeSession_EdgeCases(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		sess, msgs := runClaudeParserTest(t, "test.jsonl", "")
		assert.Empty(t, msgs)
		assert.Equal(t, 0, sess.MessageCount)
	})

	t.Run("skips blank content", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeUserJSON("", tsZero),
			testjsonl.ClaudeUserJSON("  ", tsZeroS1),
			testjsonl.ClaudeUserJSON("actual message", tsZeroS2),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1, sess.MessageCount)
	})

	t.Run("truncates long first message", func(t *testing.T) {
		content := testjsonl.ClaudeUserJSON(generateLargeString(400), tsZero) + "\n"
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 303, len(sess.FirstMessage))
	})

	t.Run("skips invalid JSON lines", func(t *testing.T) {
		content := "not valid json\n" +
			testjsonl.ClaudeUserJSON("hello", tsZero) + "\n" +
			"also not valid\n"
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1, sess.MessageCount)
	})

	t.Run("malformed UTF-8", func(t *testing.T) {
		badUTF8 := `{"type":"user","timestamp":"` + tsZeroS1 + `","message":{"content":"bad ` + string([]byte{0xff, 0xfe}) + `"}}` + "\n"
		content := testjsonl.ClaudeUserJSON("valid message", tsZero) + "\n" + badUTF8
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.GreaterOrEqual(t, sess.MessageCount, 1)
	})

	t.Run("very large message", func(t *testing.T) {
		content := testjsonl.ClaudeUserJSON(generateLargeString(1024*1024), tsZero) + "\n"
		_, msgs := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1024*1024, msgs[0].ContentLength)
	})

	t.Run("skips empty lines in file", func(t *testing.T) {
		content := "\n\n" +
			testjsonl.ClaudeUserJSON("msg1", tsZero) +
			"\n   \n\t\n" +
			testjsonl.ClaudeAssistantJSON([]map[string]any{{"type": "text", "text": "reply"}}, tsZeroS1) +
			"\n\n"
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 2, sess.MessageCount)
	})

	t.Run("skips partial/truncated JSON", func(t *testing.T) {
		content := testjsonl.ClaudeUserJSON("first", tsZero) + "\n" +
			`{"type":"user","truncated` + "\n" +
			testjsonl.ClaudeAssistantJSON([]map[string]any{{"type": "text", "text": "last"}}, tsZeroS2) + "\n"
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 2, sess.MessageCount)
	})
}

func TestParseClaudeSession_SkippedMessages(t *testing.T) {
	t.Run("skips isMeta user messages", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeMetaUserJSON("meta context", tsZero, true, false),
			testjsonl.ClaudeUserJSON("real question", tsZeroS1),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1, sess.MessageCount)
		assert.Equal(t, "real question", sess.FirstMessage)
	})

	t.Run("skips isCompactSummary user messages", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeMetaUserJSON("summary of prior turns", tsZero, false, true),
			testjsonl.ClaudeUserJSON("actual prompt", tsZeroS1),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1, sess.MessageCount)
		assert.Equal(t, "actual prompt", sess.FirstMessage)
	})

	t.Run("skips content-heuristic system messages", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeUserJSON("This session is being continued from a previous conversation.", tsZero),
			testjsonl.ClaudeUserJSON("[Request interrupted by user]", tsZeroS1),
			testjsonl.ClaudeUserJSON("<task-notification>data</task-notification>", tsZeroS2),
			testjsonl.ClaudeUserJSON("<command-message>x</command-message>", "2024-01-01T00:00:03Z"),
			testjsonl.ClaudeUserJSON("<command-name>commit</command-name>", "2024-01-01T00:00:04Z"),
			testjsonl.ClaudeUserJSON("<local-command-result>ok</local-command-result>", "2024-01-01T00:00:05Z"),
			testjsonl.ClaudeUserJSON("Stop hook feedback: rejected", "2024-01-01T00:00:06Z"),
			testjsonl.ClaudeUserJSON("real user message", "2024-01-01T00:00:07Z"),
		)
		sess, msgs := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1, sess.MessageCount)
		assert.Equal(t, "real user message", msgs[0].Content)
		assert.Equal(t, "real user message", sess.FirstMessage)
	})

	t.Run("assistant with system-like content not filtered", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeUserJSON("hello", tsZero),
			testjsonl.ClaudeAssistantJSON([]map[string]any{
				{"type": "text", "text": "This session is being continued from a previous conversation."},
			}, tsZeroS1),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 2, sess.MessageCount)
	})

	t.Run("firstMsg from first non-system user message", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeMetaUserJSON("context data", tsZero, true, false),
			testjsonl.ClaudeUserJSON("This session is being continued from a previous conversation.", tsZeroS1),
			testjsonl.ClaudeUserJSON("Fix the auth bug", tsZeroS2),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, 1, sess.MessageCount)
		assert.Equal(t, "Fix the auth bug", sess.FirstMessage)
	})
}

func TestParseClaudeSession_ParentSessionID(t *testing.T) {
	t.Run("sessionId != fileId sets ParentSessionID", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeUserWithSessionIDJSON("hello", tsZero, "parent-uuid"),
			testjsonl.ClaudeAssistantJSON([]map[string]any{
				{"type": "text", "text": "hi"},
			}, tsZeroS1),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Equal(t, "parent-uuid", sess.ParentSessionID)
	})

	t.Run("sessionId == fileId yields empty ParentSessionID", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeUserWithSessionIDJSON("hello", tsZero, "test"),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Empty(t, sess.ParentSessionID)
	})

	t.Run("no sessionId field yields empty ParentSessionID", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.ClaudeUserJSON("hello", tsZero),
		)
		sess, _ := runClaudeParserTest(t, "test.jsonl", content)
		assert.Empty(t, sess.ParentSessionID)
	})
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

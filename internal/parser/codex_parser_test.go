package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wesm/agentsview/internal/testjsonl"
)

func runCodexParserTest(t *testing.T, fileName, content string, includeExec bool) (*ParsedSession, []ParsedMessage) {
	t.Helper()
	if fileName == "" {
		fileName = "test.jsonl"
	}
	path := createTestFile(t, fileName, content)
	sess, msgs, err := ParseCodexSession(path, "local", includeExec)
	require.NoError(t, err)
	return sess, msgs
}

func TestParseCodexSession_Basic(t *testing.T) {
	content := loadFixture(t, "codex/standard_session.jsonl")
	sess, msgs := runCodexParserTest(t, "test.jsonl", content, false)
	
	require.NotNil(t, sess)
	assert.Equal(t, "codex:abc-123", sess.ID)
	assert.Equal(t, 2, len(msgs))
	assertSessionMeta(t, sess, "codex:abc-123", "my_api", AgentCodex)
}

func TestParseCodexSession_ExecOriginator(t *testing.T) {
	execContent := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON("abc", "/tmp", "codex_exec", tsEarly),
		testjsonl.CodexMsgJSON("user", "test", tsEarlyS1),
	)

	t.Run("skips exec originator", func(t *testing.T) {
		sess, _ := runCodexParserTest(t, "test.jsonl", execContent, false)
		assert.Nil(t, sess)
	})

	t.Run("includes exec when requested", func(t *testing.T) {
		sess, msgs := runCodexParserTest(t, "test.jsonl", execContent, true)
		require.NotNil(t, sess)
		assert.Equal(t, "codex:abc", sess.ID)
		assert.Equal(t, 1, len(msgs))
	})
}

func TestParseCodexSession_FunctionCalls(t *testing.T) {
	t.Run("function calls", func(t *testing.T) {
		content := loadFixture(t, "codex/function_calls.jsonl")
		sess, msgs := runCodexParserTest(t, "test.jsonl", content, false)
		
		require.NotNil(t, sess)
		assert.Equal(t, "codex:fc-1", sess.ID)
		assert.Equal(t, 3, len(msgs))
		
		assert.Equal(t, RoleUser, msgs[0].Role)
		assert.False(t, msgs[0].HasToolUse)
		
		assert.Equal(t, RoleAssistant, msgs[1].Role)
		assert.True(t, msgs[1].HasToolUse)
		assertToolCalls(t, msgs[1].ToolCalls, []ParsedToolCall{{ToolName: "shell_command", Category: "Bash"}})
		assert.Equal(t, "[Bash: Running tests]", msgs[1].Content)
		
		assert.True(t, msgs[2].HasToolUse)
		assertToolCalls(t, msgs[2].ToolCalls, []ParsedToolCall{{ToolName: "apply_patch", Category: "Edit"}})
		
		for i, m := range msgs {
			assert.Equal(t, i, m.Ordinal)
		}
	})

	t.Run("exec_command arguments include command detail", func(t *testing.T) {
		content := loadFixture(t, "codex/fc_args_1.jsonl")
		_, msgs := runCodexParserTest(t, "test.jsonl", content, false)
		assert.Equal(t, "[Bash]\n$ rg --files", msgs[1].Content)
	})

	t.Run("apply_patch arguments summarize edited files", func(t *testing.T) {
		content := loadFixture(t, "codex/fc_args_2.jsonl")
		_, msgs := runCodexParserTest(t, "test.jsonl", content, false)
		want := "[Edit: internal/parser/codex.go (+1 more)]\ninternal/parser/codex.go\ninternal/parser/parser_test.go"
		assert.Equal(t, want, msgs[1].Content)
	})
	
	t.Run("write_stdin formats with session and chars", func(t *testing.T) {
		content := loadFixture(t, "codex/fc_stdin.jsonl")
		_, msgs := runCodexParserTest(t, "test.jsonl", content, false)
		want := "[Bash: stdin -> sess-42]\nyes\\n"
		assert.Equal(t, want, msgs[1].Content)
		assert.Equal(t, "Bash", msgs[1].ToolCalls[0].Category)
	})
}

func TestParseCodexSession_EdgeCases(t *testing.T) {
	t.Run("skips system messages", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.CodexSessionMetaJSON("abc", "/tmp", "user", tsEarly),
			testjsonl.CodexMsgJSON("user", "# AGENTS.md\nsome instructions", tsEarlyS1),
			testjsonl.CodexMsgJSON("user", "<environment_context>stuff</environment_context>", "2024-01-01T10:00:02Z"),
			testjsonl.CodexMsgJSON("user", "<INSTRUCTIONS>ignore</INSTRUCTIONS>", "2024-01-01T10:00:03Z"),
			testjsonl.CodexMsgJSON("user", "Actual user message", "2024-01-01T10:00:04Z"),
		)
		sess, msgs := runCodexParserTest(t, "test.jsonl", content, false)
		require.NotNil(t, sess)
		assert.Equal(t, 1, len(msgs))
		assert.Equal(t, "Actual user message", msgs[0].Content)
	})

	t.Run("fallback ID from filename", func(t *testing.T) {
		content := testjsonl.CodexMsgJSON("user", "hello", tsEarlyS1)
		sess, _ := runCodexParserTest(t, "test.jsonl", content, false)
		require.NotNil(t, sess)
		assert.Equal(t, "codex:test", sess.ID)
	})

	t.Run("large message within scanner limit", func(t *testing.T) {
		content := testjsonl.JoinJSONL(
			testjsonl.CodexSessionMetaJSON("big", "/tmp", "user", tsEarly),
			testjsonl.CodexMsgJSON("user", generateLargeString(1024*1024), tsEarlyS1),
		)
		_, msgs := runCodexParserTest(t, "test.jsonl", content, false)
		assert.Equal(t, 1024*1024, msgs[0].ContentLength)
	})
}

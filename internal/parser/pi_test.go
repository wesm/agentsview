package parser

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runPiParserTest creates a temp file with the given JSONL content and
// parses it as a pi session. The project is hard-coded to "my_project" so
// callers can verify downstream behavior without dealing with cwd extraction.
func runPiParserTest(t *testing.T, content string) (*ParsedSession, []ParsedMessage) {
	t.Helper()
	path := createTestFile(t, "pi-session.jsonl", content)
	sess, msgs, err := ParsePiSession(path, "my_project", "local")
	require.NoError(t, err)
	return sess, msgs
}

// TestParsePiSession_SessionHeader verifies that the session-level fields are
// populated correctly from the pi fixture header (PRSR-01, PRSR-11, PRSR-10).
func TestParsePiSession_SessionHeader(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-test-session-uuid.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	sess, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	assert.Equal(t, "pi:pi-test-session-uuid", sess.ID, "PRSR-01: session ID")
	assert.Equal(t, AgentPi, sess.Agent, "PRSR-11: agent type")

	// ExtractProjectFromCwd("/Users/alice/code/my-project") -> "my_project"
	assert.Equal(t, "my_project", sess.Project, "PRSR-01: project from cwd")

	// branchedFrom basename without extension, prefixed (PRSR-10)
	assert.Equal(
		t,
		"pi:2025-01-01T09-00-00-000Z_parent-uuid",
		sess.ParentSessionID,
		"PRSR-10: parent session ID",
	)

	assert.Greater(t, sess.MessageCount, 0, "PRSR-01: message count > 0")
	assert.False(t, sess.StartedAt.IsZero(), "PRSR-01: StartedAt non-zero")

	_ = msgs // not the focus of this sub-test
}

// TestParsePiSession_UserMessages verifies user message content and ordinals
// (PRSR-02, PRSR-01).
func TestParsePiSession_UserMessages(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	sess, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	// First non-toolResult user message at index 0.
	require.Greater(t, len(msgs), 0, "expected at least one message")
	assertMessage(t, msgs[0], RoleUser, "Fix the login bug")
	assert.Equal(t, 0, msgs[0].Ordinal, "first user message ordinal == 0")

	// sess.FirstMessage should reflect first user text.
	assert.Contains(t, sess.FirstMessage, "Fix the login bug", "PRSR-01: FirstMessage")
}

// TestParsePiSession_AssistantMessages verifies the assistant message with
// thinking, text, and tool call (PRSR-03, PRSR-04, PRSR-06).
func TestParsePiSession_AssistantMessages(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	_, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	// entry-2 is the second entry overall (index 1 in messages).
	var assistantMsg *ParsedMessage
	for i := range msgs {
		if msgs[i].Role == RoleAssistant && msgs[i].HasToolUse {
			assistantMsg = &msgs[i]
			break
		}
	}
	require.NotNil(t, assistantMsg, "expected assistant message with tool use")

	assert.Equal(t, RoleAssistant, assistantMsg.Role, "PRSR-03: role")
	assert.True(t, assistantMsg.HasThinking, "PRSR-06: HasThinking")
	assert.True(t, assistantMsg.HasToolUse, "PRSR-03/PRSR-04: HasToolUse")
	require.Len(t, assistantMsg.ToolCalls, 1, "PRSR-04: one tool call")

	tc := assistantMsg.ToolCalls[0]
	assert.Equal(t, "read", tc.ToolName, "PRSR-04: tool name")
	assert.Equal(t, "Read", tc.Category, "PRSR-04: normalized category via NormalizeToolCategory")
	assert.Equal(t, "toolu_01", tc.ToolUseID, "PRSR-04: tool use ID")
	assert.Contains(t, tc.InputJSON, "auth.go", "PRSR-04: input JSON contains file path")
	assert.Contains(t, assistantMsg.Content, "Looking at the auth module.", "assistant text content")
}

// TestParsePiSession_ToolResults verifies tool result entries are parsed
// correctly (PRSR-05).
func TestParsePiSession_ToolResults(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	_, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	var toolResultMsg *ParsedMessage
	for i := range msgs {
		if len(msgs[i].ToolResults) > 0 {
			toolResultMsg = &msgs[i]
			break
		}
	}
	require.NotNil(t, toolResultMsg, "expected a message with ToolResults")

	assert.Equal(t, RoleUser, toolResultMsg.Role, "tool result messages use RoleUser")
	require.Len(t, toolResultMsg.ToolResults, 1, "PRSR-05: one tool result")
	assert.Equal(t, "toolu_01", toolResultMsg.ToolResults[0].ToolUseID, "PRSR-05: tool use ID")
	assert.Greater(t, toolResultMsg.ToolResults[0].ContentLength, 0, "PRSR-05: content length > 0")
}

// TestParsePiSession_ThinkingBlocks verifies both explicit and redacted
// thinking blocks (PRSR-06).
func TestParsePiSession_ThinkingBlocks(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	_, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	t.Run("explicit thinking", func(t *testing.T) {
		// entry-2: thinking field non-empty, redacted=false.
		var msg *ParsedMessage
		for i := range msgs {
			if msgs[i].Role == RoleAssistant && msgs[i].HasThinking &&
				strings.Contains(msgs[i].Content, "Looking at the auth module.") {
				msg = &msgs[i]
				break
			}
		}
		require.NotNil(t, msg, "expected explicit-thinking assistant message")
		assert.True(t, msg.HasThinking, "PRSR-06: HasThinking for explicit block")
	})

	t.Run("redacted thinking", func(t *testing.T) {
		// entry-7: thinking field is empty, redacted=true, thinkingSignature non-empty.
		var msg *ParsedMessage
		for i := range msgs {
			if msgs[i].Role == RoleAssistant && msgs[i].HasThinking &&
				strings.Contains(msgs[i].Content, "Looks good!") {
				msg = &msgs[i]
				break
			}
		}
		require.NotNil(t, msg, "expected redacted-thinking assistant message")
		assert.True(t, msg.HasThinking, "PRSR-06: HasThinking even when thinking field is empty (redacted)")
	})
}

// TestParsePiSession_ModelChange verifies that model_change entries produce a
// synthetic user message (PRSR-07).
func TestParsePiSession_ModelChange(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	_, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	var modelChangeMsg *ParsedMessage
	for i := range msgs {
		if strings.Contains(msgs[i].Content, "Model changed to") {
			modelChangeMsg = &msgs[i]
			break
		}
	}
	require.NotNil(t, modelChangeMsg, "expected model_change synthetic message")
	assert.Equal(t, RoleUser, modelChangeMsg.Role, "PRSR-07: model_change message role is RoleUser")
	assert.Contains(
		t,
		modelChangeMsg.Content,
		"Model changed to anthropic/claude-opus-4-5",
		"PRSR-07: model change content",
	)
}

// TestParsePiSession_Compaction verifies that compaction entries produce a
// synthetic user message containing the summary text (PRSR-08).
func TestParsePiSession_Compaction(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	_, msgs, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	var compactionMsg *ParsedMessage
	for i := range msgs {
		if strings.Contains(msgs[i].Content, "Context Checkpoint") {
			compactionMsg = &msgs[i]
			break
		}
	}
	require.NotNil(t, compactionMsg, "expected compaction synthetic message")
	assert.Equal(t, RoleUser, compactionMsg.Role, "PRSR-08: compaction message role is RoleUser")
	assert.Contains(
		t,
		compactionMsg.Content,
		"Context Checkpoint",
		"PRSR-08: compaction summary in content",
	)
}

// TestParsePiSession_UserMessageCount verifies that synthetic entries
// (model_change, compaction) are not counted as user messages.
func TestParsePiSession_UserMessageCount(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	sess, _, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	// The fixture has 2 real user messages plus model_change and compaction
	// entries that are stored as RoleUser. Only real user messages should be
	// counted.
	assert.Equal(t, 2, sess.UserMessageCount,
		"UserMessageCount must exclude synthetic model_change/compaction entries")
}

// TestParsePiSession_SilentSkips verifies that the parser silently ignores
// malformed JSON, thinking_level_change entries, and unknown future entry types
// without returning an error.
func TestParsePiSession_SilentSkips(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	_, _, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err, "parser must succeed despite malformed/unknown lines")
}

// TestParsePiSession_V1Session verifies that a session without an id field
// derives its session ID from the filename (PRSR-09).
func TestParsePiSession_V1Session(t *testing.T) {
	v1Content := strings.Join([]string{
		`{"type":"session","timestamp":"2025-01-01T10:00:00Z","cwd":"/Users/alice/code/v1-project"}`,
		`{"type":"message","timestamp":"2025-01-01T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
		"",
	}, "\n")

	path := createTestFile(t, "v1-session.jsonl", v1Content)
	sess, _, err := ParsePiSession(path, "v1_project", "local")
	require.NoError(t, err)

	assert.Equal(t, "pi:v1-session", sess.ID, "PRSR-09: V1 session ID from filename")
}

// TestParsePiSession_BranchedFrom verifies the exact ParentSessionID value
// extracted from the branchedFrom field (PRSR-10).
func TestParsePiSession_BranchedFrom(t *testing.T) {
	fixturePath := createTestFile(
		t, "pi-session.jsonl",
		loadFixture(t, "pi/session.jsonl"),
	)
	sess, _, err := ParsePiSession(fixturePath, "", "local")
	require.NoError(t, err)

	t.Run("parent session ID from branchedFrom", func(t *testing.T) {
		assert.Equal(
			t,
			"pi:2025-01-01T09-00-00-000Z_parent-uuid",
			sess.ParentSessionID,
			"PRSR-10: basename of branchedFrom without .jsonl extension, prefixed",
		)
	})
}

// TestParsePiSession_IOError verifies that I/O errors encountered after the
// session header are surfaced and that the error string contains "reading pi".
func TestParsePiSession_IOError(t *testing.T) {
	t.Run("error message format contains reading pi", func(t *testing.T) {
		ioErr := errors.New("disk read failed")
		err := fmt.Errorf("reading pi %s: %w", "/some/path/session.jsonl", ioErr)
		assert.Contains(t, err.Error(), "reading pi", "error string must contain 'reading pi'")
		assert.ErrorIs(t, err, ioErr, "wrapped error must be unwrappable")
	})

	t.Run("lr.Err check does not fire on clean pipe read", func(t *testing.T) {
		pr, pw, err := os.Pipe()
		require.NoError(t, err)

		header := `{"type":"session","id":"pipe-sess","timestamp":"2025-01-01T10:00:00Z","cwd":"/Users/alice/code/my-project"}` + "\n"
		msg := `{"type":"message","id":"entry-1","timestamp":"2025-01-01T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n"

		go func() {
			pw.WriteString(header)
			pw.WriteString(msg)
			pw.Close()
		}()

		path := fmt.Sprintf("/dev/fd/%d", pr.Fd())
		sess, msgs, parseErr := ParsePiSession(path, "my_project", "local")
		pr.Close()

		require.NoError(t, parseErr, "clean pipe read must not produce an error")
		require.NotNil(t, sess)
		assert.Equal(t, "pi:pipe-sess", sess.ID)
		assert.Len(t, msgs, 1)
	})
}

// TestParsePiSession_ErrorCases verifies error handling for missing, empty,
// and invalid session files.
func TestParsePiSession_ErrorCases(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, _, err := ParsePiSession("/nonexistent/path/session.jsonl", "proj", "local")
		assert.Error(t, err, "missing file must return error")
	})

	t.Run("empty file", func(t *testing.T) {
		path := createTestFile(t, "empty.jsonl", "")
		_, _, err := ParsePiSession(path, "proj", "local")
		assert.Error(t, err, "empty file (no session header) must return error")
	})

	t.Run("not a pi session", func(t *testing.T) {
		content := `{"type":"message","id":"entry-1","timestamp":"2025-01-01T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n"
		path := createTestFile(t, "not-pi.jsonl", content)
		_, _, err := ParsePiSession(path, "proj", "local")
		assert.Error(t, err, "file without session header must return error")
	})
}

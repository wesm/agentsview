package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAmpParserTest(
	t *testing.T, content string,
) (*ParsedSession, []ParsedMessage, error) {
	t.Helper()
	path := createTestFile(t, "T-test.json", content)
	return ParseAmpSession(path, "local")
}

func TestParseAmpSession_Basic(t *testing.T) {
	threadID := "T-019ca26f-aaaa-bbbb-cccc-dddddddddddd"
	content := `{
		"v": 1,
		"id": "` + threadID + `",
		"created": 1704067200000,
		"title": "Migrate database schema",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Migrate the DB schema."}]},
			{"role": "assistant", "content": [{"type": "text", "text": "Sure, I will help."}]}
		],
		"env": {
			"initial": {
				"trees": [{"displayName": "myproject", "uri": "file:///home/user/myproject"}]
			}
		},
		"meta": {
			"traces": [
				{"name": "inference", "startTime": "2024-01-01T00:00:01Z", "endTime": "2024-01-01T00:00:02Z"},
				{"name": "tools",     "startTime": "2024-01-01T00:00:03Z", "endTime": "2024-01-01T00:00:05Z"}
			]
		}
	}`

	path := createTestFile(t, threadID+".json", content)
	sess, msgs, err := ParseAmpSession(path, "local")
	require.NoError(t, err)
	require.NotNil(t, sess)

	assertSessionMeta(t, sess,
		"amp:"+threadID,
		"myproject", AgentAmp,
	)

	// Title takes precedence as FirstMessage.
	assert.Equal(t, "Migrate database schema", sess.FirstMessage)
	assertMessageCount(t, sess.MessageCount, 2)
	assert.Equal(t, 1, sess.UserMessageCount)

	// Start time from created (epoch ms).
	wantStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assertTimestamp(t, sess.StartedAt, wantStart)

	// End time from meta.traces[last].endTime.
	wantEnd := time.Date(2024, 1, 1, 0, 0, 5, 0, time.UTC)
	assertTimestamp(t, sess.EndedAt, wantEnd)

	require.Equal(t, 2, len(msgs))
	assertMessage(t, msgs[0], RoleUser, "Migrate the DB schema.")
	assertMessage(t, msgs[1], RoleAssistant, "Sure, I will help.")
	assert.Equal(t, 0, msgs[0].Ordinal)
	assert.Equal(t, 1, msgs[1].Ordinal)
}

func TestParseAmpSession_ToolUseAndThinking(t *testing.T) {
	content := `{
		"v": 1,
		"id": "T-tooluse",
		"created": 1704067200000,
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Read the file."}]},
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "Let me plan this."},
				{"type": "text", "text": "I will read it now."},
				{"type": "tool_use", "complete": true, "id": "tu1", "name": "Read", "input": {"file_path": "main.go"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "tu1", "content": "package main"}
			]}
		],
		"env": {"initial": {"trees": [{"displayName": "myrepo"}]}},
		"meta": {"traces": []}
	}`

	sess, msgs, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	require.NotNil(t, sess)

	assertMessageCount(t, sess.MessageCount, 3)
	// tool_result-only user messages have no text content,
	// so only the first user message (with text) is counted.
	assert.Equal(t, 1, sess.UserMessageCount)

	assert.False(t, msgs[0].HasThinking)
	assert.False(t, msgs[0].HasToolUse)

	assert.True(t, msgs[1].HasThinking)
	assert.True(t, msgs[1].HasToolUse)
	assert.Contains(t, msgs[1].Content, "[Thinking]")
	assert.Contains(t, msgs[1].Content, "Let me plan this.")
	assert.Contains(t, msgs[1].Content, "[Read: main.go]")

	require.Equal(t, 1, len(msgs[1].ToolCalls))
	assert.Equal(t, "Read", msgs[1].ToolCalls[0].ToolName)
	assert.Equal(t, "Read", msgs[1].ToolCalls[0].Category)
	assert.Equal(t, "tu1", msgs[1].ToolCalls[0].ToolUseID)

	// tool_result message: content should be empty (no text blocks),
	// but tool results are recorded.
	assert.Equal(t, 1, len(msgs[2].ToolResults))
	assert.Equal(t, "tu1", msgs[2].ToolResults[0].ToolUseID)

	// EndTime absent (empty traces) → zero.
	assertZeroTimestamp(t, sess.EndedAt, "EndedAt")
}

func TestParseAmpSession_NoEnv(t *testing.T) {
	content := `{
		"v": 1,
		"id": "T-noenv",
		"created": 1704067200000,
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]}
		]
	}`

	sess, msgs, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	require.NotNil(t, sess)

	// No env → project falls back to "amp".
	assert.Equal(t, "amp", sess.Project)
	require.Equal(t, 1, len(msgs))
}

func TestParseAmpSession_NoTitle(t *testing.T) {
	content := `{
		"v": 1,
		"id": "T-notitle",
		"created": 1704067200000,
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Fix the bug in main.go please."}]},
			{"role": "assistant", "content": [{"type": "text", "text": "Done."}]}
		]
	}`

	sess, _, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	require.NotNil(t, sess)

	// No title → FirstMessage extracted from first user message.
	assert.Equal(t, "Fix the bug in main.go please.", sess.FirstMessage)
}

func TestParseAmpSession_NoMetaTraces(t *testing.T) {
	content := `{
		"v": 1,
		"id": "T-notraces",
		"created": 1704067200000,
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]}
		],
		"meta": {}
	}`

	sess, _, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	require.NotNil(t, sess)

	// No traces → EndedAt is zero.
	assertZeroTimestamp(t, sess.EndedAt, "EndedAt")
}

func TestParseAmpSession_LastTraceWithoutEndTime(t *testing.T) {
	content := `{
		"v": 1,
		"id": "T-trace-end-missing",
		"created": 1704067200000,
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]}
		],
		"meta": {
			"traces": [
				{"name": "inference", "endTime": "2024-01-01T00:00:02Z"},
				{"name": "tools"}
			]
		}
	}`

	sess, _, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Earlier trace endTime is used when the last trace has none.
	assert.Equal(t, "2024-01-01T00:00:02Z", sess.EndedAt.UTC().Format(time.RFC3339))
}

func TestParseAmpSession_EmptyThread(t *testing.T) {
	content := `{
		"v": 1,
		"id": "T-empty",
		"created": 1704067200000,
		"messages": []
	}`

	sess, msgs, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	// Empty thread → nil, nil, nil (non-interactive).
	assert.Nil(t, sess)
	assert.Nil(t, msgs)
}

func TestParseAmpSession_FirstMessageTruncation(t *testing.T) {
	longText := strings.Repeat("a", 400)
	content := `{"v":1,"id":"T-trunc","created":1704067200000,"messages":[` +
		`{"role":"user","content":[{"type":"text","text":"` + longText + `"}]}]}`

	sess, _, err := runAmpParserTest(t, content)
	require.NoError(t, err)
	require.NotNil(t, sess)
	// truncate clips at 300 chars + 3 ellipsis chars = 303.
	assert.Equal(t, 303, len(sess.FirstMessage))
}

func TestParseAmpSession_InvalidCreated(t *testing.T) {
	t.Run("missing created", func(t *testing.T) {
		content := `{
			"v": 1,
			"id": "T-missing-created",
			"messages": [
				{"role": "user", "content": [{"type": "text", "text": "hello"}]}
			]
		}`

		sess, _, err := runAmpParserTest(t, content)
		require.NoError(t, err)
		require.NotNil(t, sess)
		assertZeroTimestamp(t, sess.StartedAt, "StartedAt")
	})

	t.Run("non numeric created", func(t *testing.T) {
		content := `{
			"v": 1,
			"id": "T-invalid-created",
			"created": "nope",
			"messages": [
				{"role": "user", "content": [{"type": "text", "text": "hello"}]}
			]
		}`

		sess, _, err := runAmpParserTest(t, content)
		require.NoError(t, err)
		require.NotNil(t, sess)
		assertZeroTimestamp(t, sess.StartedAt, "StartedAt")
	})

	t.Run("non positive numeric created", func(t *testing.T) {
		cases := []struct {
			name    string
			created string
		}{
			{name: "zero", created: "0"},
			{name: "negative", created: "-1"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				content := `{
					"v": 1,
					"id": "T-invalid-created-numeric",
					"created": ` + tc.created + `,
					"messages": [
						{"role": "user", "content": [{"type": "text", "text": "hello"}]}
					]
				}`

				sess, _, err := runAmpParserTest(t, content)
				require.NoError(t, err)
				require.NotNil(t, sess)
				assertZeroTimestamp(t, sess.StartedAt, "StartedAt")
			})
		}
	})
}

func TestParseAmpSession_Errors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, _, err := ParseAmpSession("/nonexistent/T-xxx.json", "local")
		assert.Error(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, _, err := runAmpParserTest(t, "{not valid json")
		assert.Error(t, err)
	})

	t.Run("missing id falls back to filename", func(t *testing.T) {
		content := `{"v":1,"created":1704067200000,"messages":[` +
			`{"role":"user","content":[{"type":"text","text":"hello"}]}]}`
		sess, _, err := runAmpParserTest(t, content)
		require.NoError(t, err)
		require.NotNil(t, sess)
		// Falls back to filename-derived ID (T-test from T-test.json).
		assert.Equal(t, "amp:T-test", sess.ID)
	})

	t.Run("missing id and invalid filename", func(t *testing.T) {
		content := `{"v":1,"created":1704067200000,"messages":[]}`
		path := createTestFile(t, "bad-name.json", content)
		_, _, err := ParseAmpSession(path, "local")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing or invalid id")
	})
}

func TestParseAmpSession_MismatchedID(t *testing.T) {
	t.Run("invalid JSON id", func(t *testing.T) {
		content := `{
			"v": 1,
			"id": "bogus-not-a-thread-id",
			"created": 1704067200000,
			"messages": [
				{"role": "user", "content": [{"type": "text", "text": "hello"}]}
			]
		}`

		path := createTestFile(t, "T-fallback-uuid.json", content)
		sess, _, err := ParseAmpSession(path, "local")
		require.NoError(t, err)
		require.NotNil(t, sess)
		assert.Equal(t, "amp:T-fallback-uuid", sess.ID)
	})

	t.Run("valid JSON id differs from filename", func(t *testing.T) {
		// Both the JSON id and filename are valid T-<id>
		// patterns, but they disagree. Filename must win so
		// FindAmpSourceFile can locate the file by ID.
		content := `{
			"v": 1,
			"id": "T-from-json",
			"created": 1704067200000,
			"messages": [
				{"role": "user", "content": [{"type": "text", "text": "hello"}]}
			]
		}`

		path := createTestFile(t, "T-from-file.json", content)
		sess, _, err := ParseAmpSession(path, "local")
		require.NoError(t, err)
		require.NotNil(t, sess)
		assert.Equal(t, "amp:T-from-file", sess.ID)
	})
}

func TestAmpThreadID(t *testing.T) {
	data := []byte(`{"id":"T-abc123","v":1}`)
	assert.Equal(t, "T-abc123", AmpThreadID(data))

	assert.Equal(t, "", AmpThreadID([]byte(`{"v":1}`)))
}

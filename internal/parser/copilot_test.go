package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCopilotJSONL writes JSONL lines to a temp file and
// returns the file path.
func writeCopilotJSONL(
	t *testing.T, lines ...string,
) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(
		path, []byte(content), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseCopilotSession_Basic(t *testing.T) {
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"abc-123","context":{"cwd":"/home/alice/code/myproject","branch":"main"}},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"Fix the login bug"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"I'll fix the login bug."},"timestamp":"2025-01-15T10:00:02Z"}`,
	)

	sess, msgs, err := ParseCopilotSession(path, "test-machine")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}

	if sess.ID != "copilot:abc-123" {
		t.Errorf("session ID = %q, want %q",
			sess.ID, "copilot:abc-123")
	}
	if sess.Agent != AgentCopilot {
		t.Errorf("agent = %q, want %q",
			sess.Agent, AgentCopilot)
	}
	if sess.Machine != "test-machine" {
		t.Errorf("machine = %q, want %q",
			sess.Machine, "test-machine")
	}
	if sess.Project != "myproject" {
		t.Errorf("project = %q, want %q",
			sess.Project, "myproject")
	}
	if sess.FirstMessage != "Fix the login bug" {
		t.Errorf("first_message = %q, want %q",
			sess.FirstMessage, "Fix the login bug")
	}
	if sess.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2",
			sess.MessageCount)
	}

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("msgs[0].Role = %q, want %q",
			msgs[0].Role, RoleUser)
	}
	if msgs[1].Role != RoleAssistant {
		t.Errorf("msgs[1].Role = %q, want %q",
			msgs[1].Role, RoleAssistant)
	}
	if msgs[0].Content != "Fix the login bug" {
		t.Errorf("msgs[0].Content = %q, want %q",
			msgs[0].Content, "Fix the login bug")
	}
}

func TestParseCopilotSession_ToolCalls(t *testing.T) {
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"tool-test"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"Read the config file"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"tc-1","name":"view","arguments":"{\"path\":\"config.json\"}"}]},"timestamp":"2025-01-15T10:00:02Z"}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"tc-1","success":true,"result":"{\"key\":\"value\"}"},"timestamp":"2025-01-15T10:00:03Z"}`,
		`{"type":"assistant.message","data":{"content":"The config file contains a key-value pair."},"timestamp":"2025-01-15T10:00:04Z"}`,
	)

	sess, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}

	// Messages: user, assistant (tool call), user (tool result),
	// assistant (text).
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}

	// Check tool call message.
	tcMsg := msgs[1]
	if !tcMsg.HasToolUse {
		t.Error("expected HasToolUse on tool call message")
	}
	if len(tcMsg.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1",
			len(tcMsg.ToolCalls))
	}
	tc := tcMsg.ToolCalls[0]
	if tc.ToolName != "view" {
		t.Errorf("tool name = %q, want %q",
			tc.ToolName, "view")
	}
	if tc.Category != "Read" {
		t.Errorf("tool category = %q, want %q",
			tc.Category, "Read")
	}
	if tc.ToolUseID != "tc-1" {
		t.Errorf("tool use ID = %q, want %q",
			tc.ToolUseID, "tc-1")
	}
	// arguments was a stringified JSON string in the JSONL;
	// InputJSON should be the unwrapped JSON, not
	// double-encoded with enclosing quotes.
	wantInput := `{"path":"config.json"}`
	if tc.InputJSON != wantInput {
		t.Errorf("InputJSON = %q, want %q",
			tc.InputJSON, wantInput)
	}

	// Check tool result message.
	trMsg := msgs[2]
	if len(trMsg.ToolResults) != 1 {
		t.Fatalf("got %d tool results, want 1",
			len(trMsg.ToolResults))
	}
	if trMsg.ToolResults[0].ToolUseID != "tc-1" {
		t.Errorf("tool result ID = %q, want %q",
			trMsg.ToolResults[0].ToolUseID, "tc-1")
	}
	if got := trMsg.ToolResults[0].ContentLength; got != 15 {
		t.Errorf("tool result ContentLength = %d, want 15",
			got)
	}

	// Tool result message should carry the event timestamp.
	wantTS := parseTimestamp("2025-01-15T10:00:03Z")
	if trMsg.Timestamp != wantTS {
		t.Errorf("tool result timestamp = %v, want %v",
			trMsg.Timestamp, wantTS)
	}
}

func TestParseCopilotSession_ObjectToolResult(t *testing.T) {
	// result is a JSON object, not a string — Str would be
	// empty, but Raw gives us the length.
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"obj-result"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"list files"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"tc-2","name":"ls","arguments":"{}"}]},"timestamp":"2025-01-15T10:00:02Z"}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"tc-2","success":true,"result":{"files":["a.go","b.go"]}},"timestamp":"2025-01-15T10:00:03Z"}`,
		`{"type":"assistant.message","data":{"content":"Found 2 files."},"timestamp":"2025-01-15T10:00:04Z"}`,
	)

	_, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}

	trMsg := msgs[2]
	if len(trMsg.ToolResults) != 1 {
		t.Fatalf("got %d tool results, want 1",
			len(trMsg.ToolResults))
	}
	// {"files":["a.go","b.go"]} = 25 bytes
	if got := trMsg.ContentLength; got != 25 {
		t.Errorf(
			"ContentLength = %d, want 25 for object result",
			got,
		)
	}
	if got := trMsg.ToolResults[0].ContentLength; got != 25 {
		t.Errorf(
			"tool result ContentLength = %d, want 25 "+
				"for object result", got,
		)
	}
}

func TestParseCopilotSession_ArrayToolResult(t *testing.T) {
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"arr-result"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"get items"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"tc-3","name":"list","arguments":"{}"}]},"timestamp":"2025-01-15T10:00:02Z"}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"tc-3","success":true,"result":["one","two","three"]},"timestamp":"2025-01-15T10:00:03Z"}`,
		`{"type":"assistant.message","data":{"content":"Got 3 items."},"timestamp":"2025-01-15T10:00:04Z"}`,
	)

	_, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}

	trMsg := msgs[2]
	// ["one","two","three"] = 21 bytes
	if got := trMsg.ContentLength; got != 21 {
		t.Errorf(
			"ContentLength = %d, want 21 for array result",
			got,
		)
	}
	if got := trMsg.ToolResults[0].ContentLength; got != 21 {
		t.Errorf(
			"tool result ContentLength = %d, want 21 "+
				"for array result", got,
		)
	}
}

func TestParseCopilotSession_EmptyStringToolResult(
	t *testing.T,
) {
	// result is an explicit empty string — ContentLength
	// should be 0, not 2 (the length of `""`).
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"empty-str"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"run cmd"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"tc-4","name":"exec","arguments":"{}"}]},"timestamp":"2025-01-15T10:00:02Z"}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"tc-4","success":true,"result":""},"timestamp":"2025-01-15T10:00:03Z"}`,
		`{"type":"assistant.message","data":{"content":"Done."},"timestamp":"2025-01-15T10:00:04Z"}`,
	)

	_, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}

	trMsg := msgs[2]
	if got := trMsg.ContentLength; got != 0 {
		t.Errorf(
			"ContentLength = %d, want 0 for empty-string result",
			got,
		)
	}
	if got := trMsg.ToolResults[0].ContentLength; got != 0 {
		t.Errorf(
			"tool result ContentLength = %d, want 0 "+
				"for empty-string result", got,
		)
	}
}

func TestParseCopilotSession_Reasoning(t *testing.T) {
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"reason-test"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"Explain the bug"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"Here is my analysis.","reasoningText":"Let me think about this carefully..."},"timestamp":"2025-01-15T10:00:02Z"}`,
	)

	sess, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if !msgs[1].HasThinking {
		t.Error("expected HasThinking on assistant message " +
			"with reasoningText")
	}
}

func TestParseCopilotSession_AssistantReasoningEvent(
	t *testing.T,
) {
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"reason-event"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"Hello"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"Hi there."},"timestamp":"2025-01-15T10:00:02Z"}`,
		`{"type":"assistant.reasoning","data":{},"timestamp":"2025-01-15T10:00:03Z"}`,
	)

	_, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if !msgs[1].HasThinking {
		t.Error("expected HasThinking set by " +
			"assistant.reasoning event")
	}
}

func TestParseCopilotSession_DirectoryFormat(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "abc-456")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := strings.Join([]string{
		`{"type":"session.start","data":{"sessionId":"abc-456"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"hello"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"hi"},"timestamp":"2025-01-15T10:00:02Z"}`,
	}, "\n") + "\n"

	path := filepath.Join(sessDir, "events.jsonl")
	if err := os.WriteFile(
		path, []byte(content), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	sess, _, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if sess.ID != "copilot:abc-456" {
		t.Errorf("session ID = %q, want %q",
			sess.ID, "copilot:abc-456")
	}
}

func TestParseCopilotSession_DirectoryFormatFallbackID(
	t *testing.T,
) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "def-789")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// No session.start event, so ID comes from dir name.
	content := strings.Join([]string{
		`{"type":"user.message","data":{"content":"test"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"ok"},"timestamp":"2025-01-15T10:00:02Z"}`,
	}, "\n") + "\n"

	path := filepath.Join(sessDir, "events.jsonl")
	if err := os.WriteFile(
		path, []byte(content), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	sess, _, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if sess.ID != "copilot:def-789" {
		t.Errorf("session ID = %q, want %q",
			sess.ID, "copilot:def-789")
	}
}

func TestParseCopilotSession_EmptySession(t *testing.T) {
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"empty"},"timestamp":"2025-01-15T10:00:00Z"}`,
	)

	sess, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil session for empty, got %+v",
			sess)
	}
	if msgs != nil {
		t.Errorf("expected nil messages for empty, got %d",
			len(msgs))
	}
}

func TestParseCopilotSession_NonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.jsonl")

	sess, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sess != nil {
		t.Error("expected nil session for nonexistent file")
	}
	if msgs != nil {
		t.Error("expected nil messages for nonexistent file")
	}
}

func TestParseCopilotSession_ObjectArguments(t *testing.T) {
	// arguments is a native JSON object, not a string.
	path := writeCopilotJSONL(t,
		`{"type":"session.start","data":{"sessionId":"obj-args"},"timestamp":"2025-01-15T10:00:00Z"}`,
		`{"type":"user.message","data":{"content":"list"},"timestamp":"2025-01-15T10:00:01Z"}`,
		`{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"tc-5","name":"glob","arguments":{"pattern":"*.go"}}]},"timestamp":"2025-01-15T10:00:02Z"}`,
		`{"type":"assistant.message","data":{"content":"done"},"timestamp":"2025-01-15T10:00:03Z"}`,
	)

	_, msgs, err := ParseCopilotSession(path, "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc := msgs[1].ToolCalls[0]
	wantInput := `{"pattern":"*.go"}`
	if tc.InputJSON != wantInput {
		t.Errorf("InputJSON = %q, want %q",
			tc.InputJSON, wantInput)
	}
}

func TestSessionIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/tmp/abc-123.jsonl", "abc-123"},
		{"/tmp/abc-123/events.jsonl", "abc-123"},
		{"/tmp/foo/bar.jsonl", "bar"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := sessionIDFromPath(tt.path)
			if got != tt.want {
				t.Errorf("sessionIDFromPath(%q) = %q, want %q",
					tt.path, got, tt.want)
			}
		})
	}
}

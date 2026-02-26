package parser

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

// Timestamp constants for test data.
const (
	tsZero    = "2024-01-01T00:00:00Z"
	tsZeroS1  = "2024-01-01T00:00:01Z"
	tsZeroS2  = "2024-01-01T00:00:02Z"
	tsEarly   = "2024-01-01T10:00:00Z"
	tsEarlyS1 = "2024-01-01T10:00:01Z"
	tsEarlyS5 = "2024-01-01T10:00:05Z"
	tsLate    = "2024-01-01T10:01:00Z"
	tsLateS5  = "2024-01-01T10:01:05Z"
)

// Parsed time.Time values used as expected results in
// timestamp parsing tests.
var testJan15_1030UTC = time.Date(
	2024, 1, 15, 10, 30, 0, 0, time.UTC,
)

// --- Data Generators ---

func generateLargeString(size int) string {
	return strings.Repeat("x", size)
}

// --- Assertions ---

func assertSessionMeta(t *testing.T, s *ParsedSession, wantID, wantProject string, wantAgent AgentType) {
	t.Helper()
	if s == nil {
		t.Fatal("session is nil")
	}
	if s.ID != wantID {
		t.Errorf("session ID = %q, want %q", s.ID, wantID)
	}
	if s.Project != wantProject {
		t.Errorf("project = %q, want %q", s.Project, wantProject)
	}
	if s.Agent != wantAgent {
		t.Errorf("agent = %q, want %q", s.Agent, wantAgent)
	}
}

func assertMessage(t *testing.T, m ParsedMessage, wantRole RoleType, wantContentSnippet string) {
	t.Helper()
	if m.Role != wantRole {
		t.Errorf("role = %q, want %q", m.Role, wantRole)
	}
	if wantContentSnippet != "" && !strings.Contains(m.Content, wantContentSnippet) {
		t.Errorf("content missing snippet %q, got %q", wantContentSnippet, m.Content)
	}
}

func assertMessageCount(t *testing.T, count, want int) {
	t.Helper()
	if count != want {
		t.Fatalf("message count = %d, want %d", count, want)
	}
}

func assertTimestamp(t *testing.T, got time.Time, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("timestamp = %v, want %v", got, want)
	}
}

func assertZeroTimestamp(
	t *testing.T, ts time.Time, label string,
) {
	t.Helper()
	if !ts.IsZero() {
		t.Errorf("%s = %v, want zero", label, ts)
	}
}

// captureLog redirects log output to a buffer for the
// duration of the test and restores it on cleanup.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return &buf
}

func assertLogContains(
	t *testing.T, buf *bytes.Buffer, substrs ...string,
) {
	t.Helper()
	got := buf.String()
	var missing []string
	for _, s := range substrs {
		if !strings.Contains(got, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		t.Errorf("log missing substrings %q. Full log:\n%s", missing, got)
	}
}

func assertLogNotContains(
	t *testing.T, buf *bytes.Buffer, substrs ...string,
) {
	t.Helper()
	got := buf.String()
	var unexpected []string
	for _, s := range substrs {
		if strings.Contains(got, s) {
			unexpected = append(unexpected, s)
		}
	}
	if len(unexpected) > 0 {
		t.Errorf("log should not contain substrings %q. Full log:\n%s", unexpected, got)
	}
}

func assertLogEmpty(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	if buf.Len() > 0 {
		t.Errorf(
			"expected no log output, got: %q",
			buf.String(),
		)
	}
}

func assertToolCallField(t *testing.T, i int, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("tool_calls[%d].%s = %q, want %q", i, field, got, want)
	}
}

func assertToolCall(t *testing.T, i int, got, want ParsedToolCall) {
	t.Helper()
	assertToolCallField(t, i, "ToolName", got.ToolName, want.ToolName)
	assertToolCallField(t, i, "Category", got.Category, want.Category)
	if want.ToolUseID != "" {
		assertToolCallField(t, i, "ToolUseID", got.ToolUseID, want.ToolUseID)
	}
	if want.InputJSON != "" {
		assertToolCallField(t, i, "InputJSON", got.InputJSON, want.InputJSON)
	}
	if want.SkillName != "" {
		assertToolCallField(t, i, "SkillName", got.SkillName, want.SkillName)
	}
	assertToolCallField(t, i, "SubagentSessionID", got.SubagentSessionID, want.SubagentSessionID)
}

func assertToolCalls(
	t *testing.T, got, want []ParsedToolCall,
) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("tool calls count = %d, want %d",
			len(got), len(want))
		return
	}
	for i := range want {
		assertToolCall(t, i, got[i], want[i])
	}
}

func parseClaudeTestFile(
	t *testing.T, name, content, project string,
) (ParsedSession, []ParsedMessage) {
	t.Helper()
	path := createTestFile(t, name, content)
	results, err := ParseClaudeSession(
		path, project, "local",
	)
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("ParseClaudeSession returned no results")
	}
	return results[0].Session, results[0].Messages
}

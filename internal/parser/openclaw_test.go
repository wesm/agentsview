package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeOpenClawTestFile creates a test JSONL file inside an
// agent directory structure: <root>/<agentId>/sessions/<name>.jsonl.
// Returns the full path to the file and the root agents directory.
func writeOpenClawTestFile(
	t *testing.T, agentID string, lines ...string,
) (path, agentsDir string) {
	t.Helper()
	root := t.TempDir()
	sessDir := filepath.Join(root, agentID, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(sessDir, "test-session.jsonl")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	content := b.String()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path, root
}

func TestParseOpenClawSession_Basic(t *testing.T) {
	path, _ := writeOpenClawTestFile(t, "main",
		`{"type":"session","version":3,"id":"abc-123","timestamp":"2026-02-25T10:00:00Z","cwd":"/home/user/project"}`,
		`{"type":"model_change","id":"mc1","timestamp":"2026-02-25T10:00:00Z","provider":"anthropic","modelId":"claude-sonnet-4-6"}`,
		`{"type":"message","id":"m1","timestamp":"2026-02-25T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"Hello, how are you?"}],"timestamp":"2026-02-25T10:00:01Z"}}`,
		`{"type":"message","id":"m2","timestamp":"2026-02-25T10:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"I'm doing well, thanks!"}],"timestamp":"2026-02-25T10:00:02Z"}}`,
	)

	sess, msgs, err := ParseOpenClawSession(path, "", "test-machine")
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}

	if sess.ID != "openclaw:main:abc-123" {
		t.Errorf("expected ID openclaw:main:abc-123, got %s", sess.ID)
	}
	if sess.Agent != AgentOpenClaw {
		t.Errorf("expected agent openclaw, got %s", sess.Agent)
	}
	if sess.Machine != "test-machine" {
		t.Errorf("expected machine test-machine, got %s", sess.Machine)
	}
	if sess.Project != "project" {
		t.Errorf("expected project 'project', got %s", sess.Project)
	}
	if sess.FirstMessage != "Hello, how are you?" {
		t.Errorf("expected first message 'Hello, how are you?', got %s", sess.FirstMessage)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != RoleUser {
		t.Errorf("expected first role user, got %s", msgs[0].Role)
	}
	if msgs[1].Role != RoleAssistant {
		t.Errorf("expected second role assistant, got %s", msgs[1].Role)
	}
	if sess.UserMessageCount != 1 {
		t.Errorf("expected 1 user message, got %d", sess.UserMessageCount)
	}
}

func TestParseOpenClawSession_Thinking(t *testing.T) {
	path, _ := writeOpenClawTestFile(t, "main",
		`{"type":"session","version":3,"id":"think-123","timestamp":"2026-02-25T10:00:00Z","cwd":"/tmp"}`,
		`{"type":"message","id":"m1","timestamp":"2026-02-25T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"Think about this"}],"timestamp":"2026-02-25T10:00:01Z"}}`,
		`{"type":"message","id":"m2","timestamp":"2026-02-25T10:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Let me consider..."},{"type":"text","text":"Here is my response."}],"timestamp":"2026-02-25T10:00:02Z"}}`,
	)

	_, msgs, err := ParseOpenClawSession(path, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !msgs[1].HasThinking {
		t.Error("expected HasThinking=true for assistant message")
	}
}

func TestParseOpenClawSession_ToolResult(t *testing.T) {
	path, _ := writeOpenClawTestFile(t, "main",
		`{"type":"session","version":3,"id":"tool-123","timestamp":"2026-02-25T10:00:00Z","cwd":"/tmp"}`,
		`{"type":"message","id":"m1","timestamp":"2026-02-25T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"Read a file"}],"timestamp":"2026-02-25T10:00:01Z"}}`,
		`{"type":"message","id":"m2","timestamp":"2026-02-25T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"read","input":{"path":"/etc/hosts"}}],"timestamp":"2026-02-25T10:00:02Z"}}`,
		`{"type":"message","id":"m3","timestamp":"2026-02-25T10:00:03Z","message":{"role":"toolResult","toolCallId":"tu1","toolName":"read","content":[{"type":"text","text":"127.0.0.1 localhost"}],"isError":false,"timestamp":"2026-02-25T10:00:03Z"}}`,
		`{"type":"message","id":"m4","timestamp":"2026-02-25T10:00:04Z","message":{"role":"assistant","content":[{"type":"text","text":"The hosts file contains localhost."}],"timestamp":"2026-02-25T10:00:04Z"}}`,
	)

	sess, msgs, err := ParseOpenClawSession(path, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	// Assistant with tool_use
	if !msgs[1].HasToolUse {
		t.Error("expected HasToolUse=true for tool-use message")
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msgs[1].ToolCalls))
	}
	if msgs[1].ToolCalls[0].ToolName != "read" {
		t.Errorf("expected tool name 'read', got %s", msgs[1].ToolCalls[0].ToolName)
	}
	if msgs[1].ToolCalls[0].Category != "Read" {
		t.Errorf("expected category 'Read', got %s", msgs[1].ToolCalls[0].Category)
	}

	// Tool result mapped to user role
	if msgs[2].Role != RoleUser {
		t.Errorf("expected tool result as user role, got %s", msgs[2].Role)
	}
	if len(msgs[2].ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(msgs[2].ToolResults))
	}
	if msgs[2].ToolResults[0].ToolUseID != "tu1" {
		t.Errorf("expected tool use ID 'tu1', got %s", msgs[2].ToolResults[0].ToolUseID)
	}
	if sess.MessageCount != 4 {
		t.Errorf("expected 4 messages, got %d", sess.MessageCount)
	}

	// UserMessageCount should only count the real user message,
	// not the synthetic tool-result message.
	if sess.UserMessageCount != 1 {
		t.Errorf("expected UserMessageCount 1 (tool results excluded), got %d", sess.UserMessageCount)
	}
}

func TestParseOpenClawSession_EmptyFile(t *testing.T) {
	path, _ := writeOpenClawTestFile(t, "main",
		`{"type":"session","version":3,"id":"empty","timestamp":"2026-02-25T10:00:00Z","cwd":"/tmp"}`,
	)

	sess, _, err := ParseOpenClawSession(path, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if sess != nil {
		t.Error("expected nil session for file with no messages")
	}
}

func TestParseOpenClawSession_Compaction(t *testing.T) {
	path, _ := writeOpenClawTestFile(t, "main",
		`{"type":"session","version":3,"id":"compact","timestamp":"2026-02-25T10:00:00Z","cwd":"/tmp"}`,
		`{"type":"compaction","id":"c1","timestamp":"2026-02-25T10:00:01Z","summary":"Previous work summary"}`,
		`{"type":"message","id":"m1","timestamp":"2026-02-25T10:00:02Z","message":{"role":"user","content":[{"type":"text","text":"Continue from here"}],"timestamp":"2026-02-25T10:00:02Z"}}`,
		`{"type":"message","id":"m2","timestamp":"2026-02-25T10:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"Continuing..."}],"timestamp":"2026-02-25T10:00:03Z"}}`,
	)

	sess, msgs, err := ParseOpenClawSession(path, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	// Compaction should be skipped, only messages remain.
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (compaction skipped), got %d", len(msgs))
	}
}

func TestParseOpenClawSession_AgentIDInSessionID(t *testing.T) {
	// Verify different agent subdirectories produce distinct
	// session IDs even when the raw session ID is the same.
	pathA, _ := writeOpenClawTestFile(t, "alpha",
		`{"type":"session","version":3,"id":"same-id","timestamp":"2026-02-25T10:00:00Z","cwd":"/tmp"}`,
		`{"type":"message","id":"m1","timestamp":"2026-02-25T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}],"timestamp":"2026-02-25T10:00:01Z"}}`,
	)
	pathB, _ := writeOpenClawTestFile(t, "beta",
		`{"type":"session","version":3,"id":"same-id","timestamp":"2026-02-25T10:00:00Z","cwd":"/tmp"}`,
		`{"type":"message","id":"m1","timestamp":"2026-02-25T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"Hello"}],"timestamp":"2026-02-25T10:00:01Z"}}`,
	)

	sessA, _, err := ParseOpenClawSession(pathA, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	sessB, _, err := ParseOpenClawSession(pathB, "", "test")
	if err != nil {
		t.Fatal(err)
	}

	if sessA.ID == sessB.ID {
		t.Errorf("expected different session IDs for different agents, both got %s", sessA.ID)
	}
	if sessA.ID != "openclaw:alpha:same-id" {
		t.Errorf("expected openclaw:alpha:same-id, got %s", sessA.ID)
	}
	if sessB.ID != "openclaw:beta:same-id" {
		t.Errorf("expected openclaw:beta:same-id, got %s", sessB.ID)
	}
}

func TestDiscoverOpenClawSessions(t *testing.T) {
	// Build a mock directory structure:
	// <root>/main/sessions/sess1.jsonl
	// <root>/main/sessions/sessions.json
	// <root>/claude/sessions/sess2.jsonl
	root := t.TempDir()

	mainSessions := filepath.Join(root, "main", "sessions")
	if err := os.MkdirAll(mainSessions, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainSessions, "sess1.jsonl"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainSessions, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	claudeSessions := filepath.Join(root, "claude", "sessions")
	if err := os.MkdirAll(claudeSessions, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeSessions, "sess2.jsonl"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	files := DiscoverOpenClawSessions(root)
	if len(files) != 2 {
		t.Fatalf("expected 2 session files, got %d", len(files))
	}
	for _, f := range files {
		if f.Agent != AgentOpenClaw {
			t.Errorf("expected agent openclaw, got %s", f.Agent)
		}
	}
}

func TestFindOpenClawSourceFile(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(root, "main", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sessDir, "abc-123.jsonl")
	if err := os.WriteFile(target, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Raw ID is now "agentId:sessionId".
	found := FindOpenClawSourceFile(root, "main:abc-123")
	if found != target {
		t.Errorf("expected %s, got %s", target, found)
	}

	// Non-existent session.
	notFound := FindOpenClawSourceFile(root, "main:nonexistent")
	if notFound != "" {
		t.Errorf("expected empty string, got %s", notFound)
	}

	// Non-existent agent.
	notFound2 := FindOpenClawSourceFile(root, "other:abc-123")
	if notFound2 != "" {
		t.Errorf("expected empty string, got %s", notFound2)
	}

	// Invalid format (no colon separator).
	notFound3 := FindOpenClawSourceFile(root, "abc-123")
	if notFound3 != "" {
		t.Errorf("expected empty string for bare ID, got %s", notFound3)
	}
}

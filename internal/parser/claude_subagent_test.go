// ABOUTME: Tests for queue-operation parsing that maps Task tool_use IDs to subagent sessions.
// ABOUTME: Validates that ParsedToolCall entries get annotated with SubagentSessionID.
package parser

import (
	"strings"
	"testing"
)

func TestSubagentSessionIDMapping(t *testing.T) {
	lines := []string{
		// user entry with uuid
		`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
		// assistant with Task tool_use
		`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_abc123","name":"Task","input":{"description":"do stuff","subagent_type":"general-purpose","prompt":"test"}}]}}`,
		// queue-operation enqueue linking toolu_abc123 -> task_id deadbeef123
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"deadbeef123\",\"tool_use_id\":\"toolu_abc123\",\"description\":\"do stuff\",\"task_type\":\"local_agent\"}"}`,
		// user with tool_result
		`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_abc123","content":"done"}]}}`,
	}
	content := strings.Join(lines, "\n")

	path := createTestFile(t, "subagent-map.jsonl", content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	// Find the Task tool call and check SubagentSessionID.
	found := false
	for _, msg := range results[0].Messages {
		for _, tc := range msg.ToolCalls {
			if tc.ToolName == "Task" {
				found = true
				want := "agent-deadbeef123"
				if tc.SubagentSessionID != want {
					t.Errorf("SubagentSessionID = %q, want %q", tc.SubagentSessionID, want)
				}
			}
		}
	}
	if !found {
		t.Error("Task tool call not found in messages")
	}
}

func TestSubagentSessionIDMapping_NoUUIDs(t *testing.T) {
	// Same test but without UUIDs to exercise the linear parsing path.
	lines := []string{
		`{"type":"user","timestamp":"2024-01-01T10:00:00Z","message":{"content":"hello"},"cwd":"/tmp"}`,
		`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","message":{"content":[{"type":"tool_use","id":"toolu_xyz789","name":"Task","input":{"description":"research","subagent_type":"researcher","prompt":"look stuff up"}}]}}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"cafebabe\",\"tool_use_id\":\"toolu_xyz789\",\"description\":\"research\",\"task_type\":\"local_agent\"}"}`,
		`{"type":"user","timestamp":"2024-01-01T10:00:05Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_xyz789","content":"done"}]}}`,
	}
	content := strings.Join(lines, "\n")

	path := createTestFile(t, "subagent-linear.jsonl", content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	found := false
	for _, msg := range results[0].Messages {
		for _, tc := range msg.ToolCalls {
			if tc.ToolName == "Task" {
				found = true
				want := "agent-cafebabe"
				if tc.SubagentSessionID != want {
					t.Errorf("SubagentSessionID = %q, want %q", tc.SubagentSessionID, want)
				}
			}
		}
	}
	if !found {
		t.Error("Task tool call not found in messages")
	}
}

func TestSubagentSessionIDMapping_NonTaskToolUnchanged(t *testing.T) {
	// Non-Task tool calls should not get SubagentSessionID even if
	// their tool_use_id appears in a queue-operation.
	lines := []string{
		`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
		`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_read1","name":"Read","input":{"file_path":"/tmp/foo.txt"}}]}}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"deadbeef\",\"tool_use_id\":\"toolu_read1\",\"description\":\"test\",\"task_type\":\"local_agent\"}"}`,
		`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_read1","content":"file contents"}]}}`,
	}
	content := strings.Join(lines, "\n")

	path := createTestFile(t, "subagent-nontask.jsonl", content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	for _, msg := range results[0].Messages {
		for _, tc := range msg.ToolCalls {
			if tc.SubagentSessionID != "" {
				t.Errorf("non-Task tool %q got SubagentSessionID = %q, want empty",
					tc.ToolName, tc.SubagentSessionID)
			}
		}
	}
}

func TestSubagentSessionIDMapping_XMLContent(t *testing.T) {
	// XML-format queue-operation entries use <task-id> and <tool-use-id> tags
	// instead of JSON content.
	lines := []string{
		`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
		`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_01CuRUbKy9rSQUo2Beu9xjLu","name":"Task","input":{"description":"do stuff","subagent_type":"general-purpose","prompt":"test"}}]}}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"<task-notification>\n<task-id>a02eb277c065b35a2</task-id>\n<tool-use-id>toolu_01CuRUbKy9rSQUo2Beu9xjLu</tool-use-id>\n<status>completed</status>\n<summary>Agent completed</summary>\n</task-notification>"}`,
		`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_01CuRUbKy9rSQUo2Beu9xjLu","content":"done"}]}}`,
	}
	content := strings.Join(lines, "\n")

	path := createTestFile(t, "subagent-xml.jsonl", content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	found := false
	for _, msg := range results[0].Messages {
		for _, tc := range msg.ToolCalls {
			if tc.ToolName == "Task" {
				found = true
				want := "agent-a02eb277c065b35a2"
				if tc.SubagentSessionID != want {
					t.Errorf("SubagentSessionID = %q, want %q", tc.SubagentSessionID, want)
				}
			}
		}
	}
	if !found {
		t.Error("Task tool call not found in messages")
	}
}

func TestSubagentSessionIDMapping_XMLContent_NoUUIDs(t *testing.T) {
	// XML-format queue-operation entries through the linear parsing path.
	lines := []string{
		`{"type":"user","timestamp":"2024-01-01T10:00:00Z","message":{"content":"hello"},"cwd":"/tmp"}`,
		`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","message":{"content":[{"type":"tool_use","id":"toolu_01XYZ","name":"Task","input":{"description":"research","subagent_type":"researcher","prompt":"look stuff up"}}]}}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"<task-notification>\n<task-id>beef4567</task-id>\n<tool-use-id>toolu_01XYZ</tool-use-id>\n<status>running</status>\n</task-notification>"}`,
		`{"type":"user","timestamp":"2024-01-01T10:00:05Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_01XYZ","content":"done"}]}}`,
	}
	content := strings.Join(lines, "\n")

	path := createTestFile(t, "subagent-xml-linear.jsonl", content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	found := false
	for _, msg := range results[0].Messages {
		for _, tc := range msg.ToolCalls {
			if tc.ToolName == "Task" {
				found = true
				want := "agent-beef4567"
				if tc.SubagentSessionID != want {
					t.Errorf("SubagentSessionID = %q, want %q", tc.SubagentSessionID, want)
				}
			}
		}
	}
	if !found {
		t.Error("Task tool call not found in messages")
	}
}

func TestSubagentSessionIDMapping_MultipleSubagents(t *testing.T) {
	// Multiple Task tool calls each linked to different subagent sessions.
	lines := []string{
		`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
		`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_a","name":"Task","input":{"description":"task A","subagent_type":"general-purpose","prompt":"A"}},{"type":"tool_use","id":"toolu_b","name":"Task","input":{"description":"task B","subagent_type":"researcher","prompt":"B"}}]}}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"aaa111\",\"tool_use_id\":\"toolu_a\",\"description\":\"task A\",\"task_type\":\"local_agent\"}"}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"bbb222\",\"tool_use_id\":\"toolu_b\",\"description\":\"task B\",\"task_type\":\"local_agent\"}"}`,
		`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"done A"},{"type":"tool_result","tool_use_id":"toolu_b","content":"done B"}]}}`,
	}
	content := strings.Join(lines, "\n")

	path := createTestFile(t, "subagent-multi.jsonl", content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	wantMap := map[string]string{
		"toolu_a": "agent-aaa111",
		"toolu_b": "agent-bbb222",
	}

	foundCount := 0
	for _, msg := range results[0].Messages {
		for _, tc := range msg.ToolCalls {
			if tc.ToolName == "Task" {
				foundCount++
				want := wantMap[tc.ToolUseID]
				if tc.SubagentSessionID != want {
					t.Errorf("Task %s SubagentSessionID = %q, want %q",
						tc.ToolUseID, tc.SubagentSessionID, want)
				}
			}
		}
	}
	if foundCount != 2 {
		t.Errorf("found %d Task tool calls, want 2", foundCount)
	}
}

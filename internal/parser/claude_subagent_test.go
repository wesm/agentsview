// ABOUTME: Tests for queue-operation parsing that maps Task tool_use IDs to subagent sessions.
// ABOUTME: Validates that ParsedToolCall entries get annotated with SubagentSessionID.
package parser

import (
	"strings"
	"testing"
)

// parseAndGetToolCalls is a helper function that takes test lines, runs the parser,
// and returns the flattened tool calls.
func parseAndGetToolCalls(t *testing.T, filename string, lines []string) []ParsedToolCall {
	t.Helper()
	content := strings.Join(lines, "\n")
	path := createTestFile(t, filename, content)
	results, err := ParseClaudeSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}

	var toolCalls []ParsedToolCall
	for _, msg := range results[0].Messages {
		toolCalls = append(toolCalls, msg.ToolCalls...)
	}
	return toolCalls
}

func TestSubagentSessionIDMapping(t *testing.T) {
	tests := []struct {
		name             string
		lines            []string
		wantMap          map[string]string // maps ToolUseID to expected SubagentSessionID
		wantTask         int               // expected number of Task tools
		wantEmptyNonTask bool              // if true, verifies non-Task tools have empty SubagentSessionID
	}{
		{
			name: "Basic Mapping",
			lines: []string{
				// user entry with uuid
				`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
				// assistant with Task tool_use
				`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_abc123","name":"Task","input":{"description":"do stuff","subagent_type":"general-purpose","prompt":"test"}}]}}`,
				// queue-operation enqueue linking toolu_abc123 -> task_id deadbeef123
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"deadbeef123\",\"tool_use_id\":\"toolu_abc123\",\"description\":\"do stuff\",\"task_type\":\"local_agent\"}"}`,
				// user with tool_result
				`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_abc123","content":"done"}]}}`,
			},
			wantMap:  map[string]string{"toolu_abc123": "agent-deadbeef123"},
			wantTask: 1,
		},
		{
			name: "No UUIDs",
			lines: []string{
				`{"type":"user","timestamp":"2024-01-01T10:00:00Z","message":{"content":"hello"},"cwd":"/tmp"}`,
				`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","message":{"content":[{"type":"tool_use","id":"toolu_xyz789","name":"Task","input":{"description":"research","subagent_type":"researcher","prompt":"look stuff up"}}]}}`,
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"cafebabe\",\"tool_use_id\":\"toolu_xyz789\",\"description\":\"research\",\"task_type\":\"local_agent\"}"}`,
				`{"type":"user","timestamp":"2024-01-01T10:00:05Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_xyz789","content":"done"}]}}`,
			},
			wantMap:  map[string]string{"toolu_xyz789": "agent-cafebabe"},
			wantTask: 1,
		},
		{
			name: "Non-Task Tool Unchanged",
			lines: []string{
				`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
				`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_read1","name":"Read","input":{"file_path":"/tmp/foo.txt"}}]}}`,
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"deadbeef\",\"tool_use_id\":\"toolu_read1\",\"description\":\"test\",\"task_type\":\"local_agent\"}"}`,
				`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_read1","content":"file contents"}]}}`,
			},
			wantMap:          map[string]string{},
			wantTask:         0,
			wantEmptyNonTask: true,
		},
		{
			name: "XML Content",
			lines: []string{
				`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
				`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_01CuRUbKy9rSQUo2Beu9xjLu","name":"Task","input":{"description":"do stuff","subagent_type":"general-purpose","prompt":"test"}}]}}`,
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"<task-notification>\n<task-id>a02eb277c065b35a2</task-id>\n<tool-use-id>toolu_01CuRUbKy9rSQUo2Beu9xjLu</tool-use-id>\n<status>completed</status>\n<summary>Agent completed</summary>\n</task-notification>"}`,
				`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_01CuRUbKy9rSQUo2Beu9xjLu","content":"done"}]}}`,
			},
			wantMap:  map[string]string{"toolu_01CuRUbKy9rSQUo2Beu9xjLu": "agent-a02eb277c065b35a2"},
			wantTask: 1,
		},
		{
			name: "XML Content No UUIDs",
			lines: []string{
				`{"type":"user","timestamp":"2024-01-01T10:00:00Z","message":{"content":"hello"},"cwd":"/tmp"}`,
				`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","message":{"content":[{"type":"tool_use","id":"toolu_01XYZ","name":"Task","input":{"description":"research","subagent_type":"researcher","prompt":"look stuff up"}}]}}`,
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"<task-notification>\n<task-id>beef4567</task-id>\n<tool-use-id>toolu_01XYZ</tool-use-id>\n<status>running</status>\n</task-notification>"}`,
				`{"type":"user","timestamp":"2024-01-01T10:00:05Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_01XYZ","content":"done"}]}}`,
			},
			wantMap:  map[string]string{"toolu_01XYZ": "agent-beef4567"},
			wantTask: 1,
		},
		{
			name: "Multiple Subagents",
			lines: []string{
				`{"type":"user","timestamp":"2024-01-01T10:00:00Z","uuid":"u1","message":{"content":"hello"},"cwd":"/tmp"}`,
				`{"type":"assistant","timestamp":"2024-01-01T10:00:01Z","uuid":"u2","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_a","name":"Task","input":{"description":"task A","subagent_type":"general-purpose","prompt":"A"}},{"type":"tool_use","id":"toolu_b","name":"Task","input":{"description":"task B","subagent_type":"researcher","prompt":"B"}}]}}`,
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"aaa111\",\"tool_use_id\":\"toolu_a\",\"description\":\"task A\",\"task_type\":\"local_agent\"}"}`,
				`{"type":"queue-operation","operation":"enqueue","timestamp":"2024-01-01T10:00:01Z","sessionId":"test-session","content":"{\"task_id\":\"bbb222\",\"tool_use_id\":\"toolu_b\",\"description\":\"task B\",\"task_type\":\"local_agent\"}"}`,
				`{"type":"user","timestamp":"2024-01-01T10:00:05Z","uuid":"u3","parentUuid":"u2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_a","content":"done A"},{"type":"tool_result","tool_use_id":"toolu_b","content":"done B"}]}}`,
			},
			wantMap: map[string]string{
				"toolu_a": "agent-aaa111",
				"toolu_b": "agent-bbb222",
			},
			wantTask: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a safe filename for the test
			filename := strings.ReplaceAll(tt.name, " ", "_") + ".jsonl"
			toolCalls := parseAndGetToolCalls(t, filename, tt.lines)

			if tt.wantEmptyNonTask {
				for _, tc := range toolCalls {
					if tc.SubagentSessionID != "" {
						t.Errorf("non-Task tool %q got SubagentSessionID = %q, want empty",
							tc.ToolName, tc.SubagentSessionID)
					}
				}
				return
			}

			taskCount := 0
			for _, tc := range toolCalls {
				if tc.ToolName == "Task" {
					taskCount++
					want := tt.wantMap[tc.ToolUseID]
					if tc.SubagentSessionID != want {
						t.Errorf("SubagentSessionID = %q, want %q", tc.SubagentSessionID, want)
					}
				}
			}
			if taskCount != tt.wantTask {
				t.Errorf("found %d Task tool calls, want %d", taskCount, tt.wantTask)
			}
		})
	}
}

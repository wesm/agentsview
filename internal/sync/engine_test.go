// ABOUTME: Tests for sync engine helper functions.
// ABOUTME: Covers pairToolResults and related conversion logic.
package sync

import (
	"testing"

	"github.com/wesm/agentsview/internal/db"
)

func TestPairToolResults(t *testing.T) {
	tests := []struct {
		name string
		msgs []db.Message
		want map[string]int // tool_use_id → expected ResultContentLength
	}{
		{
			"basic pairing across messages",
			[]db.Message{
				{ToolCalls: []db.ToolCall{
					{ToolUseID: "t1", ToolName: "Read"},
					{ToolUseID: "t2", ToolName: "Grep"},
				}},
				{ToolResults: []db.ToolResult{
					{ToolUseID: "t1", ContentLength: 100},
					{ToolUseID: "t2", ContentLength: 200},
				}},
			},
			map[string]int{"t1": 100, "t2": 200},
		},
		{
			"unmatched tool_result ignored",
			[]db.Message{
				{ToolCalls: []db.ToolCall{
					{ToolUseID: "t1", ToolName: "Read"},
				}},
				{ToolResults: []db.ToolResult{
					{ToolUseID: "t1", ContentLength: 50},
					{ToolUseID: "t_unknown", ContentLength: 999},
				}},
			},
			map[string]int{"t1": 50},
		},
		{
			"unmatched tool_call keeps zero",
			[]db.Message{
				{ToolCalls: []db.ToolCall{
					{ToolUseID: "t1", ToolName: "Read"},
					{ToolUseID: "t2", ToolName: "Bash"},
				}},
				{ToolResults: []db.ToolResult{
					{ToolUseID: "t1", ContentLength: 42},
				}},
			},
			map[string]int{"t1": 42, "t2": 0},
		},
		{
			"empty messages",
			nil,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pairToolResults(tt.msgs)
			for _, m := range tt.msgs {
				for _, tc := range m.ToolCalls {
					if expected, ok := tt.want[tc.ToolUseID]; ok {
						if tc.ResultContentLength != expected {
							t.Errorf("ToolCall %q: ResultContentLength = %d, want %d",
								tc.ToolUseID, tc.ResultContentLength, expected)
						}
					}
				}
			}
		})
	}
}

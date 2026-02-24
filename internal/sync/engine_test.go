// ABOUTME: Tests for sync engine helper functions.
// ABOUTME: Covers pairToolResults and related conversion logic.
package sync

import (
	"testing"

	"github.com/wesm/agentsview/internal/db"
)

func TestFilterEmptyMessages(t *testing.T) {
	tests := []struct {
		name     string
		msgs     []db.Message
		wantLen  int
		wantPair map[string]int // tool_use_id → expected ResultContentLength
	}{
		{
			"removes empty-content user message after pairing",
			[]db.Message{
				{
					Role:    "assistant",
					Content: "Let me read the file.",
					ToolCalls: []db.ToolCall{
						{ToolUseID: "t1", ToolName: "Read"},
					},
				},
				{
					Role:    "user",
					Content: "",
					ToolResults: []db.ToolResult{
						{ToolUseID: "t1", ContentLength: 500},
					},
				},
			},
			1, // only assistant message remains
			map[string]int{"t1": 500},
		},
		{
			"keeps user message with real content",
			[]db.Message{
				{
					Role:    "assistant",
					Content: "Here is the result.",
					ToolCalls: []db.ToolCall{
						{ToolUseID: "t1", ToolName: "Bash"},
					},
				},
				{
					Role:    "user",
					Content: "",
					ToolResults: []db.ToolResult{
						{ToolUseID: "t1", ContentLength: 100},
					},
				},
				{
					Role:    "user",
					Content: "Thanks, now do something else.",
				},
			},
			2, // assistant + user with content
			map[string]int{"t1": 100},
		},
		{
			"whitespace-only content treated as empty",
			[]db.Message{
				{
					Role:    "assistant",
					Content: "Reading...",
					ToolCalls: []db.ToolCall{
						{ToolUseID: "t1", ToolName: "Read"},
					},
				},
				{
					Role:    "user",
					Content: "   \n\t  ",
					ToolResults: []db.ToolResult{
						{ToolUseID: "t1", ContentLength: 300},
					},
				},
			},
			1,
			map[string]int{"t1": 300},
		},
		{
			"preserves empty assistant message",
			[]db.Message{
				{
					Role:    "assistant",
					Content: "",
				},
			},
			1,
			nil,
		},
		{
			"only removes user messages with tool results",
			[]db.Message{
				{
					Role:    "assistant",
					Content: "",
				},
				{
					Role:    "user",
					Content: "",
				},
			},
			2, // both preserved: no ToolResults
			nil,
		},
		{
			"no messages returns empty",
			nil,
			0,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pairAndFilter(tt.msgs)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			for _, m := range got {
				for _, tc := range m.ToolCalls {
					if expected, ok := tt.wantPair[tc.ToolUseID]; ok {
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

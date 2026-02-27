package parser

import (
	"testing"
)

func TestExtractAssistantContent(t *testing.T) {
	tests := []struct {
		name          string
		lines         []string
		wantText      string
		wantThinking  bool
		wantToolCount int
	}{
		{
			name: "plain text",
			lines: []string{
				"Hello, how can I help?",
			},
			wantText: "Hello, how can I help?",
		},
		{
			name: "thinking then text",
			lines: []string{
				"[Thinking]",
				"  internal reasoning...",
				"Here is my answer.",
			},
			wantText:     "Here is my answer.",
			wantThinking: true,
		},
		{
			name: "tool call then prose",
			lines: []string{
				"[Tool call] EditFile",
				"  path=/tmp/foo.go",
				"  content=bar",
				"I edited the file for you.",
			},
			wantText:      "I edited the file for you.",
			wantToolCount: 1,
		},
		{
			name: "tool result then prose",
			lines: []string{
				"[Tool result]",
				"  success: true",
				"The operation completed.",
			},
			wantText: "The operation completed.",
		},
		{
			name: "thinking, tool, prose sequence",
			lines: []string{
				"[Thinking]",
				"  let me think...",
				"[Tool call] Shell",
				"  command=ls",
				"[Tool result]",
				"  file1.go",
				"Here are the files I found.",
			},
			wantText:      "Here are the files I found.",
			wantThinking:  true,
			wantToolCount: 1,
		},
		{
			name: "prose between markers",
			lines: []string{
				"First I'll check the file.",
				"[Tool call] ReadFile",
				"  path=main.go",
				"[Tool result]",
				"  package main",
				"The file looks good.",
				"[Tool call] Shell",
				"  command=go build",
				"Build succeeded.",
			},
			wantText: "First I'll check the file.\n" +
				"The file looks good.\n" +
				"Build succeeded.",
			wantToolCount: 2,
		},
		{
			name:  "empty lines",
			lines: []string{},
		},
		{
			name: "only markers no prose",
			lines: []string{
				"[Thinking]",
				"  reasoning",
				"[Tool call] Shell",
				"  command=echo hi",
			},
			wantThinking:  true,
			wantToolCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, hasThinking, toolCalls := extractAssistantContent(
				tt.lines,
			)
			if text != tt.wantText {
				t.Errorf(
					"text = %q, want %q", text, tt.wantText,
				)
			}
			if hasThinking != tt.wantThinking {
				t.Errorf(
					"hasThinking = %v, want %v",
					hasThinking, tt.wantThinking,
				)
			}
			if len(toolCalls) != tt.wantToolCount {
				t.Errorf(
					"tool call count = %d, want %d",
					len(toolCalls), tt.wantToolCount,
				)
			}
		})
	}
}

func TestIsContainedIn_EdgeCases(t *testing.T) {
	// isContainedIn is in sync/discovery.go; we test
	// isBlockBodyEnd here since it's in cursor.go.
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"marker", "[Tool call] Shell", true},
		{"indented", "  param=value", false},
		{"tab indented", "\tparam=value", false},
		{"empty", "", false},
		{"left-margin prose", "Here is text.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBlockBodyEnd(tt.line)
			if got != tt.want {
				t.Errorf(
					"isBlockBodyEnd(%q) = %v, want %v",
					tt.line, got, tt.want,
				)
			}
		})
	}
}

func TestDecodeCursorProjectDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{
			"Users-fiona-Documents-project",
			"project",
		},
		{
			"Users-fiona-Documents-my-app",
			"my_app",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DecodeCursorProjectDir(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

package parser

import "testing"

func TestNormalizeToolCategory(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		// Claude Code tools
		{"Read", "Read"},
		{"Edit", "Edit"},
		{"Write", "Write"},
		{"NotebookEdit", "Write"},
		{"Bash", "Bash"},
		{"Grep", "Grep"},
		{"Glob", "Glob"},
		{"Task", "Task"},
		{"Skill", "Tool"},

		// Codex tools
		{"shell_command", "Bash"},
		{"exec_command", "Bash"},
		{"apply_patch", "Edit"},
		{"write_stdin", "Bash"},
		{"shell", "Bash"},

		// Gemini tools
		{"read_file", "Read"},
		{"write_file", "Write"},
		{"edit_file", "Write"},
		{"run_command", "Bash"},
		{"execute_command", "Bash"},
		{"search_files", "Grep"},
		{"grep", "Grep"},

		// Unknown
		{"view_image", "Other"},
		{"update_plan", "Other"},
		{"list_mcp_resources", "Other"},
		{"AskUserQuestion", "Other"},
		{"EnterPlanMode", "Other"},
		{"ExitPlanMode", "Other"},
		{"", "Other"},
		{"some_random_tool", "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeToolCategory(tt.name)
			if got != tt.want {
				t.Errorf(
					"NormalizeToolCategory(%q) = %q, want %q",
					tt.name, got, tt.want,
				)
			}
		})
	}
}

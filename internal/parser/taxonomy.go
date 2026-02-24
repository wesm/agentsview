package parser

// NormalizeToolCategory maps a raw tool name to a normalized
// category. Categories: Read, Edit, Write, Bash, Grep, Glob,
// Task, Other.
func NormalizeToolCategory(rawName string) string {
	switch rawName {
	// Claude Code tools
	case "Read":
		return "Read"
	case "Edit":
		return "Edit"
	case "Write", "NotebookEdit":
		return "Write"
	case "Bash":
		return "Bash"
	case "Grep":
		return "Grep"
	case "Glob":
		return "Glob"
	case "Task":
		return "Task"

	// Codex tools
	case "shell_command", "exec_command",
		"write_stdin", "shell":
		return "Bash"
	case "apply_patch":
		return "Edit"

	// Gemini tools
	case "read_file":
		return "Read"
	case "write_file", "edit_file":
		return "Write"
	case "run_command", "execute_command":
		return "Bash"
	case "search_files", "grep":
		return "Grep"

	// Cursor tools
	case "Shell":
		return "Bash"
	case "StrReplace":
		return "Edit"
	case "LS":
		return "Read"

	default:
		return "Other"
	}
}

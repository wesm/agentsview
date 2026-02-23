package parser

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// ExtractTextContent extracts readable text from message content.
// content can be a string or a JSON array of blocks.
// Returns the text, hasThinking, hasToolUse, tool calls, and tool results.
func ExtractTextContent(
	content gjson.Result,
) (string, bool, bool, []ParsedToolCall, []ParsedToolResult) {
	if content.Type == gjson.String {
		return content.Str, false, false, nil, nil
	}

	if !content.IsArray() {
		return "", false, false, nil, nil
	}

	var (
		parts       []string
		toolCalls   []ParsedToolCall
		toolResults []ParsedToolResult
		hasThinking bool
		hasToolUse  bool
	)
	content.ForEach(func(_, block gjson.Result) bool {
		switch block.Get("type").Str {
		case "text":
			text := block.Get("text").Str
			if text != "" {
				parts = append(parts, text)
			}
		case "thinking":
			thinking := block.Get("thinking").Str
			if thinking != "" {
				hasThinking = true
				parts = append(parts, "[Thinking]\n"+thinking)
			}
		case "tool_use":
			hasToolUse = true
			name := block.Get("name").Str
			if name != "" {
				tc := ParsedToolCall{
					ToolUseID: block.Get("id").Str,
					ToolName:  name,
					Category:  NormalizeToolCategory(name),
					InputJSON: block.Get("input").Raw,
				}
				if name == "Skill" {
					tc.SkillName = block.Get("input.skill").Str
				}
				toolCalls = append(toolCalls, tc)
			}
			parts = append(parts, formatToolUse(block))
		case "tool_result":
			tuid := block.Get("tool_use_id").Str
			if tuid != "" {
				rc := block.Get("content")
				cl := toolResultContentLength(rc)
				toolResults = append(toolResults, ParsedToolResult{
					ToolUseID:     tuid,
					ContentLength: cl,
				})
			}
		}
		return true
	})

	return strings.Join(parts, "\n"),
		hasThinking, hasToolUse, toolCalls, toolResults
}

func toolResultContentLength(content gjson.Result) int {
	if content.Type == gjson.String {
		return len(content.Str)
	}
	if content.IsArray() {
		total := 0
		content.ForEach(func(_, block gjson.Result) bool {
			total += len(block.Get("text").Str)
			return true
		})
		return total
	}
	return 0
}

var todoIcons = map[string]string{
	"completed":   "✓",
	"in_progress": "→",
	"pending":     "○",
}

func formatToolUse(block gjson.Result) string {
	name := block.Get("name").Str
	input := block.Get("input")

	switch name {
	case "AskUserQuestion":
		return formatAskUserQuestion(name, input)
	case "TodoWrite":
		return formatTodoWrite(input)
	case "EnterPlanMode":
		return "[Entering Plan Mode]"
	case "ExitPlanMode":
		return "[Exiting Plan Mode]"
	case "Read":
		return fmt.Sprintf("[Read: %s]", input.Get("file_path").Str)
	case "Glob":
		return formatGlob(input)
	case "Grep":
		return fmt.Sprintf("[Grep: %s]", input.Get("pattern").Str)
	case "Edit":
		return fmt.Sprintf("[Edit: %s]", input.Get("file_path").Str)
	case "Write":
		return fmt.Sprintf("[Write: %s]", input.Get("file_path").Str)
	case "Bash":
		return formatBash(input)
	case "Task":
		return formatTask(input)
	case "Skill":
		return fmt.Sprintf("[Skill: %s]", input.Get("skill").Str)
	case "TaskCreate":
		subject := input.Get("subject").Str
		if subject != "" {
			return fmt.Sprintf("[TaskCreate: %s]", subject)
		}
		return "[TaskCreate]"
	case "TaskUpdate":
		taskID := input.Get("taskId").Str
		status := input.Get("status").Str
		if status != "" {
			return fmt.Sprintf("[TaskUpdate: #%s %s]", taskID, status)
		}
		return fmt.Sprintf("[TaskUpdate: #%s]", taskID)
	case "TaskGet":
		return fmt.Sprintf("[TaskGet: #%s]", input.Get("taskId").Str)
	case "TaskList":
		return "[TaskList]"
	case "SendMessage":
		msgType := input.Get("type").Str
		recipient := input.Get("recipient").Str
		if recipient != "" {
			return fmt.Sprintf("[SendMessage: %s to %s]", msgType, recipient)
		}
		return fmt.Sprintf("[SendMessage: %s]", msgType)
	default:
		return fmt.Sprintf("[Tool: %s]", name)
	}
}

func formatAskUserQuestion(
	name string, input gjson.Result,
) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("[Question: %s]", name))
	input.Get("questions").ForEach(func(_, q gjson.Result) bool {
		lines = append(lines, "  "+q.Get("question").Str)
		q.Get("options").ForEach(func(_, opt gjson.Result) bool {
			lines = append(lines, fmt.Sprintf(
				"    - %s: %s",
				opt.Get("label").Str,
				opt.Get("description").Str,
			))
			return true
		})
		return true
	})
	return strings.Join(lines, "\n")
}

func formatTodoWrite(input gjson.Result) string {
	var lines []string
	lines = append(lines, "[Todo List]")
	input.Get("todos").ForEach(func(_, todo gjson.Result) bool {
		status := todo.Get("status").Str
		icon := todoIcons[status]
		if icon == "" {
			icon = "○"
		}
		lines = append(lines, fmt.Sprintf(
			"  %s %s", icon, todo.Get("content").Str,
		))
		return true
	})
	return strings.Join(lines, "\n")
}

func formatGlob(input gjson.Result) string {
	return fmt.Sprintf("[Glob: %s in %s]",
		input.Get("pattern").Str,
		orDefault(input.Get("path").Str, "."))
}

func formatBash(input gjson.Result) string {
	cmd := input.Get("command").Str
	desc := input.Get("description").Str
	if desc != "" {
		return fmt.Sprintf("[Bash: %s]\n$ %s", desc, cmd)
	}
	return fmt.Sprintf("[Bash]\n$ %s", cmd)
}

func formatTask(input gjson.Result) string {
	desc := input.Get("description").Str
	agentType := input.Get("subagent_type").Str
	return fmt.Sprintf("[Task: %s (%s)]", desc, agentType)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

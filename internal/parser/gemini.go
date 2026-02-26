package parser

import (
	"fmt"
	"os"
	"strings"

	"github.com/tidwall/gjson"
)

// ParseGeminiSession parses a Gemini CLI session JSON file.
// Unlike Claude/Codex JSONL, each Gemini file is a single JSON
// document containing all messages.
func ParseGeminiSession(
	path, project, machine string,
) (*ParsedSession, []ParsedMessage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	if !gjson.ValidBytes(data) {
		return nil, nil, fmt.Errorf("invalid JSON in %s", path)
	}

	root := gjson.ParseBytes(data)

	sessionID := root.Get("sessionId").Str
	if sessionID == "" {
		return nil, nil, fmt.Errorf(
			"missing sessionId in %s", path,
		)
	}

	startTime := parseTimestamp(root.Get("startTime").Str)
	lastUpdated := parseTimestamp(root.Get("lastUpdated").Str)

	var (
		messages     []ParsedMessage
		firstMessage string
		ordinal      int
	)

	root.Get("messages").ForEach(
		func(_, msg gjson.Result) bool {
			msgType := msg.Get("type").Str
			if msgType != "user" && msgType != "gemini" {
				return true
			}

			ts := parseTimestamp(msg.Get("timestamp").Str)

			role := RoleUser
			if msgType == "gemini" {
				role = RoleAssistant
			}

			content, hasThinking, hasToolUse, tcs :=
				extractGeminiContent(msg)
			if strings.TrimSpace(content) == "" {
				return true
			}

			if role == RoleUser && firstMessage == "" {
				firstMessage = truncate(
					strings.ReplaceAll(content, "\n", " "),
					300,
				)
			}

			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          role,
				Content:       content,
				Timestamp:     ts,
				HasThinking:   hasThinking,
				HasToolUse:    hasToolUse,
				ContentLength: len(content),
				ToolCalls:     tcs,
			})
			ordinal++
			return true
		},
	)

	userCount := 0
	for _, m := range messages {
		if m.Role == RoleUser && m.Content != "" {
			userCount++
		}
	}

	sess := &ParsedSession{
		ID:               "gemini:" + sessionID,
		Project:          project,
		Machine:          machine,
		Agent:            AgentGemini,
		FirstMessage:     firstMessage,
		StartedAt:        startTime,
		EndedAt:          lastUpdated,
		MessageCount:     len(messages),
		UserMessageCount: userCount,
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		},
	}

	return sess, messages, nil
}

// extractGeminiContent builds readable text from a Gemini
// message, including its content, thoughts, and tool calls.
func extractGeminiContent(
	msg gjson.Result,
) (string, bool, bool, []ParsedToolCall) {
	var (
		parts       []string
		parsed      []ParsedToolCall
		hasThinking bool
		hasToolUse  bool
	)

	// Extract thoughts (appear before content chronologically)
	thoughts := msg.Get("thoughts")
	if thoughts.IsArray() {
		thoughts.ForEach(func(_, thought gjson.Result) bool {
			desc := thought.Get("description").Str
			if desc != "" {
				hasThinking = true
				subj := thought.Get("subject").Str
				if subj != "" {
					parts = append(parts,
						fmt.Sprintf(
							"[Thinking]\n%s\n%s", subj, desc,
						),
					)
				} else {
					parts = append(parts,
						"[Thinking]\n"+desc,
					)
				}
			}
			return true
		})
	}

	// Extract main content (string or Part[] array)
	content := msg.Get("content")
	if content.Type == gjson.String {
		if t := content.Str; t != "" {
			parts = append(parts, t)
		}
	} else if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			if t := part.Get("text").Str; t != "" {
				parts = append(parts, t)
			}
			return true
		})
	}

	// Extract tool calls
	toolCalls := msg.Get("toolCalls")
	if toolCalls.IsArray() {
		toolCalls.ForEach(func(_, tc gjson.Result) bool {
			hasToolUse = true
			name := tc.Get("name").Str
			if name != "" {
				parsed = append(parsed, ParsedToolCall{
					ToolName: name,
					Category: NormalizeToolCategory(name),
				})
			}
			parts = append(parts, formatGeminiToolCall(tc))
			return true
		})
	}

	return strings.Join(parts, "\n"),
		hasThinking, hasToolUse, parsed
}

func formatGeminiToolCall(tc gjson.Result) string {
	name := tc.Get("name").Str
	displayName := tc.Get("displayName").Str
	args := tc.Get("args")

	switch name {
	case "read_file":
		return fmt.Sprintf(
			"[Read: %s]", args.Get("file_path").Str,
		)
	case "write_file", "edit_file":
		return fmt.Sprintf(
			"[Write: %s]", args.Get("file_path").Str,
		)
	case "run_command", "execute_command":
		cmd := args.Get("command").Str
		return fmt.Sprintf("[Bash]\n$ %s", cmd)
	case "list_directory":
		return fmt.Sprintf(
			"[List: %s]", args.Get("dir_path").Str,
		)
	case "search_files", "grep":
		query := args.Get("query").Str
		if query == "" {
			query = args.Get("pattern").Str
		}
		return fmt.Sprintf("[Grep: %s]", query)
	default:
		label := displayName
		if label == "" {
			label = name
		}
		return fmt.Sprintf("[Tool: %s]", label)
	}
}

// GeminiSessionID extracts the sessionId field from raw
// Gemini session JSON data without fully parsing.
func GeminiSessionID(data []byte) string {
	return gjson.GetBytes(data, "sessionId").Str
}

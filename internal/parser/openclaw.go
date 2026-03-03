// ABOUTME: Parses OpenClaw JSONL session files into structured session data.
// ABOUTME: Handles OpenClaw's wrapped message format with toolResult role.
package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// ParseOpenClawSession parses an OpenClaw JSONL session file.
// OpenClaw stores messages in a JSONL format with a session header
// line, message entries, compaction summaries, and metadata events.
func ParseOpenClawSession(
	path, project, machine string,
) (*ParsedSession, []ParsedMessage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	lr := newLineReader(f, maxLineSize)
	var (
		messages      []ParsedMessage
		startedAt     time.Time
		endedAt       time.Time
		ordinal       int
		realUserCount int
		firstMsg      string
		sessionID     string
		cwd           string
	)

	for {
		line, ok := lr.next()
		if !ok {
			break
		}
		if !gjson.Valid(line) {
			continue
		}

		entryType := gjson.Get(line, "type").Str

		// Track timestamps from all entries for session bounds.
		if ts := parseOpenClawTimestamp(line); !ts.IsZero() {
			if startedAt.IsZero() || ts.Before(startedAt) {
				startedAt = ts
			}
			if ts.After(endedAt) {
				endedAt = ts
			}
		}

		switch entryType {
		case "session":
			// Session header — extract session ID and cwd.
			if sessionID == "" {
				sessionID = gjson.Get(line, "id").Str
			}
			if cwd == "" {
				cwd = gjson.Get(line, "cwd").Str
			}
			continue

		case "model_change", "thinking_level_change", "custom",
			"compaction":
			// Metadata entries — skip for message extraction.
			continue

		case "message":
			// Actual message entry.
		default:
			continue
		}

		msg := gjson.Get(line, "message")
		if !msg.Exists() {
			continue
		}

		role := msg.Get("role").Str
		ts := parseTimestamp(msg.Get("timestamp").Str)
		if ts.IsZero() {
			ts = parseTimestamp(gjson.Get(line, "timestamp").Str)
		}

		switch role {
		case "user":
			content := msg.Get("content")
			text, hasThinking, hasToolUse, tcs, trs :=
				ExtractTextContent(content)
			text = strings.TrimSpace(text)
			if text == "" && len(tcs) == 0 && len(trs) == 0 {
				continue
			}

			if firstMsg == "" && text != "" {
				firstMsg = truncate(
					strings.ReplaceAll(text, "\n", " "), 300,
				)
			}

			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleUser,
				Content:       text,
				Timestamp:     ts,
				HasThinking:   hasThinking,
				HasToolUse:    hasToolUse,
				ContentLength: len(text),
				ToolCalls:     tcs,
				ToolResults:   trs,
			})
			ordinal++
			realUserCount++

		case "assistant":
			content := msg.Get("content")
			text, hasThinking, hasToolUse, tcs, trs :=
				ExtractTextContent(content)
			text = strings.TrimSpace(text)
			if text == "" && len(tcs) == 0 && len(trs) == 0 {
				continue
			}

			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleAssistant,
				Content:       text,
				Timestamp:     ts,
				HasThinking:   hasThinking,
				HasToolUse:    hasToolUse,
				ContentLength: len(text),
				ToolCalls:     tcs,
				ToolResults:   trs,
			})
			ordinal++

		case "toolResult":
			// Tool results in OpenClaw are separate messages with
			// role "toolResult". We merge them into the previous
			// assistant message as tool_result content, matching
			// the pattern the frontend expects.
			toolCallID := msg.Get("toolCallId").Str
			toolName := msg.Get("toolName").Str
			isError := msg.Get("isError").Bool()

			content := msg.Get("content")
			resultText := extractToolResultText(content)
			contentLen := len(resultText)

			// Format as a user message with tool_result block,
			// matching Claude Code's format that the frontend
			// already handles.
			var tr []ParsedToolResult
			if toolCallID != "" {
				tr = append(tr, ParsedToolResult{
					ToolUseID:     toolCallID,
					ContentLength: contentLen,
				})
			}

			display := fmt.Sprintf(
				"[Tool Result: %s]", toolName,
			)
			if isError {
				display = fmt.Sprintf(
					"[Tool Error: %s]", toolName,
				)
			}
			if resultText != "" {
				display += "\n" + truncate(resultText, 2000)
			}

			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleUser,
				Content:       display,
				Timestamp:     ts,
				HasThinking:   false,
				HasToolUse:    false,
				ContentLength: contentLen,
				ToolResults:   tr,
			})
			ordinal++
		}
	}

	if err := lr.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if len(messages) == 0 {
		return nil, nil, nil
	}

	// Build session ID with prefix, including the agent
	// subdirectory to avoid collisions across agents.
	if sessionID == "" {
		sessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	agentID := openClawAgentIDFromPath(path)
	fullID := "openclaw:" + agentID + ":" + sessionID

	// Derive project from cwd if not provided.
	if project == "" && cwd != "" {
		project = ExtractProjectFromCwd(cwd)
	}
	if project == "" {
		project = "openclaw"
	}

	sess := &ParsedSession{
		ID:               fullID,
		Project:          project,
		Machine:          machine,
		Agent:            AgentOpenClaw,
		FirstMessage:     firstMsg,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		MessageCount:     len(messages),
		UserMessageCount: realUserCount,
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		},
	}

	return sess, messages, nil
}

// extractToolResultText extracts plain text from an OpenClaw
// tool result content field (which is an array of blocks).
func extractToolResultText(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.Str
	}
	if !content.IsArray() {
		return ""
	}

	var parts []string
	content.ForEach(func(_, block gjson.Result) bool {
		if block.Get("type").Str == "text" {
			if t := block.Get("text").Str; t != "" {
				parts = append(parts, t)
			}
		}
		return true
	})
	return strings.Join(parts, "\n")
}

// openClawAgentIDFromPath extracts the agent subdirectory name
// from an OpenClaw session file path. The expected layout is
// <agentsDir>/<agentId>/sessions/<sessionId>.jsonl, so the
// agent ID is the grandparent directory of the file.
func openClawAgentIDFromPath(path string) string {
	// path = .../agents/<agentId>/sessions/<file>.jsonl
	sessionsDir := filepath.Dir(path)     // .../agents/<agentId>/sessions
	agentDir := filepath.Dir(sessionsDir) // .../agents/<agentId>
	name := filepath.Base(agentDir)
	if name == "" || name == "." || name == "/" {
		return "unknown"
	}
	return name
}

// parseOpenClawTimestamp extracts and parses the timestamp from
// any OpenClaw JSONL entry.
func parseOpenClawTimestamp(line string) time.Time {
	tsStr := gjson.Get(line, "timestamp").Str
	return parseTimestamp(tsStr)
}

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
					strings.ReplaceAll(
						stripOpenClawDatePrefix(text),
						"\n", " ",
					), 300,
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
			// Tool results in OpenClaw are separate messages.
			// Emit as a user message with empty Content so
			// pairAndFilter removes it after pairToolResults
			// copies ResultContentLength to the matching call.
			toolCallID := msg.Get("toolCallId").Str
			if toolCallID == "" {
				continue
			}

			content := msg.Get("content")
			resultText := extractToolResultText(content)
			contentLen := len(resultText)

			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleUser,
				Content:       "",
				Timestamp:     ts,
				HasThinking:   false,
				HasToolUse:    false,
				ContentLength: contentLen,
				ToolResults: []ParsedToolResult{{
					ToolUseID:     toolCallID,
					ContentLength: contentLen,
				}},
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
		sessionID = OpenClawSessionID(filepath.Base(path))
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

// IsOpenClawSessionFile reports whether a filename is an OpenClaw
// session file. It matches active files (*.jsonl) and the known
// archive suffixes: .jsonl.deleted.<ts>, .jsonl.reset.<ts>, and
// .jsonl.full.bak.
func IsOpenClawSessionFile(name string) bool {
	if strings.HasSuffix(name, ".jsonl") {
		return true
	}
	idx := strings.Index(name, ".jsonl.")
	if idx <= 0 {
		return false
	}
	suffix := name[idx+len(".jsonl."):]
	return strings.HasPrefix(suffix, "deleted.") ||
		strings.HasPrefix(suffix, "reset.") ||
		suffix == "full.bak"
}

// OpenClawSessionID extracts the session UUID from an OpenClaw
// session filename, stripping any archive suffix.
// "abc.jsonl" → "abc"
// "abc.jsonl.deleted.2026-02-19T08-59-24.951Z" → "abc"
// "abc.jsonl.full.bak" → "abc"
func OpenClawSessionID(name string) string {
	if idx := strings.Index(name, ".jsonl"); idx > 0 {
		return name[:idx]
	}
	return strings.TrimSuffix(name, ".jsonl")
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

// stripOpenClawDatePrefix removes the gateway-injected date
// prefix from user messages. OpenClaw prepends timestamps like
// "[Wed 2026-02-18 11:21 GMT+1] " to messages received via
// Telegram/channels. We strip this so session titles are clean.
func stripOpenClawDatePrefix(s string) string {
	if len(s) < 2 || s[0] != '[' {
		return s
	}
	idx := strings.Index(s, "] ")
	if idx < 0 || idx > 40 {
		return s
	}
	return strings.TrimSpace(s[idx+2:])
}

// parseOpenClawTimestamp extracts and parses the timestamp from
// any OpenClaw JSONL entry.
func parseOpenClawTimestamp(line string) time.Time {
	tsStr := gjson.Get(line, "timestamp").Str
	return parseTimestamp(tsStr)
}

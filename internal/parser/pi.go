package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// ParsePiSession parses a pi-agent JSONL session file.
// The file format uses a leading session-header entry followed by
// message, model_change, and compaction entries.
func ParsePiSession(
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

	// --- Parse session header (first non-empty line) ---
	headerLine, ok := lr.next()
	if !ok {
		return nil, nil, fmt.Errorf(
			"not a pi session: missing session header in %s", path,
		)
	}

	if !gjson.Valid(headerLine) {
		return nil, nil, fmt.Errorf(
			"not a pi session: invalid JSON header in %s", path,
		)
	}

	if gjson.Get(headerLine, "type").Str != "session" {
		return nil, nil, fmt.Errorf(
			"not a pi session: missing session header in %s", path,
		)
	}

	sessionID := gjson.Get(headerLine, "id").Str
	cwd := gjson.Get(headerLine, "cwd").Str
	headerTimestamp := parseTimestamp(gjson.Get(headerLine, "timestamp").Str)

	// If project was not passed in, derive from cwd.
	if project == "" && cwd != "" {
		project = ExtractProjectFromCwd(cwd)
	}

	// branchedFrom handling: store basename without extension.
	var parentSessionID string
	branchedFrom := gjson.Get(headerLine, "branchedFrom").Str
	if branchedFrom != "" {
		base := filepath.Base(branchedFrom)
		parentSessionID = "pi:" + strings.TrimSuffix(base, filepath.Ext(base))
	}

	// V1 detection: if header has no id, we may need to derive from filename.
	isV1 := sessionID == ""

	// --- Main message loop ---
	var (
		messages     []ParsedMessage
		firstMessage string
		ordinal      int
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
		if entryType == "" {
			continue
		}

		// If any message entry has an id field, this is a V2 session.
		if isV1 && gjson.Get(line, "id").Str != "" {
			isV1 = false
		}

		switch entryType {
		case "message":
			role := gjson.Get(line, "message.role").Str
			switch role {
			case "user":
				msg := parsePiUserMessage(line, ordinal)
				if msg == nil {
					continue
				}
				if firstMessage == "" && msg.Content != "" {
					firstMessage = truncate(
						strings.ReplaceAll(msg.Content, "\n", " "),
						300,
					)
				}
				messages = append(messages, *msg)
				ordinal++

			case "assistant":
				msg := parsePiAssistantMessage(line, ordinal)
				if msg == nil {
					continue
				}
				messages = append(messages, *msg)
				ordinal++

			case "toolResult":
				msg := parsePiToolResultMessage(line, ordinal)
				if msg == nil {
					continue
				}
				messages = append(messages, *msg)
				ordinal++

			default:
				// skip silently
			}

		case "model_change":
			provider := gjson.Get(line, "provider").Str
			modelID := gjson.Get(line, "modelId").Str
			content := fmt.Sprintf(
				"Model changed to %s/%s", provider, modelID,
			)
			ts := parseTimestamp(gjson.Get(line, "timestamp").Str)
			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleUser,
				Content:       content,
				Timestamp:     ts,
				ContentLength: len(content),
			})
			ordinal++

		case "compaction":
			summary := gjson.Get(line, "summary").Str
			if summary == "" {
				summary = "[session compacted]"
			}
			ts := parseTimestamp(gjson.Get(line, "timestamp").Str)
			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleUser,
				Content:       summary,
				Timestamp:     ts,
				ContentLength: len(summary),
			})
			ordinal++

		default:
			// skip silently (e.g., thinking_level_change)
		}
	}

	if err := lr.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading pi %s: %w", path, err)
	}

	// V1 fallback: derive session ID from filename.
	if isV1 || sessionID == "" {
		sessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}

	// Compute StartedAt and EndedAt from message timestamps.
	startedAt := headerTimestamp
	var endedAt time.Time
	for _, m := range messages {
		if m.Timestamp.IsZero() {
			continue
		}
		if startedAt.IsZero() || m.Timestamp.Before(startedAt) {
			startedAt = m.Timestamp
		}
		if endedAt.IsZero() || m.Timestamp.After(endedAt) {
			endedAt = m.Timestamp
		}
	}

	// Count user messages (those with non-empty content).
	userCount := 0
	for _, m := range messages {
		if m.Role == RoleUser && m.Content != "" {
			userCount++
		}
	}

	sess := &ParsedSession{
		ID:               "pi:" + sessionID,
		Project:          project,
		Machine:          machine,
		Agent:            AgentPi,
		ParentSessionID:  parentSessionID,
		FirstMessage:     firstMessage,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
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

// parsePiUserMessage parses a message entry with role="user".
// Returns nil if the entry is malformed.
func parsePiUserMessage(line string, ordinal int) *ParsedMessage {
	content := gjson.Get(line, "message.content")

	var text string
	if content.Type == gjson.String {
		text = content.Str
	} else if content.IsArray() {
		var parts []string
		content.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").Str == "text" {
				if t := block.Get("text").Str; t != "" {
					parts = append(parts, t)
				}
			}
			return true
		})
		text = strings.Join(parts, "\n")
	}

	ts := piTimestamp(line)

	return &ParsedMessage{
		Ordinal:       ordinal,
		Role:          RoleUser,
		Content:       text,
		Timestamp:     ts,
		ContentLength: len(text),
	}
}

// parsePiAssistantMessage parses a message entry with role="assistant".
// Returns nil if the entry is malformed.
func parsePiAssistantMessage(line string, ordinal int) *ParsedMessage {
	var (
		parts       []string
		toolCalls   []ParsedToolCall
		hasThinking bool
		hasToolUse  bool
	)

	gjson.Get(line, "message.content").ForEach(func(_, block gjson.Result) bool {
		switch block.Get("type").Str {
		case "text":
			if t := block.Get("text").Str; t != "" {
				parts = append(parts, t)
			}
		case "thinking":
			// Set hasThinking regardless of whether the thinking field is
			// empty — redacted thinking blocks have an empty field but the
			// block type presence is sufficient to mark the message.
			hasThinking = true
		case "toolCall":
			hasToolUse = true
			id := block.Get("id").Str
			name := block.Get("name").Str
			argsRaw := block.Get("arguments").Raw
			toolCalls = append(toolCalls, ParsedToolCall{
				ToolUseID: id,
				ToolName:  name,
				Category:  NormalizeToolCategory(name),
				InputJSON: argsRaw,
			})
		}
		return true
	})

	content := strings.Join(parts, "\n")
	ts := piTimestamp(line)

	return &ParsedMessage{
		Ordinal:       ordinal,
		Role:          RoleAssistant,
		Content:       content,
		Timestamp:     ts,
		HasThinking:   hasThinking,
		HasToolUse:    hasToolUse,
		ContentLength: len(content),
		ToolCalls:     toolCalls,
	}
}

// parsePiToolResultMessage parses a message entry with role="toolResult".
// Returns nil if the entry is malformed.
func parsePiToolResultMessage(line string, ordinal int) *ParsedMessage {
	toolUseID := gjson.Get(line, "message.toolCallId").Str

	contentLen := 0
	gjson.Get(line, "message.content").ForEach(func(_, block gjson.Result) bool {
		contentLen += len(block.Get("text").Str)
		return true
	})

	ts := piTimestamp(line)

	return &ParsedMessage{
		Ordinal:   ordinal,
		Role:      RoleUser,
		Timestamp: ts,
		ToolResults: []ParsedToolResult{
			{
				ToolUseID:     toolUseID,
				ContentLength: contentLen,
			},
		},
	}
}

// piTimestamp extracts the timestamp for a pi JSONL entry.
// Tries the top-level "timestamp" field first (ISO 8601), then
// falls back to "message.timestamp" as Unix milliseconds.
func piTimestamp(line string) time.Time {
	if ts := parseTimestamp(gjson.Get(line, "timestamp").Str); !ts.IsZero() {
		return ts
	}
	if ms := gjson.Get(line, "message.timestamp").Int(); ms != 0 {
		return time.UnixMilli(ms).UTC()
	}
	return time.Time{}
}

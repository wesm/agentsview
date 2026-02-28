package parser

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// ParseAmpSession parses an Amp thread JSON file.
// Each thread is a single JSON document at ~/.local/share/amp/threads/T-*.json.
func ParseAmpSession(
	path, machine string,
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

	threadID := root.Get("id").Str
	if threadID == "" {
		return nil, nil, fmt.Errorf("missing id in %s", path)
	}

	// Start time from created (epoch ms) when valid and positive.
	var startTime time.Time
	if created := root.Get("created"); created.Type == gjson.Number {
		if ms := created.Int(); ms > 0 {
			startTime = time.UnixMilli(ms)
		}
	}

	// End time from meta.traces[last].endTime.
	var endTime time.Time
	traces := root.Get("meta.traces")
	if traces.IsArray() {
		traceList := traces.Array()
		if len(traceList) > 0 {
			endTime = parseTimestamp(
				traceList[len(traceList)-1].Get("endTime").Str,
			)
		}
	}

	// Project from env.initial.trees[0].displayName.
	project := root.Get("env.initial.trees.0.displayName").Str
	if project == "" {
		project = "amp"
	}

	// Title is used as FirstMessage when present.
	title := root.Get("title").Str

	var (
		messages     []ParsedMessage
		firstMessage string
		ordinal      int
	)

	root.Get("messages").ForEach(func(_, msg gjson.Result) bool {
		roleStr := msg.Get("role").Str
		if roleStr != "user" && roleStr != "assistant" {
			return true
		}

		role := RoleUser
		if roleStr == "assistant" {
			role = RoleAssistant
		}

		content, hasThinking, hasToolUse, tcs, trs :=
			ExtractTextContent(msg.Get("content"))
		if strings.TrimSpace(content) == "" && len(trs) == 0 {
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
			HasThinking:   hasThinking,
			HasToolUse:    hasToolUse,
			ContentLength: len(content),
			ToolCalls:     tcs,
			ToolResults:   trs,
		})
		ordinal++
		return true
	})

	// Empty threads are non-interactive; skip them.
	if len(messages) == 0 {
		return nil, nil, nil
	}

	// Use title as FirstMessage when available.
	if title != "" {
		firstMessage = title
	}

	userCount := 0
	for _, m := range messages {
		if m.Role == RoleUser && m.Content != "" {
			userCount++
		}
	}

	sess := &ParsedSession{
		ID:               "amp:" + threadID,
		Project:          project,
		Machine:          machine,
		Agent:            AgentAmp,
		FirstMessage:     firstMessage,
		StartedAt:        startTime,
		EndedAt:          endTime,
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

// AmpThreadID extracts the id field from raw Amp thread JSON
// data without fully parsing.
func AmpThreadID(data []byte) string {
	return gjson.GetBytes(data, "id").Str
}

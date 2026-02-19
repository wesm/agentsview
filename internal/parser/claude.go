package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	initialScanBufSize = 64 * 1024        // 64KB
	maxScanTokenSize   = 20 * 1024 * 1024 // 20MB
)

// ParseClaudeSession parses a Claude Code JSONL session file.
func ParseClaudeSession(
	path, project, machine string,
) (ParsedSession, []ParsedMessage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ParsedSession{}, nil, fmt.Errorf("stat %s: %w", path, err)
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	f, err := os.Open(path)
	if err != nil {
		return ParsedSession{}, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var (
		messages  []ParsedMessage
		firstMsg  string
		startedAt time.Time
		endedAt   time.Time
		ordinal   int
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(
		make([]byte, 0, initialScanBufSize), maxScanTokenSize,
	)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		if !gjson.Valid(line) {
			continue
		}

		// Extract timestamp
		tsStr := gjson.Get(line, "timestamp").Str
		ts := parseTimestamp(tsStr)
		if ts.IsZero() {
			snapTsStr := gjson.Get(line, "snapshot.timestamp").Str
			ts = parseTimestamp(snapTsStr)

			if ts.IsZero() {
				if tsStr != "" {
					logParseError(tsStr)
				} else if snapTsStr != "" {
					logParseError(snapTsStr)
				}
			}
		}
		if !ts.IsZero() {
			if startedAt.IsZero() {
				startedAt = ts
			}
			endedAt = ts
		}

		entryType := gjson.Get(line, "type").Str

		if entryType == "user" || entryType == "assistant" {
			content := gjson.Get(line, "message.content")
			text, hasThinking, hasToolUse, tcs :=
				ExtractTextContent(content)
			if strings.TrimSpace(text) == "" {
				continue
			}

			if entryType == "user" && firstMsg == "" {
				firstMsg = truncate(
					strings.ReplaceAll(text, "\n", " "), 300,
				)
			}

			messages = append(messages, ParsedMessage{
				Ordinal:       ordinal,
				Role:          RoleType(entryType),
				Content:       text,
				Timestamp:     ts,
				HasThinking:   hasThinking,
				HasToolUse:    hasToolUse,
				ContentLength: len(text),
				ToolCalls:     tcs,
			})
			ordinal++
		}
	}

	if err := scanner.Err(); err != nil {
		return ParsedSession{}, nil,
			fmt.Errorf("scanning %s: %w", path, err)
	}

	sess := ParsedSession{
		ID:           sessionID,
		Project:      project,
		Machine:      machine,
		Agent:        AgentClaude,
		FirstMessage: firstMsg,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		MessageCount: len(messages),
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		},
	}

	return sess, messages, nil
}

// ExtractCwdFromSession reads the first cwd field from a Claude
// Code JSONL session file.
func ExtractCwdFromSession(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(
		make([]byte, 0, initialScanBufSize), maxScanTokenSize,
	)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !gjson.Valid(line) {
			continue
		}
		if gjson.Get(line, "type").Str == "user" {
			cwd := gjson.Get(line, "cwd").Str
			if cwd != "" {
				return cwd
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

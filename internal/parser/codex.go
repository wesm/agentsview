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

// Codex JSONL entry types.
const (
	codexTypeSessionMeta  = "session_meta"
	codexTypeResponseItem = "response_item"
	codexOriginatorExec   = "codex_exec"
)

// codexSessionBuilder accumulates state while scanning a Codex
// JSONL session file line by line.
type codexSessionBuilder struct {
	messages     []ParsedMessage
	firstMessage string
	startedAt    time.Time
	endedAt      time.Time
	sessionID    string
	project      string
	ordinal      int
	includeExec  bool
}

func newCodexSessionBuilder(
	includeExec bool,
) *codexSessionBuilder {
	return &codexSessionBuilder{
		project:     "unknown",
		includeExec: includeExec,
	}
}

// processLine handles a single non-empty, valid JSON line.
// Returns (skip=true) if the session should be discarded
// (e.g. non-interactive codex_exec).
func (b *codexSessionBuilder) processLine(
	line string,
) (skip bool) {
	tsStr := gjson.Get(line, "timestamp").Str
	ts := parseTimestamp(tsStr)
	if ts.IsZero() {
		if tsStr != "" {
			logParseError(tsStr)
		}
	} else {
		if b.startedAt.IsZero() {
			b.startedAt = ts
		}
		b.endedAt = ts
	}

	payload := gjson.Get(line, "payload")

	switch gjson.Get(line, "type").Str {
	case codexTypeSessionMeta:
		return b.handleSessionMeta(payload)
	case codexTypeResponseItem:
		b.handleResponseItem(payload, ts)
	}
	return false
}

func (b *codexSessionBuilder) handleSessionMeta(
	payload gjson.Result,
) (skip bool) {
	b.sessionID = payload.Get("id").Str

	if cwd := payload.Get("cwd").Str; cwd != "" {
		if proj := ExtractProjectFromCwd(cwd); proj != "" {
			b.project = proj
		} else {
			b.project = "unknown"
		}
	}

	if !b.includeExec &&
		payload.Get("originator").Str == codexOriginatorExec {
		return true
	}
	return false
}

func (b *codexSessionBuilder) handleResponseItem(
	payload gjson.Result, ts time.Time,
) {
	if payload.Get("type").Str == "function_call" {
		b.handleFunctionCall(payload, ts)
		return
	}

	role := payload.Get("role").Str
	if role != "user" && role != "assistant" {
		return
	}

	content := extractCodexContent(payload)
	if strings.TrimSpace(content) == "" {
		return
	}

	if role == "user" && isCodexSystemMessage(content) {
		return
	}

	if role == "user" && b.firstMessage == "" {
		b.firstMessage = truncate(
			strings.ReplaceAll(content, "\n", " "), 300,
		)
	}

	b.messages = append(b.messages, ParsedMessage{
		Ordinal:       b.ordinal,
		Role:          RoleType(role),
		Content:       content,
		Timestamp:     ts,
		ContentLength: len(content),
	})
	b.ordinal++
}

func (b *codexSessionBuilder) handleFunctionCall(
	payload gjson.Result, ts time.Time,
) {
	name := payload.Get("name").Str
	if name == "" {
		return
	}

	content := formatCodexFunctionCall(name, payload)

	b.messages = append(b.messages, ParsedMessage{
		Ordinal:       b.ordinal,
		Role:          RoleAssistant,
		Content:       content,
		Timestamp:     ts,
		HasToolUse:    true,
		ContentLength: len(content),
		ToolCalls: []ParsedToolCall{{
			ToolName: name,
			Category: NormalizeToolCategory(name),
		}},
	})
	b.ordinal++
}

func formatCodexFunctionCall(
	name string, payload gjson.Result,
) string {
	summary := payload.Get("summary").Str
	if summary != "" {
		return fmt.Sprintf("[%s: %s]", name, summary)
	}
	return fmt.Sprintf("[%s]", name)
}

// extractCodexContent joins all text blocks from a Codex
// response item's content array.
func extractCodexContent(payload gjson.Result) string {
	var texts []string
	payload.Get("content").ForEach(
		func(_, block gjson.Result) bool {
			switch block.Get("type").Str {
			case "input_text", "output_text", "text":
				if t := block.Get("text").Str; t != "" {
					texts = append(texts, t)
				}
			}
			return true
		},
	)
	return strings.Join(texts, "\n")
}

// ParseCodexSession parses a Codex JSONL session file.
// Returns nil session if the session is non-interactive and
// includeExec is false.
func ParseCodexSession(
	path, machine string, includeExec bool,
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

	scanner := bufio.NewScanner(f)
	scanner.Buffer(
		make([]byte, 0, initialScanBufSize), maxScanTokenSize,
	)

	b := newCodexSessionBuilder(includeExec)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !gjson.Valid(line) {
			continue
		}
		if b.processLine(line) {
			return nil, nil, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil,
			fmt.Errorf("scanning codex %s: %w", path, err)
	}

	sessionID := b.sessionID
	if sessionID == "" {
		sessionID = strings.TrimSuffix(
			filepath.Base(path), ".jsonl",
		)
	}
	sessionID = "codex:" + sessionID

	sess := &ParsedSession{
		ID:           sessionID,
		Project:      b.project,
		Machine:      machine,
		Agent:        AgentCodex,
		FirstMessage: b.firstMessage,
		StartedAt:    b.startedAt,
		EndedAt:      b.endedAt,
		MessageCount: len(b.messages),
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		},
	}

	return sess, b.messages, nil
}

func isCodexSystemMessage(content string) bool {
	return strings.HasPrefix(content, "# AGENTS.md") ||
		strings.HasPrefix(content, "<environment_context>") ||
		strings.HasPrefix(content, "<INSTRUCTIONS>")
}

package parser

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxCursorTranscriptSize is the maximum transcript file size
// we'll read into memory. Cursor transcripts are typically
// under 500 KB; 10 MB provides generous headroom.
const maxCursorTranscriptSize = 10 << 20

// ParseCursorSession parses a Cursor agent transcript file.
// Transcripts are plain text with "user:" and "assistant:" role
// markers, tool calls, and thinking blocks.
func ParseCursorSession(
	path, project, machine string,
) (*ParsedSession, []ParsedMessage, error) {
	// Open the file once and use the fd for all operations
	// (fstat + read) to eliminate TOCTOU races between
	// validation and read.
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Use Fstat on the open fd — this reflects the actual
	// opened file, not whatever the path currently points to.
	info, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf(
			"skip %s: not a regular file", path,
		)
	}
	if info.Size() > maxCursorTranscriptSize {
		return nil, nil, fmt.Errorf(
			"skip %s: file too large (%d bytes, max %d)",
			path, info.Size(), maxCursorTranscriptSize,
		)
	}

	// Read from the already-open fd, not by re-opening path.
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	messages := parseCursorMessages(lines)
	if len(messages) == 0 {
		return nil, nil, nil
	}

	sessionID := "cursor:" + strings.TrimSuffix(
		filepath.Base(path), ".txt",
	)

	var firstMessage string
	for _, m := range messages {
		if m.Role == RoleUser && m.Content != "" {
			firstMessage = truncate(
				strings.ReplaceAll(m.Content, "\n", " "), 300,
			)
			break
		}
	}

	// Compute hash from the already-read data to avoid
	// re-opening the file by path (which would be another
	// TOCTOU opportunity).
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	mtime := info.ModTime()
	sess := &ParsedSession{
		ID:           sessionID,
		Project:      project,
		Machine:      machine,
		Agent:        AgentCursor,
		FirstMessage: firstMessage,
		StartedAt:    mtime,
		EndedAt:      mtime,
		MessageCount: len(messages),
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: mtime.UnixNano(),
			Hash:  hash,
		},
	}
	return sess, messages, nil
}

// cursorBlock represents a raw block of lines between role
// markers in a Cursor transcript.
type cursorBlock struct {
	role  RoleType
	lines []string
}

// parseCursorMessages splits transcript lines on role markers
// and converts each block into a ParsedMessage.
func parseCursorMessages(lines []string) []ParsedMessage {
	blocks := splitCursorBlocks(lines)
	messages := make([]ParsedMessage, 0, len(blocks))

	for i, block := range blocks {
		content, hasThinking, toolCalls := extractCursorContent(
			block.role, block.lines,
		)
		content = strings.TrimSpace(content)
		if content == "" && len(toolCalls) == 0 {
			continue
		}

		messages = append(messages, ParsedMessage{
			Ordinal:       i,
			Role:          block.role,
			Content:       content,
			Timestamp:     time.Time{},
			HasThinking:   hasThinking,
			HasToolUse:    len(toolCalls) > 0,
			ContentLength: len(content),
			ToolCalls:     toolCalls,
		})
	}

	// Re-number ordinals to be contiguous after filtering
	for i := range messages {
		messages[i].Ordinal = i
	}
	return messages
}

// splitCursorBlocks splits lines into blocks delimited by
// "user:" or "assistant:" on a line by itself.
func splitCursorBlocks(lines []string) []cursorBlock {
	var blocks []cursorBlock
	var current *cursorBlock

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "user:" || trimmed == "assistant:" {
			if current != nil {
				blocks = append(blocks, *current)
			}
			role := RoleUser
			if trimmed == "assistant:" {
				role = RoleAssistant
			}
			current = &cursorBlock{role: role}
			continue
		}
		if current != nil {
			current.lines = append(current.lines, line)
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}
	return blocks
}

// extractCursorContent processes lines for a single message
// block, returning the visible text content, whether thinking
// was present, and any tool calls found.
func extractCursorContent(
	role RoleType, lines []string,
) (string, bool, []ParsedToolCall) {
	if role == RoleUser {
		content := extractUserQuery(lines)
		return content, false, nil
	}
	return extractAssistantContent(lines)
}

// extractUserQuery extracts text from <user_query> tags.
// Falls back to joining all lines if no tags are found.
func extractUserQuery(lines []string) string {
	text := strings.Join(lines, "\n")

	start := strings.Index(text, "<user_query>")
	end := strings.Index(text, "</user_query>")
	if start >= 0 && end > start {
		return strings.TrimSpace(
			text[start+len("<user_query>") : end],
		)
	}

	return strings.TrimSpace(text)
}

// extractAssistantContent parses assistant message lines for
// visible text, thinking blocks, and tool calls.
func extractAssistantContent(
	lines []string,
) (string, bool, []ParsedToolCall) {
	var textParts []string
	var toolCalls []ParsedToolCall
	hasThinking := false

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Thinking block
		if strings.HasPrefix(trimmed, "[Thinking]") {
			hasThinking = true
			i++
			// Skip thinking content until next marker
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if isAssistantMarker(t) {
					break
				}
				i++
			}
			continue
		}

		// Tool call
		if strings.HasPrefix(trimmed, "[Tool call] ") {
			toolName := strings.TrimPrefix(
				trimmed, "[Tool call] ",
			)
			toolCalls = append(toolCalls, ParsedToolCall{
				ToolName: toolName,
				Category: NormalizeToolCategory(toolName),
			})
			i++
			// Skip tool call parameters until next marker
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if isAssistantMarker(t) {
					break
				}
				i++
			}
			continue
		}

		// Tool result — skip the header and body
		if strings.HasPrefix(trimmed, "[Tool result]") {
			i++
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if isAssistantMarker(t) {
					break
				}
				i++
			}
			continue
		}

		// Regular text
		textParts = append(textParts, line)
		i++
	}

	content := strings.TrimSpace(strings.Join(textParts, "\n"))
	return content, hasThinking, toolCalls
}

// isAssistantMarker returns true if the line is a structural
// marker within an assistant block (thinking, tool call, or
// tool result).
func isAssistantMarker(trimmed string) bool {
	return strings.HasPrefix(trimmed, "[Thinking]") ||
		strings.HasPrefix(trimmed, "[Tool call] ") ||
		strings.HasPrefix(trimmed, "[Tool result]")
}

// DecodeCursorProjectDir extracts a clean project name from
// a Cursor-style hyphenated directory name. Cursor encodes
// absolute paths by replacing / and . with hyphens, e.g.
// "Users-fiona-fan-Documents-mcp-cursor-analytics".
func DecodeCursorProjectDir(dirName string) string {
	if dirName == "" {
		return ""
	}

	parts := strings.Split(dirName, "-")

	// Find the last known parent directory marker.
	// Everything after it is the project path.
	markers := map[string]bool{
		"Documents": true, "Code": true,
		"code": true, "projects": true,
		"repos": true, "src": true,
		"work": true, "dev": true,
	}

	lastMarkerIdx := -1
	for i, part := range parts {
		if markers[part] {
			lastMarkerIdx = i
		}
	}

	if lastMarkerIdx >= 0 && lastMarkerIdx+1 < len(parts) {
		result := strings.Join(
			parts[lastMarkerIdx+1:], "-",
		)
		if result != "" {
			return normalizeName(result)
		}
	}

	// Fallback: last two components
	if len(parts) >= 2 {
		return normalizeName(
			strings.Join(parts[len(parts)-2:], "-"),
		)
	}
	return normalizeName(dirName)
}

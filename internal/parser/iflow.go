// ABOUTME: Parses iFlow JSONL session files into structured session data.
// iFlow uses a similar format to Claude Code with uuid/parentUuid structure.
package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// dagEntryIflow holds metadata for a single JSONL entry participating
// in the uuid/parentUuid DAG.
type dagEntryIflow struct {
	uuid       string
	parentUuid string
	entryType  string // "user" or "assistant"
	lineIndex  int
	line       string
	timestamp  time.Time
}

// ParseIflowSession parses an iFlow JSONL session file.
// Returns one or more ParseResult structs (multiple when forks
// are detected in the uuid/parentUuid DAG).
func ParseIflowSession(
	path, project, machine string,
) ([]ParseResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	// Extract session ID from filename: session-<uuid>.jsonl
	filename := filepath.Base(path)
	sessionID := strings.TrimSuffix(filename, ".jsonl")
	if strings.HasPrefix(sessionID, "session-") {
		sessionID = strings.TrimPrefix(sessionID, "session-")
	}
	// Normalize iFlow IDs with namespace prefix
	sessionID = "iflow:" + sessionID

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// First pass: collect all valid lines with metadata.
	var (
		entries         []dagEntryIflow
		hasAnyUUID      bool
		allHaveUUID     bool
		parentSessionID string
		foundParentSID  bool
		lineIndex       int
		subagentMap     = map[string]string{}
		globalStart     time.Time
		globalEnd       time.Time
	)
	allHaveUUID = true

	lr := newLineReader(f, maxLineSize)
	for {
		line, ok := lr.next()
		if !ok {
			break
		}
		if !gjson.Valid(line) {
			continue
		}

		entryType := gjson.Get(line, "type").Str

		// Track global timestamps from all lines for session
		// bounds, including non-message events.
		if ts := extractTimestampIflow(line); !ts.IsZero() {
			if globalStart.IsZero() || ts.Before(globalStart) {
				globalStart = ts
			}
			if ts.After(globalEnd) {
				globalEnd = ts
			}
		}

		if entryType != "user" && entryType != "assistant" {
			continue
		}

		// Track parentSessionID from first user/assistant entry.
		if !foundParentSID {
			if sid := gjson.Get(line, "sessionId").Str; sid != "" {
				foundParentSID = true
				// iFlow sessionId is the full session filename (e.g., "session-uuid")
				// Extract the ID by trimming "session-" prefix and compare with sessionID
				sidID := strings.TrimPrefix(sid, "session-")
				// Compare with the raw UUID (without iflow: prefix)
				rawSessionID := strings.TrimPrefix(sessionID, "iflow:")
				if sidID != rawSessionID {
					parentSessionID = "iflow:" + sidID
				}
			}
		}

		uuid := gjson.Get(line, "uuid").Str
		parentUuid := gjson.Get(line, "parentUuid").Str

		if uuid != "" {
			hasAnyUUID = true
		} else {
			allHaveUUID = false
		}

		ts := extractTimestampIflow(line)

		entries = append(entries, dagEntryIflow{
			uuid:       uuid,
			parentUuid: parentUuid,
			entryType:  entryType,
			lineIndex:  lineIndex,
			line:       line,
			timestamp:  ts,
		})
		lineIndex++
	}

	if err := lr.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	fileInfo := FileInfo{
		Path:  path,
		Size:  info.Size(),
		Mtime: info.ModTime().UnixNano(),
	}

	// If all user/assistant entries have uuids, use DAG-aware processing.
	if hasAnyUUID && allHaveUUID {
		return parseDAGIflow(
			entries, sessionID, project, machine,
			parentSessionID, fileInfo, subagentMap,
			globalStart, globalEnd,
		)
	}

	// Fall back to linear processing.
	return parseLinearIflow(
		entries, sessionID, project, machine,
		parentSessionID, fileInfo, subagentMap,
		globalStart, globalEnd,
	)
}

// parseLinearIflow processes entries sequentially without DAG awareness.
func parseLinearIflow(
	entries []dagEntryIflow,
	sessionID, project, machine, parentSessionID string,
	fileInfo FileInfo,
	subagentMap map[string]string,
	globalStart, globalEnd time.Time,
) ([]ParseResult, error) {
	messages, startedAt, endedAt := extractMessagesIflow(entries)
	startedAt = earlierTime(globalStart, startedAt)
	endedAt = laterTime(globalEnd, endedAt)

	userCount := 0
	firstMsg := ""
	for _, m := range messages {
		if m.Role == RoleUser && m.Content != "" {
			userCount++
			if firstMsg == "" {
				firstMsg = truncate(
					strings.ReplaceAll(m.Content, "\n", " "), 300,
				)
			}
		}
	}

	sess := ParsedSession{
		ID:               sessionID,
		Project:          project,
		Machine:          machine,
		Agent:            AgentIflow,
		ParentSessionID:  parentSessionID,
		FirstMessage:     firstMsg,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		MessageCount:     len(messages),
		UserMessageCount: userCount,
		File:             fileInfo,
	}

	return []ParseResult{{Session: sess, Messages: messages}}, nil
}

// parseDAGIflow builds a parent->children adjacency map and walks the
// tree to detect fork points. Large-gap forks produce separate
// ParseResults; small-gap retries follow the latest branch.
func parseDAGIflow(
	entries []dagEntryIflow,
	sessionID, project, machine, parentSessionID string,
	fileInfo FileInfo,
	subagentMap map[string]string,
	globalStart, globalEnd time.Time,
) ([]ParseResult, error) {
	// Build parent -> children ordered by line position and
	// collect the set of all uuids for connectivity checks.
	children := make(map[string][]int, len(entries))
	uuidSet := make(map[string]struct{}, len(entries))
	var roots []int
	for i, e := range entries {
		if e.uuid != "" {
			uuidSet[e.uuid] = struct{}{}
		}
		if e.parentUuid == "" {
			roots = append(roots, i)
		} else {
			children[e.parentUuid] = append(children[e.parentUuid], i)
		}
	}

	// A well-formed DAG has exactly one root and all parentUuid
	// references resolve to an existing entry's uuid. If not,
	// fall back to linear parsing to avoid dropping messages.
	if len(roots) != 1 {
		return parseLinearIflow(
			entries, sessionID, project, machine,
			parentSessionID, fileInfo, subagentMap,
			globalStart, globalEnd,
		)
	}
	for _, e := range entries {
		if e.parentUuid != "" {
			if _, ok := uuidSet[e.parentUuid]; !ok {
				return parseLinearIflow(
					entries, sessionID, project, machine,
					parentSessionID, fileInfo, subagentMap,
					globalStart, globalEnd,
				)
			}
		}
	}

	// Walk from the root, collecting branches.
	// branches[0] is the main branch; subsequent entries are forks.
	type branch struct {
		indices  []int
		parentID string // immediate parent session ID
	}

	var branches []branch

	// walkBranch follows the DAG from a starting index, collecting
	// all entries on the chosen path. At fork points, it either
	// follows the latest child (small gap) or splits (large gap).
	// ownerID is the session ID of the branch that owns this walk.
	var walkBranch func(startIdx int, ownerID string) []int
	var forkBranches []branch

	walkBranch = func(startIdx int, ownerID string) []int {
		var path []int
		current := startIdx

		for current >= 0 {
			path = append(path, current)
			uuid := entries[current].uuid
			kids := children[uuid]
			if len(kids) == 0 {
				break
			}
			if len(kids) == 1 {
				current = kids[0]
				continue
			}

			// Fork point: count user turns on first child's branch.
			firstChildTurns := countUserTurnsIflow(entries, children, kids[0])
			if firstChildTurns <= forkThreshold {
				// Small-gap retry: follow the last child.
				current = kids[len(kids)-1]
			} else {
				// Large-gap fork: follow first child on main,
				// collect other children as fork branches.
				for _, kid := range kids[1:] {
					// Normalize fork session ID with iflow: prefix
					rawSessionID := strings.TrimPrefix(sessionID, "iflow:")
					forkSID := "iflow:" + rawSessionID + "-" +
						entries[kid].uuid
					forkPath := walkBranch(kid, forkSID)
					forkBranches = append(
						forkBranches,
						branch{
							indices:  forkPath,
							parentID: ownerID,
						},
					)
				}
				current = kids[0]
			}
		}

		return path
	}

	mainPath := walkBranch(roots[0], sessionID)
	branches = append(
		branches,
		branch{indices: mainPath, parentID: parentSessionID},
	)
	branches = append(branches, forkBranches...)

	// Build results for each branch.
	var results []ParseResult

	for i, b := range branches {
		branchEntries := make([]dagEntryIflow, len(b.indices))
		for j, idx := range b.indices {
			branchEntries[j] = entries[idx]
		}

		messages, startedAt, endedAt := extractMessagesIflow(branchEntries)
		// Main session uses global bounds to capture timestamps
		// from non-message events (e.g. queue-operation).
		if i == 0 {
			startedAt = earlierTime(globalStart, startedAt)
			endedAt = laterTime(globalEnd, endedAt)
		}

		userCount := 0
		firstMsg := ""
		for _, m := range messages {
			if m.Role == RoleUser && m.Content != "" {
				userCount++
				if firstMsg == "" {
					firstMsg = truncate(
						strings.ReplaceAll(m.Content, "\n", " "), 300,
					)
				}
			}
		}

		sid := sessionID
		pSID := b.parentID
		relType := RelationshipType("")

		if i > 0 {
			// Fork session: ID derived from first entry's uuid,
			// parent is the branch that forked.
			firstEntry := entries[b.indices[0]]
			// Normalize fork session ID with iflow: prefix
			rawSessionID := strings.TrimPrefix(sessionID, "iflow:")
			sid = "iflow:" + rawSessionID + "-" + firstEntry.uuid
			relType = RelFork
		}

		sess := ParsedSession{
			ID:               sid,
			Project:          project,
			Machine:          machine,
			Agent:            AgentIflow,
			ParentSessionID:  pSID,
			RelationshipType: relType,
			FirstMessage:     firstMsg,
			StartedAt:        startedAt,
			EndedAt:          endedAt,
			MessageCount:     len(messages),
			UserMessageCount: userCount,
			File:             fileInfo,
		}

		results = append(results, ParseResult{
			Session:  sess,
			Messages: messages,
		})
	}

	return results, nil
}

// countUserTurnsIflow counts the number of user entries reachable from
// a starting index by following the first child at each node.
func countUserTurnsIflow(
	entries []dagEntryIflow,
	children map[string][]int,
	startIdx int,
) int {
	count := 0
	current := startIdx
	for current >= 0 {
		if entries[current].entryType == "user" {
			count++
		}
		uuid := entries[current].uuid
		kids := children[uuid]
		if len(kids) == 0 {
			break
		}
		current = kids[0]
	}
	return count
}

// extractMessagesIflow converts dagEntryIflow into ParsedMessages, applying
// the same filtering and content extraction as the original linear
// parser.
func extractMessagesIflow(entries []dagEntryIflow) (
	[]ParsedMessage, time.Time, time.Time,
) {
	var (
		messages  []ParsedMessage
		startedAt time.Time
		endedAt   time.Time
		ordinal   int
	)

	for _, e := range entries {
		if !e.timestamp.IsZero() {
			if startedAt.IsZero() {
				startedAt = e.timestamp
			}
			endedAt = e.timestamp
		}

		// Tier 1: skip system-injected user entries.
		if e.entryType == "user" {
			if gjson.Get(e.line, "isMeta").Bool() ||
				gjson.Get(e.line, "isCompactSummary").Bool() {
				continue
			}
		}

		content := gjson.Get(e.line, "message.content")
		text, hasThinking, hasToolUse, tcs, trs :=
			ExtractTextContent(content)
		if strings.TrimSpace(text) == "" && len(trs) == 0 {
			continue
		}

		// Tier 2: skip known system-injected patterns.
		if e.entryType == "user" && isIflowSystemMessage(text) {
			continue
		}

		messages = append(messages, ParsedMessage{
			Ordinal:       ordinal,
			Role:          RoleType(e.entryType),
			Content:       text,
			Timestamp:     e.timestamp,
			HasThinking:   hasThinking,
			HasToolUse:    hasToolUse,
			ContentLength: len(text),
			ToolCalls:     tcs,
			ToolResults:   trs,
		})
		ordinal++
	}

	return messages, startedAt, endedAt
}

// extractTimestampIflow parses the timestamp from a JSONL line.
func extractTimestampIflow(line string) time.Time {
	tsStr := gjson.Get(line, "timestamp").Str
	return parseTimestamp(tsStr)
}

// ExtractIflowProjectHints reads project-identifying metadata
// from an iFlow JSONL session file.
func ExtractIflowProjectHints(
	path string,
) (cwd, gitBranch string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	lr := newLineReader(f, maxLineSize)

	for {
		line, ok := lr.next()
		if !ok {
			break
		}
		if !gjson.Valid(line) {
			continue
		}
		if gjson.Get(line, "type").Str == "user" {
			// Skip meta/system-injected user entries
			if gjson.Get(line, "isMeta").Bool() ||
				gjson.Get(line, "isCompactSummary").Bool() {
				continue
			}

			if cwd == "" {
				cwd = gjson.Get(line, "cwd").Str
			}
			if gitBranch == "" {
				gitBranch = gjson.Get(line, "gitBranch").Str
			}

			// Only return early after capturing at least one non-empty hint
			// to avoid missing valid hints from later entries
			if cwd != "" || gitBranch != "" {
				return cwd, gitBranch
			}
		}
	}
	if err := lr.Err(); err != nil {
		logParseError(fmt.Sprintf("reading hints from %s: %v", path, err))
	}
	return cwd, gitBranch
}

// isIflowSystemMessage returns true if the content matches
// a known system-injected user message pattern.
func isIflowSystemMessage(content string) bool {
	trimmed := strings.TrimSpace(content)
	prefixes := [...]string{
		"This session is being continued",
		"[Request interrupted",
		"<task-notification>",
		"<command-message>",
		"<command-name>",
		"<local-command-",
		"Stop hook feedback:",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(trimmed, p) {
			return true
		}
	}
	return false
}
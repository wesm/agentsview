package parser

import (
	"strings"
	"time"
)

// AgentType identifies the AI agent that produced a session.
type AgentType string

const (
	AgentClaude   AgentType = "claude"
	AgentCodex    AgentType = "codex"
	AgentCopilot  AgentType = "copilot"
	AgentGemini   AgentType = "gemini"
	AgentOpenCode AgentType = "opencode"
	AgentCursor   AgentType = "cursor"
)

// RelationshipType describes how a session relates to its parent.
type RelationshipType string

const (
	RelNone         RelationshipType = ""
	RelContinuation RelationshipType = "continuation"
	RelSubagent     RelationshipType = "subagent"
	RelFork         RelationshipType = "fork"
)

// RoleType identifies the role of a message sender.
type RoleType string

const (
	RoleUser      RoleType = "user"
	RoleAssistant RoleType = "assistant"
)

// FileInfo holds file system metadata for a session source file.
type FileInfo struct {
	Path  string
	Size  int64
	Mtime int64
	Hash  string
}

// ParsedSession holds session metadata extracted from a JSONL file.
type ParsedSession struct {
	ID               string
	Project          string
	Machine          string
	Agent            AgentType
	ParentSessionID  string
	RelationshipType RelationshipType
	FirstMessage     string
	StartedAt        time.Time
	EndedAt          time.Time
	MessageCount     int
	UserMessageCount int
	File             FileInfo
}

// ParsedToolCall holds a single tool invocation extracted from
// a message.
type ParsedToolCall struct {
	ToolUseID         string // tool_use block id from session data
	ToolName          string // raw name from session data
	Category          string // normalized: Read, Edit, Write, Bash, etc.
	InputJSON         string // raw JSON of the input object
	SkillName         string // skill name when ToolName is "Skill"
	SubagentSessionID string // linked subagent session file (e.g. "agent-{task_id}")
}

// ParsedToolResult holds metadata about a tool result block in a
// user message (the response to a prior tool_use).
type ParsedToolResult struct {
	ToolUseID     string
	ContentLength int
}

// ParsedMessage holds a single extracted message.
type ParsedMessage struct {
	Ordinal       int
	Role          RoleType
	Content       string
	Timestamp     time.Time
	HasThinking   bool
	HasToolUse    bool
	ContentLength int
	ToolCalls     []ParsedToolCall
	ToolResults   []ParsedToolResult
}

// ParseResult pairs a parsed session with its messages.
type ParseResult struct {
	Session  ParsedSession
	Messages []ParsedMessage
}

// InferRelationshipTypes sets RelationshipType on results that have
// a ParentSessionID but no explicit type. Sessions with an "agent-"
// prefix are subagents; others are continuations.
func InferRelationshipTypes(results []ParseResult) {
	for i := range results {
		if results[i].Session.ParentSessionID == "" {
			continue
		}
		if results[i].Session.RelationshipType != RelNone {
			continue
		}
		if strings.HasPrefix(results[i].Session.ID, "agent-") {
			results[i].Session.RelationshipType = RelSubagent
		} else {
			results[i].Session.RelationshipType = RelContinuation
		}
	}
}

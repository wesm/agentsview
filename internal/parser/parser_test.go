package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
	"github.com/wesm/agentsview/internal/testjsonl"
)

func TestGetProjectName(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"simple name", "my-project", "my_project"},
		{"encoded path with code",
			"-Users-alice-code-my-app", "my_app"},
		{"encoded path with projects",
			"-Users-alice-projects-api-server", "api_server"},
		{"encoded path with repos",
			"-home-user-repos-frontend", "frontend"},
		{"encoded path without marker",
			"-Users-alice", "alice"},
		{"empty", "", ""},
		{"no prefix", "plain_name", "plain_name"},
		{"with src marker",
			"-Users-alice-src-my-lib", "my_lib"},
		{"multi-word after marker",
			"-Users-alice-code-my-cool-project", "my_cool_project"},
		{"deeply nested",
			"-Users-alice-code-org-team-repo", "org_team_repo"},
		{"unicode components",
			"-Users-alice-code-café-app", "café_app"},
		{"trailing dash",
			"-Users-alice-code-myapp-", "myapp_"},
		{"double dashes",
			"-Users-alice-code--my-app", "_my_app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetProjectName(tt.dir)
			if got != tt.want {
				t.Errorf("GetProjectName(%q) = %q, want %q",
					tt.dir, got, tt.want)
			}
		})
	}
}

func TestExtractProjectFromCwd(t *testing.T) {
	tests := []struct {
		cwd  string
		want string
	}{
		{"/Users/alice/code/my-app", "my_app"},
		{"/home/user/projects/api-server", "api_server"},
		{"", ""},
		{"/", ""},
		{".", ""},
		{"..", ""},
	}

	// Platform-specific behavior for Windows paths
	if runtime.GOOS == "windows" {
		tests = append(tests, []struct{ cwd, want string }{
			{`C:\Users\me\my-app`, "my_app"},
			{`D:\projects\frontend`, "frontend"},
			{`/mixed\path/to\project`, "project"},
		}...)
	} else {
		tests = append(tests, []struct{ cwd, want string }{
			{`C:\Users\me\my-app`, ""},
			{`D:\projects\frontend`, ""},
			{`/mixed\path/to\project`, ""},
		}...)
	}

	for _, tt := range tests {
		t.Run(tt.cwd, func(t *testing.T) {
			got := ExtractProjectFromCwd(tt.cwd)
			if got != tt.want {
				t.Errorf("ExtractProjectFromCwd(%q) = %q, want %q",
					tt.cwd, got, tt.want)
			}
		})
	}
}

func TestNeedsProjectReparse(t *testing.T) {
	tests := []struct {
		project string
		want    bool
	}{
		{"my_project", false},
		{"_Users_alice_code_app", true},
		{"_home_user_project", true},
		{"_private_var_folders", true},
		{"good_project_var_folders_ok", true},
		{"good_project", false},
		{"_var_folders_xx_temp", true},
		{"_private_tmp_build", true},
		{"_tmp_workspace", true},
		{"normal_var_project", false},
	}

	for _, tt := range tests {
		t.Run(tt.project, func(t *testing.T) {
			got := NeedsProjectReparse(tt.project)
			if got != tt.want {
				t.Errorf("NeedsProjectReparse(%q) = %v, want %v",
					tt.project, got, tt.want)
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name          string
		json          string
		wantText      string
		wantThink     bool
		wantToolUse   bool
		wantToolCalls []ParsedToolCall
	}{
		{
			"plain string",
			`"Hello world"`,
			"Hello world", false, false, nil,
		},
		{
			"text block array",
			`[{"type":"text","text":"Hi"}]`,
			"Hi", false, false, nil,
		},
		{
			"thinking block",
			`[{"type":"thinking","thinking":"Let me think..."}]`,
			"[Thinking]\nLet me think...", true, false, nil,
		},
		{
			"tool_use block",
			`[{"type":"tool_use","name":"Read","input":{"file_path":"test.go"}}]`,
			"[Read: test.go]", false, true,
			[]ParsedToolCall{{ToolName: "Read", Category: "Read"}},
		},
		{
			"mixed blocks",
			`[{"type":"text","text":"Looking at"},{"type":"tool_use","name":"Bash","input":{"command":"ls","description":"list files"}}]`,
			"Looking at\n[Bash: list files]\n$ ls", false, true,
			[]ParsedToolCall{{ToolName: "Bash", Category: "Bash"}},
		},
		{
			"multiple tool_use blocks",
			`[{"type":"tool_use","name":"Read","input":{"file_path":"a.go"}},{"type":"tool_use","name":"Grep","input":{"pattern":"TODO"}}]`,
			"[Read: a.go]\n[Grep: TODO]", false, true,
			[]ParsedToolCall{
				{ToolName: "Read", Category: "Read"},
				{ToolName: "Grep", Category: "Grep"},
			},
		},
		{
			"tool_use with id and input",
			`[{"type":"tool_use","id":"toolu_123","name":"Read","input":{"file_path":"main.go"}}]`,
			"[Read: main.go]", false, true,
			[]ParsedToolCall{{ToolUseID: "toolu_123", ToolName: "Read", Category: "Read", InputJSON: `{"file_path":"main.go"}`}},
		},
		{
			"Skill tool extracts skill_name",
			`[{"type":"tool_use","id":"toolu_456","name":"Skill","input":{"skill":"superpowers:brainstorming"}}]`,
			"[Skill: superpowers:brainstorming]", false, true,
			[]ParsedToolCall{{ToolUseID: "toolu_456", ToolName: "Skill", Category: "Tool", InputJSON: `{"skill":"superpowers:brainstorming"}`, SkillName: "superpowers:brainstorming"}},
		},
		{
			"tool_use with empty name",
			`[{"type":"tool_use","name":"","input":{}}]`,
			"[Tool: ]", false, true, nil,
		},
		{
			"empty array",
			`[]`,
			"", false, false, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gjson.Parse(tt.json)
			text, hasThinking, hasToolUse, tcs, _ :=
				ExtractTextContent(result)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if hasThinking != tt.wantThink {
				t.Errorf("hasThinking = %v, want %v",
					hasThinking, tt.wantThink)
			}
			if hasToolUse != tt.wantToolUse {
				t.Errorf("hasToolUse = %v, want %v",
					hasToolUse, tt.wantToolUse)
			}
			assertToolCalls(t, tcs, tt.wantToolCalls)
		})
	}
}

func TestExtractToolResults(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantResults []ParsedToolResult
	}{
		{
			"no tool_result blocks",
			`[{"type":"text","text":"hello"}]`,
			nil,
		},
		{
			"single tool_result",
			`[{"type":"tool_result","tool_use_id":"toolu_123","content":"file contents here"}]`,
			[]ParsedToolResult{{ToolUseID: "toolu_123", ContentLength: 18}},
		},
		{
			"tool_result with array content",
			`[{"type":"tool_result","tool_use_id":"toolu_456","content":[{"type":"text","text":"output data"}]}]`,
			[]ParsedToolResult{{ToolUseID: "toolu_456", ContentLength: 11}},
		},
		{
			"multiple tool_results",
			`[{"type":"tool_result","tool_use_id":"toolu_1","content":"abc"},{"type":"tool_result","tool_use_id":"toolu_2","content":"defgh"}]`,
			[]ParsedToolResult{
				{ToolUseID: "toolu_1", ContentLength: 3},
				{ToolUseID: "toolu_2", ContentLength: 5},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gjson.Parse(tt.json)
			_, _, _, _, trs := ExtractTextContent(result)
			if len(trs) != len(tt.wantResults) {
				t.Fatalf("tool_results count = %d, want %d",
					len(trs), len(tt.wantResults))
			}
			for i := range tt.wantResults {
				if trs[i].ToolUseID != tt.wantResults[i].ToolUseID {
					t.Errorf("[%d].ToolUseID = %q, want %q",
						i, trs[i].ToolUseID, tt.wantResults[i].ToolUseID)
				}
				if trs[i].ContentLength != tt.wantResults[i].ContentLength {
					t.Errorf("[%d].ContentLength = %d, want %d",
						i, trs[i].ContentLength, tt.wantResults[i].ContentLength)
				}
			}
		})
	}
}

func TestFormatToolUseVariants(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"Read",
			`{"type":"tool_use","name":"Read","input":{"file_path":"main.go"}}`,
			"[Read: main.go]",
		},
		{
			"Glob",
			`{"type":"tool_use","name":"Glob","input":{"pattern":"*.ts","path":"src"}}`,
			"[Glob: *.ts in src]",
		},
		{
			"Glob default path",
			`{"type":"tool_use","name":"Glob","input":{"pattern":"*.ts"}}`,
			"[Glob: *.ts in .]",
		},
		{
			"Grep",
			`{"type":"tool_use","name":"Grep","input":{"pattern":"TODO"}}`,
			"[Grep: TODO]",
		},
		{
			"Edit",
			`{"type":"tool_use","name":"Edit","input":{"file_path":"config.go"}}`,
			"[Edit: config.go]",
		},
		{
			"Write",
			`{"type":"tool_use","name":"Write","input":{"file_path":"new.go"}}`,
			"[Write: new.go]",
		},
		{
			"Bash with description",
			`{"type":"tool_use","name":"Bash","input":{"command":"go test ./...","description":"run tests"}}`,
			"[Bash: run tests]\n$ go test ./...",
		},
		{
			"Bash without description",
			`{"type":"tool_use","name":"Bash","input":{"command":"ls"}}`,
			"[Bash]\n$ ls",
		},
		{
			"Task",
			`{"type":"tool_use","name":"Task","input":{"description":"explore","subagent_type":"Explore"}}`,
			"[Task: explore (Explore)]",
		},
		{
			"EnterPlanMode",
			`{"type":"tool_use","name":"EnterPlanMode","input":{}}`,
			"[Entering Plan Mode]",
		},
		{
			"ExitPlanMode",
			`{"type":"tool_use","name":"ExitPlanMode","input":{}}`,
			"[Exiting Plan Mode]",
		},
		{
			"Unknown tool",
			`{"type":"tool_use","name":"CustomTool","input":{}}`,
			"[Tool: CustomTool]",
		},
		{
			"AskUserQuestion",
			`{"type":"tool_use","name":"AskUserQuestion","input":{"questions":[{"question":"Which approach?","options":[{"label":"A","description":"First option"},{"label":"B","description":"Second option"}]}]}}`,
			"[Question: AskUserQuestion]\n  Which approach?\n    - A: First option\n    - B: Second option",
		},
		{
			"TodoWrite",
			`{"type":"tool_use","name":"TodoWrite","input":{"todos":[{"content":"Fix bug","status":"completed"},{"content":"Write tests","status":"in_progress"},{"content":"Deploy","status":"pending"}]}}`,
			"[Todo List]\n  ✓ Fix bug\n  → Write tests\n  ○ Deploy",
		},
		{
			"TodoWrite unknown status",
			`{"type":"tool_use","name":"TodoWrite","input":{"todos":[{"content":"Something","status":"unknown"}]}}`,
			"[Todo List]\n  ○ Something",
		},
		{
			"Skill",
			`{"type":"tool_use","name":"Skill","input":{"skill":"superpowers:brainstorming"}}`,
			"[Skill: superpowers:brainstorming]",
		},
		{
			"Skill with args",
			`{"type":"tool_use","name":"Skill","input":{"skill":"commit","args":"-m 'Fix bug'"}}`,
			"[Skill: commit]",
		},
		{
			"TaskCreate with subject",
			`{"type":"tool_use","name":"TaskCreate","input":{"subject":"Fix authentication bug","description":"Debug the login flow"}}`,
			"[TaskCreate: Fix authentication bug]",
		},
		{
			"TaskUpdate with status",
			`{"type":"tool_use","name":"TaskUpdate","input":{"taskId":"5","status":"completed"}}`,
			"[TaskUpdate: #5 completed]",
		},
		{
			"TaskGet",
			`{"type":"tool_use","name":"TaskGet","input":{"taskId":"3"}}`,
			"[TaskGet: #3]",
		},
		{
			"TaskList",
			`{"type":"tool_use","name":"TaskList","input":{}}`,
			"[TaskList]",
		},
		{
			"SendMessage",
			`{"type":"tool_use","name":"SendMessage","input":{"type":"message","recipient":"researcher","content":"hello"}}`,
			"[SendMessage: message to researcher]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := gjson.Parse(tt.json)
			got := formatToolUse(block)
			if got != tt.want {
				t.Errorf("formatToolUse = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantUTC time.Time
		wantOK  bool
	}{
		{
			"empty string",
			"",
			time.Time{},
			false,
		},
		{
			"RFC3339 UTC",
			"2024-01-15T10:30:00Z",
			testJan15_1030UTC,
			true,
		},
		{
			"RFC3339Nano UTC",
			"2024-01-15T10:30:00.123456789Z",
			time.Date(
				2024, 1, 15, 10, 30, 0, 123456789,
				time.UTC,
			),
			true,
		},
		{
			"milliseconds with Z",
			"2024-01-15T10:30:00.500Z",
			time.Date(
				2024, 1, 15, 10, 30, 0, 500000000,
				time.UTC,
			),
			true,
		},
		{
			"positive timezone offset",
			"2024-01-15T15:30:00+05:00",
			testJan15_1030UTC,
			true,
		},
		{
			"negative timezone offset",
			"2024-01-15T03:30:00-07:00",
			testJan15_1030UTC,
			true,
		},
		{
			"millis with offset",
			"2024-01-15T15:30:00.500+05:00",
			time.Date(
				2024, 1, 15, 10, 30, 0, 500000000,
				time.UTC,
			),
			true,
		},
		{
			"space-separated datetime",
			"2024-01-15 10:30:00",
			testJan15_1030UTC,
			true,
		},
		{
			"unparseable value",
			"not-a-timestamp",
			time.Time{},
			false,
		},
		{
			"date only",
			"2024-01-15",
			time.Time{},
			false,
		},
		{
			"unix epoch number string",
			"1705315800",
			time.Time{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestamp(tt.input)
			if tt.wantOK {
				if got.IsZero() {
					t.Fatalf(
						"parseTimestamp(%q) = zero, want %v",
						tt.input, tt.wantUTC,
					)
				}
				if !got.Equal(tt.wantUTC) {
					t.Errorf(
						"parseTimestamp(%q) = %v, want %v",
						tt.input, got, tt.wantUTC,
					)
				}
				if got.Location() != time.UTC {
					t.Errorf(
						"parseTimestamp(%q) location = %v, want UTC",
						tt.input, got.Location(),
					)
				}
			} else {
				if !got.IsZero() {
					t.Errorf(
						"parseTimestamp(%q) = %v, want zero",
						tt.input, got,
					)
				}
			}
		})
	}
}

func TestClaudeSessionTimestampSemantics(t *testing.T) {
	t.Run("snapshot.timestamp fallback", func(t *testing.T) {
		content := testjsonl.ClaudeSnapshotJSON("2024-06-15T12:00:00Z")
		sess, msgs := parseClaudeTestFile(
			t, "ts-fallback.jsonl", content, "proj",
		)
		wantTS := time.Date(
			2024, 6, 15, 12, 0, 0, 0, time.UTC,
		)
		assertTimestamp(t, sess.StartedAt, wantTS)
		assertTimestamp(t, sess.EndedAt, wantTS)

		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		assertTimestamp(t, msgs[0].Timestamp, wantTS)
	})

	t.Run("offset timestamp normalized to UTC", func(t *testing.T) {
		content := testjsonl.ClaudeUserJSON("hello", "2024-06-15T17:00:00+05:00")
		sess, msgs := parseClaudeTestFile(
			t, "ts-offset.jsonl", content, "proj",
		)
		wantUTC := time.Date(
			2024, 6, 15, 12, 0, 0, 0, time.UTC,
		)
		assertTimestamp(t, sess.StartedAt, wantUTC)

		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		assertTimestamp(t, msgs[0].Timestamp, wantUTC)
	})

	t.Run("unparseable timestamp yields zero", func(t *testing.T) {
		content := testjsonl.ClaudeUserJSON("hello", "garbage")
		sess, msgs := parseClaudeTestFile(
			t, "ts-bad.jsonl", content, "proj",
		)
		assertZeroTimestamp(t, sess.StartedAt, "StartedAt")
		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		assertZeroTimestamp(t, msgs[0].Timestamp, "msg timestamp")
	})

	t.Run("invalid primary but valid fallback logs no warning", func(t *testing.T) {
		content := `{"type":"user","timestamp":"garbage","snapshot":{"timestamp":"2024-06-15T12:00:00Z"},"message":{"content":"hello"}}` + "\n"
		buf := captureLog(t)

		sess, msgs := parseClaudeTestFile(
			t, "ts-mixed.jsonl", content, "proj",
		)

		wantTS := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		assertTimestamp(t, sess.StartedAt, wantTS)
		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		assertTimestamp(t, msgs[0].Timestamp, wantTS)
		assertLogEmpty(t, buf)
	})

	t.Run("both timestamps invalid logs warning", func(t *testing.T) {
		content := `{"type":"user","timestamp":"garbage1","snapshot":{"timestamp":"garbage2"},"message":{"content":"hello"}}` + "\n"
		buf := captureLog(t)

		sess, _ := parseClaudeTestFile(
			t, "ts-invalid-both.jsonl", content, "proj",
		)

		assertZeroTimestamp(t, sess.StartedAt, "StartedAt")
		assertLogContains(t, buf,
			"unparseable timestamp", "garbage1",
		)
	})

	t.Run("very long invalid timestamp is truncated in log", func(t *testing.T) {
		longInvalid := strings.Repeat("x", 200)
		content := `{"type":"user","timestamp":"` + longInvalid + `","message":{"content":"hello"}}` + "\n"
		buf := captureLog(t)

		path := createTestFile(t, "ts-long-invalid.jsonl", content)
		_, _, err := ParseClaudeSession(
			path, "proj", "local",
		)
		if err != nil {
			t.Fatalf("ParseClaudeSession: %v", err)
		}

		assertLogContains(t, buf,
			"unparseable timestamp", "x...",
		)
		if buf.Len() > 1000 {
			t.Errorf("log output too long: %d bytes", buf.Len())
		}
		assertLogNotContains(t, buf, longInvalid)
	})
}

func createTestFile(
	t *testing.T, name, content string,
) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(
		path, []byte(content), 0o644,
	); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return path
}

func TestIsClaudeSystemMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"context continuation",
			"This session is being continued from a previous conversation.",
			true},
		{"request interrupted",
			"[Request interrupted by user]", true},
		{"task-notification",
			"<task-notification>some data</task-notification>",
			true},
		{"command-message",
			"<command-message>foo</command-message>", true},
		{"command-name",
			"<command-name>commit</command-name>", true},
		{"local-command tag",
			"<local-command-result>ok</local-command-result>",
			true},
		{"stop hook feedback",
			"Stop hook feedback: rejected by policy", true},
		{"leading whitespace trimmed",
			"  \n This session is being continued...",
			true},
		{"leading tabs trimmed",
			"\t<command-name>commit</command-name>",
			true},
		{"normal user message",
			"Fix the login bug", false},
		{"implement plan is not filtered",
			"Implement the following plan:\n## Steps",
			false},
		{"empty string", "", false},
		{"partial prefix mismatch",
			"This session was great", false},
		{"assistant-like content not matched",
			"Looking at the auth module...", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isClaudeSystemMessage(tt.content)
			if got != tt.want {
				t.Errorf(
					"isClaudeSystemMessage(%q) = %v, want %v",
					tt.content, got, tt.want,
				)
			}
		})
	}
}

func TestParseClaudeSession(t *testing.T) {
	validContent := testjsonl.JoinJSONL(
		testjsonl.ClaudeUserJSON("Fix the login bug", tsEarly, "/Users/alice/code/my-app"),
		testjsonl.ClaudeAssistantJSON([]map[string]any{
			{"type": "text", "text": "Looking at the auth module..."},
			{"type": "tool_use", "name": "Read", "input": map[string]string{"file_path": "src/auth.ts"}},
		}, tsEarlyS5),
		testjsonl.ClaudeUserJSON("That looks right", tsLate),
		testjsonl.ClaudeAssistantJSON([]map[string]any{
			{"type": "text", "text": "Applied the fix."},
		}, tsLateS5),
	)
	longMsg := generateLargeString(400)
	goodUTF8 := testjsonl.ClaudeUserJSON("valid message", tsZero) + "\n"
	// Keeping badUTF8 manual as it tests raw byte corruption which is hard to structured-build safely
	badUTF8 := `{"type":"user","timestamp":"` + tsZeroS1 + `","message":{"content":"bad ` +
		string([]byte{0xff, 0xfe}) + `"}}` + "\n"
	bigMsg := generateLargeString(1024 * 1024)

	tests := []struct {
		name         string
		fileName     string // defaults to "test.jsonl"
		content      string
		wantMsgCount int
		wantErr      bool
		check        func(t *testing.T, sess ParsedSession, msgs []ParsedMessage)
	}{
		{
			name:         "valid session",
			content:      validContent,
			wantMsgCount: 4,
			check: func(t *testing.T, sess ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				assertSessionMeta(t, &sess, "test", "my_app", AgentClaude)
				if sess.FirstMessage != "Fix the login bug" {
					t.Errorf("first_message = %q", sess.FirstMessage)
				}
				assertMessage(t, msgs[0], RoleUser, "")
				assertMessage(t, msgs[1], RoleAssistant, "")

				if !msgs[1].HasToolUse {
					t.Error("msg[1] should have tool_use")
				}
				if msgs[0].Ordinal != 0 || msgs[1].Ordinal != 1 {
					t.Errorf("ordinals: %d, %d",
						msgs[0].Ordinal, msgs[1].Ordinal)
				}
			},
		},
		{
			name:         "hyphenated filename derives session ID",
			fileName:     "my-test-session.jsonl",
			content:      validContent,
			wantMsgCount: 4,
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.ID != "my-test-session" {
					t.Errorf(
						"session ID = %q, want %q",
						sess.ID, "my-test-session",
					)
				}
			},
		},
		{
			name:         "empty file",
			content:      "",
			wantMsgCount: 0,
		},
		{
			name: "skips blank content",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeUserJSON("", tsZero),
				testjsonl.ClaudeUserJSON("  ", tsZeroS1),
				testjsonl.ClaudeUserJSON("actual message", tsZeroS2),
			),
			wantMsgCount: 1,
		},
		{
			name:         "truncates long first message",
			content:      testjsonl.ClaudeUserJSON(longMsg, tsZero) + "\n",
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if len(sess.FirstMessage) != 303 {
					t.Errorf("first_message length = %d, want 303",
						len(sess.FirstMessage))
				}
			},
		},
		{
			name: "skips invalid JSON lines",
			content: "not valid json\n" +
				testjsonl.ClaudeUserJSON("hello", tsZero) + "\n" +
				"also not valid\n",
			wantMsgCount: 1,
		},
		{
			name:         "malformed UTF-8",
			content:      goodUTF8 + badUTF8,
			wantMsgCount: -1, // at least 1
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.MessageCount < 1 {
					t.Errorf("expected at least 1 message, got %d",
						sess.MessageCount)
				}
			},
		},
		{
			name:         "very large message",
			content:      testjsonl.ClaudeUserJSON(bigMsg, tsZero) + "\n",
			wantMsgCount: 1,
			check: func(t *testing.T, _ ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if msgs[0].ContentLength != 1024*1024 {
					t.Errorf("content_length = %d, want %d",
						msgs[0].ContentLength, 1024*1024)
				}
			},
		},
		{
			name: "skips empty lines in file",
			content: "\n\n" +
				testjsonl.ClaudeUserJSON("msg1", tsZero) +
				"\n   \n\t\n" +
				testjsonl.ClaudeAssistantJSON([]map[string]any{{"type": "text", "text": "reply"}}, tsZeroS1) +
				"\n\n",
			wantMsgCount: 2,
		},
		{
			name: "skips partial/truncated JSON",
			content: testjsonl.ClaudeUserJSON("first", tsZero) + "\n" +
				`{"type":"user","truncated` + "\n" +
				testjsonl.ClaudeAssistantJSON([]map[string]any{{"type": "text", "text": "last"}}, tsZeroS2) + "\n",
			wantMsgCount: 2,
		},
		{
			name: "skips isMeta user messages",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeMetaUserJSON("meta context", tsZero, true, false),
				testjsonl.ClaudeUserJSON("real question", tsZeroS1),
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if sess.FirstMessage != "real question" {
					t.Errorf("first_message = %q, want %q",
						sess.FirstMessage, "real question")
				}
			},
		},
		{
			name: "skips isCompactSummary user messages",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeMetaUserJSON("summary of prior turns", tsZero, false, true),
				testjsonl.ClaudeUserJSON("actual prompt", tsZeroS1),
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if sess.FirstMessage != "actual prompt" {
					t.Errorf("first_message = %q, want %q",
						sess.FirstMessage, "actual prompt")
				}
			},
		},
		{
			name: "skips content-heuristic system messages",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeUserJSON("This session is being continued from a previous conversation.", tsZero),
				testjsonl.ClaudeUserJSON("[Request interrupted by user]", tsZeroS1),
				testjsonl.ClaudeUserJSON("<task-notification>data</task-notification>", tsZeroS2),
				testjsonl.ClaudeUserJSON("<command-message>x</command-message>", "2024-01-01T00:00:03Z"),
				testjsonl.ClaudeUserJSON("<command-name>commit</command-name>", "2024-01-01T00:00:04Z"),
				testjsonl.ClaudeUserJSON("<local-command-result>ok</local-command-result>", "2024-01-01T00:00:05Z"),
				testjsonl.ClaudeUserJSON("Stop hook feedback: rejected", "2024-01-01T00:00:06Z"),
				testjsonl.ClaudeUserJSON("real user message", "2024-01-01T00:00:07Z"),
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if msgs[0].Content != "real user message" {
					t.Errorf("content = %q", msgs[0].Content)
				}
				if sess.FirstMessage != "real user message" {
					t.Errorf("first_message = %q", sess.FirstMessage)
				}
			},
		},
		{
			name: "sessionId != fileId sets ParentSessionID",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeUserWithSessionIDJSON(
					"hello", tsZero, "parent-uuid",
				),
				testjsonl.ClaudeAssistantJSON([]map[string]any{
					{"type": "text", "text": "hi"},
				}, tsZeroS1),
			),
			wantMsgCount: 2,
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.ParentSessionID != "parent-uuid" {
					t.Errorf("ParentSessionID = %q, want %q",
						sess.ParentSessionID, "parent-uuid")
				}
			},
		},
		{
			name: "sessionId == fileId yields empty ParentSessionID",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeUserWithSessionIDJSON(
					"hello", tsZero, "test",
				),
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.ParentSessionID != "" {
					t.Errorf(
						"ParentSessionID = %q, want empty",
						sess.ParentSessionID,
					)
				}
			},
		},
		{
			name: "no sessionId field yields empty ParentSessionID",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeUserJSON("hello", tsZero),
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.ParentSessionID != "" {
					t.Errorf(
						"ParentSessionID = %q, want empty",
						sess.ParentSessionID,
					)
				}
			},
		},
		{
			name: "assistant with system-like content not filtered",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeUserJSON("hello", tsZero),
				testjsonl.ClaudeAssistantJSON([]map[string]any{
					{"type": "text", "text": "This session is being continued from a previous conversation."},
				}, tsZeroS1),
			),
			wantMsgCount: 2,
		},
		{
			name: "firstMsg from first non-system user message",
			content: testjsonl.JoinJSONL(
				testjsonl.ClaudeMetaUserJSON("context data", tsZero, true, false),
				testjsonl.ClaudeUserJSON("This session is being continued from a previous conversation.", tsZeroS1),
				testjsonl.ClaudeUserJSON("Fix the auth bug", tsZeroS2),
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.FirstMessage != "Fix the auth bug" {
					t.Errorf("first_message = %q, want %q",
						sess.FirstMessage, "Fix the auth bug")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName := tt.fileName
			if fileName == "" {
				fileName = "test.jsonl"
			}
			path := createTestFile(t, fileName, tt.content)
			sess, msgs, err := ParseClaudeSession(
				path, "my_app", "local",
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantMsgCount >= 0 {
				assertMessageCount(t, len(msgs), tt.wantMsgCount)
				assertMessageCount(t, sess.MessageCount, tt.wantMsgCount)
			}
			if tt.check != nil {
				tt.check(t, sess, msgs)
			}
		})
	}
}

func TestParseCodexSession(t *testing.T) {
	execContent := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON("abc", "/tmp", "codex_exec", tsEarly),
		testjsonl.CodexMsgJSON("user", "test", tsEarlyS1),
	)

	tests := []struct {
		name        string
		fileName    string // defaults to "test.jsonl"
		content     string
		includeExec bool
		wantNil     bool
		wantID      string
		wantMsgs    int
		check       func(t *testing.T, sess *ParsedSession, msgs []ParsedMessage)
	}{
		{
			name: "standard session",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("abc-123", "/Users/alice/code/my-api", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "Add rate limiting", tsEarlyS1),
				testjsonl.CodexMsgJSON("assistant", "I'll add rate limiting to the API.", tsEarlyS5),
			),
			wantID:   "codex:abc-123",
			wantMsgs: 2,
			check: func(t *testing.T, sess *ParsedSession, _ []ParsedMessage) {
				t.Helper()
				assertSessionMeta(t, sess, "codex:abc-123", "my_api", AgentCodex)
			},
		},
		{
			name:        "skips exec originator",
			content:     execContent,
			includeExec: false,
			wantNil:     true,
		},
		{
			name:        "includes exec when requested",
			content:     execContent,
			includeExec: true,
			wantID:      "codex:abc",
			wantMsgs:    1,
		},
		{
			name: "skips system messages",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("abc", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "# AGENTS.md\nsome instructions", tsEarlyS1),
				testjsonl.CodexMsgJSON("user", "<environment_context>stuff</environment_context>", "2024-01-01T10:00:02Z"),
				testjsonl.CodexMsgJSON("user", "<INSTRUCTIONS>ignore</INSTRUCTIONS>", "2024-01-01T10:00:03Z"),
				testjsonl.CodexMsgJSON("user", "Actual user message", "2024-01-01T10:00:04Z"),
			),
			wantID:   "codex:abc",
			wantMsgs: 1,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if msgs[0].Content != "Actual user message" {
					t.Errorf("content = %q", msgs[0].Content)
				}
			},
		},
		{
			name: "fallback ID from filename",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexMsgJSON("user", "hello", tsEarlyS1),
			),
			wantID:   "codex:test",
			wantMsgs: 1,
		},
		{
			name:     "fallback ID from hyphenated filename",
			fileName: "my-codex-session.jsonl",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexMsgJSON("user", "hello", tsEarlyS1),
			),
			wantID:   "codex:my-codex-session",
			wantMsgs: 1,
		},
		{
			name: "function calls",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("fc-1", "/Users/alice/code/app", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "Run the tests", tsEarlyS1),
				testjsonl.CodexFunctionCallJSON("shell_command", "Running tests", tsEarlyS5),
				testjsonl.CodexFunctionCallJSON("apply_patch", "Fixing typo", tsLate),
			),
			wantID:   "codex:fc-1",
			wantMsgs: 3,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				// user msg
				if msgs[0].Role != RoleUser {
					t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
				}
				if msgs[0].HasToolUse {
					t.Error("msgs[0] should not have tool_use")
				}
				// shell_command
				if msgs[1].Role != RoleAssistant {
					t.Errorf("msgs[1].Role = %q, want assistant", msgs[1].Role)
				}
				if !msgs[1].HasToolUse {
					t.Error("msgs[1] should have tool_use")
				}
				assertToolCalls(t, msgs[1].ToolCalls, []ParsedToolCall{
					{ToolName: "shell_command", Category: "Bash"},
				})
				if msgs[1].Content != "[Bash: Running tests]" {
					t.Errorf("msgs[1].Content = %q", msgs[1].Content)
				}
				// apply_patch
				if !msgs[2].HasToolUse {
					t.Error("msgs[2] should have tool_use")
				}
				assertToolCalls(t, msgs[2].ToolCalls, []ParsedToolCall{
					{ToolName: "apply_patch", Category: "Edit"},
				})
				// ordinals
				for i, m := range msgs {
					if m.Ordinal != i {
						t.Errorf("msgs[%d].Ordinal = %d", i, m.Ordinal)
					}
				}
			},
		},
		{
			name: "function call no name skipped",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("fc-2", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "hello", tsEarlyS1),
				testjsonl.CodexFunctionCallJSON("", "", tsEarlyS5),
			),
			wantID:   "codex:fc-2",
			wantMsgs: 1,
		},
		{
			name: "mixed content and function calls",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("fc-3", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "Fix it", tsEarlyS1),
				testjsonl.CodexMsgJSON("assistant", "Looking at it", tsEarlyS5),
				testjsonl.CodexFunctionCallJSON("shell_command", "Running rg", tsLate),
				testjsonl.CodexMsgJSON("assistant", "Found the issue", tsLateS5),
			),
			wantID:   "codex:fc-3",
			wantMsgs: 4,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				// ordinals sequential
				for i, m := range msgs {
					if m.Ordinal != i {
						t.Errorf("msgs[%d].Ordinal = %d", i, m.Ordinal)
					}
				}
				// only function_call msg has HasToolUse
				for i, m := range msgs {
					wantTool := i == 2
					if m.HasToolUse != wantTool {
						t.Errorf("msgs[%d].HasToolUse = %v, want %v",
							i, m.HasToolUse, wantTool)
					}
				}
			},
		},
		{
			name: "function call without summary",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("fc-4", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "do it", tsEarlyS1),
				testjsonl.CodexFunctionCallJSON("exec_command", "", tsEarlyS5),
			),
			wantID:   "codex:fc-4",
			wantMsgs: 2,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if msgs[1].Content != "[Bash]" {
					t.Errorf("content = %q, want %q",
						msgs[1].Content, "[Bash]")
				}
			},
		},
		{
			name: "exec_command arguments include command detail",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("fc-args-1", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "inspect files", tsEarlyS1),
				testjsonl.CodexFunctionCallArgsJSON(
					"exec_command",
					`{"cmd":"rg --files","workdir":"/tmp"}`,
					tsEarlyS5,
				),
			),
			wantID:   "codex:fc-args-1",
			wantMsgs: 2,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if msgs[1].Content != "[Bash]\n$ rg --files" {
					t.Errorf("content = %q", msgs[1].Content)
				}
			},
		},
		{
			name: "apply_patch arguments summarize edited files",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("fc-args-2", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "apply patch", tsEarlyS1),
				testjsonl.CodexFunctionCallArgsJSON(
					"apply_patch",
					map[string]any{
						"patch": strings.Join([]string{
							"*** Begin Patch",
							"*** Update File: internal/parser/codex.go",
							"*** Update File: internal/parser/parser_test.go",
							"*** End Patch",
						}, "\n"),
					},
					tsEarlyS5,
				),
			),
			wantID:   "codex:fc-args-2",
			wantMsgs: 2,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				want := "[Edit: internal/parser/codex.go (+1 more)]\n" +
					"internal/parser/codex.go\ninternal/parser/parser_test.go"
				if msgs[1].Content != want {
					t.Errorf("content = %q, want %q",
						msgs[1].Content, want)
				}
			},
		},
		{
			name: "empty arguments falls through to input",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON(
					"fc-empty-args", "/tmp", "user", tsEarly,
				),
				testjsonl.CodexMsgJSON(
					"user", "run command", tsEarlyS1,
				),
				testjsonl.CodexFunctionCallFieldsJSON(
					"exec_command",
					map[string]any{},
					`{"cmd":"ls -la"}`,
					tsEarlyS5,
				),
			),
			wantID:   "codex:fc-empty-args",
			wantMsgs: 2,
			check: func(
				t *testing.T, _ *ParsedSession,
				msgs []ParsedMessage,
			) {
				t.Helper()
				want := "[Bash]\n$ ls -la"
				if msgs[1].Content != want {
					t.Errorf(
						"content = %q, want %q",
						msgs[1].Content, want,
					)
				}
			},
		},
		{
			name: "empty array arguments falls through to input",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON(
					"fc-empty-arr", "/tmp", "user", tsEarly,
				),
				testjsonl.CodexMsgJSON(
					"user", "run command", tsEarlyS1,
				),
				testjsonl.CodexFunctionCallFieldsJSON(
					"exec_command",
					[]any{},
					`{"cmd":"echo hello"}`,
					tsEarlyS5,
				),
			),
			wantID:   "codex:fc-empty-arr",
			wantMsgs: 2,
			check: func(
				t *testing.T, _ *ParsedSession,
				msgs []ParsedMessage,
			) {
				t.Helper()
				want := "[Bash]\n$ echo hello"
				if msgs[1].Content != want {
					t.Errorf(
						"content = %q, want %q",
						msgs[1].Content, want,
					)
				}
			},
		},
		{
			name: "write_stdin formats with session and chars",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON(
					"fc-stdin", "/tmp", "user", tsEarly,
				),
				testjsonl.CodexMsgJSON(
					"user", "send input", tsEarlyS1,
				),
				testjsonl.CodexFunctionCallArgsJSON(
					"write_stdin",
					map[string]any{
						"session_id": "sess-42",
						"chars":      "yes\n",
					},
					tsEarlyS5,
				),
			),
			wantID:   "codex:fc-stdin",
			wantMsgs: 2,
			check: func(
				t *testing.T, _ *ParsedSession,
				msgs []ParsedMessage,
			) {
				t.Helper()
				want := "[Bash: stdin -> sess-42]\nyes\\n"
				if msgs[1].Content != want {
					t.Errorf(
						"content = %q, want %q",
						msgs[1].Content, want,
					)
				}
				if msgs[1].ToolCalls[0].Category != "Bash" {
					t.Errorf(
						"category = %q, want Bash",
						msgs[1].ToolCalls[0].Category,
					)
				}
			},
		},
		{
			name: "large message within scanner limit",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("big", "/tmp", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", generateLargeString(1024*1024), tsEarlyS1),
			),
			wantID:   "codex:big",
			wantMsgs: 1,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if msgs[0].ContentLength != 1024*1024 {
					t.Errorf(
						"content_length = %d, want %d",
						msgs[0].ContentLength, 1024*1024,
					)
				}
			},
		},
		{
			name: "second session_meta with unparsable cwd resets project",
			content: testjsonl.JoinJSONL(
				testjsonl.CodexSessionMetaJSON("multi", "/Users/alice/code/my-api", "user", tsEarly),
				testjsonl.CodexMsgJSON("user", "hello", tsEarlyS1),
				testjsonl.CodexSessionMetaJSON("multi", "/", "user", "2024-01-01T10:00:02Z"),
			),
			wantID:   "codex:multi",
			wantMsgs: 1,
			check: func(t *testing.T, sess *ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if sess.Project != "unknown" {
					t.Errorf(
						"project = %q, want %q",
						sess.Project, "unknown",
					)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName := tt.fileName
			if fileName == "" {
				fileName = "test.jsonl"
			}
			path := createTestFile(t, fileName, tt.content)
			sess, msgs, err := ParseCodexSession(
				path, "local", tt.includeExec,
			)
			if err != nil {
				t.Fatalf("ParseCodexSession: %v", err)
			}
			if tt.wantNil {
				if sess != nil {
					t.Fatal("expected nil session")
				}
				return
			}
			if sess == nil {
				t.Fatal("session is nil")
			}
			if sess.ID != tt.wantID {
				t.Errorf("session ID = %q, want %q",
					sess.ID, tt.wantID)
			}
			if len(msgs) != tt.wantMsgs {
				t.Fatalf("got %d messages, want %d",
					len(msgs), tt.wantMsgs)
			}
			if tt.check != nil {
				tt.check(t, sess, msgs)
			}
		})
	}
}

func TestCodexSessionTimestampSemantics(t *testing.T) {
	t.Run("invalid timestamp logs warning", func(t *testing.T) {
		content := testjsonl.CodexMsgJSON("user", "hello", "garbage") + "\n"
		path := createTestFile(t, "codex-ts-invalid.jsonl", content)
		buf := captureLog(t)

		sess, msgs, err := ParseCodexSession(
			path, "local", false,
		)
		if err != nil {
			t.Fatalf("ParseCodexSession: %v", err)
		}

		assertZeroTimestamp(t, sess.StartedAt, "StartedAt")
		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		assertZeroTimestamp(t, msgs[0].Timestamp, "msg timestamp")
		assertLogContains(t, buf,
			"unparseable timestamp", "garbage",
		)
	})

	t.Run("very long invalid timestamp is truncated in log", func(t *testing.T) {
		longInvalid := strings.Repeat("x", 200)
		content := testjsonl.CodexMsgJSON("user", "hello", longInvalid) + "\n"
		path := createTestFile(t, "codex-ts-long-invalid.jsonl", content)
		buf := captureLog(t)

		_, _, err := ParseCodexSession(
			path, "local", false,
		)
		if err != nil {
			t.Fatalf("ParseCodexSession: %v", err)
		}

		assertLogContains(t, buf,
			"unparseable timestamp", "...",
		)
		assertLogNotContains(t, buf, longInvalid)
	})
}

func TestParseCodexSessionScannerLimit(t *testing.T) {
	meta := testjsonl.CodexSessionMetaJSON("huge", "/tmp", "user", tsEarly) + "\n"
	prefix := `{"type":"response_item","timestamp":"` + tsEarlyS1 + `","payload":{"role":"user","content":[{"type":"input_text","text":"`
	suffix := `"}]}}` + "\n"
	envelope := len(prefix) + len(suffix)

	t.Run("exact limit succeeds", func(t *testing.T) {
		// envelope includes the trailing newline. bufio.Scanner buffer limit applies
		// to the token plus the delimiter (if scanned). So the total size including
		// newline must be <= maxScanTokenSize.
		textLen := maxScanTokenSize - envelope
		nearLimitText := strings.Repeat("y", textLen)
		content := meta + prefix + nearLimitText + suffix
		path := createTestFile(t, "near-limit.jsonl", content)
		sess, msgs, err := ParseCodexSession(
			path, "local", false,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess == nil {
			t.Fatal("session is nil")
		}
		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
		if msgs[0].ContentLength != textLen {
			t.Errorf(
				"content_length = %d, want %d",
				msgs[0].ContentLength, textLen,
			)
		}
	})

	t.Run("exceeds limit returns error", func(t *testing.T) {
		textLen := maxScanTokenSize - envelope + 1
		hugeText := strings.Repeat("x", textLen)
		content := meta + prefix + hugeText + suffix
		path := createTestFile(t, "huge.jsonl", content)
		_, _, err := ParseCodexSession(path, "local", false)
		if err == nil {
			t.Fatal(
				"expected scanner error for line " +
					"exceeding maxScanTokenSize",
			)
		}
		if !strings.Contains(err.Error(), "scanning") {
			t.Errorf(
				"error = %q, want it to mention scanning",
				err,
			)
		}
	})
}

func TestExtractCwdFromSession(t *testing.T) {
	tests := []struct {
		name    string
		content string // empty means use nonexistent file
		want    string
	}{
		{
			"has cwd field",
			`{"type":"user","timestamp":"2024-01-01T00:00:00Z","message":{"content":"hi"},"cwd":"/Users/alice/code/my-app"}` + "\n",
			"/Users/alice/code/my-app",
		},
		{
			"no cwd field",
			`{"type":"user","timestamp":"2024-01-01T00:00:00Z","message":{"content":"hi"}}` + "\n",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := createTestFile(t, "test.jsonl", tt.content)
			got := ExtractCwdFromSession(path)
			if got != tt.want {
				t.Errorf("ExtractCwdFromSession = %q, want %q",
					got, tt.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		got := ExtractCwdFromSession("/nonexistent/path.jsonl")
		if got != "" {
			t.Errorf("ExtractCwdFromSession = %q, want empty", got)
		}
	})
}

func TestParseCodexSession_WorktreeBranchFallback(t *testing.T) {
	content := `{"type":"session_meta","timestamp":"2024-01-01T00:00:00Z","payload":{"id":"test-uuid","cwd":"/Users/wesm/code/agentsview-worktree-tool-call-arguments","originator":"user","git":{"branch":"worktree-tool-call-arguments"}}}` + "\n" +
		`{"type":"response_item","timestamp":"2024-01-01T00:00:01Z","payload":{"role":"user","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	path := createTestFile(t, "codex-worktree.jsonl", content)

	sess, _, err := ParseCodexSession(path, "local", false)
	if err != nil {
		t.Fatalf("ParseCodexSession: %v", err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if sess.Project != "agentsview" {
		t.Fatalf("project = %q, want %q", sess.Project, "agentsview")
	}
}

func TestExtractClaudeProjectHints(t *testing.T) {
	t.Run("extracts cwd and gitBranch", func(t *testing.T) {
		content := `{"type":"user","timestamp":"2024-01-01T00:00:00Z","cwd":"/Users/alice/code/my-app-worktree-fix","gitBranch":"worktree-fix","message":{"content":"hi"}}` + "\n"
		path := createTestFile(t, "hints.jsonl", content)

		cwd, branch := ExtractClaudeProjectHints(path)
		if cwd != "/Users/alice/code/my-app-worktree-fix" {
			t.Fatalf("cwd = %q, want %q",
				cwd, "/Users/alice/code/my-app-worktree-fix")
		}
		if branch != "worktree-fix" {
			t.Fatalf("branch = %q, want %q", branch, "worktree-fix")
		}
	})

	t.Run("missing branch still returns cwd", func(t *testing.T) {
		content := `{"type":"user","timestamp":"2024-01-01T00:00:00Z","cwd":"/Users/alice/code/my-app","message":{"content":"hi"}}` + "\n"
		path := createTestFile(t, "hints-nobranch.jsonl", content)

		cwd, branch := ExtractClaudeProjectHints(path)
		if cwd != "/Users/alice/code/my-app" {
			t.Fatalf("cwd = %q, want %q",
				cwd, "/Users/alice/code/my-app")
		}
		if branch != "" {
			t.Fatalf("branch = %q, want empty", branch)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		cwd, branch := ExtractClaudeProjectHints(
			"/nonexistent/path.jsonl",
		)
		if cwd != "" || branch != "" {
			t.Fatalf("got cwd=%q branch=%q, want both empty",
				cwd, branch)
		}
	})
}

func TestParseGeminiSession(t *testing.T) {
	hash := "abc123def456"

	standardMessages := []map[string]any{
		testjsonl.GeminiUserMsg("u1", tsEarly, "Fix the login bug"),
		testjsonl.GeminiAssistantMsg(
			"a1", tsEarlyS5, "Looking at the auth module...", nil,
		),
		testjsonl.GeminiUserMsg("u2", tsLate, "That looks right"),
		testjsonl.GeminiAssistantMsg(
			"a2", tsLateS5, "Applied the fix.", nil,
		),
	}

	toolCallMessages := []map[string]any{
		testjsonl.GeminiUserMsg("u1", tsEarly, "Read this file"),
		testjsonl.GeminiAssistantMsg(
			"a1", tsEarlyS5, "Let me read it.", &testjsonl.GeminiMsgOpts{
				Thoughts: []testjsonl.GeminiThought{
					{
						Subject:     "Planning",
						Description: "I need to read the file first.",
						Timestamp:   tsEarlyS1,
					},
				},
				ToolCalls: []testjsonl.GeminiToolCall{
					{
						Name:        "read_file",
						DisplayName: "ReadFile",
						Args:        map[string]string{"file_path": "main.go"},
					},
				},
				Model: "gemini-2.5-pro",
			},
		),
	}

	longMsg := generateLargeString(400)

	tests := []struct {
		name         string
		content      string
		wantMsgCount int
		wantErr      bool
		check        func(t *testing.T, sess *ParsedSession, msgs []ParsedMessage)
	}{
		{
			name: "standard session",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-1", hash, tsEarly, tsLateS5,
				standardMessages,
			),
			wantMsgCount: 4,
			check: func(t *testing.T, sess *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				assertSessionMeta(t, sess,
					"gemini:sess-uuid-1", "my_project", AgentGemini,
				)
				if sess.FirstMessage != "Fix the login bug" {
					t.Errorf("first_message = %q", sess.FirstMessage)
				}
				assertMessage(t, msgs[0], RoleUser, "Fix the login bug")
				assertMessage(t, msgs[1], RoleAssistant, "Looking at")
				if msgs[0].Ordinal != 0 || msgs[1].Ordinal != 1 {
					t.Errorf("ordinals: %d, %d",
						msgs[0].Ordinal, msgs[1].Ordinal)
				}
			},
		},
		{
			name: "tool calls and thinking",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-2", hash, tsEarly, tsEarlyS5,
				toolCallMessages,
			),
			wantMsgCount: 2,
			check: func(t *testing.T, sess *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if !msgs[1].HasToolUse {
					t.Error("msg[1] should have tool_use")
				}
				if !msgs[1].HasThinking {
					t.Error("msg[1] should have thinking")
				}
				if !strings.Contains(msgs[1].Content, "[Thinking: Planning]") {
					t.Errorf("missing thinking in content: %q", msgs[1].Content)
				}
				if !strings.Contains(msgs[1].Content, "[Read: main.go]") {
					t.Errorf("missing tool call in content: %q", msgs[1].Content)
				}
				assertToolCalls(t, msgs[1].ToolCalls, []ParsedToolCall{
					{ToolName: "read_file", Category: "Read"},
				})
			},
		},
		{
			name: "empty tool name skipped",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-empty-tc", hash, tsEarly, tsEarlyS5,
				[]map[string]any{
					testjsonl.GeminiUserMsg("u1", tsEarly, "do it"),
					testjsonl.GeminiAssistantMsg(
						"a1", tsEarlyS5, "Using tool.", &testjsonl.GeminiMsgOpts{
							ToolCalls: []testjsonl.GeminiToolCall{
								{Name: "", DisplayName: "", Args: nil},
							},
						},
					),
				},
			),
			wantMsgCount: 2,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if !msgs[1].HasToolUse {
					t.Error("msg[1] should have tool_use")
				}
				assertToolCalls(t, msgs[1].ToolCalls, nil)
			},
		},
		{
			name: "only system messages",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-3", hash, tsEarly, tsEarlyS5,
				[]map[string]any{
					testjsonl.GeminiInfoMsg("i1", tsEarly, "Starting session", "info"),
					testjsonl.GeminiInfoMsg("e1", tsEarlyS1, "Some error", "error"),
					testjsonl.GeminiInfoMsg("w1", tsEarlyS5, "Some warning", "warning"),
				},
			),
			wantMsgCount: 0,
		},
		{
			name: "empty messages array",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-4", hash, tsEarly, tsEarlyS5,
				[]map[string]any{},
			),
			wantMsgCount: 0,
		},
		{
			name:    "malformed JSON",
			content: "not valid json {{{",
			wantErr: true,
		},
		{
			name: "content as Part array",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-5", hash, tsEarly, tsEarlyS5,
				[]map[string]any{
					{
						"id":        "u1",
						"timestamp": tsEarly,
						"type":      "user",
						"content": []map[string]string{
							{"text": "part one"},
							{"text": "part two"},
						},
					},
				},
			),
			wantMsgCount: 1,
			check: func(t *testing.T, _ *ParsedSession, msgs []ParsedMessage) {
				t.Helper()
				if !strings.Contains(msgs[0].Content, "part one") ||
					!strings.Contains(msgs[0].Content, "part two") {
					t.Errorf("content = %q", msgs[0].Content)
				}
			},
		},
		{
			name: "first message truncation",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-6", hash, tsEarly, tsEarlyS5,
				[]map[string]any{
					testjsonl.GeminiUserMsg("u1", tsEarly, longMsg),
				},
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess *ParsedSession, _ []ParsedMessage) {
				t.Helper()
				if len(sess.FirstMessage) != 303 {
					t.Errorf("first_message length = %d, want 303",
						len(sess.FirstMessage))
				}
			},
		},
		{
			name: "timestamps from startTime and lastUpdated",
			content: testjsonl.GeminiSessionJSON(
				"sess-uuid-7", hash,
				"2024-06-15T10:00:00Z",
				"2024-06-15T11:30:00Z",
				[]map[string]any{
					testjsonl.GeminiUserMsg("u1", "2024-06-15T10:00:00Z", "hello"),
				},
			),
			wantMsgCount: 1,
			check: func(t *testing.T, sess *ParsedSession, _ []ParsedMessage) {
				t.Helper()
				wantStart := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
				wantEnd := time.Date(2024, 6, 15, 11, 30, 0, 0, time.UTC)
				assertTimestamp(t, sess.StartedAt, wantStart)
				assertTimestamp(t, sess.EndedAt, wantEnd)
			},
		},
		{
			name:    "missing sessionId",
			content: `{"projectHash":"abc","startTime":"2024-01-01T00:00:00Z","lastUpdated":"2024-01-01T00:00:00Z","messages":[]}`,
			wantErr: true,
		},
		{
			name:    "missing file",
			content: "", // special: test with nonexistent path
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.name == "missing file" {
				path = filepath.Join(t.TempDir(), "nonexistent.json")
			} else {
				path = createTestFile(t, "session.json", tt.content)
			}

			sess, msgs, err := ParseGeminiSession(
				path, "my_project", "local",
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if sess == nil {
				t.Fatal("session is nil")
			}
			assertMessageCount(t, len(msgs), tt.wantMsgCount)
			assertMessageCount(t, sess.MessageCount, tt.wantMsgCount)
			if tt.check != nil {
				tt.check(t, sess, msgs)
			}
		})
	}
}

func TestFormatGeminiToolCall(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"read_file",
			`{"name":"read_file","args":{"file_path":"main.go"},"displayName":"ReadFile"}`,
			"[Read: main.go]",
		},
		{
			"write_file",
			`{"name":"write_file","args":{"file_path":"out.txt"},"displayName":"WriteFile"}`,
			"[Write: out.txt]",
		},
		{
			"edit_file",
			`{"name":"edit_file","args":{"file_path":"fix.go"},"displayName":"EditFile"}`,
			"[Write: fix.go]",
		},
		{
			"run_command",
			`{"name":"run_command","args":{"command":"go test ./..."},"displayName":"RunCommand"}`,
			"[Bash]\n$ go test ./...",
		},
		{
			"execute_command",
			`{"name":"execute_command","args":{"command":"ls -la"},"displayName":"Exec"}`,
			"[Bash]\n$ ls -la",
		},
		{
			"list_directory",
			`{"name":"list_directory","args":{"dir_path":"src"},"displayName":"ReadFolder"}`,
			"[List: src]",
		},
		{
			"search_files with query",
			`{"name":"search_files","args":{"query":"TODO"},"displayName":"Search"}`,
			"[Grep: TODO]",
		},
		{
			"grep with pattern",
			`{"name":"grep","args":{"pattern":"func main"},"displayName":"Grep"}`,
			"[Grep: func main]",
		},
		{
			"unknown tool with displayName",
			`{"name":"custom_tool","args":{},"displayName":"CustomTool"}`,
			"[Tool: CustomTool]",
		},
		{
			"unknown tool without displayName",
			`{"name":"custom_tool","args":{}}`,
			"[Tool: custom_tool]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := gjson.Parse(tt.json)
			got := formatGeminiToolCall(tc)
			if got != tt.want {
				t.Errorf("formatGeminiToolCall = %q, want %q",
					got, tt.want)
			}
		})
	}
}

func TestGeminiSessionID(t *testing.T) {
	data := []byte(`{"sessionId":"abc-123","messages":[]}`)
	got := GeminiSessionID(data)
	if got != "abc-123" {
		t.Errorf("GeminiSessionID = %q, want %q", got, "abc-123")
	}

	got = GeminiSessionID([]byte(`{}`))
	if got != "" {
		t.Errorf("GeminiSessionID empty = %q, want empty", got)
	}
}

func TestParseClaudeToolResults(t *testing.T) {
	lines := []string{
		`{"type":"assistant","timestamp":"2024-01-01T00:00:00Z","message":{"content":[{"type":"tool_use","id":"toolu_abc","name":"Read","input":{"file_path":"main.go"}}]}}`,
		`{"type":"user","timestamp":"2024-01-01T00:00:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_abc","content":"package main\nfunc main() {}"}]}}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	path := createTestFile(t, "tool-results.jsonl", content)

	_, msgs, err := ParseClaudeSession(path, "test-project", "local")
	if err != nil {
		t.Fatalf("ParseClaudeSession: %v", err)
	}

	// Should have 2 messages: assistant tool_use + user tool_result
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	// User message should have ToolResults populated
	userMsg := msgs[1]
	if len(userMsg.ToolResults) != 1 {
		t.Fatalf("ToolResults count = %d, want 1", len(userMsg.ToolResults))
	}
	if userMsg.ToolResults[0].ToolUseID != "toolu_abc" {
		t.Errorf("ToolUseID = %q, want toolu_abc",
			userMsg.ToolResults[0].ToolUseID)
	}
	if userMsg.ToolResults[0].ContentLength != 27 {
		t.Errorf("ContentLength = %d, want 27",
			userMsg.ToolResults[0].ContentLength)
	}
}

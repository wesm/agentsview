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
			"-Users-wesm-code-my-app", "my_app"},
		{"encoded path with projects",
			"-Users-wesm-projects-api-server", "api_server"},
		{"encoded path with repos",
			"-home-user-repos-frontend", "frontend"},
		{"encoded path without marker",
			"-Users-wesm", "wesm"},
		{"empty", "", ""},
		{"no prefix", "plain_name", "plain_name"},
		{"with src marker",
			"-Users-wesm-src-my-lib", "my_lib"},
		{"multi-word after marker",
			"-Users-wesm-code-my-cool-project", "my_cool_project"},
		{"deeply nested",
			"-Users-wesm-code-org-team-repo", "org_team_repo"},
		{"unicode components",
			"-Users-wesm-code-café-app", "café_app"},
		{"trailing dash",
			"-Users-wesm-code-myapp-", "myapp_"},
		{"double dashes",
			"-Users-wesm-code--my-app", "_my_app"},
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
		{"/Users/wesm/code/my-app", "my_app"},
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
		{"_Users_wesm_code_app", true},
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
		name        string
		json        string
		wantText    string
		wantThink   bool
		wantToolUse bool
	}{
		{
			"plain string",
			`"Hello world"`,
			"Hello world", false, false,
		},
		{
			"text block array",
			`[{"type":"text","text":"Hi"}]`,
			"Hi", false, false,
		},
		{
			"thinking block",
			`[{"type":"thinking","thinking":"Let me think..."}]`,
			"[Thinking]\nLet me think...", true, false,
		},
		{
			"tool_use block",
			`[{"type":"tool_use","name":"Read","input":{"file_path":"test.go"}}]`,
			"[Read: test.go]", false, true,
		},
		{
			"mixed blocks",
			`[{"type":"text","text":"Looking at"},{"type":"tool_use","name":"Bash","input":{"command":"ls","description":"list files"}}]`,
			"Looking at\n[Bash: list files]\n$ ls", false, true,
		},
		{
			"empty array",
			`[]`,
			"", false, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gjson.Parse(tt.json)
			text, hasThinking, hasToolUse := ExtractTextContent(result)
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

func TestParseClaudeSession(t *testing.T) {
	validContent := testjsonl.JoinJSONL(
		testjsonl.ClaudeUserJSON("Fix the login bug", tsEarly, "/Users/wesm/code/my-app"),
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
				testjsonl.CodexSessionMetaJSON("abc-123", "/Users/wesm/code/my-api", "user", tsEarly),
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
				testjsonl.CodexSessionMetaJSON("multi", "/Users/wesm/code/my-api", "user", tsEarly),
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
			`{"type":"user","timestamp":"2024-01-01T00:00:00Z","message":{"content":"hi"},"cwd":"/Users/wesm/code/my-app"}` + "\n",
			"/Users/wesm/code/my-app",
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

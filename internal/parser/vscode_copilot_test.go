package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVSCodeCopilotSession(t *testing.T) {
	tests := []struct {
		name         string
		json         string
		wantNil      bool
		wantMessages int
		wantTitle    string
		wantAgent    AgentType
		wantToolUse  bool
	}{
		{
			name:    "empty requests",
			json:    `{"version":3,"sessionId":"abc","requests":[]}`,
			wantNil: true,
		},
		{
			name: "single user+assistant turn",
			json: `{
				"version": 3,
				"sessionId": "test-123",
				"creationDate": 1755347684754,
				"lastMessageDate": 1755347728048,
				"customTitle": "Test session",
				"requests": [{
					"requestId": "req1",
					"message": {"text": "Hello world", "parts": []},
					"response": [
						{"value": "Hi there! ", "supportThemeIcons": false},
						{"value": "How can I help?", "supportThemeIcons": false}
					],
					"timestamp": 1755347728047,
					"modelId": "copilot/gpt-5"
				}]
			}`,
			wantMessages: 2,
			wantTitle:    "Hello world",
			wantAgent:    AgentVSCodeCopilot,
		},
		{
			name: "with tool invocations",
			json: `{
				"version": 3,
				"sessionId": "tools-456",
				"creationDate": 1755347684754,
				"lastMessageDate": 1755347728048,
				"customTitle": "Tool session",
				"requests": [{
					"requestId": "req1",
					"message": {"text": "Read the file", "parts": []},
					"response": [
						{"value": "Reading the file... "},
						{"kind": "prepareToolInvocation", "toolName": "copilot_readFile"},
						{"kind": "toolInvocationSerialized", "toolId": "copilot_readFile", "toolCallId": "tc1", "isConfirmed": true, "isComplete": true},
						{"value": "Done reading."}
					],
					"timestamp": 1755347728047,
					"modelId": "copilot/gpt-5"
				}]
			}`,
			wantMessages: 2,
			wantToolUse:  true,
		},
		{
			name: "multiple requests",
			json: `{
				"version": 3,
				"sessionId": "multi-789",
				"creationDate": 1755340000000,
				"lastMessageDate": 1755350000000,
				"customTitle": "Multi turn",
				"requests": [
					{
						"requestId": "req1",
						"message": {"text": "First question"},
						"response": [{"value": "First answer"}],
						"timestamp": 1755340000000
					},
					{
						"requestId": "req2",
						"message": {"text": "Second question"},
						"response": [{"value": "Second answer"}],
						"timestamp": 1755350000000
					}
				]
			}`,
			wantMessages: 4,
			wantTitle:    "First question",
		},
		{
			name: "no user text uses customTitle",
			json: `{
				"version": 3,
				"sessionId": "notitle-000",
				"creationDate": 1755340000000,
				"lastMessageDate": 1755340000000,
				"customTitle": "Fallback Title",
				"requests": [{
					"requestId": "req1",
					"message": {"text": ""},
					"response": [{"value": "Some response"}],
					"timestamp": 1755340000000
				}]
			}`,
			wantMessages: 1,
			wantTitle:    "Fallback Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test-session.json")
			if err := os.WriteFile(
				path, []byte(tt.json), 0644,
			); err != nil {
				t.Fatal(err)
			}

			sess, msgs, err := ParseVSCodeCopilotSession(
				path, "testproject", "local",
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if sess != nil {
					t.Fatal("expected nil session")
				}
				return
			}

			if sess == nil {
				t.Fatal("expected non-nil session")
			}

			if len(msgs) != tt.wantMessages {
				t.Errorf(
					"messages: got %d, want %d",
					len(msgs), tt.wantMessages,
				)
			}

			if tt.wantTitle != "" &&
				sess.FirstMessage != tt.wantTitle {
				t.Errorf(
					"first message: got %q, want %q",
					sess.FirstMessage, tt.wantTitle,
				)
			}

			if tt.wantAgent != "" && sess.Agent != tt.wantAgent {
				t.Errorf(
					"agent: got %q, want %q",
					sess.Agent, tt.wantAgent,
				)
			}

			if sess.Project != "testproject" {
				t.Errorf(
					"project: got %q, want %q",
					sess.Project, "testproject",
				)
			}

			if tt.wantToolUse {
				found := false
				for _, m := range msgs {
					if m.HasToolUse {
						found = true
						break
					}
				}
				if !found {
					t.Error("expected tool use in messages")
				}
			}
		})
	}
}

func TestParseVSCodeCopilotSession_NonExistent(t *testing.T) {
	sess, msgs, err := ParseVSCodeCopilotSession(
		"/nonexistent/path.json", "proj", "local",
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sess != nil || msgs != nil {
		t.Fatal("expected nil for non-existent file")
	}
}

func TestParseVSCodeCopilotSession_MixedTextAndTools(t *testing.T) {
	data := `{
		"version": 3,
		"sessionId": "mixed-001",
		"creationDate": 1755340000000,
		"lastMessageDate": 1755340000000,
		"customTitle": "Mixed content",
		"requests": [{
			"requestId": "req1",
			"message": {"text": "Read the file"},
			"response": [
				{"kind": "toolInvocationSerialized", "toolId": "copilot_readFile", "toolCallId": "tc1", "isConfirmed": true, "isComplete": true, "pastTenseMessage": {"value": "Read main.go, lines 1 to 50"}},
				{"value": "Here is the file content."}
			],
			"timestamp": 1755340000000
		}]
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	_, msgs, err := ParseVSCodeCopilotSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find assistant message
	var assistant *ParsedMessage
	for i := range msgs {
		if msgs[i].Role == RoleAssistant {
			assistant = &msgs[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant message")
	}

	if !assistant.HasToolUse {
		t.Error("expected HasToolUse=true")
	}

	// Content should include both tool markers and text
	if len(assistant.Content) == 0 {
		t.Error("expected non-empty content")
	}

	// Tool calls should have InputJSON populated
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(assistant.ToolCalls))
	}
	tc := assistant.ToolCalls[0]
	if tc.InputJSON == "" {
		t.Error("expected non-empty InputJSON")
	}
	if tc.Category != "Read" {
		t.Errorf("category: got %q, want %q", tc.Category, "Read")
	}
}

func TestParseVSCodeCopilotSession_TerminalToolData(t *testing.T) {
	data := `{
		"version": 3,
		"sessionId": "term-001",
		"creationDate": 1755340000000,
		"lastMessageDate": 1755340000000,
		"customTitle": "Terminal session",
		"requests": [{
			"requestId": "req1",
			"message": {"text": "Run tests"},
			"response": [
				{"kind": "toolInvocationSerialized", "toolId": "copilot_runInTerminal", "toolCallId": "tc1", "isConfirmed": true, "isComplete": true, "invocationMessage": "Using \"Run In Terminal\"", "toolSpecificData": {"kind": "terminal", "language": "sh", "command": "npm test"}}
			],
			"timestamp": 1755340000000
		}]
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	_, msgs, err := ParseVSCodeCopilotSession(path, "proj", "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var assistant *ParsedMessage
	for i := range msgs {
		if msgs[i].Role == RoleAssistant {
			assistant = &msgs[i]
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant message")
	}

	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(assistant.ToolCalls))
	}
	tc := assistant.ToolCalls[0]
	if tc.Category != "Bash" {
		t.Errorf("category: got %q, want %q", tc.Category, "Bash")
	}
	if tc.InputJSON == "" {
		t.Error("expected non-empty InputJSON")
	}

	// Content should include the command
	if !containsStr(assistant.Content, "npm test") {
		t.Errorf("content should contain command, got: %s", assistant.Content)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle ||
			len(haystack) > len(needle) &&
				(haystack[:len(needle)] == needle ||
					containsSubstring(haystack, needle)))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestExtractProjectFromURI(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///Users/dev/projects/myapp", "myapp"},
		{"file:///home/user/code/repo", "repo"},
		{"file:///C:/Users/dev/projects/app", "app"},
		{"some-name", "some-name"},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := extractProjectFromURI(tt.uri)
			if got != tt.want {
				t.Errorf(
					"extractProjectFromURI(%q) = %q, want %q",
					tt.uri, got, tt.want,
				)
			}
		})
	}
}

func TestReadVSCodeWorkspaceManifest(t *testing.T) {
	dir := t.TempDir()

	// Valid workspace.json
	content := `{"folder":"file:///Users/dev/projects/agentsview"}`
	if err := os.WriteFile(
		filepath.Join(dir, "workspace.json"),
		[]byte(content), 0644,
	); err != nil {
		t.Fatal(err)
	}

	project := ReadVSCodeWorkspaceManifest(dir)
	if project != "agentsview" {
		t.Errorf("got %q, want %q", project, "agentsview")
	}

	// Non-existent dir
	project = ReadVSCodeWorkspaceManifest("/nonexistent")
	if project != "" {
		t.Errorf("expected empty, got %q", project)
	}
}

func TestDiscoverVSCodeCopilotSessions(t *testing.T) {
	root := t.TempDir()

	// Create workspace structure
	hash := "abc123def456"
	chatDir := filepath.Join(
		root, "workspaceStorage", hash, "chatSessions",
	)
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		t.Fatal(err)
	}

	// workspace.json
	wsJSON := `{"folder":"file:///Users/dev/projects/myproject"}`
	wsPath := filepath.Join(
		root, "workspaceStorage", hash, "workspace.json",
	)
	if err := os.WriteFile(
		wsPath, []byte(wsJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Chat session file
	sessionJSON := `{"version":3,"sessionId":"sess1","requests":[{"requestId":"r1","message":{"text":"hi"},"response":[{"value":"hello"}],"timestamp":1755340000000}]}`
	sessPath := filepath.Join(chatDir, "sess1.json")
	if err := os.WriteFile(
		sessPath, []byte(sessionJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// globalStorage/emptyWindowChatSessions
	globalDir := filepath.Join(
		root, "globalStorage", "emptyWindowChatSessions",
	)
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	globalPath := filepath.Join(globalDir, "global-sess.json")
	if err := os.WriteFile(
		globalPath, []byte(sessionJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	files := DiscoverVSCodeCopilotSessions(root)

	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}

	// Check workspace session
	var wsFile, globalFile DiscoveredFile
	for _, f := range files {
		switch f.Project {
		case "myproject":
			wsFile = f
		case "empty-window":
			globalFile = f
		}
	}

	if wsFile.Path == "" {
		t.Error("missing workspace session file")
	}
	if wsFile.Agent != AgentVSCodeCopilot {
		t.Errorf("agent: got %q, want %q",
			wsFile.Agent, AgentVSCodeCopilot)
	}

	if globalFile.Path == "" {
		t.Error("missing global session file")
	}
}

func TestNormalizeVSCodeToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"copilot_readFile", "read_file"},
		{"copilot_replaceString", "edit_file"},
		{"copilot_runCommand", "shell"},
		{"copilot_searchFiles", "grep"},
		{"copilot_listDir", "glob"},
		{"copilot_createFile", "create_file"},
		{"copilot_runInTerminal", "shell"},
		{"copilot_getTerminalOutput", "shell"},
		{"copilot_findTextInFiles", "grep"},
		{"copilot_findFiles", "glob"},
		{"copilot_listDirectory", "glob"},
		{"copilot_applyPatch", "edit_file"},
		{"copilot_multiReplaceString", "edit_file"},
		{"copilot_fetchWebPage", "read_web_page"},
		{"copilot_think", "Tool"},
		{"runSubagent", "Task"},
		{"unknown_tool", "unknown_tool"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeVSCodeToolName(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractVSCopilotInputJSON(t *testing.T) {
	tests := []struct {
		name     string
		invMsg   string
		pastMsg  string
		toolData string
		wantKey  string
		wantVal  string
	}{
		{
			name:    "string invocation message",
			invMsg:  `"Using Run In Terminal"`,
			wantKey: "message",
			wantVal: "Using Run In Terminal",
		},
		{
			name:    "object invocation message",
			invMsg:  `{"value": "Reading file.txt, lines 1 to 50"}`,
			wantKey: "message",
			wantVal: "Reading file.txt, lines 1 to 50",
		},
		{
			name:    "prefers pastTenseMessage",
			invMsg:  `"Reading file..."`,
			pastMsg: `"Read file.txt, lines 1 to 50"`,
			wantKey: "message",
			wantVal: "Read file.txt, lines 1 to 50",
		},
		{
			name:     "terminal tool data",
			invMsg:   `"Using Run In Terminal"`,
			toolData: `{"kind":"terminal","language":"sh","command":"ls -la"}`,
			wantKey:  "command",
			wantVal:  "ls -la",
		},
		{
			name: "empty fields",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inv, past, td json.RawMessage
			if tt.invMsg != "" {
				inv = json.RawMessage(tt.invMsg)
			}
			if tt.pastMsg != "" {
				past = json.RawMessage(tt.pastMsg)
			}
			if tt.toolData != "" {
				td = json.RawMessage(tt.toolData)
			}
			got := extractVSCopilotInputJSON(inv, past, td)

			if tt.wantKey == "" {
				if got != "" {
					t.Errorf("expected empty, got %q", got)
				}
				return
			}

			var m map[string]any
			if err := json.Unmarshal(
				[]byte(got), &m,
			); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			val, ok := m[tt.wantKey].(string)
			if !ok || val != tt.wantVal {
				t.Errorf(
					"got %q=%q, want %q=%q",
					tt.wantKey, val,
					tt.wantKey, tt.wantVal,
				)
			}
		})
	}
}

func TestParseVSCodeCopilotSession_JSONL(t *testing.T) {
	tests := []struct {
		name         string
		lines        []string
		wantNil      bool
		wantMessages int
		wantTitle    string
		wantToolUse  bool
	}{
		{
			name: "simple session with mutations",
			lines: []string{
				// kind=0: initial snapshot with empty requests
				`{"kind":0,"v":{"version":3,"sessionId":"jsonl-001","creationDate":1770650022790,"customTitle":"","requests":[],"responderUsername":"GitHub Copilot"}}`,
				// kind=1: set customTitle
				`{"kind":1,"k":["customTitle"],"v":"Test JSONL Session"}`,
				// kind=2: push a request
				`{"kind":2,"k":["requests"],"v":[{"requestId":"req1","timestamp":1770650031889,"message":{"text":"Hello JSONL","parts":[]},"response":[{"value":"Hi from JSONL!"}],"modelId":"copilot/gpt-4o"}]}`,
			},
			wantMessages: 2,
			wantTitle:    "Hello JSONL",
		},
		{
			name: "empty session no requests",
			lines: []string{
				`{"kind":0,"v":{"version":3,"sessionId":"jsonl-empty","creationDate":1770650022790,"requests":[]}}`,
			},
			wantNil: true,
		},
		{
			name: "session with tool calls",
			lines: []string{
				`{"kind":0,"v":{"version":3,"sessionId":"jsonl-tools","creationDate":1770650022790,"requests":[]}}`,
				`{"kind":2,"k":["requests"],"v":[{"requestId":"req1","timestamp":1770650031889,"message":{"text":"Read file","parts":[]},"response":[{"kind":"toolInvocationSerialized","toolId":"copilot_readFile","toolCallId":"tc1","isConfirmed":true,"isComplete":true},{"value":"Done."}],"modelId":"copilot/gpt-4o"}]}`,
			},
			wantMessages: 2,
			wantToolUse:  true,
			wantTitle:    "Read file",
		},
		{
			name: "multiple requests via push",
			lines: []string{
				`{"kind":0,"v":{"version":3,"sessionId":"jsonl-multi","creationDate":1770650022790,"requests":[]}}`,
				`{"kind":2,"k":["requests"],"v":[{"requestId":"req1","timestamp":1770650031889,"message":{"text":"First","parts":[]},"response":[{"value":"Answer 1"}],"modelId":"copilot/gpt-4o"}]}`,
				`{"kind":2,"k":["requests"],"v":[{"requestId":"req2","timestamp":1770650041889,"message":{"text":"Second","parts":[]},"response":[{"value":"Answer 2"}],"modelId":"copilot/gpt-4o"}]}`,
			},
			wantMessages: 4,
			wantTitle:    "First",
		},
		{
			name: "set mutation on response",
			lines: []string{
				`{"kind":0,"v":{"version":3,"sessionId":"jsonl-set","creationDate":1770650022790,"requests":[{"requestId":"req1","timestamp":1770650031889,"message":{"text":"Q","parts":[]},"response":[{"value":"partial"}],"modelId":"copilot/gpt-4o"}]}}`,
				// Update the first response item
				`{"kind":1,"k":["requests",0,"response",0],"v":{"value":"Complete answer"}}`,
			},
			wantMessages: 2,
			wantTitle:    "Q",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test-session.jsonl")

			content := strings.Join(tt.lines, "\n") + "\n"
			if err := os.WriteFile(
				path, []byte(content), 0644,
			); err != nil {
				t.Fatal(err)
			}

			sess, msgs, err := ParseVSCodeCopilotSession(
				path, "testproject", "local",
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if sess != nil {
					t.Fatal("expected nil session")
				}
				return
			}

			if sess == nil {
				t.Fatal("expected non-nil session")
			}

			if len(msgs) != tt.wantMessages {
				t.Errorf(
					"messages: got %d, want %d",
					len(msgs), tt.wantMessages,
				)
			}

			if tt.wantTitle != "" &&
				sess.FirstMessage != tt.wantTitle {
				t.Errorf(
					"first message: got %q, want %q",
					sess.FirstMessage, tt.wantTitle,
				)
			}

			if sess.Agent != AgentVSCodeCopilot {
				t.Errorf(
					"agent: got %q, want %q",
					sess.Agent, AgentVSCodeCopilot,
				)
			}

			if tt.wantToolUse {
				found := false
				for _, m := range msgs {
					if m.HasToolUse {
						found = true
						break
					}
				}
				if !found {
					t.Error("expected tool use in messages")
				}
			}
		})
	}
}

func TestReconstructJSONL(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		wantErr bool
		check   func(t *testing.T, data []byte)
	}{
		{
			name: "initial only",
			lines: []string{
				`{"kind":0,"v":{"sessionId":"s1","version":3}}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				if m["sessionId"] != "s1" {
					t.Errorf("sessionId: got %v", m["sessionId"])
				}
			},
		},
		{
			name: "set nested property",
			lines: []string{
				`{"kind":0,"v":{"a":{"b":"old"}}}`,
				`{"kind":1,"k":["a","b"],"v":"new"}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				a := m["a"].(map[string]any)
				if a["b"] != "new" {
					t.Errorf("got %v, want new", a["b"])
				}
			},
		},
		{
			name: "push to array",
			lines: []string{
				`{"kind":0,"v":{"items":["a"]}}`,
				`{"kind":2,"k":["items"],"v":["b","c"]}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				items := m["items"].([]any)
				if len(items) != 3 {
					t.Fatalf("len: got %d, want 3", len(items))
				}
				if items[2] != "c" {
					t.Errorf("items[2]: got %v", items[2])
				}
			},
		},
		{
			name: "push with splice index",
			lines: []string{
				`{"kind":0,"v":{"items":["a","c"]}}`,
				`{"kind":2,"k":["items"],"v":["b"],"i":1}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				items := m["items"].([]any)
				if len(items) != 3 {
					t.Fatalf("len: got %d, want 3", len(items))
				}
				if items[0] != "a" || items[1] != "b" || items[2] != "c" {
					t.Errorf("items: got %v", items)
				}
			},
		},
		{
			name: "push with negative splice index",
			lines: []string{
				`{"kind":0,"v":{"items":["a","b"]}}`,
				`{"kind":2,"k":["items"],"v":["z"],"i":-1}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				items := m["items"].([]any)
				if len(items) != 3 {
					t.Fatalf("len: got %d, want 3",
						len(items))
				}
				// Negative index clamped to 0: inserted at front
				if items[0] != "z" {
					t.Errorf("items[0]: got %v, want z",
						items[0])
				}
			},
		},
		{
			name: "delete property",
			lines: []string{
				`{"kind":0,"v":{"a":"keep","b":"remove"}}`,
				`{"kind":3,"k":["b"]}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				if _, ok := m["b"]; ok {
					t.Error("expected b to be deleted")
				}
				if m["a"] != "keep" {
					t.Errorf("a: got %v", m["a"])
				}
			},
		},
		{
			name: "set array element by index",
			lines: []string{
				`{"kind":0,"v":{"arr":["x","y","z"]}}`,
				`{"kind":1,"k":["arr",1],"v":"Y"}`,
			},
			check: func(t *testing.T, data []byte) {
				var m map[string]any
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				arr := m["arr"].([]any)
				if arr[1] != "Y" {
					t.Errorf("arr[1]: got %v", arr[1])
				}
			},
		},
		{
			name:  "empty file returns nil",
			lines: []string{},
			check: func(t *testing.T, data []byte) {
				if data != nil {
					t.Errorf("expected nil, got %s", data)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.jsonl")

			content := strings.Join(tt.lines, "\n") + "\n"
			if err := os.WriteFile(
				path, []byte(content), 0644,
			); err != nil {
				t.Fatal(err)
			}

			data, err := reconstructJSONL(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, data)
		})
	}
}

func TestDiscoverVSCodeCopilot_JSONLDedup(t *testing.T) {
	root := t.TempDir()

	hash := "abc123def456"
	chatDir := filepath.Join(
		root, "workspaceStorage", hash, "chatSessions",
	)
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		t.Fatal(err)
	}

	wsJSON := `{"folder":"file:///Users/dev/projects/myproject"}`
	wsPath := filepath.Join(
		root, "workspaceStorage", hash, "workspace.json",
	)
	if err := os.WriteFile(
		wsPath, []byte(wsJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Session with both .json and .jsonl - jsonl should win
	sessionJSON := `{"version":3,"sessionId":"dup1","requests":[{"requestId":"r1","message":{"text":"hi"},"response":[{"value":"hello"}],"timestamp":1755340000000}]}`
	if err := os.WriteFile(
		filepath.Join(chatDir, "dup1.json"),
		[]byte(sessionJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}
	jsonlContent := `{"kind":0,"v":{"version":3,"sessionId":"dup1","creationDate":1755340000000,"requests":[{"requestId":"r1","timestamp":1755340000000,"message":{"text":"hi"},"response":[{"value":"hello"}]}]}}` + "\n"
	if err := os.WriteFile(
		filepath.Join(chatDir, "dup1.jsonl"),
		[]byte(jsonlContent), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Session with only .jsonl
	if err := os.WriteFile(
		filepath.Join(chatDir, "only-jsonl.jsonl"),
		[]byte(jsonlContent), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Session with only .json
	if err := os.WriteFile(
		filepath.Join(chatDir, "only-json.json"),
		[]byte(sessionJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	files := DiscoverVSCodeCopilotSessions(root)

	// Should get 3 files: dup1.jsonl, only-jsonl.jsonl, only-json.json
	if len(files) != 3 {
		for _, f := range files {
			t.Logf("  %s", f.Path)
		}
		t.Fatalf("got %d files, want 3", len(files))
	}

	// Verify dup1.json was excluded (dup1.jsonl present)
	for _, f := range files {
		if filepath.Base(f.Path) == "dup1.json" {
			t.Error("dup1.json should be excluded when dup1.jsonl exists")
		}
	}
}

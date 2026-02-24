package insight

import (
	"strings"
	"testing"
)

func TestParseCodexStream_AgentMessages(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"thread.started","thread_id":"abc"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"m1","type":"agent_message"}}`,
		`{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"partial"}}`,
		`{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"# Summary\nDone."}}`,
		`{"type":"turn.completed"}`,
	}, "\n") + "\n"

	result, err := parseCodexStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseCodexStream: %v", err)
	}
	if result != "# Summary\nDone." {
		t.Errorf("result = %q", result)
	}
}

func TestParseCodexStream_MultipleMessages(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"First"}}`,
		`{"type":"item.completed","item":{"id":"m2","type":"agent_message","text":"Second"}}`,
	}, "\n") + "\n"

	result, err := parseCodexStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseCodexStream: %v", err)
	}
	if result != "First\nSecond" {
		t.Errorf("result = %q", result)
	}
}

func TestParseCodexStream_TurnFailed(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"turn.started"}`,
		`{"type":"turn.failed","error":{"message":"rate limit"}}`,
	}, "\n") + "\n"

	_, err := parseCodexStream(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %q, want rate limit", err.Error())
	}
}

func TestParseCodexStream_DeduplicatesByID(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"v1"}}`,
		`{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"v2"}}`,
		`{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"v3"}}`,
	}, "\n") + "\n"

	result, err := parseCodexStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseCodexStream: %v", err)
	}
	if result != "v3" {
		t.Errorf("result = %q, want v3", result)
	}
}

func TestParseStreamJSON_ResultEvent(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"assistant","message":{"content":"Working..."}}`,
		`{"type":"result","result":"# Final Summary"}`,
	}, "\n") + "\n"

	result, err := parseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if result != "# Final Summary" {
		t.Errorf("result = %q", result)
	}
}

func TestParseStreamJSON_FallsBackToAssistantMessages(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":"Part 1"}}`,
		`{"type":"assistant","message":{"content":"Part 2"}}`,
	}, "\n") + "\n"

	result, err := parseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if result != "Part 1\nPart 2" {
		t.Errorf("result = %q", result)
	}
}

func TestParseStreamJSON_GeminiFormat(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"message","role":"assistant","content":"Analysis done.","delta":true}`,
		`{"type":"result","result":"# Full Result"}`,
	}, "\n") + "\n"

	result, err := parseStreamJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if result != "# Full Result" {
		t.Errorf("result = %q", result)
	}
}

func TestParseStreamJSON_ErrorEvent(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"error","error":{"message":"rate limited"}}`,
	}, "\n") + "\n"

	_, err := parseStreamJSON(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error = %q, want rate limited",
			err.Error())
	}
}

func TestParseCodexStream_SkipsMalformedJSON(t *testing.T) {
	input := strings.Join([]string{
		`not valid json`,
		`{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"OK"}}`,
	}, "\n") + "\n"

	result, err := parseCodexStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseCodexStream: %v", err)
	}
	if result != "OK" {
		t.Errorf("result = %q, want OK", result)
	}
}

func TestParseStreamJSON_Empty(t *testing.T) {
	result, err := parseStreamJSON(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parseStreamJSON: %v", err)
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
}

func TestCleanEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("HOME", "/home/test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "s3cret")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("UNKNOWN_VAR", "should-be-dropped")

	env := cleanEnv()

	// Normalize keys to uppercase for cross-platform assertions
	// (Windows may return Path instead of PATH).
	envMap := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		envMap[strings.ToUpper(k)] = v
	}

	// Secrets and unknown vars must not pass through.
	for _, blocked := range []string{
		"ANTHROPIC_API_KEY", "CLAUDECODE",
		"AWS_SECRET_ACCESS_KEY", "UNKNOWN_VAR",
	} {
		if _, ok := envMap[blocked]; ok {
			t.Errorf("%s should not be in env", blocked)
		}
	}

	// Allowed system vars must pass through.
	for _, allowed := range []string{
		"HOME", "PATH", "LANG",
	} {
		if _, ok := envMap[allowed]; !ok {
			t.Errorf("%s should be preserved", allowed)
		}
	}

	if v, ok := envMap["CLAUDE_NO_SOUND"]; !ok || v != "1" {
		t.Errorf(
			"CLAUDE_NO_SOUND should be 1, got %q", v,
		)
	}
}

func TestEnvKeyAllowed(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PATH", true},
		{"Path", true}, // Windows-style
		{"path", true}, // lowercase
		{"HOME", true},
		{"Home", true},
		{"COMSPEC", true},
		{"ComSpec", true}, // Windows-style
		{"LC_ALL", true},  // prefix match
		{"XDG_CONFIG_HOME", true},
		{"SSL_CERT_FILE", true},
		{"HTTP_PROXY", true},
		{"APPDATA", true},
		{"AppData", true},
		{"LOCALAPPDATA", true},
		{"PROGRAMDATA", true},
		{"PATHEXT", true},
		{"PathExt", true},
		{"WINDIR", true},
		{"HOMEDRIVE", true},
		{"HOMEPATH", true},
		{"ANTHROPIC_API_KEY", false},
		{"AWS_SECRET_ACCESS_KEY", false},
		{"DATABASE_URL", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := envKeyAllowed(tt.key); got != tt.want {
				t.Errorf(
					"envKeyAllowed(%q) = %v, want %v",
					tt.key, got, tt.want,
				)
			}
		})
	}
}

func TestValidAgents(t *testing.T) {
	for _, agent := range []string{
		"claude", "codex", "gemini",
	} {
		if !ValidAgents[agent] {
			t.Errorf("%s should be valid", agent)
		}
	}
	if ValidAgents["gpt"] {
		t.Error("gpt should not be valid")
	}
}

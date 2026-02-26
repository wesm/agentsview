package insight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseCodexStream(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError string
	}{
		{
			name: "agent messages",
			input: `{"type":"thread.started","thread_id":"abc"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"m1","type":"agent_message"}}
{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"partial"}}
{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"# Summary\nDone."}}
{"type":"turn.completed"}
`,
			want: `# Summary
Done.`,
		},
		{
			name: "multiple messages",
			input: `{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"First"}}
{"type":"item.completed","item":{"id":"m2","type":"agent_message","text":"Second"}}
`,
			want: `First
Second`,
		},
		{
			name: "turn failed",
			input: `{"type":"turn.started"}
{"type":"turn.failed","error":{"message":"rate limit"}}
`,
			wantError: "rate limit",
		},
		{
			name: "deduplicates by id",
			input: `{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"v1"}}
{"type":"item.updated","item":{"id":"m1","type":"agent_message","text":"v2"}}
{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"v3"}}
`,
			want: "v3",
		},
		{
			name: "skips malformed json",
			input: `not valid json
{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"OK"}}
`,
			want: "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCodexStream(strings.NewReader(tt.input))
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("got %q, want %q", result, tt.want)
			}
		})
	}
}

func TestParseStreamJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError string
	}{
		{
			name: "result event",
			input: `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":"Working..."}}
{"type":"result","result":"# Final Summary"}
`,
			want: "# Final Summary",
		},
		{
			name: "falls back to assistant messages",
			input: `{"type":"assistant","message":{"content":"Part 1"}}
{"type":"assistant","message":{"content":"Part 2"}}
`,
			want: `Part 1
Part 2`,
		},
		{
			name: "gemini format",
			input: `{"type":"system","subtype":"init"}
{"type":"message","role":"assistant","content":"Analysis done.","delta":true}
{"type":"result","result":"# Full Result"}
`,
			want: "# Full Result",
		},
		{
			name: "error event",
			input: `{"type":"system","subtype":"init"}
{"type":"error","error":{"message":"rate limited"}}
`,
			wantError: "rate limited",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseStreamJSON(strings.NewReader(tt.input))
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("got %q, want %q", result, tt.want)
			}
		})
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

func createMockBinary(
	t *testing.T, stdout string, exitCode int, writeArgs bool, name string,
) (bin, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	dataFile := filepath.Join(dir, "stdout.txt")
	if err := os.WriteFile(dataFile, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}

	if writeArgs {
		argsFile = filepath.Join(dir, "args.txt")
	}

	if runtime.GOOS == "windows" {
		bin = filepath.Join(dir, name+".cmd")
		var script string
		if writeArgs {
			script = fmt.Sprintf("@echo %%* > %q\r\n@type %q\r\n@exit /b %d\r\n", argsFile, dataFile, exitCode)
		} else {
			script = fmt.Sprintf("@type %q\r\n@exit /b %d\r\n", dataFile, exitCode)
		}
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return bin, argsFile
	}

	bin = filepath.Join(dir, name)
	var script string
	if writeArgs {
		script = fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %s\ncat %s\nexit %d\n", shellQuote(argsFile), shellQuote(dataFile), exitCode)
	} else {
		script = fmt.Sprintf("#!/bin/sh\ncat %s\nexit %d\n", shellQuote(dataFile), exitCode)
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

// fakeClaudeBin writes a script that prints the given stdout
// and exits with the given code, ignoring all flags. Uses a
// .cmd batch file on Windows and a shell script elsewhere.
func fakeClaudeBin(
	t *testing.T, stdout string, exitCode int,
) string {
	bin, _ := createMockBinary(t, stdout, exitCode, false, "claude")
	return bin
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestGenerateClaude_SalvageOnNonZeroExit(t *testing.T) {
	tests := []struct {
		name       string
		stdout     string
		exitCode   int
		wantResult string
		wantErr    bool
	}{
		{
			name:       "non-zero exit with valid result",
			stdout:     `{"result":"# Analysis\nDone.","model":"m1"}`,
			exitCode:   1,
			wantResult: "# Analysis\nDone.",
		},
		{
			name:     "non-zero exit with empty result",
			stdout:   `{"result":"","model":"m1"}`,
			exitCode: 1,
			wantErr:  true,
		},
		{
			name:     "non-zero exit with invalid JSON",
			stdout:   `not json`,
			exitCode: 1,
			wantErr:  true,
		},
		{
			name:     "non-zero exit with no stdout",
			stdout:   "",
			exitCode: 1,
			wantErr:  true,
		},
		{
			name:       "zero exit with valid result",
			stdout:     `{"result":"OK","model":"m2"}`,
			exitCode:   0,
			wantResult: "OK",
		},
		{
			name:     "zero exit with empty result",
			stdout:   `{"result":"","model":"m2"}`,
			exitCode: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := fakeClaudeBin(
				t, tt.stdout, tt.exitCode,
			)
			result, err := generateClaude(
				context.Background(), bin, "test",
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Content != tt.wantResult {
				t.Errorf(
					"content = %q, want %q",
					result.Content, tt.wantResult,
				)
			}
			if result.Agent != "claude" {
				t.Errorf(
					"agent = %q, want claude",
					result.Agent,
				)
			}
		})
	}
}

// fakeGeminiBin writes a script that records its argv to an
// args file, then prints stream-json output. This lets tests
// verify both the CLI flags and the parsed result.
func fakeGeminiBin(
	t *testing.T, stdout string, exitCode int,
) (bin, argsFile string) {
	return createMockBinary(t, stdout, exitCode, true, "gemini")
}

func TestGenerateGemini_ModelFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test not supported on windows")
	}

	streamJSON := `{"type":"message","role":"assistant","content":"Hello"}
{"type":"result","result":"# Analysis"}
`

	bin, argsFile := fakeGeminiBin(t, streamJSON, 0)

	result, err := generateGemini(
		context.Background(), bin, "test prompt",
	)
	if err != nil {
		t.Fatalf("generateGemini: %v", err)
	}

	if result.Content != "# Analysis" {
		t.Errorf("Content = %q, want %q",
			result.Content, "# Analysis")
	}
	if result.Agent != "gemini" {
		t.Errorf("Agent = %q, want gemini", result.Agent)
	}
	if result.Model != geminiInsightModel {
		t.Errorf("Model = %q, want %q",
			result.Model, geminiInsightModel)
	}

	// Verify the CLI was invoked with --model flag.
	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args: %v", err)
	}
	args := strings.Split(
		strings.TrimSpace(string(argsData)), "\n",
	)

	wantArgs := []string{
		"--model", geminiInsightModel,
		"--output-format", "stream-json",
	}
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", args, wantArgs)
	}
	for i, want := range wantArgs {
		if args[i] != want {
			t.Errorf("arg[%d] = %q, want %q",
				i, args[i], want)
		}
	}
}

func TestGenerateClaude_CancelledContext(t *testing.T) {
	// Pre-cancelled context: cmd.Run fails (runErr != nil)
	// and ctx.Err() != nil → cancellation error.
	bin := fakeClaudeBin(
		t, `{"result":"OK","model":"m1"}`, 0,
	)
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	_, err := generateClaude(ctx, bin, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !strings.Contains(err.Error(), "cancel") {
		t.Errorf("error = %q, want cancel", err)
	}
}

func TestGenerateClaude_SuccessNotDiscarded(t *testing.T) {
	// Successful cmd.Run should return the result even if
	// the context is not fresh (regression test for gating
	// ctx.Err() on runErr != nil).
	bin := fakeClaudeBin(
		t, `{"result":"OK","model":"m1"}`, 0,
	)
	result, err := generateClaude(
		context.Background(), bin, "test",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "OK" {
		t.Errorf("content = %q, want OK", result.Content)
	}
}

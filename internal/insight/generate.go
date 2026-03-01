package insight

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// geminiInsightModel is the model passed to the gemini CLI
// for insight generation.
const geminiInsightModel = "gemini-3.1-pro-preview"

// Result holds the output from an AI agent invocation.
type Result struct {
	Content string
	Agent   string
	Model   string
}

// ValidAgents lists the supported agent names.
var ValidAgents = map[string]bool{
	"claude": true,
	"codex":  true,
	"gemini": true,
}

// GenerateFunc is the signature for insight generation,
// allowing tests to substitute a stub.
type GenerateFunc func(
	ctx context.Context, agent, prompt string,
) (Result, error)

// LogEvent represents one log line emitted by an insight
// agent process.
type LogEvent struct {
	Stream string `json:"stream"` // stdout|stderr
	Line   string `json:"line"`
}

// LogFunc receives real-time log events from the agent
// subprocess.
type LogFunc func(LogEvent)

// GenerateStreamFunc is like GenerateFunc but includes a
// log callback for streaming stdout/stderr events.
type GenerateStreamFunc func(
	ctx context.Context, agent, prompt string, onLog LogFunc,
) (Result, error)

// Generate invokes an AI agent CLI to generate an insight.
// The agent parameter selects which CLI to use (claude,
// codex, gemini). The prompt is passed via stdin.
func Generate(
	ctx context.Context, agent, prompt string,
) (Result, error) {
	return GenerateStream(ctx, agent, prompt, nil)
}

// GenerateStream invokes an AI agent CLI to generate an
// insight while optionally streaming process logs.
func GenerateStream(
	ctx context.Context, agent, prompt string, onLog LogFunc,
) (Result, error) {
	if !ValidAgents[agent] {
		return Result{}, fmt.Errorf(
			"unsupported agent: %s", agent,
		)
	}

	path, err := exec.LookPath(agent)
	if err != nil {
		return Result{}, fmt.Errorf(
			"%s CLI not found: %w", agent, err,
		)
	}

	switch agent {
	case "codex":
		return generateCodex(ctx, path, prompt, onLog)
	case "gemini":
		return generateGemini(ctx, path, prompt, onLog)
	default:
		return generateClaude(ctx, path, prompt, onLog)
	}
}

// allowedKeyPrefixes lists uppercase key prefixes that are
// safe to pass to agent CLI subprocesses. Matched
// case-insensitively so Windows-style casing (Path, ComSpec)
// is handled correctly. Using an allowlist prevents leaking
// secrets to child processes.
var allowedKeyPrefixes = []string{
	"PATH",
	"HOME", "USERPROFILE",
	"USER", "USERNAME", "LOGNAME",
	"LANG", "LC_",
	"TERM", "COLORTERM",
	"TMPDIR", "TEMP", "TMP",
	"XDG_",
	"SHELL",
	"SSL_CERT_", "CURL_CA_BUNDLE",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
	"SYSTEMROOT", "COMSPEC", "PATHEXT", "WINDIR",
	"HOMEDRIVE", "HOMEPATH",
	"APPDATA", "LOCALAPPDATA", "PROGRAMDATA",
}

// envKeyAllowed reports whether key (case-insensitive) is
// on the allowlist. Prefix entries ending with _ (LC_,
// XDG_, SSL_CERT_) match any key starting with that prefix;
// all others require an exact match.
func envKeyAllowed(key string) bool {
	upper := strings.ToUpper(key)
	for _, p := range allowedKeyPrefixes {
		if strings.HasSuffix(p, "_") {
			if strings.HasPrefix(upper, p) {
				return true
			}
		} else if upper == p {
			return true
		}
	}
	return false
}

// cleanEnv returns an allowlisted subset of the current
// environment for agent CLI subprocesses, plus
// CLAUDE_NO_SOUND=1.
func cleanEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if envKeyAllowed(k) {
			filtered = append(filtered, e)
		}
	}
	return append(filtered, "CLAUDE_NO_SOUND=1")
}

func emitLog(onLog LogFunc, stream, line string) {
	if onLog == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	onLog(LogEvent{
		Stream: stream,
		Line:   line,
	})
}

func collectStreamLines(
	r io.Reader, stream string, onLog LogFunc,
) <-chan string {
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		br := bufio.NewReader(r)
		var lines []string
		for {
			line, err := br.ReadString('\n')
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed != "" {
				lines = append(lines, trimmed)
				emitLog(onLog, stream, trimmed)
			}
			if err == nil {
				continue
			}
			if err == io.EOF {
				break
			}
			emitLog(
				onLog, "stderr",
				fmt.Sprintf("read %s: %v", stream, err),
			)
			_, _ = io.Copy(io.Discard, br)
			break
		}
		ch <- strings.Join(lines, "\n")
	}()
	return ch
}

// generateClaude invokes `claude -p --output-format json`.
func generateClaude(
	ctx context.Context, path, prompt string, onLog LogFunc,
) (Result, error) {
	cmd := exec.CommandContext(
		ctx, path,
		"-p", "--output-format", "json",
	)
	cmd.Env = append(os.Environ(), "CLAUDE_NO_SOUND=1")
	cmd.Stdin = strings.NewReader(prompt)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf(
			"create stdout pipe: %w", err,
		)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf(
			"create stderr pipe: %w", err,
		)
	}

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start claude: %w", err)
	}

	stderrDone := collectStreamLines(
		stderrPipe, "stderr", onLog,
	)
	stdoutBytes, readErr := io.ReadAll(stdoutPipe)
	if readErr != nil {
		_, _ = io.Copy(io.Discard, stdoutPipe)
	}
	stderrText := <-stderrDone
	runErr := cmd.Wait()
	if readErr != nil {
		return Result{}, fmt.Errorf(
			"read claude stdout: %w", readErr,
		)
	}

	emitLog(onLog, "stdout", string(stdoutBytes))

	// Honor context cancellation over salvaging stdout, but
	// only when the command actually failed. A successful
	// cmd.Run with a race-y post-completion cancel should
	// still return the valid result.
	if runErr != nil && ctx.Err() != nil {
		return Result{}, fmt.Errorf(
			"claude CLI cancelled: %w", ctx.Err(),
		)
	}

	var resp struct {
		Result string `json:"result"`
		Model  string `json:"model"`
	}
	if json.Unmarshal(stdoutBytes, &resp) == nil &&
		strings.TrimSpace(resp.Result) != "" {
		return Result{
			Content: resp.Result,
			Agent:   "claude",
			Model:   resp.Model,
		}, nil
	}

	if runErr != nil {
		return Result{}, fmt.Errorf(
			"claude CLI failed: %w\nstderr: %s",
			runErr, stderrText,
		)
	}

	return Result{}, fmt.Errorf(
		"claude returned empty result\nraw: %s",
		string(stdoutBytes),
	)
}

// generateCodex invokes `codex exec` in read-only sandbox
// and parses the JSONL stream for agent_message items.
func generateCodex(
	ctx context.Context, path, prompt string, onLog LogFunc,
) (Result, error) {
	cmd := exec.CommandContext(
		ctx, path,
		"exec", "--json",
		"--sandbox", "read-only", "-",
	)
	cmd.Stdin = strings.NewReader(prompt)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf(
			"create stdout pipe: %w", err,
		)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf(
			"create stderr pipe: %w", err,
		)
	}

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf(
			"start codex: %w", err,
		)
	}

	stderrDone := collectStreamLines(
		stderrPipe, "stderr", onLog,
	)
	content, parseErr := parseCodexStream(stdoutPipe, onLog)

	// Drain remaining stdout so cmd.Wait doesn't block.
	if parseErr != nil {
		_, _ = io.Copy(io.Discard, stdoutPipe)
	}

	stderrText := <-stderrDone
	if waitErr := cmd.Wait(); waitErr != nil {
		if parseErr != nil {
			return Result{}, fmt.Errorf(
				"codex failed: %w (parse: %v)\nstderr: %s",
				waitErr, parseErr, stderrText,
			)
		}
		return Result{}, fmt.Errorf(
			"codex failed: %w\nstderr: %s",
			waitErr, stderrText,
		)
	}
	if parseErr != nil {
		return Result{}, fmt.Errorf(
			"parse codex stream: %w\nstderr: %s",
			parseErr, stderrText,
		)
	}

	return Result{
		Content: content,
		Agent:   "codex",
	}, nil
}

// codexEvent represents a JSONL event from codex --json.
type codexEvent struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Item struct {
		ID   string `json:"id,omitempty"`
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"item,omitempty"`
}

// parseCodexStream reads codex JSONL and extracts
// agent_message text from item.completed/item.updated events.
func parseCodexStream(
	r io.Reader, onLog LogFunc,
) (string, error) {
	br := bufio.NewReader(r)
	var messages []string
	indexByID := make(map[string]int)

	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read stream: %w", err)
		}

		trimmed := strings.TrimSpace(line)
		emitLog(onLog, "stdout", trimmed)
		if trimmed != "" {
			var ev codexEvent
			if json.Unmarshal(
				[]byte(trimmed), &ev,
			) == nil {
				if ev.Type == "turn.failed" ||
					ev.Type == "error" {
					msg := ev.Error.Message
					if msg == "" {
						msg = "codex stream error"
					}
					return "", fmt.Errorf(
						"codex: %s", msg,
					)
				}

				isMsg := (ev.Type == "item.completed" ||
					ev.Type == "item.updated") &&
					ev.Item.Type == "agent_message" &&
					ev.Item.Text != ""
				if isMsg {
					if ev.Item.ID == "" {
						messages = append(
							messages, ev.Item.Text,
						)
					} else if idx, ok := indexByID[ev.Item.ID]; ok {
						messages[idx] = ev.Item.Text
					} else {
						indexByID[ev.Item.ID] = len(messages)
						messages = append(
							messages, ev.Item.Text,
						)
					}
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	return strings.Join(messages, "\n"), nil
}

// generateGemini invokes `gemini --output-format stream-json`
// and parses the JSONL stream for result/assistant messages.
func generateGemini(
	ctx context.Context, path, prompt string, onLog LogFunc,
) (Result, error) {
	cmd := exec.CommandContext(
		ctx, path,
		"--model", geminiInsightModel,
		"--output-format", "stream-json",
	)
	cmd.Stdin = strings.NewReader(prompt)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf(
			"create stdout pipe: %w", err,
		)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf(
			"create stderr pipe: %w", err,
		)
	}

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf(
			"start gemini: %w", err,
		)
	}

	stderrDone := collectStreamLines(
		stderrPipe, "stderr", onLog,
	)
	content, parseErr := parseStreamJSON(stdoutPipe, onLog)

	// Drain remaining stdout so cmd.Wait doesn't block.
	if parseErr != nil {
		_, _ = io.Copy(io.Discard, stdoutPipe)
	}

	stderrText := <-stderrDone
	if waitErr := cmd.Wait(); waitErr != nil {
		if parseErr != nil {
			return Result{}, fmt.Errorf(
				"gemini failed: %w (parse: %v)\nstderr: %s",
				waitErr, parseErr, stderrText,
			)
		}
		return Result{}, fmt.Errorf(
			"gemini failed: %w\nstderr: %s",
			waitErr, stderrText,
		)
	}
	if parseErr != nil {
		return Result{}, fmt.Errorf(
			"parse gemini stream: %w\nstderr: %s",
			parseErr, stderrText,
		)
	}

	return Result{
		Content: content,
		Agent:   "gemini",
		Model:   geminiInsightModel,
	}, nil
}

// streamMessage represents a JSONL event from stream-json
// output (shared format between Claude and Gemini CLIs).
type streamMessage struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Message struct {
		Content string `json:"content,omitempty"`
	} `json:"message,omitempty"`
	Result string `json:"result,omitempty"`
	Error  struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

// parseStreamJSON reads stream-json JSONL and extracts the
// result text. Prefers type=result, falls back to collecting
// assistant messages.
func parseStreamJSON(
	r io.Reader, onLog LogFunc,
) (string, error) {
	br := bufio.NewReader(r)
	var lastResult string
	var assistantMsgs []string

	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read stream: %w", err)
		}

		trimmed := strings.TrimSpace(line)
		emitLog(onLog, "stdout", trimmed)
		if trimmed != "" {
			var msg streamMessage
			if json.Unmarshal(
				[]byte(trimmed), &msg,
			) != nil {
				continue
			}
			if msg.Type == "error" {
				m := msg.Error.Message
				if m == "" {
					m = "stream error"
				}
				return "", fmt.Errorf(
					"stream: %s", m,
				)
			}
			if msg.Type == "message" &&
				msg.Role == "assistant" &&
				msg.Content != "" {
				assistantMsgs = append(
					assistantMsgs, msg.Content,
				)
			}
			if msg.Type == "assistant" &&
				msg.Message.Content != "" {
				assistantMsgs = append(
					assistantMsgs,
					msg.Message.Content,
				)
			}
			if msg.Type == "result" &&
				msg.Result != "" {
				lastResult = msg.Result
			}
		}

		if err == io.EOF {
			break
		}
	}

	if lastResult != "" {
		return lastResult, nil
	}
	if len(assistantMsgs) > 0 {
		return strings.Join(assistantMsgs, "\n"), nil
	}
	return "", nil
}

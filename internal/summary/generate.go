package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Result holds the output from an AI agent invocation.
type Result struct {
	Content string
	Agent   string
	Model   string
}

// claudeResponse is the JSON output from `claude -p --output-format json`.
type claudeResponse struct {
	Result string `json:"result"`
	Model  string `json:"model"`
}

// Generate invokes the claude CLI to generate a summary from
// the given prompt. The prompt is passed via stdin.
func Generate(
	ctx context.Context, prompt string,
) (Result, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return Result{}, fmt.Errorf(
			"claude CLI not found: %w "+
				"(install from https://docs.anthropic.com/en/docs/claude-code)",
			err,
		)
	}

	cmd := exec.CommandContext(
		ctx, path,
		"-p", "--output-format", "json",
	)
	cmd.Stdin = bytes.NewReader([]byte(prompt))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf(
			"claude CLI failed: %w\nstderr: %s",
			err, stderr.String(),
		)
	}

	var resp claudeResponse
	if err := json.Unmarshal(
		stdout.Bytes(), &resp,
	); err != nil {
		return Result{}, fmt.Errorf(
			"parsing claude output: %w\nraw: %s",
			err, stdout.String(),
		)
	}

	return Result{
		Content: resp.Result,
		Agent:   "claude",
		Model:   resp.Model,
	}, nil
}

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/wesm/agentsview/internal/config"
)

// resumeRequest is the JSON body for POST /api/v1/sessions/{id}/resume.
type resumeRequest struct {
	SkipPermissions bool `json:"skip_permissions"`
	ForkSession     bool `json:"fork_session"`
}

// resumeResponse is the JSON response for a resume request.
type resumeResponse struct {
	Launched bool   `json:"launched"`
	Terminal string `json:"terminal,omitempty"`
	Command  string `json:"command"`
	Error    string `json:"error,omitempty"`
}

// resumeAgents maps agent type strings to their resume command templates.
// The %s placeholder is replaced with the (quoted) session ID.
var resumeAgents = map[string]string{
	"claude":   "claude --resume %s",
	"codex":    "codex resume %s",
	"gemini":   "gemini --resume %s",
	"opencode": "opencode --session %s",
	"amp":      "amp --resume %s",
}

// terminalCandidates lists terminal emulators to try on Linux, in
// preference order. Each entry is {binary, args-before-command...}.
// The resume command is appended after the last arg.
var terminalCandidates = []struct {
	bin  string
	args []string
}{
	{"kitty", []string{"--"}},
	{"alacritty", []string{"-e"}},
	{"wezterm", []string{"start", "--"}},
	{"gnome-terminal", []string{"--", "bash", "-c"}},
	{"konsole", []string{"-e"}},
	{"xfce4-terminal", []string{"-e"}},
	{"tilix", []string{"-e"}},
	{"xterm", []string{"-e"}},
	{"x-terminal-emulator", []string{"-e"}},
}

func (s *Server) handleResumeSession(
	w http.ResponseWriter, r *http.Request,
) {
	id := r.PathValue("id")

	// Look up the session to get agent type.
	session, err := s.db.GetSession(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// Check if this agent supports resumption.
	tmpl, ok := resumeAgents[string(session.Agent)]
	if !ok {
		writeError(
			w, http.StatusBadRequest,
			fmt.Sprintf("agent %q does not support resume", session.Agent),
		)
		return
	}

	// Parse optional flags.
	var req resumeRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// Strip agent prefix from compound ID only when it matches the
	// expected agent (e.g. "codex:abc" → "abc"). Raw IDs that
	// happen to contain ":" are left untouched.
	rawID := id
	prefix := string(session.Agent) + ":"
	if strings.HasPrefix(rawID, prefix) {
		rawID = rawID[len(prefix):]
	}

	// Build the CLI command.
	cmd := fmt.Sprintf(tmpl, shellQuote(rawID))
	if string(session.Agent) == "claude" {
		if req.SkipPermissions {
			cmd += " --dangerously-skip-permissions"
		}
		if req.ForkSession {
			cmd += " --fork-session"
		}
	}

	// Check terminal config.
	s.mu.RLock()
	termCfg := s.cfg.Terminal
	s.mu.RUnlock()

	if termCfg.Mode == "clipboard" {
		// User explicitly chose clipboard-only mode.
		writeJSON(w, http.StatusOK, resumeResponse{
			Launched: false,
			Command:  cmd,
		})
		return
	}

	// Detect and launch a terminal.
	termBin, termArgs, termErr := detectTerminal(cmd, session.Project, termCfg)
	if termErr != nil {
		// Can't launch — return the command for clipboard fallback.
		log.Printf("resume: terminal detection failed: %v", termErr)
		writeJSON(w, http.StatusOK, resumeResponse{
			Launched: false,
			Command:  cmd,
			Error:    "no_terminal_found",
		})
		return
	}

	// Fire and forget — we don't need the terminal process to
	// complete before responding.
	proc := exec.Command(termBin, termArgs...)
	proc.Stdout = nil
	proc.Stderr = nil
	proc.Stdin = nil
	// If we have a project directory, use it as the working dir.
	if session.Project != "" {
		if info, err := os.Stat(session.Project); err == nil && info.IsDir() {
			proc.Dir = session.Project
		}
	}

	if err := proc.Start(); err != nil {
		log.Printf("resume: terminal start failed: %v", err)
		writeJSON(w, http.StatusOK, resumeResponse{
			Launched: false,
			Command:  cmd,
			Error:    "terminal_launch_failed",
		})
		return
	}

	// Detach — don't wait for the terminal process.
	go func() { _ = proc.Wait() }()

	writeJSON(w, http.StatusOK, resumeResponse{
		Launched: true,
		Terminal: termBin,
		Command:  cmd,
	})
}

// shellQuote applies POSIX single-quote escaping.
func shellQuote(s string) string {
	// Simple IDs: alphanumeric + hyphens need no quoting.
	safe := true
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// detectTerminal finds a suitable terminal emulator and builds the
// full argument list to launch the given command.
func detectTerminal(
	cmd string, cwd string, tc config.TerminalConfig,
) (bin string, args []string, err error) {
	// Custom terminal mode — use the user-configured binary + args.
	if tc.Mode == "custom" && tc.CustomBin != "" {
		path, lookErr := exec.LookPath(tc.CustomBin)
		if lookErr != nil {
			return "", nil, fmt.Errorf(
				"custom terminal %q not found: %w",
				tc.CustomBin, lookErr,
			)
		}
		if tc.CustomArgs != "" {
			// Split args on space and replace {cmd} placeholder.
			parts := strings.Fields(tc.CustomArgs)
			a := make([]string, 0, len(parts))
			for _, p := range parts {
				a = append(a, strings.ReplaceAll(p, "{cmd}", cmd))
			}
			return path, a, nil
		}
		// No args template — default pattern.
		return path, []string{"-e", "bash", "-c", cmd + "; exec bash"}, nil
	}

	switch runtime.GOOS {
	case "darwin":
		return detectTerminalDarwin(cmd, cwd)
	case "linux":
		return detectTerminalLinux(cmd)
	default:
		return "", nil, fmt.Errorf(
			"unsupported OS %q for terminal launch", runtime.GOOS,
		)
	}
}

func detectTerminalDarwin(
	cmd string, cwd string,
) (string, []string, error) {
	// Check for iTerm2 first, then fall back to Terminal.app.
	// Use osascript to tell the app to open a new window and run
	// the command.
	script := cmd
	if cwd != "" {
		if info, err := os.Stat(cwd); err == nil && info.IsDir() {
			script = fmt.Sprintf("cd %s && %s", shellQuote(cwd), cmd)
		}
	}

	// Try iTerm2 first.
	if _, err := exec.LookPath("osascript"); err == nil {
		// Sanitize for AppleScript: escape backslashes, then quotes,
		// and reject newlines to prevent multi-line injection.
		safe := strings.NewReplacer(
			"\n", " ",
			"\r", " ",
			`\`, `\\`,
			`"`, `\"`,
		).Replace(script)

		// Check if iTerm is installed.
		iterm := "/Applications/iTerm.app"
		if _, err := os.Stat(iterm); err == nil {
			appleScript := fmt.Sprintf(
				`tell application "iTerm"
					create window with default profile command "%s"
				end tell`,
				safe,
			)
			return "osascript", []string{"-e", appleScript}, nil
		}
		// Fall back to Terminal.app.
		appleScript := fmt.Sprintf(
			`tell application "Terminal"
				activate
				do script "%s"
			end tell`,
			safe,
		)
		return "osascript", []string{"-e", appleScript}, nil
	}
	return "", nil, fmt.Errorf("osascript not found on macOS")
}

func (s *Server) handleGetTerminalConfig(
	w http.ResponseWriter, _ *http.Request,
) {
	s.mu.RLock()
	tc := s.cfg.Terminal
	s.mu.RUnlock()
	if tc.Mode == "" {
		tc.Mode = "auto"
	}
	writeJSON(w, http.StatusOK, tc)
}

func (s *Server) handleSetTerminalConfig(
	w http.ResponseWriter, r *http.Request,
) {
	var tc config.TerminalConfig
	if err := json.NewDecoder(r.Body).Decode(&tc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	switch tc.Mode {
	case "auto", "custom", "clipboard":
		// ok
	default:
		writeError(w, http.StatusBadRequest,
			`mode must be "auto", "custom", or "clipboard"`)
		return
	}

	s.mu.Lock()
	err := s.cfg.SaveTerminalConfig(tc)
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tc)
}

func detectTerminalLinux(cmd string) (string, []string, error) {
	// Check $TERMINAL env var first.
	if envTerm := os.Getenv("TERMINAL"); envTerm != "" {
		if path, err := exec.LookPath(envTerm); err == nil {
			return path, []string{"-e", "bash", "-c", cmd}, nil
		}
	}

	// Try each candidate in preference order.
	for _, c := range terminalCandidates {
		path, err := exec.LookPath(c.bin)
		if err != nil {
			continue
		}

		args := make([]string, len(c.args))
		copy(args, c.args)

		// gnome-terminal uses "bash -c CMD" pattern.
		// Others use "-e CMD" pattern.
		switch c.bin {
		case "gnome-terminal":
			// gnome-terminal -- bash -c "CMD; exec bash"
			// The exec bash keeps the terminal open after the
			// command completes (or if the user Ctrl+C).
			args = []string{"--", "bash", "-c", cmd + "; exec bash"}
		case "kitty":
			args = []string{"--", "bash", "-c", cmd + "; exec bash"}
		case "alacritty":
			args = []string{"-e", "bash", "-c", cmd + "; exec bash"}
		case "wezterm":
			args = []string{"start", "--", "bash", "-c", cmd + "; exec bash"}
		case "konsole":
			args = []string{"-e", "bash", "-c", cmd + "; exec bash"}
		case "xfce4-terminal":
			args = []string{"-e", "bash -c '" + strings.ReplaceAll(cmd, "'", `'"'"'`) + "; exec bash'"}
		case "tilix":
			args = []string{"-e", "bash -c '" + strings.ReplaceAll(cmd, "'", `'"'"'`) + "; exec bash'"}
		case "xterm":
			args = []string{"-e", "bash", "-c", cmd + "; exec bash"}
		default:
			args = append(args, "bash", "-c", cmd+"; exec bash")
		}

		return path, args, nil
	}

	return "", nil, fmt.Errorf(
		"no terminal emulator found; install kitty, alacritty, " +
			"gnome-terminal, or set $TERMINAL",
	)
}

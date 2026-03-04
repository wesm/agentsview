package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Opener represents an application that can open a project directory.
type Opener struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"` // "editor", "terminal", "files", "action"
	Bin  string `json:"bin"`
}

// openerCandidate is a binary we check for on PATH.
type openerCandidate struct {
	id   string
	name string
	kind string
	bins []string // alternatives to try in order
}

// Linux + cross-platform candidates.
var openerCandidates = []openerCandidate{
	// File managers
	{"nautilus", "Files", "files", []string{"nautilus"}},
	{"dolphin", "Dolphin", "files", []string{"dolphin"}},
	{"thunar", "Thunar", "files", []string{"thunar"}},
	{"nemo", "Nemo", "files", []string{"nemo"}},
	{"pcmanfm", "PCManFM", "files", []string{"pcmanfm"}},

	// Editors / IDEs
	{"cursor", "Cursor", "editor", []string{"cursor"}},
	{"vscode", "VS Code", "editor", []string{"code"}},
	{"zed", "Zed", "editor", []string{"zed"}},
	{"sublime", "Sublime Text", "editor", []string{"subl"}},
	{"vim", "Vim", "editor", []string{"nvim", "vim"}},
	{"emacs", "Emacs", "editor", []string{"emacs"}},
	{"intellij", "IntelliJ IDEA", "editor", []string{"idea"}},
	{"goland", "GoLand", "editor", []string{"goland"}},
	{"webstorm", "WebStorm", "editor", []string{"webstorm"}},

	// Terminals
	{"ghostty", "Ghostty", "terminal", []string{"ghostty"}},
	{"kitty", "kitty", "terminal", []string{"kitty"}},
	{"alacritty", "Alacritty", "terminal", []string{"alacritty"}},
	{"wezterm", "WezTerm", "terminal", []string{"wezterm"}},
	{"gnome-terminal", "GNOME Terminal", "terminal", []string{"gnome-terminal"}},
	{"konsole", "Konsole", "terminal", []string{"konsole"}},
	{"xfce4-terminal", "Xfce Terminal", "terminal", []string{"xfce4-terminal"}},
	{"tilix", "Tilix", "terminal", []string{"tilix"}},
	{"xterm", "xterm", "terminal", []string{"xterm"}},
}

// macOS-specific candidates.
var darwinOpenerCandidates = []openerCandidate{
	// File manager is always Finder on macOS — use "open" command
	{"finder", "Finder", "files", []string{"open"}},

	// Editors
	{"cursor", "Cursor", "editor", []string{"cursor"}},
	{"vscode", "VS Code", "editor", []string{"code"}},
	{"zed", "Zed", "editor", []string{"zed"}},
	{"xcode", "Xcode", "editor", []string{"xed"}},
	{"sublime", "Sublime Text", "editor", []string{"subl"}},
	{"vim", "Vim", "editor", []string{"nvim", "vim"}},

	// Terminals
	{"ghostty", "Ghostty", "terminal", []string{"ghostty"}},
	{"iterm2", "iTerm2", "terminal", []string{"iterm2"}},
	{"kitty", "kitty", "terminal", []string{"kitty"}},
	{"alacritty", "Alacritty", "terminal", []string{"alacritty"}},
	{"wezterm", "WezTerm", "terminal", []string{"wezterm"}},
	{"terminal", "Terminal", "terminal", []string{"open"}}, // Terminal.app
}

// cachedOpeners stores detected openers with a TTL.
var (
	openerCache   []Opener
	openerCacheMu sync.Mutex
	openerCacheAt time.Time
	openerCacheTTL = 60 * time.Second
)

func detectOpeners() []Opener {
	openerCacheMu.Lock()
	defer openerCacheMu.Unlock()

	if time.Since(openerCacheAt) < openerCacheTTL && openerCache != nil {
		return openerCache
	}

	candidates := openerCandidates
	if runtime.GOOS == "darwin" {
		candidates = darwinOpenerCandidates
	}

	var result []Opener
	for _, c := range candidates {
		for _, bin := range c.bins {
			path, err := exec.LookPath(bin)
			if err != nil {
				continue
			}
			result = append(result, Opener{
				ID:   c.id,
				Name: c.name,
				Kind: c.kind,
				Bin:  path,
			})
			break // found one binary for this candidate
		}
	}

	openerCache = result
	openerCacheAt = time.Now()
	return result
}

func (s *Server) handleListOpeners(
	w http.ResponseWriter, _ *http.Request,
) {
	openers := detectOpeners()
	if openers == nil {
		openers = []Opener{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"openers": openers,
	})
}

type openRequest struct {
	OpenerID string `json:"opener_id"`
}

func (s *Server) handleOpenSession(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")
	session, err := s.db.GetSession(r.Context(), sessionID)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req openRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Find the project directory.
	projectDir := session.Project
	if session.FilePath != nil {
		if cwd := readSessionCwd(*session.FilePath); cwd != "" {
			projectDir = cwd
		}
	}
	if projectDir == "" {
		writeError(w, http.StatusBadRequest, "session has no project directory")
		return
	}

	// Find the opener.
	openers := detectOpeners()
	var opener *Opener
	for i := range openers {
		if openers[i].ID == req.OpenerID {
			opener = &openers[i]
			break
		}
	}
	if opener == nil {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("opener %q not found", req.OpenerID))
		return
	}

	// Launch the opener with the project directory.
	if err := launchOpener(*opener, projectDir); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to launch")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"launched": true,
		"opener":   opener.Name,
		"path":     projectDir,
	})
}

func launchOpener(o Opener, dir string) error {
	var cmd *exec.Cmd

	switch o.Kind {
	case "files":
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", dir)
		} else {
			cmd = exec.Command(o.Bin, dir)
		}
	case "editor":
		if runtime.GOOS == "darwin" && o.ID == "xcode" {
			cmd = exec.Command(o.Bin, dir)
		} else {
			cmd = exec.Command(o.Bin, dir)
		}
	case "terminal":
		cmd = launchTerminalInDir(o, dir)
	default:
		return fmt.Errorf("unsupported opener kind: %s", o.Kind)
	}

	if cmd == nil {
		return fmt.Errorf("could not build command for opener %s", o.Name)
	}

	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func launchTerminalInDir(o Opener, dir string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		switch o.ID {
		case "iterm2":
			// Build shell command first, then AppleScript-escape the
			// entire string once. shellQuote provides POSIX quoting for
			// the directory, then escapeForAppleScript escapes the whole
			// command for embedding in an AppleScript string literal.
			shellCmd := fmt.Sprintf("cd %s && exec bash", shellQuote(dir))
			script := fmt.Sprintf(
				`tell application "iTerm"
					create window with default profile command "%s"
				end tell`,
				escapeForAppleScript(shellCmd),
			)
			return exec.Command("osascript", "-e", script)
		case "terminal":
			shellCmd := fmt.Sprintf("cd %s", shellQuote(dir))
			script := fmt.Sprintf(
				`tell application "Terminal"
					activate
					do script "%s"
				end tell`,
				escapeForAppleScript(shellCmd),
			)
			return exec.Command("osascript", "-e", script)
		}
	}

	switch o.ID {
	case "kitty":
		return exec.Command(o.Bin, "--directory", dir)
	case "alacritty":
		return exec.Command(o.Bin, "--working-directory", dir)
	case "wezterm":
		return exec.Command(o.Bin, "start", "--cwd", dir)
	case "gnome-terminal":
		return exec.Command(o.Bin, "--working-directory="+dir)
	case "konsole":
		return exec.Command(o.Bin, "--workdir", dir)
	case "xfce4-terminal":
		return exec.Command(o.Bin, "--default-working-directory="+dir)
	case "tilix":
		return exec.Command(o.Bin, "--working-directory="+dir)
	case "ghostty":
		return exec.Command(o.Bin, "--working-directory="+dir)
	default:
		// generic: try --working-directory, fallback to cd
		cmd := exec.Command(o.Bin)
		cmd.Dir = dir
		return cmd
	}
}

// escapeForAppleScript escapes a string for embedding inside an
// AppleScript double-quoted string literal. Does NOT add outer quotes.
func escapeForAppleScript(s string) string {
	return strings.NewReplacer(
		"\n", " ",
		"\r", " ",
		`\`, `\\`,
		`"`, `\"`,
	).Replace(s)
}

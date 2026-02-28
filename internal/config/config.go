package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Host              string        `json:"host"`
	Port              int           `json:"port"`
	NoBrowser         bool          `json:"no_browser"`
	ClaudeProjectDir  string        `json:"claude_project_dir"`
	CodexSessionsDir  string        `json:"codex_sessions_dir"`
	CopilotDir        string        `json:"copilot_dir"`
	GeminiDir         string        `json:"gemini_dir"`
	OpenCodeDir       string        `json:"opencode_dir"`
	CursorProjectsDir string        `json:"cursor_projects_dir"`
	IflowDir          string        `json:"iflow_dir"`
	DataDir           string        `json:"data_dir"`
	DBPath            string        `json:"-"`
	CursorSecret      string        `json:"cursor_secret"`
	GithubToken       string        `json:"github_token,omitempty"`
	WriteTimeout      time.Duration `json:"-"`

	// Multi-directory support (from config.json).
	// When set, these take precedence over the single-dir
	// fields above. Env vars override these with a
	// single-element slice.
	ClaudeProjectDirs []string `json:"claude_project_dirs,omitempty"`
	CodexSessionsDirs []string `json:"codex_sessions_dirs,omitempty"`
	CopilotDirs       []string `json:"copilot_dirs,omitempty"`
	GeminiDirs        []string `json:"gemini_dirs,omitempty"`
	OpenCodeDirs      []string `json:"opencode_dirs,omitempty"`
	IflowDirs         []string `json:"iflow_dirs,omitempty"`
}

// Default returns a Config with default values.
func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf(
			"determining home directory: %w", err,
		)
	}
	dataDir := filepath.Join(home, ".agentsview")
	return Config{
		Host:              "127.0.0.1",
		Port:              8080,
		ClaudeProjectDir:  filepath.Join(home, ".claude", "projects"),
		CodexSessionsDir:  filepath.Join(home, ".codex", "sessions"),
		CopilotDir:        filepath.Join(home, ".copilot"),
		GeminiDir:         filepath.Join(home, ".gemini"),
		OpenCodeDir:       filepath.Join(home, ".local", "share", "opencode"),
		CursorProjectsDir: filepath.Join(home, ".cursor", "projects"),
		IflowDir:          filepath.Join(home, ".iflow", "projects"),
		DataDir:           dataDir,
		DBPath:            filepath.Join(dataDir, "sessions.db"),
		WriteTimeout:      30 * time.Second,
	}, nil
}

// Load builds a Config by layering: defaults < config file < env < flags.
// The provided FlagSet must already be parsed by the caller.
// Only flags that were explicitly set override the lower layers.
func Load(fs *flag.FlagSet) (Config, error) {
	cfg, err := LoadMinimal()
	if err != nil {
		return cfg, err
	}
	applyFlags(&cfg, fs)
	return cfg, nil
}

// LoadMinimal builds a Config from defaults, env, and config file,
// without parsing CLI flags. Use this for subcommands that manage
// their own flag sets.
func LoadMinimal() (Config, error) {
	cfg, err := Default()
	if err != nil {
		return cfg, err
	}
	cfg.loadEnv()

	if err := cfg.loadFile(); err != nil {
		return cfg, fmt.Errorf("loading config file: %w", err)
	}
	if err := cfg.ensureCursorSecret(); err != nil {
		return cfg, fmt.Errorf("ensuring cursor secret: %w", err)
	}
	cfg.DBPath = filepath.Join(cfg.DataDir, "sessions.db")
	return cfg, nil
}

func (c *Config) configPath() string {
	return filepath.Join(c.DataDir, "config.json")
}

func (c *Config) loadFile() error {
	data, err := os.ReadFile(c.configPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var file struct {
		GithubToken       string   `json:"github_token"`
		CursorSecret      string   `json:"cursor_secret"`
		ClaudeProjectDirs []string `json:"claude_project_dirs"`
		CodexSessionsDirs []string `json:"codex_sessions_dirs"`
		CopilotDirs       []string `json:"copilot_dirs"`
		GeminiDirs        []string `json:"gemini_dirs"`
		OpenCodeDirs      []string `json:"opencode_dirs"`
		IflowDirs         []string `json:"iflow_dirs"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}
	if file.GithubToken != "" {
		c.GithubToken = file.GithubToken
	}
	if file.CursorSecret != "" {
		c.CursorSecret = file.CursorSecret
	}
	// Only apply config-file arrays when not already set by
	// env var. loadEnv runs before loadFile, so a non-nil
	// slice here means the env var won.
	if len(file.ClaudeProjectDirs) > 0 && c.ClaudeProjectDirs == nil {
		c.ClaudeProjectDirs = file.ClaudeProjectDirs
	}
	if len(file.CodexSessionsDirs) > 0 && c.CodexSessionsDirs == nil {
		c.CodexSessionsDirs = file.CodexSessionsDirs
	}
	if len(file.CopilotDirs) > 0 && c.CopilotDirs == nil {
		c.CopilotDirs = file.CopilotDirs
	}
	if len(file.GeminiDirs) > 0 && c.GeminiDirs == nil {
		c.GeminiDirs = file.GeminiDirs
	}
	if len(file.OpenCodeDirs) > 0 && c.OpenCodeDirs == nil {
		c.OpenCodeDirs = file.OpenCodeDirs
	}
	if len(file.IflowDirs) > 0 && c.IflowDirs == nil {
		c.IflowDirs = file.IflowDirs
	}
	return nil
}

func (c *Config) ensureCursorSecret() error {
	if c.CursorSecret != "" {
		return nil
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("generating secret: %w", err)
	}
	secret := base64.StdEncoding.EncodeToString(b)
	c.CursorSecret = secret

	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing := make(map[string]any)
	data, err := os.ReadFile(c.configPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("existing config invalid: %w", err)
		}
	}

	existing["cursor_secret"] = secret
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(c.configPath(), out, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) loadEnv() {
	if v := os.Getenv("CLAUDE_PROJECTS_DIR"); v != "" {
		c.ClaudeProjectDir = v
		c.ClaudeProjectDirs = []string{v}
	}
	if v := os.Getenv("CODEX_SESSIONS_DIR"); v != "" {
		c.CodexSessionsDir = v
		c.CodexSessionsDirs = []string{v}
	}
	if v := os.Getenv("COPILOT_DIR"); v != "" {
		c.CopilotDir = v
		c.CopilotDirs = []string{v}
	}
	if v := os.Getenv("GEMINI_DIR"); v != "" {
		c.GeminiDir = v
		c.GeminiDirs = []string{v}
	}
	if v := os.Getenv("OPENCODE_DIR"); v != "" {
		c.OpenCodeDir = v
		c.OpenCodeDirs = []string{v}
	}
	if v := os.Getenv("CURSOR_PROJECTS_DIR"); v != "" {
		c.CursorProjectsDir = v
	}
	if v := os.Getenv("IFLOW_DIR"); v != "" {
		c.IflowDir = v
		c.IflowDirs = []string{v}
	}
	if v := os.Getenv("AGENT_VIEWER_DATA_DIR"); v != "" {
		c.DataDir = v
	}
}

// ResolveClaudeDirs returns the effective list of Claude
// project directories. Precedence: env var (single) >
// config file array > default (single).
func (c *Config) ResolveClaudeDirs() []string {
	return c.resolveDirs(c.ClaudeProjectDirs, c.ClaudeProjectDir)
}

func (c *Config) ResolveCodexDirs() []string {
	return c.resolveDirs(c.CodexSessionsDirs, c.CodexSessionsDir)
}

func (c *Config) ResolveCopilotDirs() []string {
	return c.resolveDirs(c.CopilotDirs, c.CopilotDir)
}

func (c *Config) ResolveGeminiDirs() []string {
	return c.resolveDirs(c.GeminiDirs, c.GeminiDir)
}

func (c *Config) ResolveOpenCodeDirs() []string {
	return c.resolveDirs(c.OpenCodeDirs, c.OpenCodeDir)
}

func (c *Config) ResolveIflowDirs() []string {
	return c.resolveDirs(c.IflowDirs, c.IflowDir)
}

func (c *Config) resolveDirs(multi []string, single string) []string {
	if len(multi) > 0 {
		return multi
	}
	if single != "" {
		return []string{single}
	}
	return nil
}

// RegisterServeFlags registers serve-command flags on fs.
// The caller must call fs.Parse before passing fs to Load.
func RegisterServeFlags(fs *flag.FlagSet) {
	fs.String("host", "127.0.0.1", "Host to bind to")
	fs.Int("port", 8080, "Port to listen on")
	fs.Bool(
		"no-browser", false,
		"Don't open browser on startup",
	)
}

// applyFlags copies explicitly-set flags from fs into cfg.
func applyFlags(cfg *Config, fs *flag.FlagSet) {
	if fs == nil {
		return
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			cfg.Host = f.Value.String()
		case "port":
			// flag already validated the int; ignore parse error
			cfg.Port, _ = strconv.Atoi(f.Value.String())
		case "no-browser":
			cfg.NoBrowser = f.Value.String() == "true"
		}
	})
}

// ResolveDataDir returns the effective data directory by applying
// defaults and environment overrides, without reading any files.
// Use this to determine where migration should target before
// calling Load or LoadMinimal.
func ResolveDataDir() (string, error) {
	cfg, err := Default()
	if err != nil {
		return "", err
	}
	if v := os.Getenv("AGENT_VIEWER_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	return cfg.DataDir, nil
}

// SaveGithubToken persists the GitHub token to the config file.
func (c *Config) SaveGithubToken(token string) error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	existing := make(map[string]any)
	data, err := os.ReadFile(c.configPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config file: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf(
				"existing config is invalid, cannot update: %w",
				err,
			)
		}
	}

	existing["github_token"] = token
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(c.configPath(), out, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	c.GithubToken = token
	return nil
}

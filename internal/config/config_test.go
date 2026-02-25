package config

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipIfNotUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(
			"skipping: Unix permissions not reliable on Windows",
		)
	}
	if os.Getuid() == 0 {
		t.Skip(
			"skipping: running as root bypasses permissions",
		)
	}
}

func writeConfig(t *testing.T, dir string, data any) {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// setupConfigDir creates a temp data dir, sets the env var,
// and returns (dir, configPath).
func setupConfigDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGENT_VIEWER_DATA_DIR", dir)
	return dir, filepath.Join(dir, "config.json")
}

// writeConfigRaw writes raw string content to config.json.
// Use writeConfig for structured data; use this for exact
// string control or intentionally invalid JSON.
func writeConfigRaw(
	t *testing.T, dir string, content string,
) {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(
		path, []byte(content), 0o600,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// readConfigFile reads and unmarshals config.json into a Config.
func readConfigFile(t *testing.T, dir string) Config {
	t.Helper()
	data, err := os.ReadFile(
		filepath.Join(dir, "config.json"),
	)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing config file: %v", err)
	}
	return cfg
}

// configWithTmpDir returns a Config with DataDir set to a fresh
// temp directory.
func configWithTmpDir(t *testing.T) (Config, string) {
	t.Helper()
	dir := t.TempDir()
	return Config{DataDir: dir}, dir
}

func loadConfigFromFlags(t *testing.T, args ...string) (Config, error) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	RegisterServeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	return Load(fs)
}

func TestLoadEnv_OverridesDataDir(t *testing.T) {
	custom, _ := setupConfigDir(t)

	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.loadEnv()

	if cfg.DataDir != custom {
		t.Errorf(
			"DataDir = %q, want %q", cfg.DataDir, custom,
		)
	}
}

func TestLoad_AppliesExplicitFlags(t *testing.T) {
	cfg, err := loadConfigFromFlags(t, "-host", "0.0.0.0", "-port", "9090")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9090)
	}
}

func TestLoad_DefaultsWithoutFlags(t *testing.T) {
	cfg, err := loadConfigFromFlags(t)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf(
			"Host = %q, want default %q",
			cfg.Host, "127.0.0.1",
		)
	}
	if cfg.Port != 8080 {
		t.Errorf(
			"Port = %d, want default %d", cfg.Port, 8080,
		)
	}
}

func TestLoad_NilFlagSet(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", cfg.Host, "127.0.0.1")
	}
}

func TestSaveGithubToken_RejectsCorruptConfig(t *testing.T) {
	cfg, tmp := configWithTmpDir(t)

	// Write invalid JSON to config file
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(
		path, []byte("not json"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	err := cfg.SaveGithubToken("tok")
	if err == nil {
		t.Fatal("expected error for corrupt config")
	}
}

func TestSaveGithubToken_ReturnsErrorOnReadFailure(t *testing.T) {
	skipIfNotUnix(t)

	cfg, tmp := configWithTmpDir(t)

	// Create a config file that is not readable
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(
		path, []byte(`{"k":"v"}`), 0o000,
	); err != nil {
		t.Fatal(err)
	}

	err := cfg.SaveGithubToken("tok")
	if err == nil {
		t.Fatal("expected error for unreadable config file")
	}
	if !strings.Contains(err.Error(), "reading config file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSaveGithubToken_PreservesExistingKeys(t *testing.T) {
	cfg, tmp := configWithTmpDir(t)

	existing := map[string]any{"custom_key": "value"}
	writeConfig(t, tmp, existing)

	if err := cfg.SaveGithubToken("new-token"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatal(err)
	}
	if result["custom_key"] != "value" {
		t.Errorf(
			"custom_key = %v, want %q",
			result["custom_key"], "value",
		)
	}
	if result["github_token"] != "new-token" {
		t.Errorf(
			"github_token = %v, want %q",
			result["github_token"], "new-token",
		)
	}
}

func TestLoadFile_ReadsDirArrays(t *testing.T) {
	dir, _ := setupConfigDir(t)
	writeConfig(t, dir, map[string]any{
		"claude_project_dirs": []string{"/path/one", "/path/two"},
		"codex_sessions_dirs": []string{"/codex/a"},
	})

	cfg, err := LoadMinimal()
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.ClaudeProjectDirs) != 2 {
		t.Fatalf("ClaudeProjectDirs len = %d, want 2", len(cfg.ClaudeProjectDirs))
	}
	if cfg.ClaudeProjectDirs[0] != "/path/one" || cfg.ClaudeProjectDirs[1] != "/path/two" {
		t.Errorf("ClaudeProjectDirs = %v", cfg.ClaudeProjectDirs)
	}
	if len(cfg.CodexSessionsDirs) != 1 || cfg.CodexSessionsDirs[0] != "/codex/a" {
		t.Errorf("CodexSessionsDirs = %v", cfg.CodexSessionsDirs)
	}
}

func TestResolveDirs_DefaultOnly(t *testing.T) {
	dir, _ := setupConfigDir(t)
	// Empty config file — no dir arrays
	writeConfig(t, dir, map[string]any{})

	cfg, err := LoadMinimal()
	if err != nil {
		t.Fatal(err)
	}

	dirs := cfg.ResolveClaudeDirs()
	if len(dirs) != 1 {
		t.Fatalf("len = %d, want 1", len(dirs))
	}
	if dirs[0] != cfg.ClaudeProjectDir {
		t.Errorf("got %q, want %q", dirs[0], cfg.ClaudeProjectDir)
	}
}

func TestResolveDirs_ConfigFileOverridesDefault(t *testing.T) {
	dir, _ := setupConfigDir(t)
	writeConfig(t, dir, map[string]any{
		"claude_project_dirs": []string{"/a", "/b"},
	})

	cfg, err := LoadMinimal()
	if err != nil {
		t.Fatal(err)
	}

	dirs := cfg.ResolveClaudeDirs()
	if len(dirs) != 2 || dirs[0] != "/a" || dirs[1] != "/b" {
		t.Errorf("got %v, want [/a /b]", dirs)
	}
}

func TestResolveDirs_EnvVarOverridesConfigFile(t *testing.T) {
	dir, _ := setupConfigDir(t)
	writeConfig(t, dir, map[string]any{
		"claude_project_dirs": []string{"/a", "/b"},
	})
	t.Setenv("CLAUDE_PROJECTS_DIR", "/env/override")

	cfg, err := LoadMinimal()
	if err != nil {
		t.Fatal(err)
	}

	dirs := cfg.ResolveClaudeDirs()
	if len(dirs) != 1 || dirs[0] != "/env/override" {
		t.Errorf("got %v, want [/env/override]", dirs)
	}
}

func TestResolveDataDir_DefaultAndEnvOverride(t *testing.T) {
	// Without env override, should return default
	dir, err := ResolveDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Error("ResolveDataDir returned empty string")
	}

	// With env override, should return the override
	custom := t.TempDir()
	t.Setenv("AGENT_VIEWER_DATA_DIR", custom)
	dir, err = ResolveDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != custom {
		t.Errorf("ResolveDataDir = %q, want %q", dir, custom)
	}
}

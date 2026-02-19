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

const (
	legacyDirName = ".agent-session-viewer"
	newDirName    = ".agentsview"
)

// setupLegacyEnv creates a temp directory with a populated legacy
// data dir and returns (tmp, newDir). Files are written into the
// legacy directory with config.json getting 0o600 permissions and
// all other files getting 0o644.
func setupLegacyEnv(
	t *testing.T, files map[string]string,
) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	legacyDir := filepath.Join(tmp, legacyDirName)
	newDir := filepath.Join(tmp, newDirName)

	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}

	for name, content := range files {
		perm := os.FileMode(0o644)
		if name == "config.json" {
			perm = 0o600
		}
		path := filepath.Join(legacyDir, name)
		if err := os.WriteFile(
			path, []byte(content), perm,
		); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return tmp, newDir
}

func assertFileContent(
	t *testing.T, path, expected string,
) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	if string(data) != expected {
		t.Errorf(
			"%s content = %q, want %q",
			filepath.Base(path), data, expected,
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

func loadConfigFromFlags(t *testing.T, args ...string) (Config, error) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	RegisterServeFlags(fs)
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	return Load(fs)
}

func TestMigrateFromLegacy(t *testing.T) {
	tests := []struct {
		name         string
		legacyFiles  map[string]string
		preCreateNew bool
		wantFiles    map[string]string // Content to assert in new dir
		wantMissing  []string          // Files that should NOT exist
	}{
		{
			name: "CopiesGoDBAndConfig",
			legacyFiles: map[string]string{
				"sessions-go.db": "go-db-content",
				"config.json":    `{"github_token": "secret"}`,
			},
			wantFiles: map[string]string{
				"sessions.db": "go-db-content",
				"config.json": `{"github_token": "secret"}`,
			},
		},
		{
			name: "CopiesGoDBOnly",
			legacyFiles: map[string]string{
				"sessions-go.db": "just-db",
			},
			wantFiles: map[string]string{
				"sessions.db": "just-db",
			},
			wantMissing: []string{"config.json"},
		},
		{
			name: "IgnoresPythonDB",
			legacyFiles: map[string]string{
				"sessions.db": "python-db",
				"config.json": `{"github_token":"tok"}`,
			},
			wantFiles: map[string]string{
				"config.json": `{"github_token":"tok"}`,
			},
			wantMissing: []string{"sessions.db"},
		},
		{
			name: "SkipsIfNewDirExists",
			legacyFiles: map[string]string{
				"sessions.db": "db",
			},
			preCreateNew: true,
			wantMissing:  []string{"sessions.db"},
		},
		{
			name:        "SkipsIfNoLegacyDir",
			legacyFiles: nil,
			wantMissing: []string{"."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmp, newDir string
			if tt.legacyFiles != nil {
				tmp, newDir = setupLegacyEnv(t, tt.legacyFiles)
			} else {
				tmp = t.TempDir()
				newDir = filepath.Join(tmp, newDirName)
			}

			if tt.preCreateNew {
				if err := os.MkdirAll(newDir, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			t.Setenv("HOME", tmp)
			MigrateFromLegacy(newDir)

			if tt.legacyFiles == nil {
				if _, err := os.Stat(newDir); err == nil {
					t.Error("new dir should not be created without legacy dir")
				}
				return
			}

			for path, content := range tt.wantFiles {
				assertFileContent(t, filepath.Join(newDir, path), content)
			}

			for _, path := range tt.wantMissing {
				if _, err := os.Stat(filepath.Join(newDir, path)); err == nil {
					t.Errorf("file %s should not exist", path)
				}
			}
		})
	}
}

func TestMigrateFromLegacy_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission checks not reliable on Windows")
	}

	tmp, newDir := setupLegacyEnv(t, map[string]string{
		"sessions-go.db": "db",
		"config.json":    `{"github_token":"x"}`,
	})

	t.Setenv("HOME", tmp)
	MigrateFromLegacy(newDir)

	// Data dir must not be group/other accessible
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf(
			"data dir perm = %o, group/other bits should be 0",
			perm,
		)
	}

	// config.json must not be group/other readable
	info, err = os.Stat(filepath.Join(newDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf(
			"config.json perm = %o, group/other bits should be 0",
			perm,
		)
	}

	// sessions.db should be owner-accessible
	info, err = os.Stat(filepath.Join(newDir, "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o400 == 0 {
		t.Errorf(
			"sessions.db perm = %o, owner should have read",
			perm,
		)
	}
}

func TestLoadEnv_OverridesDataDir(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("AGENT_VIEWER_DATA_DIR", custom)

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
	tmp := t.TempDir()
	cfg := Config{DataDir: tmp}

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
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}

	tmp := t.TempDir()
	cfg := Config{DataDir: tmp}

	// Create a config file that is not readable
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(
		path, []byte(`{"k":"v"}`), 0o000,
	); err != nil {
		t.Fatal(err)
	}

	// Verify the file is actually unreadable. If we're running as root
	// (or in an environment that overrides permissions), we can still read it.
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("skipping test: 0o000 file is readable (running as root?)")
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
	tmp := t.TempDir()
	cfg := Config{DataDir: tmp}

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

// TestMigrateThenLoad_GithubTokenAvailable verifies that the
// startup sequence (resolve data dir, migrate, load) makes
// legacy github_token immediately available without a second
// load.
func TestMigrateThenLoad_GithubTokenAvailable(t *testing.T) {
	cfgJSON, _ := json.Marshal(map[string]string{
		"github_token": "legacy-secret",
	})
	tmp, newDir := setupLegacyEnv(t, map[string]string{
		"config.json": string(cfgJSON),
	})

	t.Setenv("HOME", tmp)
	t.Setenv("AGENT_VIEWER_DATA_DIR", newDir)

	// Simulate startup: resolve, migrate, then load
	dataDir, err := ResolveDataDir()
	if err != nil {
		t.Fatal(err)
	}
	MigrateFromLegacy(dataDir)

	cfg, err := LoadMinimal()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GithubToken != "legacy-secret" {
		t.Errorf(
			"GithubToken = %q, want %q",
			cfg.GithubToken, "legacy-secret",
		)
	}
}

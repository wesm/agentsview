package main

import (
	"path/filepath"
	"testing"
)

func TestMustLoadConfig(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantHost      string
		wantPort      int
		wantNoBrowser bool
	}{
		{
			name:          "DefaultArgs",
			args:          []string{},
			wantHost:      "127.0.0.1",
			wantPort:      8080,
			wantNoBrowser: false,
		},
		{
			name:          "ExplicitFlags",
			args:          []string{"-host", "0.0.0.0", "-port", "9090", "-no-browser"},
			wantHost:      "0.0.0.0",
			wantPort:      9090,
			wantNoBrowser: true,
		},
		{
			name:          "PartialFlags",
			args:          []string{"-port", "3000"},
			wantHost:      "127.0.0.1",
			wantPort:      3000,
			wantNoBrowser: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGENT_VIEWER_DATA_DIR", t.TempDir())
			cfg := mustLoadConfig(tt.args)

			if cfg.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.wantHost)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.NoBrowser != tt.wantNoBrowser {
				t.Errorf("NoBrowser = %v, want %v", cfg.NoBrowser, tt.wantNoBrowser)
			}

			if cfg.DataDir == "" {
				t.Error("DataDir should be set")
			}
			wantDBPath := filepath.Join(cfg.DataDir, "sessions.db")
			if cfg.DBPath != wantDBPath {
				t.Errorf("DBPath = %q, want %q", cfg.DBPath, wantDBPath)
			}
		})
	}
}

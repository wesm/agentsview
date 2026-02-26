package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func skipIfNotUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("skipping: Unix permissions not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping: running as root bypasses permissions")
	}
}

func assertCacheCleared(t *testing.T, sourcePath string, lastMtime int64) {
	t.Helper()
	if sourcePath != "" {
		t.Errorf("expected sourcePath cleared, got %q", sourcePath)
	}
	if lastMtime != 0 {
		t.Errorf("expected lastMtime cleared, got %d", lastMtime)
	}
}

func assertCachePreserved(t *testing.T, sourcePath, wantPath string, lastMtime, wantMtime int64) {
	t.Helper()
	if sourcePath != wantPath {
		t.Errorf("sourcePath = %q, want %q", sourcePath, wantPath)
	}
	if lastMtime != wantMtime {
		t.Errorf("lastMtime = %d, want %d", lastMtime, wantMtime)
	}
}

func makeUnreadableDir(t *testing.T) string {
	t.Helper()
	skipIfNotUnix(t)
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(subDir, "target")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(subDir, 0o755) })
	if err := os.Chmod(subDir, 0o000); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestSyncIfModified_CacheClearing(t *testing.T) {
	t.Parallel()
	srv := &Server{}

	tests := []struct {
		name        string
		setupPath   func(t *testing.T) string
		wantCleared bool
	}{
		{
			name: "NotExist_ClearsCache",
			setupPath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			wantCleared: true,
		},
		{
			name: "NotDir_ClearsCache",
			setupPath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "file")
				if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(filePath, "child")
			},
			wantCleared: true,
		},
		{
			name:        "PermissionDenied_KeepsCache",
			setupPath:   makeUnreadableDir,
			wantCleared: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.setupPath(t)
			sourcePath := path
			var lastMtime int64 = 12345

			srv.syncIfModified("s1", &sourcePath, &lastMtime)

			if tt.wantCleared {
				assertCacheCleared(t, sourcePath, lastMtime)
			} else {
				assertCachePreserved(t, sourcePath, path, lastMtime, 12345)
			}
		})
	}
}

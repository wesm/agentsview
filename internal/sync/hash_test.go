package sync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTempFile(t *testing.T, content []byte) string {
	t.Helper()
	cleanName := strings.ReplaceAll(t.Name(), "/", "_")
	path := filepath.Join(t.TempDir(), cleanName+".txt")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	return path
}

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "hello world",
			input: "hello world\n",
			want:  helloWorldHash,
		},
		{
			name:  "empty input",
			input: "",
			want:  emptyInputHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeHash(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("ComputeHash: %v", err)
			}
			if got != tt.want {
				t.Errorf("ComputeHash() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeFileHash(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    string
		wantErr bool
	}{
		{
			name: "hello world",
			setup: func(t *testing.T) string {
				return createTempFile(t, []byte("hello world\n"))
			},
			want: helloWorldHash,
		},
		{
			name: "empty file",
			setup: func(t *testing.T) string {
				return createTempFile(t, []byte(""))
			},
			want: emptyInputHash,
		},
		{
			name: "missing file",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.txt")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			got, err := ComputeFileHash(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ComputeFileHash() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				requirePathError(t, err)
				return
			}
			if got != tt.want {
				t.Errorf("ComputeFileHash() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeHash_ReaderError(t *testing.T) {
	errInjected := errors.New("injected error")
	reader := &failingReader{err: errInjected}
	_, err := ComputeHash(reader)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errInjected) {
		t.Errorf("expected error wrapping 'injected error', got %v", err)
	}
}

func TestComputeFileHash_ReadError(t *testing.T) {
	// Use a directory to simulate a read error after open
	dir := t.TempDir()
	_, err := ComputeFileHash(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// On most systems, reading a directory fails.
	requirePathError(t, err)
}

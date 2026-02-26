package sync

import (
	"errors"
	"io/fs"
	"testing"
)

const (
	helloWorldHash = "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	emptyInputHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// requirePathError asserts that err is non-nil and wraps *fs.PathError.
func requirePathError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected *fs.PathError, got %T: %v", err, err)
	}
}

// failingReader is an io.Reader that always returns an error.
type failingReader struct {
	err error
}

func (f failingReader) Read(p []byte) (n int, err error) {
	return 0, f.err
}

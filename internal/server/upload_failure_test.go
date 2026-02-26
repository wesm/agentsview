package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadSession_SaveFailure(t *testing.T) {
	te := setup(t)

	// Create a file where the project directory should be
	// to force os.MkdirAll to fail
	projectName := "failproj"
	projectPath := filepath.Join(te.dataDir, "uploads", projectName)
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o755); err != nil {
		t.Fatalf("creating uploads dir: %v", err)
	}
	if err := os.WriteFile(projectPath, nil, 0o644); err != nil {
		t.Fatalf("creating conflict file: %v", err)
	}

	w := te.upload(t, "test.jsonl", "{}", "project="+projectName)
	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorResponse(t, w, "failed to save upload")
}

func TestUploadSession_DBFailure(t *testing.T) {
	te := setup(t)

	// Close DB to force saveSessionToDB to fail
	te.db.Close()

	content := `{"type":"user","timestamp":"2024-01-01T10:00:00Z","message":{"content":"Hello"}}`
	w := te.upload(t, "test.jsonl", content, "project=myproj")
	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorResponse(t, w, "failed to save session to database")
}

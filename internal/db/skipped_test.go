package db_test

import (
	"testing"

	"github.com/wesm/agentsview/internal/dbtest"
)

func TestSkippedFiles_RoundTrip(t *testing.T) {
	d := dbtest.OpenTestDB(t)

	// Initially empty.
	loaded, err := d.LoadSkippedFiles()
	if err != nil {
		t.Fatalf("LoadSkippedFiles: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty, got %d entries", len(loaded))
	}

	// Persist some entries.
	entries := map[string]int64{
		"/a/b/c.jsonl": 100,
		"/d/e/f.jsonl": 200,
		"/g/h/i.jsonl": 300,
	}
	if err := d.ReplaceSkippedFiles(entries); err != nil {
		t.Fatalf("ReplaceSkippedFiles: %v", err)
	}

	// Load them back.
	loaded, err = d.LoadSkippedFiles()
	if err != nil {
		t.Fatalf("LoadSkippedFiles: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("got %d entries, want 3", len(loaded))
	}
	for path, mtime := range entries {
		if loaded[path] != mtime {
			t.Errorf(
				"loaded[%q] = %d, want %d",
				path, loaded[path], mtime,
			)
		}
	}
}

func TestSkippedFiles_ReplaceOverwrites(t *testing.T) {
	d := dbtest.OpenTestDB(t)

	first := map[string]int64{
		"/a.jsonl": 100,
		"/b.jsonl": 200,
	}
	if err := d.ReplaceSkippedFiles(first); err != nil {
		t.Fatalf("ReplaceSkippedFiles: %v", err)
	}

	// Replace with different entries.
	second := map[string]int64{
		"/c.jsonl": 300,
	}
	if err := d.ReplaceSkippedFiles(second); err != nil {
		t.Fatalf("ReplaceSkippedFiles: %v", err)
	}

	loaded, err := d.LoadSkippedFiles()
	if err != nil {
		t.Fatalf("LoadSkippedFiles: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("got %d entries, want 1", len(loaded))
	}
	if loaded["/c.jsonl"] != 300 {
		t.Errorf(
			"loaded[/c.jsonl] = %d, want 300",
			loaded["/c.jsonl"],
		)
	}
}

func TestSkippedFiles_DeleteSingle(t *testing.T) {
	d := dbtest.OpenTestDB(t)

	entries := map[string]int64{
		"/a.jsonl": 100,
		"/b.jsonl": 200,
	}
	if err := d.ReplaceSkippedFiles(entries); err != nil {
		t.Fatalf("ReplaceSkippedFiles: %v", err)
	}

	if err := d.DeleteSkippedFile("/a.jsonl"); err != nil {
		t.Fatalf("DeleteSkippedFile: %v", err)
	}

	loaded, err := d.LoadSkippedFiles()
	if err != nil {
		t.Fatalf("LoadSkippedFiles: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("got %d entries, want 1", len(loaded))
	}
	if _, ok := loaded["/a.jsonl"]; ok {
		t.Error("/a.jsonl should have been deleted")
	}
	if loaded["/b.jsonl"] != 200 {
		t.Errorf(
			"loaded[/b.jsonl] = %d, want 200",
			loaded["/b.jsonl"],
		)
	}
}

func TestSkippedFiles_DeleteNonexistent(t *testing.T) {
	d := dbtest.OpenTestDB(t)

	// Should not error.
	if err := d.DeleteSkippedFile("/nope"); err != nil {
		t.Fatalf("DeleteSkippedFile: %v", err)
	}
}

func TestSkippedFiles_EmptyReplace(t *testing.T) {
	d := dbtest.OpenTestDB(t)

	entries := map[string]int64{"/a.jsonl": 100}
	if err := d.ReplaceSkippedFiles(entries); err != nil {
		t.Fatalf("ReplaceSkippedFiles: %v", err)
	}

	// Replace with empty map clears the table.
	if err := d.ReplaceSkippedFiles(
		map[string]int64{},
	); err != nil {
		t.Fatalf("ReplaceSkippedFiles empty: %v", err)
	}

	loaded, err := d.LoadSkippedFiles()
	if err != nil {
		t.Fatalf("LoadSkippedFiles: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("got %d entries, want 0", len(loaded))
	}
}

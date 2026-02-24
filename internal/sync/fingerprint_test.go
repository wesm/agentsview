package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSqliteFingerprint_NoWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := os.WriteFile(
		dbPath, []byte("data"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	fp1 := sqliteFingerprint(dbPath)
	fp2 := sqliteFingerprint(dbPath)
	if fp1 != fp2 {
		t.Errorf(
			"same file should produce same fingerprint: %d != %d",
			fp1, fp2,
		)
	}
}

func TestSqliteFingerprint_WALChange(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	walPath := dbPath + "-wal"

	if err := os.WriteFile(
		dbPath, []byte("data"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Fingerprint without WAL file.
	fpNoWAL := sqliteFingerprint(dbPath)

	// Create a WAL file — fingerprint must change.
	if err := os.WriteFile(
		walPath, []byte("wal-data"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	fpWithWAL := sqliteFingerprint(dbPath)
	if fpNoWAL == fpWithWAL {
		t.Error(
			"fingerprint should change when WAL appears",
		)
	}

	// Modify WAL contents — fingerprint must change again.
	// Ensure mtime differs by sleeping briefly.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(
		walPath, []byte("wal-data-updated"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	fpUpdatedWAL := sqliteFingerprint(dbPath)
	if fpWithWAL == fpUpdatedWAL {
		t.Error(
			"fingerprint should change when WAL is modified",
		)
	}
}

func TestSqliteFingerprint_MissingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nonexistent.db")

	fp := sqliteFingerprint(dbPath)
	if fp != 0 {
		t.Errorf("expected 0 for missing DB, got %d", fp)
	}
}

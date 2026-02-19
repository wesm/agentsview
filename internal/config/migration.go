package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// MigrateFromLegacy copies data from ~/.agent-session-viewer
// to the current data directory if it doesn't exist yet.
// Call this once during startup, before opening the database.
func MigrateFromLegacy(dataDir string) {
	if _, err := os.Stat(dataDir); err == nil {
		return // new dir already exists
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	legacyDir := filepath.Join(home, ".agent-session-viewer")
	if _, err := os.Stat(legacyDir); err != nil {
		return // no legacy dir either
	}
	log.Printf(
		"Migrating data from %s to %s", legacyDir, dataDir,
	)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Printf(
			"migration: cannot create %s: %v", dataDir, err,
		)
		return
	}

	// Only copy sessions-go.db (from a previous Go run).
	// The Python-era sessions.db has an incompatible schema
	// and the Go backend rebuilds from JSONL source files.
	src := filepath.Join(legacyDir, "sessions-go.db")
	if _, err := os.Stat(src); err == nil {
		dst := filepath.Join(dataDir, "sessions.db")
		if err := copyFile(src, dst, 0o644); err != nil {
			log.Printf("migration: copying DB: %v", err)
		} else {
			log.Printf("migration: copied sessions-go.db")
		}
	}

	// Copy config.json if present
	src = filepath.Join(legacyDir, "config.json")
	if _, err := os.Stat(src); err == nil {
		dst := filepath.Join(dataDir, "config.json")
		if err := copyFile(src, dst, 0o600); err != nil {
			log.Printf("migration: copying config: %v", err)
		}
	}
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(
		dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode,
	)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying: %w", err)
	}
	return out.Close()
}

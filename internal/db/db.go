package db

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

const schemaFTS = `
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    content='messages',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
        VALUES('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
        VALUES('delete', old.id, old.content);
    INSERT INTO messages_fts(rowid, content) VALUES (new.id, new.content);
END;
`

// DB manages a write connection and a read-only pool.
type DB struct {
	writer *sql.DB
	reader *sql.DB
	mu     sync.Mutex // serializes writes

	cursorMu     sync.RWMutex
	cursorSecret []byte
}

// SetCursorSecret updates the secret key used for cursor signing.
func (db *DB) SetCursorSecret(secret []byte) {
	db.cursorMu.Lock()
	defer db.cursorMu.Unlock()
	db.cursorSecret = append([]byte(nil), secret...)
}

// makeDSN builds a SQLite connection string with shared pragmas.
func makeDSN(path string, readOnly bool) string {
	params := url.Values{}
	params.Set("_journal_mode", "WAL")
	params.Set("_busy_timeout", "5000")
	params.Set("_foreign_keys", "ON")
	params.Set("_mmap_size", "268435456")
	params.Set("_cache_size", "-64000")
	if readOnly {
		params.Set("mode", "ro")
	} else {
		params.Set("_synchronous", "NORMAL")
	}
	return path + "?" + params.Encode()
}

// Open creates or opens a SQLite database at the given path.
// It configures WAL mode, mmap, and returns a DB with separate
// writer and reader connections.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	writer, err := sql.Open("sqlite3", makeDSN(path, false))
	if err != nil {
		return nil, fmt.Errorf("opening writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite3", makeDSN(path, true))
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("opening reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	db := &DB{writer: writer, reader: reader}

	// Initialize with a random secret (will be overridden by SetCursorSecret)
	db.cursorSecret = make([]byte, 32)
	if _, err := rand.Read(db.cursorSecret); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("generating cursor secret: %w", err)
	}

	if err := db.init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}
	return db, nil
}

// HasFTS checks if Full Text Search is available.
func (db *DB) HasFTS() bool {
	// We need to actually try to access the table, because it might exist
	// in sqlite_master but fail to load if the fts5 module is missing
	// in the current runtime.
	_, err := db.reader.Exec("SELECT 1 FROM messages_fts LIMIT 1")
	return err == nil
}

// ensureColumn adds a column if it doesn't already exist.
func (db *DB) ensureColumn(
	table, column, definition string,
) error {
	var count int
	err := db.writer.QueryRow(
		fmt.Sprintf(
			"SELECT count(*) FROM pragma_table_info('%s')"+
				" WHERE name='%s'",
			table, column,
		),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf(
			"checking column %s.%s: %w", table, column, err,
		)
	}
	if count > 0 {
		return nil
	}
	_, err = db.writer.Exec(fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN %s %s",
		table, column, definition,
	))
	if err == nil {
		return nil
	}
	// If ALTER TABLE failed, check if the column exists now.
	// This handles race conditions where another process added it
	// concurrently, without relying on brittle error string matching.
	var check int
	if checkErr := db.writer.QueryRow(
		fmt.Sprintf(
			"SELECT count(*) FROM pragma_table_info('%s')"+
				" WHERE name='%s'",
			table, column,
		),
	).Scan(&check); checkErr == nil && check > 0 {
		return nil
	}
	return err
}

func (db *DB) init() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, err := db.writer.Exec(schemaSQL); err != nil {
		return err
	}

	// Check if FTS table exists before trying to create it
	var ftsCount int
	if err := db.writer.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='messages_fts'",
	).Scan(&ftsCount); err != nil {
		return fmt.Errorf("checking fts table: %w", err)
	}
	hadFTS := ftsCount > 0

	// Attempt to initialize FTS. Failure is non-fatal (might be missing module).
	if _, err := db.writer.Exec(schemaFTS); err != nil {
		// Only ignore "no such module" error
		if !strings.Contains(err.Error(), "no such module") {
			return fmt.Errorf("initializing FTS: %w", err)
		}
	} else if !hadFTS {
		// Schema init succeeded and we didn't have FTS before.
		// We need to populate the index for existing messages.
		if _, err := db.writer.Exec("INSERT INTO messages_fts(messages_fts) VALUES('rebuild')"); err != nil {
			return fmt.Errorf("backfilling FTS: %w", err)
		}
	}

	// Migration: add file_hash column for existing databases.
	if err := db.ensureColumn(
		"sessions", "file_hash", "TEXT",
	); err != nil {
		return fmt.Errorf("adding file_hash column: %w", err)
	}

	// Migration: add parent_session_id column for session
	// continuity chaining.
	if err := db.ensureColumn(
		"sessions", "parent_session_id", "TEXT",
	); err != nil {
		return fmt.Errorf(
			"adding parent_session_id column: %w", err,
		)
	}
	if _, err := db.writer.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sessions_parent
		 ON sessions(parent_session_id)
		 WHERE parent_session_id IS NOT NULL`,
	); err != nil {
		return fmt.Errorf(
			"creating parent_session_id index: %w", err,
		)
	}

	return nil
}

// Close closes both writer and reader connections.
func (db *DB) Close() error {
	return errors.Join(db.writer.Close(), db.reader.Close())
}

// Update executes fn within a write lock and transaction.
// The transaction is committed if fn returns nil, rolled back
// otherwise.
func (db *DB) Update(fn func(tx *sql.Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.writer.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Reader returns the read-only connection pool.
func (db *DB) Reader() *sql.DB {
	return db.reader
}

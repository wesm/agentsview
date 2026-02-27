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
	"sync/atomic"

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
// The reader and writer fields use atomic.Pointer so that
// concurrent HTTP handler goroutines can safely read while
// Reopen/CloseConnections swap the underlying *sql.DB.
type DB struct {
	path    string
	writer  atomic.Pointer[sql.DB]
	reader  atomic.Pointer[sql.DB]
	mu      sync.Mutex // serializes writes
	retired []*sql.DB  // old pools kept open for in-flight reads

	cursorMu     sync.RWMutex
	cursorSecret []byte
}

// getReader returns the current read-only connection pool.
func (db *DB) getReader() *sql.DB { return db.reader.Load() }

// getWriter returns the current write connection.
func (db *DB) getWriter() *sql.DB { return db.writer.Load() }

// Path returns the file path of the database.
func (db *DB) Path() string {
	return db.path
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
//
// If an existing database has an outdated schema, it is deleted
// and recreated from scratch. Session data is re-synced from
// the source files on the next sync cycle.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	rebuild, err := needsRebuild(path)
	if err != nil {
		return nil, fmt.Errorf("checking schema: %w", err)
	}
	if rebuild {
		if err := dropDatabase(path); err != nil {
			return nil, fmt.Errorf(
				"rebuilding database: %w", err,
			)
		}
	}

	return openAndInit(path)
}

// needsRebuild checks whether an existing database has an
// outdated schema that requires a full rebuild. Returns an
// error on probe failures so callers can surface them.
func needsRebuild(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil // no existing DB
		}
		return false, fmt.Errorf(
			"checking database file: %w", err,
		)
	}
	conn, err := sql.Open("sqlite3", makeDSN(path, true))
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	defer conn.Close()

	var count int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('sessions')
		 WHERE name = 'parent_session_id'`,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	if count == 0 {
		return true, nil
	}

	var insightCount int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('insights')
		 WHERE name = 'date_from'`,
	).Scan(&insightCount)
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	if insightCount == 0 {
		return true, nil
	}

	var toolCount int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('tool_calls')
		 WHERE name = 'tool_use_id'`,
	).Scan(&toolCount)
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	if toolCount == 0 {
		return true, nil
	}

	var umcCount int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('sessions')
		 WHERE name = 'user_message_count'`,
	).Scan(&umcCount)
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	if umcCount == 0 {
		return true, nil
	}

	var relTypeCount int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('sessions')
		 WHERE name = 'relationship_type'`,
	).Scan(&relTypeCount)
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	if relTypeCount == 0 {
		return true, nil
	}

	var subagentColCount int
	err = conn.QueryRow(
		`SELECT count(*) FROM pragma_table_info('tool_calls')
		 WHERE name = 'subagent_session_id'`,
	).Scan(&subagentColCount)
	if err != nil {
		return false, fmt.Errorf(
			"probing schema: %w", err,
		)
	}
	return subagentColCount == 0, nil
}

func dropDatabase(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil &&
			!os.IsNotExist(err) {
			return fmt.Errorf(
				"removing %s: %w", path+suffix, err,
			)
		}
	}
	return nil
}

func openAndInit(path string) (*DB, error) {
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

	db := &DB{path: path}
	db.writer.Store(writer)
	db.reader.Store(reader)

	db.cursorSecret = make([]byte, 32)
	if _, err := rand.Read(db.cursorSecret); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf(
			"generating cursor secret: %w", err,
		)
	}

	if err := db.init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}
	return db, nil
}

// DropFTS drops the FTS table and its triggers. This makes
// bulk message delete+reinsert fast by avoiding per-row FTS
// index updates. Call RebuildFTS after to restore search.
func (db *DB) DropFTS() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	stmts := []string{
		"DROP TRIGGER IF EXISTS messages_ai",
		"DROP TRIGGER IF EXISTS messages_ad",
		"DROP TRIGGER IF EXISTS messages_au",
		"DROP TABLE IF EXISTS messages_fts",
	}
	w := db.getWriter()
	for _, s := range stmts {
		if _, err := w.Exec(s); err != nil {
			return fmt.Errorf("drop fts (%s): %w", s, err)
		}
	}
	return nil
}

// RebuildFTS recreates the FTS table, triggers, and
// repopulates the index from the messages table.
func (db *DB) RebuildFTS() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	w := db.getWriter()
	if _, err := w.Exec(schemaFTS); err != nil {
		return fmt.Errorf("recreate fts: %w", err)
	}
	_, err := w.Exec(
		"INSERT INTO messages_fts(messages_fts)" +
			" VALUES('rebuild')",
	)
	if err != nil {
		return fmt.Errorf("rebuild fts index: %w", err)
	}
	return nil
}

// HasFTS checks if Full Text Search is available.
func (db *DB) HasFTS() bool {
	// We need to actually try to access the table, because it might exist
	// in sqlite_master but fail to load if the fts5 module is missing
	// in the current runtime.
	_, err := db.getReader().Exec(
		"SELECT 1 FROM messages_fts LIMIT 1",
	)
	return err == nil
}

func (db *DB) init() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	w := db.getWriter()
	if _, err := w.Exec(schemaSQL); err != nil {
		return err
	}

	// Check if FTS table exists before trying to create it
	var ftsCount int
	if err := w.QueryRow(
		"SELECT count(*) FROM sqlite_master" +
			" WHERE type='table' AND name='messages_fts'",
	).Scan(&ftsCount); err != nil {
		return fmt.Errorf("checking fts table: %w", err)
	}
	hadFTS := ftsCount > 0

	// Attempt to initialize FTS. Failure is non-fatal
	// (might be missing module).
	if _, err := w.Exec(schemaFTS); err != nil {
		if !strings.Contains(
			err.Error(), "no such module",
		) {
			return fmt.Errorf("initializing FTS: %w", err)
		}
	} else if !hadFTS {
		// Schema init succeeded and we didn't have FTS
		// before. Populate the index for existing messages.
		if _, err := w.Exec(
			"INSERT INTO messages_fts(messages_fts)" +
				" VALUES('rebuild')",
		); err != nil {
			return fmt.Errorf("backfilling FTS: %w", err)
		}
	}

	return nil
}

// Close closes both writer and reader connections, plus any
// retired pools left over from previous Reopen calls.
func (db *DB) Close() error {
	errs := []error{
		db.getWriter().Close(),
		db.getReader().Close(),
	}
	for _, p := range db.retired {
		errs = append(errs, p.Close())
	}
	db.retired = nil
	return errors.Join(errs...)
}

// CloseConnections closes both connections without reopening,
// releasing file locks so the database file can be renamed.
// Callers must call Reopen afterwards to restore service.
func (db *DB) CloseConnections() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return errors.Join(
		db.getWriter().Close(),
		db.getReader().Close(),
	)
}

// Reopen closes and reopens both connections to the same
// path. Used after an atomic file swap to pick up the new
// database contents. Preserves cursorSecret.
func (db *DB) Reopen() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.reopenLocked()
}

// reopenLocked performs the reopen while db.mu is already
// held. New connections are opened before closing old ones
// so the struct never points at closed handles on failure.
func (db *DB) reopenLocked() error {
	writer, err := sql.Open(
		"sqlite3", makeDSN(db.path, false),
	)
	if err != nil {
		return fmt.Errorf("reopening writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open(
		"sqlite3", makeDSN(db.path, true),
	)
	if err != nil {
		writer.Close()
		return fmt.Errorf("reopening reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	oldWriter := db.writer.Swap(writer)
	oldReader := db.reader.Swap(reader)

	// Retire old pools instead of closing them immediately.
	// Concurrent readers that loaded the old pointer before
	// the swap may still have in-flight queries. The retired
	// pools are closed when the DB itself is closed.
	db.retired = append(db.retired, oldWriter, oldReader)
	return nil
}

// Update executes fn within a write lock and transaction.
// The transaction is committed if fn returns nil, rolled back
// otherwise.
func (db *DB) Update(fn func(tx *sql.Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.getWriter().Begin()
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
	return db.getReader()
}

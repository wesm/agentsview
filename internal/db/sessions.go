package db

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidCursor is returned when a cursor cannot be decoded or verified.
var ErrInvalidCursor = errors.New("invalid cursor")

// sessionBaseCols is the column list for standard session queries
// (list, get). Keep in sync with scanSessionRow.
const sessionBaseCols = `id, project, machine, agent,
	first_message, started_at, ended_at,
	message_count, parent_session_id, created_at`

// sessionPruneCols extends sessionBaseCols with file metadata
// needed by FindPruneCandidates.
const sessionPruneCols = `id, project, machine, agent,
	first_message, started_at, ended_at,
	message_count, parent_session_id,
	file_path, file_size, created_at`

// sessionFullCols includes all columns for a complete session record.
const sessionFullCols = `id, project, machine, agent,
	first_message, started_at, ended_at,
	message_count, parent_session_id,
	file_path, file_size, file_mtime,
	file_hash, created_at`

const (
	// DefaultSessionLimit is the default number of sessions returned.
	DefaultSessionLimit = 200
	// MaxSessionLimit is the maximum number of sessions returned.
	MaxSessionLimit = 500
)

// rowScanner is satisfied by both *sql.Row and *sql.Rows,
// allowing a single scan helper for both.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanSessionRow scans sessionBaseCols into a Session.
func scanSessionRow(rs rowScanner) (Session, error) {
	var s Session
	err := rs.Scan(
		&s.ID, &s.Project, &s.Machine, &s.Agent,
		&s.FirstMessage, &s.StartedAt, &s.EndedAt,
		&s.MessageCount, &s.ParentSessionID, &s.CreatedAt,
	)
	return s, err
}

// Session represents a row in the sessions table.
type Session struct {
	ID              string  `json:"id"`
	Project         string  `json:"project"`
	Machine         string  `json:"machine"`
	Agent           string  `json:"agent"`
	FirstMessage    *string `json:"first_message"`
	StartedAt       *string `json:"started_at"`
	EndedAt         *string `json:"ended_at"`
	MessageCount    int     `json:"message_count"`
	ParentSessionID *string `json:"parent_session_id,omitempty"`
	FilePath        *string `json:"file_path,omitempty"`
	FileSize        *int64  `json:"file_size,omitempty"`
	FileMtime       *int64  `json:"file_mtime,omitempty"`
	FileHash        *string `json:"file_hash,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

// SessionCursor is the opaque pagination token.
type SessionCursor struct {
	EndedAt string `json:"e"`
	ID      string `json:"i"`
	Total   int    `json:"t,omitempty"`
}

// EncodeCursor returns a base64-encoded cursor string.
func (db *DB) EncodeCursor(endedAt, id string, total ...int) string {
	t := 0
	if len(total) > 0 {
		t = total[0]
	}
	c := SessionCursor{EndedAt: endedAt, ID: id, Total: t}
	data, _ := json.Marshal(c)

	db.cursorMu.RLock()
	mac := hmac.New(sha256.New, db.cursorSecret)
	db.cursorMu.RUnlock()

	mac.Write(data)
	sig := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(data) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// DecodeCursor parses a base64-encoded cursor string.
func (db *DB) DecodeCursor(s string) (SessionCursor, error) {
	parts := strings.Split(s, ".")
	if len(parts) == 1 {
		// Legacy cursor (unsigned). Trust nothing about the Total.
		data, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			return SessionCursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
		}
		var c SessionCursor
		if err := json.Unmarshal(data, &c); err != nil {
			return SessionCursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
		}
		c.Total = 0 // Force re-computation
		return c, nil
	} else if len(parts) != 2 {
		return SessionCursor{}, fmt.Errorf("%w: invalid format", ErrInvalidCursor)
	}

	payload := parts[0]
	sigStr := parts[1]

	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return SessionCursor{}, fmt.Errorf("%w: invalid payload: %v", ErrInvalidCursor, err)
	}

	sig, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return SessionCursor{}, fmt.Errorf("%w: invalid signature encoding: %v", ErrInvalidCursor, err)
	}

	db.cursorMu.RLock()
	mac := hmac.New(sha256.New, db.cursorSecret)
	db.cursorMu.RUnlock()

	mac.Write(data)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return SessionCursor{}, fmt.Errorf("%w: signature mismatch", ErrInvalidCursor)
	}

	var c SessionCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return SessionCursor{}, fmt.Errorf("%w: invalid json: %v", ErrInvalidCursor, err)
	}
	return c, nil
}

// SessionFilter specifies how to query sessions.
type SessionFilter struct {
	Project     string
	Machine     string
	Agent       string
	Date        string // exact date YYYY-MM-DD
	DateFrom    string // range start (inclusive)
	DateTo      string // range end (inclusive)
	MinMessages int    // message_count >= N (0 = no filter)
	MaxMessages int    // message_count <= N (0 = no filter)
	Cursor      string // opaque cursor from previous page
	Limit       int
}

// SessionPage is a page of session results.
type SessionPage struct {
	Sessions   []Session `json:"sessions"`
	NextCursor string    `json:"next_cursor,omitempty"`
	Total      int       `json:"total"`
}

// buildSessionFilter returns a WHERE clause and args for the
// non-cursor predicates in SessionFilter.
func buildSessionFilter(f SessionFilter) (string, []any) {
	preds := []string{"message_count > 0"}
	var args []any

	if f.Project != "" {
		preds = append(preds, "project = ?")
		args = append(args, f.Project)
	}
	if f.Machine != "" {
		preds = append(preds, "machine = ?")
		args = append(args, f.Machine)
	}
	if f.Agent != "" {
		preds = append(preds, "agent = ?")
		args = append(args, f.Agent)
	}
	if f.Date != "" {
		preds = append(preds,
			"date(COALESCE(started_at, created_at)) = ?")
		args = append(args, f.Date)
	}
	if f.DateFrom != "" {
		preds = append(preds,
			"date(COALESCE(started_at, created_at)) >= ?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		preds = append(preds,
			"date(COALESCE(started_at, created_at)) <= ?")
		args = append(args, f.DateTo)
	}
	if f.MinMessages > 0 {
		preds = append(preds, "message_count >= ?")
		args = append(args, f.MinMessages)
	}
	if f.MaxMessages > 0 {
		preds = append(preds, "message_count <= ?")
		args = append(args, f.MaxMessages)
	}

	return strings.Join(preds, " AND "), args
}

// ListSessions returns a cursor-paginated list of sessions.
func (db *DB) ListSessions(
	ctx context.Context, f SessionFilter,
) (SessionPage, error) {
	if f.Limit <= 0 || f.Limit > MaxSessionLimit {
		f.Limit = DefaultSessionLimit
	}

	where, args := buildSessionFilter(f)

	var total int
	var cur SessionCursor
	if f.Cursor != "" {
		var err error
		cur, err = db.DecodeCursor(f.Cursor)
		if err != nil {
			return SessionPage{}, err
		}
		total = cur.Total
	}
	// Total count applies filters but not cursor. To avoid
	// re-counting on every pagination request, newer cursors carry
	// the first-page total and we reuse it here.
	if total <= 0 {
		countQuery := "SELECT COUNT(*) FROM sessions WHERE " + where
		if err := db.reader.QueryRowContext(
			ctx, countQuery, args...,
		).Scan(&total); err != nil {
			return SessionPage{},
				fmt.Errorf("counting sessions: %w", err)
		}
	}

	// Paginated results
	cursorArgs := append([]any{}, args...)
	cursorWhere := where
	if f.Cursor != "" {
		cursorWhere += ` AND (
				COALESCE(ended_at, started_at, created_at), id
			) < (?, ?)`
		cursorArgs = append(cursorArgs, cur.EndedAt, cur.ID)
	}

	query := "SELECT " + sessionBaseCols +
		" FROM sessions WHERE " + cursorWhere + `
		ORDER BY COALESCE(
			ended_at, started_at, created_at
		) DESC, id DESC
		LIMIT ?`
	cursorArgs = append(cursorArgs, f.Limit+1)

	rows, err := db.reader.QueryContext(ctx, query, cursorArgs...)
	if err != nil {
		return SessionPage{},
			fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	sessions, err := scanSessionRows(rows)
	if err != nil {
		return SessionPage{}, err
	}

	page := SessionPage{Sessions: sessions, Total: total}
	if len(sessions) > f.Limit {
		page.Sessions = sessions[:f.Limit]
		last := page.Sessions[f.Limit-1]
		ea := ""
		if last.EndedAt != nil {
			ea = *last.EndedAt
		} else if last.StartedAt != nil {
			ea = *last.StartedAt
		} else {
			ea = last.CreatedAt
		}
		page.NextCursor = db.EncodeCursor(ea, last.ID, total)
	}

	return page, nil
}

// GetSession returns a single session by ID.
func (db *DB) GetSession(
	ctx context.Context, id string,
) (*Session, error) {
	row := db.reader.QueryRowContext(
		ctx,
		"SELECT "+sessionBaseCols+" FROM sessions WHERE id = ?",
		id,
	)

	s, err := scanSessionRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session %s: %w", id, err)
	}
	return &s, nil
}

// GetSessionFull returns a single session by ID with all file metadata.
func (db *DB) GetSessionFull(
	ctx context.Context, id string,
) (*Session, error) {
	row := db.reader.QueryRowContext(
		ctx,
		"SELECT "+sessionFullCols+" FROM sessions WHERE id = ?",
		id,
	)

	var s Session
	err := row.Scan(
		&s.ID, &s.Project, &s.Machine, &s.Agent,
		&s.FirstMessage, &s.StartedAt, &s.EndedAt,
		&s.MessageCount, &s.ParentSessionID,
		&s.FilePath, &s.FileSize,
		&s.FileMtime, &s.FileHash, &s.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session full %s: %w", id, err)
	}
	return &s, nil
}

// UpsertSession inserts or updates a session.
func (db *DB) UpsertSession(s Session) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.writer.Exec(`
		INSERT INTO sessions (
			id, project, machine, agent, first_message,
			started_at, ended_at, message_count,
			parent_session_id,
			file_path, file_size, file_mtime, file_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project = excluded.project,
			machine = excluded.machine,
			agent = excluded.agent,
			first_message = excluded.first_message,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at,
			message_count = excluded.message_count,
			parent_session_id = excluded.parent_session_id,
			file_path = excluded.file_path,
			file_size = excluded.file_size,
			file_mtime = excluded.file_mtime,
			file_hash = excluded.file_hash`,
		s.ID, s.Project, s.Machine, s.Agent, s.FirstMessage,
		s.StartedAt, s.EndedAt, s.MessageCount,
		s.ParentSessionID,
		s.FilePath, s.FileSize, s.FileMtime, s.FileHash)
	if err != nil {
		return fmt.Errorf("upserting session %s: %w", s.ID, err)
	}
	return nil
}

// GetSessionFileInfo returns file_size and file_mtime for a
// session. Used for fast skip checks during sync.
func (db *DB) GetSessionFileInfo(
	id string,
) (size int64, mtime int64, ok bool) {
	var s, m sql.NullInt64
	err := db.reader.QueryRow(
		"SELECT file_size, file_mtime FROM sessions WHERE id = ?",
		id,
	).Scan(&s, &m)
	if err != nil {
		return 0, 0, false
	}
	return s.Int64, m.Int64, true
}

// GetFileInfoByPath returns file_size and file_mtime for a
// session identified by file_path. Used for codex/gemini files
// where the session ID requires parsing.
func (db *DB) GetFileInfoByPath(
	path string,
) (size int64, mtime int64, ok bool) {
	var s, m sql.NullInt64
	err := db.reader.QueryRow(
		"SELECT file_size, file_mtime FROM sessions"+
			" WHERE file_path = ?"+
			" ORDER BY file_mtime DESC LIMIT 1",
		path,
	).Scan(&s, &m)
	if err != nil {
		return 0, 0, false
	}
	return s.Int64, m.Int64, true
}

// DeleteSession removes a session and its messages (cascading).
func (db *DB) DeleteSession(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.writer.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// GetProjects returns project names with session counts.
func (db *DB) GetProjects(
	ctx context.Context,
) ([]ProjectInfo, error) {
	rows, err := db.reader.QueryContext(ctx, `
		SELECT project, COUNT(*) as session_count
		FROM sessions
		WHERE message_count > 0
		GROUP BY project
		ORDER BY project`)
	if err != nil {
		return nil, fmt.Errorf("querying projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectInfo
	for rows.Next() {
		var p ProjectInfo
		if err := rows.Scan(&p.Name, &p.SessionCount); err != nil {
			return nil, fmt.Errorf("scanning project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ProjectInfo holds a project name and its session count.
type ProjectInfo struct {
	Name         string `json:"name"`
	SessionCount int    `json:"session_count"`
}

// GetMachines returns distinct machine names.
func (db *DB) GetMachines(
	ctx context.Context,
) ([]string, error) {
	rows, err := db.reader.QueryContext(ctx,
		"SELECT DISTINCT machine FROM sessions ORDER BY machine",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		machines = append(machines, m)
	}
	return machines, rows.Err()
}

// scanSessionRows iterates rows and scans each using
// scanSessionRow.
func scanSessionRows(rows *sql.Rows) ([]Session, error) {
	var sessions []Session
	for rows.Next() {
		s, err := scanSessionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// PruneFilter defines criteria for finding sessions to prune.
// Filters combine with AND. At least one must be set.
type PruneFilter struct {
	Project      string // substring match (LIKE '%x%')
	MaxMessages  *int   // user messages <= N (nil = no filter)
	Before       string // ended_at < date (YYYY-MM-DD)
	FirstMessage string // first_message LIKE 'prefix%'
}

// HasFilters reports whether at least one filter is set.
func (f PruneFilter) HasFilters() bool {
	return f.Project != "" ||
		f.MaxMessages != nil ||
		f.Before != "" ||
		f.FirstMessage != ""
}

// escapeLike escapes SQL LIKE wildcard characters so user
// input is matched literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`, `%`, `\%`, `_`, `\_`,
	)
	return r.Replace(s)
}

// FindPruneCandidates returns sessions matching all filter
// criteria. Returns full Session rows including file metadata.
func (db *DB) FindPruneCandidates(
	f PruneFilter,
) ([]Session, error) {
	if !f.HasFilters() {
		return nil, fmt.Errorf("at least one filter is required")
	}

	where := "1=1"
	args := []any{}

	if f.Project != "" {
		where += ` AND project LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(f.Project)+"%")
	}
	if f.MaxMessages != nil {
		where += ` AND (SELECT COUNT(*) FROM messages
			WHERE messages.session_id = sessions.id
			AND messages.role = 'user') <= ?`
		args = append(args, *f.MaxMessages)
	}
	if f.Before != "" {
		where += " AND COALESCE(ended_at, started_at, created_at) < ?"
		args = append(args, f.Before)
	}
	if f.FirstMessage != "" {
		where += ` AND first_message LIKE ? ESCAPE '\'`
		args = append(args, escapeLike(f.FirstMessage)+"%")
	}

	// Exclude sessions that are parents of other sessions.
	where += ` AND NOT EXISTS (
		SELECT 1 FROM sessions AS child
		WHERE child.parent_session_id = sessions.id)`

	query := "SELECT " + sessionPruneCols +
		" FROM sessions WHERE " + where + `
		ORDER BY COALESCE(
			ended_at, started_at, created_at
		) DESC`

	rows, err := db.reader.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("finding prune candidates: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		err := rows.Scan(
			&s.ID, &s.Project, &s.Machine, &s.Agent,
			&s.FirstMessage, &s.StartedAt, &s.EndedAt,
			&s.MessageCount, &s.ParentSessionID,
			&s.FilePath, &s.FileSize, &s.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning prune candidate: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// DeleteSessions removes multiple sessions by ID in a single
// transaction. Batches DELETEs in groups of 500 to stay under
// SQLite variable limits. Returns count of deleted rows.
func (db *DB) DeleteSessions(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.writer.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	total := 0
	const batchSize = 500
	for i := 0; i < len(ids); i += batchSize {
		end := min(i+batchSize, len(ids))
		batch := ids[i:end]

		args := make([]any, len(batch))
		for j, id := range batch {
			args[j] = id
		}
		placeholders := strings.Repeat(",?", len(batch))[1:]

		res, err := tx.Exec(
			"DELETE FROM sessions WHERE id IN ("+placeholders+")",
			args...,
		)
		if err != nil {
			return 0, fmt.Errorf("deleting batch: %w", err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}
	return total, nil
}

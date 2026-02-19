package db

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	selectMessageCols = `id, session_id, ordinal, role, content,
		timestamp, has_thinking, has_tool_use, content_length`

	insertMessageCols = `session_id, ordinal, role, content,
		timestamp, has_thinking, has_tool_use, content_length`

	// DefaultMessageLimit is the default number of messages returned.
	DefaultMessageLimit = 100
	// MaxMessageLimit is the maximum number of messages returned.
	MaxMessageLimit = 1000
)

// ToolCall represents a single tool invocation stored in
// the tool_calls table.
type ToolCall struct {
	MessageID int64
	SessionID string
	ToolName  string
	Category  string
}

// Message represents a row in the messages table.
type Message struct {
	ID            int64      `json:"id"`
	SessionID     string     `json:"session_id"`
	Ordinal       int        `json:"ordinal"`
	Role          string     `json:"role"`
	Content       string     `json:"content"`
	Timestamp     string     `json:"timestamp"`
	HasThinking   bool       `json:"has_thinking"`
	HasToolUse    bool       `json:"has_tool_use"`
	ContentLength int        `json:"content_length"`
	ToolCalls     []ToolCall `json:"-"` // transient, not a DB column
}

// MinimapEntry is a lightweight message summary for minimap rendering.
type MinimapEntry struct {
	Ordinal       int    `json:"ordinal"`
	Role          string `json:"role"`
	ContentLength int    `json:"content_length"`
	HasThinking   bool   `json:"has_thinking"`
	HasToolUse    bool   `json:"has_tool_use"`
}

// GetMessages returns paginated messages for a session.
// from: starting ordinal (inclusive)
// limit: max messages to return
// asc: true for ascending ordinal order, false for descending
func (db *DB) GetMessages(
	ctx context.Context,
	sessionID string, from, limit int, asc bool,
) ([]Message, error) {
	if limit <= 0 || limit > MaxMessageLimit {
		limit = DefaultMessageLimit
	}

	dir := "ASC"
	op := ">="
	if !asc {
		dir = "DESC"
		op = "<="
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM messages
		WHERE session_id = ? AND ordinal %s ?
		ORDER BY ordinal %s
		LIMIT ?`, selectMessageCols, op, dir)

	rows, err := db.reader.QueryContext(
		ctx, query, sessionID, from, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetAllMessages returns all messages for a session ordered by ordinal.
func (db *DB) GetAllMessages(
	ctx context.Context, sessionID string,
) ([]Message, error) {
	rows, err := db.reader.QueryContext(ctx, fmt.Sprintf(`
		SELECT %s
		FROM messages
		WHERE session_id = ?
		ORDER BY ordinal ASC`, selectMessageCols), sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying all messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetMinimap returns lightweight metadata for all messages in a session.
func (db *DB) GetMinimap(
	ctx context.Context, sessionID string,
) ([]MinimapEntry, error) {
	return db.GetMinimapFrom(ctx, sessionID, 0)
}

// GetMinimapFrom returns lightweight metadata for messages in a
// session starting at ordinal >= from.
func (db *DB) GetMinimapFrom(
	ctx context.Context, sessionID string, from int,
) ([]MinimapEntry, error) {
	rows, err := db.reader.QueryContext(ctx, `
		SELECT ordinal, role, content_length, has_thinking, has_tool_use
		FROM messages
		WHERE session_id = ? AND ordinal >= ?
		ORDER BY ordinal ASC`, sessionID, from)
	if err != nil {
		return nil, fmt.Errorf("querying minimap: %w", err)
	}
	defer rows.Close()

	var entries []MinimapEntry
	for rows.Next() {
		var e MinimapEntry
		if err := rows.Scan(
			&e.Ordinal, &e.Role, &e.ContentLength,
			&e.HasThinking, &e.HasToolUse,
		); err != nil {
			return nil, fmt.Errorf("scanning minimap entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// SampleMinimap downsamples entries to at most max points while
// preserving ordering and both endpoints.
func SampleMinimap(
	entries []MinimapEntry, max int,
) []MinimapEntry {
	if max <= 0 || len(entries) <= max {
		return entries
	}
	if max == 1 {
		return []MinimapEntry{entries[0]}
	}

	sampled := make([]MinimapEntry, 0, max)
	lastIdx := len(entries) - 1
	den := max - 1
	for i := range max {
		idx := (i * lastIdx) / den
		sampled = append(sampled, entries[idx])
	}
	return sampled
}

// insertMessagesTx batch-inserts messages within an existing
// transaction. Returns a map of ordinal to message ID for
// resolving tool call foreign keys. The caller must hold db.mu.
func (db *DB) insertMessagesTx(
	tx *sql.Tx, msgs []Message,
) (map[int]int64, error) {
	stmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO messages (%s)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, insertMessageCols))
	if err != nil {
		return nil, fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	ordinalToID := make(map[int]int64, len(msgs))
	for _, m := range msgs {
		res, err := stmt.Exec(
			m.SessionID, m.Ordinal, m.Role, m.Content,
			m.Timestamp, m.HasThinking, m.HasToolUse,
			m.ContentLength,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"inserting message ord=%d: %w", m.Ordinal, err,
			)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf(
				"last insert id ord=%d: %w", m.Ordinal, err,
			)
		}
		ordinalToID[m.Ordinal] = id
	}
	return ordinalToID, nil
}

// insertToolCallsTx batch-inserts tool calls within an
// existing transaction.
func insertToolCallsTx(
	tx *sql.Tx, calls []ToolCall,
) error {
	if len(calls) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO tool_calls
			(message_id, session_id, tool_name, category)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing tool_calls insert: %w", err)
	}
	defer stmt.Close()

	for _, tc := range calls {
		if _, err := stmt.Exec(
			tc.MessageID, tc.SessionID,
			tc.ToolName, tc.Category,
		); err != nil {
			return fmt.Errorf(
				"inserting tool_call %q: %w", tc.ToolName, err,
			)
		}
	}
	return nil
}

// InsertMessages batch-inserts messages for a session.
func (db *DB) InsertMessages(msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.writer.Begin()
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ordinalToID, err := db.insertMessagesTx(tx, msgs)
	if err != nil {
		return err
	}

	toolCalls := resolveToolCalls(msgs, ordinalToID)
	if err := insertToolCallsTx(tx, toolCalls); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteSessionMessages removes all messages for a session.
func (db *DB) DeleteSessionMessages(sessionID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.writer.Exec(
		"DELETE FROM messages WHERE session_id = ?", sessionID,
	)
	return err
}

// ReplaceSessionMessages deletes existing and inserts new messages
// in a single transaction.
func (db *DB) ReplaceSessionMessages(
	sessionID string, msgs []Message,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.writer.Begin()
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		"DELETE FROM tool_calls WHERE session_id = ?",
		sessionID,
	); err != nil {
		return fmt.Errorf("deleting old tool_calls: %w", err)
	}

	if _, err := tx.Exec(
		"DELETE FROM messages WHERE session_id = ?", sessionID,
	); err != nil {
		return fmt.Errorf("deleting old messages: %w", err)
	}

	if len(msgs) > 0 {
		ordinalToID, err := db.insertMessagesTx(tx, msgs)
		if err != nil {
			return err
		}
		toolCalls := resolveToolCalls(msgs, ordinalToID)
		if err := insertToolCallsTx(tx, toolCalls); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var msgs []Message
	for rows.Next() {
		var m Message
		err := rows.Scan(
			&m.ID, &m.SessionID, &m.Ordinal, &m.Role,
			&m.Content, &m.Timestamp,
			&m.HasThinking, &m.HasToolUse, &m.ContentLength,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// MessageCount returns the number of messages for a session.
func (db *DB) MessageCount(sessionID string) (int, error) {
	var count int
	err := db.reader.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE session_id = ?",
		sessionID,
	).Scan(&count)
	return count, err
}

// GetMessageByOrdinal returns a single message by session ID and ordinal.
func (db *DB) GetMessageByOrdinal(
	sessionID string, ordinal int,
) (*Message, error) {
	row := db.reader.QueryRow(fmt.Sprintf(`
		SELECT %s
		FROM messages
		WHERE session_id = ? AND ordinal = ?`, selectMessageCols),
		sessionID, ordinal)

	var m Message
	err := row.Scan(
		&m.ID, &m.SessionID, &m.Ordinal, &m.Role,
		&m.Content, &m.Timestamp,
		&m.HasThinking, &m.HasToolUse, &m.ContentLength,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// resolveToolCalls builds ToolCall rows from messages using
// the ordinal-to-ID map from insertMessagesTx.
func resolveToolCalls(
	msgs []Message, ordinalToID map[int]int64,
) []ToolCall {
	var calls []ToolCall
	for _, m := range msgs {
		msgID := ordinalToID[m.Ordinal]
		for _, tc := range m.ToolCalls {
			calls = append(calls, ToolCall{
				MessageID: msgID,
				SessionID: m.SessionID,
				ToolName:  tc.ToolName,
				Category:  tc.Category,
			})
		}
	}
	return calls
}

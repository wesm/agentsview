package db

import (
	"context"
	"fmt"
	"log"
	"time"
)

// CopyOrphanedDataFrom copies sessions (and their messages
// and tool_calls) that exist in the source database but not
// in this database. This preserves archived sessions whose
// source files no longer exist on disk.
//
// Orphaned sessions are identified by ID-diff: any session
// present in the source but absent from the target after a
// full file sync. This correctly captures sessions whose
// source files were deleted, moved, or otherwise lost —
// exactly the set that would be dropped by a naive DB swap.
//
// The source database must not have active connections (call
// CloseConnections on its DB handle first). Uses ATTACH
// DATABASE on a pinned connection for atomicity.
func (d *DB) CopyOrphanedDataFrom(
	sourcePath string,
) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	ctx := context.Background()
	conn, err := d.getWriter().Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf(
			"acquiring connection: %w", err,
		)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(
		ctx, "ATTACH DATABASE ? AS old_db", sourcePath,
	); err != nil {
		return 0, fmt.Errorf(
			"attaching source db: %w", err,
		)
	}
	defer func() {
		_, _ = conn.ExecContext(
			ctx, "DETACH DATABASE old_db",
		)
	}()

	// Snapshot orphaned session IDs before any inserts
	// change main.sessions.
	if _, err := conn.ExecContext(ctx, `
		CREATE TEMP TABLE _orphaned_ids AS
		SELECT id FROM old_db.sessions
		WHERE id NOT IN (SELECT id FROM main.sessions)`,
	); err != nil {
		return 0, fmt.Errorf(
			"identifying orphaned sessions: %w", err,
		)
	}
	defer func() {
		_, _ = conn.ExecContext(
			ctx,
			"DROP TABLE IF EXISTS _orphaned_ids",
		)
	}()

	var count int
	if err := conn.QueryRowContext(ctx,
		"SELECT count(*) FROM _orphaned_ids",
	).Scan(&count); err != nil {
		return 0, fmt.Errorf(
			"counting orphaned sessions: %w", err,
		)
	}
	if count == 0 {
		return 0, nil
	}

	t := time.Now()

	// Use a transaction so all three inserts are atomic.
	// Partial orphan copies would leave dangling sessions
	// without messages or tool_calls.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin orphan tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Copy session rows.
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO sessions
			(id, project, machine, agent, first_message,
			 started_at, ended_at, message_count,
			 user_message_count, file_path, file_size,
			 file_mtime, file_hash, parent_session_id,
			 relationship_type, created_at)
		SELECT
			id, project, machine, agent, first_message,
			started_at, ended_at, message_count,
			user_message_count, file_path, file_size,
			file_mtime, file_hash, parent_session_id,
			relationship_type, created_at
		FROM old_db.sessions
		WHERE id IN (SELECT id FROM _orphaned_ids)`,
	); err != nil {
		return 0, fmt.Errorf(
			"copying orphaned sessions: %w", err,
		)
	}

	// Copy messages. Omit id to let auto-increment assign
	// new IDs (old IDs may collide with freshly synced
	// messages).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages
			(session_id, ordinal, role, content,
			 timestamp, has_thinking, has_tool_use,
			 content_length)
		SELECT
			session_id, ordinal, role, content,
			timestamp, has_thinking, has_tool_use,
			content_length
		FROM old_db.messages
		WHERE session_id IN (
			SELECT id FROM _orphaned_ids
		)`,
	); err != nil {
		return 0, fmt.Errorf(
			"copying orphaned messages: %w", err,
		)
	}

	// Copy tool_calls. Map old message_id to new
	// message_id via the (session_id, ordinal) natural key.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tool_calls
			(message_id, session_id, tool_name, category,
			 tool_use_id, input_json, skill_name,
			 result_content_length, subagent_session_id)
		SELECT
			new_m.id, otc.session_id, otc.tool_name,
			otc.category, otc.tool_use_id, otc.input_json,
			otc.skill_name, otc.result_content_length,
			otc.subagent_session_id
		FROM old_db.tool_calls otc
		JOIN old_db.messages old_m
			ON old_m.id = otc.message_id
		JOIN main.messages new_m
			ON new_m.session_id = old_m.session_id
			AND new_m.ordinal = old_m.ordinal
		WHERE otc.session_id IN (
			SELECT id FROM _orphaned_ids
		)`,
	); err != nil {
		return 0, fmt.Errorf(
			"copying orphaned tool_calls: %w", err,
		)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf(
			"committing orphaned data: %w", err,
		)
	}

	log.Printf(
		"resync: copied %d orphaned sessions in %s",
		count, time.Since(t).Round(time.Millisecond),
	)

	return count, nil
}

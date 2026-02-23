package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Summary represents a row in the summaries table.
type Summary struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Date      string  `json:"date"`
	Project   *string `json:"project"`
	Agent     string  `json:"agent"`
	Model     *string `json:"model"`
	Prompt    *string `json:"prompt"`
	Content   string  `json:"content"`
	CreatedAt string  `json:"created_at"`
}

// SummaryFilter specifies how to query summaries.
type SummaryFilter struct {
	Type       string // "daily_activity" or "agent_analysis"
	Date       string // YYYY-MM-DD
	Project    string // "" = no filter
	GlobalOnly bool   // true = project IS NULL only
}

const summaryBaseCols = `id, type, date, project, agent,
	model, prompt, content, created_at`

func scanSummaryRow(rs rowScanner) (Summary, error) {
	var s Summary
	err := rs.Scan(
		&s.ID, &s.Type, &s.Date, &s.Project, &s.Agent,
		&s.Model, &s.Prompt, &s.Content, &s.CreatedAt,
	)
	return s, err
}

func buildSummaryFilter(
	f SummaryFilter,
) (string, []any) {
	var preds []string
	var args []any

	if f.Type != "" {
		preds = append(preds, "type = ?")
		args = append(args, f.Type)
	}
	if f.Date != "" {
		preds = append(preds, "date = ?")
		args = append(args, f.Date)
	}
	if f.GlobalOnly {
		preds = append(preds, "project IS NULL")
	} else if f.Project != "" {
		preds = append(preds, "project = ?")
		args = append(args, f.Project)
	}

	if len(preds) == 0 {
		return "1=1", nil
	}
	return strings.Join(preds, " AND "), args
}

// InsertSummary inserts a summary and returns its ID.
func (db *DB) InsertSummary(s Summary) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	res, err := db.writer.Exec(`
		INSERT INTO summaries (
			type, date, project, agent,
			model, prompt, content
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.Type, s.Date, s.Project, s.Agent,
		s.Model, s.Prompt, s.Content,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting summary: %w", err)
	}
	return res.LastInsertId()
}

// ListSummaries returns summaries matching the filter,
// ordered by created_at DESC.
func (db *DB) ListSummaries(
	ctx context.Context, f SummaryFilter,
) ([]Summary, error) {
	where, args := buildSummaryFilter(f)
	query := "SELECT " + summaryBaseCols +
		" FROM summaries WHERE " + where +
		" ORDER BY created_at DESC, id DESC"

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying summaries: %w", err)
	}
	defer rows.Close()

	var summaries []Summary
	for rows.Next() {
		s, err := scanSummaryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// GetSummary returns a single summary by ID.
// Returns nil, nil if not found.
func (db *DB) GetSummary(
	ctx context.Context, id int64,
) (*Summary, error) {
	row := db.reader.QueryRowContext(
		ctx,
		"SELECT "+summaryBaseCols+
			" FROM summaries WHERE id = ?",
		id,
	)
	s, err := scanSummaryRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf(
			"getting summary %d: %w", id, err,
		)
	}
	return &s, nil
}

// DeleteSummary removes a summary by ID.
func (db *DB) DeleteSummary(id int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.writer.Exec(
		"DELETE FROM summaries WHERE id = ?", id,
	)
	return err
}

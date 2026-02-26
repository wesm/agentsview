package db

import (
	"context"
	"fmt"
)

// Stats holds database statistics from the trigger-maintained table.
type Stats struct {
	SessionCount int `json:"session_count"`
	MessageCount int `json:"message_count"`
	ProjectCount int `json:"project_count"`
	MachineCount int `json:"machine_count"`
}

// GetStats returns O(1) statistics from the stats table,
// supplemented with project/machine counts from indexes.
func (db *DB) GetStats(ctx context.Context) (Stats, error) {
	const query = `
		SELECT
			(SELECT value FROM stats WHERE key = 'session_count'),
			(SELECT value FROM stats WHERE key = 'message_count'),
			(SELECT COUNT(DISTINCT project) FROM sessions),
			(SELECT COUNT(DISTINCT machine) FROM sessions)`

	var s Stats
	err := db.getReader().QueryRowContext(ctx, query).Scan(
		&s.SessionCount,
		&s.MessageCount,
		&s.ProjectCount,
		&s.MachineCount,
	)
	if err != nil {
		return Stats{}, fmt.Errorf("fetching stats: %w", err)
	}
	return s, nil
}

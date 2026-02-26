package db

import (
	"context"
	"fmt"
)

// Stats holds database-wide statistics.
type Stats struct {
	SessionCount int `json:"session_count"`
	MessageCount int `json:"message_count"`
	ProjectCount int `json:"project_count"`
	MachineCount int `json:"machine_count"`
}

// rootSessionFilter is the WHERE clause shared by session list
// and stats to exclude sub-agent, fork, and empty sessions.
const rootSessionFilter = `message_count > 0
	AND relationship_type NOT IN ('subagent', 'fork')`

// GetStats returns database statistics, counting only root
// sessions with messages (matching the session list filter).
func (db *DB) GetStats(ctx context.Context) (Stats, error) {
	const query = `
		SELECT
			(SELECT COUNT(*) FROM sessions
			 WHERE ` + rootSessionFilter + `),
			(SELECT value FROM stats
			 WHERE key = 'message_count'),
			(SELECT COUNT(DISTINCT project) FROM sessions
			 WHERE ` + rootSessionFilter + `),
			(SELECT COUNT(DISTINCT machine) FROM sessions
			 WHERE ` + rootSessionFilter + `)`

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

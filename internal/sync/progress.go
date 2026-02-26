package sync

// Phase describes the current sync phase.
type Phase string

const (
	PhaseIdle        Phase = "idle"
	PhaseDiscovering Phase = "discovering"
	PhaseSyncing     Phase = "syncing"
	PhaseDone        Phase = "done"
)

// Progress reports sync progress to listeners.
type Progress struct {
	Phase           Phase  `json:"phase"`
	CurrentProject  string `json:"current_project,omitempty"`
	ProjectsTotal   int    `json:"projects_total"`
	ProjectsDone    int    `json:"projects_done"`
	SessionsTotal   int    `json:"sessions_total"`
	SessionsDone    int    `json:"sessions_done"`
	MessagesIndexed int    `json:"messages_indexed"`
}

// SyncResult describes the outcome of syncing a single session.
type SyncResult struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Skipped   bool   `json:"skipped"`
	Messages  int    `json:"messages"`
}

// SyncStats summarizes a full sync run.
type SyncStats struct {
	TotalSessions int      `json:"total_sessions"`
	Synced        int      `json:"synced"`
	Skipped       int      `json:"skipped"`
	Failed        int      `json:"failed"`
	Warnings      []string `json:"warnings,omitempty"`
}

// RecordSkip increments the skipped session counter.
func (s *SyncStats) RecordSkip() {
	s.Skipped++
}

// RecordSynced adds n to the synced session counter.
func (s *SyncStats) RecordSynced(n int) {
	s.Synced += n
}

// RecordFailed increments the hard-failure counter.
func (s *SyncStats) RecordFailed() {
	s.Failed++
}

// Percent returns the sync progress as a percentage (0–100).
func (p Progress) Percent() float64 {
	if p.SessionsTotal == 0 {
		return 0
	}
	return float64(p.SessionsDone) /
		float64(p.SessionsTotal) * 100
}

// ProgressFunc is called with progress updates during sync.
type ProgressFunc func(Progress)

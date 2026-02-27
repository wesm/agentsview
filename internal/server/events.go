package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	syncpkg "github.com/wesm/agentsview/internal/sync"
)

const (
	// pollInterval is how often the file watcher checks for changes.
	pollInterval = 1500 * time.Millisecond
	// heartbeatInterval is how often a keepalive is sent to the
	// client. Expressed as a multiple of pollInterval (~30s).
	heartbeatTicks = 20
)

// sessionMonitor polls a session's source file for changes and
// syncs on modification. It sends on the returned channel after
// each successful sync. The channel is closed when ctx is done.
func (s *Server) sessionMonitor(
	ctx context.Context, sessionID string,
) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		defer close(ch)

		sourcePath := s.engine.FindSourceFile(sessionID)
		var lastMtime int64
		if sourcePath != "" {
			if info, err := os.Stat(sourcePath); err == nil {
				lastMtime = info.ModTime().UnixNano()
			}
		}

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				changed := s.checkAndSync(
					sessionID, &sourcePath, &lastMtime,
				)
				if changed {
					select {
					case ch <- struct{}{}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return ch
}

// checkAndSync polls the source file and syncs if modified.
// It updates sourcePath and lastMtime through the pointers.
// Returns true if the session was synced successfully.
func (s *Server) checkAndSync(
	sessionID string,
	sourcePath *string,
	lastMtime *int64,
) bool {
	if *sourcePath != "" {
		return s.syncIfModified(
			sessionID, sourcePath, lastMtime,
		)
	}
	// Try to re-resolve a previously unknown path
	*sourcePath = s.engine.FindSourceFile(sessionID)
	if *sourcePath == "" {
		return false
	}
	info, err := os.Stat(*sourcePath)
	if err != nil {
		return false
	}
	*lastMtime = info.ModTime().UnixNano()
	if err := s.engine.SyncSingleSession(sessionID); err != nil {
		log.Printf("watch sync error: %v", err)
		return false
	}
	return true
}

// syncIfModified checks whether the file at path has been
// modified since lastMtime. If so, it syncs and updates
// lastMtime. On not-exist or invalid-path stat errors, clear cache;
// transient errors preserve cache so checkAndSync can re-try.
// Returns true on a successful sync.
func (s *Server) syncIfModified(
	sessionID string,
	sourcePath *string,
	lastMtime *int64,
) bool {
	info, err := os.Stat(*sourcePath)
	if err != nil {
		// Clear cache if the file is gone or the path is invalid (e.g. ENOTDIR).
		// Transient errors (like permission denied) preserve the cache.
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			*sourcePath = ""
			*lastMtime = 0
		}
		return false
	}
	mtime := info.ModTime().UnixNano()
	if mtime <= *lastMtime {
		return false
	}
	*lastMtime = mtime
	if err := s.engine.SyncSingleSession(sessionID); err != nil {
		log.Printf("watch sync error: %v", err)
		return false
	}
	return true
}

func (s *Server) handleWatchSession(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")

	stream, err := NewSSEStream(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"streaming not supported")
		return
	}

	updates := s.sessionMonitor(r.Context(), sessionID)
	heartbeat := time.NewTicker(
		pollInterval * heartbeatTicks,
	)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-updates:
			if !ok {
				return
			}
			stream.Send("session_updated", sessionID)
		case <-heartbeat.C:
			stream.Send("heartbeat",
				time.Now().Format(time.RFC3339))
		}
	}
}

func (s *Server) handleTriggerSync(
	w http.ResponseWriter, r *http.Request,
) {
	stream, err := NewSSEStream(w)
	if err != nil {
		// Non-streaming fallback
		stats := s.engine.SyncAll(nil)
		writeJSON(w, http.StatusOK, stats)
		return
	}

	stats := s.engine.SyncAll(func(p syncpkg.Progress) {
		stream.SendJSON("progress", p)
	})
	stream.SendJSON("done", stats)
}

func (s *Server) handleTriggerResync(
	w http.ResponseWriter, r *http.Request,
) {
	stream, err := NewSSEStream(w)
	if err != nil {
		stats := s.engine.ResyncAll(nil)
		writeJSON(w, http.StatusOK, stats)
		return
	}

	stats := s.engine.ResyncAll(func(p syncpkg.Progress) {
		stream.SendJSON("progress", p)
	})
	stream.SendJSON("done", stats)
}

func (s *Server) handleSyncStatus(
	w http.ResponseWriter, r *http.Request,
) {
	lastSync := s.engine.LastSync()
	stats := s.engine.LastSyncStats()

	var lastSyncStr string
	if !lastSync.IsZero() {
		lastSyncStr = lastSync.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"last_sync": lastSyncStr,
		"stats":     stats,
	})
}

func (s *Server) handleGetStats(
	w http.ResponseWriter, r *http.Request,
) {
	stats, err := s.db.GetStats(r.Context())
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleListProjects(
	w http.ResponseWriter, r *http.Request,
) {
	projects, err := s.db.GetProjects(r.Context())
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"projects": projects,
	})
}

func (s *Server) handleListMachines(
	w http.ResponseWriter, r *http.Request,
) {
	machines, err := s.db.GetMachines(r.Context())
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"machines": machines,
	})
}

func (s *Server) handleListAgents(
	w http.ResponseWriter, r *http.Request,
) {
	agents, err := s.db.GetAgents(r.Context())
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
	})
}

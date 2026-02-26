package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

func TestHandlers_Internal_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	s := testServer(t, 30*time.Second)

	// Seed a session just in case handlers check for existence before context.
	started := "2025-01-15T10:00:00Z"
	sess := db.Session{
		ID:        "s1",
		Project:   "test-proj",
		StartedAt: &started,
	}
	if err := s.db.UpsertSession(sess); err != nil {
		t.Fatalf("seeding session: %v", err)
	}

	tests := []struct {
		name        string
		handler     func(http.ResponseWriter, *http.Request)
		requiresFTS bool
	}{
		{"ListSessions", s.handleListSessions, false},
		{"GetSession", s.handleGetSession, false},
		{"GetMessages", s.handleGetMessages, false},
		{"GetMinimap", s.handleGetMinimap, false},
		{"GetStats", s.handleGetStats, false},
		{"ListProjects", s.handleListProjects, false},
		{"ListMachines", s.handleListMachines, false},
		{"Search", s.handleSearch, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.requiresFTS && !s.db.HasFTS() {
				t.Skip("skipping test: no FTS support")
			}
			ctx, cancel := expiredCtx(t)
			defer cancel()

			req := httptest.NewRequest(http.MethodGet, "/?q=test", nil)
			req.SetPathValue("id", "s1")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			// Call handler directly, bypassing middleware.
			// handleContextError writes 504 for deadline exceeded.
			tt.handler(w, req)

			assertRecorderStatus(t, w, http.StatusGatewayTimeout)
			assertContentType(t, w, "application/json")
		})
	}
}

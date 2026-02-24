package server_test

import (
	"net/http/httptest"
	"testing"
)

func TestMiddleware_Timeout(t *testing.T) {
	te := setup(t)
	// Seed some data so handlers don't fail with 404 before checking context
	te.seedSession(t, "s1", "my-app", 10)
	te.seedMessages(t, "s1", 10)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"ListSessions", "GET", "/api/v1/sessions"},
		{"GetSession", "GET", "/api/v1/sessions/s1"},
		{"GetMessages", "GET", "/api/v1/sessions/s1/messages"},
		{"GetMinimap", "GET", "/api/v1/sessions/s1/minimap"},
		{"GetStats", "GET", "/api/v1/stats"},
		{"ListProjects", "GET", "/api/v1/projects"},
		{"ListMachines", "GET", "/api/v1/machines"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := expiredContext(t)
			defer cancel()

			req := httptest.NewRequest(tt.method, tt.path, nil).WithContext(ctx)
			w := httptest.NewRecorder()
			te.handler.ServeHTTP(w, req)

			assertTimeoutRace(t, w)
		})
	}
}

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestContentTypeWrapper verifies that Content-Type is only set if missing
// when the status code matches the trigger status.
func TestContentTypeWrapper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		handler         http.HandlerFunc
		triggerStatus   int
		wantStatus      int
		wantContentType string
		wantBody        string
	}{
		{
			name: "SetsContentTypeOnTriggerStatusMissingHeader",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error":"timeout"}`))
			},
			triggerStatus:   http.StatusServiceUnavailable,
			wantStatus:      http.StatusServiceUnavailable,
			wantContentType: "application/json",
			wantBody:        `{"error":"timeout"}`,
		},
		{
			name: "RespectsExistingContentTypeOnTriggerStatus",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("timeout error"))
			},
			triggerStatus:   http.StatusServiceUnavailable,
			wantStatus:      http.StatusServiceUnavailable,
			wantContentType: "text/plain",
			wantBody:        "timeout error",
		},
		{
			name: "IgnoresNonTriggerStatus",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			},
			triggerStatus:   http.StatusServiceUnavailable,
			wantStatus:      http.StatusOK,
			wantContentType: "", // Not set by wrapper
			wantBody:        "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			wrapper := &contentTypeWrapper{
				ResponseWriter: w,
				contentType:    "application/json",
				triggerStatus:  tt.triggerStatus,
			}

			req := httptest.NewRequest("GET", "/", nil)
			tt.handler(wrapper, req)

			assertRecorderStatus(t, w, tt.wantStatus)

			resp := w.Result()
			defer resp.Body.Close()

			gotCT := resp.Header.Get("Content-Type")
			if tt.wantContentType == "" && gotCT == "application/json" {
				t.Errorf("Content-Type = %q; wrapper should not force application/json on non-trigger status", gotCT)
			} else if tt.wantContentType != "" && gotCT != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", gotCT, tt.wantContentType)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", string(body), tt.wantBody)
			}
		})
	}
}

// TestWithTimeoutTriggersOnSlowHandler verifies that withTimeout produces a
// 503 JSON timeout response when the handler exceeds the configured duration.
func TestWithTimeoutTriggersOnSlowHandler(t *testing.T) {
	t.Parallel()

	srv := testServer(t, 10*time.Millisecond)

	// Handler that blocks well past the timeout.
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		// If we reach here after context cancel, TimeoutHandler
		// already wrote the 503.
	})

	handler := srv.withTimeout(slow)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	assertTimeoutResponse(t, resp)
}

// TestRoutesTimeoutWiring verifies that API routes are wrapped with timeout
// middleware (positive assertion) and that export/SPA routes are NOT wrapped
// (negative assertion).
func TestRoutesTimeoutWiring(t *testing.T) {
	t.Parallel()

	srv := testServer(
		t, 10*time.Millisecond,
		withHandlerDelay(100*time.Millisecond),
	)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	tests := []struct {
		name        string
		path        string
		wantTimeout bool
		wantStatus  int // Only checked if wantTimeout is false
	}{
		{"Wrapped_ListSessions", "/api/v1/sessions", true, 0},
		{"Wrapped_GetStats", "/api/v1/stats", true, 0},
		{"Unwrapped_ExportSession", "/api/v1/sessions/invalid-id/export", false, http.StatusNotFound},
		{"Unwrapped_SPA", "/", false, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := ts.Client().Get(ts.URL + tt.path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if tt.wantTimeout {
				if !isTimeoutResponse(t, resp) {
					t.Errorf("%s: expected timeout 503, got %d", tt.path, resp.StatusCode)
				}
			} else {
				if isTimeoutResponse(t, resp) {
					t.Errorf("%s: unexpected timeout for unwrapped route", tt.path)
				}
				if resp.StatusCode != tt.wantStatus {
					t.Errorf("%s: status = %d, want %d", tt.path, resp.StatusCode, tt.wantStatus)
				}
			}
		})
	}
}

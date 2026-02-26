package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWithTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		timeout        time.Duration
		handler        http.HandlerFunc
		wantStatus     int
		wantBody       string
		wantHeaderKey  string
		wantHeaderVal  string
		assertResponse func(t *testing.T, resp *http.Response)
	}{
		{
			name:    "timeout",
			timeout: 10 * time.Millisecond,
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(50 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("too slow"))
			},
			assertResponse: assertTimeoutResponse,
		},
		{
			name:    "success",
			timeout: 100 * time.Millisecond,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Custom", "value")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"status":"ok"}`))
			},
			wantStatus:    http.StatusCreated,
			wantBody:      `{"status":"ok"}`,
			wantHeaderKey: "X-Custom",
			wantHeaderVal: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServerMinimal(t, tt.timeout)
			wrapped := s.withTimeout(tt.handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if tt.assertResponse != nil {
				tt.assertResponse(t, resp)
				return
			}

			assertRecorderStatus(t, w, tt.wantStatus)

			if tt.wantHeaderKey != "" {
				if val := resp.Header.Get(tt.wantHeaderKey); val != tt.wantHeaderVal {
					t.Errorf("expected header %s=%q, got %q", tt.wantHeaderKey, tt.wantHeaderVal, val)
				}
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			if string(body) != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, string(body))
			}
		})
	}
}

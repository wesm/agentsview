package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/config"
)

func TestWithTimeout_Timeout(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		WriteTimeout: 10 * time.Millisecond,
	}
	s := &Server{
		cfg: cfg,
	}

	slowHandler := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("too slow"))
	}

	wrapped := s.withTimeout(slowHandler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assertTimeoutResponse(t, resp)
}

func TestWithTimeout_Success(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		WriteTimeout: 100 * time.Millisecond,
	}
	s := &Server{
		cfg: cfg,
	}

	fastHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"ok"}`))
	}

	wrapped := s.withTimeout(fastHandler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	assertRecorderStatus(t, w, http.StatusCreated)

	resp := w.Result()
	defer resp.Body.Close()

	if val := resp.Header.Get("X-Custom"); val != "value" {
		t.Errorf("expected X-Custom header 'value', got %q", val)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status":"ok"}` {
		t.Errorf("expected body '{\"status\":\"ok\"}', got %q", string(body))
	}
}

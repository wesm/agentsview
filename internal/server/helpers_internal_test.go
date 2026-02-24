package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/config"
	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/sync"
)

// testServer creates a Server for internal tests with the given
// write timeout. It registers cleanup of the database via
// t.Cleanup.
func testServer(
	t *testing.T, writeTimeout time.Duration,
) *Server {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.Config{
		Host:         "127.0.0.1",
		Port:         0,
		DataDir:      dir,
		DBPath:       dbPath,
		WriteTimeout: writeTimeout,
	}
	engine := sync.NewEngine(database, dir, "", "", "test")
	return New(cfg, database, engine)
}

// assertTimeoutResponse checks that the response is a 503 with
// a JSON body containing "request timed out" and the correct
// Content-Type header.
func assertTimeoutResponse(
	t *testing.T, resp *http.Response,
) {
	t.Helper()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf(
			"status = %d, want %d",
			resp.StatusCode, http.StatusServiceUnavailable,
		)
	}
	body, _ := io.ReadAll(resp.Body)
	var je jsonError
	if err := json.Unmarshal(body, &je); err != nil {
		t.Fatalf(
			"body is not valid JSON: %v (body=%q)",
			err, string(body),
		)
	}
	if je.Error != "request timed out" {
		t.Errorf(
			"error = %q, want %q",
			je.Error, "request timed out",
		)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf(
			"Content-Type = %q, want %q",
			ct, "application/json",
		)
	}
}

// isTimeoutResponse returns true when the response is a 503
// JSON timeout. Use this for negative assertions where a route
// should NOT produce a timeout.
func isTimeoutResponse(
	t *testing.T, resp *http.Response,
) bool {
	t.Helper()
	if resp.StatusCode != http.StatusServiceUnavailable {
		return false
	}
	body, _ := io.ReadAll(resp.Body)
	var je jsonError
	if json.Unmarshal(body, &je) != nil {
		return false
	}
	return je.Error == "request timed out"
}

// newTestContext returns a recorder and request for lightweight
// handler tests. Pass an empty query for no query string.
func newTestContext(
	t *testing.T, query string,
) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	target := "/test"
	if query != "" {
		target += "?" + query
	}
	return httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, target, nil)
}

// assertRecorderStatus checks that the recorder has the
// expected HTTP status code.
func assertRecorderStatus(
	t *testing.T, w *httptest.ResponseRecorder, code int,
) {
	t.Helper()
	if w.Code != code {
		t.Fatalf(
			"expected status %d, got %d: %s",
			code, w.Code, w.Body.String(),
		)
	}
}

// assertContentType checks that the recorder has the expected
// Content-Type header.
func assertContentType(
	t *testing.T, w *httptest.ResponseRecorder, expected string,
) {
	t.Helper()
	if got := w.Header().Get("Content-Type"); got != expected {
		t.Errorf(
			"Content-Type = %q, want %q", got, expected,
		)
	}
}

// newTestServerMinimal creates a lightweight Server with only the
// config set (no database, engine, or temp dirs). Use this for
// handler-level tests that only need withTimeout or similar
// config-driven wrappers.
func newTestServerMinimal(
	t *testing.T, timeout time.Duration,
) *Server {
	t.Helper()
	return &Server{
		cfg: config.Config{WriteTimeout: timeout},
	}
}

// expiredCtx returns a context with a deadline in the past.
func expiredCtx(
	t *testing.T,
) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithDeadline(
		context.Background(), time.Now().Add(-1*time.Hour),
	)
}

// assertContainsAll checks that got contains every string
// in wants.
func assertContainsAll(
	t *testing.T, got string, wants []string,
) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf(
				"expected to contain %q, got:\n%s",
				want, got,
			)
		}
	}
}

// assertContainsNone checks that got does not contain any
// string in bads.
func assertContainsNone(
	t *testing.T, got string, bads []string,
) {
	t.Helper()
	for _, bad := range bads {
		if strings.Contains(got, bad) {
			t.Errorf(
				"expected NOT to contain %q, got:\n%s",
				bad, got,
			)
		}
	}
}

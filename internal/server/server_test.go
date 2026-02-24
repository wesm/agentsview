package server_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	stdlibsync "sync"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/config"
	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/dbtest"
	"github.com/wesm/agentsview/internal/server"
	"github.com/wesm/agentsview/internal/sync"
	"github.com/wesm/agentsview/internal/testjsonl"
)

// Timestamp constants for test data.
const (
	tsZero    = "2024-01-01T00:00:00Z"
	tsZeroS5  = "2024-01-01T00:00:05Z"
	tsEarly   = "2024-01-01T10:00:00Z"
	tsEarlyS5 = "2024-01-01T10:00:05Z"
	tsSeed    = "2025-01-15T10:00:00Z"
	tsSeedEnd = "2025-01-15T11:00:00Z"
)

// --- Test helpers ---

// testEnv sets up a server with a temporary database.
type testEnv struct {
	srv       *server.Server
	handler   http.Handler
	db        *db.DB
	claudeDir string
	dataDir   string
}

// setupOption customizes the config used by setup.
type setupOption func(*config.Config)

func withWriteTimeout(d time.Duration) setupOption {
	return func(c *config.Config) { c.WriteTimeout = d }
}

func setup(
	t *testing.T,
	opts ...setupOption,
) *testEnv {
	return setupWithServerOpts(t, nil, opts...)
}

func setupWithServerOpts(
	t *testing.T,
	srvOpts []server.Option,
	opts ...setupOption,
) *testEnv {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	claudeDir := filepath.Join(dir, "claude")
	codexDir := filepath.Join(dir, "codex")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("creating claude dir: %v", err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("creating codex dir: %v", err)
	}

	cfg := config.Config{
		Host:         "127.0.0.1",
		Port:         0,
		DataDir:      dir,
		DBPath:       dbPath,
		WriteTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	engine := sync.NewEngine(
		database, claudeDir, codexDir, "", "", "test",
	)
	srv := server.New(cfg, database, engine, srvOpts...)

	return &testEnv{
		srv:       srv,
		handler:   srv.Handler(),
		db:        database,
		claudeDir: claudeDir,
		dataDir:   dir,
	}
}

func (te *testEnv) writeProjectFile(
	t *testing.T, project, filename, content string,
) string {
	t.Helper()
	path := filepath.Join(te.claudeDir, project, filename)
	dbtest.WriteTestFile(t, path, []byte(content))
	return path
}

// writeSessionFile builds JSONL from a SessionBuilder and writes it
// as a project file, returning the file path.
func (te *testEnv) writeSessionFile(
	t *testing.T,
	project, filename string,
	b *testjsonl.SessionBuilder,
) string {
	t.Helper()
	return te.writeProjectFile(t, project, filename, b.String())
}

// listenAndServe starts the server on a real port and returns the
// base URL. The server is shut down when the test finishes.
func (te *testEnv) listenAndServe(t *testing.T) string {
	t.Helper()
	port := server.FindAvailablePort("127.0.0.1", 40000)
	te.srv.SetPort(port)

	var serveErr error
	done := make(chan struct{})
	go func() {
		serveErr = te.srv.ListenAndServe()
		close(done)
	}()

	// Wait for the port to accept connections.
	deadline := time.Now().Add(2 * time.Second)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ready := false
	var lastDialErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout(
			"tcp", addr, 50*time.Millisecond,
		)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		lastDialErr = err
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		select {
		case <-done:
			t.Fatalf(
				"server failed to start: %v", serveErr,
			)
		default:
		}
		t.Fatalf(
			"server not ready after 2s: last dial error: %v",
			lastDialErr,
		)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()
		if err := te.srv.Shutdown(ctx); err != nil &&
			err != http.ErrServerClosed {
			t.Errorf("server shutdown error: %v", err)
		}
		select {
		case <-done:
			if serveErr != nil &&
				serveErr != http.ErrServerClosed {
				t.Errorf(
					"server exited with error: %v",
					serveErr,
				)
			}
		case <-time.After(5 * time.Second):
			t.Error("timed out waiting for server goroutine")
		}
	})

	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func (te *testEnv) seedSession(
	t *testing.T, id, project string, msgCount int,
	opts ...func(*db.Session),
) {
	t.Helper()
	dbtest.SeedSession(t, te.db, id, project, func(s *db.Session) {
		s.Machine = "test"
		s.MessageCount = msgCount
		s.StartedAt = dbtest.Ptr(tsSeed)
		s.EndedAt = dbtest.Ptr(tsSeedEnd)
		s.FirstMessage = dbtest.Ptr("Hello world")
		for _, opt := range opts {
			opt(s)
		}
	})
}

func (te *testEnv) seedMessages(
	t *testing.T, sessionID string, count int, mods ...func(i int, m *db.Message),
) {
	t.Helper()
	msgs := make([]db.Message, count)
	for i := range count {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = db.Message{
			SessionID:     sessionID,
			Ordinal:       i,
			Role:          role,
			Content:       "Message " + string(rune('A'+i%26)),
			Timestamp:     tsSeed,
			ContentLength: 10,
		}
		for _, mod := range mods {
			mod(i, &msgs[i])
		}
	}
	if err := te.db.ReplaceSessionMessages(
		sessionID, msgs,
	); err != nil {
		t.Fatalf("seeding messages: %v", err)
	}
}

func (te *testEnv) get(
	t *testing.T, path string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

func (te *testEnv) post(
	t *testing.T, path string, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path,
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

func (te *testEnv) del(
	t *testing.T, path string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("DELETE", path, nil)
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

// uploadFile creates a multipart upload request.
func (te *testEnv) upload(
	t *testing.T, filename, content, query string,
) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST",
		"/api/v1/sessions/upload?"+query, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

// decode unmarshals the response body into a typed struct.
func decode[T any](
	t *testing.T, w *httptest.ResponseRecorder,
) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(
		w.Body.Bytes(), &result,
	); err != nil {
		t.Fatalf("decoding JSON: %v\nbody: %s",
			err, w.Body.String())
	}
	return result
}

func assertStatus(
	t *testing.T, w *httptest.ResponseRecorder, code int,
) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("expected status %d, got %d: %s",
			code, w.Code, w.Body.String())
	}
}

func assertBodyContains(
	t *testing.T, w *httptest.ResponseRecorder, substr string,
) {
	t.Helper()
	if !strings.Contains(w.Body.String(), substr) {
		t.Errorf("body %q does not contain %q",
			w.Body.String(), substr)
	}
}

// assertErrorResponse checks that the response body is a JSON
// object with an "error" field matching wantMsg.
func assertErrorResponse(
	t *testing.T, w *httptest.ResponseRecorder,
	wantMsg string,
) {
	t.Helper()
	resp := decode[map[string]string](t, w)
	if got := resp["error"]; got != wantMsg {
		t.Errorf("error = %q, want %q", got, wantMsg)
	}
}

// assertTimeoutRace validates a timeout response where either
// the middleware (503 "request timed out") or the handler
// (504 "gateway timeout") may win the race. Checks status,
// Content-Type, and error body.
func assertTimeoutRace(
	t *testing.T, w *httptest.ResponseRecorder,
) {
	t.Helper()
	code := w.Code
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf(
			"Content-Type = %q, want application/json", ct,
		)
	}
	switch code {
	case http.StatusServiceUnavailable:
		assertBodyContains(t, w, "request timed out")
	case http.StatusGatewayTimeout:
		assertBodyContains(t, w, "gateway timeout")
	default:
		t.Fatalf(
			"expected 503 or 504, got %d: %s",
			code, w.Body.String(),
		)
	}
}

// expiredContext returns a context with a deadline in the past.
func expiredContext(
	t *testing.T,
) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithDeadline(
		context.Background(), time.Now().Add(-1*time.Hour),
	)
}

func (te *testEnv) waitForSSEEvent(t *testing.T, w *flushRecorder, expectedEvent string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		<-ticker.C
		if strings.Contains(w.BodyString(), "event: "+expectedEvent) {
			return
		}
	}
	t.Fatalf("timed out waiting for event: %s, got: %s", expectedEvent, w.BodyString())
}

// --- Typed response structs for JSON decoding ---

type sessionListResponse struct {
	Sessions []db.Session `json:"sessions"`
	Total    int          `json:"total"`
}

type messageListResponse struct {
	Messages []db.Message `json:"messages"`
	Count    int          `json:"count"`
}

type minimapResponse struct {
	Entries []db.MinimapEntry `json:"entries"`
	Count   int               `json:"count"`
}

type searchResponse struct {
	Query   string            `json:"query"`
	Results []db.SearchResult `json:"results"`
	Count   int               `json:"count"`
}

type projectListResponse struct {
	Projects []db.ProjectInfo `json:"projects"`
}

type syncStatusResponse struct {
	LastSync string `json:"last_sync"`
}

type githubConfigResponse struct {
	Configured bool `json:"configured"`
}

type uploadResponse struct {
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Machine   string `json:"machine"`
	Messages  int    `json:"messages"`
}

type syncResultResponse struct {
	TotalSessions int `json:"total_sessions"`
}

// --- Tests ---

func TestListSessions_Empty(t *testing.T) {
	te := setup(t)
	w := te.get(t, "/api/v1/sessions")
	assertStatus(t, w, http.StatusOK)

	resp := decode[sessionListResponse](t, w)
	if len(resp.Sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d",
			len(resp.Sessions))
	}
}

func TestListSessions_WithData(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)
	te.seedSession(t, "s2", "my-app", 3)
	te.seedSession(t, "s3", "other-app", 1)

	w := te.get(t, "/api/v1/sessions")
	assertStatus(t, w, http.StatusOK)

	resp := decode[sessionListResponse](t, w)
	if len(resp.Sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d",
			len(resp.Sessions))
	}
}

func TestListSessions_ProjectFilter(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)
	te.seedSession(t, "s2", "other-app", 3)

	w := te.get(t, "/api/v1/sessions?project=my-app")
	assertStatus(t, w, http.StatusOK)

	resp := decode[sessionListResponse](t, w)
	if len(resp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d",
			len(resp.Sessions))
	}
}

func TestGetSession_Found(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)

	w := te.get(t, "/api/v1/sessions/s1")
	assertStatus(t, w, http.StatusOK)

	resp := decode[db.Session](t, w)
	if resp.ID != "s1" {
		t.Fatalf("expected id=s1, got %v", resp.ID)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/sessions/nonexistent")
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetMessages_AscDefault(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 10)
	te.seedMessages(t, "s1", 10)

	w := te.get(t, "/api/v1/sessions/s1/messages")
	assertStatus(t, w, http.StatusOK)

	resp := decode[messageListResponse](t, w)
	if len(resp.Messages) != 10 {
		t.Fatalf("expected 10 messages, got %d",
			len(resp.Messages))
	}
	first := resp.Messages[0]
	last := resp.Messages[9]
	if first.Ordinal > last.Ordinal {
		t.Fatal("expected ascending ordinal order")
	}
}

func TestGetMessages_DescDefault(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 10)
	te.seedMessages(t, "s1", 10)

	w := te.get(t,
		"/api/v1/sessions/s1/messages?direction=desc",
	)
	assertStatus(t, w, http.StatusOK)

	resp := decode[messageListResponse](t, w)
	if len(resp.Messages) != 10 {
		t.Fatalf("expected 10 messages, got %d",
			len(resp.Messages))
	}
	first := resp.Messages[0]
	last := resp.Messages[len(resp.Messages)-1]
	if first.Ordinal < last.Ordinal {
		t.Fatal("expected descending ordinal order")
	}
}

func TestGetMessages_DescWithFrom(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 20)
	te.seedMessages(t, "s1", 20)

	w := te.get(t,
		"/api/v1/sessions/s1/messages?direction=desc&from=10&limit=5",
	)
	assertStatus(t, w, http.StatusOK)

	resp := decode[messageListResponse](t, w)
	if len(resp.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d",
			len(resp.Messages))
	}
	if resp.Messages[0].Ordinal != 10 {
		t.Fatalf("expected first ordinal=10, got %d",
			resp.Messages[0].Ordinal)
	}
}

func TestGetMessages_Pagination(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 20)
	te.seedMessages(t, "s1", 20)

	// First page
	w := te.get(t,
		"/api/v1/sessions/s1/messages?from=0&limit=5",
	)
	assertStatus(t, w, http.StatusOK)
	resp := decode[messageListResponse](t, w)
	if len(resp.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d",
			len(resp.Messages))
	}
	if resp.Messages[4].Ordinal != 4 {
		t.Fatalf("expected last ordinal=4, got %d",
			resp.Messages[4].Ordinal)
	}

	// Second page
	w = te.get(t,
		"/api/v1/sessions/s1/messages?from=5&limit=5",
	)
	assertStatus(t, w, http.StatusOK)
	resp = decode[messageListResponse](t, w)
	if len(resp.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d",
			len(resp.Messages))
	}
	if resp.Messages[0].Ordinal != 5 {
		t.Fatalf("expected first ordinal=5, got %d",
			resp.Messages[0].Ordinal)
	}
}

func TestGetMessages_InvalidParams(t *testing.T) {
	te := setup(t)

	tests := []struct {
		name string
		path string
	}{
		{"InvalidLimit", "/api/v1/sessions/s1/messages?limit=abc"},
		{"InvalidFrom", "/api/v1/sessions/s1/messages?from=xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t, tt.path)
			assertStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestListSessions_InvalidLimit(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/sessions?limit=bad")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestListSessions_InvalidCursor(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/sessions?cursor=invalid-cursor")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSearch_InvalidParams(t *testing.T) {
	te := setup(t)

	tests := []struct {
		name string
		path string
	}{
		{"InvalidLimit", "/api/v1/search?q=test&limit=nope"},
		{"InvalidCursor", "/api/v1/search?q=test&cursor=bad"},
		{"EmptyQuery", "/api/v1/search"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t, tt.path)
			assertStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestGetMinimap(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)
	te.seedMessages(t, "s1", 5)

	w := te.get(t, "/api/v1/sessions/s1/minimap")
	assertStatus(t, w, http.StatusOK)

	resp := decode[minimapResponse](t, w)
	if len(resp.Entries) != 5 {
		t.Fatalf("expected 5 entries, got %d",
			len(resp.Entries))
	}
	if resp.Entries[0].Role == "" {
		t.Fatal("minimap should include role")
	}

	// Verify "content" is not present in raw JSON
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding raw JSON: %v", err)
	}
	entriesRaw, ok := raw["entries"].([]any)
	if !ok {
		t.Fatal("expected entries to be a list")
	}
	if len(entriesRaw) > 0 {
		first, ok := entriesRaw[0].(map[string]any)
		if !ok {
			t.Fatal("expected entry to be a map")
		}
		if _, ok := first["content"]; ok {
			t.Fatal("minimap should not include content")
		}
	}
}

func TestGetMinimap_FromOrdinal(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)
	te.seedMessages(t, "s1", 5)

	w := te.get(t, "/api/v1/sessions/s1/minimap?from=3")
	assertStatus(t, w, http.StatusOK)

	resp := decode[minimapResponse](t, w)
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d",
			len(resp.Entries))
	}
	if got := resp.Entries[0].Ordinal; got != 3 {
		t.Fatalf("first ordinal = %d, want 3", got)
	}
}

func TestGetMinimap_InvalidFrom(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 1)
	te.seedMessages(t, "s1", 1)

	w := te.get(t, "/api/v1/sessions/s1/minimap?from=bad")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGetMinimap_MaxSampled(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 10)
	te.seedMessages(t, "s1", 10)

	w := te.get(t, "/api/v1/sessions/s1/minimap?max=3")
	assertStatus(t, w, http.StatusOK)

	resp := decode[minimapResponse](t, w)
	if len(resp.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d",
			len(resp.Entries))
	}
	if got := resp.Entries[0].Ordinal; got != 0 {
		t.Fatalf("first ordinal = %d, want 0", got)
	}
	if got := resp.Entries[1].Ordinal; got != 4 {
		t.Fatalf("second ordinal = %d, want 4", got)
	}
	if got := resp.Entries[2].Ordinal; got != 9 {
		t.Fatalf("third ordinal = %d, want 9", got)
	}
}

func TestGetMinimap_InvalidMax(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 1)
	te.seedMessages(t, "s1", 1)

	w := te.get(t, "/api/v1/sessions/s1/minimap?max=0")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSearch_WithResults(t *testing.T) {
	te := setup(t)
	if !te.db.HasFTS() {
		t.Skip("skipping search test: no FTS support")
	}
	te.seedSession(t, "s1", "my-app", 3)
	te.seedMessages(t, "s1", 3, func(i int, m *db.Message) {
		switch i {
		case 0:
			m.Role = "user"
			m.Content = "fix the login bug"
			m.ContentLength = 17
		case 1:
			m.Role = "assistant"
			m.Content = "looking at auth module"
			m.ContentLength = 22
		case 2:
			m.Role = "user"
			m.Content = "ship it"
			m.ContentLength = 7
		}
	})

	w := te.get(t, "/api/v1/search?q=login")
	assertStatus(t, w, http.StatusOK)

	resp := decode[searchResponse](t, w)
	if resp.Query != "login" {
		t.Fatalf("expected query=login, got %v", resp.Query)
	}
	if resp.Count < 1 {
		t.Fatal("expected at least 1 search result")
	}
}

func TestSearch_Limits(t *testing.T) {
	te := setup(t)
	if !te.db.HasFTS() {
		t.Skip("skipping search test: no FTS support")
	}
	// Seed enough messages to test limits
	te.seedSession(t, "s1", "my-app", 600)
	te.seedMessages(t, "s1", 600, func(i int, m *db.Message) {
		m.Content = "common search term"
		m.ContentLength = 18
	})

	tests := []struct {
		name      string
		queryVal  string
		wantCount int
	}{
		{"DefaultLimit", "", 50},          // default
		{"ExplicitLimit", "limit=10", 10}, // explicit
		{"ZeroLimit", "limit=0", 50},      // treat as default
		{"LargeLimit", "limit=1000", 500}, // clamped to 500
		{"ExactMax", "limit=500", 500},    // max allowed
		{"JustOver", "limit=501", 500},    // clamped to 500
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/v1/search?q=common"
			if tt.queryVal != "" {
				path += "&" + tt.queryVal
			}
			w := te.get(t, path)
			assertStatus(t, w, http.StatusOK)

			resp := decode[searchResponse](t, w)
			if resp.Count != tt.wantCount {
				t.Errorf("limit=%q: got %d results, want %d",
					tt.queryVal, resp.Count, tt.wantCount)
			}
		})
	}
}

func TestSearch_CanceledContext(t *testing.T) {
	te := setup(t)
	if !te.db.HasFTS() {
		t.Skip("skipping search test: no FTS support")
	}
	te.seedSession(t, "s1", "my-app", 1)
	te.seedMessages(t, "s1", 1, func(i int, m *db.Message) {
		m.Content = "searchable content"
		m.ContentLength = 18
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(
		"GET", "/api/v1/search?q=searchable", nil,
	).WithContext(ctx)
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	// A canceled request should just return without writing a response
	// (implicit 200 with empty body in httptest, but importantly NO content).
	if w.Body.Len() > 0 {
		t.Errorf("expected empty body for canceled context, got: %s",
			w.Body.String())
	}
}

func TestSearch_DeadlineExceeded(t *testing.T) {
	te := setup(t)
	if !te.db.HasFTS() {
		t.Skip("skipping search test: no FTS support")
	}
	te.seedSession(t, "s1", "my-app", 1)
	te.seedMessages(t, "s1", 1, func(i int, m *db.Message) {
		m.Content = "searchable content"
		m.ContentLength = 18
	})

	ctx, cancel := expiredContext(t)
	defer cancel()

	req := httptest.NewRequest(
		"GET", "/api/v1/search?q=searchable", nil,
	).WithContext(ctx)
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assertTimeoutRace(t, w)
}

func TestSearch_NotAvailable(t *testing.T) {
	te := setup(t)
	// Simulate missing FTS by dropping the virtual table.
	// HasFTS() will return false because the query against messages_fts will fail.
	err := te.db.Update(func(tx *sql.Tx) error {
		_, err := tx.Exec("DROP TABLE IF EXISTS messages_fts")
		return err
	})
	if err != nil {
		t.Fatalf("dropping messages_fts: %v", err)
	}

	w := te.get(t, "/api/v1/search?q=foo")
	assertStatus(t, w, http.StatusNotImplemented)
	assertErrorResponse(t, w, "search not available")
}

func TestGetStats(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)
	te.seedMessages(t, "s1", 5)

	w := te.get(t, "/api/v1/stats")
	assertStatus(t, w, http.StatusOK)

	resp := decode[db.Stats](t, w)
	if resp.SessionCount != 1 {
		t.Fatalf("expected 1 session, got %d",
			resp.SessionCount)
	}
	if resp.MessageCount != 5 {
		t.Fatalf("expected 5 messages, got %d",
			resp.MessageCount)
	}
}

func TestListProjects(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 5)
	te.seedSession(t, "s2", "my-app", 3)
	te.seedSession(t, "s3", "other-app", 1)

	w := te.get(t, "/api/v1/projects")
	assertStatus(t, w, http.StatusOK)

	resp := decode[projectListResponse](t, w)
	if len(resp.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d",
			len(resp.Projects))
	}
}

func TestSyncStatus(t *testing.T) {
	te := setup(t)

	// Trigger a sync so LastSync is set
	w := te.post(t, "/api/v1/sync", "{}")
	assertStatus(t, w, http.StatusOK)

	w = te.get(t, "/api/v1/sync/status")
	assertStatus(t, w, http.StatusOK)

	resp := decode[syncStatusResponse](t, w)
	if resp.LastSync == "" {
		t.Fatal("expected last_sync field")
	}
}

func TestCORSHeaders(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/stats")
	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Fatalf("expected CORS *, got %q", cors)
	}
}

func TestCORSPreflight(t *testing.T) {
	te := setup(t)

	req := httptest.NewRequest(
		"OPTIONS", "/api/v1/sessions", nil,
	)
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusNoContent)
}

func TestCORSAllowMethods(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/stats")
	methods := w.Header().Get(
		"Access-Control-Allow-Methods",
	)
	for _, want := range []string{
		"GET", "POST", "DELETE", "OPTIONS",
	} {
		if !strings.Contains(methods, want) {
			t.Errorf(
				"Allow-Methods %q missing %s",
				methods, want,
			)
		}
	}
}

func TestGetGithubConfig(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/config/github")
	assertStatus(t, w, http.StatusOK)

	resp := decode[githubConfigResponse](t, w)
	if resp.Configured {
		t.Fatal("expected configured=false")
	}
}

func TestExportSession(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 3)
	te.seedMessages(t, "s1", 3)

	w := te.get(t, "/api/v1/sessions/s1/export")
	assertStatus(t, w, http.StatusOK)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %q", ct)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("expected attachment disposition, got %q", cd)
	}
	assertBodyContains(t, w, "my-app")
}

func TestExportSession_NotFound(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/sessions/nonexistent/export")
	assertStatus(t, w, http.StatusNotFound)
}

func TestPublishSession_NoToken(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 3)

	w := te.post(t, "/api/v1/sessions/s1/publish", "{}")
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestSetGithubConfig_InvalidInput(t *testing.T) {
	te := setup(t)

	tests := []struct {
		name string
		body string
	}{
		{"EmptyToken", `{"token": ""}`},
		{"InvalidJSON", `{bad json`},
		{"WhitespaceToken", `{"token": "   "}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.post(t, "/api/v1/config/github", tt.body)
			assertStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestPublishSession_NotFound(t *testing.T) {
	te := setup(t)
	te.srv.SetGithubToken("fake-token")

	w := te.post(t,
		"/api/v1/sessions/nonexistent/publish", "{}")
	assertStatus(t, w, http.StatusNotFound)
}

func TestExportSession_HTMLContent(t *testing.T) {
	te := setup(t)
	te.seedSession(t, "s1", "my-app", 3)
	te.seedMessages(t, "s1", 3)

	w := te.get(t, "/api/v1/sessions/s1/export")
	assertStatus(t, w, http.StatusOK)

	body := w.Body.String()
	for _, want := range []string{
		"<!DOCTYPE html>",
		"<header>",
		"<main>",
		"message-content",
		"message-role",
		"Agent Session",
	} {
		if !strings.Contains(body, want) {
			t.Errorf(
				"expected to contain %q, got:\n%s",
				want, body,
			)
		}
	}
}

func TestUploadSession(t *testing.T) {
	te := setup(t)

	content := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsEarly, "Hello upload").
		AddClaudeAssistant(tsEarlyS5, "Hi!").
		String()

	w := te.upload(t, "upload-test.jsonl", content,
		"project=myproj&machine=remote")
	assertStatus(t, w, http.StatusOK)

	resp := decode[uploadResponse](t, w)
	if resp.SessionID != "upload-test" {
		t.Errorf("session_id = %v", resp.SessionID)
	}
	if resp.Project != "myproj" {
		t.Errorf("project = %v", resp.Project)
	}
	if resp.Machine != "remote" {
		t.Errorf("machine = %v", resp.Machine)
	}
	if resp.Messages != 2 {
		t.Errorf("messages = %v", resp.Messages)
	}

	sess, err := te.db.GetSession(context.Background(), "upload-test")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess == nil {
		t.Fatal("session not found in DB")
	}
	if sess.Project != "myproj" {
		t.Errorf("stored project = %q", sess.Project)
	}
}

func TestUploadSession_Errors(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		query    string
	}{
		{
			"InvalidExtension",
			"bad.txt", "content", "project=myproj",
		},
		{
			"MissingProject",
			"test.jsonl", "{}", "",
		},
		{
			"TraversalProject",
			"test.jsonl", "{}", "project=../../../etc",
		},
		{
			"TraversalFilename",
			"..secret.jsonl", "{}", "project=safe",
		},
		{
			"DotPrefixProject",
			"test.jsonl", "{}", "project=.hidden",
		},
		{
			"DotPrefixFilename",
			".hidden.jsonl", "{}", "project=safe",
		},
		{
			"SlashInProject",
			"test.jsonl", "{}", "project=foo/bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			w := te.upload(t,
				tt.filename, tt.content, tt.query)
			assertStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestUploadSession_EmptyFile(t *testing.T) {
	te := setup(t)

	w := te.upload(t, "empty.jsonl", "",
		"project=myproj")
	assertStatus(t, w, http.StatusOK)

	resp := decode[uploadResponse](t, w)
	if resp.Messages != 0 {
		t.Errorf("messages = %v, want 0", resp.Messages)
	}
}

// noFlushWriter wraps an http.ResponseWriter without Flusher.
type noFlushWriter struct {
	http.ResponseWriter
}

func TestTriggerSync_NonStreaming(t *testing.T) {
	te := setup(t)

	// Seed a session file so we expect at least one session in the sync result.
	te.writeSessionFile(t, "test-proj", "sync-test.jsonl",
		testjsonl.NewSessionBuilder().
			AddClaudeUser(tsZero, "msg"),
	)

	rec := httptest.NewRecorder()
	nf := &noFlushWriter{rec}

	req := httptest.NewRequest("POST", "/api/v1/sync", nil)
	req.Header.Set("Content-Type", "application/json")
	te.handler.ServeHTTP(nf, req)
	assertStatus(t, rec, http.StatusOK)

	resp := decode[syncResultResponse](t, rec)
	if resp.TotalSessions != 1 {
		t.Fatalf("expected 1 total_session, got %d", resp.TotalSessions)
	}
}

// flushRecorder wraps httptest.ResponseRecorder to implement
// http.Flusher, enabling SSE streaming tests.
type flushRecorder struct {
	*httptest.ResponseRecorder
	mu stdlibsync.Mutex
}

func (f *flushRecorder) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(b)
}

func (f *flushRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

func (f *flushRecorder) BodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Body.String()
}

func TestTriggerSync_SSE(t *testing.T) {
	te := setup(t)

	te.writeSessionFile(t, "test-proj", "sse-test.jsonl",
		testjsonl.NewSessionBuilder().
			AddClaudeUser(tsZero, "msg"),
	)

	req := httptest.NewRequest("POST", "/api/v1/sync", nil)
	w := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	te.handler.ServeHTTP(w, req)

	te.waitForSSEEvent(t, w, "done", 5*time.Second)
	te.waitForSSEEvent(t, w, "progress", 5*time.Second)
}

func TestWatchSession_Events(t *testing.T) {
	te := setup(t)

	b := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "initial")
	content := b.String()
	sessionPath := te.writeSessionFile(t, "watch-proj", "watch-sess.jsonl", b)

	engine := sync.NewEngine(
		te.db, te.claudeDir,
		filepath.Join(te.dataDir, "codex"), "", "", "test",
	)
	engine.SyncAll(nil)

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	req := httptest.NewRequest(
		"GET", "/api/v1/sessions/watch-sess/watch", nil,
	).WithContext(ctx)
	w := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		te.handler.ServeHTTP(w, req)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)

	updated := content + testjsonl.NewSessionBuilder().
		AddClaudeAssistant(tsZeroS5, "response").
		String()
	if err := os.WriteFile(
		sessionPath, []byte(updated), 0o644,
	); err != nil {
		t.Fatalf("writing updated session file: %v", err)
	}

	te.waitForSSEEvent(t, w, "session_updated", 5*time.Second)
	cancel()
	<-done
}

func TestWatchSession_FileDisappearAndResolve(t *testing.T) {
	te := setup(t)

	b := testjsonl.NewSessionBuilder().
		AddClaudeUser(tsZero, "initial")
	content := b.String()
	sessionPath := te.writeSessionFile(t, "vanish-proj", "vanish-sess.jsonl", b)

	engine := sync.NewEngine(
		te.db, te.claudeDir,
		filepath.Join(te.dataDir, "codex"), "", "", "test",
	)
	engine.SyncAll(nil)

	ctx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()

	req := httptest.NewRequest(
		"GET", "/api/v1/sessions/vanish-sess/watch", nil,
	).WithContext(ctx)
	w := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		te.handler.ServeHTTP(w, req)
		close(done)
	}()

	// Let the monitor start and record the initial mtime.
	time.Sleep(200 * time.Millisecond)

	// Delete the source file to simulate disappearance.
	if err := os.Remove(sessionPath); err != nil {
		t.Fatalf("removing session file: %v", err)
	}

	// Wait long enough for at least one poll tick to notice
	// the missing file and clear the cached path.
	time.Sleep(2 * time.Second)

	// Recreate the file with updated content at a NEW location
	// so we verify that FindSourceFile actually re-scans.
	updated := content + testjsonl.NewSessionBuilder().
		AddClaudeAssistant(tsZeroS5, "recovered").
		String()
	te.writeProjectFile(t, "moved-proj", "vanish-sess.jsonl", updated)

	te.waitForSSEEvent(t, w, "session_updated", 8*time.Second)
	cancel()
	<-done
}

// parseSSEEvents extracts event types from an SSE stream body.
func parseSSEEvents(body string) []string {
	var events []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if ev, ok := strings.CutPrefix(
			line, "event: ",
		); ok {
			events = append(events, ev)
		}
	}
	return events
}

func TestTriggerSync_SSEEvents(t *testing.T) {
	te := setup(t)

	for _, name := range []string{"a", "b"} {
		te.writeSessionFile(t, "sse-proj", name+".jsonl",
			testjsonl.NewSessionBuilder().
				AddClaudeUser(tsZero, fmt.Sprintf("msg %s", name)),
		)
	}

	req := httptest.NewRequest("POST", "/api/v1/sync", nil)
	w := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	te.handler.ServeHTTP(w, req)

	events := parseSSEEvents(w.BodyString())
	hasDone := false
	hasProgress := false
	for _, e := range events {
		if e == "done" {
			hasDone = true
		}
		if e == "progress" {
			hasProgress = true
		}
	}
	if !hasDone {
		t.Error("expected done event")
	}
	if !hasProgress {
		t.Error("expected progress event")
	}
}

func TestListSessions_Limits(t *testing.T) {
	te := setup(t)
	// db.MaxSessionLimit is 500
	for i := range 505 {
		te.seedSession(t, fmt.Sprintf("s%d", i), "my-app", 1)
	}

	tests := []struct {
		name      string
		limitVal  string
		wantCount int
	}{
		{"DefaultLimit", "", 200}, // db.DefaultSessionLimit
		{"ExplicitLimit", "limit=10", 10},
		{"LargeLimit", "limit=1000", 500}, // db.MaxSessionLimit
		{"ExactMax", "limit=500", 500},
		{"JustOver", "limit=501", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/v1/sessions"
			if tt.limitVal != "" {
				path += "?" + tt.limitVal
			}
			w := te.get(t, path)
			assertStatus(t, w, http.StatusOK)

			resp := decode[sessionListResponse](t, w)
			if len(resp.Sessions) != tt.wantCount {
				t.Errorf("limit=%q: got %d sessions, want %d",
					tt.limitVal, len(resp.Sessions), tt.wantCount)
			}
		})
	}
}

func TestGetMessages_Limits(t *testing.T) {
	te := setup(t)
	// db.MaxMessageLimit is 1000.
	te.seedSession(t, "s1", "my-app", 1005)
	te.seedMessages(t, "s1", 1005)

	tests := []struct {
		name      string
		limitVal  string
		wantCount int
	}{
		{"DefaultLimit", "", 100}, // db.DefaultMessageLimit
		{"ExplicitLimit", "limit=10", 10},
		{"LargeLimit", "limit=2000", 1000}, // db.MaxMessageLimit
		{"ExactMax", "limit=1000", 1000},
		{"JustOver", "limit=1001", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/v1/sessions/s1/messages"
			if tt.limitVal != "" {
				path += "?" + tt.limitVal
			}
			w := te.get(t, path)
			assertStatus(t, w, http.StatusOK)

			resp := decode[messageListResponse](t, w)
			if len(resp.Messages) != tt.wantCount {
				t.Errorf("limit=%q: got %d messages, want %d",
					tt.limitVal, len(resp.Messages), tt.wantCount)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	v := server.VersionInfo{
		Version:   "v1.2.3",
		Commit:    "abc1234",
		BuildDate: "2025-01-15T00:00:00Z",
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(v),
	})

	w := te.get(t, "/api/v1/version")
	assertStatus(t, w, http.StatusOK)

	resp := decode[server.VersionInfo](t, w)
	if resp.Version != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", resp.Version)
	}
	if resp.Commit != "abc1234" {
		t.Errorf("commit = %q, want abc1234", resp.Commit)
	}
	if resp.BuildDate != "2025-01-15T00:00:00Z" {
		t.Errorf(
			"build_date = %q, want 2025-01-15T00:00:00Z",
			resp.BuildDate,
		)
	}
}

func TestGetVersion_Default(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/version")
	assertStatus(t, w, http.StatusOK)

	resp := decode[server.VersionInfo](t, w)
	if resp.Version != "" {
		t.Errorf("version = %q, want empty", resp.Version)
	}
}

func TestFindAvailablePortSkipsOccupied(t *testing.T) {
	// Bind a port on 127.0.0.1 so FindAvailablePort must skip it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	occupied := ln.Addr().(*net.TCPAddr).Port

	got := server.FindAvailablePort("127.0.0.1", occupied)
	if got == occupied {
		t.Errorf(
			"FindAvailablePort returned occupied port %d", occupied,
		)
	}

	// The returned port should be bindable on the same host.
	ln2, err := net.Listen(
		"tcp",
		fmt.Sprintf("127.0.0.1:%d", got),
	)
	if err != nil {
		t.Fatalf(
			"returned port %d not bindable: %v", got, err,
		)
	}
	ln2.Close()
}

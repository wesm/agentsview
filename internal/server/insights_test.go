package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/insight"
	"github.com/wesm/agentsview/internal/server"
)

type listInsightsResponse struct {
	Insights []db.Insight `json:"insights"`
}

func TestListInsights(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(t *testing.T, te *testEnv)
		path       string
		wantStatus int
		wantCount  int
		wantBody   string
	}{
		{
			name:       "Empty",
			seed:       func(t *testing.T, te *testEnv) {},
			path:       "/api/v1/insights",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "WithData",
			seed: func(t *testing.T, te *testEnv) {
				te.seedInsight(t, "daily_activity", "2025-01-15", strPtr("my-app"))
				te.seedInsight(t, "daily_activity", "2025-01-15", strPtr("other-app"))
				te.seedInsight(t, "agent_analysis", "2025-01-15", nil)
			},
			path:       "/api/v1/insights",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name: "TypeFilter",
			seed: func(t *testing.T, te *testEnv) {
				te.seedInsight(t, "daily_activity", "2025-01-15", strPtr("my-app"))
				te.seedInsight(t, "agent_analysis", "2025-01-15", nil)
			},
			path:       "/api/v1/insights?type=daily_activity",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "ReturnsAll",
			seed: func(t *testing.T, te *testEnv) {
				te.seedInsight(t, "daily_activity", "2025-01-15", strPtr("my-app"))
				te.seedInsight(t, "daily_activity", "2025-01-16", strPtr("my-app"))
			},
			path:       "/api/v1/insights",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "InvalidType",
			seed:       func(t *testing.T, te *testEnv) {},
			path:       "/api/v1/insights?type=invalid",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			tt.seed(t, te)

			w := te.get(t, tt.path)
			assertStatus(t, w, tt.wantStatus)

			if tt.wantBody != "" {
				assertBodyContains(t, w, tt.wantBody)
			}

			if tt.wantStatus == http.StatusOK {
				r := decode[listInsightsResponse](t, w)
				if len(r.Insights) != tt.wantCount {
					t.Fatalf("expected %d insights, got %d", tt.wantCount, len(r.Insights))
				}
			}
		})
	}
}

func TestGetInsight_Found(t *testing.T) {
	te := setup(t)

	id := te.seedInsight(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))

	w := te.get(t, fmt.Sprintf("/api/v1/insights/%d", id))
	assertStatus(t, w, http.StatusOK)

	r := decode[db.Insight](t, w)
	if r.ID != id {
		t.Fatalf("expected id=%d, got %d", id, r.ID)
	}
	if r.Type != "daily_activity" {
		t.Errorf("type = %q, want daily_activity", r.Type)
	}
}

func TestGenerateInsight_Validation(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantBody string
	}{
		{"InvalidType", `{"type":"bad","date_from":"2025-01-15","date_to":"2025-01-15"}`, ""},
		{"InvalidDateFrom", `{"type":"daily_activity","date_from":"bad","date_to":"2025-01-15"}`, "date_from"},
		{"InvalidDateTo", `{"type":"daily_activity","date_from":"2025-01-15","date_to":"bad"}`, "date_to"},
		{"DateToBeforeDateFrom", `{"type":"daily_activity","date_from":"2025-01-16","date_to":"2025-01-15"}`, "date_to must be"},
		{"InvalidJSON", `{bad json`, ""},
		{"InvalidAgent", `{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"gpt"}`, "invalid agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			w := te.post(t, "/api/v1/insights/generate", tt.payload)

			assertStatus(t, w, http.StatusBadRequest)
			if tt.wantBody != "" {
				assertBodyContains(t, w, tt.wantBody)
			}
		})
	}
}

func TestGenerateInsight_DefaultAgent(t *testing.T) {
	stubGen := func(
		_ context.Context, agent, _ string,
	) (insight.Result, error) {
		if agent != "claude" {
			t.Errorf("expected default agent claude, got %q", agent)
		}
		return insight.Result{}, fmt.Errorf("stub: no CLI")
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateFunc(stubGen),
	})

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusOK)
	assertBodyContains(t, w, "event: error")
	assertBodyContains(t, w, "stub: no CLI")
}

func TestGenerateInsight_StreamsLogs(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		onLog(insight.LogEvent{
			Stream: "stdout",
			Line:   `{"type":"system","status":"ready"}`,
		})
		onLog(insight.LogEvent{
			Stream: "stderr",
			Line:   "rate limit warning",
		})
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
			Model:   "test-model",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`)
	assertStatus(t, w, http.StatusOK)

	events := parseSSE(w.Body.String())
	if len(events) < 4 {
		t.Fatalf("expected >=4 SSE events, got %d: %s", len(events), w.Body.String())
	}
	if events[0].Event != "status" {
		t.Fatalf("first event = %q, want status", events[0].Event)
	}
	if events[1].Event != "log" || events[2].Event != "log" {
		t.Fatalf("expected two log events, got: %#v", events)
	}
	if events[len(events)-1].Event != "done" {
		t.Fatalf("last event = %q, want done", events[len(events)-1].Event)
	}

	var log1 insight.LogEvent
	if err := json.Unmarshal([]byte(events[1].Data), &log1); err != nil {
		t.Fatalf("unmarshal first log event: %v", err)
	}
	if log1.Stream != "stdout" {
		t.Fatalf("first log stream = %q, want stdout", log1.Stream)
	}

	var log2 insight.LogEvent
	if err := json.Unmarshal([]byte(events[2].Data), &log2); err != nil {
		t.Fatalf("unmarshal second log event: %v", err)
	}
	if log2.Stream != "stderr" {
		t.Fatalf("second log stream = %q, want stderr", log2.Stream)
	}
}

type slowFlushRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
}

func (f *slowFlushRecorder) Write(
	b []byte,
) (int, error) {
	time.Sleep(f.delay)
	return f.ResponseRecorder.Write(b)
}

func (f *slowFlushRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

type slowLogRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
}

func (f *slowLogRecorder) Write(
	b []byte,
) (int, error) {
	if strings.HasPrefix(string(b), "event: log\n") {
		time.Sleep(f.delay)
	}
	return f.ResponseRecorder.Write(b)
}

func (f *slowLogRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

type blockingLogRecorder struct {
	*httptest.ResponseRecorder
	release <-chan struct{}
}

func (f *blockingLogRecorder) Write(
	b []byte,
) (int, error) {
	if strings.HasPrefix(string(b), "event: log\n") {
		<-f.release
	}
	return f.ResponseRecorder.Write(b)
}

func (f *blockingLogRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

type firstLogDelayRecorder struct {
	*httptest.ResponseRecorder
	delay time.Duration
	once  sync.Once
}

func (f *firstLogDelayRecorder) Write(
	b []byte,
) (int, error) {
	if strings.HasPrefix(string(b), "event: log\n") {
		f.once.Do(func() {
			time.Sleep(f.delay)
		})
	}
	return f.ResponseRecorder.Write(b)
}

func (f *firstLogDelayRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

func TestGenerateInsight_LogDropSummaryAndCompletion(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		for i := range 5000 {
			onLog(insight.LogEvent{
				Stream: "stdout",
				Line:   fmt.Sprintf("line-%d", i),
			})
		}
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	w := &slowFlushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            4 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatalf("timed out waiting for generate handler")
	}

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.Body.String())

	foundDone := false
	foundDropSummary := false
	for _, ev := range events {
		if ev.Event == "done" {
			foundDone = true
		}
		if ev.Event != "log" {
			continue
		}
		var line insight.LogEvent
		if json.Unmarshal([]byte(ev.Data), &line) != nil {
			continue
		}
		if line.Stream == "stderr" &&
			strings.Contains(line.Line, "dropped ") &&
			strings.Contains(line.Line, "slow client") {
			foundDropSummary = true
		}
	}
	if !foundDropSummary {
		t.Fatalf(
			"expected dropped-log summary event, got %d events",
			len(events),
		)
	}
	if !foundDone {
		t.Fatalf("expected done event")
	}
}

func TestGenerateInsight_LogDrainTimeoutReturnsWithoutHang(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		for i := range 10 {
			onLog(insight.LogEvent{
				Stream: "stdout",
				Line:   fmt.Sprintf("slow-line-%d", i),
			})
		}
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	w := &slowLogRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            5 * time.Second,
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatalf("timed out waiting for generate handler completion")
	}
	if elapsed := time.Since(started); elapsed > 7*time.Second {
		t.Fatalf("handler should return within bounded timeout handling, took %s", elapsed)
	}

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.Body.String())
	for _, ev := range events {
		if ev.Event == "done" {
			t.Fatalf("did not expect done event when timeout path is triggered")
		}
	}
}

func TestGenerateInsight_LogDrainTimeoutReportsBufferedDrops(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		for i := range 10 {
			onLog(insight.LogEvent{
				Stream: "stdout",
				Line:   fmt.Sprintf("slow-line-%d", i),
			})
		}
		return insight.Result{
			Content: "# Insight",
			Agent:   "claude",
		}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	w := &firstLogDelayRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		delay:            2200 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for generate handler completion")
	}

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.Body.String())
	foundTimeoutError := false
	foundDropSummary := false
	for _, ev := range events {
		if ev.Event == "done" {
			t.Fatalf("did not expect done event when timeout path is triggered")
		}
		if ev.Event == "error" &&
			strings.Contains(ev.Data, "timed out before completion") {
			foundTimeoutError = true
		}
		if ev.Event != "log" {
			continue
		}
		var line insight.LogEvent
		if json.Unmarshal([]byte(ev.Data), &line) != nil {
			continue
		}
		if line.Stream != "stderr" ||
			!strings.HasPrefix(line.Line, "dropped ") ||
			!strings.Contains(line.Line, "log stream timeout") {
			continue
		}
		parts := strings.SplitN(line.Line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		dropped, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		// 10 events were enqueued; timeout truncation should account
		// for most buffered entries that were never flushed.
		if dropped < 8 {
			t.Fatalf("expected timeout drop summary >=8, got %d (%q)", dropped, line.Line)
		}
		foundDropSummary = true
	}
	if !foundTimeoutError {
		t.Fatalf("expected timeout error event, got %d events", len(events))
	}
	if !foundDropSummary {
		t.Fatalf("expected timeout-aware drop summary, got %d events", len(events))
	}
}

func TestGenerateInsight_LogDrainTimeoutBoundedWhenWriterStuck(t *testing.T) {
	stubGen := func(
		_ context.Context, _ string, _ string, onLog insight.LogFunc,
	) (insight.Result, error) {
		onLog(insight.LogEvent{Stream: "stdout", Line: "stuck-line"})
		return insight.Result{Content: "# Insight", Agent: "claude"}, nil
	}
	te := setupWithServerOpts(t, []server.Option{
		server.WithGenerateStreamFunc(stubGen),
	})

	req := httptest.NewRequest(
		http.MethodPost, "/api/v1/insights/generate",
		strings.NewReader(
			`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"claude"}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	release := make(chan struct{})
	w := &blockingLogRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		release:          release,
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		te.handler.ServeHTTP(w, req)
	}()

	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatalf("timed out waiting for bounded timeout behavior")
	}
	elapsed := time.Since(started)
	if elapsed > 6*time.Second {
		t.Fatalf("handler returned too slowly for stuck writer path: %s", elapsed)
	}
	close(release)

	assertStatus(t, w.ResponseRecorder, http.StatusOK)
	events := parseSSE(w.Body.String())
	for _, ev := range events {
		if ev.Event == "done" {
			t.Fatalf("did not expect done event on stuck writer timeout path")
		}
	}
}

func TestDeleteInsight_Found(t *testing.T) {
	te := setup(t)

	id := te.seedInsight(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))

	w := te.del(t, fmt.Sprintf("/api/v1/insights/%d", id))
	assertStatus(t, w, http.StatusNoContent)

	// Verify it's gone.
	w = te.get(t, fmt.Sprintf("/api/v1/insights/%d", id))
	assertStatus(t, w, http.StatusNotFound)
}

func TestInsight_ResourceErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{"Get_NotFound", http.MethodGet, "/api/v1/insights/99999", http.StatusNotFound},
		{"Get_InvalidID", http.MethodGet, "/api/v1/insights/abc", http.StatusBadRequest},
		{"Delete_NotFound", http.MethodDelete, "/api/v1/insights/99999", http.StatusNotFound},
		{"Delete_InvalidID", http.MethodDelete, "/api/v1/insights/abc", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			if tt.method == http.MethodGet {
				w := te.get(t, tt.path)
				assertStatus(t, w, tt.status)
			} else {
				w := te.del(t, tt.path)
				assertStatus(t, w, tt.status)
			}
		})
	}
}

// --- helpers ---

func strPtr(s string) *string { return &s }

func (te *testEnv) seedInsight(
	t *testing.T,
	typ, date string,
	project *string,
) int64 {
	t.Helper()
	id, err := te.db.InsertInsight(db.Insight{
		Type:     typ,
		DateFrom: date,
		DateTo:   date,
		Project:  project,
		Agent:    "claude",
		Content:  "Test insight content",
	})
	if err != nil {
		t.Fatalf("seeding insight: %v", err)
	}
	return id
}

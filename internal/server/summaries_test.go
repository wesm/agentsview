package server_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/wesm/agentsview/internal/db"
)

func TestListSummaries_Empty(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/summaries")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Summaries []db.Summary `json:"summaries"`
	}
	r := decode[resp](t, w)
	if len(r.Summaries) != 0 {
		t.Fatalf("expected 0 summaries, got %d",
			len(r.Summaries))
	}
}

func TestListSummaries_WithData(t *testing.T) {
	te := setup(t)

	te.seedSummary(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))
	te.seedSummary(t, "daily_activity", "2025-01-15",
		strPtr("other-app"))
	te.seedSummary(t, "agent_analysis", "2025-01-15", nil)

	w := te.get(t, "/api/v1/summaries")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Summaries []db.Summary `json:"summaries"`
	}
	r := decode[resp](t, w)
	if len(r.Summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d",
			len(r.Summaries))
	}
}

func TestListSummaries_TypeFilter(t *testing.T) {
	te := setup(t)

	te.seedSummary(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))
	te.seedSummary(t, "agent_analysis", "2025-01-15", nil)

	w := te.get(t,
		"/api/v1/summaries?type=daily_activity")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Summaries []db.Summary `json:"summaries"`
	}
	r := decode[resp](t, w)
	if len(r.Summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d",
			len(r.Summaries))
	}
}

func TestListSummaries_DateFilter(t *testing.T) {
	te := setup(t)

	te.seedSummary(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))
	te.seedSummary(t, "daily_activity", "2025-01-16",
		strPtr("my-app"))

	w := te.get(t, "/api/v1/summaries?date=2025-01-15")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Summaries []db.Summary `json:"summaries"`
	}
	r := decode[resp](t, w)
	if len(r.Summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d",
			len(r.Summaries))
	}
}

func TestListSummaries_InvalidType(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/summaries?type=invalid")
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "invalid type")
}

func TestListSummaries_InvalidDate(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/summaries?date=not-a-date")
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "invalid date")
}

func TestGetSummary_Found(t *testing.T) {
	te := setup(t)

	id := te.seedSummary(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))

	w := te.get(t, fmt.Sprintf("/api/v1/summaries/%d", id))
	assertStatus(t, w, http.StatusOK)

	r := decode[db.Summary](t, w)
	if r.ID != id {
		t.Fatalf("expected id=%d, got %d", id, r.ID)
	}
	if r.Type != "daily_activity" {
		t.Errorf("type = %q, want daily_activity", r.Type)
	}
}

func TestGetSummary_NotFound(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/summaries/99999")
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetSummary_InvalidID(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/summaries/abc")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGenerateSummary_InvalidType(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/summaries/generate",
		`{"type":"bad","date":"2025-01-15"}`)
	// SSE or JSON error depending on Flusher support.
	// With httptest.NewRecorder, no Flusher, so falls through
	// to writeError.
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGenerateSummary_InvalidDate(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/summaries/generate",
		`{"type":"daily_activity","date":"bad"}`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGenerateSummary_InvalidJSON(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/summaries/generate",
		`{bad json`)
	assertStatus(t, w, http.StatusBadRequest)
}

// --- helpers ---

func strPtr(s string) *string { return &s }

func (te *testEnv) seedSummary(
	t *testing.T,
	typ, date string,
	project *string,
) int64 {
	t.Helper()
	id, err := te.db.InsertSummary(db.Summary{
		Type:    typ,
		Date:    date,
		Project: project,
		Agent:   "claude",
		Content: "Test summary content",
	})
	if err != nil {
		t.Fatalf("seeding summary: %v", err)
	}
	return id
}

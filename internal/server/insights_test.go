package server_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/insight"
	"github.com/wesm/agentsview/internal/server"
)

func TestListInsights_Empty(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/insights")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Insights []db.Insight `json:"insights"`
	}
	r := decode[resp](t, w)
	if len(r.Insights) != 0 {
		t.Fatalf("expected 0 insights, got %d",
			len(r.Insights))
	}
}

func TestListInsights_WithData(t *testing.T) {
	te := setup(t)

	te.seedInsight(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))
	te.seedInsight(t, "daily_activity", "2025-01-15",
		strPtr("other-app"))
	te.seedInsight(t, "agent_analysis", "2025-01-15", nil)

	w := te.get(t, "/api/v1/insights")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Insights []db.Insight `json:"insights"`
	}
	r := decode[resp](t, w)
	if len(r.Insights) != 3 {
		t.Fatalf("expected 3 insights, got %d",
			len(r.Insights))
	}
}

func TestListInsights_TypeFilter(t *testing.T) {
	te := setup(t)

	te.seedInsight(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))
	te.seedInsight(t, "agent_analysis", "2025-01-15", nil)

	w := te.get(t,
		"/api/v1/insights?type=daily_activity")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Insights []db.Insight `json:"insights"`
	}
	r := decode[resp](t, w)
	if len(r.Insights) != 1 {
		t.Fatalf("expected 1 insight, got %d",
			len(r.Insights))
	}
}

func TestListInsights_ReturnsAll(t *testing.T) {
	te := setup(t)

	te.seedInsight(t, "daily_activity", "2025-01-15",
		strPtr("my-app"))
	te.seedInsight(t, "daily_activity", "2025-01-16",
		strPtr("my-app"))

	w := te.get(t, "/api/v1/insights")
	assertStatus(t, w, http.StatusOK)

	type resp struct {
		Insights []db.Insight `json:"insights"`
	}
	r := decode[resp](t, w)
	if len(r.Insights) != 2 {
		t.Fatalf("expected 2 insights, got %d",
			len(r.Insights))
	}
}

func TestListInsights_InvalidType(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/insights?type=invalid")
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "invalid type")
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

func TestGetInsight_NotFound(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/insights/99999")
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetInsight_InvalidID(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/insights/abc")
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGenerateInsight_InvalidType(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"bad","date_from":"2025-01-15","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGenerateInsight_InvalidDateFrom(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"bad","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "date_from")
}

func TestGenerateInsight_InvalidDateTo(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"bad"}`)
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "date_to")
}

func TestGenerateInsight_DateToBeforeDateFrom(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-16","date_to":"2025-01-15"}`)
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "date_to must be")
}

func TestGenerateInsight_InvalidJSON(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/insights/generate",
		`{bad json`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGenerateInsight_InvalidAgent(t *testing.T) {
	te := setup(t)

	w := te.post(t, "/api/v1/insights/generate",
		`{"type":"daily_activity","date_from":"2025-01-15","date_to":"2025-01-15","agent":"gpt"}`)
	assertStatus(t, w, http.StatusBadRequest)
	assertBodyContains(t, w, "invalid agent")
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

func TestDeleteInsight_NotFound(t *testing.T) {
	te := setup(t)

	w := te.del(t, "/api/v1/insights/99999")
	assertStatus(t, w, http.StatusNotFound)
}

func TestDeleteInsight_InvalidID(t *testing.T) {
	te := setup(t)

	w := te.del(t, "/api/v1/insights/abc")
	assertStatus(t, w, http.StatusBadRequest)
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

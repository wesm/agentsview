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

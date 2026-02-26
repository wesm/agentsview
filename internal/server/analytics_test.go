package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/dbtest"
)

const basePath = "/api/v1/analytics/"

// seedStats holds expected values after seeding the database.
type seedStats struct {
	TotalSessions  int
	TotalMessages  int
	ActiveProjects int
}

// seedAnalyticsEnv populates the test env with sessions and
// messages suitable for analytics endpoint tests. Some messages
// include tool_calls for tool analytics testing.
func seedAnalyticsEnv(t *testing.T, te *testEnv) seedStats {
	t.Helper()

	type entry struct {
		id, project, agent, started string
		msgs                        int
	}
	entries := []entry{
		{"a1", "alpha", "claude", "2024-06-01T09:00:00Z", 10},
		{"a2", "alpha", "codex", "2024-06-01T14:00:00Z", 20},
		{"b1", "beta", "claude", "2024-06-02T10:00:00Z", 30},
	}

	stats := seedStats{
		TotalSessions:  len(entries),
		TotalMessages:  0,
		ActiveProjects: 2, // alpha and beta
	}

	for _, s := range entries {
		stats.TotalMessages += s.msgs
		started := s.started
		te.seedSession(t, s.id, s.project, s.msgs,
			func(sess *db.Session) {
				sess.Agent = s.agent
				sess.StartedAt = &started
				sess.EndedAt = &started
				sess.FirstMessage = dbtest.Ptr("Hello")
			},
		)
		te.seedMessages(t, s.id, s.msgs,
			func(i int, m *db.Message) {
				// Add tool calls on every other assistant msg
				if m.Role == "assistant" && i%4 == 1 {
					m.HasToolUse = true
					m.ToolCalls = []db.ToolCall{
						{
							SessionID: s.id,
							ToolName:  "Read",
							Category:  "Read",
						},
					}
				}
			},
		)
	}
	return stats
}

// buildURL constructs an analytics API URL.
func buildURL(path string, params map[string]string) string {
	u, _ := url.Parse(basePath + path)
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// buildURLWithRange constructs an analytics API URL with default from/to params.
func buildURLWithRange(path string, params map[string]string) string {
	if params == nil {
		params = make(map[string]string)
	}
	if _, ok := params["from"]; !ok {
		params["from"] = "2024-06-01"
	}
	if _, ok := params["to"]; !ok {
		params["to"] = "2024-06-03"
	}
	return buildURL(path, params)
}

func TestAnalyticsSummary(t *testing.T) {
	te := setup(t)
	stats := seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("summary", map[string]string{"timezone": "UTC"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.AnalyticsSummary](t, w)
		if resp.TotalSessions != stats.TotalSessions {
			t.Errorf("TotalSessions = %d, want %d", resp.TotalSessions, stats.TotalSessions)
		}
		if resp.TotalMessages != stats.TotalMessages {
			t.Errorf("TotalMessages = %d, want %d", resp.TotalMessages, stats.TotalMessages)
		}
		if resp.ActiveProjects != stats.ActiveProjects {
			t.Errorf("ActiveProjects = %d, want %d", resp.ActiveProjects, stats.ActiveProjects)
		}
	})

	t.Run("DefaultDateRange", func(t *testing.T) {
		w := te.get(t, buildURL("summary", nil))
		assertStatus(t, w, http.StatusOK)
		// Should not error — defaults to last 30 days
	})

	t.Run("NonUTCTimezone", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("summary", map[string]string{"timezone": "America/New_York"}))
		assertStatus(t, w, http.StatusOK)
	})

	t.Run("InvalidTimezone", func(t *testing.T) {
		w := te.get(t, buildURL("summary", map[string]string{"timezone": "Fake/Zone"}))
		assertStatus(t, w, http.StatusBadRequest)
	})
}

func TestAnalyticsSummary_DateValidation(t *testing.T) {
	te := setup(t)

	tests := []struct {
		name   string
		params map[string]string
		status int
	}{
		{
			"InvalidFromFormat",
			map[string]string{"from": "not-a-date", "to": "2024-06-03"},
			http.StatusBadRequest,
		},
		{
			"InvalidToFormat",
			map[string]string{"from": "2024-06-01", "to": "06-03-2024"},
			http.StatusBadRequest,
		},
		{
			"FromAfterTo",
			map[string]string{"from": "2024-07-01", "to": "2024-06-01"},
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t, buildURL("summary", tt.params))
			assertStatus(t, w, tt.status)
		})
	}
}

func TestAnalyticsErrorRedaction(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	// Valid request should succeed
	w := te.get(t, buildURLWithRange("summary", nil))
	assertStatus(t, w, http.StatusOK)

	// Force a DB error by closing the database
	te.db.Close()

	endpoints := []string{
		"summary",
		"activity",
		"heatmap",
		"projects",
		"hour-of-week",
		"sessions",
		"velocity",
		"tools",
		"top-sessions",
	}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := te.get(t, buildURLWithRange(ep, nil))
			assertStatus(t, w, http.StatusInternalServerError)
			body := w.Body.String()
			if strings.Contains(body, "sql") || strings.Contains(body, "database") {
				t.Errorf("response exposes internal error: %s", body)
			}
		})
	}
}

func TestSessionsDateValidation(t *testing.T) {
	te := setup(t)

	// We use raw URL parsing here because it accesses /api/v1/sessions instead of /api/v1/analytics/
	// So we won't use buildURL for this one, or we can use a generic request builder if we extract it.
	// But it's fine to keep query params simple or use net/url directly.

	tests := []struct {
		name   string
		query  string
		status int
	}{
		{
			"InvalidDateFormat",
			"?date=not-a-date",
			http.StatusBadRequest,
		},
		{
			"InvalidDateFromFormat",
			"?date_from=2024/06/01",
			http.StatusBadRequest,
		},
		{
			"DateFromAfterDateTo",
			"?date_from=2024-07-01&date_to=2024-06-01",
			http.StatusBadRequest,
		},
		{
			"ValidDateRange",
			"?date_from=2024-06-01&date_to=2024-06-03",
			http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t, "/api/v1/sessions"+tt.query)
			assertStatus(t, w, tt.status)
		})
	}
}

func TestActiveSinceValidation(t *testing.T) {
	te := setup(t)

	tests := []struct {
		name   string
		path   string
		query  string
		status int
	}{
		{
			"Sessions_InvalidActiveSince",
			"/api/v1/sessions",
			"?active_since=garbage",
			http.StatusBadRequest,
		},
		{
			"Sessions_ValidActiveSince",
			"/api/v1/sessions",
			"?active_since=2024-06-01T10:00:00Z",
			http.StatusOK,
		},
		{
			"Sessions_ValidActiveSinceNano",
			"/api/v1/sessions",
			"?active_since=2024-06-01T10:00:00.123456789Z",
			http.StatusOK,
		},
		{
			"Analytics_InvalidActiveSince",
			basePath + "summary",
			"?from=2024-06-01&to=2024-06-03&active_since=not-a-timestamp",
			http.StatusBadRequest,
		},
		{
			"Analytics_ValidActiveSince",
			basePath + "summary",
			"?from=2024-06-01&to=2024-06-03&active_since=2024-06-01T00:00:00Z",
			http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t, tt.path+tt.query)
			assertStatus(t, w, tt.status)
		})
	}
}

func TestAnalyticsActivity(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	tests := []struct {
		name        string
		granularity string
		wantStatus  int
	}{
		{"DayGranularity", "day", http.StatusOK},
		{"WeekGranularity", "week", http.StatusOK},
		{"DefaultGranularity", "", http.StatusOK},
		{"InvalidGranularity", "hour", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := make(map[string]string)
			if tt.granularity != "" {
				params["granularity"] = tt.granularity
			}
			w := te.get(t, buildURLWithRange("activity", params))
			assertStatus(t, w, tt.wantStatus)

			if tt.wantStatus == http.StatusOK {
				resp := decode[db.ActivityResponse](t, w)
				expectedGran := tt.granularity
				if expectedGran == "" {
					expectedGran = "day" // default
				}
				if resp.Granularity != expectedGran {
					t.Errorf("Granularity = %q, want %q", resp.Granularity, expectedGran)
				}
				if expectedGran == "day" && len(resp.Series) == 0 {
					t.Fatal("expected non-empty series")
				}
			}
		})
	}
}

func TestAnalyticsHeatmap(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	tests := []struct {
		name        string
		metric      string
		wantStatus  int
		wantEntries int
	}{
		{"MessageMetric", "messages", http.StatusOK, 3},
		{"SessionMetric", "sessions", http.StatusOK, -1}, // -1 means don't check exactly
		{"DefaultMetric", "", http.StatusOK, -1},
		{"InvalidMetric", "bytes", http.StatusBadRequest, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := make(map[string]string)
			if tt.metric != "" {
				params["metric"] = tt.metric
			}
			w := te.get(t, buildURLWithRange("heatmap", params))
			assertStatus(t, w, tt.wantStatus)

			if tt.wantStatus == http.StatusOK {
				resp := decode[db.HeatmapResponse](t, w)
				expectedMetric := tt.metric
				if expectedMetric == "" {
					expectedMetric = "messages" // default
				}
				if resp.Metric != expectedMetric {
					t.Errorf("Metric = %q, want %q", resp.Metric, expectedMetric)
				}
				if tt.wantEntries >= 0 && len(resp.Entries) != tt.wantEntries {
					t.Errorf("len(Entries) = %d, want %d", len(resp.Entries), tt.wantEntries)
				}
			}
		})
	}
}

func TestAnalyticsProjects(t *testing.T) {
	te := setup(t)
	stats := seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("projects", nil))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ProjectsAnalyticsResponse](t, w)
		if len(resp.Projects) != stats.ActiveProjects {
			t.Fatalf("len(Projects) = %d, want %d", len(resp.Projects), stats.ActiveProjects)
		}

		total := 0
		for _, p := range resp.Projects {
			total += p.Messages
		}
		if total != stats.TotalMessages {
			t.Errorf("total messages = %d, want %d", total, stats.TotalMessages)
		}
	})

	t.Run("MachineFilter", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("projects", map[string]string{"machine": "nonexistent"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ProjectsAnalyticsResponse](t, w)
		if len(resp.Projects) != 0 {
			t.Errorf("len(Projects) = %d, want 0", len(resp.Projects))
		}
	})
}

func TestAnalyticsHourOfWeek(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("hour-of-week", map[string]string{"timezone": "UTC"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.HourOfWeekResponse](t, w)
		if len(resp.Cells) != 168 {
			t.Errorf("len(Cells) = %d, want 168", len(resp.Cells))
		}
	})

	t.Run("DefaultParams", func(t *testing.T) {
		w := te.get(t, buildURL("hour-of-week", nil))
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsSessionShape(t *testing.T) {
	te := setup(t)
	stats := seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("sessions", map[string]string{"timezone": "UTC"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.SessionShapeResponse](t, w)
		if resp.Count != stats.TotalSessions {
			t.Errorf("Count = %d, want %d", resp.Count, stats.TotalSessions)
		}
	})

	t.Run("DefaultParams", func(t *testing.T) {
		w := te.get(t, buildURL("sessions", nil))
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsVelocity(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("velocity", map[string]string{"timezone": "UTC"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.VelocityResponse](t, w)
		if len(resp.ByAgent) == 0 {
			t.Error("expected non-empty ByAgent")
		}
	})

	t.Run("DefaultParams", func(t *testing.T) {
		w := te.get(t, buildURL("velocity", nil))
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsTools(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("tools", map[string]string{"timezone": "UTC"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ToolsAnalyticsResponse](t, w)
		if resp.TotalCalls == 0 {
			t.Error("expected non-zero TotalCalls")
		}
		if len(resp.ByCategory) == 0 {
			t.Error("expected non-empty ByCategory")
		}
		if len(resp.ByAgent) == 0 {
			t.Error("expected non-empty ByAgent")
		}
	})

	t.Run("WithProjectFilter", func(t *testing.T) {
		w := te.get(t, buildURLWithRange("tools", map[string]string{"project": "alpha", "timezone": "UTC"}))
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ToolsAnalyticsResponse](t, w)
		if resp.TotalCalls == 0 {
			t.Error("expected non-zero TotalCalls for alpha")
		}
	})

	t.Run("InvalidTimezone", func(t *testing.T) {
		w := te.get(t, buildURL("tools", map[string]string{"timezone": "Fake/Zone"}))
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("DefaultDateRange", func(t *testing.T) {
		w := te.get(t, buildURL("tools", nil))
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsTopSessions(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	tests := []struct {
		name       string
		metric     string
		project    string
		wantStatus int
	}{
		{"ByMessages", "messages", "", http.StatusOK},
		{"ByDuration", "duration", "", http.StatusOK},
		{"DefaultMetric", "", "", http.StatusOK},
		{"InvalidMetric", "bytes", "", http.StatusBadRequest},
		{"WithProjectFilter", "", "alpha", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := make(map[string]string)
			if tt.metric != "" {
				params["metric"] = tt.metric
			}
			if tt.project != "" {
				params["project"] = tt.project
			}
			if tt.wantStatus == http.StatusOK {
				params["timezone"] = "UTC"
			}

			w := te.get(t, buildURLWithRange("top-sessions", params))
			assertStatus(t, w, tt.wantStatus)

			if tt.wantStatus == http.StatusOK {
				resp := decode[db.TopSessionsResponse](t, w)
				expectedMetric := tt.metric
				if expectedMetric == "" {
					expectedMetric = "messages"
				}
				if resp.Metric != expectedMetric {
					t.Errorf("Metric = %q, want %q", resp.Metric, expectedMetric)
				}
				if tt.project == "" && len(resp.Sessions) == 0 {
					t.Error("expected non-empty sessions")
				}
				if tt.project != "" {
					for _, s := range resp.Sessions {
						if s.Project != tt.project {
							t.Errorf("session project = %q, want %q", s.Project, tt.project)
						}
					}
				}
			}
		})
	}

	t.Run("DefaultDateRange", func(t *testing.T) {
		w := te.get(t, buildURL("top-sessions", nil))
		assertStatus(t, w, http.StatusOK)
	})
}

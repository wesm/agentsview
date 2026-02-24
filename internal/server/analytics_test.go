package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/dbtest"
)

// Analytics date range used across analytics tests.
const analyticsRange = "?from=2024-06-01&to=2024-06-03"

// seedAnalyticsEnv populates the test env with sessions and
// messages suitable for analytics endpoint tests. Some messages
// include tool_calls for tool analytics testing.
func seedAnalyticsEnv(t *testing.T, te *testEnv) {
	t.Helper()

	type entry struct {
		id, project, agent, started string
		msgs                        int
	}
	for _, s := range []entry{
		{"a1", "alpha", "claude", "2024-06-01T09:00:00Z", 10},
		{"a2", "alpha", "codex", "2024-06-01T14:00:00Z", 20},
		{"b1", "beta", "claude", "2024-06-02T10:00:00Z", 30},
	} {
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
}

func TestAnalyticsSummary(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/summary"+analyticsRange+"&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.AnalyticsSummary](t, w)
		if resp.TotalSessions != 3 {
			t.Errorf("TotalSessions = %d, want 3",
				resp.TotalSessions)
		}
		if resp.TotalMessages != 60 {
			t.Errorf("TotalMessages = %d, want 60",
				resp.TotalMessages)
		}
		if resp.ActiveProjects != 2 {
			t.Errorf("ActiveProjects = %d, want 2",
				resp.ActiveProjects)
		}
	})

	t.Run("DefaultDateRange", func(t *testing.T) {
		w := te.get(t, "/api/v1/analytics/summary")
		assertStatus(t, w, http.StatusOK)
		// Should not error — defaults to last 30 days
	})

	t.Run("NonUTCTimezone", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/summary"+
				analyticsRange+"&timezone=America/New_York")
		assertStatus(t, w, http.StatusOK)
	})

	t.Run("InvalidTimezone", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/summary?timezone=Fake/Zone")
		assertStatus(t, w, http.StatusBadRequest)
	})
}

func TestAnalyticsSummary_DateValidation(t *testing.T) {
	te := setup(t)

	tests := []struct {
		name   string
		query  string
		status int
	}{
		{
			"InvalidFromFormat",
			"?from=not-a-date&to=2024-06-03",
			http.StatusBadRequest,
		},
		{
			"InvalidToFormat",
			"?from=2024-06-01&to=06-03-2024",
			http.StatusBadRequest,
		},
		{
			"FromAfterTo",
			"?from=2024-07-01&to=2024-06-01",
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := te.get(t,
				"/api/v1/analytics/summary"+tt.query)
			assertStatus(t, w, tt.status)
		})
	}
}

func TestAnalyticsErrorRedaction(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	// Valid request should succeed
	w := te.get(t,
		"/api/v1/analytics/summary"+analyticsRange)
	assertStatus(t, w, http.StatusOK)

	// Force a DB error by closing the database
	te.db.Close()

	endpoints := []string{
		"/api/v1/analytics/summary" + analyticsRange,
		"/api/v1/analytics/activity" + analyticsRange,
		"/api/v1/analytics/heatmap" + analyticsRange,
		"/api/v1/analytics/projects" + analyticsRange,
		"/api/v1/analytics/hour-of-week" + analyticsRange,
		"/api/v1/analytics/sessions" + analyticsRange,
		"/api/v1/analytics/velocity" + analyticsRange,
		"/api/v1/analytics/tools" + analyticsRange,
		"/api/v1/analytics/top-sessions" + analyticsRange,
	}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := te.get(t, ep)
			assertStatus(t, w, http.StatusInternalServerError)
			body := w.Body.String()
			if strings.Contains(body, "sql") ||
				strings.Contains(body, "database") {
				t.Errorf(
					"response exposes internal error: %s",
					body,
				)
			}
		})
	}
}

func TestSessionsDateValidation(t *testing.T) {
	te := setup(t)

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
			w := te.get(t,
				"/api/v1/sessions"+tt.query)
			assertStatus(t, w, tt.status)
		})
	}
}

func TestAnalyticsActivity(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("DayGranularity", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/activity"+analyticsRange+"&granularity=day")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ActivityResponse](t, w)
		if resp.Granularity != "day" {
			t.Errorf("Granularity = %q, want day",
				resp.Granularity)
		}
		if len(resp.Series) == 0 {
			t.Fatal("expected non-empty series")
		}
	})

	t.Run("WeekGranularity", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/activity"+analyticsRange+"&granularity=week")
		assertStatus(t, w, http.StatusOK)
	})

	t.Run("DefaultGranularity", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/activity"+analyticsRange)
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ActivityResponse](t, w)
		if resp.Granularity != "day" {
			t.Errorf("default granularity = %q, want day",
				resp.Granularity)
		}
	})

	t.Run("InvalidGranularity", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/activity?granularity=hour")
		assertStatus(t, w, http.StatusBadRequest)
	})
}

func TestAnalyticsHeatmap(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("MessageMetric", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/heatmap"+analyticsRange+"&metric=messages")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.HeatmapResponse](t, w)
		if resp.Metric != "messages" {
			t.Errorf("Metric = %q, want messages", resp.Metric)
		}
		if len(resp.Entries) != 3 {
			t.Errorf("len(Entries) = %d, want 3",
				len(resp.Entries))
		}
	})

	t.Run("SessionMetric", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/heatmap"+analyticsRange+"&metric=sessions")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.HeatmapResponse](t, w)
		if resp.Metric != "sessions" {
			t.Errorf("Metric = %q, want sessions", resp.Metric)
		}
	})

	t.Run("DefaultMetric", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/heatmap"+analyticsRange)
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.HeatmapResponse](t, w)
		if resp.Metric != "messages" {
			t.Errorf("default metric = %q, want messages",
				resp.Metric)
		}
	})

	t.Run("InvalidMetric", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/heatmap?metric=bytes")
		assertStatus(t, w, http.StatusBadRequest)
	})
}

func TestAnalyticsProjects(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/projects"+analyticsRange)
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ProjectsAnalyticsResponse](t, w)
		if len(resp.Projects) != 2 {
			t.Fatalf("len(Projects) = %d, want 2",
				len(resp.Projects))
		}
		// Sorted by messages desc: beta (30) > alpha (30)
		// Both are 30 — either order is fine, just check counts
		total := 0
		for _, p := range resp.Projects {
			total += p.Messages
		}
		if total != 60 {
			t.Errorf("total messages = %d, want 60", total)
		}
	})

	t.Run("MachineFilter", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/projects"+analyticsRange+"&machine=nonexistent")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ProjectsAnalyticsResponse](t, w)
		if len(resp.Projects) != 0 {
			t.Errorf("len(Projects) = %d, want 0",
				len(resp.Projects))
		}
	})
}

func TestAnalyticsHourOfWeek(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/hour-of-week"+analyticsRange+
				"&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.HourOfWeekResponse](t, w)
		if len(resp.Cells) != 168 {
			t.Errorf("len(Cells) = %d, want 168",
				len(resp.Cells))
		}
	})

	t.Run("DefaultParams", func(t *testing.T) {
		w := te.get(t, "/api/v1/analytics/hour-of-week")
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsSessionShape(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/sessions"+analyticsRange+
				"&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.SessionShapeResponse](t, w)
		if resp.Count != 3 {
			t.Errorf("Count = %d, want 3", resp.Count)
		}
	})

	t.Run("DefaultParams", func(t *testing.T) {
		w := te.get(t, "/api/v1/analytics/sessions")
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsVelocity(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/velocity"+analyticsRange+
				"&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.VelocityResponse](t, w)
		if len(resp.ByAgent) == 0 {
			t.Error("expected non-empty ByAgent")
		}
	})

	t.Run("DefaultParams", func(t *testing.T) {
		w := te.get(t, "/api/v1/analytics/velocity")
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsTools(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("OK", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/tools"+analyticsRange+
				"&timezone=UTC")
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
		w := te.get(t,
			"/api/v1/analytics/tools"+analyticsRange+
				"&project=alpha&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.ToolsAnalyticsResponse](t, w)
		if resp.TotalCalls == 0 {
			t.Error("expected non-zero TotalCalls for alpha")
		}
	})

	t.Run("InvalidTimezone", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/tools?timezone=Fake/Zone")
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("DefaultDateRange", func(t *testing.T) {
		w := te.get(t, "/api/v1/analytics/tools")
		assertStatus(t, w, http.StatusOK)
	})
}

func TestAnalyticsTopSessions(t *testing.T) {
	te := setup(t)
	seedAnalyticsEnv(t, te)

	t.Run("ByMessages", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/top-sessions"+analyticsRange+
				"&metric=messages&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.TopSessionsResponse](t, w)
		if resp.Metric != "messages" {
			t.Errorf("Metric = %q, want messages",
				resp.Metric)
		}
		if len(resp.Sessions) == 0 {
			t.Error("expected non-empty sessions")
		}
	})

	t.Run("ByDuration", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/top-sessions"+analyticsRange+
				"&metric=duration&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.TopSessionsResponse](t, w)
		if resp.Metric != "duration" {
			t.Errorf("Metric = %q, want duration",
				resp.Metric)
		}
	})

	t.Run("DefaultMetric", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/top-sessions"+analyticsRange)
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.TopSessionsResponse](t, w)
		if resp.Metric != "messages" {
			t.Errorf("default metric = %q, want messages",
				resp.Metric)
		}
	})

	t.Run("InvalidMetric", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/top-sessions?metric=bytes")
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("WithProjectFilter", func(t *testing.T) {
		w := te.get(t,
			"/api/v1/analytics/top-sessions"+analyticsRange+
				"&project=alpha&timezone=UTC")
		assertStatus(t, w, http.StatusOK)

		resp := decode[db.TopSessionsResponse](t, w)
		for _, s := range resp.Sessions {
			if s.Project != "alpha" {
				t.Errorf(
					"session project = %q, want alpha",
					s.Project,
				)
			}
		}
	})

	t.Run("DefaultDateRange", func(t *testing.T) {
		w := te.get(t, "/api/v1/analytics/top-sessions")
		assertStatus(t, w, http.StatusOK)
	})
}

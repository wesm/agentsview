package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func seedAnalyticsData(t *testing.T, d *DB) {
	t.Helper()

	// Project A: 3 sessions across 2 days, mixed agents
	insertSession(t, d, "a1", "project-alpha", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T09:00:00Z")
		s.EndedAt = Ptr("2024-06-01T10:00:00Z")
		s.MessageCount = 10
		s.Agent = "claude"
	})
	insertSession(t, d, "a2", "project-alpha", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T14:00:00Z")
		s.EndedAt = Ptr("2024-06-01T15:00:00Z")
		s.MessageCount = 20
		s.Agent = "codex"
	})
	insertSession(t, d, "a3", "project-alpha", func(s *Session) {
		s.StartedAt = Ptr("2024-06-03T09:00:00Z")
		s.EndedAt = Ptr("2024-06-03T10:00:00Z")
		s.MessageCount = 5
		s.Agent = "claude"
	})

	// Project B: 2 sessions on 1 day
	insertSession(t, d, "b1", "project-beta", func(s *Session) {
		s.StartedAt = Ptr("2024-06-02T10:00:00Z")
		s.EndedAt = Ptr("2024-06-02T11:00:00Z")
		s.MessageCount = 30
		s.Agent = "claude"
	})
	insertSession(t, d, "b2", "project-beta", func(s *Session) {
		s.StartedAt = Ptr("2024-06-02T15:00:00Z")
		s.EndedAt = Ptr("2024-06-02T16:00:00Z")
		s.MessageCount = 15
		s.Agent = "claude"
	})

	// Insert messages for each session
	for _, sess := range []struct {
		id    string
		count int
	}{
		{"a1", 10}, {"a2", 20}, {"a3", 5},
		{"b1", 30}, {"b2", 15},
	} {
		msgs := make([]Message, sess.count)
		for i := range sess.count {
			role := "user"
			if i%2 == 1 {
				role = "assistant"
			}
			msgs[i] = Message{
				SessionID:     sess.id,
				Ordinal:       i,
				Role:          role,
				Content:       fmt.Sprintf("msg %d", i),
				ContentLength: 5,
				Timestamp:     "2024-06-01T10:00:00Z",
			}
		}
		insertMessages(t, d, msgs...)
	}
}

func baseFilter() AnalyticsFilter {
	return AnalyticsFilter{
		From:     "2024-06-01",
		To:       "2024-06-03",
		Timezone: "UTC",
	}
}

func emptyFilter() AnalyticsFilter {
	return AnalyticsFilter{
		From:     "2020-01-01",
		To:       "2020-01-02",
		Timezone: "UTC",
	}
}

func mustSummary(
	t *testing.T, d *DB, ctx context.Context, f AnalyticsFilter,
) AnalyticsSummary {
	t.Helper()
	s, err := d.GetAnalyticsSummary(ctx, f)
	if err != nil {
		t.Fatalf("GetAnalyticsSummary: %v", err)
	}
	return s
}

func mustActivity(
	t *testing.T, d *DB, ctx context.Context,
	f AnalyticsFilter, gran string,
) ActivityResponse {
	t.Helper()
	r, err := d.GetAnalyticsActivity(ctx, f, gran)
	if err != nil {
		t.Fatalf("GetAnalyticsActivity: %v", err)
	}
	return r
}

func mustHeatmap(
	t *testing.T, d *DB, ctx context.Context,
	f AnalyticsFilter, metric string,
) HeatmapResponse {
	t.Helper()
	r, err := d.GetAnalyticsHeatmap(ctx, f, metric)
	if err != nil {
		t.Fatalf("GetAnalyticsHeatmap: %v", err)
	}
	return r
}

func mustProjects(
	t *testing.T, d *DB, ctx context.Context,
	f AnalyticsFilter,
) ProjectsAnalyticsResponse {
	t.Helper()
	r, err := d.GetAnalyticsProjects(ctx, f)
	if err != nil {
		t.Fatalf("GetAnalyticsProjects: %v", err)
	}
	return r
}

func TestGetAnalyticsSummary(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	t.Run("EmptyDB", func(t *testing.T) {
		s := mustSummary(t, d, ctx, baseFilter())
		if s.TotalSessions != 0 {
			t.Errorf("TotalSessions = %d, want 0", s.TotalSessions)
		}
	})

	seedAnalyticsData(t, d)

	t.Run("FullRange", func(t *testing.T) {
		s := mustSummary(t, d, ctx, baseFilter())
		if s.TotalSessions != 5 {
			t.Errorf("TotalSessions = %d, want 5", s.TotalSessions)
		}
		if s.TotalMessages != 80 {
			t.Errorf("TotalMessages = %d, want 80", s.TotalMessages)
		}
		if s.ActiveProjects != 2 {
			t.Errorf("ActiveProjects = %d, want 2", s.ActiveProjects)
		}
		if s.ActiveDays != 3 {
			t.Errorf("ActiveDays = %d, want 3", s.ActiveDays)
		}
		if s.MostActive != "project-beta" {
			t.Errorf("MostActive = %q, want project-beta", s.MostActive)
		}

		// Sorted message counts: [5, 10, 15, 20, 30]
		if s.MedianMessages != 15 {
			t.Errorf("MedianMessages = %d, want 15", s.MedianMessages)
		}
		// P90 index = int(5*0.9) = 4 → value 30
		if s.P90Messages != 30 {
			t.Errorf("P90Messages = %d, want 30", s.P90Messages)
		}

		if s.Agents["claude"] == nil {
			t.Fatal("expected claude agent entry")
		}
		if s.Agents["claude"].Sessions != 4 {
			t.Errorf("claude sessions = %d, want 4",
				s.Agents["claude"].Sessions)
		}
		if s.Agents["codex"] == nil {
			t.Fatal("expected codex agent entry")
		}
		if s.Agents["codex"].Sessions != 1 {
			t.Errorf("codex sessions = %d, want 1",
				s.Agents["codex"].Sessions)
		}
	})

	t.Run("DateSubset", func(t *testing.T) {
		f := AnalyticsFilter{
			From:     "2024-06-01",
			To:       "2024-06-01",
			Timezone: "UTC",
		}
		s := mustSummary(t, d, ctx, f)
		if s.TotalSessions != 2 {
			t.Errorf("TotalSessions = %d, want 2", s.TotalSessions)
		}
	})

	t.Run("MachineFilter", func(t *testing.T) {
		f := baseFilter()
		f.Machine = "nonexistent"
		s := mustSummary(t, d, ctx, f)
		if s.TotalSessions != 0 {
			t.Errorf("TotalSessions = %d, want 0", s.TotalSessions)
		}
	})

	t.Run("EmptyDateRange", func(t *testing.T) {
		s := mustSummary(t, d, ctx, emptyFilter())
		if s.TotalSessions != 0 {
			t.Errorf("TotalSessions = %d, want 0", s.TotalSessions)
		}
	})
}

func TestGetAnalyticsActivity(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedAnalyticsData(t, d)

	t.Run("DayGranularity", func(t *testing.T) {
		resp := mustActivity(t, d, ctx, baseFilter(), "day")
		if resp.Granularity != "day" {
			t.Errorf("Granularity = %q, want day", resp.Granularity)
		}
		if len(resp.Series) != 3 {
			t.Fatalf("len(Series) = %d, want 3", len(resp.Series))
		}
		// Day 1: 2 sessions (a1, a2)
		if resp.Series[0].Sessions != 2 {
			t.Errorf("Day1 sessions = %d, want 2",
				resp.Series[0].Sessions)
		}
	})

	t.Run("WeekGranularity", func(t *testing.T) {
		resp := mustActivity(t, d, ctx, baseFilter(), "week")
		// 2024-06-01 is Saturday, 2024-06-03 is Monday
		// So we expect 2 weeks: week of May 27 and week of Jun 3
		if len(resp.Series) != 2 {
			t.Errorf("len(Series) = %d, want 2", len(resp.Series))
		}
	})

	t.Run("MonthGranularity", func(t *testing.T) {
		resp := mustActivity(t, d, ctx, baseFilter(), "month")
		if len(resp.Series) != 1 {
			t.Errorf("len(Series) = %d, want 1", len(resp.Series))
		}
	})

	t.Run("HasRoleCounts", func(t *testing.T) {
		resp := mustActivity(t, d, ctx, baseFilter(), "day")
		totalUser := 0
		totalAsst := 0
		for _, e := range resp.Series {
			totalUser += e.UserMessages
			totalAsst += e.AssistantMessages
		}
		if totalUser == 0 {
			t.Error("expected non-zero user messages")
		}
		if totalAsst == 0 {
			t.Error("expected non-zero assistant messages")
		}
	})
}

func TestGetAnalyticsHeatmap(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedAnalyticsData(t, d)

	t.Run("MessageMetric", func(t *testing.T) {
		resp := mustHeatmap(t, d, ctx, baseFilter(), "messages")
		if resp.Metric != "messages" {
			t.Errorf("Metric = %q, want messages", resp.Metric)
		}
		// 3 days in range: Jun 1, 2, 3
		if len(resp.Entries) != 3 {
			t.Fatalf("len(Entries) = %d, want 3", len(resp.Entries))
		}
		// Jun 1: 10+20=30, Jun 2: 30+15=45, Jun 3: 5
		if resp.Entries[0].Value != 30 {
			t.Errorf("Jun1 value = %d, want 30", resp.Entries[0].Value)
		}
		if resp.Entries[1].Value != 45 {
			t.Errorf("Jun2 value = %d, want 45", resp.Entries[1].Value)
		}
		if resp.Entries[2].Value != 5 {
			t.Errorf("Jun3 value = %d, want 5", resp.Entries[2].Value)
		}
	})

	t.Run("SessionMetric", func(t *testing.T) {
		resp := mustHeatmap(t, d, ctx, baseFilter(), "sessions")
		if resp.Metric != "sessions" {
			t.Errorf("Metric = %q, want sessions", resp.Metric)
		}
		// Jun 1: 2, Jun 2: 2, Jun 3: 1
		if resp.Entries[0].Value != 2 {
			t.Errorf("Jun1 sessions = %d, want 2",
				resp.Entries[0].Value)
		}
	})

	t.Run("LevelsAssigned", func(t *testing.T) {
		resp := mustHeatmap(t, d, ctx, baseFilter(), "messages")
		// All entries should have levels 0-4
		for _, e := range resp.Entries {
			if e.Level < 0 || e.Level > 4 {
				t.Errorf("date %s level = %d, want 0-4",
					e.Date, e.Level)
			}
		}
	})

	t.Run("EmptyRange", func(t *testing.T) {
		f := emptyFilter()
		f.To = "2020-01-03"
		resp := mustHeatmap(t, d, ctx, f, "messages")
		if len(resp.Entries) != 3 {
			t.Fatalf("len(Entries) = %d, want 3", len(resp.Entries))
		}
		for _, e := range resp.Entries {
			if e.Value != 0 {
				t.Errorf("date %s value = %d, want 0", e.Date, e.Value)
			}
			if e.Level != 0 {
				t.Errorf("date %s level = %d, want 0", e.Date, e.Level)
			}
		}
	})
}

func TestGetAnalyticsProjects(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedAnalyticsData(t, d)

	t.Run("FullRange", func(t *testing.T) {
		resp := mustProjects(t, d, ctx, baseFilter())
		if len(resp.Projects) != 2 {
			t.Fatalf("len(Projects) = %d, want 2", len(resp.Projects))
		}
		// Sorted by message count desc: beta (45) > alpha (35)
		if resp.Projects[0].Name != "project-beta" {
			t.Errorf("first project = %q, want project-beta",
				resp.Projects[0].Name)
		}
		if resp.Projects[0].Messages != 45 {
			t.Errorf("beta messages = %d, want 45",
				resp.Projects[0].Messages)
		}
		if resp.Projects[1].Name != "project-alpha" {
			t.Errorf("second project = %q, want project-alpha",
				resp.Projects[1].Name)
		}
		if resp.Projects[1].Sessions != 3 {
			t.Errorf("alpha sessions = %d, want 3",
				resp.Projects[1].Sessions)
		}
	})

	t.Run("AgentBreakdown", func(t *testing.T) {
		resp := mustProjects(t, d, ctx, baseFilter())
		alpha := resp.Projects[1]
		if alpha.Agents["claude"] != 2 {
			t.Errorf("alpha claude = %d, want 2",
				alpha.Agents["claude"])
		}
		if alpha.Agents["codex"] != 1 {
			t.Errorf("alpha codex = %d, want 1",
				alpha.Agents["codex"])
		}
	})

	t.Run("MedianMessages", func(t *testing.T) {
		resp := mustProjects(t, d, ctx, baseFilter())
		// Alpha counts sorted: [5, 10, 20], median = 10
		alpha := resp.Projects[1]
		if alpha.MedianMessages != 10 {
			t.Errorf("alpha median = %d, want 10",
				alpha.MedianMessages)
		}
	})

	t.Run("EmptyRange", func(t *testing.T) {
		resp := mustProjects(t, d, ctx, emptyFilter())
		if len(resp.Projects) != 0 {
			t.Errorf("len(Projects) = %d, want 0", len(resp.Projects))
		}
	})
}

func TestMedianInt(t *testing.T) {
	tests := []struct {
		name   string
		sorted []int
		want   int
	}{
		{"Empty", []int{}, 0},
		{"Single", []int{5}, 5},
		{"OddCount", []int{1, 3, 7}, 3},
		{"EvenCount", []int{1, 3, 7, 9}, 5},
		{"EvenCountTwo", []int{10, 20}, 15},
		{"EvenCountFour", []int{2, 4, 6, 8}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianInt(tt.sorted, len(tt.sorted))
			if got != tt.want {
				t.Errorf(
					"medianInt(%v) = %d, want %d",
					tt.sorted, got, tt.want,
				)
			}
		})
	}
}

func TestLocalDate(t *testing.T) {
	utc := time.UTC

	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"RFC3339", "2024-06-01T15:00:00Z", "2024-06-01"},
		{"RFC3339Nano", "2024-06-01T15:00:00.123Z", "2024-06-01"},
		{"NoFraction", "2024-06-01T15:00:00Z", "2024-06-01"},
		{"Fallback10Char", "2024-06-01", "2024-06-01"},
		{"Short", "2024", ""},
		{"Empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := localDate(tt.ts, utc)
			if got != tt.want {
				t.Errorf(
					"localDate(%q) = %q, want %q",
					tt.ts, got, tt.want,
				)
			}
		})
	}
}

func TestMostActiveTieBreak(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	// Two projects with equal message counts
	insertSession(t, d, "t1", "zebra", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T09:00:00Z")
		s.MessageCount = 20
		s.Agent = "claude"
	})
	insertSession(t, d, "t2", "alpha", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T10:00:00Z")
		s.MessageCount = 20
		s.Agent = "claude"
	})

	f := AnalyticsFilter{
		From:     "2024-06-01",
		To:       "2024-06-01",
		Timezone: "UTC",
	}
	s := mustSummary(t, d, ctx, f)

	// Alphabetically, "alpha" < "zebra"
	if s.MostActive != "alpha" {
		t.Errorf(
			"MostActive = %q, want alphabetically first (alpha)",
			s.MostActive,
		)
	}
}

func TestEvenCountMedianInSummary(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	// 4 sessions: message counts [5, 10, 20, 30]
	for i, mc := range []int{10, 30, 5, 20} {
		id := fmt.Sprintf("e%d", i)
		insertSession(t, d, id, "proj", func(s *Session) {
			ts := fmt.Sprintf("2024-06-01T%02d:00:00Z", i+9)
			s.StartedAt = &ts
			s.MessageCount = mc
			s.Agent = "claude"
		})
	}

	f := AnalyticsFilter{
		From:     "2024-06-01",
		To:       "2024-06-01",
		Timezone: "UTC",
	}
	s := mustSummary(t, d, ctx, f)

	// Sorted: [5, 10, 20, 30] → median = (10+20)/2 = 15
	if s.MedianMessages != 15 {
		t.Errorf(
			"MedianMessages = %d, want 15", s.MedianMessages,
		)
	}
}

func TestAnalyticsTimezone(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	// Session at 2024-06-01T23:00:00Z = 2024-06-02 in UTC+5
	insertSession(t, d, "tz1", "tz-project", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T23:00:00Z")
		s.MessageCount = 10
		s.Agent = "claude"
	})
	insertMessages(t, d, userMsg("tz1", 0, "late night"))

	t.Run("UTCBucket", func(t *testing.T) {
		f := AnalyticsFilter{
			From:     "2024-06-01",
			To:       "2024-06-02",
			Timezone: "UTC",
		}
		resp := mustHeatmap(t, d, ctx, f, "messages")
		// In UTC, this is Jun 1
		if resp.Entries[0].Value != 10 {
			t.Errorf("Jun1 UTC value = %d, want 10",
				resp.Entries[0].Value)
		}
		if resp.Entries[1].Value != 0 {
			t.Errorf("Jun2 UTC value = %d, want 0",
				resp.Entries[1].Value)
		}
	})

	t.Run("PlusFiveBucket", func(t *testing.T) {
		f := AnalyticsFilter{
			From:     "2024-06-01",
			To:       "2024-06-02",
			Timezone: "Asia/Karachi", // UTC+5
		}
		resp := mustHeatmap(t, d, ctx, f, "messages")
		// In UTC+5, 23:00Z = 04:00 Jun 2
		if resp.Entries[0].Value != 0 {
			t.Errorf("Jun1 PKT value = %d, want 0",
				resp.Entries[0].Value)
		}
		if resp.Entries[1].Value != 10 {
			t.Errorf("Jun2 PKT value = %d, want 10",
				resp.Entries[1].Value)
		}
	})
}

func TestAnalyticsCanceledContext(t *testing.T) {
	d := testDB(t)
	ctx := canceledCtx()

	f := baseFilter()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Summary", func() error {
			_, err := d.GetAnalyticsSummary(ctx, f)
			return err
		}},
		{"Activity", func() error {
			_, err := d.GetAnalyticsActivity(ctx, f, "day")
			return err
		}},
		{"Heatmap", func() error {
			_, err := d.GetAnalyticsHeatmap(ctx, f, "messages")
			return err
		}},
		{"Projects", func() error {
			_, err := d.GetAnalyticsProjects(ctx, f)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireCanceledErr(t, tt.fn())
		})
	}
}

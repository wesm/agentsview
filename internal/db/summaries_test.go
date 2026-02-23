package db

import (
	"context"
	"testing"
)

func ptr(s string) *string { return &s }

func TestSummaries_InsertAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	s := Summary{
		Type:    "daily_activity",
		Date:    "2025-01-15",
		Project: ptr("my-app"),
		Agent:   "claude",
		Model:   ptr("claude-sonnet-4-20250514"),
		Prompt:  ptr("What happened today?"),
		Content: "# Summary\nStuff happened.",
	}

	id, err := d.InsertSummary(s)
	if err != nil {
		t.Fatalf("InsertSummary: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	got, err := d.GetSummary(ctx, id)
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if got == nil {
		t.Fatal("expected summary, got nil")
	}
	if got.Type != "daily_activity" {
		t.Errorf("type = %q, want daily_activity", got.Type)
	}
	if got.Date != "2025-01-15" {
		t.Errorf("date = %q, want 2025-01-15", got.Date)
	}
	if got.Project == nil || *got.Project != "my-app" {
		t.Errorf("project = %v, want my-app", got.Project)
	}
	if got.Content != "# Summary\nStuff happened." {
		t.Errorf("content = %q", got.Content)
	}
	if got.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}
}

func TestSummaries_GetNonexistent(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	got, err := d.GetSummary(ctx, 99999)
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSummaries_ListWithFilters(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	summaries := []Summary{
		{
			Type: "daily_activity", Date: "2025-01-15",
			Project: ptr("app-a"), Agent: "claude",
			Content: "Day 1 app-a",
		},
		{
			Type: "daily_activity", Date: "2025-01-15",
			Project: ptr("app-b"), Agent: "claude",
			Content: "Day 1 app-b",
		},
		{
			Type: "agent_analysis", Date: "2025-01-15",
			Agent: "claude", Content: "Analysis",
		},
		{
			Type: "daily_activity", Date: "2025-01-16",
			Project: ptr("app-a"), Agent: "claude",
			Content: "Day 2 app-a",
		},
	}
	for _, s := range summaries {
		if _, err := d.InsertSummary(s); err != nil {
			t.Fatalf("InsertSummary: %v", err)
		}
	}

	tests := []struct {
		name  string
		f     SummaryFilter
		count int
	}{
		{
			"AllSummaries",
			SummaryFilter{},
			4,
		},
		{
			"ByType",
			SummaryFilter{Type: "daily_activity"},
			3,
		},
		{
			"ByDate",
			SummaryFilter{Date: "2025-01-15"},
			3,
		},
		{
			"ByTypeAndDate",
			SummaryFilter{
				Type: "daily_activity",
				Date: "2025-01-15",
			},
			2,
		},
		{
			"ByProject",
			SummaryFilter{Project: "app-a"},
			2,
		},
		{
			"GlobalOnly",
			SummaryFilter{GlobalOnly: true},
			1,
		},
		{
			"NoMatch",
			SummaryFilter{
				Type: "daily_activity",
				Date: "2025-12-31",
			},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.ListSummaries(ctx, tt.f)
			if err != nil {
				t.Fatalf("ListSummaries: %v", err)
			}
			if len(got) != tt.count {
				t.Errorf(
					"got %d summaries, want %d",
					len(got), tt.count,
				)
			}
		})
	}
}

func TestSummaries_OrderByCreatedAtDesc(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	ids := make([]int64, 3)
	for i, content := range []string{"first", "second", "third"} {
		id, err := d.InsertSummary(Summary{
			Type: "daily_activity", Date: "2025-01-15",
			Agent: "claude", Content: content,
		})
		if err != nil {
			t.Fatalf("InsertSummary: %v", err)
		}
		ids[i] = id
	}

	got, err := d.ListSummaries(ctx, SummaryFilter{})
	if err != nil {
		t.Fatalf("ListSummaries: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d summaries, want 3", len(got))
	}
	// ORDER BY created_at DESC; when timestamps tie, higher
	// IDs (inserted later) should come first because SQLite
	// ROWID is monotonically increasing and acts as tiebreaker.
	// Verify IDs are in descending order.
	if got[0].ID != ids[2] {
		t.Errorf("first id = %d, want %d", got[0].ID, ids[2])
	}
	if got[2].ID != ids[0] {
		t.Errorf("last id = %d, want %d", got[2].ID, ids[0])
	}
}

func TestSummaries_Delete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id, err := d.InsertSummary(Summary{
		Type: "daily_activity", Date: "2025-01-15",
		Agent: "claude", Content: "to be deleted",
	})
	if err != nil {
		t.Fatalf("InsertSummary: %v", err)
	}

	if err := d.DeleteSummary(id); err != nil {
		t.Fatalf("DeleteSummary: %v", err)
	}

	got, err := d.GetSummary(ctx, id)
	if err != nil {
		t.Fatalf("GetSummary after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

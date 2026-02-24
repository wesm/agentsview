package db

import (
	"context"
	"fmt"
	"testing"
)

func ptr(s string) *string { return &s }

func TestInsights_InsertAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	s := Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
		Project:  ptr("my-app"),
		Agent:    "claude",
		Model:    ptr("claude-sonnet-4-20250514"),
		Prompt:   ptr("What happened today?"),
		Content:  "# Summary\nStuff happened.",
	}

	id, err := d.InsertInsight(s)
	if err != nil {
		t.Fatalf("InsertInsight: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	got, err := d.GetInsight(ctx, id)
	if err != nil {
		t.Fatalf("GetInsight: %v", err)
	}
	if got == nil {
		t.Fatal("expected insight, got nil")
	}
	if got.Type != "daily_activity" {
		t.Errorf("type = %q, want daily_activity", got.Type)
	}
	if got.DateFrom != "2025-01-15" {
		t.Errorf(
			"date_from = %q, want 2025-01-15",
			got.DateFrom,
		)
	}
	if got.DateTo != "2025-01-15" {
		t.Errorf(
			"date_to = %q, want 2025-01-15",
			got.DateTo,
		)
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

func TestInsights_InsertDateRange(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id, err := d.InsertInsight(Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-13",
		DateTo:   "2025-01-17",
		Agent:    "claude",
		Content:  "Weekly summary",
	})
	if err != nil {
		t.Fatalf("InsertInsight: %v", err)
	}

	got, err := d.GetInsight(ctx, id)
	if err != nil {
		t.Fatalf("GetInsight: %v", err)
	}
	if got.DateFrom != "2025-01-13" {
		t.Errorf(
			"date_from = %q, want 2025-01-13",
			got.DateFrom,
		)
	}
	if got.DateTo != "2025-01-17" {
		t.Errorf(
			"date_to = %q, want 2025-01-17",
			got.DateTo,
		)
	}
}

func TestInsights_GetNonexistent(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	got, err := d.GetInsight(ctx, 99999)
	if err != nil {
		t.Fatalf("GetInsight: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestInsights_ListWithFilters(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	entries := []Insight{
		{
			Type:     "daily_activity",
			DateFrom: "2025-01-15", DateTo: "2025-01-15",
			Project: ptr("app-a"), Agent: "claude",
			Content: "Day 1 app-a",
		},
		{
			Type:     "daily_activity",
			DateFrom: "2025-01-15", DateTo: "2025-01-15",
			Project: ptr("app-b"), Agent: "claude",
			Content: "Day 1 app-b",
		},
		{
			Type:     "agent_analysis",
			DateFrom: "2025-01-15", DateTo: "2025-01-15",
			Agent: "claude", Content: "Analysis",
		},
		{
			Type:     "daily_activity",
			DateFrom: "2025-01-16", DateTo: "2025-01-16",
			Project: ptr("app-a"), Agent: "claude",
			Content: "Day 2 app-a",
		},
	}
	for _, s := range entries {
		if _, err := d.InsertInsight(s); err != nil {
			t.Fatalf("InsertInsight: %v", err)
		}
	}

	tests := []struct {
		name  string
		f     InsightFilter
		count int
	}{
		{
			"AllInsights",
			InsightFilter{},
			4,
		},
		{
			"ByType",
			InsightFilter{Type: "daily_activity"},
			3,
		},
		{
			"ByProject",
			InsightFilter{Project: "app-a"},
			2,
		},
		{
			"GlobalOnly",
			InsightFilter{GlobalOnly: true},
			1,
		},
		{
			"NoMatch",
			InsightFilter{Type: "agent_analysis",
				Project: "nonexistent"},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.ListInsights(ctx, tt.f)
			if err != nil {
				t.Fatalf("ListInsights: %v", err)
			}
			if len(got) != tt.count {
				t.Errorf(
					"got %d insights, want %d",
					len(got), tt.count,
				)
			}
		})
	}
}

func TestInsights_OrderByCreatedAtDesc(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	ids := make([]int64, 3)
	for i, content := range []string{"first", "second", "third"} {
		id, err := d.InsertInsight(Insight{
			Type:     "daily_activity",
			DateFrom: "2025-01-15", DateTo: "2025-01-15",
			Agent: "claude", Content: content,
		})
		if err != nil {
			t.Fatalf("InsertInsight: %v", err)
		}
		ids[i] = id
	}

	got, err := d.ListInsights(ctx, InsightFilter{})
	if err != nil {
		t.Fatalf("ListInsights: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d insights, want 3", len(got))
	}
	if got[0].ID != ids[2] {
		t.Errorf("first id = %d, want %d", got[0].ID, ids[2])
	}
	if got[2].ID != ids[0] {
		t.Errorf("last id = %d, want %d", got[2].ID, ids[0])
	}
}

func TestInsights_ListCappedAt500(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	const total = 502
	for i := range total {
		_, err := d.InsertInsight(Insight{
			Type:     "daily_activity",
			DateFrom: "2025-01-15",
			DateTo:   "2025-01-15",
			Agent:    "claude",
			Content:  fmt.Sprintf("insight %d", i),
		})
		if err != nil {
			t.Fatalf("InsertInsight %d: %v", i, err)
		}
	}

	got, err := d.ListInsights(ctx, InsightFilter{})
	if err != nil {
		t.Fatalf("ListInsights: %v", err)
	}
	if len(got) != 500 {
		t.Fatalf(
			"got %d insights, want 500 (capped)",
			len(got),
		)
	}
	// Newest first: highest ID should be first.
	if got[0].ID < got[len(got)-1].ID {
		t.Error(
			"expected newest-first ordering",
		)
	}
}

func TestInsights_Delete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	id, err := d.InsertInsight(Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-15", DateTo: "2025-01-15",
		Agent: "claude", Content: "to be deleted",
	})
	if err != nil {
		t.Fatalf("InsertInsight: %v", err)
	}

	if err := d.DeleteInsight(id); err != nil {
		t.Fatalf("DeleteInsight: %v", err)
	}

	got, err := d.GetInsight(ctx, id)
	if err != nil {
		t.Fatalf("GetInsight after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

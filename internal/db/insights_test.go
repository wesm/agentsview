package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func ptr(s string) *string { return &s }

func TestInsights_InsertAndGet(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	want := &Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
		Project:  ptr("my-app"),
		Agent:    "claude",
		Model:    ptr("claude-sonnet-4-20250514"),
		Prompt:   ptr("What happened today?"),
		Content:  "# Summary\nStuff happened.",
	}

	id, err := d.InsertInsight(*want)
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

	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(Insight{}, "ID", "CreatedAt")); diff != "" {
		t.Errorf("Insight mismatch (-want +got):\n%s", diff)
	}
	if got.CreatedAt == "" {
		t.Error("expected created_at to be set")
	}
}

func TestInsights_InsertDateRange(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	want := &Insight{
		Type:     "daily_activity",
		DateFrom: "2025-01-13",
		DateTo:   "2025-01-17",
		Agent:    "claude",
		Content:  "Weekly summary",
	}

	id, err := d.InsertInsight(*want)
	if err != nil {
		t.Fatalf("InsertInsight: %v", err)
	}

	got, err := d.GetInsight(ctx, id)
	if err != nil {
		t.Fatalf("GetInsight: %v", err)
	}
	if got == nil {
		t.Fatal("expected insight, got nil")
	}

	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(Insight{}, "ID", "CreatedAt")); diff != "" {
		t.Errorf("Insight mismatch (-want +got):\n%s", diff)
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

func TestListInsights(t *testing.T) {
	ctx := context.Background()

	seedFiltersData := func(t *testing.T, d *DB) []int64 {
		entries := []Insight{
			{Type: "daily_activity", DateFrom: "2025-01-15", DateTo: "2025-01-15", Project: ptr("app-a"), Agent: "claude", Content: "Day 1 app-a"},
			{Type: "daily_activity", DateFrom: "2025-01-15", DateTo: "2025-01-15", Project: ptr("app-b"), Agent: "claude", Content: "Day 1 app-b"},
			{Type: "agent_analysis", DateFrom: "2025-01-15", DateTo: "2025-01-15", Agent: "claude", Content: "Analysis"},
			{Type: "daily_activity", DateFrom: "2025-01-16", DateTo: "2025-01-16", Project: ptr("app-a"), Agent: "claude", Content: "Day 2 app-a"},
		}
		var ids []int64
		for _, s := range entries {
			id, err := d.InsertInsight(s)
			if err != nil {
				t.Fatalf("InsertInsight: %v", err)
			}
			ids = append(ids, id)
		}
		return ids
	}

	tests := []struct {
		name   string
		seed   func(t *testing.T, d *DB) []int64
		filter InsightFilter
		verify func(t *testing.T, got []Insight, ids []int64)
	}{
		{
			name:   "AllInsights",
			seed:   seedFiltersData,
			filter: InsightFilter{},
			verify: func(t *testing.T, got []Insight, _ []int64) {
				wantContent := []string{"Day 2 app-a", "Analysis", "Day 1 app-b", "Day 1 app-a"}
				if len(got) != len(wantContent) {
					t.Fatalf("got %d insights, want %d", len(got), len(wantContent))
				}
				for i, want := range wantContent {
					if got[i].Content != want {
						t.Errorf("got[%d].Content = %q, want %q", i, got[i].Content, want)
					}
				}
			},
		},
		{
			name:   "ByType",
			seed:   seedFiltersData,
			filter: InsightFilter{Type: "daily_activity"},
			verify: func(t *testing.T, got []Insight, _ []int64) {
				wantContent := []string{"Day 2 app-a", "Day 1 app-b", "Day 1 app-a"}
				if len(got) != len(wantContent) {
					t.Fatalf("got %d insights, want %d", len(got), len(wantContent))
				}
				for i, want := range wantContent {
					if got[i].Content != want {
						t.Errorf("got[%d].Content = %q, want %q", i, got[i].Content, want)
					}
				}
			},
		},
		{
			name:   "ByProject",
			seed:   seedFiltersData,
			filter: InsightFilter{Project: "app-a"},
			verify: func(t *testing.T, got []Insight, _ []int64) {
				wantContent := []string{"Day 2 app-a", "Day 1 app-a"}
				if len(got) != len(wantContent) {
					t.Fatalf("got %d insights, want %d", len(got), len(wantContent))
				}
				for i, want := range wantContent {
					if got[i].Content != want {
						t.Errorf("got[%d].Content = %q, want %q", i, got[i].Content, want)
					}
				}
			},
		},
		{
			name:   "GlobalOnly",
			seed:   seedFiltersData,
			filter: InsightFilter{GlobalOnly: true},
			verify: func(t *testing.T, got []Insight, _ []int64) {
				wantContent := []string{"Analysis"}
				if len(got) != len(wantContent) {
					t.Fatalf("got %d insights, want %d", len(got), len(wantContent))
				}
				for i, want := range wantContent {
					if got[i].Content != want {
						t.Errorf("got[%d].Content = %q, want %q", i, got[i].Content, want)
					}
				}
			},
		},
		{
			name:   "NoMatch",
			seed:   seedFiltersData,
			filter: InsightFilter{Type: "agent_analysis", Project: "nonexistent"},
			verify: func(t *testing.T, got []Insight, _ []int64) {
				if len(got) != 0 {
					t.Fatalf("got %d insights, want 0", len(got))
				}
			},
		},
		{
			name: "OrderByCreatedAtDesc",
			seed: func(t *testing.T, d *DB) []int64 {
				var ids []int64
				for _, content := range []string{"first", "second", "third"} {
					id, err := d.InsertInsight(Insight{
						Type:     "daily_activity",
						DateFrom: "2025-01-15", DateTo: "2025-01-15",
						Agent: "claude", Content: content,
					})
					if err != nil {
						t.Fatalf("InsertInsight: %v", err)
					}
					ids = append(ids, id)
				}
				return ids
			},
			filter: InsightFilter{},
			verify: func(t *testing.T, got []Insight, ids []int64) {
				if len(got) != 3 {
					t.Fatalf("got %d insights, want 3", len(got))
				}
				if got[0].ID != ids[2] {
					t.Errorf("first id = %d, want %d", got[0].ID, ids[2])
				}
				if got[2].ID != ids[0] {
					t.Errorf("last id = %d, want %d", got[2].ID, ids[0])
				}
			},
		},
		{
			name: "CappedAt500",
			seed: func(t *testing.T, d *DB) []int64 {
				const total = 502
				var ids []int64
				for i := range total {
					id, err := d.InsertInsight(Insight{
						Type:     "daily_activity",
						DateFrom: "2025-01-15",
						DateTo:   "2025-01-15",
						Agent:    "claude",
						Content:  fmt.Sprintf("insight %d", i),
					})
					if err != nil {
						t.Fatalf("InsertInsight %d: %v", i, err)
					}
					ids = append(ids, id)
				}
				return ids
			},
			filter: InsightFilter{},
			verify: func(t *testing.T, got []Insight, ids []int64) {
				const total = 502
				if len(got) != 500 {
					t.Fatalf("got %d insights, want 500 (capped)", len(got))
				}

				// Newest (id 502) should be first.
				newestID := ids[total-1]
				if got[0].ID != newestID {
					t.Errorf("first ID = %d, want %d (newest)", got[0].ID, newestID)
				}
				// Oldest retained should be id 3 (skipping 1 and 2).
				oldestRetainedID := ids[total-500]
				if got[499].ID != oldestRetainedID {
					t.Errorf("last ID = %d, want %d (oldest retained)", got[499].ID, oldestRetainedID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDB(t)
			ids := tt.seed(t, d)
			got, err := d.ListInsights(ctx, tt.filter)
			if err != nil {
				t.Fatalf("ListInsights: %v", err)
			}
			tt.verify(t, got, ids)
		})
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

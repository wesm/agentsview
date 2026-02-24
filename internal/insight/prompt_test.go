package insight

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/dbtest"
)

func TestBuildPrompt_WithSessions(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	ctx := context.Background()

	dbtest.SeedSession(t, d, "s1", "my-app", func(s *db.Session) {
		s.MessageCount = 5
		s.StartedAt = dbtest.Ptr("2025-01-15T10:00:00Z")
		s.EndedAt = dbtest.Ptr("2025-01-15T11:00:00Z")
		s.FirstMessage = dbtest.Ptr("Fix the login bug")
	})
	dbtest.SeedSession(t, d, "s2", "other-app", func(s *db.Session) {
		s.MessageCount = 3
		s.StartedAt = dbtest.Ptr("2025-01-15T14:00:00Z")
		s.EndedAt = dbtest.Ptr("2025-01-15T15:00:00Z")
		s.FirstMessage = dbtest.Ptr("Add tests")
	})

	prompt, err := BuildPrompt(ctx, d, GenerateRequest{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	checks := []string{
		"summarizing a day",
		"Date: 2025-01-15",
		"s1",
		"my-app",
		"Fix the login bug",
		"s2",
		"other-app",
		"Add tests",
	}
	for _, want := range checks {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildPrompt_ProjectFilter(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	ctx := context.Background()

	dbtest.SeedSession(t, d, "s1", "my-app", func(s *db.Session) {
		s.MessageCount = 5
		s.StartedAt = dbtest.Ptr("2025-01-15T10:00:00Z")
		s.EndedAt = dbtest.Ptr("2025-01-15T11:00:00Z")
	})
	dbtest.SeedSession(t, d, "s2", "other-app", func(s *db.Session) {
		s.MessageCount = 3
		s.StartedAt = dbtest.Ptr("2025-01-15T14:00:00Z")
		s.EndedAt = dbtest.Ptr("2025-01-15T15:00:00Z")
	})

	prompt, err := BuildPrompt(ctx, d, GenerateRequest{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
		Project:  "my-app",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	if !strings.Contains(prompt, "Project: my-app") {
		t.Error("prompt missing project header")
	}
	if strings.Contains(prompt, "other-app") {
		t.Error("prompt should not include other-app sessions")
	}
}

func TestBuildPrompt_UserPrompt(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	ctx := context.Background()

	prompt, err := BuildPrompt(ctx, d, GenerateRequest{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
		Prompt:   "Focus on security improvements",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	if !strings.Contains(prompt, "Additional Context") {
		t.Error("prompt missing Additional Context section")
	}
	if !strings.Contains(
		prompt, "Focus on security improvements",
	) {
		t.Error("prompt missing user prompt text")
	}
}

func TestBuildPrompt_AgentAnalysis(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	ctx := context.Background()

	prompt, err := BuildPrompt(ctx, d, GenerateRequest{
		Type:     "agent_analysis",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	if !strings.Contains(prompt, "analyzing AI agent") {
		t.Error("prompt missing analysis instruction")
	}
}

func TestBuildPrompt_Truncation(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	ctx := context.Background()

	for i := range 55 {
		dbtest.SeedSession(
			t, d,
			fmt.Sprintf("s%d", i), "my-app",
			func(s *db.Session) {
				s.MessageCount = 1
				s.StartedAt = dbtest.Ptr(
					"2025-01-15T10:00:00Z",
				)
				s.EndedAt = dbtest.Ptr(
					fmt.Sprintf(
						"2025-01-15T11:%02d:00Z", i,
					),
				)
			},
		)
	}

	prompt, err := BuildPrompt(ctx, d, GenerateRequest{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	if !strings.Contains(prompt, "omitted") {
		t.Error("prompt should mention truncation")
	}
	count := strings.Count(prompt, "### Session")
	if count != 50 {
		t.Errorf("got %d sessions in prompt, want 50", count)
	}
}

func TestBuildPrompt_NoSessions(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	ctx := context.Background()

	prompt, err := BuildPrompt(ctx, d, GenerateRequest{
		Type:     "daily_activity",
		DateFrom: "2025-01-15",
		DateTo:   "2025-01-15",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	if !strings.Contains(prompt, "No sessions found") {
		t.Error("prompt should indicate no sessions")
	}
}

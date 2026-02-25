package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/wesm/agentsview/internal/db"
)

type sessionSpec struct {
	project      string
	suffix       string
	msgCount     int
	userMsgCount int
}

var specs = []sessionSpec{
	{"project-alpha", "small-2", 2, 1},
	{"project-alpha", "small-5", 5, 3},
	{"project-beta", "mixed-content-6", 6, 3},
	{"project-beta", "medium-8", 8, 4},
	{"project-beta", "medium-100", 100, 50},
	{"project-gamma", "large-200", 200, 100},
	{"project-gamma", "large-1500", 1500, 750},
	{"project-delta", "xlarge-5500", 5500, 2750},
}

func main() {
	out := flag.String("out", "", "output database path")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: testfixture -out <path>")
		os.Exit(1)
	}

	if err := os.Remove(*out); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		log.Fatalf("removing existing db: %v", err)
	}

	database, err := db.Open(*out)
	if err != nil {
		log.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	base := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	for i, spec := range specs {
		if err := createSessionFixture(
			database, spec, i, base,
		); err != nil {
			log.Fatalf("creating fixture %s: %v", spec.suffix, err)
		}
		fmt.Printf(
			"  test-session-%s: %d messages\n",
			spec.suffix, spec.msgCount,
		)
	}

	fmt.Printf("Fixture DB written to %s\n", *out)
}

func ptr[T any](v T) *T { return &v }

func createSessionFixture(
	database *db.DB, spec sessionSpec,
	index int, base time.Time,
) error {
	sessionID := fmt.Sprintf("test-session-%s", spec.suffix)
	startedAt := base.Add(
		time.Duration(index) * 24 * time.Hour,
	)
	endedAt := startedAt.Add(
		time.Duration(spec.msgCount) * time.Minute,
	)

	sess := db.Session{
		ID:      sessionID,
		Project: spec.project,
		Machine: "test-machine",
		Agent:   "claude",
		FirstMessage: ptr(
			fmt.Sprintf("First message for %s", spec.project),
		),
		StartedAt:        ptr(startedAt.Format(time.RFC3339Nano)),
		EndedAt:          ptr(endedAt.Format(time.RFC3339Nano)),
		MessageCount:     spec.msgCount,
		UserMessageCount: spec.userMsgCount,
	}
	if err := database.UpsertSession(sess); err != nil {
		return fmt.Errorf("upserting session: %w", err)
	}

	var msgs []db.Message
	if spec.suffix == "mixed-content-6" {
		msgs = generateMixedContentMessages(sessionID, startedAt)
	} else {
		msgs = generateMessages(
			sessionID, spec.msgCount, startedAt,
		)
	}
	if err := database.InsertMessages(msgs); err != nil {
		return fmt.Errorf("inserting messages: %w", err)
	}
	return nil
}

func generateMessages(
	sessionID string, count int, start time.Time,
) []db.Message {
	msgs := make([]db.Message, 0, count)
	for i := range count {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}

		ts := start.Add(time.Duration(i) * time.Minute)
		content := generateContent(role, i, count)

		msgs = append(msgs, db.Message{
			SessionID:     sessionID,
			Ordinal:       i,
			Role:          role,
			Content:       content,
			Timestamp:     ts.Format(time.RFC3339Nano),
			HasThinking:   role == "assistant" && i%5 == 0,
			HasToolUse:    role == "assistant" && i%3 == 0,
			ContentLength: len(content),
		})
	}
	return msgs
}

func generateMixedContentMessages(
	sessionID string, start time.Time,
) []db.Message {
	type spec struct {
		role        string
		content     string
		hasThinking bool
		hasToolUse  bool
	}

	specs := []spec{
		{
			role:    "user",
			content: "Help me read a file",
		},
		{
			role: "assistant",
			content: "[Thinking]\nLet me analyze..." +
				"\n\nHere is my analysis.",
			hasThinking: true,
		},
		{
			role:    "user",
			content: "Now check the directory",
		},
		{
			role:       "assistant",
			content:    "[Read /src/main.ts]\nconst app = express();",
			hasToolUse: true,
		},
		{
			role:       "assistant",
			content:    "[Bash]\nls -la /src",
			hasToolUse: true,
		},
		{
			role:    "user",
			content: "Thanks",
		},
	}

	msgs := make([]db.Message, 0, len(specs))
	for i, s := range specs {
		ts := start.Add(time.Duration(i) * time.Minute)
		msgs = append(msgs, db.Message{
			SessionID:     sessionID,
			Ordinal:       i,
			Role:          s.role,
			Content:       s.content,
			Timestamp:     ts.Format(time.RFC3339Nano),
			HasThinking:   s.hasThinking,
			HasToolUse:    s.hasToolUse,
			ContentLength: len(s.content),
		})
	}
	return msgs
}

func generateContent(role string, idx, total int) string {
	if role == "user" {
		return fmt.Sprintf(
			"User message %d of %d. "+
				"Please help me with this task. "+
				"I need to understand how the code works.",
			idx, total,
		)
	}
	return fmt.Sprintf(
		"Assistant response %d of %d. "+
			"Here is my analysis of the code. "+
			"The implementation follows standard patterns "+
			"and uses well-known libraries. "+
			"Let me explain the key components.",
		idx, total,
	)
}

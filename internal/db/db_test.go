package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// filterWith returns a SessionFilter with Limit defaulted to 100.
func filterWith(fn func(*SessionFilter)) SessionFilter {
	f := SessionFilter{Limit: 100}
	fn(&f)
	return f
}

// sessionSet inserts 3 sessions with sequential dates and
// increasing message counts (5, 15, 25).
func sessionSet(t *testing.T, d *DB) {
	t.Helper()
	for i, mc := range []int{5, 15, 25} {
		day := fmt.Sprintf("2024-06-0%dT10:00:00Z", i+1)
		end := fmt.Sprintf("2024-06-0%dT11:00:00Z", i+1)
		insertSession(t, d, fmt.Sprintf("s%d", i+1),
			"proj", func(s *Session) {
				s.StartedAt = Ptr(day)
				s.EndedAt = Ptr(end)
				s.MessageCount = mc
			})
	}
}

// requireCount lists sessions with filter and asserts the count.
func requireCount(
	t *testing.T, d *DB, f SessionFilter, want int,
) {
	t.Helper()
	page, err := d.ListSessions(
		context.Background(), f,
	)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got := len(page.Sessions); got != want {
		t.Errorf("got %d sessions, want %d", got, want)
	}
}

// requireErrContains fails if err is nil or doesn't contain
// substr.
func requireErrContains(
	t *testing.T, err error, substr string,
) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q",
			err.Error(), substr)
	}
}

const (
	defaultMachine = "local"
	defaultAgent   = "claude"

	// Timestamp constants for test data.
	tsZero    = "2024-01-01T00:00:00Z"
	tsZeroS1  = "2024-01-01T00:00:01Z"
	tsZeroS2  = "2024-01-01T00:00:02Z"
	tsHour1   = "2024-01-01T01:00:00Z"
	tsMidYear = "2024-06-01T10:00:00Z"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// Ptr returns a pointer to v.
func Ptr[T any](v T) *T { return &v }

// insertSession creates and upserts a session with sensible
// defaults. Override any field via the opts functions.
func insertSession(
	t *testing.T, d *DB, id, project string,
	opts ...func(*Session),
) {
	t.Helper()
	s := Session{
		ID:           id,
		Project:      project,
		Machine:      defaultMachine,
		Agent:        defaultAgent,
		MessageCount: 1,
	}
	for _, opt := range opts {
		opt(&s)
	}
	if err := d.UpsertSession(s); err != nil {
		t.Fatalf("insertSession %s: %v", id, err)
	}
}

// insertMessages is a helper that inserts messages and fails
// the test on error.
func insertMessages(t *testing.T, d *DB, msgs ...Message) {
	t.Helper()
	if err := d.InsertMessages(msgs); err != nil {
		t.Fatalf("insertMessages: %v", err)
	}
}

// userMsg creates a user message with the given content.
func userMsg(sid string, ordinal int, content string) Message {
	return Message{
		SessionID:     sid,
		Ordinal:       ordinal,
		Role:          "user",
		Content:       content,
		ContentLength: len(content),
		Timestamp:     tsZero,
	}
}

// asstMsg creates an assistant message with the given content.
func asstMsg(sid string, ordinal int, content string) Message {
	return Message{
		SessionID:     sid,
		Ordinal:       ordinal,
		Role:          "assistant",
		Content:       content,
		ContentLength: len(content),
		Timestamp:     tsZero,
	}
}

// userMsgAt creates a user message with the given content and
// timestamp.
func userMsgAt(
	sid string, ordinal int, content, ts string,
) Message {
	m := userMsg(sid, ordinal, content)
	m.Timestamp = ts
	return m
}

// asstMsgAt creates an assistant message with the given content
// and timestamp.
func asstMsgAt(
	sid string, ordinal int, content, ts string,
) Message {
	m := asstMsg(sid, ordinal, content)
	m.Timestamp = ts
	return m
}

// canceledCtx returns an already-canceled context.
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// requireCanceledErr asserts that err is context.Canceled.
func requireCanceledErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// requireFTS skips the test if FTS is not available.
func requireFTS(t *testing.T, d *DB) {
	t.Helper()
	if !d.HasFTS() {
		t.Skip("no FTS support")
	}
}

// requireSessionExists asserts that a session exists and returns it.
func requireSessionExists(t *testing.T, d *DB, id string) *Session {
	t.Helper()
	s, err := d.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("GetSession %q: %v", id, err)
	}
	if s == nil {
		t.Fatalf("session %q should exist", id)
	}
	return s
}

// requireSessionGone asserts that a session does not exist.
func requireSessionGone(t *testing.T, d *DB, id string) {
	t.Helper()
	s, err := d.GetSession(context.Background(), id)
	if err != nil {
		t.Fatalf("GetSession %q: %v", id, err)
	}
	if s != nil {
		t.Fatalf("session %q should be gone", id)
	}
}

func TestOpenCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}

func TestOpenProbeErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping: chmod semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("skipping: running as root")
	}

	t.Run("StatPermissionError", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(sub, "test.db")

		d, err := Open(path)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		d.Close()

		// Remove execute on parent dir so os.Stat fails
		// with EACCES, not ENOENT.
		if err := os.Chmod(sub, 0o000); err != nil {
			t.Skipf("cannot remove permissions: %v", err)
		}
		t.Cleanup(func() { os.Chmod(sub, 0o755) })

		_, err = Open(path)
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("expected permission error, got: %v",
				err)
		}
		if !strings.Contains(err.Error(),
			"checking schema") {
			t.Errorf("expected 'checking schema' wrapper: %v",
				err)
		}
	})

	t.Run("ProbeReadError", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.db")

		d, err := Open(path)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		d.Close()

		// Remove read on the file so os.Stat succeeds
		// but the SQLite probe fails.
		if err := os.Chmod(path, 0o000); err != nil {
			t.Skipf("cannot remove permissions: %v", err)
		}
		t.Cleanup(func() { os.Chmod(path, 0o644) })

		_, err = Open(path)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(),
			"checking schema") &&
			!strings.Contains(err.Error(),
				"probing schema") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSessionCRUD(t *testing.T) {
	d := testDB(t)

	s := Session{
		ID:           "test-session-1",
		Project:      "my_project",
		Machine:      defaultMachine,
		Agent:        defaultAgent,
		FirstMessage: Ptr("Hello world"),
		StartedAt:    Ptr(tsZero),
		EndedAt:      Ptr(tsHour1),
		MessageCount: 5,
	}

	if err := d.UpsertSession(s); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	got := requireSessionExists(t, d, "test-session-1")
	if got.Project != "my_project" {
		t.Errorf("project = %q, want %q", got.Project, "my_project")
	}
	if got.MessageCount != 5 {
		t.Errorf("message_count = %d, want 5", got.MessageCount)
	}

	// Update
	s.MessageCount = 10
	if err := d.UpsertSession(s); err != nil {
		t.Fatalf("UpsertSession update: %v", err)
	}
	got = requireSessionExists(t, d, "test-session-1")
	if got.MessageCount != 10 {
		t.Errorf("after update: message_count = %d, want 10",
			got.MessageCount)
	}

	// Get nonexistent
	requireSessionGone(t, d, "nonexistent")
}

func TestSessionParentSessionID(t *testing.T) {
	d := testDB(t)

	t.Run("UpsertWithParent", func(t *testing.T) {
		insertSession(t, d, "child-1", "proj", func(s *Session) {
			s.ParentSessionID = Ptr("parent-uuid")
		})

		got := requireSessionExists(t, d, "child-1")
		if got.ParentSessionID == nil ||
			*got.ParentSessionID != "parent-uuid" {
			t.Errorf("parent_session_id = %v, want %q",
				got.ParentSessionID, "parent-uuid")
		}
	})

	t.Run("WithoutParent", func(t *testing.T) {
		insertSession(t, d, "child-2", "proj")

		got := requireSessionExists(t, d, "child-2")
		if got.ParentSessionID != nil {
			t.Errorf("parent_session_id = %v, want nil",
				got.ParentSessionID)
		}
	})

	t.Run("ParentInListSessions", func(t *testing.T) {
		page, err := d.ListSessions(
			context.Background(),
			filterWith(func(f *SessionFilter) {
				f.Project = "proj"
			}),
		)
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		found := false
		for _, s := range page.Sessions {
			if s.ID == "child-1" {
				found = true
				if s.ParentSessionID == nil ||
					*s.ParentSessionID != "parent-uuid" {
					t.Errorf("parent_session_id = %v, want %q",
						s.ParentSessionID, "parent-uuid")
				}
			}
		}
		if !found {
			t.Error("child-1 not found in list")
		}
	})

	t.Run("ParentInGetSessionFull", func(t *testing.T) {
		got, err := d.GetSessionFull(
			context.Background(), "child-1",
		)
		if err != nil {
			t.Fatalf("GetSessionFull: %v", err)
		}
		if got == nil {
			t.Fatal("session not found")
		}
		if got.ParentSessionID == nil ||
			*got.ParentSessionID != "parent-uuid" {
			t.Errorf("parent_session_id = %v, want %q",
				got.ParentSessionID, "parent-uuid")
		}
	})
}

func TestListSessions(t *testing.T) {
	d := testDB(t)

	for i := range 5 {
		ea := fmt.Sprintf("2024-01-01T0%d:00:00Z", i)
		insertSession(t, d,
			fmt.Sprintf("session-%c", 'a'+i), "proj",
			func(s *Session) {
				s.EndedAt = Ptr(ea)
				s.MessageCount = i + 1
			},
		)
	}

	requireCount(t, d, SessionFilter{Limit: 10}, 5)

	page, err := d.ListSessions(
		context.Background(), SessionFilter{Limit: 2},
	)
	if err != nil {
		t.Fatalf("ListSessions limit: %v", err)
	}
	if len(page.Sessions) != 2 {
		t.Errorf("got %d sessions, want 2", len(page.Sessions))
	}
	if page.NextCursor == "" {
		t.Error("expected next cursor")
	}

	requireCount(t, d, SessionFilter{
		Limit:  10,
		Cursor: page.NextCursor,
	}, 3)
}

func TestListSessionsPaginationNoDuplicates(t *testing.T) {
	d := testDB(t)

	// 5 sessions: 2 share the same ended_at to test
	// tie-breaking at page boundaries.
	times := []string{
		"2024-01-01T01:00:00Z",
		"2024-01-01T02:00:00Z",
		"2024-01-01T02:00:00Z", // same as previous
		"2024-01-01T03:00:00Z",
		"2024-01-01T04:00:00Z",
	}
	for i, ea := range times {
		insertSession(t, d,
			fmt.Sprintf("page-%c", 'a'+i), "proj",
			func(s *Session) { s.EndedAt = Ptr(ea) },
		)
	}

	// Paginate through all sessions 2 at a time.
	seen := make(map[string]bool)
	cursor := ""
	pages := 0
	for {
		page, err := d.ListSessions(
			context.Background(),
			SessionFilter{Limit: 2, Cursor: cursor},
		)
		if err != nil {
			t.Fatalf("ListSessions page %d: %v", pages, err)
		}
		for _, s := range page.Sessions {
			if seen[s.ID] {
				t.Errorf("duplicate session %s on page %d",
					s.ID, pages)
			}
			seen[s.ID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 5 {
		t.Errorf("saw %d sessions, want 5", len(seen))
	}
}

func TestListSessionsProjectFilter(t *testing.T) {
	d := testDB(t)

	for i, proj := range []string{"proj_a", "proj_a", "proj_b"} {
		ea := fmt.Sprintf("2024-01-01T00:00:0%dZ", i)
		insertSession(t, d,
			fmt.Sprintf("%s-%d", proj, i), proj,
			func(s *Session) { s.EndedAt = Ptr(ea) },
		)
	}

	requireCount(t, d, filterWith(func(f *SessionFilter) {
		f.Project = "proj_a"
	}), 2)
}

func TestMessageCRUD(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.MessageCount = 4
	})

	m1 := userMsg("s1", 0, "Hello")
	m2 := asstMsgAt("s1", 1, "Hi there", tsZeroS1)
	m3 := userMsgAt("s1", 2, "Thanks", tsZeroS2)
	m4 := userMsgAt("s1", 3, "Empty TS", "")

	insertMessages(t, d, m1, m2, m3, m4)

	got, err := d.GetAllMessages(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetAllMessages: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d messages, want 4", len(got))
	}
	if got[0].Content != "Hello" {
		t.Errorf("first message = %q", got[0].Content)
	}
	if got[3].Timestamp != "" {
		t.Errorf("expected empty timestamp, got %q", got[3].Timestamp)
	}

	// Paginated
	got, err = d.GetMessages(context.Background(), "s1", 1, 2, true)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if got[0].Ordinal != 1 {
		t.Errorf("first ordinal = %d, want 1", got[0].Ordinal)
	}

	// Descending
	got, err = d.GetMessages(context.Background(), "s1", 2, 10, false)
	if err != nil {
		t.Fatalf("GetMessages desc: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	if got[0].Ordinal != 2 {
		t.Errorf("desc first ordinal = %d, want 2", got[0].Ordinal)
	}
}

func TestMinimap(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.MessageCount = 2
	})

	m2 := asstMsg("s1", 1, "Hi")
	m2.HasThinking = true
	m2.HasToolUse = true

	insertMessages(t, d,
		userMsg("s1", 0, "Hello"),
		m2,
	)

	entries, err := d.GetMinimap(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetMinimap: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if !entries[1].HasThinking {
		t.Error("expected HasThinking on second entry")
	}
}

func TestReplaceSessionMessages(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p")

	insertMessages(t, d, userMsg("s1", 0, "old"))

	if err := d.ReplaceSessionMessages("s1", []Message{
		userMsg("s1", 0, "new1"),
		asstMsg("s1", 1, "new2"),
	}); err != nil {
		t.Fatalf("ReplaceSessionMessages: %v", err)
	}

	got, _ := d.GetAllMessages(context.Background(), "s1")
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if got[0].Content != "new1" {
		t.Errorf("content = %q, want %q", got[0].Content, "new1")
	}
}

func TestSearch(t *testing.T) {
	d := testDB(t)
	requireFTS(t, d)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.MessageCount = 2
	})

	m1 := userMsg("s1", 0, "Fix the authentication bug")
	m2 := asstMsgAt("s1", 1, "Looking at the auth module",
		tsZeroS1)

	insertMessages(t, d, m1, m2)

	page, err := d.Search(context.Background(), SearchFilter{
		Query: "authentication",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(page.Results))
	}
	if page.Results[0].SessionID != "s1" {
		t.Errorf("session_id = %q", page.Results[0].SessionID)
	}
}

func TestCanceledContext(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.MessageCount = 1
	})
	insertMessages(t, d, userMsg("s1", 0, "searchable content"))

	ctx := canceledCtx()

	tests := []struct {
		name string
		fn   func() error
		skip bool
	}{
		{"Search", func() error {
			_, err := d.Search(ctx, SearchFilter{
				Query: "searchable", Limit: 10,
			})
			return err
		}, !d.HasFTS()},
		{"ListSessions", func() error {
			_, err := d.ListSessions(ctx, SessionFilter{Limit: 10})
			return err
		}, false},
		{"GetMessages", func() error {
			_, err := d.GetMessages(ctx, "s1", 0, 10, true)
			return err
		}, false},
		{"GetStats", func() error {
			_, err := d.GetStats(ctx)
			return err
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("no FTS support")
			}
			requireCanceledErr(t, tt.fn())
		})
	}
}

func TestStats(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p1")
	insertSession(t, d, "s2", "p2", func(s *Session) {
		s.Machine = "remote"
		s.Agent = "codex"
	})
	insertMessages(t, d,
		userMsg("s1", 0, "hi"),
		userMsg("s2", 0, "bye"),
	)

	stats, err := d.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.SessionCount != 2 {
		t.Errorf("session_count = %d, want 2", stats.SessionCount)
	}
	if stats.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", stats.MessageCount)
	}
	if stats.ProjectCount != 2 {
		t.Errorf("project_count = %d, want 2", stats.ProjectCount)
	}
	if stats.MachineCount != 2 {
		t.Errorf("machine_count = %d, want 2", stats.MachineCount)
	}
}

func TestGetProjects(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "alpha")
	insertSession(t, d, "s2", "beta", func(s *Session) {
		s.MessageCount = 2
	})
	insertSession(t, d, "s3", "alpha")

	projects, err := d.GetProjects(context.Background())
	if err != nil {
		t.Fatalf("GetProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	if projects[0].Name != "alpha" || projects[0].SessionCount != 2 {
		t.Errorf("alpha: %+v", projects[0])
	}
}

// setupPruneData inserts the standard sessions used by the prune
// candidate filter tests. Each session gets real message rows so
// the user-message subquery in FindPruneCandidates works.
func setupPruneData(t *testing.T, d *DB) {
	t.Helper()
	// s1: 2 user messages
	insertSession(t, d, "s1", "spicytakes", func(s *Session) {
		s.FirstMessage = Ptr("You are a code reviewer")
		s.EndedAt = Ptr("2024-01-15T00:00:00Z")
		s.MessageCount = 2
	})
	insertMessages(t, d,
		userMsg("s1", 0, "You are a code reviewer"),
		userMsg("s1", 1, "Review this"),
	)
	// s2: 2 user messages
	insertSession(t, d, "s2", "spicytakes", func(s *Session) {
		s.FirstMessage = Ptr("Analyze this blog post")
		s.EndedAt = Ptr("2024-03-01T00:00:00Z")
		s.MessageCount = 2
	})
	insertMessages(t, d,
		userMsg("s2", 0, "Analyze this blog post"),
		userMsg("s2", 1, "More analysis"),
	)
	// s3: 2 user messages
	insertSession(t, d, "s3", "roborev", func(s *Session) {
		s.FirstMessage = Ptr("You are a code reviewer")
		s.EndedAt = Ptr("2024-03-01T00:00:00Z")
		s.MessageCount = 2
	})
	insertMessages(t, d,
		userMsg("s3", 0, "You are a code reviewer"),
		userMsg("s3", 1, "Check this file"),
	)
	// s4: 5 user messages + 5 assistant messages = 10 total
	insertSession(t, d, "s4", "spicytakes", func(s *Session) {
		s.FirstMessage = Ptr("Help me refactor")
		s.EndedAt = Ptr("2024-06-01T00:00:00Z")
		s.MessageCount = 10
	})
	insertMessages(t, d,
		userMsg("s4", 0, "Help me refactor"),
		asstMsg("s4", 1, "Sure, here's a plan"),
		userMsg("s4", 2, "Do step 1"),
		asstMsg("s4", 3, "Done with step 1"),
		userMsg("s4", 4, "Do step 2"),
		asstMsg("s4", 5, "Done with step 2"),
		userMsg("s4", 6, "Do step 3"),
		asstMsg("s4", 7, "Done with step 3"),
		userMsg("s4", 8, "Looks good"),
		asstMsg("s4", 9, "Thanks"),
	)
}

func TestFindPruneCandidates(t *testing.T) {
	d := testDB(t)
	setupPruneData(t, d)

	tests := []struct {
		name   string
		filter PruneFilter
		want   int
	}{
		{
			name:   "ProjectSubstring",
			filter: PruneFilter{Project: "spicy"},
			want:   3,
		},
		{
			name:   "MaxMessages",
			filter: PruneFilter{MaxMessages: Ptr(2)},
			want:   3,
		},
		{
			name: "BeforeDate",
			filter: PruneFilter{
				Before: "2024-02-01",
			},
			want: 1,
		},
		{
			name: "FirstMessagePrefix",
			filter: PruneFilter{
				FirstMessage: "You are a code reviewer",
			},
			want: 2,
		},
		{
			name: "CombinedProjectAndMaxMessages",
			filter: PruneFilter{
				Project: "spicytakes", MaxMessages: Ptr(2),
			},
			want: 2,
		},
		{
			name: "AllFiltersNoMatch",
			filter: PruneFilter{
				Project:      "spicytakes",
				MaxMessages:  Ptr(2),
				Before:       "2024-02-01",
				FirstMessage: "Analyze",
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.FindPruneCandidates(tt.filter)
			if err != nil {
				t.Fatalf("FindPruneCandidates: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d candidates, want %d",
					len(got), tt.want)
			}
		})
	}

	// The "before" case also checks the specific ID returned.
	t.Run("BeforeDateReturnsCorrectID", func(t *testing.T) {
		got, err := d.FindPruneCandidates(PruneFilter{
			Before: "2024-02-01",
		})
		if err != nil {
			t.Fatalf("FindPruneCandidates: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d, want 1", len(got))
		}
		if got[0].ID != "s1" {
			t.Errorf("got ID %q, want s1", got[0].ID)
		}
	})

	// File metadata returned correctly.
	t.Run("ReturnsFileMetadata", func(t *testing.T) {
		fp := "/path/to/file.jsonl"
		insertSession(t, d, "s5", "test", func(s *Session) {
			s.FilePath = Ptr(fp)
			s.FileSize = Ptr(int64(4096))
		})
		got, err := d.FindPruneCandidates(PruneFilter{
			Project: "test",
		})
		if err != nil {
			t.Fatalf("FindPruneCandidates: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d, want 1", len(got))
		}
		if got[0].FilePath == nil || *got[0].FilePath != fp {
			t.Errorf("file_path = %v, want %q", got[0].FilePath, fp)
		}
		if got[0].FileSize == nil || *got[0].FileSize != 4096 {
			t.Errorf("file_size = %v, want 4096", got[0].FileSize)
		}
	})
}

// collectIDs extracts session IDs for error messages.
func collectIDs(sessions []Session) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	return ids
}

func TestFindPruneCandidatesExcludesParents(t *testing.T) {
	d := testDB(t)

	// Create a parent -> child chain.
	insertSession(t, d, "parent1", "proj", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T10:00:00Z")
		s.EndedAt = Ptr("2024-06-01T11:00:00Z")
	})
	insertSession(t, d, "child1", "proj", func(s *Session) {
		s.ParentSessionID = Ptr("parent1")
		s.StartedAt = Ptr("2024-06-01T12:00:00Z")
		s.EndedAt = Ptr("2024-06-01T13:00:00Z")
	})
	// A standalone session with no children.
	insertSession(t, d, "standalone", "proj", func(s *Session) {
		s.StartedAt = Ptr("2024-06-01T14:00:00Z")
		s.EndedAt = Ptr("2024-06-01T15:00:00Z")
	})

	got, err := d.FindPruneCandidates(PruneFilter{
		Project: "proj",
	})
	if err != nil {
		t.Fatalf("FindPruneCandidates: %v", err)
	}

	ids := collectIDs(got)

	// Parent should be excluded; child and standalone eligible.
	if len(got) != 2 {
		t.Fatalf("got %d candidates %v, want 2",
			len(got), ids)
	}
	for _, s := range got {
		if s.ID == "parent1" {
			t.Errorf("parent1 should be excluded, "+
				"got candidates: %v", ids)
		}
	}
}

func TestFindPruneCandidatesLikeEscaping(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "e1", "my%project", func(s *Session) {
		s.FirstMessage = Ptr("100% complete")
	})
	insertSession(t, d, "e2", "my_project", func(s *Session) {
		s.FirstMessage = Ptr("100% complete")
	})
	insertSession(t, d, "e3", "myXproject")
	insertSession(t, d, "e4", `my\project`, func(s *Session) {
		s.FirstMessage = Ptr(`path\to\file`)
	})

	tests := []struct {
		name     string
		filter   PruneFilter
		wantN    int
		wantOnly string
	}{
		{
			name: "LiteralPercent",
			filter: PruneFilter{
				Project: "%",
			},
			wantN: 1, wantOnly: "e1",
		},
		{
			name: "LiteralUnderscore",
			filter: PruneFilter{
				Project: "_",
			},
			wantN: 1, wantOnly: "e2",
		},
		{
			name: "PercentInFirstMessage",
			filter: PruneFilter{
				FirstMessage: "100%",
			},
			wantN: 2,
		},
		{
			name: "BackslashInProject",
			filter: PruneFilter{
				Project: `\`,
			},
			wantN: 1, wantOnly: "e4",
		},
		{
			name: "BackslashInFirstMessage",
			filter: PruneFilter{
				FirstMessage: `path\to`,
			},
			wantN: 1, wantOnly: "e4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.FindPruneCandidates(tt.filter)
			if err != nil {
				t.Fatalf("FindPruneCandidates: %v", err)
			}
			if len(got) != tt.wantN {
				t.Fatalf("got %v, want %d results",
					collectIDs(got), tt.wantN)
			}
			if tt.wantOnly != "" && got[0].ID != tt.wantOnly {
				t.Errorf("got %v, want [%s]",
					collectIDs(got), tt.wantOnly)
			}
		})
	}
}

func TestFindPruneCandidatesMaxMessagesSentinel(t *testing.T) {
	d := testDB(t)

	// m1: 0 user messages
	insertSession(t, d, "m1", "p", func(s *Session) {
		s.MessageCount = 0
	})
	// m2: 1 user message (default from insertSession)
	insertSession(t, d, "m2", "p")
	insertMessages(t, d, userMsg("m2", 0, "hello"))
	// m3: 3 user messages + 2 assistant = 5 total
	insertSession(t, d, "m3", "p", func(s *Session) {
		s.MessageCount = 5
	})
	insertMessages(t, d,
		userMsg("m3", 0, "msg1"),
		asstMsg("m3", 1, "reply1"),
		userMsg("m3", 2, "msg2"),
		asstMsg("m3", 3, "reply2"),
		userMsg("m3", 4, "msg3"),
	)

	tests := []struct {
		name   string
		filter PruneFilter
		want   int
	}{
		{
			name:   "ZeroMatchesOnlyZero",
			filter: PruneFilter{MaxMessages: Ptr(0)},
			want:   1,
		},
		{
			name: "NilDisablesFilter",
			filter: PruneFilter{
				Project: "p",
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.FindPruneCandidates(tt.filter)
			if err != nil {
				t.Fatalf("FindPruneCandidates: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d, want %d", len(got), tt.want)
			}
		})
	}

	// Additional check: MaxMessages=0 returns m1 specifically.
	got, err := d.FindPruneCandidates(PruneFilter{MaxMessages: Ptr(0)})
	if err != nil {
		t.Fatalf("FindPruneCandidates MaxMessages=0: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("MaxMessages 0: got %d results, want 1", len(got))
	}
	if got[0].ID != "m1" {
		t.Errorf("MaxMessages 0: got %q, want m1", got[0].ID)
	}
}

func TestDeleteSessions(t *testing.T) {
	d := testDB(t)

	for _, id := range []string{"s1", "s2", "s3"} {
		insertSession(t, d, id, "p")
		insertMessages(t, d, userMsg(id, 0, "msg for "+id))
	}

	stats, _ := d.GetStats(context.Background())
	if stats.SessionCount != 3 {
		t.Fatalf("initial sessions = %d, want 3", stats.SessionCount)
	}
	if stats.MessageCount != 3 {
		t.Fatalf("initial messages = %d, want 3", stats.MessageCount)
	}

	deleted, err := d.DeleteSessions([]string{"s1", "s3"})
	if err != nil {
		t.Fatalf("DeleteSessions: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	requireSessionGone(t, d, "s1")
	requireSessionExists(t, d, "s2")
	requireSessionGone(t, d, "s3")

	msgs, _ := d.GetAllMessages(context.Background(), "s1")
	if len(msgs) != 0 {
		t.Errorf("s1 messages = %d, want 0", len(msgs))
	}
	msgs, _ = d.GetAllMessages(context.Background(), "s2")
	if len(msgs) != 1 {
		t.Errorf("s2 messages = %d, want 1", len(msgs))
	}

	stats, _ = d.GetStats(context.Background())
	if stats.SessionCount != 1 {
		t.Errorf("session_count = %d, want 1", stats.SessionCount)
	}
	if stats.MessageCount != 1 {
		t.Errorf("message_count = %d, want 1", stats.MessageCount)
	}

	deleted, err = d.DeleteSessions(nil)
	if err != nil {
		t.Fatalf("DeleteSessions empty: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted empty = %d, want 0", deleted)
	}
}

func TestSessionFileInfo(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.FileSize = Ptr(int64(1024))
		s.FileMtime = Ptr(int64(1700000000))
		s.FileHash = Ptr("abc123def456")
	})

	gotSize, gotHash, ok := d.GetSessionFileInfo("s1")
	if !ok {
		t.Fatal("expected ok")
	}
	if gotSize != 1024 {
		t.Errorf("got size=%d, want 1024", gotSize)
	}
	if gotHash != "abc123def456" {
		t.Errorf("got hash=%q, want %q", gotHash, "abc123def456")
	}

	_, _, ok = d.GetSessionFileInfo("nonexistent")
	if ok {
		t.Error("expected !ok for nonexistent")
	}
}

func TestGetSessionFull(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	t.Run("AllMetadata", func(t *testing.T) {
		insertSession(t, d, "full-1", "proj", func(s *Session) {
			s.FirstMessage = Ptr("hello")
			s.StartedAt = Ptr(tsZero)
			s.EndedAt = Ptr(tsHour1)
			s.MessageCount = 5
			s.FilePath = Ptr("/tmp/session.jsonl")
			s.FileSize = Ptr(int64(2048))
			s.FileMtime = Ptr(int64(1700000000))
			s.FileHash = Ptr("abc123")
		})

		got, err := d.GetSessionFull(ctx, "full-1")
		if err != nil {
			t.Fatalf("GetSessionFull: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil session")
		}
		if got.ID != "full-1" {
			t.Errorf("ID = %q, want %q", got.ID, "full-1")
		}
		if got.Project != "proj" {
			t.Errorf("Project = %q, want %q", got.Project, "proj")
		}
		if got.MessageCount != 5 {
			t.Errorf("MessageCount = %d, want 5", got.MessageCount)
		}
		if got.FilePath == nil || *got.FilePath != "/tmp/session.jsonl" {
			t.Errorf("FilePath = %v, want %q", got.FilePath, "/tmp/session.jsonl")
		}
		if got.FileSize == nil || *got.FileSize != 2048 {
			t.Errorf("FileSize = %v, want 2048", got.FileSize)
		}
		if got.FileMtime == nil || *got.FileMtime != 1700000000 {
			t.Errorf("FileMtime = %v, want 1700000000", got.FileMtime)
		}
		if got.FileHash == nil || *got.FileHash != "abc123" {
			t.Errorf("FileHash = %v, want %q", got.FileHash, "abc123")
		}
		if got.FirstMessage == nil || *got.FirstMessage != "hello" {
			t.Errorf("FirstMessage = %v, want %q", got.FirstMessage, "hello")
		}
		if got.StartedAt == nil || *got.StartedAt != tsZero {
			t.Errorf("StartedAt = %v, want %q", got.StartedAt, tsZero)
		}
		if got.EndedAt == nil || *got.EndedAt != tsHour1 {
			t.Errorf("EndedAt = %v, want %q", got.EndedAt, tsHour1)
		}
	})

	t.Run("NullMetadata", func(t *testing.T) {
		insertSession(t, d, "full-2", "proj", func(s *Session) {
			s.MessageCount = 1
		})

		got, err := d.GetSessionFull(ctx, "full-2")
		if err != nil {
			t.Fatalf("GetSessionFull: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil session")
		}
		if got.FilePath != nil {
			t.Errorf("FilePath = %v, want nil", got.FilePath)
		}
		if got.FileSize != nil {
			t.Errorf("FileSize = %v, want nil", got.FileSize)
		}
		if got.FileMtime != nil {
			t.Errorf("FileMtime = %v, want nil", got.FileMtime)
		}
		if got.FileHash != nil {
			t.Errorf("FileHash = %v, want nil", got.FileHash)
		}
		if got.FirstMessage != nil {
			t.Errorf("FirstMessage = %v, want nil", got.FirstMessage)
		}
		if got.StartedAt != nil {
			t.Errorf("StartedAt = %v, want nil", got.StartedAt)
		}
		if got.EndedAt != nil {
			t.Errorf("EndedAt = %v, want nil", got.EndedAt)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		got, err := d.GetSessionFull(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("GetSessionFull: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil session, got %+v", got)
		}
	})
}

func TestCursorEncodeDecode(t *testing.T) {
	d := testDB(t)
	encoded := d.EncodeCursor(tsZero, "session-1")
	cur, err := d.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if cur.EndedAt != tsZero {
		t.Errorf("EndedAt = %q", cur.EndedAt)
	}
	if cur.ID != "session-1" {
		t.Errorf("ID = %q", cur.ID)
	}

	encodedWithTotal := d.EncodeCursor(
		tsZero,
		"session-1",
		123,
	)
	cur, err = d.DecodeCursor(encodedWithTotal)
	if err != nil {
		t.Fatalf("DecodeCursor with total: %v", err)
	}
	if cur.Total != 123 {
		t.Errorf("Total = %d, want 123", cur.Total)
	}
}

func TestCursorTampering(t *testing.T) {
	d := testDB(t)
	// 1. Create a valid signed cursor
	original := d.EncodeCursor(tsZero, "s1", 100)

	parts := strings.Split(original, ".")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (payload.sig), got %d", len(parts))
	}

	payload := parts[0]
	sig := parts[1]

	// 2. Decode payload, modify Total, re-encode
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("DecodeString payload: %v", err)
	}
	var c SessionCursor
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	c.Total = 999
	tamperedData, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal tampered: %v", err)
	}
	tamperedPayload := base64.RawURLEncoding.EncodeToString(tamperedData)

	// 3. Construct tampered cursor with original signature
	tamperedCursor := tamperedPayload + "." + sig

	// 4. Decode should fail signature check
	_, err = d.DecodeCursor(tamperedCursor)
	if err == nil {
		t.Fatal("expected error for tampered cursor, got nil")
	}
	if !strings.Contains(err.Error(), "signature mismatch") {
		t.Errorf("expected signature mismatch error, got: %v", err)
	}
}

func TestLegacyCursor(t *testing.T) {
	d := testDB(t)
	// Create a legacy cursor (base64 json only, no signature)
	c := SessionCursor{
		EndedAt: tsZero,
		ID:      "s1",
		Total:   100, // Should be ignored
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal legacy: %v", err)
	}
	legacy := base64.RawURLEncoding.EncodeToString(data)

	// Decode
	got, err := d.DecodeCursor(legacy)
	if err != nil {
		t.Fatalf("DecodeCursor legacy: %v", err)
	}

	// Verify ID/EndedAt are preserved
	if got.ID != "s1" {
		t.Errorf("ID = %q, want s1", got.ID)
	}
	// Verify Total is ZEROED out
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0 (untrusted legacy)", got.Total)
	}
}

func TestCursorSecretConcurrency(t *testing.T) {
	d := testDB(t)

	const goroutines = 8
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				switch id % 3 {
				case 0:
					secret := fmt.Appendf(
						nil, "secret-%d-%d", id, j,
					)
					d.SetCursorSecret(secret)
				case 1:
					d.EncodeCursor(
						tsZero,
						fmt.Sprintf("s-%d-%d", id, j),
						42,
					)
				case 2:
					encoded := d.EncodeCursor(
						tsZero, "s1",
					)
					// Decode may fail if secret rotated
					// between encode and decode; that's OK.
					_, err := d.DecodeCursor(encoded)
					if err != nil &&
						!errors.Is(err, ErrInvalidCursor) {
						t.Errorf(
							"unexpected DecodeCursor error: %v",
							err,
						)
					}
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestSetCursorSecretDefensiveCopy(t *testing.T) {
	d := testDB(t)

	secret := []byte("my-secret-key-for-testing-copy!!")
	d.SetCursorSecret(secret)

	encoded := d.EncodeCursor(tsZero, "s1")

	// Mutate the original slice — should not affect the DB.
	for i := range secret {
		secret[i] = 0
	}

	_, err := d.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf(
			"DecodeCursor failed after caller mutated secret: %v",
			err,
		)
	}
}

func TestSampleMinimap(t *testing.T) {
	// Create a helper to generate n entries
	makeEntries := func(n int) []MinimapEntry {
		entries := make([]MinimapEntry, 0, n)
		for i := range n {
			entries = append(entries, MinimapEntry{
				Ordinal: i,
				Role:    "user",
			})
		}
		return entries
	}

	tests := []struct {
		name    string
		entries []MinimapEntry
		max     int
		wantLen int
		// simple check function for ordinals
		check func([]MinimapEntry) error
	}{
		{
			name:    "SampleDown",
			entries: makeEntries(10),
			max:     3,
			wantLen: 3,
			check: func(got []MinimapEntry) error {
				if got[0].Ordinal != 0 || got[1].Ordinal != 4 || got[2].Ordinal != 9 {
					return fmt.Errorf("ordinals = [%d %d %d], want [0 4 9]",
						got[0].Ordinal, got[1].Ordinal, got[2].Ordinal)
				}
				return nil
			},
		},
		{
			name:    "ExactSize",
			entries: makeEntries(5),
			max:     5,
			wantLen: 5,
		},
		{
			name:    "SmallerThanMax",
			entries: makeEntries(3),
			max:     5,
			wantLen: 3,
		},
		{
			name:    "Empty",
			entries: makeEntries(0),
			max:     5,
			wantLen: 0,
		},
		{
			name:    "MaxOne",
			entries: makeEntries(10),
			max:     1,
			wantLen: 1,
			check: func(got []MinimapEntry) error {
				if got[0].Ordinal != 0 {
					return fmt.Errorf("ordinal = %d, want 0", got[0].Ordinal)
				}
				return nil
			},
		},
		{
			name:    "MaxZero",
			entries: makeEntries(10),
			max:     0,
			wantLen: 10, // Returns original if max <= 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SampleMinimap(tt.entries, tt.max)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
				return
			}
			if tt.check != nil {
				if err := tt.check(got); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestDeleteSession(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p")
	insertMessages(t, d, userMsg("s1", 0, "test"))

	if err := d.DeleteSession("s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	requireSessionGone(t, d, "s1")

	msgs, _ := d.GetAllMessages(context.Background(), "s1")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after cascade, got %d",
			len(msgs))
	}
}

func TestMigrationRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "race.db")

	// 1. Create a current schema so concurrent Opens exercise
	// the normal init path (old schemas are now dropped and
	// rebuilt, making concurrent migration less interesting).
	db1, err := Open(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	db1.Close()

	// 2. Run concurrent Open.
	errCh := make(chan error, 2)
	var (
		mu         sync.Mutex
		cond       = sync.NewCond(&mu)
		readyCount = 0
		start      = false
	)

	for range 2 {
		go func() {
			mu.Lock()
			readyCount++
			if readyCount == 2 {
				cond.Broadcast()
			}
			for !start {
				cond.Wait()
			}
			mu.Unlock()

			db, err := Open(path)
			if err != nil {
				errCh <- err
				return
			}
			db.Close()
			errCh <- nil
		}()
	}

	mu.Lock()
	for readyCount < 2 {
		cond.Wait()
	}
	start = true
	cond.Broadcast()
	mu.Unlock()

	var successes int
	for range 2 {
		if err := <-errCh; err != nil {
			msg := err.Error()
			if strings.Contains(msg, "database is locked") ||
				strings.Contains(msg, "database schema is locked") ||
				strings.Contains(msg, "SQLITE_BUSY") ||
				strings.Contains(msg, "SQLITE_LOCKED") {
				t.Logf("concurrent Open lock contention: %v", err)
			} else {
				t.Errorf("unexpected concurrent Open error: %v", err)
			}
		} else {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("both concurrent Opens failed")
	}

	// 3. Verify schema is intact
	dbCheck, err := Open(path)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer dbCheck.Close()

	_, err = dbCheck.writer.Exec(
		"SELECT parent_session_id FROM sessions LIMIT 1",
	)
	if err != nil {
		t.Errorf("parent_session_id column missing: %v", err)
	}
}

func TestToolCallsInsertedWithMessages(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.MessageCount = 2
	})

	m1 := userMsg("s1", 0, "hello")
	m2 := asstMsg("s1", 1, "[Read: main.go]")
	m2.HasToolUse = true
	m2.ToolCalls = []ToolCall{
		{SessionID: "s1", ToolName: "Read", Category: "Read"},
		{SessionID: "s1", ToolName: "Grep", Category: "Grep"},
	}

	insertMessages(t, d, m1, m2)

	// Query tool_calls directly
	rows, err := d.Reader().Query(
		`SELECT message_id, session_id, tool_name, category
		 FROM tool_calls WHERE session_id = ?
		 ORDER BY id`, "s1")
	if err != nil {
		t.Fatalf("query tool_calls: %v", err)
	}
	defer rows.Close()

	var calls []ToolCall
	for rows.Next() {
		var tc ToolCall
		if err := rows.Scan(
			&tc.MessageID, &tc.SessionID,
			&tc.ToolName, &tc.Category,
		); err != nil {
			t.Fatalf("scan tool_call: %v", err)
		}
		calls = append(calls, tc)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("got %d tool_calls, want 2", len(calls))
	}
	if calls[0].ToolName != "Read" || calls[0].Category != "Read" {
		t.Errorf("calls[0] = %+v", calls[0])
	}
	if calls[1].ToolName != "Grep" || calls[1].Category != "Grep" {
		t.Errorf("calls[1] = %+v", calls[1])
	}
	if calls[0].MessageID == 0 {
		t.Error("message_id should be non-zero")
	}
	if calls[0].SessionID != "s1" {
		t.Errorf("session_id = %q, want s1", calls[0].SessionID)
	}
}

func TestToolCallsCascadeOnSessionDelete(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p")

	m := asstMsg("s1", 0, "[Bash]")
	m.HasToolUse = true
	m.ToolCalls = []ToolCall{
		{SessionID: "s1", ToolName: "Bash", Category: "Bash"},
	}
	insertMessages(t, d, m)

	if err := d.DeleteSession("s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	var count int
	if err := d.Reader().QueryRow(
		"SELECT COUNT(*) FROM tool_calls WHERE session_id = ?",
		"s1",
	).Scan(&count); err != nil {
		t.Fatalf("count tool_calls: %v", err)
	}
	if count != 0 {
		t.Errorf("tool_calls count = %d, want 0", count)
	}
}

func TestReplaceSessionMessagesReplacesToolCalls(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p")

	m := asstMsg("s1", 0, "[Read: a.go]")
	m.HasToolUse = true
	m.ToolCalls = []ToolCall{
		{SessionID: "s1", ToolName: "Read", Category: "Read"},
	}
	insertMessages(t, d, m)

	// Replace with different tool calls
	m2 := asstMsg("s1", 0, "[Bash]")
	m2.HasToolUse = true
	m2.ToolCalls = []ToolCall{
		{SessionID: "s1", ToolName: "Bash", Category: "Bash"},
		{SessionID: "s1", ToolName: "Write", Category: "Write"},
	}
	if err := d.ReplaceSessionMessages("s1", []Message{m2}); err != nil {
		t.Fatalf("ReplaceSessionMessages: %v", err)
	}

	var names []string
	rows, err := d.Reader().Query(
		`SELECT tool_name FROM tool_calls
		 WHERE session_id = ? ORDER BY id`, "s1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("got %d tool_calls, want 2", len(names))
	}
	if names[0] != "Bash" {
		t.Errorf("names[0] = %q, want Bash", names[0])
	}
	if names[1] != "Write" {
		t.Errorf("names[1] = %q, want Write", names[1])
	}
}

func TestToolCallsNoToolCalls(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p")
	insertMessages(t, d, userMsg("s1", 0, "hello"))

	var count int
	if err := d.Reader().QueryRow(
		"SELECT COUNT(*) FROM tool_calls WHERE session_id = ?",
		"s1",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("tool_calls count = %d, want 0", count)
	}
}

func TestToolCallsMixedSessionsOverlappingOrdinals(t *testing.T) {
	d := testDB(t)

	insertSession(t, d, "s1", "p")
	insertSession(t, d, "s2", "p")

	// Both sessions have ordinal 0 with tool calls
	m1 := asstMsg("s1", 0, "[Read]")
	m1.HasToolUse = true
	m1.ToolCalls = []ToolCall{
		{SessionID: "s1", ToolName: "Read", Category: "Read"},
	}
	m2 := asstMsg("s2", 0, "[Bash]")
	m2.HasToolUse = true
	m2.ToolCalls = []ToolCall{
		{SessionID: "s2", ToolName: "Bash", Category: "Bash"},
	}

	insertMessages(t, d, m1, m2)

	// Verify each tool_call.message_id joins to the correct
	// session: Read→s1, Bash→s2.
	rows, err := d.Reader().Query(`
		SELECT tc.tool_name, tc.session_id, m.session_id
		FROM tool_calls tc
		JOIN messages m ON m.id = tc.message_id
		ORDER BY tc.tool_name`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	type row struct {
		toolName, tcSession, msgSession string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.toolName, &r.tcSession, &r.msgSession,
		); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d tool_calls, want 2", len(got))
	}
	// Bash should be linked to s2
	if got[0].toolName != "Bash" ||
		got[0].tcSession != "s2" ||
		got[0].msgSession != "s2" {
		t.Errorf("Bash row = %+v", got[0])
	}
	// Read should be linked to s1
	if got[1].toolName != "Read" ||
		got[1].tcSession != "s1" ||
		got[1].msgSession != "s1" {
		t.Errorf("Read row = %+v", got[1])
	}
}

func TestResolveToolCallsPanicsOnLengthMismatch(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "resolveToolCalls") {
			t.Errorf("unexpected panic value: %v", r)
		}
	}()

	msgs := []Message{
		{SessionID: "s1", Ordinal: 0, Role: "user"},
		{SessionID: "s1", Ordinal: 1, Role: "assistant"},
	}
	ids := []int64{1} // length mismatch
	resolveToolCalls(msgs, ids)
}

func TestToolCallNewColumns(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "proj")
	insertMessages(t, d, Message{
		SessionID:     "s1",
		Ordinal:       0,
		Role:          "assistant",
		Content:       "[Read: main.go]",
		ContentLength: 15,
		Timestamp:     tsZero,
		ToolCalls: []ToolCall{{
			SessionID:           "s1",
			ToolName:            "Read",
			Category:            "Read",
			ToolUseID:           "toolu_abc",
			InputJSON:           `{"file_path":"main.go"}`,
			ResultContentLength: 500,
		}},
	})

	var toolUseID, inputJSON sql.NullString
	var resultLen sql.NullInt64
	err := d.Reader().QueryRow(`
        SELECT tool_use_id, input_json, result_content_length
        FROM tool_calls WHERE session_id = 's1'
    `).Scan(&toolUseID, &inputJSON, &resultLen)
	if err != nil {
		t.Fatalf("query tool_calls: %v", err)
	}
	if !toolUseID.Valid || toolUseID.String != "toolu_abc" {
		t.Errorf("tool_use_id = %v, want toolu_abc", toolUseID)
	}
	if !inputJSON.Valid || inputJSON.String != `{"file_path":"main.go"}` {
		t.Errorf("input_json = %v", inputJSON)
	}
	if !resultLen.Valid || resultLen.Int64 != 500 {
		t.Errorf("result_content_length = %v, want 500", resultLen)
	}
}

func TestToolCallSkillName(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "proj")
	insertMessages(t, d, Message{
		SessionID:     "s1",
		Ordinal:       0,
		Role:          "assistant",
		Content:       "[Skill: superpowers:brainstorming]",
		ContentLength: 34,
		Timestamp:     tsZero,
		ToolCalls: []ToolCall{{
			SessionID: "s1",
			ToolName:  "Skill",
			Category:  "Tool",
			ToolUseID: "toolu_skill1",
			InputJSON: `{"skill":"superpowers:brainstorming"}`,
			SkillName: "superpowers:brainstorming",
		}},
	})

	var skillName sql.NullString
	err := d.Reader().QueryRow(`
        SELECT skill_name FROM tool_calls WHERE session_id = 's1'
    `).Scan(&skillName)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !skillName.Valid || skillName.String != "superpowers:brainstorming" {
		t.Errorf("skill_name = %v, want superpowers:brainstorming", skillName)
	}
}

func TestGetMessagesReturnsToolCalls(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "proj")
	insertMessages(t, d, Message{
		SessionID:     "s1",
		Ordinal:       0,
		Role:          "assistant",
		Content:       "[Skill: superpowers:brainstorming]",
		ContentLength: 34,
		Timestamp:     tsZero,
		HasToolUse:    true,
		ToolCalls: []ToolCall{{
			SessionID:           "s1",
			ToolName:            "Skill",
			Category:            "Tool",
			ToolUseID:           "toolu_s1",
			InputJSON:           `{"skill":"superpowers:brainstorming"}`,
			SkillName:           "superpowers:brainstorming",
			ResultContentLength: 42,
		}},
	})

	msgs, err := d.GetMessages(
		context.Background(), "s1", 0, 100, true,
	)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if len(msgs[0].ToolCalls) != 1 {
		t.Fatalf("got %d tool_calls, want 1",
			len(msgs[0].ToolCalls))
	}
	tc := msgs[0].ToolCalls[0]
	if tc.ToolName != "Skill" {
		t.Errorf("ToolName = %q", tc.ToolName)
	}
	if tc.SkillName != "superpowers:brainstorming" {
		t.Errorf("SkillName = %q", tc.SkillName)
	}
	if tc.InputJSON != `{"skill":"superpowers:brainstorming"}` {
		t.Errorf("InputJSON = %q", tc.InputJSON)
	}
	if tc.ResultContentLength != 42 {
		t.Errorf("ResultContentLength = %d", tc.ResultContentLength)
	}
}

func TestGetAllMessagesReturnsToolCallsAcrossBatches(t *testing.T) {
	d := testDB(t)
	insertSession(t, d, "s1", "proj")

	total := attachToolCallBatchSize + 25
	msgs := make([]Message, 0, total)
	for i := range total {
		content := fmt.Sprintf("[Read: file-%d.txt]", i)
		msgs = append(msgs, Message{
			SessionID:     "s1",
			Ordinal:       i,
			Role:          "assistant",
			Content:       content,
			ContentLength: len(content),
			Timestamp:     tsZero,
			HasToolUse:    true,
			ToolCalls: []ToolCall{{
				SessionID: "s1",
				ToolName:  "Read",
				Category:  "Read",
				ToolUseID: fmt.Sprintf("toolu_%d", i),
			}},
		})
	}
	insertMessages(t, d, msgs...)

	got, err := d.GetAllMessages(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetAllMessages: %v", err)
	}
	if len(got) != total {
		t.Fatalf("got %d messages, want %d", len(got), total)
	}

	for i := range total {
		if len(got[i].ToolCalls) != 1 {
			t.Fatalf("msg %d: got %d tool_calls, want 1",
				i, len(got[i].ToolCalls))
		}
		if got[i].ToolCalls[0].ToolUseID != fmt.Sprintf("toolu_%d", i) {
			t.Fatalf("msg %d: tool_use_id = %q, want %q",
				i, got[i].ToolCalls[0].ToolUseID,
				fmt.Sprintf("toolu_%d", i))
		}
	}
}

func TestFTSBackfill(t *testing.T) {
	dCheck := testDB(t)
	requireFTS(t, dCheck)
	dCheck.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "backfill.db")

	// 1. Create DB and drop FTS to simulate "old" DB or broken state
	d1, err := Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	// Use writer directly to ensure it happens
	if _, err := d1.writer.Exec("DROP TABLE IF EXISTS messages_fts"); err != nil {
		t.Fatalf("dropping fts: %v", err)
	}
	// Also drop triggers, otherwise inserts will fail
	for _, tr := range []string{"messages_ai", "messages_ad", "messages_au"} {
		if _, err := d1.writer.Exec("DROP TRIGGER IF EXISTS " + tr); err != nil {
			t.Fatalf("dropping trigger %s: %v", tr, err)
		}
	}

	// 2. Insert messages while FTS is missing
	insertSession(t, d1, "s1", "proj")
	insertMessages(t, d1, userMsg("s1", 0, "unique_keyword"))

	if err := d1.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}

	// 3. Re-open. This should detect missing FTS, create it, and backfill.
	d2, err := Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer d2.Close()

	if !d2.HasFTS() {
		t.Fatal("FTS should be available after re-open")
	}

	// 4. Verify search finds the message
	page, err := d2.Search(context.Background(), SearchFilter{
		Query: "unique_keyword",
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(page.Results))
	}
	if page.Results[0].SessionID != "s1" {
		t.Errorf("result session_id = %q, want s1", page.Results[0].SessionID)
	}
}

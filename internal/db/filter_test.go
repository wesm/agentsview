package db

import (
	"testing"
)

func TestPruneFilterZeroValue(t *testing.T) {
	f := PruneFilter{}

	if f.HasFilters() {
		t.Error("HasFilters() returned true for zero value")
	}

	d := testDB(t)

	insertSession(t, d, "s1", "p", func(s *Session) {
		s.MessageCount = 0
	})
	insertSession(t, d, "s2", "p", func(s *Session) {
		s.MessageCount = 5
	})

	_, err := d.FindPruneCandidates(f)
	requireErrContains(t, err, "at least one filter is required")
}

func TestSessionFilterDateFields(t *testing.T) {
	d := testDB(t)
	sessionSet(t, d)

	tests := []struct {
		name   string
		filter SessionFilter
		want   int
	}{
		{
			name: "ExactDate",
			filter: filterWith(func(f *SessionFilter) {
				f.Date = "2024-06-01"
			}),
			want: 1,
		},
		{
			name: "DateRange",
			filter: filterWith(func(f *SessionFilter) {
				f.DateFrom = "2024-06-01"
				f.DateTo = "2024-06-02"
			}),
			want: 2,
		},
		{
			name: "DateFrom",
			filter: filterWith(func(f *SessionFilter) {
				f.DateFrom = "2024-06-02"
			}),
			want: 2,
		},
		{
			name: "DateTo",
			filter: filterWith(func(f *SessionFilter) {
				f.DateTo = "2024-06-01"
			}),
			want: 1,
		},
		{
			name: "MinMessages",
			filter: filterWith(func(f *SessionFilter) {
				f.MinMessages = 10
			}),
			want: 2,
		},
		{
			name: "MaxMessages",
			filter: filterWith(func(f *SessionFilter) {
				f.MaxMessages = 10
			}),
			want: 1,
		},
		{
			name: "CombinedDateAndMessages",
			filter: filterWith(func(f *SessionFilter) {
				f.DateFrom = "2024-06-02"
				f.MinMessages = 20
			}),
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireCount(t, d, tt.filter, tt.want)
		})
	}
}

func TestSessionFilterActiveSince(t *testing.T) {
	d := testDB(t)

	// Session that started and ended long ago.
	insertSession(t, d, "old", "proj", func(s *Session) {
		s.StartedAt = Ptr("2024-01-01T10:00:00Z")
		s.EndedAt = Ptr("2024-01-01T11:00:00Z")
		s.MessageCount = 5
	})

	// Session that started long ago but ended recently.
	insertSession(t, d, "recent-end", "proj", func(s *Session) {
		s.StartedAt = Ptr("2024-01-01T10:00:00Z")
		s.EndedAt = Ptr("2024-06-03T10:00:00Z")
		s.MessageCount = 5
	})

	// Session that started recently, no ended_at.
	insertSession(t, d, "recent-start", "proj", func(s *Session) {
		s.StartedAt = Ptr("2024-06-03T08:00:00Z")
		s.MessageCount = 5
	})

	// Session with no started_at or ended_at, only created_at
	// (created_at defaults to now in schema, but here we set
	// started_at to nil; the fallback is created_at).
	insertSession(t, d, "no-times", "proj", func(s *Session) {
		s.MessageCount = 5
	})

	// no-times has created_at = now (schema default), so it
	// matches any past cutoff.
	tests := []struct {
		name        string
		activeSince string
		want        int
	}{
		{
			name:        "ExcludesOldEndedAt",
			activeSince: "2024-06-03T00:00:00Z",
			want:        3, // old excluded; recent-end, recent-start, no-times match
		},
		{
			name:        "NarrowCutoffOnlyCreatedAtNow",
			activeSince: "2024-06-03T12:00:00Z",
			want:        1, // only no-times (created_at=now) survives
		},
		{
			name:        "IncludesAll",
			activeSince: "2024-01-01T00:00:00Z",
			want:        4,
		},
		{
			name:        "EmptyMeansNoFilter",
			activeSince: "",
			want:        4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := filterWith(func(f *SessionFilter) {
				f.ActiveSince = tt.activeSince
			})
			requireCount(t, d, f, tt.want)
		})
	}
}

func TestActiveSinceUsesEndedAtOverStartedAt(t *testing.T) {
	d := testDB(t)

	// Session started in January, ended in June.
	// A date_from filter for June would miss it (started too early),
	// but active_since should catch it via ended_at.
	insertSession(t, d, "s1", "proj", func(s *Session) {
		s.StartedAt = Ptr("2024-01-15T10:00:00Z")
		s.EndedAt = Ptr("2024-06-15T10:00:00Z")
		s.MessageCount = 5
	})

	// date_from filters on started_at — should NOT match.
	dateFromFilter := filterWith(func(f *SessionFilter) {
		f.DateFrom = "2024-06-01"
	})
	requireCount(t, d, dateFromFilter, 0)

	// active_since filters on ended_at — SHOULD match.
	activeSinceFilter := filterWith(func(f *SessionFilter) {
		f.ActiveSince = "2024-06-01T00:00:00Z"
	})
	requireCount(t, d, activeSinceFilter, 1)
}

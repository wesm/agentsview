package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyncStats_RecordSkip(t *testing.T) {
	tests := []struct {
		name  string
		skips int
		want  int
	}{
		{"zero skips", 0, 0},
		{"multiple skips", 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SyncStats
			for i := 0; i < tt.skips; i++ {
				s.RecordSkip()
			}
			assert.Equal(t, tt.want, s.Skipped)
			assert.Equal(t, 0, s.Synced)
		})
	}
}

func TestSyncStats_RecordSynced(t *testing.T) {
	tests := []struct {
		name   string
		synced []int
		want   int
	}{
		{"zero synced", []int{}, 0},
		{"multiple synced", []int{5, 3}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SyncStats
			for _, v := range tt.synced {
				s.RecordSynced(v)
			}
			assert.Equal(t, 0, s.Skipped)
			assert.Equal(t, tt.want, s.Synced)
		})
	}
}

func TestProgress_Percent(t *testing.T) {
	tests := []struct {
		name string
		p    Progress
		want float64
	}{
		{
			name: "zero total",
			p:    Progress{SessionsTotal: 0, SessionsDone: 0},
			want: 0,
		},
		{
			name: "half done",
			p:    Progress{SessionsTotal: 10, SessionsDone: 5},
			want: 50,
		},
		{
			name: "all done",
			p:    Progress{SessionsTotal: 4, SessionsDone: 4},
			want: 100,
		},
		{
			name: "one third",
			p:    Progress{SessionsTotal: 3, SessionsDone: 1},
			want: 33.333333,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Percent()
			assert.InDelta(t, tt.want, got, 1e-4)
		})
	}
}

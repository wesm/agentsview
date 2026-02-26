package server

import (
	"net/http"
	"testing"
)

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		param      string
		wantVal    int
		wantOK     bool
		wantStatus int
	}{
		{
			name:       "absent param returns zero",
			query:      "",
			param:      "limit",
			wantVal:    0,
			wantOK:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid integer",
			query:      "limit=42",
			param:      "limit",
			wantVal:    42,
			wantOK:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "negative integer",
			query:      "limit=-5",
			param:      "limit",
			wantVal:    -5,
			wantOK:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-numeric returns 400",
			query:      "limit=abc",
			param:      "limit",
			wantVal:    0,
			wantOK:     false,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "float returns 400",
			query:      "limit=3.5",
			param:      "limit",
			wantVal:    0,
			wantOK:     false,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, r := newTestRequest(t, tt.query)

			val, ok := parseIntParam(w, r, tt.param)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if val != tt.wantVal {
				t.Errorf("val = %d, want %d", val, tt.wantVal)
			}
			if w.Code != tt.wantStatus {
				t.Errorf(
					"status = %d, want %d", w.Code, tt.wantStatus,
				)
			}
		})
	}
}

func TestClampLimit(t *testing.T) {
	const max = 1000
	const defaultLimit = 100
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero uses default", 0, defaultLimit},
		{"negative uses default", -1, defaultLimit},
		{"within range", defaultLimit / 2, defaultLimit / 2},
		{"at max", max, max},
		{"exceeds max", max + 1, max},
		{"default itself", defaultLimit, defaultLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampLimit(tt.limit, defaultLimit, max)
			if got != tt.want {
				t.Errorf("clampLimit(%d, %d, %d) = %d, want %d",
					tt.limit, defaultLimit, max, got, tt.want)
			}
		})
	}
}

package server

import (
	"net/http"
	"testing"
)

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		param   string
		wantVal int
		wantOK  bool
		wantErr int // expected HTTP status, 0 = no error
	}{
		{
			name:    "absent param returns zero",
			query:   "",
			param:   "limit",
			wantVal: 0,
			wantOK:  true,
		},
		{
			name:    "valid integer",
			query:   "limit=42",
			param:   "limit",
			wantVal: 42,
			wantOK:  true,
		},
		{
			name:    "negative integer",
			query:   "limit=-5",
			param:   "limit",
			wantVal: -5,
			wantOK:  true,
		},
		{
			name:    "non-numeric returns 400",
			query:   "limit=abc",
			param:   "limit",
			wantVal: 0,
			wantOK:  false,
			wantErr: http.StatusBadRequest,
		},
		{
			name:    "float returns 400",
			query:   "limit=3.5",
			param:   "limit",
			wantVal: 0,
			wantOK:  false,
			wantErr: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, r := newTestRequest(t, tt.query)

			val, ok := parseIntParam(w, r, tt.param)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if val != tt.wantVal {
				t.Fatalf("val = %d, want %d", val, tt.wantVal)
			}
			if tt.wantErr != 0 && w.Code != tt.wantErr {
				t.Fatalf(
					"status = %d, want %d", w.Code, tt.wantErr,
				)
			}
		})
	}
}

func TestClampLimit(t *testing.T) {
	const max = 1000
	tests := []struct {
		name         string
		limit        int
		defaultLimit int
		want         int
	}{
		{"zero uses default", 0, 100, 100},
		{"negative uses default", -1, 100, 100},
		{"within range", 50, 100, 50},
		{"at max", max, 100, max},
		{"exceeds max", max + 1, 100, max},
		{"default itself", 100, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampLimit(tt.limit, tt.defaultLimit, max)
			if got != tt.want {
				t.Fatalf("clampLimit(%d, %d, %d) = %d, want %d",
					tt.limit, tt.defaultLimit, max, got, tt.want)
			}
		})
	}
}

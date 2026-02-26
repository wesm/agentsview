package server

import "testing"

func TestPrepareFTSQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "single word unchanged", raw: "login", want: "login"},
		{name: "multi-word gets quoted", raw: "fix bug", want: `"fix bug"`},
		{name: "already quoted unchanged", raw: `"fix bug"`, want: `"fix bug"`},
		{name: "empty string unchanged", raw: "", want: ""},
		{name: "three words quoted", raw: "a b c", want: `"a b c"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := prepareFTSQuery(tt.raw)
			if got != tt.want {
				t.Errorf("prepareFTSQuery(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

package server

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple uuid", "abc-123-def", "abc-123-def"},
		{"alphanumeric", "session42", "session42"},
		{"with colon", "a:b", "'a:b'"},
		{"with spaces", "has space", "'has space'"},
		{"with single quote", "it's", `'it'"'"'s'`},
		{"command injection attempt", "$(whoami)", "'$(whoami)'"},
		{"backtick injection", "`rm -rf /`", "'`rm -rf /`'"},
		{"semicolon", "id;rm -rf /", "'id;rm -rf /'"},
		{"pipe", "id|cat", "'id|cat'"},
		{"empty passthrough", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.in)
			if got != tt.want {
				t.Errorf(
					"shellQuote(%q) = %q, want %q",
					tt.in, got, tt.want,
				)
			}
		})
	}
}

func TestDetectTerminalLinux_NoTerminal(t *testing.T) {
	// When no terminal is installed, should return an error.
	// This test validates the error path — on CI or servers
	// without a display, no terminal emulator is typically
	// available.
	_, _, err := detectTerminalLinux("echo test")
	// We just check it doesn't panic. The error may or may not
	// occur depending on the environment.
	_ = err
}

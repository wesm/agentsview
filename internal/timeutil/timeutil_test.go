package timeutil

import (
	"testing"
	"time"
)

func ptr(s string) *string {
	return &s
}

func TestPtr(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want *string
	}{
		{
			name: "zero time returns nil",
			in:   time.Time{},
			want: nil,
		},
		{
			name: "non-zero returns RFC3339Nano UTC",
			in:   time.Date(2024, 6, 15, 12, 30, 45, 123000000, time.UTC),
			want: ptr("2024-06-15T12:30:45.123Z"),
		},
		{
			name: "converts to UTC",
			in:   time.Date(2024, 6, 15, 7, 30, 0, 0, time.FixedZone("EST", -5*60*60)),
			want: ptr("2024-06-15T12:30:00Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Ptr(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Errorf("Ptr() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Ptr() returned nil, want %q", *tt.want)
			}
			if *got != *tt.want {
				t.Errorf("Ptr() = %q, want %q", *got, *tt.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero time returns empty", time.Time{}, ""},
		{"non-zero returns RFC3339Nano UTC", time.Date(2024, 6, 15, 12, 30, 45, 0, time.UTC), "2024-06-15T12:30:45Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.in); got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}

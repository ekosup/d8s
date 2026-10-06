package version

import "testing"

func TestString(t *testing.T) {
	tests := []struct {
		name                  string
		version, commit, date string
		want                  string
	}{
		{"release", "0.1.0", "abc1234", "2026-10-06T10:00:00Z", "d8s v0.1.0 (abc1234, 2026-10-06T10:00:00Z)"},
		{"dev build", "dev", "unknown", "unknown", "d8s dev (unknown, unknown)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version, Commit, Date = tt.version, tt.commit, tt.date
			if got := String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

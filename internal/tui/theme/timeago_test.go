package theme

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRelative(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero time is empty", time.Time{}, ""},
		{"future collapses to just now", now.Add(5 * time.Minute), "just now"},
		{"30 seconds ago is just now", now.Add(-30 * time.Second), "just now"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1 minute ago"},
		{"14 minutes ago", now.Add(-14 * time.Minute), "14 minutes ago"},
		{"59 minutes ago", now.Add(-59 * time.Minute), "59 minutes ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1 hour ago"},
		{"23 hours ago", now.Add(-23 * time.Hour), "23 hours ago"},
		{"1 day ago", now.Add(-24 * time.Hour), "1 day ago"},
		{"3 days ago", now.Add(-3 * 24 * time.Hour), "3 days ago"},
		{"29 days ago", now.Add(-29 * 24 * time.Hour), "29 days ago"},
		{"1 month ago", now.Add(-30 * 24 * time.Hour), "1 month ago"},
		{"11 months ago", now.Add(-11 * 30 * 24 * time.Hour), "11 months ago"},
		{"1 year ago", now.Add(-365 * 24 * time.Hour), "1 year ago"},
		{"2 years ago", now.Add(-2 * 365 * 24 * time.Hour), "2 years ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Relative(tt.t, now)

			require.Equal(t, tt.want, got)
		})
	}
}

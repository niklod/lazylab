package theme

import (
	"fmt"
	"time"
)

// Relative renders t as a human-readable offset from now. now is taken as a
// parameter so tests can freeze the clock — production callers pass
// time.Now(). Buckets follow the design wireframes ("14 minutes ago",
// "3 days ago", "just now" for offsets under a minute). Future timestamps
// collapse to "just now" to avoid negative deltas leaking into the UI.
func Relative(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	delta := now.Sub(t)
	if delta < time.Minute {
		return "just now"
	}

	switch {
	case delta < time.Hour:
		return plural(int(delta/time.Minute), "minute")
	case delta < 24*time.Hour:
		return plural(int(delta/time.Hour), "hour")
	case delta < 30*24*time.Hour:
		return plural(int(delta/(24*time.Hour)), "day")
	case delta < 365*24*time.Hour:
		return plural(int(delta/(30*24*time.Hour)), "month")
	}

	return plural(int(delta/(365*24*time.Hour)), "year")
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}

	return fmt.Sprintf("%d %ss ago", n, unit)
}

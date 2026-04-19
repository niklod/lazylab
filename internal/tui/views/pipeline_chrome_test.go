package views

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/niklod/lazylab/internal/models"
)

func TestRenderChromeLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		title  string
		meta   string
		innerW int
		want   func(t *testing.T, got string)
	}{
		{
			name:   "both fit with spacer",
			title:  "Detail · !482",
			meta:   "Pipeline · #91422 · 4m 12s",
			innerW: 80,
			want: func(t *testing.T, got string) {
				t.Helper()
				require.Contains(t, got, "Detail · !482")
				require.Contains(t, got, "Pipeline · #91422")
				require.Equal(t, 80, visibleWidth(got), "chrome line fills innerW")
			},
		},
		{
			name:   "narrow pane falls back to two-space join",
			title:  "Detail · !482",
			meta:   "Pipeline · #91422 · 4m 12s · ↻ auto 5s · updated 2s ago",
			innerW: 20,
			want: func(t *testing.T, got string) {
				t.Helper()
				require.Contains(t, got, "!482")
				require.Contains(t, got, "91422")
			},
		},
		{
			name:   "zero width uses compact form",
			title:  "Log · ✗ e2e",
			meta:   "stage test",
			innerW: 0,
			want: func(t *testing.T, got string) {
				t.Helper()
				require.Contains(t, got, "Log · ✗ e2e")
				require.Contains(t, got, "stage test")
			},
		},
		{
			name:   "empty inputs return empty",
			title:  "",
			meta:   "",
			innerW: 40,
			want: func(t *testing.T, got string) {
				t.Helper()
				require.Empty(t, got)
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.want(t, renderChromeLine(tt.title, tt.meta, tt.innerW))
		})
	}
}

func TestRenderRefreshIndicator(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	t.Run("enabled running shows auto + updated", func(t *testing.T) {
		t.Parallel()
		got := renderRefreshIndicator(pipelineRefreshState{
			enabled:     true,
			interval:    5 * time.Second,
			lastRefresh: now.Add(-2 * time.Second),
		}, now)
		require.Contains(t, got, "\u21BB")
		require.Contains(t, got, "auto")
		require.Contains(t, got, "5s")
		require.Contains(t, got, "updated")
	})

	t.Run("sub-minute updated counts per second", func(t *testing.T) {
		t.Parallel()
		got := renderRefreshIndicator(pipelineRefreshState{
			enabled:     true,
			interval:    5 * time.Second,
			lastRefresh: now.Add(-7 * time.Second),
		}, now)
		require.Contains(t, got, "7s ago", "sub-minute delta shown with per-second resolution")
		require.NotContains(t, got, "just now")
	})

	t.Run("over one minute shows Nm Ss ago", func(t *testing.T) {
		t.Parallel()
		got := renderRefreshIndicator(pipelineRefreshState{
			enabled:     true,
			interval:    5 * time.Second,
			lastRefresh: now.Add(-(2*time.Minute + 3*time.Second)),
		}, now)
		require.Contains(t, got, "2m 3s ago")
	})

	t.Run("user-paused collapses to paused label", func(t *testing.T) {
		t.Parallel()
		got := renderRefreshIndicator(pipelineRefreshState{enabled: false}, now)
		require.Contains(t, got, "paused")
		require.NotContains(t, got, "auto")
	})

	t.Run("paused with reason", func(t *testing.T) {
		t.Parallel()
		got := renderRefreshIndicator(pipelineRefreshState{
			enabled:      false,
			pausedReason: "job finished",
		}, now)
		require.Contains(t, got, "paused — job finished")
	})
}

func TestPipelineStagesMeta(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	p := models.Pipeline{
		ID:        91422,
		Status:    models.PipelineStatusRunning,
		CreatedAt: now.Add(-4 * time.Minute),
	}
	state := pipelineRefreshState{enabled: true, interval: 5 * time.Second, lastRefresh: now.Add(-3 * time.Second)}

	got := pipelineStagesMeta(p, state, now)
	require.Contains(t, got, "#91422")
	require.Contains(t, got, "4m 0s")
	require.Contains(t, got, "\u21BB")
}

func TestRenderRefreshIndicator_StableWidthAcrossTicks(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	state := pipelineRefreshState{enabled: true, interval: 5 * time.Second, lastRefresh: base}

	nows := []time.Time{
		base,
		base.Add(1 * time.Second),
		base.Add(7 * time.Second),
		base.Add(59 * time.Second),
		base.Add(2*time.Minute + 3*time.Second),
	}
	widths := make([]int, 0, len(nows))
	for _, now := range nows {
		widths = append(widths, visibleWidth(renderRefreshIndicator(state, now)))
	}
	for i, w := range widths {
		require.Equal(t, widths[0], w, "tick %d changed width (shift)", i)
	}
}

func TestFormatUpdatedAgo(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		last time.Time
		want string
	}{
		{name: "just now for < 1s", last: base, want: "just now"},
		{name: "1 second", last: base.Add(-1 * time.Second), want: "1s ago"},
		{name: "59 seconds", last: base.Add(-59 * time.Second), want: "59s ago"},
		{name: "60 seconds rolls to 1m", last: base.Add(-60 * time.Second), want: "1m ago"},
		{name: "2m 3s", last: base.Add(-(2*time.Minute + 3*time.Second)), want: "2m 3s ago"},
		{name: "90 minutes rolls to 1h", last: base.Add(-90 * time.Minute), want: "1h ago"},
		{name: "zero last is just now", last: time.Time{}, want: "just now"},
		{name: "future deltas collapse", last: base.Add(5 * time.Second), want: "just now"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, formatUpdatedAgo(tt.last, base))
		})
	}
}

func TestPipelineDurationForMeta_RunningAdvances(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	now := created.Add(3*time.Minute + 15*time.Second)

	got := pipelineDurationForMeta(models.Pipeline{
		Status:    models.PipelineStatusRunning,
		CreatedAt: created,
	}, now)

	require.Equal(t, "3m 15s", got)
}

func TestJobLogMeta_TerminalJobForcesPausedReason(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	dur := 123.0
	job := &models.PipelineJob{
		ID:       77431,
		Name:     "e2e-smoke",
		Stage:    "test",
		Status:   models.PipelineStatusFailed,
		Duration: &dur,
	}
	state := pipelineRefreshState{enabled: true, interval: 5 * time.Second, lastRefresh: now}

	got := jobLogMeta(job, state, now)
	require.Contains(t, got, "stage test")
	require.Contains(t, got, "#77431")
	require.Contains(t, got, "paused — job finished",
		"wireframe pins terminal jobs to the `paused — job finished` refresh indicator")
	require.NotContains(t, strings.ToLower(got), "auto")
}

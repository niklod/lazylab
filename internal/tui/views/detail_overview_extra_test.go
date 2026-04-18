package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/theme"
)

// These focus tests exercise the Overview helpers in isolation. The
// DetailViewSuite covers end-to-end rendering; these pin the branch-level
// behaviour so future edits to a single helper fail fast before surfacing as
// a confusing integration miss.

func TestPipelineSummary_LoadingNotLoaded(t *testing.T) {
	t.Parallel()

	got := pipelineSummary(false, nil, nil)

	require.Contains(t, got, "loading")
	require.Contains(t, got, theme.Dim)
}

func TestPipelineSummary_LoadedNilIsNoPipeline(t *testing.T) {
	t.Parallel()

	got := pipelineSummary(true, nil, nil)

	require.Contains(t, got, "no pipeline")
	require.Contains(t, got, theme.Dim)
}

func TestPipelineSummary_ErrorWrappedInRed(t *testing.T) {
	t.Parallel()

	got := pipelineSummary(true, nil, errors.New("boom"))

	require.Contains(t, got, "error: boom")
	require.Contains(t, got, sgrPrefix(theme.FgErr))
}

func TestPipelineSummary_SuccessShowsDotLabelAndDuration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	pd := &models.PipelineDetail{
		Pipeline: models.Pipeline{
			ID:        91422,
			Status:    models.PipelineStatusSuccess,
			CreatedAt: start,
			UpdatedAt: start.Add(4*time.Minute + 12*time.Second),
		},
	}

	got := pipelineSummary(true, pd, nil)

	require.Contains(t, got, "passed")
	require.Contains(t, got, "#91422")
	require.Contains(t, got, "4m 12s")
	require.Contains(t, got, sgrPrefix(theme.FgOK))
}

func TestPipelineSummary_RunningOmitsDuration(t *testing.T) {
	t.Parallel()

	pd := &models.PipelineDetail{
		Pipeline: models.Pipeline{
			ID:        7,
			Status:    models.PipelineStatusRunning,
			CreatedAt: time.Now().Add(-5 * time.Minute),
			UpdatedAt: time.Now(),
		},
	}

	got := pipelineSummary(true, pd, nil)

	require.Contains(t, got, "#7")
	require.Contains(t, got, "running")
	require.NotContains(t, got, "m 0s")
	require.NotContains(t, got, "·")
}

func TestPipelineDurationText(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		p    models.Pipeline
		want string
	}{
		{
			name: "non-terminal returns empty",
			p:    models.Pipeline{Status: models.PipelineStatusRunning, CreatedAt: start, UpdatedAt: start.Add(10 * time.Second)},
			want: "",
		},
		{
			name: "zero CreatedAt returns empty",
			p:    models.Pipeline{Status: models.PipelineStatusSuccess, CreatedAt: time.Time{}, UpdatedAt: start},
			want: "",
		},
		{
			name: "negative delta returns empty",
			p:    models.Pipeline{Status: models.PipelineStatusSuccess, CreatedAt: start.Add(time.Hour), UpdatedAt: start},
			want: "",
		},
		{
			name: "sub-minute",
			p:    models.Pipeline{Status: models.PipelineStatusSuccess, CreatedAt: start, UpdatedAt: start.Add(45 * time.Second)},
			want: "45s",
		},
		{
			name: "minutes and seconds",
			p:    models.Pipeline{Status: models.PipelineStatusSuccess, CreatedAt: start, UpdatedAt: start.Add(2*time.Minute + 5*time.Second)},
			want: "2m 5s",
		},
		{
			name: "exactly zero delta returns empty",
			p:    models.Pipeline{Status: models.PipelineStatusSuccess, CreatedAt: start, UpdatedAt: start},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pipelineDurationText(tt.p)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestPipelineStatusLabel(t *testing.T) {
	t.Parallel()

	tests := map[models.PipelineStatus]string{
		models.PipelineStatusSuccess: "passed",
		models.PipelineStatusFailed:  "failed",
		models.PipelineStatusRunning: "running",
		models.PipelineStatusManual:  "manual",
	}
	for status, want := range tests {
		require.Equal(t, want, pipelineStatusLabel(status), "status %q", status)
	}
}

func TestPipelineStatusColor(t *testing.T) {
	t.Parallel()

	tests := map[models.PipelineStatus]string{
		models.PipelineStatusSuccess:            theme.FgOK,
		models.PipelineStatusFailed:             theme.FgErr,
		models.PipelineStatusRunning:            theme.FgWarn,
		models.PipelineStatusPending:            theme.FgWarn,
		models.PipelineStatusPreparing:          theme.FgWarn,
		models.PipelineStatusWaitingForResource: theme.FgWarn,
		models.PipelineStatusCanceled:           theme.FgDraft,
		models.PipelineStatusSkipped:            theme.FgDraft,
		models.PipelineStatusCreated:            theme.FgDraft,
		models.PipelineStatusManual:             theme.FgInfo,
		models.PipelineStatusScheduled:          theme.FgInfo,
		models.PipelineStatus("mystery"):        theme.FgDraft,
	}
	for status, want := range tests {
		require.Equal(t, want, pipelineStatusColor(status), "status %q", status)
	}
}

func TestOverviewSubtitle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	merged := now.Add(-2 * time.Hour)
	tests := []struct {
		name            string
		mr              *models.MergeRequest
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "opened with project path",
			mr: &models.MergeRequest{
				IID:         42,
				State:       models.MRStateOpened,
				ProjectPath: "grp/alpha",
				CreatedAt:   now.Add(-1 * time.Hour),
			},
			wantContains: []string{"!42", "grp/alpha", "opened 1 hour ago"},
		},
		{
			name: "empty project path omits project reference",
			mr: &models.MergeRequest{
				IID:       7,
				State:     models.MRStateOpened,
				CreatedAt: now.Add(-3 * 24 * time.Hour),
			},
			wantContains:    []string{"!7", "opened 3 days ago"},
			wantNotContains: []string{"grp/", "project"},
		},
		{
			name: "merged with MergedAt uses merged time",
			mr: &models.MergeRequest{
				IID:       5,
				State:     models.MRStateMerged,
				CreatedAt: now.Add(-5 * 24 * time.Hour),
				MergedAt:  &merged,
			},
			wantContains: []string{"!5", "merged 2 hours ago"},
		},
		{
			name: "merged without MergedAt falls back to CreatedAt",
			mr: &models.MergeRequest{
				IID:       6,
				State:     models.MRStateMerged,
				CreatedAt: now.Add(-24 * time.Hour),
			},
			wantContains: []string{"!6", "merged 1 day ago"},
		},
		{
			name: "closed renders closed verb",
			mr: &models.MergeRequest{
				IID:       9,
				State:     models.MRStateClosed,
				CreatedAt: now.Add(-1 * time.Hour),
			},
			wantContains: []string{"!9", "closed 1 hour ago"},
		},
		{
			name: "zero CreatedAt omits trailing verb",
			mr: &models.MergeRequest{
				IID:   10,
				State: models.MRStateOpened,
			},
			wantContains:    []string{"!10"},
			wantNotContains: []string{"opened", " ago"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := overviewSubtitle(tt.mr, now)

			for _, want := range tt.wantContains {
				require.Contains(t, got, want)
			}
			for _, banned := range tt.wantNotContains {
				require.NotContains(t, got, banned)
			}
		})
	}
}

func TestUpdatedLine_ZeroReturnsEmDash(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

	got := updatedLine(time.Time{}, now)

	require.Contains(t, got, "—")
	require.Contains(t, got, theme.Dim)
}

func TestUpdatedLine_NonZeroWrapsRelativeInDim(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

	got := updatedLine(now.Add(-15*time.Minute), now)

	require.Contains(t, got, "15 minutes ago")
	require.Contains(t, got, theme.Dim)
}

func TestRenderOverview_WithDescription_RendersRuleAndHeader(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		IID:         1,
		Title:       "T",
		State:       models.MRStateOpened,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
		Description: "Line one\nLine two",
	}

	got := renderOverview(overviewState{mr: mr, now: now})

	require.Contains(t, got, detailDescRule)
	require.Contains(t, got, "Description")
	require.Contains(t, got, "   Line one")
	require.Contains(t, got, "   Line two")
}

func TestRenderOverview_EmptyDescription_OmitsRule(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		IID:       1,
		Title:     "T",
		State:     models.MRStateOpened,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}

	got := renderOverview(overviewState{mr: mr, now: now})

	require.NotContains(t, got, detailDescRule)
	require.NotContains(t, got, "Description")
}

func TestWriteRow_PadsKeyToFixedColumn(t *testing.T) {
	t.Parallel()

	var sb strings.Builder

	writeRow(&sb, "Author", "@alice")

	got := sb.String()
	require.Equal(t, " Author       @alice\n", got)
	require.Equal(t, detailKeyWidth+2, strings.Index(got, "@alice"))
}

func TestWriteDescription_IndentsAndTrimsTrailingWhitespace(t *testing.T) {
	t.Parallel()

	var sb strings.Builder

	writeDescription(&sb, "first  \nsecond\r\n\nfourth")

	require.Equal(t, "   first\n   second\n   \n   fourth\n", sb.String())
}

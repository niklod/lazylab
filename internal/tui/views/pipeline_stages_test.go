package views

import (
	"strings"
	"testing"
	"time"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/theme"
)

type PipelineStagesSuite struct {
	suite.Suite
	g *gocui.Gui
	v *PipelineStagesView
}

func (s *PipelineStagesSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	_, err = s.g.SetView(keymap.ViewDetailPipelineStages, 0, 0, 60, 30, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView: %v", err)
	}

	s.v = NewPipelineStages()
}

func (s *PipelineStagesSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *PipelineStagesSuite) detail() *models.PipelineDetail {
	d1, d2 := 12.5, 90.0

	return &models.PipelineDetail{
		Jobs: []models.PipelineJob{
			{ID: 1, Name: "build:bin", Stage: "build", Status: models.PipelineStatusSuccess, Duration: &d1},
			{ID: 2, Name: "test:unit", Stage: "test", Status: models.PipelineStatusFailed, Duration: &d2},
			{ID: 3, Name: "test:lint", Stage: "test", Status: models.PipelineStatusSuccess},
			{ID: 4, Name: "deploy:prod", Stage: "deploy", Status: models.PipelineStatusManual},
		},
	}
}

func (s *PipelineStagesSuite) TestSetDetail_GroupsByStagePreservingAPIOrder() {
	s.v.SetDetail(s.detail())

	rows := s.v.rowsSnapshot()
	var headers, jobs []string
	for _, r := range rows {
		if r.job == nil {
			headers = append(headers, r.text)
		} else {
			jobs = append(jobs, r.job.Name)
		}
	}

	s.Require().Len(headers, 3, "one header per stage")
	s.Require().Contains(headers[0], "build")
	s.Require().Contains(headers[1], "test")
	s.Require().Contains(headers[2], "deploy")
	s.Require().Equal([]string{"build:bin", "test:unit", "test:lint", "deploy:prod"}, jobs)
}

func (s *PipelineStagesSuite) TestSetDetail_CursorLandsOnFirstJob() {
	s.v.SetDetail(s.detail())

	rows := s.v.rowsSnapshot()
	s.Require().NotEmpty(rows)
	s.Require().Equal(1, s.v.Cursor(), "cursor skips the initial stage header")
	s.Require().NotNil(s.v.SelectedJob())
	s.Require().Equal("build:bin", s.v.SelectedJob().Name)
}

func (s *PipelineStagesSuite) TestMoveCursor_SkipsStageHeaders() {
	s.v.SetDetail(s.detail())

	moved := s.v.MoveCursor(1)
	s.Require().True(moved)
	s.Require().Equal("test:unit", s.v.SelectedJob().Name, "next job crosses the stage boundary")

	s.v.MoveCursor(1)
	s.Require().Equal("test:lint", s.v.SelectedJob().Name)

	s.v.MoveCursor(1)
	s.Require().Equal("deploy:prod", s.v.SelectedJob().Name)

	s.Require().False(s.v.MoveCursor(1), "past the last job is a no-op")
	s.Require().Equal("deploy:prod", s.v.SelectedJob().Name)
}

func (s *PipelineStagesSuite) TestMoveCursorToStartEnd() {
	s.v.SetDetail(s.detail())
	s.v.MoveCursorToEnd()
	s.Require().Equal("deploy:prod", s.v.SelectedJob().Name)

	s.v.MoveCursorToStart()
	s.Require().Equal("build:bin", s.v.SelectedJob().Name)
}

func (s *PipelineStagesSuite) TestSetDetail_PreservesCursorOnRefresh() {
	s.v.SetDetail(s.detail())
	s.v.MoveCursor(2)
	s.Require().Equal("test:lint", s.v.SelectedJob().Name, "sanity")

	refreshed := s.detail()
	refreshed.Jobs[2].Status = models.PipelineStatusRunning
	s.v.SetDetail(refreshed)

	s.Require().Equal("test:lint", s.v.SelectedJob().Name,
		"auto-refresh must not yank the cursor back to the first job")
}

func (s *PipelineStagesSuite) TestSetDetail_CursorFallsBackWhenJobDisappears() {
	s.v.SetDetail(s.detail())
	s.v.MoveCursor(3)
	s.Require().Equal("deploy:prod", s.v.SelectedJob().Name)

	s.v.SetDetail(&models.PipelineDetail{
		Jobs: []models.PipelineJob{
			{ID: 99, Name: "fresh", Stage: "build", Status: models.PipelineStatusRunning},
		},
	})

	s.Require().Equal("fresh", s.v.SelectedJob().Name,
		"missing job falls back to first row of the new pipeline")
}

func (s *PipelineStagesSuite) TestSetDetail_NilShowsEmptyHint() {
	s.v.SetDetail(nil)

	s.Require().Equal(0, s.v.RowCount())
	s.Require().Equal(pipelineStagesEmptyHint, s.v.statusSnapshot())
	s.Require().Nil(s.v.SelectedJob())
}

func (s *PipelineStagesSuite) TestSetDetail_NoJobsShowsNoJobsHint() {
	s.v.SetDetail(&models.PipelineDetail{})

	s.Require().Equal(0, s.v.RowCount())
	s.Require().Equal(pipelineStagesNoJobsHint, s.v.statusSnapshot())
}

func (s *PipelineStagesSuite) TestShowLoadingAndError() {
	s.v.SetDetail(s.detail())
	s.Require().Empty(s.v.statusSnapshot())

	s.v.ShowLoading()
	s.Require().Equal(pipelineStagesLoadingHint, s.v.statusSnapshot())
	s.Require().Equal(0, s.v.RowCount())

	s.v.ShowError("boom")
	s.Require().Contains(s.v.statusSnapshot(), "boom")
	s.Require().Contains(s.v.statusSnapshot(), theme.FgErr)
}

func (s *PipelineStagesSuite) TestRender_RendersStagesAndJobs() {
	s.v.SetDetail(s.detail())

	pane, err := s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)
	s.v.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "build")
	s.Require().Contains(buf, "test")
	s.Require().Contains(buf, "build:bin")
	s.Require().Contains(buf, "test:unit")
	s.Require().Contains(buf, "12s")
	s.Require().Contains(buf, "1m 30s")
	s.Require().Regexp(`   \x1b\[[^m]*m\x{2713} build:bin`, buf, "3-space indent + ok colour + glyph")
}

func (s *PipelineStagesSuite) TestRender_StagesFooterContainsSeparatorOverallAndKeybinds() {
	s.v.SetDetail(&models.PipelineDetail{
		Pipeline: models.Pipeline{
			ID:          91422,
			Status:      models.PipelineStatusFailed,
			SHA:         "a3f7b2e0f1d2c3b4a5",
			CreatedAt:   time.Now().Add(-14 * time.Minute),
			TriggeredBy: &models.User{Username: "mira.k"},
		},
		Jobs: []models.PipelineJob{
			{ID: 1, Name: "compile", Stage: "build", Status: models.PipelineStatusSuccess},
		},
	})

	pane, err := s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)
	s.v.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "\u2500", "separator uses U+2500 dashes")
	s.Require().Contains(buf, "Overall", "overall summary label")
	s.Require().Contains(buf, "failed", "overall status label")
	s.Require().Contains(buf, "a3f7b2e", "commit short SHA")
	s.Require().Contains(buf, "ago", "relative age fragment")
	s.Require().Contains(buf, "@mira.k", "triggered-by fragment with username")
	s.Require().Contains(buf, "j/k", "keybind strip shows j/k hint")
	s.Require().Contains(buf, "Enter", "keybind strip shows Enter")
	s.Require().Contains(buf, "retry", "keybind strip shows retry")
}

func (s *PipelineStagesSuite) TestRender_CursorTracksChromeOffset() {
	s.v.SetChrome("Detail · !482", "Pipeline · #91422 · 4m 12s")
	s.v.SetDetail(s.detail())

	pane, err := s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)

	s.v.Render(pane)

	_, cy := pane.Cursor()
	s.Require().Equal(3, cy,
		"cursor y must land on the first job row — chrome+blank add 2 offset, cursor index 1 = 1+2 = 3")

	s.v.MoveCursorToEnd()
	s.v.Render(pane)

	_, cy = pane.Cursor()
	_, origin := pane.Origin()
	lastJobIdx := lastJobRow(s.v.rowsSnapshot())
	s.Require().Equal(lastJobIdx+2-origin, cy,
		"last-job cursor must land on deploy:prod after accounting for chrome offset and scroll origin")
}

func (s *PipelineStagesSuite) TestRender_StatusBranch() {
	pane, err := s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)

	s.v.Render(pane)
	s.Require().Contains(pane.Buffer(), pipelineStagesLoadingHint)
}

func TestFormatJobDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *float64
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "sub-second", in: ptrF(0.4), want: "0s"},
		{name: "5 seconds", in: ptrF(5.9), want: "5s"},
		{name: "59 seconds", in: ptrF(59), want: "59s"},
		{name: "60 seconds", in: ptrF(60), want: "1m 0s"},
		{name: "1m 30s", in: ptrF(90), want: "1m 30s"},
		{name: "1h 1m 1s", in: ptrF(3661), want: "61m 1s"},
		{name: "negative clamps to zero", in: ptrF(-12), want: "0s"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := formatJobDuration(tt.in); got != tt.want {
				t.Fatalf("formatJobDuration(%v) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPipelineJobStatusIcon_CoversAllKnownStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status    models.PipelineStatus
		wantGlyph string
		wantColor string
	}{
		{models.PipelineStatusSuccess, "\u2713", theme.FgOK},
		{models.PipelineStatusFailed, "\u2717", theme.FgErr},
		{models.PipelineStatusRunning, "\u25CF", theme.FgOK},
		{models.PipelineStatusPending, "\u25CB", theme.FgDim},
		{models.PipelineStatusWaitingForResource, "\u25CB", theme.FgDim},
		{models.PipelineStatusPreparing, "\u25CB", theme.FgDim},
		{models.PipelineStatusCreated, "\u25CB", theme.FgDim},
		{models.PipelineStatusSkipped, "\u2298", theme.FgWarn},
		{models.PipelineStatusManual, "\u25B6", theme.FgDim},
		{models.PipelineStatusCanceled, "\u2298", theme.FgErr},
		{models.PipelineStatusScheduled, "\u25B6", theme.FgDim},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()

			got := pipelineJobStatusIcon(tt.status)
			if !strings.Contains(got, tt.wantGlyph) {
				t.Fatalf("icon for %q missing glyph %q: %q", tt.status, tt.wantGlyph, got)
			}
			if !strings.Contains(got, tt.wantColor) {
				t.Fatalf("icon for %q missing color prefix: %q", tt.status, got)
			}
			if !strings.Contains(got, theme.Reset) {
				t.Fatalf("icon for %q missing Reset: %q", tt.status, got)
			}
		})
	}

	unknown := pipelineJobStatusIcon(models.PipelineStatus("WEIRD"))
	if !strings.Contains(unknown, "weird") || !strings.Contains(unknown, theme.FgDim) {
		t.Fatalf("unknown status must render dim lowercased label, got %q", unknown)
	}
}

func ptrF(v float64) *float64 { return &v }

func TestRenderKeybindStrip_StagesMode(t *testing.T) {
	t.Parallel()

	strip := renderKeybindStrip(keybindModePipelineStages)
	for _, want := range []string{"j/k", "Enter", "r", "o", "R", "a", "toggle auto-refresh"} {
		require.Contains(t, strip, want, "stages strip missing %q", want)
	}
	require.Contains(t, strip, theme.FgAccent, "keys painted in accent")
	require.Contains(t, strip, theme.FgDim, "labels painted in dim")
}

func TestRenderKeybindStrip_LogMode(t *testing.T) {
	t.Parallel()

	strip := renderKeybindStrip(keybindModePipelineLog)
	for _, want := range []string{"j/k", "ctrl+d/u", "y", "copy", "Esc", "close"} {
		require.Contains(t, strip, want, "log strip missing %q", want)
	}
}

func TestRenderOverallLine(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		pipeline    models.Pipeline
		triggeredBy *models.User
		wantAll     []string
		wantNone    []string
	}{
		{
			name: "failed with user and sha",
			pipeline: models.Pipeline{
				Status:    models.PipelineStatusFailed,
				SHA:       "a3f7b2e1234567",
				CreatedAt: now.Add(-14 * time.Minute),
			},
			triggeredBy: &models.User{Username: "mira.k"},
			wantAll:     []string{"Overall", "failed", "triggered by", "@mira.k", "commit", "a3f7b2e", "14 minutes ago"},
			wantNone:    []string{"a3f7b2e1"},
		},
		{
			name: "success without user",
			pipeline: models.Pipeline{
				Status:    models.PipelineStatusSuccess,
				SHA:       "abcdefg1234",
				CreatedAt: now.Add(-3 * time.Hour),
			},
			wantAll:  []string{"Overall", "passed", "commit", "abcdefg", "3 hours ago"},
			wantNone: []string{"triggered by"},
		},
		{
			name: "short sha preserved",
			pipeline: models.Pipeline{
				Status:    models.PipelineStatusRunning,
				SHA:       "abc",
				CreatedAt: now.Add(-45 * time.Second),
			},
			wantAll: []string{"Overall", "running", "commit", "abc", "just now"},
		},
		{
			name: "missing sha omits commit fragment",
			pipeline: models.Pipeline{
				Status:    models.PipelineStatusCanceled,
				CreatedAt: now.Add(-2 * time.Hour),
			},
			wantAll:  []string{"Overall", "canceled", "2 hours ago"},
			wantNone: []string{"commit"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := renderOverallLine(tt.pipeline, tt.triggeredBy, now)
			for _, want := range tt.wantAll {
				require.Contains(t, got, want, "missing %q", want)
			}
			for _, none := range tt.wantNone {
				require.NotContains(t, got, none, "unexpected %q", none)
			}
		})
	}
}

// rowsSnapshot returns a defensive copy of the row slice so tests can
// inspect internal structure without holding the view mutex.
func (p *PipelineStagesView) rowsSnapshot() []stageRow {
	p.mu.Lock()
	defer p.mu.Unlock()

	cp := make([]stageRow, len(p.rows))
	copy(cp, p.rows)

	return cp
}

// statusSnapshot exposes the cached status message to tests.
func (p *PipelineStagesView) statusSnapshot() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.status
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestPipelineStagesSuite(t *testing.T) {
	suite.Run(t, new(PipelineStagesSuite))
}

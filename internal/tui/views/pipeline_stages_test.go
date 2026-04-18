package views

import (
	"strings"
	"testing"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
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
	s.Require().Contains(s.v.statusSnapshot(), ansiRed)
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

	statuses := []models.PipelineStatus{
		models.PipelineStatusSuccess,
		models.PipelineStatusFailed,
		models.PipelineStatusRunning,
		models.PipelineStatusPending,
		models.PipelineStatusWaitingForResource,
		models.PipelineStatusPreparing,
		models.PipelineStatusCreated,
		models.PipelineStatusCanceled,
		models.PipelineStatusSkipped,
		models.PipelineStatusManual,
		models.PipelineStatusScheduled,
	}
	for _, st := range statuses {
		st := st
		t.Run(string(st), func(t *testing.T) {
			t.Parallel()

			got := pipelineJobStatusIcon(st)
			if !strings.Contains(got, ansiReset) {
				t.Fatalf("icon for %q missing ansiReset: %q", st, got)
			}
		})
	}

	unknown := pipelineJobStatusIcon(models.PipelineStatus("WEIRD"))
	if !strings.Contains(unknown, "weird") || !strings.Contains(unknown, ansiDim) {
		t.Fatalf("unknown status must render dim lowercased label, got %q", unknown)
	}
}

func ptrF(v float64) *float64 { return &v }

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

package views

import (
	"context"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/pkg/browser"
	"github.com/niklod/lazylab/internal/pkg/clipboard"
)

type PipelineActionsSuite struct {
	suite.Suite
	g *gocui.Gui
	d *DetailView
}

func (s *PipelineActionsSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g
	s.d = NewDetail(g, &appcontext.AppContext{})
}

func (s *PipelineActionsSuite) TearDownTest() {
	if s.d != nil {
		s.d.mu.Lock()
		s.d.stopPipelineRefreshLocked()
		s.d.mu.Unlock()
	}
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *PipelineActionsSuite) TestRetryCurrentJob_NilProject_IsNoop() {
	err := s.d.RetryCurrentJob(context.Background(), nil)
	s.Require().NoError(err)
}

func (s *PipelineActionsSuite) TestRetryCurrentJob_NoSelectedJob_IsNoop() {
	err := s.d.RetryCurrentJob(context.Background(), &models.Project{ID: 11})
	s.Require().NoError(err, "no cursor → no-op, not error")
}

func (s *PipelineActionsSuite) TestOpenCurrentJobInBrowser_NoSelection_IsNoop() {
	err := s.d.OpenCurrentJobInBrowser()
	s.Require().NoError(err)
}

func (s *PipelineActionsSuite) TestOpenCurrentJobInBrowser_DispatchesURL() {
	job := &models.PipelineJob{ID: 1, Name: "e2e", Status: models.PipelineStatusFailed, WebURL: "https://gitlab.example/j/1"}
	s.d.pipelineStages.SetDetail(&models.PipelineDetail{
		Jobs: []models.PipelineJob{*job},
	})

	var captured string
	restore := browser.SetOpenFunc(func(url string) error {
		captured = url

		return nil
	})
	defer restore()

	s.Require().NoError(s.d.OpenCurrentJobInBrowser())
	s.Require().Equal(job.WebURL, captured)
}

func (s *PipelineActionsSuite) TestCopyLogBody_WritesSanitizedTraceToClipboard() {
	trace := "\x1b[31mFATAL: boom\x1b[0m\nline 2\n"
	s.d.jobLog.SetJob(&models.PipelineJob{
		ID: 7, Name: "unit", Stage: "test", Status: models.PipelineStatusFailed,
	}, trace)

	fake := &clipboard.Fake{}
	s.d.SetClipboard(fake)

	s.Require().NoError(s.d.CopyLogBody())
	s.Require().Contains(fake.Text, "FATAL: boom", "body is copied")
	s.Require().NotContains(fake.Text, "\x1b[", "SGR escapes are stripped")
	s.Require().NotContains(fake.Text, "unit", "header stays in pane chrome, not clipboard")
}

func (s *PipelineActionsSuite) TestToggleAutoRefresh_PersistsFlip() {
	s.Require().True(s.d.AutoRefreshEnabled())
	s.d.ToggleAutoRefresh()
	s.Require().False(s.d.AutoRefreshEnabled())
	s.d.ToggleAutoRefresh()
	s.Require().True(s.d.AutoRefreshEnabled())
}

//nolint:paralleltest // gocui simulator is process-global.
func TestPipelineActionsSuite(t *testing.T) {
	suite.Run(t, new(PipelineActionsSuite))
}

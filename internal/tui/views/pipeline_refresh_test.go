package views

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/models"
)

// PipelineRefreshSuite covers the auto-refresh goroutine lifecycle — the
// ticker pause/resume toggles, the interval-reset signal channel, and
// cancellation on tab leave. Network-side behaviour (invalidate + refetch)
// is covered by the e2e pipeline tests in tests/e2e.
type PipelineRefreshSuite struct {
	suite.Suite
	g *gocui.Gui
	d *DetailView
}

func (s *PipelineRefreshSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g
	s.d = NewDetail(g, &appcontext.AppContext{})
}

func (s *PipelineRefreshSuite) TearDownTest() {
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

func (s *PipelineRefreshSuite) TestInitialRefreshStateIsEnabled() {
	s.Require().True(s.d.AutoRefreshEnabled(), "DetailView constructor seeds enabled=true")
}

func (s *PipelineRefreshSuite) TestToggleAutoRefreshFlipsState() {
	s.Require().True(s.d.AutoRefreshEnabled())

	s.d.ToggleAutoRefresh()
	s.Require().False(s.d.AutoRefreshEnabled())

	s.d.ToggleAutoRefresh()
	s.Require().True(s.d.AutoRefreshEnabled())
}

func (s *PipelineRefreshSuite) TestStartPipelineRefresh_NoProject_IsNoop() {
	s.d.startPipelineRefresh(nil, nil)

	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.Require().Nil(s.d.pipelineRefreshCtl, "no goroutines spawn without project/mr")
}

func (s *PipelineRefreshSuite) TestApplyPipelineRefresh_KeepsFastIntervalOnTerminal() {
	project := &models.Project{ID: 11}
	mr := &models.MergeRequest{IID: 5}

	s.d.startPipelineRefresh(project, mr)
	s.d.mu.Lock()
	seq := s.d.pipelineSeq
	s.Require().Equal(pipelineRefreshFast, s.d.pipelineRefresh.interval)
	s.d.mu.Unlock()

	s.d.applyPipelineRefresh(seq, &models.PipelineDetail{
		Pipeline: models.Pipeline{Status: models.PipelineStatusSuccess},
	}, nil)

	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.Require().Equal(pipelineRefreshFast, s.d.pipelineRefresh.interval,
		"interval stays at 5s regardless of terminal status (per user preference)")
	s.Require().False(s.d.pipelineRefresh.lastRefresh.IsZero(), "lastRefresh updated on apply")
}

func (s *PipelineRefreshSuite) TestStopPipelineRefreshCancelsGoroutines() {
	project := &models.Project{ID: 11}
	mr := &models.MergeRequest{IID: 5}

	before := runtime.NumGoroutine()
	s.d.startPipelineRefresh(project, mr)

	s.d.mu.Lock()
	s.d.stopPipelineRefreshLocked()
	s.d.mu.Unlock()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.Require().LessOrEqual(runtime.NumGoroutine(), before+1,
		"refresh goroutines must exit on cancel (tolerate ±1 runtime-owned)")
}

func (s *PipelineRefreshSuite) TestForceRefreshPipeline_NoProjectIsNoop() {
	// Nothing set up — should not panic or block.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	s.d.ForceRefreshPipeline(ctx)
}

//nolint:paralleltest // gocui simulator is process-global.
func TestPipelineRefreshSuite(t *testing.T) {
	suite.Run(t, new(PipelineRefreshSuite))
}

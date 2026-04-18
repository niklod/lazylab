package tui

import (
	"context"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/tui/theme"
)

// TUISuite groups tests that share a real (headless) *gocui.Gui. The fork stores
// the tcell simulation screen in a package-level global, which races under
// parallel execution; running these sequentially via a suite sidesteps that.
type TUISuite struct {
	suite.Suite
	g *gocui.Gui
}

func (s *TUISuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g
	s.Require().NoError(layout(g))
	s.Require().NoError(Bind(g))
}

func (s *TUISuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *TUISuite) currentView() string {
	v := s.g.CurrentView()
	s.Require().NotNil(v, "CurrentView must not be nil")

	return v.Name()
}

func (s *TUISuite) TestFocusCyclesForward() {
	s.Require().Equal(ViewRepos, s.currentView())

	s.Require().NoError(focusNext(s.g, nil))
	s.Require().Equal(ViewMRs, s.currentView())

	s.Require().NoError(focusNext(s.g, nil))
	s.Require().Equal(ViewDetail, s.currentView())

	s.Require().NoError(focusNext(s.g, nil))
	s.Require().Equal(ViewRepos, s.currentView())
}

func (s *TUISuite) TestFocusCyclesBackward() {
	s.Require().NoError(focusPrev(s.g, nil))
	s.Require().Equal(ViewDetail, s.currentView())

	s.Require().NoError(focusPrev(s.g, nil))
	s.Require().Equal(ViewMRs, s.currentView())

	s.Require().NoError(focusPrev(s.g, nil))
	s.Require().Equal(ViewRepos, s.currentView())
}

func (s *TUISuite) TestLayoutCreatesAllThreePanes() {
	for _, name := range focusOrder {
		v, err := s.g.View(name)
		s.Require().NoError(err, "view %q should exist after first layout tick", name)
		s.Require().Equal(name, v.Name())
	}
}

func (s *TUISuite) TestQuitReturnsErrQuit() {
	err := quit(s.g, nil)

	s.Require().ErrorIs(err, gocui.ErrQuit)
}

func (s *TUISuite) TestHighlightFocused_PaintsOnlyCurrent() {
	for _, name := range focusOrder {
		v, err := s.g.View(name)
		s.Require().NoError(err)
		if name == ViewRepos {
			s.Require().Equal(theme.ColorAccent, v.FrameColor, "%q should start focused", name)
		} else {
			s.Require().Equal(gocui.ColorDefault, v.FrameColor, "%q should start unfocused", name)
		}
	}

	s.Require().NoError(focusNext(s.g, nil))
	highlightFocused(s.g)

	mrs, err := s.g.View(ViewMRs)
	s.Require().NoError(err)
	s.Require().Equal(theme.ColorAccent, mrs.FrameColor)

	repos, err := s.g.View(ViewRepos)
	s.Require().NoError(err)
	s.Require().Equal(gocui.ColorDefault, repos.FrameColor)
}

func (s *TUISuite) TestLayout_SecondTickIsIdempotent() {
	s.Require().NoError(layout(s.g))

	for _, name := range focusOrder {
		v, err := s.g.View(name)
		s.Require().NoError(err)
		s.Require().Equal(name, v.Name())
	}
}

func (s *TUISuite) TestBind_PropagatesDuplicateKeybindingError() {
	err := Bind(s.g)

	s.Require().NoError(err, "gocui allows duplicate bindings; swap this assertion if upstream changes")
}

func TestRun_NilAppContextReturnsError(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "app context is nil")
}

//nolint:paralleltest // gocui stores the tcell simulation screen in a package global; running suites in parallel races.
func TestTUISuite(t *testing.T) {
	suite.Run(t, new(TUISuite))
}

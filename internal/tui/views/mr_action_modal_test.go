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

type MRActionModalSuite struct {
	suite.Suite
	g    *gocui.Gui
	pane *gocui.View
	m    *MRActionModal
}

func (s *MRActionModalSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	pv, err := s.g.SetView(keymap.ViewMRActionsModal, 0, 0, ModalWidth, ModalMergeHeight, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView: %v", err)
	}
	s.pane = pv

	s.m = NewMRActionModal()
}

func (s *MRActionModalSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *MRActionModalSuite) mr() *models.MergeRequest {
	return &models.MergeRequest{
		IID:          482,
		Title:        "Add pipeline retry button",
		SourceBranch: "feat/retry-btn",
		TargetBranch: "main",
		State:        models.MRStateOpened,
	}
}

func (s *MRActionModalSuite) TestOpen_ActivatesAndSeedsDefaults() {
	s.m.Open(ModalClose, s.mr())

	s.Require().True(s.m.IsActive())
	s.Require().Equal(ModalClose, s.m.Kind())
	s.Require().False(s.m.DeleteBranch())
	s.Require().False(s.m.Squash())
	s.Require().False(s.m.Busy())
	s.Require().Empty(s.m.ErrMsg())

	s.m.Open(ModalMerge, s.mr())

	s.Require().True(s.m.DeleteBranch(), "delete-branch defaults on for merge")
	s.Require().False(s.m.Squash(), "squash defaults off")
}

func (s *MRActionModalSuite) TestClose_Deactivates() {
	s.m.Open(ModalMerge, s.mr())
	s.m.SetErr("boom")

	s.m.Close()

	s.Require().False(s.m.IsActive())
	s.Require().Nil(s.m.MR())
	s.Require().Empty(s.m.ErrMsg())
	s.Require().False(s.m.Busy())
}

func (s *MRActionModalSuite) TestToggles_FlipMergeOnly() {
	s.m.Open(ModalClose, s.mr())
	s.m.ToggleDeleteBranch()
	s.m.ToggleSquash()

	s.Require().False(s.m.DeleteBranch(), "close kind ignores toggles")
	s.Require().False(s.m.Squash())

	s.m.Open(ModalMerge, s.mr())

	s.m.ToggleDeleteBranch()
	s.Require().False(s.m.DeleteBranch())

	s.m.ToggleDeleteBranch()
	s.Require().True(s.m.DeleteBranch())

	s.m.ToggleSquash()
	s.Require().True(s.m.Squash())

	s.m.ToggleSquash()
	s.Require().False(s.m.Squash())
}

func (s *MRActionModalSuite) TestSetBusy_ClearsError() {
	s.m.Open(ModalClose, s.mr())
	s.m.SetErr("boom")

	s.m.SetBusy(true)
	s.Require().True(s.m.Busy())
	s.Require().Empty(s.m.ErrMsg(), "SetBusy(true) must clear a stale error")

	s.m.SetBusy(false)
	s.Require().False(s.m.Busy())
}

func (s *MRActionModalSuite) TestSetErr_ClearsBusy() {
	s.m.Open(ModalClose, s.mr())
	s.m.SetBusy(true)

	s.m.SetErr("nope")

	s.Require().Equal("nope", s.m.ErrMsg())
	s.Require().False(s.m.Busy())
}

func (s *MRActionModalSuite) TestRender_NilPaneNoPanic() {
	s.m.Open(ModalClose, s.mr())

	s.Require().NotPanics(func() { s.m.Render(nil) })
}

func (s *MRActionModalSuite) TestRender_InactiveDrawsNothing() {
	s.m.Render(s.pane)

	body := s.pane.Buffer()
	s.Require().Empty(strings.TrimSpace(body))
}

func (s *MRActionModalSuite) TestRender_ClosePaintsTitleAndBody() {
	s.m.Open(ModalClose, s.mr())

	s.m.Render(s.pane)

	body := s.pane.Buffer()
	s.Require().Contains(body, "Add pipeline retry button")
	s.Require().Contains(body, "!482")
	s.Require().NotContains(body, "Squash")
	s.Require().NotContains(body, "Delete source branch")
	// Keybind hints live in the global FooterView, not in the modal body.
	s.Require().NotContains(body, "Enter confirm")
	s.Require().NotContains(body, "Esc cancel")
}

func (s *MRActionModalSuite) TestRender_MergePaintsTogglesWithDefaults() {
	s.m.Open(ModalMerge, s.mr())

	s.m.Render(s.pane)

	body := s.pane.Buffer()
	s.Require().Contains(body, "feat/retry-btn")
	s.Require().Contains(body, "main")
	s.Require().Contains(body, "Squash commits")
	s.Require().Contains(body, "Delete source branch")
	s.Require().Contains(body, "[x]", "delete-branch defaults on, must render [x]")
	s.Require().Contains(body, "[ ]", "squash defaults off, must render [ ]")
	// Keybind hints live in the global footer; the modal only renders
	// toggle labels.
	s.Require().NotContains(body, "d delete-branch")
	s.Require().NotContains(body, "s squash")
}

func (s *MRActionModalSuite) TestRender_BusyShowsRunningIndicator() {
	s.m.Open(ModalMerge, s.mr())
	s.m.SetBusy(true)

	s.m.Render(s.pane)

	body := s.pane.Buffer()
	s.Require().Contains(body, "Running…")
}

func (s *MRActionModalSuite) TestRender_ErrMsgAppearsInBody() {
	s.m.Open(ModalClose, s.mr())
	s.m.SetErr("cannot close: something failed")

	s.m.Render(s.pane)

	body := s.pane.Buffer()
	s.Require().Contains(body, "cannot close: something failed")
}

func (s *MRActionModalSuite) TestTitle_MatchesKind() {
	s.m.Open(ModalClose, s.mr())
	s.Require().Equal(modalCloseTitle, s.m.Title())

	s.m.Open(ModalMerge, s.mr())
	s.Require().Equal(modalMergeTitle, s.m.Title())
}

func TestMRActionModalSuite(t *testing.T) { //nolint:paralleltest // gocui headless guis use a shared tcell simulation screen; parallel suites panic on teardown (close of closed channel).
	suite.Run(t, new(MRActionModalSuite))
}

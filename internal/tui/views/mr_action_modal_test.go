package views

import (
	"strings"
	"testing"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/rivo/uniseg"
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
	s.Require().False(s.m.Locked())
	s.Require().Nil(s.m.Snapshot().ErrLines)
}

func (s *MRActionModalSuite) TestOpenGuarded_SetsLockedAndWrapsReason() {
	reason := "Cannot close: MR !10 is merged"
	s.m.OpenGuarded(ModalClose, s.mr(), reason)

	snap := s.m.Snapshot()
	s.Require().True(snap.Active)
	s.Require().True(snap.Locked)
	s.Require().Equal(reason, snap.ErrMsg)
	s.Require().NotEmpty(snap.ErrLines)
}

func (s *MRActionModalSuite) TestOpen_ClearsLockedFromPreviousGuard() {
	s.m.OpenGuarded(ModalClose, s.mr(), "Cannot close: merged")
	s.Require().True(s.m.Locked())

	s.m.Open(ModalClose, s.mr())

	s.Require().False(s.m.Locked(), "fresh Open must drop locked from a prior guard")
	s.Require().Empty(s.m.ErrMsg())
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
	s.Require().NotEmpty(s.m.Snapshot().ErrLines, "precondition: SetErr must populate errLines")

	s.m.SetBusy(true)
	s.Require().True(s.m.Busy())
	s.Require().Empty(s.m.ErrMsg(), "SetBusy(true) must clear a stale error")
	s.Require().Nil(s.m.Snapshot().ErrLines,
		"SetBusy(true) must also drop the cached wrapped lines — otherwise layout would size the pane for a stale err count",
	)

	s.m.SetBusy(false)
	s.Require().False(s.m.Busy())
}

func (s *MRActionModalSuite) TestSetErr_ClearsBusy() {
	s.m.Open(ModalClose, s.mr())
	s.m.SetBusy(true)

	s.m.SetErr("nope")

	s.Require().Equal("nope", s.m.ErrMsg())
	s.Require().False(s.m.Busy())
	s.Require().NotEmpty(s.m.Snapshot().ErrLines, "SetErr must cache the wrapped lines for layout + render")
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
}

func (s *MRActionModalSuite) TestRender_ActionButtonsAlwaysPresentForClose() {
	s.m.Open(ModalClose, s.mr())

	s.m.Render(s.pane)

	body := stripANSI(s.pane.Buffer())
	s.Require().Contains(body, "[ Close (Enter) ]")
	s.Require().Contains(body, "[ Cancel (Esc) ]")
	s.Require().NotContains(body, "[ Merge (Enter) ]", "close modal must not show the merge primary")
}

func (s *MRActionModalSuite) TestRender_ActionButtonsAlwaysPresentForMerge() {
	s.m.Open(ModalMerge, s.mr())

	s.m.Render(s.pane)

	body := stripANSI(s.pane.Buffer())
	s.Require().Contains(body, "[ Merge (Enter) ]")
	s.Require().Contains(body, "[ Cancel (Esc) ]")
	s.Require().NotContains(body, "[ Close (Enter) ]", "merge modal must not show the close primary")
}

func (s *MRActionModalSuite) TestRender_ActionButtonsSurviveBusy() {
	s.m.Open(ModalMerge, s.mr())
	s.m.SetBusy(true)

	s.m.Render(s.pane)

	body := stripANSI(s.pane.Buffer())
	s.Require().Contains(body, "Running…")
	s.Require().Contains(body, "[ Merge (Enter) ]", "buttons must render even while the mutation is in flight")
	s.Require().Contains(body, "[ Cancel (Esc) ]")
}

func (s *MRActionModalSuite) TestRender_ActionButtonsSurviveErr() {
	s.m.Open(ModalMerge, s.mr())
	s.m.SetErr("accept merge request: permission denied")

	s.m.Render(s.pane)

	body := stripANSI(s.pane.Buffer())
	s.Require().Contains(body, "permission denied")
	s.Require().Contains(body, "[ Merge (Enter) ]")
	s.Require().Contains(body, "[ Cancel (Esc) ]")
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

func (s *MRActionModalSuite) TestRender_ErrMsgWrapsWithinInnerWidth() {
	// Craft a realistic long upstream error (no early whitespace boundary so
	// the wrap must honour modalInnerWidth).
	long := "gitlab: accept merge request !482: PUT https://gitlab.example.com/api/v4/projects/1234/merge_requests/482/merge: 405 Method Not Allowed — branch cannot be merged while pipeline is pending"
	s.m.Open(ModalMerge, s.mr())
	s.m.SetErr(long)

	// Resize the pane to match the dynamic height layout.go would have picked.
	snap := s.m.Snapshot()
	want := ModalHeight(ModalMerge, len(snap.ErrLines))
	_, err := s.g.SetView(keymap.ViewMRActionsModal, 0, 0, ModalWidth, want, 0)
	s.Require().NoError(err)
	pv, err := s.g.View(keymap.ViewMRActionsModal)
	s.Require().NoError(err)

	s.m.Render(pv)

	s.Require().GreaterOrEqual(len(snap.ErrLines), 2, "long error must wrap into multiple lines")
	for i, line := range snap.ErrLines {
		w := uniseg.StringWidth(line)
		s.Require().LessOrEqualf(w, modalInnerWidth,
			"err line %d (%q) width=%d exceeds inner width %d", i, line, w, modalInnerWidth)
	}
}

func (s *MRActionModalSuite) TestModalHeight_GrowsWithErrLines() {
	s.Require().Equal(ModalCloseHeight, ModalHeight(ModalClose, 0))
	s.Require().Equal(ModalMergeHeight, ModalHeight(ModalMerge, 0))
	// Once the dynamic sum crosses the floor constants the pane grows one
	// row per extra wrapped err line.
	s.Require().Greater(ModalHeight(ModalMerge, 20), ModalHeight(ModalMerge, 0))
	s.Require().Greater(ModalHeight(ModalClose, 20), ModalHeight(ModalClose, 0))
	s.Require().Equal(
		1,
		ModalHeight(ModalMerge, 21)-ModalHeight(ModalMerge, 20),
		"above the floor, each extra wrapped err line adds exactly one pane row",
	)
}

func (s *MRActionModalSuite) TestWrapErrMsg_HardBreaksOversizedToken() {
	// Single whitespace-free token longer than width forces the hard-break
	// branch (typical: a URL inside the upstream error).
	tok := strings.Repeat("a", modalInnerWidth*2+5)
	lines := wrapErrMsg(tok, modalInnerWidth)

	s.Require().GreaterOrEqual(len(lines), 3)
	for _, line := range lines {
		s.Require().LessOrEqual(uniseg.StringWidth(line), modalInnerWidth)
	}
}

func (s *MRActionModalSuite) TestWrapErrMsg_EmptyReturnsNil() {
	s.Require().Nil(wrapErrMsg("", modalInnerWidth))
	s.Require().Nil(wrapErrMsg("anything", 0))
}

func (s *MRActionModalSuite) TestWrapErrMsg_StripsSGREscapes() {
	// Attacker-controlled upstream error embedding an SGR escape that would
	// forge green "Merged" text inside the red error block.
	evil := "\x1b[0mfake success\x1b[31m still red"
	lines := wrapErrMsg(evil, modalInnerWidth)

	joined := strings.Join(lines, "\n")
	s.Require().NotContains(joined, "\x1b", "ANSI escape bytes must be stripped before render")
	s.Require().Contains(joined, "fake success", "visible text survives sanitisation")
	s.Require().Contains(joined, "still red")
}

func (s *MRActionModalSuite) TestWrapErrMsg_HonoursWideRuneCellWidth() {
	// Mix wide (CJK / emoji) and narrow graphemes; every wrapped line must
	// stay within modalInnerWidth DISPLAY cells (uniseg.StringWidth), not
	// byte-count.
	msg := strings.Repeat("漢字テスト ", 20) + "done"
	lines := wrapErrMsg(msg, modalInnerWidth)

	s.Require().NotEmpty(lines)
	for _, line := range lines {
		s.Require().LessOrEqual(uniseg.StringWidth(line), modalInnerWidth, line)
	}
}

func (s *MRActionModalSuite) TestModalActionButtons_ReturnsCorrectLabelsPerKind() {
	closeBtns := stripANSI(modalActionButtons(ModalClose))
	mergeBtns := stripANSI(modalActionButtons(ModalMerge))

	s.Require().Contains(closeBtns, "[ Close (Enter) ]")
	s.Require().Contains(closeBtns, "[ Cancel (Esc) ]")
	s.Require().NotContains(closeBtns, "Merge")

	s.Require().Contains(mergeBtns, "[ Merge (Enter) ]")
	s.Require().Contains(mergeBtns, "[ Cancel (Esc) ]")
	s.Require().NotContains(mergeBtns, "Close")
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

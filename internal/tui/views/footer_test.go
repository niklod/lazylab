package views

import (
	"strings"
	"testing"
	"time"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/tui/keymap"
)

type FooterSuite struct {
	suite.Suite
	g    *gocui.Gui
	pane *gocui.View
	f    *FooterView
}

func (s *FooterSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 160, Height: 40})
	s.Require().NoError(err)
	s.g = g

	pv, err := s.g.SetView("footer", 0, 36, 159, 39, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView: %v", err)
	}
	s.pane = pv
	s.f = NewFooter()
}

func (s *FooterSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *FooterSuite) TestMetaLine_NoRepoYet_ShowsLazylabOnly() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewRepos})

	body := s.pane.Buffer()
	first := firstLine(body)
	s.Require().Contains(first, "lazylab")
	s.Require().NotContains(first, "·")
	s.Require().NotContains(first, "!")
	s.Require().NotContains(first, "last sync")
}

func (s *FooterSuite) TestMetaLine_RepoOnly_AddsBreadcrumb() {
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/alpha",
	})

	first := firstLine(s.pane.Buffer())
	s.Require().Contains(first, "lazylab")
	s.Require().Contains(first, "grp/alpha")
	s.Require().NotContains(first, "!")
}

func (s *FooterSuite) TestMetaLine_MRSelected_AddsIIDAndPosition() {
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/alpha",
		MRIID:       482,
		MRIndex:     3,
		MRTotal:     4,
	})

	first := firstLine(s.pane.Buffer())
	s.Require().Contains(first, "!482")
	s.Require().Contains(first, "3/4")
}

func (s *FooterSuite) TestMetaLine_LastSync_RendersRelative() {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	synced := now.Add(-3 * time.Minute)

	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/alpha",
		MRIID:       10,
		MRIndex:     1,
		MRTotal:     2,
		LastSync:    synced,
		Now:         now,
	})

	first := firstLine(s.pane.Buffer())
	s.Require().Contains(first, "last sync")
	s.Require().Contains(first, "3 minutes ago")
}

func (s *FooterSuite) TestMetaLine_LastSync_JustNowBucket() {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/alpha",
		LastSync:    now.Add(-10 * time.Second),
		Now:         now,
	})

	first := firstLine(s.pane.Buffer())
	s.Require().Contains(first, "last sync just now")
}

func (s *FooterSuite) TestMetaLine_ZeroLastSync_OmitsSegment() {
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/alpha",
	})

	first := firstLine(s.pane.Buffer())
	s.Require().NotContains(first, "last sync")
}

func (s *FooterSuite) TestKeybindStrip_ReposMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewRepos})

	strip := secondLine(s.pane.Buffer())
	for _, want := range []string{"h/l", "pane", "j/k", "row", "f", "fav", "Enter", "open", "?", "help", "q", "quit"} {
		s.Require().Contains(strip, want, "repos strip missing %q", want)
	}
}

func (s *FooterSuite) TestKeybindStrip_MRsMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewMRs})

	strip := secondLine(s.pane.Buffer())
	for _, want := range []string{"s", "state", "o", "owner", "x", "close", "M", "merge"} {
		s.Require().Contains(strip, want, "mrs strip missing %q", want)
	}
}

func (s *FooterSuite) TestKeybindStrip_DetailOverviewFallback() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewDetail})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "[/]")
	s.Require().Contains(strip, "tab")
}

func (s *FooterSuite) TestKeybindStrip_DiffTreeMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewDetailDiffTree})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "file")
	s.Require().Contains(strip, "ctrl+d/u")
	s.Require().Contains(strip, "scroll")
}

func (s *FooterSuite) TestKeybindStrip_ConversationMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewDetailConversation})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "thread")
	s.Require().Contains(strip, "J/K")
	s.Require().Contains(strip, "note")
}

func (s *FooterSuite) TestKeybindStrip_PipelineLogMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewDetailPipelineJobLog})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "copy")
	s.Require().Contains(strip, "g/G")
}

func (s *FooterSuite) TestKeybindStrip_SearchActiveOverridesFocus() {
	s.f.Render(s.pane, FooterState{
		FocusedView:  keymap.ViewMRs,
		SearchActive: true,
	})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "apply")
	s.Require().NotContains(strip, "merge")
}

func (s *FooterSuite) TestKeybindStrip_ModalCloseOverridesFocus() {
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		ModalActive: true,
		ModalKind:   ModalClose,
	})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "confirm")
	s.Require().NotContains(strip, "delete-branch")
}

func (s *FooterSuite) TestKeybindStrip_ModalMergeShowsToggles() {
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		ModalActive: true,
		ModalKind:   ModalMerge,
	})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "confirm")
	s.Require().Contains(strip, "delete-branch")
	s.Require().Contains(strip, "squash")
}

func (s *FooterSuite) TestRender_NilPaneNoPanic() {
	s.Require().NotPanics(func() {
		s.f.Render(nil, FooterState{FocusedView: keymap.ViewRepos})
	})
}

func (s *FooterSuite) TestMetaLine_NowZero_FallsBackToTimeNow() {
	synced := time.Now().Add(-3 * time.Minute)
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/x",
		LastSync:    synced,
		// Now left zero — Render must fall back to time.Now().
	})

	first := firstLine(s.pane.Buffer())
	s.Require().Contains(first, "last sync")
	s.Require().Contains(first, "3 minutes ago")
}

func (s *FooterSuite) TestMetaLine_MRWithoutPosition_OmitsNM() {
	s.f.Render(s.pane, FooterState{
		FocusedView: keymap.ViewMRs,
		RepoPath:    "grp/x",
		MRIID:       5,
	})

	first := firstLine(s.pane.Buffer())
	s.Require().Contains(first, "!5")
	s.Require().NotContains(first, "0/0")
}

func (s *FooterSuite) TestKeybindStrip_SearchOverridesModal() {
	s.f.Render(s.pane, FooterState{
		FocusedView:  keymap.ViewMRs,
		SearchActive: true,
		ModalActive:  true,
		ModalKind:    ModalMerge,
	})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "apply")
	s.Require().NotContains(strip, "delete-branch", "search must win when both overlays set")
}

func (s *FooterSuite) TestKeybindStrip_DiffContentMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewDetailDiffContent})

	strip := secondLine(s.pane.Buffer())
	s.Require().Contains(strip, "scroll")
	s.Require().Contains(strip, "ctrl+d/u")
}

func (s *FooterSuite) TestKeybindStrip_PipelineStagesMode() {
	s.f.Render(s.pane, FooterState{FocusedView: keymap.ViewDetailPipelineStages})

	strip := secondLine(s.pane.Buffer())
	for _, want := range []string{"job", "log", "retry", "refresh", "auto", "browser"} {
		s.Require().Contains(strip, want, "stages strip missing %q", want)
	}
}

func firstLine(buf string) string {
	lines := strings.SplitN(buf, "\n", 2)
	if len(lines) == 0 {
		return ""
	}

	return lines[0]
}

func secondLine(buf string) string {
	lines := strings.SplitN(buf, "\n", 3)
	if len(lines) < 2 {
		return ""
	}

	return lines[1]
}

func TestFooterSuite(t *testing.T) { //nolint:paralleltest // gocui headless guis share tcell simulation; parallel suites panic on teardown.
	suite.Run(t, new(FooterSuite))
}

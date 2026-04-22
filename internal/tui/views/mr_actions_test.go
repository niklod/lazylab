package views

import (
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

const mrActionsCfgPath = "/cfg/config.yaml"

// MRActionsBindSuite covers the global-shortcut rewiring from Phase G5.1:
// x/M must open the modal from every detail-family pane, resolve the MR
// from the source pane, and restore focus to the origin on dismiss.
type MRActionsBindSuite struct {
	suite.Suite
	g   *gocui.Gui
	v   *Views
	app *appcontext.AppContext
	fs  afero.Fs
}

func (s *MRActionsBindSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 160, Height: 40})
	s.Require().NoError(err)
	s.g = g

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = "https://example.test"
	cfg.GitLab.Token = testGitLabToken
	s.Require().NoError(cfg.Save(s.fs, mrActionsCfgPath))
	c := cache.New(cfg.Cache, s.fs)
	s.app = appcontext.New(cfg, nil, c, s.fs, mrActionsCfgPath)

	s.v = New(g, s.app)

	for _, name := range []string{
		keymap.ViewRepos, keymap.ViewMRs, keymap.ViewDetail,
		keymap.ViewDetailDiffTree, keymap.ViewDetailDiffContent,
		keymap.ViewDetailConversation,
		keymap.ViewDetailPipelineStages, keymap.ViewDetailPipelineJobLog,
		keymap.ViewMRActionsModal,
	} {
		_, err := s.g.SetView(name, 0, 0, 60, 30, 0)
		if err != nil && !strings.Contains(err.Error(), "unknown view") {
			s.T().Fatalf("SetView %s: %v", name, err)
		}
	}
}

func (s *MRActionsBindSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *MRActionsBindSuite) seedMRsCursor(mr *models.MergeRequest) {
	s.v.MRs.mu.Lock()
	defer s.v.MRs.mu.Unlock()

	s.v.MRs.current = &models.Project{ID: 42, PathWithNamespace: "grp/alpha"}
	s.v.MRs.loading = false
	s.v.MRs.all = []*models.MergeRequest{mr}
	s.v.MRs.rebuildLowerLocked()
	s.v.MRs.applyFilterLocked()
	s.v.MRs.cursor = 0
}

func (s *MRActionsBindSuite) seedDetailMR(mr *models.MergeRequest) {
	s.v.Detail.mu.Lock()
	defer s.v.Detail.mu.Unlock()

	s.v.Detail.mr = mr
}

func (s *MRActionsBindSuite) bindingFor(view string, key any) (found, duplicate bool, handler keymap.HandlerFunc) {
	for _, b := range s.v.Bindings() {
		if b.View == view && b.Key == key {
			if found {
				duplicate = true
			}
			handler = b.Handler
			found = true
		}
	}

	return found, duplicate, handler
}

func (s *MRActionsBindSuite) openedMR(iid int, title string) *models.MergeRequest {
	return &models.MergeRequest{
		IID:          iid,
		Title:        title,
		State:        models.MRStateOpened,
		SourceBranch: "feat/x",
		TargetBranch: "main",
		Author:       models.User{Username: "alice"},
	}
}

func (s *MRActionsBindSuite) TestBindingsCoverDetailFamily() {
	cases := []struct {
		view string
		kind ModalKind
	}{
		{keymap.ViewMRs, ModalClose},
		{keymap.ViewMRs, ModalMerge},
		{keymap.ViewDetail, ModalClose},
		{keymap.ViewDetail, ModalMerge},
		{keymap.ViewDetailDiffTree, ModalClose},
		{keymap.ViewDetailDiffTree, ModalMerge},
		{keymap.ViewDetailDiffContent, ModalClose},
		{keymap.ViewDetailDiffContent, ModalMerge},
		{keymap.ViewDetailConversation, ModalClose},
		{keymap.ViewDetailConversation, ModalMerge},
		{keymap.ViewDetailPipelineStages, ModalClose},
		{keymap.ViewDetailPipelineStages, ModalMerge},
		{keymap.ViewDetailPipelineJobLog, ModalClose},
		{keymap.ViewDetailPipelineJobLog, ModalMerge},
	}

	for _, tc := range cases {
		tc := tc
		key := 'x'
		if tc.kind == ModalMerge {
			key = 'M'
		}
		name := tc.view + "_" + string(key)
		s.Run(name, func() {
			s.SetupTest()
			defer s.TearDownTest()

			mr := s.openedMR(77, "Feature Alpha")
			s.seedMRsCursor(mr)
			s.seedDetailMR(mr)

			found, duplicate, handler := s.bindingFor(tc.view, key)
			s.Require().True(found, "binding missing for view=%s key=%c", tc.view, key)
			s.Require().False(duplicate, "duplicate binding for view=%s key=%c", tc.view, key)

			pv, err := s.g.View(tc.view)
			s.Require().NoError(err)
			s.Require().NoError(handler(s.g, pv))

			s.Require().True(s.v.ActionsModal.IsActive(), "modal must activate after binding fires")
			s.Require().Equal(tc.kind, s.v.ActionsModal.Kind())
		})
	}
}

func (s *MRActionsBindSuite) TestOpenFromDetailPane_ResolvesMRFromDetailView() {
	cursor := s.openedMR(2, "Cursor MR")
	shown := s.openedMR(1, "Shown MR")
	s.seedMRsCursor(cursor)
	s.seedDetailMR(shown)

	pv, err := s.g.View(keymap.ViewDetail)
	s.Require().NoError(err)
	s.Require().NoError(s.v.openCloseModal(s.g, pv))

	snap := s.v.ActionsModal.Snapshot()
	s.Require().True(snap.Active)
	s.Require().NotNil(snap.MR)
	s.Require().Equal(1, snap.MR.IID, "detail pane must operate on the MR the user is looking at")
}

func (s *MRActionsBindSuite) TestOpenFromMRsPane_ResolvesMRFromMRsView() {
	cursor := s.openedMR(2, "Cursor MR")
	shown := s.openedMR(1, "Stale detail MR")
	s.seedMRsCursor(cursor)
	s.seedDetailMR(shown)

	pv, err := s.g.View(keymap.ViewMRs)
	s.Require().NoError(err)
	s.Require().NoError(s.v.openCloseModal(s.g, pv))

	snap := s.v.ActionsModal.Snapshot()
	s.Require().True(snap.Active)
	s.Require().Equal(2, snap.MR.IID, "MRs pane must operate on the cursor MR")
}

func (s *MRActionsBindSuite) TestOpenFromDetailPane_FallsBackToMRsWhenDetailEmpty() {
	cursor := s.openedMR(5, "Cursor MR")
	s.seedMRsCursor(cursor)

	pv, err := s.g.View(keymap.ViewDetailDiffTree)
	s.Require().NoError(err)
	s.Require().NoError(s.v.openMergeModal(s.g, pv))

	snap := s.v.ActionsModal.Snapshot()
	s.Require().True(snap.Active)
	s.Require().Equal(5, snap.MR.IID, "empty detail view must fall through to the MRs cursor")
}

func (s *MRActionsBindSuite) TestStateGuardFromDetailPane_RoutesToastToMRsView() {
	merged := &models.MergeRequest{IID: 10, Title: "Done", State: models.MRStateMerged}
	s.seedMRsCursor(merged)
	s.seedDetailMR(merged)

	pv, err := s.g.View(keymap.ViewDetailConversation)
	s.Require().NoError(err)
	s.Require().NoError(s.v.openCloseModal(s.g, pv))

	s.Require().False(s.v.ActionsModal.IsActive(), "merged MR must not open the modal")
	status := s.v.MRs.TransientStatus()
	s.Require().Contains(status, "Cannot close")
	s.Require().Contains(status, "!10")
	s.Require().Contains(status, "merged")
}

func (s *MRActionsBindSuite) TestOpenRecordsActionOriginView() {
	mr := s.openedMR(12, "Feature")
	s.seedMRsCursor(mr)
	s.seedDetailMR(mr)

	pv, err := s.g.View(keymap.ViewDetailPipelineStages)
	s.Require().NoError(err)
	s.Require().NoError(s.v.openMergeModal(s.g, pv))

	s.Require().Equal(keymap.ViewDetailPipelineStages, s.v.actionOriginView)
}

func (s *MRActionsBindSuite) TestStateGuardDoesNotRecordOrigin() {
	merged := &models.MergeRequest{IID: 10, Title: "Done", State: models.MRStateMerged}
	s.seedMRsCursor(merged)
	s.seedDetailMR(merged)

	s.v.actionOriginView = "stale"
	pv, err := s.g.View(keymap.ViewDetail)
	s.Require().NoError(err)
	s.Require().NoError(s.v.openCloseModal(s.g, pv))

	s.Require().Equal("stale", s.v.actionOriginView, "state-guard path must not rewrite actionOriginView; it only opens modal paths")
}

func (s *MRActionsBindSuite) TestFocusRestoreTarget_UsesOriginWhenMounted() {
	s.v.actionOriginView = keymap.ViewDetailPipelineStages

	target := s.v.focusRestoreTarget(s.g)

	s.Require().Equal(keymap.ViewDetailPipelineStages, target)
}

func (s *MRActionsBindSuite) TestFocusRestoreTarget_FallsBackToMRsWhenOriginUnmounted() {
	s.v.actionOriginView = "ghost_view"

	target := s.v.focusRestoreTarget(s.g)

	s.Require().Equal(keymap.ViewMRs, target)
}

func (s *MRActionsBindSuite) TestFocusRestoreTarget_EmptyOriginFallsBackToMRs() {
	s.v.actionOriginView = ""

	target := s.v.focusRestoreTarget(s.g)

	s.Require().Equal(keymap.ViewMRs, target)
}

func (s *MRActionsBindSuite) TestKeymapIsDetailFamily_TableCoversEveryPaneName() {
	cases := []struct {
		name   string
		expect bool
	}{
		{keymap.ViewDetail, true},
		{keymap.ViewDetailDiffTree, true},
		{keymap.ViewDetailDiffContent, true},
		{keymap.ViewDetailConversation, true},
		{keymap.ViewDetailPipelineStages, true},
		{keymap.ViewDetailPipelineJobLog, true},
		{keymap.ViewMRs, false},
		{keymap.ViewRepos, false},
		{keymap.ViewMRActionsModal, false},
		{"", false},
	}

	for _, tc := range cases {
		s.Require().Equal(tc.expect, keymap.IsDetailFamily(tc.name), "view=%q", tc.name)
	}
}

func TestMRActionsBindSuite(t *testing.T) { //nolint:paralleltest // gocui headless guis share a tcell simulation screen; parallel suites panic on teardown.
	suite.Run(t, new(MRActionsBindSuite))
}

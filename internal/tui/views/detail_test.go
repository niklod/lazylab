package views

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	goerrors "github.com/go-errors/errors"
	"github.com/jesseduffield/gocui"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

const detailTestCfgPath = "/cfg/config.yaml"

type DetailViewSuite struct {
	suite.Suite
	g      *gocui.Gui
	detail *DetailView
	srv    *httptest.Server
	app    *appcontext.AppContext
}

func (s *DetailViewSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	_, err = s.g.SetView(keymap.ViewDetail, 0, 0, 80, 30, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView detail: %v", err)
	}

	s.detail = NewDetail(g, nil)
}

func (s *DetailViewSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *DetailViewSuite) buildAppWithHandler(handler http.HandlerFunc) {
	s.srv = httptest.NewServer(handler)

	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = testGitLabToken
	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	fs := afero.NewMemMapFs()
	s.Require().NoError(cfg.Save(fs, detailTestCfgPath))
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, fs), fs, detailTestCfgPath)

	s.detail = NewDetail(s.g, s.app)
}

func (s *DetailViewSuite) pane() *gocui.View {
	pane, err := s.g.View(keymap.ViewDetail)
	s.Require().NoError(err)

	return pane
}

func (s *DetailViewSuite) TestRender_NilMR_ShowsEmptyHint() {
	s.detail.Render(s.pane())

	s.Require().Contains(s.pane().Buffer(), detailStatusEmpty)
}

func (s *DetailViewSuite) TestRender_WithMR_RendersAllOverviewFields() {
	created := time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC)
	mr := &models.MergeRequest{
		IID:            42,
		Title:          "Feature Alpha",
		State:          models.MRStateOpened,
		Author:         models.User{Username: "alice"},
		SourceBranch:   "feat/alpha",
		TargetBranch:   "main",
		CreatedAt:      created,
		HasConflicts:   false,
		UserNotesCount: 7,
	}

	s.detail.SetMR(nil, mr)
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "!42 Feature Alpha")
	s.Require().Contains(buf, "@alice")
	s.Require().Contains(buf, "2026-04-10 14:30")
	s.Require().Contains(buf, "O opened")
	s.Require().Contains(buf, "feat/alpha \u2192 main")
	s.Require().Contains(buf, iconOK+" No conflicts")
	s.Require().Contains(buf, "Comments: 7")
}

func (s *DetailViewSuite) TestRender_WithConflicts_ShowsConflictText() {
	s.detail.SetMR(nil, &models.MergeRequest{
		IID:          1,
		Title:        "x",
		State:        models.MRStateOpened,
		HasConflicts: true,
	})

	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, iconBad+" Has conflicts")
	s.Require().NotContains(buf, iconOK+" No conflicts")
}

func (s *DetailViewSuite) TestSetMR_ReplacesPreviousMR() {
	first := &models.MergeRequest{IID: 1, Title: "First", State: models.MRStateOpened}
	second := &models.MergeRequest{IID: 2, Title: "Second", State: models.MRStateMerged}

	s.detail.SetMR(nil, first)
	s.Require().Equal(first, s.detail.CurrentMR())

	s.detail.SetMR(nil, second)
	s.Require().Equal(second, s.detail.CurrentMR())

	s.detail.Render(s.pane())
	buf := s.pane().Buffer()
	s.Require().Contains(buf, "!2 Second")
	s.Require().NotContains(buf, "First")
}

func (s *DetailViewSuite) TestRender_StateLetter_CoversAllStates() {
	tests := []struct {
		name       string
		state      models.MRState
		wantLetter string
	}{
		{name: "opened", state: models.MRStateOpened, wantLetter: "O opened"},
		{name: "merged", state: models.MRStateMerged, wantLetter: "M merged"},
		{name: "closed", state: models.MRStateClosed, wantLetter: "C closed"},
		{name: "unknown", state: models.MRState("weird"), wantLetter: "? weird"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "t", State: tt.state})

			s.detail.Render(s.pane())

			s.Require().Contains(s.pane().Buffer(), tt.wantLetter)
		})
	}
}

func (s *DetailViewSuite) TestSetMR_Nil_ClearsContent() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 42, Title: "Feature", State: models.MRStateOpened})
	s.detail.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), "Feature")

	s.detail.SetMR(nil, nil)
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, detailStatusEmpty)
	s.Require().NotContains(buf, "Feature")
}

func (s *DetailViewSuite) TestSelectMRForDetail_NilMRsView_NoPanic() {
	v := &Views{Detail: s.detail}

	err := v.selectMRForDetail(nil, nil)

	s.Require().NoError(err)
	s.Require().Nil(s.detail.CurrentMR())
}

func (s *DetailViewSuite) TestSelectMRForDetail_NilDetailView_NoPanic() {
	v := &Views{MRs: &MRsView{}}

	err := v.selectMRForDetail(nil, nil)

	s.Require().NoError(err)
}

func (s *DetailViewSuite) TestSelectMRForDetail_NoSelectedMR_LeavesDetailUntouched() {
	existing := &models.MergeRequest{IID: 1, Title: "Existing", State: models.MRStateOpened}
	s.detail.SetMR(nil, existing)
	v := &Views{MRs: &MRsView{}, Detail: s.detail}

	err := v.selectMRForDetail(nil, nil)

	s.Require().NoError(err)
	s.Require().Equal(existing, s.detail.CurrentMR())
}

func (s *DetailViewSuite) TestCommentsText_NoResolvable_PlainCount() {
	s.Require().Equal("5", commentsText(5, nil))
	s.Require().Equal("0", commentsText(0, &models.DiscussionStats{}))
	s.Require().Equal("3", commentsText(3, &models.DiscussionStats{TotalResolvable: 0, Resolved: 0}))
}

func (s *DetailViewSuite) TestCommentsText_PartiallyResolved_YellowWarnIcon() {
	got := commentsText(7, &models.DiscussionStats{TotalResolvable: 3, Resolved: 2})

	s.Require().Contains(got, ansiYellow)
	s.Require().Contains(got, iconWarn)
	s.Require().Contains(got, "7 ")
	s.Require().Contains(got, "(2/3 resolved)")
	s.Require().Contains(got, ansiReset)
}

func (s *DetailViewSuite) TestCommentsText_FullyResolved_GreenCheckIcon() {
	got := commentsText(5, &models.DiscussionStats{TotalResolvable: 5, Resolved: 5})

	s.Require().Contains(got, ansiGreen)
	s.Require().Contains(got, iconOK)
	s.Require().Contains(got, "5 ")
	s.Require().Contains(got, "(5/5 resolved)")
	s.Require().Contains(got, ansiReset)
}

func (s *DetailViewSuite) TestCommentsText_NoneResolved_YellowWarnIcon() {
	got := commentsText(1, &models.DiscussionStats{TotalResolvable: 2, Resolved: 0})

	s.Require().Contains(got, ansiYellow)
	s.Require().Contains(got, iconWarn)
	s.Require().Contains(got, "(0/2 resolved)")
}

func (s *DetailViewSuite) TestSetMRSync_FetchesStatsAndRendersResolved() {
	var hits atomic.Int32
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/discussions") {
			http.NotFound(w, r)

			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"id":"d1","notes":[{"id":1,"resolvable":true,"resolved":true}]},
			{"id":"d2","notes":[{"id":2,"resolvable":true,"resolved":false}]},
			{"id":"d3","notes":[{"id":3,"resolvable":false,"resolved":false}]}
		]`)
	})
	mr := &models.MergeRequest{IID: 11, Title: "T", State: models.MRStateOpened, UserNotesCount: 4}
	project := &models.Project{ID: 7, PathWithNamespace: "grp/x"}

	s.Require().NoError(s.detail.SetMRSync(context.Background(), project, mr))
	s.detail.Render(s.pane())

	s.Require().Equal(int32(1), hits.Load())
	s.Require().Equal(&models.DiscussionStats{TotalResolvable: 2, Resolved: 1}, s.detail.Stats())
	s.Require().Contains(s.pane().Buffer(), "Comments: 4 "+iconWarn+" (1/2 resolved)")
}

func (s *DetailViewSuite) TestSetMRSync_NilProject_SkipsFetch() {
	s.buildAppWithHandler(http.NotFound)
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened, UserNotesCount: 2}

	s.Require().NoError(s.detail.SetMRSync(context.Background(), nil, mr))

	s.Require().Nil(s.detail.Stats())
	s.detail.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), "Comments: 2")
}

func (s *DetailViewSuite) TestApplyStats_StaleSeqIgnored() {
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened, UserNotesCount: 0}
	s.detail.SetMR(nil, mr)

	stats := &models.DiscussionStats{TotalResolvable: 1, Resolved: 1}
	s.detail.applyStats(0, stats, nil)

	s.Require().Nil(s.detail.Stats(), "stale seq must not clobber current state")
}

func (s *DetailViewSuite) TestSetTab_CyclesThroughEveryTab() {
	got := make([]DetailTab, 0, 4)
	got = append(got, s.detail.CurrentTab())
	for i := 0; i < 3; i++ {
		next := nextDetailTab(s.detail.CurrentTab(), 1)
		s.detail.SetTab(next, nil)
		got = append(got, s.detail.CurrentTab())
	}

	s.Require().Equal([]DetailTab{
		DetailTabOverview, DetailTabDiff, DetailTabConversation, DetailTabPipeline,
	}, got)

	next := nextDetailTab(s.detail.CurrentTab(), 1)
	s.detail.SetTab(next, nil)
	s.Require().Equal(DetailTabOverview, s.detail.CurrentTab(), "cycle wraps")
}

func (s *DetailViewSuite) TestRenderTabBar_HighlightsActiveTab() {
	got := renderTabBar(DetailTabDiff)

	s.Require().Contains(got, ansiReverse+"Diff"+ansiReset)
	s.Require().Contains(got, "Overview")
	s.Require().Contains(got, "Conversation")
	s.Require().Contains(got, "Pipeline")
	s.Require().NotContains(got, ansiReverse+"Overview"+ansiReset)
}

func (s *DetailViewSuite) TestSetTab_RendersActiveTabLabel() {
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(nil, mr)

	s.detail.SetTab(DetailTabDiff, nil)
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Diff")
	s.Require().Contains(buf, "Overview")
}

func (s *DetailViewSuite) TestSetTab_ConversationAndPipelineShowStubs() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})

	s.detail.SetTab(DetailTabConversation, nil)
	s.detail.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), detailConversationStub)

	s.detail.SetTab(DetailTabPipeline, nil)
	s.detail.Render(s.pane())
	s.Require().Contains(s.pane().Buffer(), detailPipelineStub)
}

func (s *DetailViewSuite) TestSetTabSync_FetchesDiffAndPopulatesTree() {
	var hits atomic.Int32
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"old_path":"src/a.go","new_path":"src/a.go","diff":"@@ -1 +1 @@\n-old\n+new\n"},
			{"old_path":"src/b.go","new_path":"src/b.go","diff":"@@ -1 +1 @@\n-x\n+y\n","new_file":true}
		]`)
	})
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabDiff, project))

	s.Require().Equal(int32(1), hits.Load())
	s.Require().Equal(DetailTabDiff, s.detail.CurrentTab())

	tree := s.detail.DiffTree()
	s.Require().NotNil(tree)
	s.Require().Equal(3, tree.RowCount(), "one dir header + two leaves")
	selected := tree.SelectedFile()
	s.Require().NotNil(selected)
	s.Require().Equal("src/a.go", selected.NewPath)

	content := s.detail.DiffContent()
	s.Require().NotNil(content)
	s.Require().NotNil(content.CurrentFile())
	s.Require().Equal("src/a.go", content.CurrentFile().NewPath)
}

func (s *DetailViewSuite) TestSetTabSync_EmptyChangesShowsEmptyHint() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	})
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabDiff, project))

	s.Require().Equal(0, s.detail.DiffTree().RowCount())
}

func (s *DetailViewSuite) TestSetTabSync_UpstreamErrorSurfaces() {
	s.buildAppWithHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"fail"}`)
	})
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	err := s.detail.SetTabSync(context.Background(), DetailTabDiff, project)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "fetch mr changes")
}

func (s *DetailViewSuite) TestSetMR_ResetsDiffState() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"old_path":"a","new_path":"a","diff":"@@ -1 +1 @@\n-x\n+y\n"}]`)
	})
	project := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabDiff, project))
	s.Require().Equal(1, s.detail.DiffTree().RowCount())

	s.detail.SetMR(project, &models.MergeRequest{IID: 2, Title: "U", State: models.MRStateOpened})

	s.Require().Equal(0, s.detail.DiffTree().RowCount(), "tree is cleared on MR swap")
	s.Require().Nil(s.detail.DiffContent().CurrentFile())
}

func (s *DetailViewSuite) TestSetTabSync_ErrorPropagatesToWidgets() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"explode"}`)
	})
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	err := s.detail.SetTabSync(context.Background(), DetailTabDiff, project)

	s.Require().Error(err)

	tree := s.detail.DiffTree()
	content := s.detail.DiffContent()

	treePane, gvErr := s.g.SetView(keymap.ViewDetailDiffTree, 0, 0, 30, 20, 0)
	if gvErr != nil {
		tree.Render(treePane)
	}
	contentPane, gvErr := s.g.SetView(keymap.ViewDetailDiffContent, 0, 0, 30, 20, 0)
	if gvErr != nil {
		content.Render(contentPane)
	}

	s.Require().Contains(tree.statusSnapshot(), "explode")
}

func (s *DetailViewSuite) TestSetMR_Nil_ClearsDiffWidgets() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"old_path":"a","new_path":"a","diff":"@@ -1 +1 @@\n-x\n+y\n"}]`)
	})
	project := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabDiff, project))
	s.Require().Equal(1, s.detail.DiffTree().RowCount())

	s.detail.SetMR(nil, nil)

	s.Require().Equal(0, s.detail.DiffTree().RowCount())
	s.Require().Nil(s.detail.DiffContent().CurrentFile())
}

func (s *DetailViewSuite) TestConsumePendingFocus_IsIdempotent() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})
	s.detail.SetTab(DetailTabDiff, nil)

	first := s.detail.ConsumePendingFocus()
	second := s.detail.ConsumePendingFocus()

	s.Require().Equal(keymap.ViewDetailDiffTree, first)
	s.Require().Empty(second, "second consume returns empty")
}

func (s *DetailViewSuite) TestFocusTargetForTab_TabletoPane() {
	s.Require().Equal(keymap.ViewDetail, focusTargetForTab(DetailTabOverview))
	s.Require().Equal(keymap.ViewDetailDiffTree, focusTargetForTab(DetailTabDiff))
	s.Require().Equal(keymap.ViewDetail, focusTargetForTab(DetailTabConversation))
	s.Require().Equal(keymap.ViewDetail, focusTargetForTab(DetailTabPipeline))
}

func (s *DetailViewSuite) TestNextDetailTab_Wraps() {
	s.Require().Equal(DetailTabDiff, nextDetailTab(DetailTabOverview, 1))
	s.Require().Equal(DetailTabPipeline, nextDetailTab(DetailTabOverview, -1))
	s.Require().Equal(DetailTabOverview, nextDetailTab(DetailTabPipeline, 1))
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestDetailViewSuite(t *testing.T) {
	suite.Run(t, new(DetailViewSuite))
}

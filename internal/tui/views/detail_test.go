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
	"github.com/niklod/lazylab/internal/tui/theme"
)

const detailTestCfgPath = "/cfg/config.yaml"

// sgrPrefix trims the trailing 'm' from an SGR escape so assertions match
// regardless of how gocui re-serializes the final semicolon when the
// rendered buffer is read back.
func sgrPrefix(seq string) string {
	return strings.TrimSuffix(seq, "m")
}

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
	fs := afero.NewMemMapFs()
	s.Require().NoError(cfg.Save(fs, detailTestCfgPath))
	c := cache.New(cfg.Cache, fs)
	client, err := gitlab.New(cfg.GitLab,
		gitlab.WithHTTPClient(s.srv.Client()),
		gitlab.WithCache(c),
	)
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, c, fs, detailTestCfgPath)

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
	created := time.Now().Add(-3 * 24 * time.Hour).UTC()
	mr := &models.MergeRequest{
		IID:            42,
		Title:          "Feature Alpha",
		State:          models.MRStateOpened,
		Author:         models.User{Username: "alice"},
		SourceBranch:   "feat/alpha",
		TargetBranch:   "main",
		CreatedAt:      created,
		UpdatedAt:      created,
		ProjectPath:    "grp/alpha",
		HasConflicts:   false,
		UserNotesCount: 7,
	}

	s.detail.SetMR(nil, mr)
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Feature Alpha")
	s.Require().Contains(buf, "!42")
	s.Require().Contains(buf, "grp/alpha")
	s.Require().Contains(buf, "@alice")
	s.Require().Contains(buf, "opened")
	s.Require().Contains(buf, "feat/alpha")
	s.Require().Contains(buf, "main")
	s.Require().Contains(buf, "none")
	s.Require().Contains(buf, "Comments")
	s.Require().Contains(buf, "7")
	s.Require().Contains(buf, "Updated")
}

func (s *DetailViewSuite) TestReviewersLine_EmptyReturnsEmpty() {
	s.Require().Empty(reviewersLine(nil))
	s.Require().Empty(reviewersLine([]models.User{}))
}

func (s *DetailViewSuite) TestReviewersLine_SingleReviewer_WrapsInAccent() {
	got := reviewersLine([]models.User{{Username: "alice"}})

	s.Require().Contains(got, "@alice")
	s.Require().Contains(got, theme.FgAccent)
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestReviewersLine_MultipleReviewersCommaSeparated() {
	got := reviewersLine([]models.User{
		{Username: "alice"},
		{Username: "bob"},
		{Username: "carol"},
	})

	s.Require().Contains(got, "@alice")
	s.Require().Contains(got, "@bob")
	s.Require().Contains(got, "@carol")
	s.Require().Contains(got, ", ")
}

func (s *DetailViewSuite) TestRender_MRWithReviewers_RendersReviewersLine() {
	s.detail.SetMR(nil, &models.MergeRequest{
		IID:    1,
		Title:  "T",
		State:  models.MRStateOpened,
		Author: models.User{Username: "alice"},
		Reviewers: []models.User{
			{Username: "bob"},
			{Username: "carol"},
		},
	})

	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "@alice")
	s.Require().Contains(buf, "@bob")
	s.Require().Contains(buf, "@carol")
	s.Require().Contains(buf, "Reviewers")
}

func (s *DetailViewSuite) TestRender_MRWithoutReviewers_OmitsReviewersLine() {
	s.detail.SetMR(nil, &models.MergeRequest{
		IID:       1,
		Title:     "T",
		State:     models.MRStateOpened,
		Author:    models.User{Username: "alice"},
		Reviewers: nil,
	})

	s.detail.Render(s.pane())

	s.Require().NotContains(s.pane().Buffer(), "Reviewers")
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
	s.Require().Contains(buf, "has conflicts")
	s.Require().NotContains(buf, "Conflicts    "+theme.FgOK+"none")
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
	s.Require().Contains(buf, "!2")
	s.Require().Contains(buf, "Second")
	s.Require().NotContains(buf, "First")
}

func (s *DetailViewSuite) TestRender_State_RendersColoredDotAndWord() {
	tests := []struct {
		name      string
		state     models.MRState
		wantColor string
		wantWord  string
	}{
		{name: "opened", state: models.MRStateOpened, wantColor: theme.FgOK, wantWord: "opened"},
		{name: "merged", state: models.MRStateMerged, wantColor: theme.FgMerged, wantWord: "merged"},
		{name: "closed", state: models.MRStateClosed, wantColor: theme.FgErr, wantWord: "closed"},
		{name: "unknown", state: models.MRState("weird"), wantColor: theme.FgDraft, wantWord: "weird"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "t", State: tt.state})

			s.detail.Render(s.pane())

			buf := s.pane().Buffer()
			s.Require().Contains(buf, tt.wantWord)
			s.Require().Contains(buf, sgrPrefix(tt.wantColor))
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

func (s *DetailViewSuite) TestCommentsText_PartiallyResolved_DimRatio() {
	got := commentsText(7, &models.DiscussionStats{TotalResolvable: 3, Resolved: 2})

	s.Require().Contains(got, theme.Dim)
	s.Require().Contains(got, "7")
	s.Require().Contains(got, "(2/3 resolved)")
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestCommentsText_FullyResolved_DimRatio() {
	got := commentsText(5, &models.DiscussionStats{TotalResolvable: 5, Resolved: 5})

	s.Require().Contains(got, theme.Dim)
	s.Require().Contains(got, "5")
	s.Require().Contains(got, "(5/5 resolved)")
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestCommentsText_NoneResolved_DimRatio() {
	got := commentsText(1, &models.DiscussionStats{TotalResolvable: 2, Resolved: 0})

	s.Require().Contains(got, theme.Dim)
	s.Require().Contains(got, "(0/2 resolved)")
}

func (s *DetailViewSuite) TestSetMRSync_FetchesStatsAndRendersResolved() {
	var hits atomic.Int32
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/discussions"):
			hits.Add(1)
			_, _ = fmt.Fprint(w, `[
				{"id":"d1","notes":[{"id":1,"resolvable":true,"resolved":true}]},
				{"id":"d2","notes":[{"id":2,"resolvable":true,"resolved":false}]},
				{"id":"d3","notes":[{"id":3,"resolvable":false,"resolved":false}]}
			]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":0,"approvals_left":0}`)
		case strings.Contains(r.URL.Path, "/pipelines"):
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})
	mr := &models.MergeRequest{IID: 11, Title: "T", State: models.MRStateOpened, UserNotesCount: 4}
	project := &models.Project{ID: 7, PathWithNamespace: "grp/x"}

	s.Require().NoError(s.detail.SetMRSync(context.Background(), project, mr))
	s.detail.Render(s.pane())

	s.Require().Equal(int32(1), hits.Load())
	s.Require().Equal(&models.DiscussionStats{TotalResolvable: 2, Resolved: 1}, s.detail.Stats())
	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Comments")
	s.Require().Contains(buf, "4")
	s.Require().Contains(buf, "(1/2 resolved)")
}

func (s *DetailViewSuite) TestSetMRSync_NilProject_SkipsFetch() {
	s.buildAppWithHandler(http.NotFound)
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened, UserNotesCount: 2}

	s.Require().NoError(s.detail.SetMRSync(context.Background(), nil, mr))

	s.Require().Nil(s.detail.Stats())
	s.detail.Render(s.pane())
	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Comments")
	s.Require().Contains(buf, "2")
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

	s.Require().Contains(got, theme.FgAccent+theme.Bold+"Diff"+theme.Reset)
	s.Require().Contains(got, "Overview")
	s.Require().Contains(got, "Conversation")
	s.Require().Contains(got, "Pipeline")
	s.Require().NotContains(got, theme.FgAccent+theme.Bold+"Overview"+theme.Reset)
	s.Require().Contains(got, theme.Dim+"[ "+theme.Reset)
	s.Require().Contains(got, theme.Dim+" ]"+theme.Reset)
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

func (s *DetailViewSuite) TestSetTab_ConversationShowsLoadingHintUntilDataArrives() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})

	s.detail.SetTab(DetailTabConversation, nil)

	conv := s.detail.Conversation()
	s.Require().NotNil(conv)
}

func (s *DetailViewSuite) TestSetTabSync_FetchesDiffAndPopulatesTree() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
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

func (s *DetailViewSuite) TestApprovalsText_NilReturnsLoadingHint() {
	got := approvalsText(nil)

	s.Require().Contains(got, "loading")
	s.Require().Contains(got, theme.Dim)
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestApprovalsText_ZeroRequired_RendersDimNoApprovalsRequired() {
	got := approvalsText(&models.ApprovalStatus{ApprovalsRequired: 0, ApprovalsLeft: 0, Approved: true})

	s.Require().Contains(got, "no approvals required")
	s.Require().Contains(got, theme.Dim)
	s.Require().NotContains(got, iconOK)
	s.Require().NotContains(got, iconBad)
}

func (s *DetailViewSuite) TestApprovalsText_AllReceived_RendersGreenCheck() {
	got := approvalsText(&models.ApprovalStatus{ApprovalsRequired: 1, ApprovalsLeft: 0, Approved: true})

	s.Require().Contains(got, theme.FgOK)
	s.Require().Contains(got, iconOK)
	s.Require().Contains(got, "1/1 approvals received")
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestApprovalsText_SomeMissing_RendersRedCross() {
	got := approvalsText(&models.ApprovalStatus{ApprovalsRequired: 2, ApprovalsLeft: 2, Approved: false})

	s.Require().Contains(got, theme.FgErr)
	s.Require().Contains(got, iconBad)
	s.Require().Contains(got, "0/2 approvals received")
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestApprovalsText_PartialReceived_RendersRedCross() {
	got := approvalsText(&models.ApprovalStatus{ApprovalsRequired: 3, ApprovalsLeft: 1, Approved: false})

	s.Require().Contains(got, theme.FgErr)
	s.Require().Contains(got, iconBad)
	s.Require().Contains(got, "2/3 approvals received")
}

func (s *DetailViewSuite) TestApplyApprovals_StaleSeqIgnored() {
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(nil, mr)

	s.detail.applyApprovals(0, &models.ApprovalStatus{ApprovalsRequired: 1, ApprovalsLeft: 0, Approved: true}, nil)

	s.Require().Nil(s.detail.Approvals(), "stale seq must not clobber current state")
}

func (s *DetailViewSuite) TestSetMRSync_FetchesApprovalsAndRendersRedCross() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":false,"approvals_required":2,"approvals_left":2}`)
		case strings.Contains(r.URL.Path, "/pipelines"):
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 7, PathWithNamespace: "grp/x"}

	s.Require().NoError(s.detail.SetMRSync(context.Background(), project, mr))
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Approvals")
	s.Require().Contains(buf, iconBad+" 0/2 approvals received")
	approvals := s.detail.Approvals()
	s.Require().NotNil(approvals)
	s.Require().Equal(2, approvals.ApprovalsRequired)
	s.Require().Equal(2, approvals.ApprovalsLeft)
}

func (s *DetailViewSuite) TestSetMRSync_FetchesApprovalsAndRendersGreenCheck() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":1,"approvals_left":0}`)
		case strings.Contains(r.URL.Path, "/pipelines"):
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 7, PathWithNamespace: "grp/x"}

	s.Require().NoError(s.detail.SetMRSync(context.Background(), project, mr))
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, iconOK+" 1/1 approvals received")
}

func (s *DetailViewSuite) TestSetMRSync_ApprovalsUpstreamError_LeavesLoadingHint() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"message":"boom"}`)
		default:
			http.NotFound(w, r)
		}
	})
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 7, PathWithNamespace: "grp/x"}

	err := s.detail.SetMRSync(context.Background(), project, mr)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "fetch mr approvals")
	s.Require().Nil(s.detail.Approvals(), "upstream error must leave approvals nil")
	s.detail.Render(s.pane())
	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Approvals")
	s.Require().Contains(buf, "loading")
}

func (s *DetailViewSuite) TestRenderOverview_BeforeApprovalsFetch_ShowsLoadingHint() {
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(nil, mr)

	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Approvals")
	s.Require().Contains(buf, "loading")
}

func (s *DetailViewSuite) TestDiffStatsText_NilReturnsLoadingHint() {
	got := diffStatsText(nil)

	s.Require().Contains(got, "loading")
	s.Require().Contains(got, theme.Dim)
	s.Require().Contains(got, theme.Reset)
}

func (s *DetailViewSuite) TestDiffStatsText_RendersGreenPlusRedMinus() {
	got := diffStatsText(&models.DiffStats{Added: 12, Removed: 3})

	s.Require().Contains(got, theme.FgOK+"+12"+theme.Reset)
	s.Require().Contains(got, theme.FgErr+"-3"+theme.Reset)
}

func (s *DetailViewSuite) TestSetTabSync_PopulatesDiffStatsAndRendersInOverview() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/diffs") {
			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"old_path":"a","new_path":"a","diff":"@@ -1,2 +1,3 @@\n-x\n+y\n+z\n"}
		]`)
	})
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabDiff, project))

	stats := s.detail.DiffStatsSnapshot()
	s.Require().NotNil(stats)
	s.Require().Equal(2, stats.Added)
	s.Require().Equal(1, stats.Removed)

	s.detail.SetTab(DetailTabOverview, nil)
	s.detail.Render(s.pane())

	buf := s.pane().Buffer()
	s.Require().Contains(buf, "Changes")
	s.Require().Contains(buf, "+2")
	s.Require().Contains(buf, "-1")
}

func (s *DetailViewSuite) TestRenderOverview_BeforeDiffFetch_ShowsLoadingHint() {
	mr := &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(nil, mr)

	s.detail.Render(s.pane())

	s.Require().Contains(s.pane().Buffer(), "Changes")
	s.Require().Contains(s.pane().Buffer(), "loading")
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
	s.Require().Equal(keymap.ViewDetailConversation, focusTargetForTab(DetailTabConversation))
	s.Require().Equal(keymap.ViewDetailPipelineStages, focusTargetForTab(DetailTabPipeline))
}

func (s *DetailViewSuite) TestNextDetailTab_Wraps() {
	s.Require().Equal(DetailTabDiff, nextDetailTab(DetailTabOverview, 1))
	s.Require().Equal(DetailTabPipeline, nextDetailTab(DetailTabOverview, -1))
	s.Require().Equal(DetailTabOverview, nextDetailTab(DetailTabPipeline, 1))
}

func (s *DetailViewSuite) pipelineHandler(pipelinesBody, pipelineBody, jobsBody, traceBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/merge_requests/") && strings.HasSuffix(r.URL.Path, "/pipelines"):
			_, _ = fmt.Fprint(w, pipelinesBody)
		case strings.Contains(r.URL.Path, "/pipelines/") && strings.HasSuffix(r.URL.Path, "/jobs"):
			_, _ = fmt.Fprint(w, jobsBody)
		case strings.Contains(r.URL.Path, "/pipelines/"):
			_, _ = fmt.Fprint(w, pipelineBody)
		case strings.Contains(r.URL.Path, "/jobs/") && strings.HasSuffix(r.URL.Path, "/trace"):
			_, _ = fmt.Fprint(w, traceBody)
		case strings.Contains(r.URL.Path, "/discussions"),
			strings.Contains(r.URL.Path, "/approvals"),
			strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}
}

func (s *DetailViewSuite) TestSetTabSync_Pipeline_FetchesDetailAndPopulatesStages() {
	s.buildAppWithHandler(s.pipelineHandler(
		`[{"id":77,"iid":1,"project_id":11,"status":"running","ref":"feat/x","web_url":"u"}]`,
		`{"id":77,"status":"running","ref":"feat/x","sha":"deadbeef","web_url":"u"}`,
		`[
			{"id":1,"name":"build","stage":"build","status":"success","duration":12.0},
			{"id":2,"name":"test","stage":"test","status":"failed","duration":30.0}
		]`,
		"",
	))
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))

	s.Require().Equal(DetailTabPipeline, s.detail.CurrentTab())

	stages := s.detail.PipelineStages()
	s.Require().NotNil(stages)
	s.Require().Positive(stages.RowCount(), "rows populated after sync fetch")
	s.Require().NotNil(s.detail.PipelineDetailSnapshot())
	s.Require().Len(s.detail.PipelineDetailSnapshot().Jobs, 2)
	// Fixture: [build, test]. Client reverses to pipeline exec order:
	// newest-first API → exec-order [test, build] after flip. Cursor
	// lands on the first job row under the first stage header = "test".
	s.Require().Equal("test", stages.SelectedJob().Stage)
}

func (s *DetailViewSuite) TestSetTabSync_Pipeline_EmptyPipelineShowsEmptyHint() {
	s.buildAppWithHandler(s.pipelineHandler(`[]`, ``, ``, ``))
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))

	s.Require().Equal(0, s.detail.PipelineStages().RowCount())
	s.Require().Nil(s.detail.PipelineDetailSnapshot())
}

func (s *DetailViewSuite) TestSetTabSync_Pipeline_UpstreamErrorWrapped() {
	s.buildAppWithHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"boom"}`)
	})
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, mr)

	err := s.detail.SetTabSync(context.Background(), DetailTabPipeline, project)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "fetch mr pipeline")
}

func (s *DetailViewSuite) TestApplyPipeline_StaleSeqIgnored() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})

	s.detail.applyPipeline(0, &models.PipelineDetail{Jobs: []models.PipelineJob{{Name: "build"}}}, nil)

	s.Require().Nil(s.detail.PipelineDetailSnapshot(), "stale seq must not clobber current state")
}

func (s *DetailViewSuite) TestSetMR_Pipeline_ResetsState() {
	s.buildAppWithHandler(s.pipelineHandler(
		`[{"id":77,"iid":1,"project_id":11,"status":"running","ref":"feat/x","web_url":"u"}]`,
		`{"id":77,"status":"running","ref":"feat/x","sha":"d","web_url":"u"}`,
		`[{"id":1,"name":"build","stage":"build","status":"success","duration":5.0}]`,
		"",
	))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened})
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))
	s.Require().Positive(s.detail.PipelineStages().RowCount())

	s.detail.SetMR(project, &models.MergeRequest{IID: 6, Title: "U", State: models.MRStateOpened})

	s.Require().Equal(0, s.detail.PipelineStages().RowCount(), "stages cleared on MR swap")
	s.Require().Nil(s.detail.PipelineDetailSnapshot())
	s.Require().False(s.detail.LogOpen())
}

func (s *DetailViewSuite) TestOpenJobLogSync_PopulatesLogAndMarksOpen() {
	s.buildAppWithHandler(s.pipelineHandler(
		`[{"id":77,"iid":1,"project_id":11,"status":"failed","ref":"feat/x","web_url":"u"}]`,
		`{"id":77,"status":"failed","ref":"feat/x","sha":"d","web_url":"u"}`,
		`[{"id":21,"name":"test:unit","stage":"test","status":"failed","duration":42.0}]`,
		"trace line 1\ntrace line 2\n",
	))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))
	s.Require().NotNil(s.detail.PipelineStages().SelectedJob())

	s.Require().NoError(s.detail.OpenJobLogSync(context.Background(), project))

	s.Require().True(s.detail.LogOpen())
	jobLog := s.detail.JobLog()
	s.Require().NotNil(jobLog)
	s.Require().NotNil(jobLog.CurrentJob())
	s.Require().Equal(21, jobLog.CurrentJob().ID)
}

func (s *DetailViewSuite) TestOpenJobLogSync_TraceErrorSurfacesAndWidgetShowsError() {
	s.buildAppWithHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/jobs/") && strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"message":"trace boom"}`)
		case strings.Contains(r.URL.Path, "/merge_requests/") && strings.HasSuffix(r.URL.Path, "/pipelines"):
			_, _ = fmt.Fprint(w, `[{"id":77,"iid":1,"project_id":11,"status":"failed","ref":"feat/x","web_url":"u"}]`)
		case strings.HasSuffix(r.URL.Path, "/pipelines/77/jobs"):
			_, _ = fmt.Fprint(w, `[{"id":21,"name":"test:unit","stage":"test","status":"failed","duration":42.0}]`)
		case strings.Contains(r.URL.Path, "/pipelines/"):
			_, _ = fmt.Fprint(w, `{"id":77,"status":"failed","ref":"feat/x","sha":"d","web_url":"u"}`)
		default:
			_, _ = fmt.Fprint(w, `[]`)
		}
	})
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))

	err := s.detail.OpenJobLogSync(context.Background(), project)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "fetch job trace")
	s.Require().True(s.detail.LogOpen(), "error leaves log open showing the error message")
	s.Require().Contains(s.detail.JobLog().statusSnapshot(), ansiRed)
}

func (s *DetailViewSuite) TestApplyJobTrace_StaleSeqIgnored() {
	job := &models.PipelineJob{ID: 21, Name: "test", Stage: "test", Status: models.PipelineStatusFailed}
	s.detail.beginJobLogLoad(job)

	s.detail.applyJobTrace(0, job, "stale trace", nil)

	s.Require().Nil(s.detail.JobLog().CurrentJob(),
		"stale seq must not populate the log")
}

func (s *DetailViewSuite) TestApplyJobTrace_IgnoredAfterClose() {
	job := &models.PipelineJob{ID: 21, Name: "test", Stage: "test", Status: models.PipelineStatusFailed}
	seq := s.detail.beginJobLogLoad(job)
	s.detail.CloseJobLog()

	s.detail.applyJobTrace(seq, job, "late arrival", nil)

	s.Require().Nil(s.detail.JobLog().CurrentJob(),
		"closed log swallows in-flight trace result")
}

func (s *DetailViewSuite) TestPipelineLogHalfPage_DispatchesHalfPaneHeight() {
	s.buildAppWithHandler(s.pipelineHandler(
		`[{"id":77,"iid":1,"project_id":11,"status":"failed","ref":"feat/x","web_url":"u"}]`,
		`{"id":77,"status":"failed","ref":"feat/x","sha":"d","web_url":"u"}`,
		`[{"id":21,"name":"test","stage":"test","status":"failed","duration":42.0}]`,
		"line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n",
	))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened})
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))
	s.Require().NoError(s.detail.OpenJobLogSync(context.Background(), project))

	pane, err := s.g.SetView(keymap.ViewDetailPipelineJobLog, 0, 0, 40, 8, 0)
	if err != nil && !goerrors.Is(err, gocui.ErrUnknownView) {
		s.T().Fatalf("SetView log pane: %v", err)
	}
	pane.SetOrigin(0, 0)

	s.detail.JobLog().ScrollBy(pane, 3)

	_, oy := pane.Origin()
	s.Require().Positive(oy, "ScrollBy advances pane origin")
}

func (s *DetailViewSuite) TestCloseJobLog_ResetsStateAndRequestsStagesFocus() {
	s.buildAppWithHandler(s.pipelineHandler(
		`[{"id":77,"iid":1,"project_id":11,"status":"failed","ref":"feat/x","web_url":"u"}]`,
		`{"id":77,"status":"failed","ref":"feat/x","sha":"d","web_url":"u"}`,
		`[{"id":21,"name":"test:unit","stage":"test","status":"failed","duration":42.0}]`,
		"body",
	))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))
	_ = s.detail.ConsumePendingFocus()
	s.Require().NoError(s.detail.OpenJobLogSync(context.Background(), project))
	_ = s.detail.ConsumePendingFocus()

	s.detail.CloseJobLog()

	s.Require().False(s.detail.LogOpen())
	s.Require().Nil(s.detail.JobLog().CurrentJob())
	s.Require().Equal(keymap.ViewDetailPipelineStages, s.detail.ConsumePendingFocus())
}

func (s *DetailViewSuite) TestCloseJobLog_WhenNotOpen_IsNoOp() {
	s.detail.CloseJobLog()

	s.Require().False(s.detail.LogOpen())
	s.Require().Empty(s.detail.ConsumePendingFocus())
}

func (s *DetailViewSuite) TestSetTab_LeavingPipeline_ResetsLogStateAndPointsFocusAtNewTab() {
	s.buildAppWithHandler(s.pipelineHandler(
		`[{"id":77,"iid":1,"project_id":11,"status":"failed","ref":"feat/x","web_url":"u"}]`,
		`{"id":77,"status":"failed","ref":"feat/x","sha":"d","web_url":"u"}`,
		`[{"id":21,"name":"test:unit","stage":"test","status":"failed","duration":42.0}]`,
		"body",
	))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabPipeline, project))
	s.Require().NoError(s.detail.OpenJobLogSync(context.Background(), project))
	s.Require().True(s.detail.LogOpen())
	_ = s.detail.ConsumePendingFocus()

	s.detail.SetTab(DetailTabConversation, project)

	s.Require().False(s.detail.LogOpen(),
		"leaving Pipeline with log open must reset log state")
	s.Require().Equal(keymap.ViewDetailConversation, s.detail.ConsumePendingFocus(),
		"focus points at Conversation pane, not the now-defunct log")
	s.Require().Nil(s.detail.JobLog().CurrentJob())

	s.detail.SetTab(DetailTabPipeline, project)
	s.Require().Equal(keymap.ViewDetailPipelineStages, s.detail.ConsumePendingFocus(),
		"re-entering Pipeline always lands on the stages pane")
}

func (s *DetailViewSuite) conversationHandler(discussionsJSON string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, discussionsJSON)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":0,"approvals_left":0}`)
		default:
			_, _ = fmt.Fprint(w, `[]`)
		}
	}
}

func (s *DetailViewSuite) TestSetTabSync_Conversation_FetchesAndPopulatesView() {
	s.buildAppWithHandler(s.conversationHandler(`[
		{"id":"d1","notes":[{"id":1,"body":"hi","author":{"username":"a","name":"A"},"resolvable":true,"resolved":false}]},
		{"id":"d2","notes":[{"id":2,"body":"done","author":{"username":"b","name":"B"},"resolvable":true,"resolved":true,"resolved_by":{"id":1,"username":"a","name":"A"}}]}
	]`))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabConversation, project))

	s.Require().Equal(DetailTabConversation, s.detail.CurrentTab())
	conv := s.detail.Conversation()
	s.Require().NotNil(conv)
	s.Require().Equal(2, conv.ThreadCount())
	s.Require().Equal(1, conv.UnresolvedCount())
}

func (s *DetailViewSuite) TestSetTabSync_Conversation_EmptyDiscussions() {
	s.buildAppWithHandler(s.conversationHandler(`[]`))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)

	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabConversation, project))

	s.Require().Equal(0, s.detail.Conversation().ThreadCount())
}

func (s *DetailViewSuite) TestSetTabSync_Conversation_UpstreamErrorWrapped() {
	s.buildAppWithHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"nope"}`)
	})
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	mr := &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened}
	s.detail.SetMR(project, mr)

	err := s.detail.SetTabSync(context.Background(), DetailTabConversation, project)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "fetch mr discussions")
}

func (s *DetailViewSuite) TestApplyConversation_StaleSeqIgnored() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})

	s.detail.applyConversation(0, []*models.Discussion{{ID: "x", Notes: []models.Note{{ID: 1, Resolvable: true, Author: models.User{Username: "u"}}}}}, nil)

	s.Require().Equal(0, s.detail.Conversation().ThreadCount(),
		"stale seq must not clobber current state")
}

func (s *DetailViewSuite) TestSetMR_Conversation_ResetsState() {
	s.buildAppWithHandler(s.conversationHandler(`[
		{"id":"d1","notes":[{"id":1,"body":"hi","author":{"username":"a","name":"A"},"resolvable":true}]}
	]`))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}
	s.detail.SetMR(project, &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened})
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabConversation, project))
	s.Require().Positive(s.detail.Conversation().ThreadCount())

	s.detail.SetMR(project, &models.MergeRequest{IID: 6, Title: "U", State: models.MRStateOpened})

	s.Require().Equal(0, s.detail.Conversation().ThreadCount(),
		"conversation data cleared on MR swap")
}

func (s *DetailViewSuite) TestSetMR_SwapWhileOnConversationTab_RefetchesData() {
	s.buildAppWithHandler(s.conversationHandler(`[
		{"id":"d1","notes":[{"id":1,"body":"hi","author":{"username":"a","name":"A"},"resolvable":true}]}
	]`))
	project := &models.Project{ID: 11, PathWithNamespace: "grp/x"}

	s.detail.SetMR(project, &models.MergeRequest{IID: 5, Title: "T", State: models.MRStateOpened})
	s.Require().NoError(s.detail.SetTabSync(context.Background(), DetailTabConversation, project))
	s.Require().Positive(s.detail.Conversation().ThreadCount())

	s.Require().NoError(s.detail.SetMRSync(context.Background(), project,
		&models.MergeRequest{IID: 6, Title: "U", State: models.MRStateOpened}))

	s.Require().Equal(DetailTabConversation, s.detail.CurrentTab(),
		"tab is preserved across MR swap")
	s.Require().Positive(s.detail.Conversation().ThreadCount(),
		"data must reload when swapping MRs while sitting on Conversation tab; "+
			"otherwise the pane is stuck on the Loading… hint forever")
}

func (s *DetailViewSuite) TestSetTab_Conversation_PendingFocusPointsAtConversationPane() {
	s.detail.SetMR(nil, &models.MergeRequest{IID: 1, Title: "T", State: models.MRStateOpened})

	s.detail.SetTab(DetailTabConversation, nil)

	s.Require().Equal(keymap.ViewDetailConversation, s.detail.ConsumePendingFocus())
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestDetailViewSuite(t *testing.T) {
	suite.Run(t, new(DetailViewSuite))
}

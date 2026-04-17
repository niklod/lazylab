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

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestDetailViewSuite(t *testing.T) {
	suite.Run(t, new(DetailViewSuite))
}

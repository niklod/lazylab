//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jesseduffield/gocui"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/tui"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/views"
)

const (
	detailCfgPath = "/cfg/config.yaml"
	detailWidth   = 160
	detailHeight  = 40
)

const detailProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

const detailOpenedMRsFixture = `[
	{
		"id":1,"iid":10,"title":"Feature Alpha",
		"state":"opened",
		"author":{"id":1,"username":"alice","name":"A","web_url":"u"},
		"reviewers":[
			{"id":3,"username":"carol","name":"C","web_url":"u"},
			{"id":4,"username":"dave","name":"D","web_url":"u"}
		],
		"source_branch":"feat/alpha","target_branch":"main",
		"web_url":"u",
		"created_at":"2026-04-10T14:30:00Z",
		"has_conflicts":false,
		"user_notes_count":3
	},
	{
		"id":2,"iid":11,"title":"Bugfix Beta",
		"state":"opened",
		"author":{"id":2,"username":"bob","name":"B","web_url":"u"},
		"source_branch":"fix/beta","target_branch":"main",
		"web_url":"u",
		"created_at":"2026-04-11T09:15:00Z",
		"has_conflicts":true,
		"user_notes_count":0
	}
]`

type DetailOverviewSuite struct {
	suite.Suite
	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error
}

func (s *DetailOverviewSuite) SetupTest() {
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, detailProjectsFixture)
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[
				{"id":"d1","notes":[{"id":1,"resolvable":true,"resolved":true}]},
				{"id":"d2","notes":[{"id":2,"resolvable":true,"resolved":false}]}
			]`)
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, `[
				{"old_path":"a","new_path":"a","diff":"@@ -1 +1 @@\n-old\n+new\n"}
			]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":0,"approvals_left":0}`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, detailOpenedMRsFixture)
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = fmt.Fprint(w, `{"id":99,"username":"me","name":"Me","web_url":"u"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = "e2e-secret"
	s.Require().NoError(cfg.Save(s.fs, detailCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, detailCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: detailWidth, Height: detailHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *DetailOverviewSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *DetailOverviewSuite) layoutTick() error { return s.manager(s.g) }

func (s *DetailOverviewSuite) detailBuffer() string {
	pane, err := s.g.View(keymap.ViewDetail)
	s.Require().NoError(err)

	return pane.Buffer()
}

func (s *DetailOverviewSuite) dispatch(view string, key any) error {
	s.T().Helper()

	v, err := s.g.View(view)
	if err != nil {
		v = nil
	}
	for _, b := range s.v.Bindings() {
		if b.View == view && b.Key == key {
			return b.Handler(s.g, v)
		}
	}
	s.T().Fatalf("no binding for view=%q key=%v", view, key)

	return nil
}

func (s *DetailOverviewSuite) loadProjectAndMRs() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	s.Require().NoError(s.layoutTick())

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())
}

func (s *DetailOverviewSuite) TestDetail_BeforeSelection_ShowsEmptyHint() {
	s.Require().NoError(s.layoutTick())

	s.Require().Contains(s.detailBuffer(), "Select a merge request")
}

func (s *DetailOverviewSuite) TestEnterOnMR_RendersOverviewFields() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	buf := s.detailBuffer()
	s.Require().Contains(buf, "!10 Feature Alpha")
	s.Require().Contains(buf, "@alice")
	s.Require().Contains(buf, "Reviewers: @carol, @dave")
	s.Require().Contains(buf, "2026-04-10 14:30")
	s.Require().Contains(buf, "O opened")
	s.Require().Contains(buf, "feat/alpha \u2192 main")
	s.Require().Contains(buf, "\u2713 No conflicts")
	s.Require().Contains(buf, "Comments: 3")
}

func (s *DetailOverviewSuite) TestEnterOnMR_KeepsFocusOnMRsPane() {
	s.loadProjectAndMRs()
	_, err := s.g.SetCurrentView(keymap.ViewMRs)
	s.Require().NoError(err)

	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().NotNil(s.g.CurrentView())
	s.Require().Equal(keymap.ViewMRs, s.g.CurrentView().Name(),
		"Enter on mrs pane populates detail without shifting focus")
}

func (s *DetailOverviewSuite) TestEnterOnSecondMR_ReplacesOverview() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'j'))
	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	buf := s.detailBuffer()
	s.Require().Contains(buf, "!11 Bugfix Beta")
	s.Require().Contains(buf, "@bob")
	s.Require().Contains(buf, "fix/beta \u2192 main")
	s.Require().Contains(buf, "\u2717 Has conflicts")
	s.Require().Contains(buf, "Comments: 0")
	s.Require().NotContains(buf, "Reviewers:")
	s.Require().NotContains(buf, "Feature Alpha")
}

func (s *DetailOverviewSuite) TestOverview_RendersDiffStatsAfterPrefetch() {
	s.loadProjectAndMRs()

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	mr := s.v.MRs.SelectedMR()
	s.Require().NotNil(mr)

	s.Require().NoError(s.v.Detail.SetMRSync(context.Background(), project, mr))
	s.Require().NoError(s.v.Detail.SetTabSync(context.Background(), views.DetailTabDiff, project))
	s.v.Detail.SetTab(views.DetailTabOverview, nil)
	s.Require().NoError(s.layoutTick())

	buf := s.detailBuffer()
	s.Require().Contains(buf, "Changes:")
	s.Require().Contains(buf, "+1")
	s.Require().Contains(buf, "-1")
}

func (s *DetailOverviewSuite) TestSetMRSync_RendersResolvedThreadCount() {
	s.loadProjectAndMRs()

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	mr := s.v.MRs.SelectedMR()
	s.Require().NotNil(mr)
	s.Require().NoError(s.v.Detail.SetMRSync(context.Background(), project, mr))
	s.Require().NoError(s.layoutTick())

	s.Require().Contains(s.detailBuffer(), "Comments: 3 \u26A0 (1/2 resolved)")
}

func (s *DetailOverviewSuite) rewireHandler(handler http.HandlerFunc) {
	s.srv.Close()
	s.srv = httptest.NewServer(handler)
	newClient, err := gitlab.New(config.GitLabConfig{URL: s.srv.URL, Token: "e2e-secret"}, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app.GitLab = newClient
}

func (s *DetailOverviewSuite) TestOverview_NoRequiredApprovals_RendersDimHint() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))

	project := s.v.Repos.SelectedProject()
	mr := s.v.MRs.SelectedMR()
	s.Require().NoError(s.v.Detail.SetMRSync(context.Background(), project, mr))
	s.Require().NoError(s.layoutTick())

	buf := s.detailBuffer()
	s.Require().Contains(buf, "Approvals:")
	s.Require().Contains(buf, "no approvals required")
}

func (s *DetailOverviewSuite) TestOverview_ApprovalsMissing_RendersRedCross() {
	s.rewireHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, detailProjectsFixture)
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":false,"approvals_required":1,"approvals_left":1}`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, detailOpenedMRsFixture)
		default:
			http.NotFound(w, r)
		}
	})
	s.loadProjectAndMRs()

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	mr := s.v.MRs.SelectedMR()
	s.Require().NotNil(mr)
	s.Require().NoError(s.v.Detail.SetMRSync(context.Background(), project, mr))
	s.Require().NoError(s.layoutTick())

	buf := s.detailBuffer()
	s.Require().Contains(buf, "Approvals:")
	s.Require().Contains(buf, "\u2717 0/1 approvals received")
	s.Require().NotContains(buf, "\u2713 0/1")
}

func (s *DetailOverviewSuite) TestOverview_AllApprovalsReceived_RendersGreenCheck() {
	s.rewireHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, detailProjectsFixture)
		case strings.Contains(r.URL.Path, "/discussions"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/diffs"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.Contains(r.URL.Path, "/approvals"):
			_, _ = fmt.Fprint(w, `{"approved":true,"approvals_required":2,"approvals_left":0}`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, detailOpenedMRsFixture)
		default:
			http.NotFound(w, r)
		}
	})
	s.loadProjectAndMRs()

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	mr := s.v.MRs.SelectedMR()
	s.Require().NotNil(mr)
	s.Require().NoError(s.v.Detail.SetMRSync(context.Background(), project, mr))
	s.Require().NoError(s.layoutTick())

	buf := s.detailBuffer()
	s.Require().Contains(buf, "Approvals:")
	s.Require().Contains(buf, "\u2713 2/2 approvals received")
}

func (s *DetailOverviewSuite) TestEnterOnEmptyMRList_LeavesEmptyHint() {
	s.srv.Close()
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, detailProjectsFixture)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	newClient, err := gitlab.New(config.GitLabConfig{URL: s.srv.URL, Token: "e2e-secret"}, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app.GitLab = newClient

	s.loadProjectAndMRs()
	s.Require().NoError(s.dispatch(keymap.ViewMRs, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().Contains(s.detailBuffer(), "Select a merge request")
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestDetailOverviewSuite(t *testing.T) {
	suite.Run(t, new(DetailOverviewSuite))
}

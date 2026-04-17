//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
	mrsCfgPath   = "/cfg/config.yaml"
	mrsE2EWidth  = 140
	mrsE2EHeight = 40
)

// Projects fixture — one row so we can Enter straight into MR loading.
const mrsProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

// MR fixtures keyed by the ?state= query arg.
var mrsByState = map[string]string{
	"opened": `[
		{"id":1,"iid":1,"title":"Feature Alpha","state":"opened","author":{"id":1,"username":"alice","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"},
		{"id":2,"iid":2,"title":"Bugfix Beta","state":"opened","author":{"id":2,"username":"bob","name":"B","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}
	]`,
	"merged": `[
		{"id":3,"iid":3,"title":"Merged Gamma","state":"merged","author":{"id":1,"username":"alice","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}
	]`,
	"closed": `[]`,
	"all":    `[]`,
}

type MRsRenderSuite struct {
	suite.Suite
	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error
	mrCalls atomic.Int32
}

func (s *MRsRenderSuite) SetupTest() {
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, mrsProjectsFixture)

			return
		case strings.Contains(r.URL.Path, "/merge_requests"):
			s.mrCalls.Add(1)
			state := r.URL.Query().Get("state")
			body, ok := mrsByState[state]
			if !ok {
				body = "[]"
			}
			_, _ = fmt.Fprint(w, body)

			return
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = fmt.Fprint(w, `{"id":99,"username":"me","name":"Me","web_url":"u"}`)

			return
		}
		http.NotFound(w, r)
	}))

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = "e2e-secret"
	s.Require().NoError(cfg.Save(s.fs, mrsCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, mrsCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: mrsE2EWidth, Height: mrsE2EHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *MRsRenderSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
	s.mrCalls.Store(0)
}

func (s *MRsRenderSuite) layoutTick() error { return s.manager(s.g) }

func (s *MRsRenderSuite) mrsBuffer() string {
	pane, err := s.g.View(keymap.ViewMRs)
	s.Require().NoError(err)

	return pane.Buffer()
}

func (s *MRsRenderSuite) dispatch(view string, key any) error {
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

// loadProjectAndMRs seeds the repos pane and runs the MR fetch synchronously
// against the fake server so we don't depend on gocui MainLoop draining
// g.Update callbacks.
func (s *MRsRenderSuite) loadProjectAndMRs() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	s.Require().NoError(s.layoutTick())

	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())
}

func (s *MRsRenderSuite) TestEnterOnReposPane_MovesFocusToMRs() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	s.Require().NoError(s.layoutTick())
	// Baseline: focus starts on the repos pane.
	s.Require().Equal(keymap.ViewRepos, s.g.CurrentView().Name())

	s.Require().NoError(s.dispatch(keymap.ViewRepos, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal(keymap.ViewMRs, s.g.CurrentView().Name(), "Enter on repos pane should transfer focus to mrs pane")
}

func (s *MRsRenderSuite) TestOpenedMRs_RenderAfterProjectSelection() {
	s.loadProjectAndMRs()

	buf := s.mrsBuffer()
	s.Require().Contains(buf, "[state=opened owner=all]")
	s.Require().Contains(buf, "!1 O alice")
	s.Require().Contains(buf, "Feature Alpha")
	s.Require().Contains(buf, "!2 O bob")
	s.Require().Contains(buf, "Bugfix Beta")
}

func (s *MRsRenderSuite) TestCycleStateFilter_ChangesTableContents() {
	s.loadProjectAndMRs()
	before := s.mrCalls.Load()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 's'))
	// cycle triggers async; in tests we bypass with a sync reload.
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal("merged", string(s.v.MRs.StateFilter()))
	buf := s.mrsBuffer()
	s.Require().Contains(buf, "[state=merged")
	s.Require().Contains(buf, "!3 M alice")
	s.Require().Contains(buf, "Merged Gamma")
	s.Require().NotContains(buf, "Feature Alpha", "opened MRs no longer rendered after cycling to merged")
	s.Require().Greater(s.mrCalls.Load(), before, "cycling state triggers an MR refetch")
}

func (s *MRsRenderSuite) TestCycleOwnerFilter_SendsAuthorIDForMine() {
	s.loadProjectAndMRs()

	var gotAuthorID atomic.Value
	s.srv.Close()
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, mrsProjectsFixture)
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = fmt.Fprint(w, `{"id":77,"username":"me","name":"Me","web_url":"u"}`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			if aid := r.URL.Query().Get("author_id"); aid != "" {
				gotAuthorID.Store(aid)
			}
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	// Point the existing client at the new server by rebuilding it — the
	// httptest.Client transport handles arbitrary URLs, but the baseURL in
	// the gogitlab client is cached, so swap both.
	newClient, err := gitlab.New(config.GitLabConfig{URL: s.srv.URL, Token: "e2e-secret"}, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app.GitLab = newClient

	s.Require().NoError(s.dispatch(keymap.ViewMRs, 'o'))
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))

	s.Require().Equal(string(s.v.MRs.OwnerFilter()), "mine")
	raw, ok := gotAuthorID.Load().(string)
	s.Require().True(ok, "author_id query captured")
	s.Require().Equal("77", raw)
}

func (s *MRsRenderSuite) TestSearch_FiltersInPlace() {
	s.loadProjectAndMRs()

	s.Require().NoError(s.dispatch(keymap.ViewMRs, '/'))
	s.Require().NoError(s.layoutTick())

	sv, err := s.g.View(keymap.ViewMRsSearch)
	s.Require().NoError(err)
	sv.Clear()
	_, err = sv.Write([]byte("alpha"))
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewMRsSearch, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	buf := s.mrsBuffer()
	s.Require().Contains(buf, "Feature Alpha")
	s.Require().NotContains(buf, "Bugfix Beta")
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestMRsRenderSuite(t *testing.T) {
	suite.Run(t, new(MRsRenderSuite))
}

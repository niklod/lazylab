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
	reposCfgPath   = "/cfg/config.yaml"
	reposE2EWidth  = 120
	reposE2EHeight = 40
)

const projectsFixture = `[
	{"id":1,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-01T10:00:00Z","archived":false},
	{"id":2,"name":"Bravo","path_with_namespace":"grp/bravo","web_url":"https://gl/grp/bravo","last_activity_at":"2026-03-20T10:00:00Z","archived":false},
	{"id":3,"name":"Charlie","path_with_namespace":"team/charlie","web_url":"https://gl/team/charlie","last_activity_at":"2026-02-10T10:00:00Z","archived":false}
]`

type ReposRenderSuite struct {
	suite.Suite
	srv     *httptest.Server
	fs      afero.Fs
	g       *gocui.Gui
	v       *views.Views
	app     *appcontext.AppContext
	manager func(*gocui.Gui) error
}

func (s *ReposRenderSuite) SetupTest() {
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/projects") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, projectsFixture)
	}))

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = "e2e-secret"
	s.Require().NoError(cfg.Save(s.fs, reposCfgPath))

	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, cache.New(cfg.Cache, s.fs), s.fs, reposCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: reposE2EWidth, Height: reposE2EHeight})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))
	s.Require().NoError(s.layoutTick())
}

func (s *ReposRenderSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *ReposRenderSuite) layoutTick() error {
	return s.manager(s.g)
}

func (s *ReposRenderSuite) reposBuffer() string {
	pane, err := s.g.View(keymap.ViewRepos)
	s.Require().NoError(err)

	return pane.Buffer()
}

func (s *ReposRenderSuite) dispatch(view string, key any) error {
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

func (s *ReposRenderSuite) TestProjects_RenderInReposPane() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	s.Require().NoError(s.layoutTick())

	buf := s.reposBuffer()
	s.Require().Contains(buf, "grp/alpha")
	s.Require().Contains(buf, "grp/bravo")
	s.Require().Contains(buf, "team/charlie")
	s.Require().NotContains(buf, "☆", "unfavourited rows render no icon (design has none)")
}

func (s *ReposRenderSuite) TestSearch_EscOnReposPaneClearsSubmittedFilter() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))

	s.Require().NoError(s.dispatch(keymap.ViewRepos, '/'))
	s.Require().NoError(s.layoutTick())

	searchV, err := s.g.View(keymap.ViewReposSearch)
	s.Require().NoError(err)
	searchV.Clear()
	_, err = searchV.Write([]byte("team"))
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewReposSearch, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	filtered := s.reposBuffer()
	s.Require().Contains(filtered, "team/charlie")
	s.Require().NotContains(filtered, "grp/alpha")

	s.Require().NoError(s.dispatch(keymap.ViewRepos, gocui.KeyEsc))
	s.Require().NoError(s.layoutTick())

	buf := s.reposBuffer()
	s.Require().Contains(buf, "grp/alpha", "Esc on repos pane restores full list")
	s.Require().Contains(buf, "grp/bravo")
	s.Require().Contains(buf, "team/charlie")
}

func (s *ReposRenderSuite) TestSearch_FiltersInPlace() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))

	s.Require().NoError(s.dispatch(keymap.ViewRepos, '/'))
	s.Require().NoError(s.layoutTick())

	searchV, err := s.g.View(keymap.ViewReposSearch)
	s.Require().NoError(err)
	searchV.Clear()
	_, err = searchV.Write([]byte("team"))
	s.Require().NoError(err)

	s.Require().NoError(s.dispatch(keymap.ViewReposSearch, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	buf := s.reposBuffer()
	s.Require().Contains(buf, "team/charlie")
	s.Require().NotContains(buf, "grp/alpha")
	s.Require().NotContains(buf, "grp/bravo")
}

func (s *ReposRenderSuite) TestToggleFavourite_PersistsToConfigYAML() {
	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))

	s.Require().NoError(s.dispatch(keymap.ViewRepos, 'j'))
	s.Require().NoError(s.dispatch(keymap.ViewRepos, 't'))

	reloaded, err := config.Load(s.fs, reposCfgPath)
	s.Require().NoError(err)
	s.Require().Equal([]string{"grp/bravo"}, reloaded.Repositories.Favorites)

	s.Require().NoError(s.layoutTick())
	buf := s.reposBuffer()
	// Header line is row 0; favourites bubble to row 1.
	lines := strings.SplitN(strings.TrimSpace(buf), "\n", 3)
	s.Require().GreaterOrEqual(len(lines), 2, "expected header + at least one row")
	favRow := lines[1]
	s.Require().Contains(favRow, "★", "favourite icon present")
	s.Require().Contains(favRow, "grp/bravo", "favourite bubbled to top")
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; running suites in parallel races.
func TestReposRenderSuite(t *testing.T) {
	suite.Run(t, new(ReposRenderSuite))
}

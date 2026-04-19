package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

const reposTestConfigPath = "/cfg/config.yaml"

type ReposViewSuite struct {
	suite.Suite
	g       *gocui.Gui
	app     *appcontext.AppContext
	fs      afero.Fs
	cfgPath string
}

func (s *ReposViewSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = "https://gitlab.example.com"
	cfg.GitLab.Token = testGitLabToken
	cfg.Repositories.Favorites = nil
	client, err := gitlab.New(cfg.GitLab)
	s.Require().NoError(err)
	c := cache.New(cfg.Cache, s.fs)
	s.cfgPath = reposTestConfigPath
	s.Require().NoError(cfg.Save(s.fs, s.cfgPath))
	s.app = appcontext.New(cfg, client, c, s.fs, s.cfgPath)

	// Pre-create the repos pane so handlers that refocus it don't error out.
	_, err = s.g.SetView(keymap.ViewRepos, 0, 0, 40, 20, 0)
	if err != nil && !strings.Contains(err.Error(), "unknown view") {
		s.T().Fatalf("SetView repos: %v", err)
	}
}

func (s *ReposViewSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
}

func (s *ReposViewSuite) seed(v *ReposView, projects []*models.Project) {
	v.mu.Lock()
	v.all = projects
	v.loading = false
	v.refreshFavSetLocked()
	v.sortLocked()
	v.applyFilterLocked()
	v.mu.Unlock()
}

func (s *ReposViewSuite) TestSort_FavouritesFirstThenLastActivityDesc() {
	now := time.Now().UTC()
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/old-fav", LastActivityAt: now.Add(-48 * time.Hour)},
		{ID: 2, PathWithNamespace: "grp/recent", LastActivityAt: now.Add(-1 * time.Hour)},
		{ID: 3, PathWithNamespace: "grp/recent-fav", LastActivityAt: now.Add(-2 * time.Hour)},
	}
	s.app.Config.Repositories.Favorites = []string{"grp/old-fav", "grp/recent-fav"}

	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	s.Require().Len(v.filtered, 3)
	s.Require().Equal("grp/recent-fav", v.filtered[0].PathWithNamespace, "fav group sorted by activity desc, comes first")
	s.Require().Equal("grp/old-fav", v.filtered[1].PathWithNamespace)
	s.Require().Equal("grp/recent", v.filtered[2].PathWithNamespace)
}

func (s *ReposViewSuite) TestFilter_SubstringCaseInsensitive() {
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/alpha"},
		{ID: 2, PathWithNamespace: "grp/BRAVO"},
		{ID: 3, PathWithNamespace: "other/alpha-mirror"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.query = "Alpha"
	v.applyFilterLocked()
	v.mu.Unlock()

	paths := make([]string, 0, len(v.filtered))
	for _, p := range v.filtered {
		paths = append(paths, p.PathWithNamespace)
	}
	s.Require().ElementsMatch([]string{"grp/alpha", "other/alpha-mirror"}, paths)
}

func (s *ReposViewSuite) TestToggleFavorite_AddsAndPersists() {
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/alpha"},
		{ID: 2, PathWithNamespace: "grp/bravo"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.cursor = 1
	v.mu.Unlock()

	s.Require().NoError(v.handleToggleFavorite(s.g, nil))

	s.Require().Equal([]string{"grp/bravo"}, s.app.Config.Repositories.Favorites)

	reloaded, err := config.Load(s.fs, s.cfgPath)
	s.Require().NoError(err)
	s.Require().Equal([]string{"grp/bravo"}, reloaded.Repositories.Favorites)

	s.Require().Equal("grp/bravo", v.filtered[0].PathWithNamespace, "favourite bubbled to top after re-sort")
}

func (s *ReposViewSuite) TestToggleFavorite_RemovesWhenAlreadyPresent() {
	s.app.Config.Repositories.Favorites = []string{"grp/alpha"}
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/alpha"},
		{ID: 2, PathWithNamespace: "grp/bravo"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.cursor = 0
	v.mu.Unlock()

	s.Require().NoError(v.handleToggleFavorite(s.g, nil))

	s.Require().Empty(s.app.Config.Repositories.Favorites)

	reloaded, err := config.Load(s.fs, s.cfgPath)
	s.Require().NoError(err)
	s.Require().Empty(reloaded.Repositories.Favorites)
}

func (s *ReposViewSuite) TestCursor_Navigation_StaysInBounds() {
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "a/a"},
		{ID: 2, PathWithNamespace: "b/b"},
		{ID: 3, PathWithNamespace: "c/c"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	s.Require().NoError(v.handleUp(s.g, nil))
	s.Require().Equal(0, v.cursor, "up at top stays at 0")

	s.Require().NoError(v.handleDown(s.g, nil))
	s.Require().NoError(v.handleDown(s.g, nil))
	s.Require().NoError(v.handleDown(s.g, nil))
	s.Require().Equal(2, v.cursor, "down at bottom stays at last index")

	s.Require().NoError(v.handleTop(s.g, nil))
	s.Require().Equal(0, v.cursor)

	s.Require().NoError(v.handleBottom(s.g, nil))
	s.Require().Equal(2, v.cursor)
}

func (s *ReposViewSuite) TestSelectedProject_EmptyList_ReturnsNil() {
	v := NewRepos(s.g, s.app)

	s.Require().Nil(v.SelectedProject())
}

func (s *ReposViewSuite) TestSearchActive_DefaultFalse() {
	v := NewRepos(s.g, s.app)

	s.Require().False(v.SearchActive())
}

func (s *ReposViewSuite) TestCancelSearch_ClearsQueryAndRestoresAllRows() {
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/alpha"},
		{ID: 2, PathWithNamespace: "grp/bravo"},
		{ID: 3, PathWithNamespace: "team/charlie"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.query = "team"
	v.searchActive = true
	v.applyFilterLocked()
	v.mu.Unlock()

	s.Require().Len(v.filtered, 1)

	s.Require().NoError(v.handleCancelSearch(s.g, nil))

	s.Require().Empty(v.query)
	s.Require().False(v.SearchActive())
	s.Require().Len(v.filtered, 3, "cancel restores all rows")
	s.Require().Equal(0, v.cursor, "cursor resets on cancel")
}

func (s *ReposViewSuite) TestClearFilter_OnReposEscRestoresFullList() {
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/alpha"},
		{ID: 2, PathWithNamespace: "grp/bravo"},
		{ID: 3, PathWithNamespace: "team/charlie"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.query = "team"
	v.cursor = 0
	v.applyFilterLocked()
	v.mu.Unlock()

	s.Require().Len(v.filtered, 1)

	s.Require().NoError(v.handleClearFilter(s.g, nil))

	s.Require().Empty(v.query)
	s.Require().Len(v.filtered, 3, "Esc on repos pane clears submitted filter")
}

func (s *ReposViewSuite) TestClearFilter_NoActiveFilter_IsNoop() {
	projects := []*models.Project{{ID: 1, PathWithNamespace: "grp/alpha"}}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	s.Require().NoError(v.handleClearFilter(s.g, nil))

	s.Require().Empty(v.query)
	s.Require().Len(v.filtered, 1)
}

func (s *ReposViewSuite) TestSubmitSearch_AppliesTrimmedQuery() {
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/alpha"},
		{ID: 2, PathWithNamespace: "grp/bravo"},
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.searchActive = true
	v.mu.Unlock()

	searchView, err := s.g.SetView(keymap.ViewReposSearch, 0, 0, 20, 2, 0)
	if err != nil && !strings.Contains(err.Error(), "unknown view") {
		s.T().Fatalf("SetView: %v", err)
	}
	searchView.Clear()
	_, _ = searchView.Write([]byte("  bravo  \n"))

	s.Require().NoError(v.handleSubmitSearch(s.g, searchView))

	s.Require().Equal("bravo", v.query, "query is trimmed on submit")
	s.Require().False(v.SearchActive())
	s.Require().Len(v.filtered, 1)
	s.Require().Equal("grp/bravo", v.filtered[0].PathWithNamespace)
}

func (s *ReposViewSuite) TestRender_CursorBeyondViewport_ScrollsAndCursorStaysInViewport() {
	// Short pane: InnerHeight = 5 (height 7 − 2 for borders).
	pane, err := s.g.SetView("scroll_test_pane", 0, 0, 20, 6, 0)
	if err != nil && !strings.Contains(err.Error(), "unknown view") {
		s.T().Fatalf("SetView: %v", err)
	}

	projects := make([]*models.Project, 0, 20)
	for i := 0; i < 20; i++ {
		projects = append(projects, &models.Project{ID: i, PathWithNamespace: fmt.Sprintf("grp/p%02d", i)})
	}
	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	v.mu.Lock()
	v.cursor = 15
	v.mu.Unlock()

	v.Render(pane)

	_, oy := pane.Origin()
	_, innerH := pane.InnerSize()
	_, cy := pane.Cursor()
	s.Require().Positive(oy, "origin must scroll down when cursor is below viewport")
	// Header line at row 0 shifts data rows by +1, so cursor=15 lives at content row 16.
	s.Require().Equal(v.cursor+1-oy, cy, "on-screen cursor row must be the content row minus origin")
	s.Require().GreaterOrEqual(cy, 0, "on-screen cursor row non-negative")
	s.Require().Less(cy, innerH, "on-screen cursor row within viewport height")
}

func (s *ReposViewSuite) TestRender_HeaderAndFavouriteIcon_MatchDesign() {
	now := time.Now().UTC()
	projects := []*models.Project{
		{ID: 1, PathWithNamespace: "grp/fav", LastActivityAt: now.Add(-2 * time.Minute)},
		{ID: 2, PathWithNamespace: "grp/plain", LastActivityAt: now.Add(-3 * time.Hour)},
	}
	s.app.Config.Repositories.Favorites = []string{"grp/fav"}

	v := NewRepos(s.g, s.app)
	s.seed(v, projects)

	pane, perr := s.g.View(keymap.ViewRepos)
	s.Require().NoError(perr)
	v.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "[1] Repositories")
	s.Require().Contains(buf, "2/2", "header includes filtered/total count")
	// gocui re-emits SGR sequences with a trailing `;` so we match the
	// truecolor RGB prefix rather than the exact `\x1b[...m` byte form.
	s.Require().Contains(buf, "\x1b[38;2;217;119;87", "favourite icon carries the accent RGB code")
	s.Require().Contains(buf, "★", "favourite glyph rendered")
	s.Require().Contains(buf, "grp/fav")
	s.Require().Contains(buf, "grp/plain")
	s.Require().Contains(buf, "\x1b[38;2;122;121;112", "dim RGB code appears for timestamps + header meta")
	s.Require().NotContains(buf, "☆", "non-favourites no longer render the empty star glyph")
}

func (s *ReposViewSuite) TestRender_FirstRunEmpty_ShowsConfigHint() {
	v := NewRepos(s.g, s.app)
	v.mu.Lock()
	v.loading = false
	v.all = nil
	v.filtered = nil
	v.query = ""
	v.mu.Unlock()

	pane, perr := s.g.View(keymap.ViewRepos)
	s.Require().NoError(perr)
	v.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "No projects yet")
	s.Require().Contains(buf, "~/.config/lazylab/config.yaml")
}

func (s *ReposViewSuite) TestRender_FilterMissEmpty_ShowsResetHint() {
	v := NewRepos(s.g, s.app)
	v.mu.Lock()
	v.loading = false
	v.all = []*models.Project{{ID: 1, PathWithNamespace: "grp/x"}}
	v.allLower = []string{"grp/x"}
	v.query = "no-match-anywhere"
	v.applyFilterLocked()
	v.mu.Unlock()

	pane, perr := s.g.View(keymap.ViewRepos)
	s.Require().NoError(perr)
	v.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "No projects match")
	s.Require().Contains(buf, "Esc")
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestReposViewSuite(t *testing.T) {
	suite.Run(t, new(ReposViewSuite))
}

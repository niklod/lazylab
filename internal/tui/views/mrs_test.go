package views

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
	"github.com/niklod/lazylab/internal/models"
	"github.com/niklod/lazylab/internal/tui/keymap"
)

const mrsTestCfgPath = "/cfg/config.yaml"

type MRsViewSuite struct {
	suite.Suite
	g   *gocui.Gui
	app *appcontext.AppContext
	fs  afero.Fs
	srv *httptest.Server
	mrs *MRsView
}

func (s *MRsViewSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	_, err = s.g.SetView(keymap.ViewMRs, 0, 0, 60, 30, 0)
	if err != nil && !strings.Contains(err.Error(), "unknown view") {
		s.T().Fatalf("SetView mrs: %v", err)
	}
}

func (s *MRsViewSuite) TearDownTest() {
	if s.g != nil {
		s.g.Close()
		s.g = nil
	}
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

// buildView stands up a fake GitLab server whose handler is driven by the
// caller and wires an MRsView against it. Returns the view plus the hit
// counter for endpoint-specific assertions.
func (s *MRsViewSuite) buildView(handler http.HandlerFunc) {
	s.srv = httptest.NewServer(handler)

	s.fs = afero.NewMemMapFs()
	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = testGitLabToken
	client, err := gitlab.New(cfg.GitLab, gitlab.WithHTTPClient(s.srv.Client()))
	s.Require().NoError(err)
	c := cache.New(cfg.Cache, s.fs)
	s.Require().NoError(cfg.Save(s.fs, mrsTestCfgPath))
	s.app = appcontext.New(cfg, client, c, s.fs, mrsTestCfgPath)
	s.mrs = NewMRs(s.g, s.app)
}

func (s *MRsViewSuite) seedItems(mrs []*models.MergeRequest) {
	s.mrs.mu.Lock()
	s.mrs.current = &models.Project{ID: 1, PathWithNamespace: "grp/alpha"}
	s.mrs.loading = false
	s.mrs.all = mrs
	s.mrs.rebuildLowerLocked()
	s.mrs.applyFilterLocked()
	s.mrs.mu.Unlock()
}

func (s *MRsViewSuite) TestRender_BeforeProjectSelection_ShowsHint() {
	s.buildView(http.NotFound)

	pane, err := s.g.View(keymap.ViewMRs)
	s.Require().NoError(err)
	s.mrs.Render(pane)

	s.Require().Contains(pane.Buffer(), "Select a project")
}

func (s *MRsViewSuite) TestSetProjectSync_PopulatesTable() {
	var path string
	s.buildView(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"id":1,"iid":10,"title":"Feature","state":"opened","author":{"id":1,"username":"alice","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"},
			{"id":2,"iid":11,"title":"Bugfix","state":"opened","author":{"id":2,"username":"bob","name":"B","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}
		]`)
	})

	p := &models.Project{ID: 42, PathWithNamespace: "grp/alpha"}
	s.Require().NoError(s.mrs.SetProjectSync(context.Background(), p))

	pane, err := s.g.View(keymap.ViewMRs)
	s.Require().NoError(err)
	s.mrs.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "!10")
	s.Require().Contains(buf, "Feature")
	s.Require().Contains(buf, "@alice")
	s.Require().Contains(buf, "!11")
	s.Require().Contains(buf, "@bob")
	s.Require().Contains(path, "/projects/42/merge_requests")
}

func (s *MRsViewSuite) TestSetProjectSync_WrapsFetchError() {
	s.buildView(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"500"}`)
	})

	p := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	err := s.mrs.SetProjectSync(context.Background(), p)

	s.Require().Error(err)

	pane, perr := s.g.View(keymap.ViewMRs)
	s.Require().NoError(perr)
	s.mrs.Render(pane)
	s.Require().Contains(pane.Buffer(), "Error loading merge requests")
}

func (s *MRsViewSuite) TestRender_WithFiltersAndItems_IncludesHeader() {
	s.buildView(http.NotFound)
	s.mrs.mu.Lock()
	s.mrs.stateFilter = models.MRStateFilterMerged
	s.mrs.ownerFilter = models.MROwnerFilterMine
	s.mrs.mu.Unlock()

	s.seedItems([]*models.MergeRequest{
		{IID: 1, State: models.MRStateMerged, Author: models.User{Username: "a"}, Title: "x"},
	})
	pane, _ := s.g.View(keymap.ViewMRs)
	s.mrs.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "[2] Merge Requests")
	s.Require().Contains(buf, "state:merged")
	s.Require().Contains(buf, "owner:mine")
	s.Require().Contains(buf, "1/1")
	s.Require().Contains(buf, "!1")
	s.Require().Contains(buf, "@a")
}

func (s *MRsViewSuite) TestFilter_SubstringCaseInsensitive_TitleOrAuthor() {
	s.buildView(http.NotFound)
	s.seedItems([]*models.MergeRequest{
		{IID: 1, Title: "Alpha FIX", Author: models.User{Username: "zoe"}, State: models.MRStateOpened},
		{IID: 2, Title: "Beta feature", Author: models.User{Username: "ALPHA-user"}, State: models.MRStateOpened},
		{IID: 3, Title: "Docs", Author: models.User{Username: "mike"}, State: models.MRStateOpened},
	})

	s.mrs.mu.Lock()
	s.mrs.query = "alpha"
	s.mrs.applyFilterLocked()
	s.mrs.mu.Unlock()

	iids := []int{}
	for _, mr := range s.mrs.filtered {
		iids = append(iids, mr.IID)
	}
	s.Require().ElementsMatch([]int{1, 2}, iids)
}

func (s *MRsViewSuite) TestCycleStateFilter_Rotates() {
	s.buildView(http.NotFound)

	s.Require().Equal(models.MRStateFilterOpened, s.mrs.StateFilter())

	next := nextStateFilter(s.mrs.StateFilter())
	s.Require().Equal(models.MRStateFilterMerged, next)
	s.Require().Equal(models.MRStateFilterClosed, nextStateFilter(next))
	s.Require().Equal(models.MRStateFilterAll, nextStateFilter(nextStateFilter(next)))
	s.Require().Equal(models.MRStateFilterOpened, nextStateFilter(models.MRStateFilterAll))
}

func (s *MRsViewSuite) TestCycleOwnerFilter_Rotates() {
	s.buildView(http.NotFound)

	s.Require().Equal(models.MROwnerFilterAll, nextOwnerFilter(models.MROwnerFilterReviewer))
	s.Require().Equal(models.MROwnerFilterMine, nextOwnerFilter(models.MROwnerFilterAll))
	s.Require().Equal(models.MROwnerFilterReviewer, nextOwnerFilter(models.MROwnerFilterMine))
}

func (s *MRsViewSuite) TestCycleOwnerFilter_MineFetchesCurrentUserAndSetsAuthorID() {
	var (
		userHits atomic.Int32
		mrQuery  atomic.Value
	)
	s.buildView(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/user"):
			userHits.Add(1)
			_, _ = fmt.Fprint(w, `{"id":77,"username":"me","name":"Me","web_url":"u"}`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			mrQuery.Store(r.URL.RawQuery)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})

	s.mrs.mu.Lock()
	s.mrs.ownerFilter = models.MROwnerFilterMine
	s.mrs.mu.Unlock()

	p := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.Require().NoError(s.mrs.SetProjectSync(context.Background(), p))

	s.Require().Equal(int32(1), userHits.Load(), "mine triggers current-user fetch")
	raw, ok := mrQuery.Load().(string)
	s.Require().True(ok, "mr fetch made")
	s.Require().Contains(raw, "author_id=77")
}

func (s *MRsViewSuite) TestCycleOwnerFilter_ReviewerSetsReviewerID() {
	var mrQuery atomic.Value
	s.buildView(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/user"):
			_, _ = fmt.Fprint(w, `{"id":77,"username":"me","name":"Me","web_url":"u"}`)
		case strings.Contains(r.URL.Path, "/merge_requests"):
			mrQuery.Store(r.URL.RawQuery)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})

	s.mrs.mu.Lock()
	s.mrs.ownerFilter = models.MROwnerFilterReviewer
	s.mrs.mu.Unlock()

	p := &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.Require().NoError(s.mrs.SetProjectSync(context.Background(), p))

	raw, ok := mrQuery.Load().(string)
	s.Require().True(ok)
	s.Require().Contains(raw, "reviewer_id=77")
}

func (s *MRsViewSuite) TestCursor_Navigation_StaysInBounds() {
	s.buildView(http.NotFound)
	s.seedItems([]*models.MergeRequest{
		{IID: 1, Author: models.User{Username: "a"}, State: models.MRStateOpened, Title: "a"},
		{IID: 2, Author: models.User{Username: "b"}, State: models.MRStateOpened, Title: "b"},
		{IID: 3, Author: models.User{Username: "c"}, State: models.MRStateOpened, Title: "c"},
	})

	s.Require().NoError(s.mrs.handleUp(s.g, nil))
	s.Require().Equal(0, s.mrs.cursor)

	s.Require().NoError(s.mrs.handleDown(s.g, nil))
	s.Require().NoError(s.mrs.handleDown(s.g, nil))
	s.Require().NoError(s.mrs.handleDown(s.g, nil))
	s.Require().Equal(2, s.mrs.cursor)

	s.Require().NoError(s.mrs.handleTop(s.g, nil))
	s.Require().Equal(0, s.mrs.cursor)

	s.Require().NoError(s.mrs.handleBottom(s.g, nil))
	s.Require().Equal(2, s.mrs.cursor)
}

func (s *MRsViewSuite) TestClearFilter_NoopWhenNoQuery() {
	s.buildView(http.NotFound)
	s.seedItems([]*models.MergeRequest{
		{IID: 1, Author: models.User{Username: "a"}, Title: "t", State: models.MRStateOpened},
	})

	s.Require().NoError(s.mrs.handleClearFilter(s.g, nil))
	s.Require().Empty(s.mrs.query)
	s.Require().Len(s.mrs.filtered, 1)
}

func (s *MRsViewSuite) TestBeginLoad_SupersedingFetchCancelsPreviousContext() {
	s.buildView(http.NotFound)

	first, _, _, _ := s.mrs.beginLoad(context.Background(), &models.Project{ID: 1, PathWithNamespace: "grp/x"}) //nolint:dogsled
	s.Require().NoError(first.Err(), "freshly derived context starts uncancelled")

	second, _, _, _ := s.mrs.beginLoad(context.Background(), &models.Project{ID: 2, PathWithNamespace: "grp/y"}) //nolint:dogsled
	s.Require().ErrorIs(first.Err(), context.Canceled,
		"previous in-flight context must be cancelled when a new load begins")
	s.Require().NoError(second.Err(), "newly derived context is not cancelled by its own birth")
}

func (s *MRsViewSuite) TestApply_StaleLoadIgnored() {
	s.buildView(http.NotFound)

	s.mrs.mu.Lock()
	s.mrs.loadSeq = 5
	s.mrs.mu.Unlock()

	s.mrs.apply(3, []*models.MergeRequest{{IID: 99}}, nil)

	s.mrs.mu.Lock()
	s.Require().Empty(s.mrs.all, "older seq must not clobber newer state")
	s.mrs.mu.Unlock()

	s.mrs.apply(5, []*models.MergeRequest{{IID: 42}}, nil)

	s.mrs.mu.Lock()
	defer s.mrs.mu.Unlock()
	s.Require().Len(s.mrs.all, 1, "matching seq applies cleanly")
	s.Require().Equal(42, s.mrs.all[0].IID)
}

func (s *MRsViewSuite) TestRender_StateGlyphs_ColouredPerDesign() {
	s.buildView(http.NotFound)
	s.seedItems([]*models.MergeRequest{
		{IID: 1, Title: "open one", Author: models.User{Username: "a"}, State: models.MRStateOpened},
		{IID: 2, Title: "merged one", Author: models.User{Username: "b"}, State: models.MRStateMerged},
		{IID: 3, Title: "closed one", Author: models.User{Username: "c"}, State: models.MRStateClosed},
		{IID: 4, Title: "[WIP] draft one", Author: models.User{Username: "d"}, State: models.MRStateOpened},
	})

	pane, _ := s.g.View(keymap.ViewMRs)
	s.mrs.Render(pane)

	buf := pane.Buffer()
	// gocui re-emits SGR sequences with a trailing `;` — match the truecolor
	// RGB prefix rather than the full `\x1b[...m` byte form.
	s.Require().Contains(buf, "\x1b[38;2;74;168;90", "opened MR uses ok-coloured glyph")
	s.Require().Contains(buf, "●")
	s.Require().Contains(buf, "\x1b[38;2;138;92;200", "merged MR uses merged-coloured glyph")
	s.Require().Contains(buf, "✓")
	s.Require().Contains(buf, "\x1b[38;2;204;80;64", "closed MR uses err-coloured glyph")
	s.Require().Contains(buf, "✕")
	s.Require().Contains(buf, "\x1b[38;2;138;133;123", "WIP-titled MR uses draft-coloured glyph")
	s.Require().Contains(buf, "◐")
}

func (s *MRsViewSuite) TestRender_EmptyResults_ShowsHintWithLiveFilters() {
	s.buildView(http.NotFound)
	s.mrs.mu.Lock()
	s.mrs.current = &models.Project{ID: 1, PathWithNamespace: "grp/x"}
	s.mrs.loading = false
	s.mrs.stateFilter = models.MRStateFilterClosed
	s.mrs.ownerFilter = models.MROwnerFilterMine
	s.mrs.mu.Unlock()

	pane, _ := s.g.View(keymap.ViewMRs)
	s.mrs.Render(pane)

	buf := pane.Buffer()
	s.Require().Contains(buf, "No MRs match state:closed owner:mine")
	s.Require().Contains(buf, "S")
	s.Require().Contains(buf, "O")
	s.Require().Contains(buf, "R")
}

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestMRsViewSuite(t *testing.T) {
	suite.Run(t, new(MRsViewSuite))
}

//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	"github.com/niklod/lazylab/internal/tui"
	"github.com/niklod/lazylab/internal/tui/keymap"
	"github.com/niklod/lazylab/internal/tui/views"
)

const cacheRefreshCfgPath = "/cfg/config.yaml"

// projectsFixture keeps the repos pane seeded with a single project so the
// test can focus on MR list behaviour.
const cacheRefreshProjectsFixture = `[
	{"id":42,"name":"Alpha","path_with_namespace":"grp/alpha","web_url":"https://gl/grp/alpha","last_activity_at":"2026-04-10T10:00:00Z","archived":false}
]`

type CacheRefreshE2ESuite struct {
	suite.Suite
	srv             *httptest.Server
	fs              afero.Fs
	g               *gocui.Gui
	v               *views.Views
	app             *appcontext.AppContext
	manager         func(*gocui.Gui) error
	mrListBody      atomic.Value // string
	mrListCalls     atomic.Int32
	refreshFired    chan refreshFiredEvent
	dispatchedEvent atomic.Value // refreshFiredEvent
}

type refreshFiredEvent struct {
	Namespace string
	Key       string
}

func (s *CacheRefreshE2ESuite) SetupTest() {
	s.mrListBody.Store(`[]`)
	s.refreshFired = make(chan refreshFiredEvent, 4)

	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprint(w, cacheRefreshProjectsFixture)

			return
		case strings.Contains(r.URL.Path, "/merge_requests"):
			s.mrListCalls.Add(1)
			_, _ = fmt.Fprint(w, s.mrListBody.Load().(string))

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
	cfg.Cache.Directory = "/cfg/.cache"
	s.Require().NoError(cfg.Save(s.fs, cacheRefreshCfgPath))

	c := cache.New(cfg.Cache, s.fs)
	client, err := gitlab.New(cfg.GitLab,
		gitlab.WithHTTPClient(s.srv.Client()),
		gitlab.WithCache(c),
	)
	s.Require().NoError(err)
	s.app = appcontext.New(cfg, client, c, s.fs, cacheRefreshCfgPath)

	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 140, Height: 40})
	s.Require().NoError(err)
	s.g = g

	s.v = views.New(g, s.app)
	s.manager = tui.NewManager(s.v)
	g.SetManagerFunc(s.manager)
	s.Require().NoError(tui.Bind(g, s.v.Bindings()...))

	s.app.Cache.SetOnRefresh(func(_ context.Context, namespace, key string) {
		ev := refreshFiredEvent{Namespace: namespace, Key: key}
		s.dispatchedEvent.Store(ev)
		select {
		case s.refreshFired <- ev:
		default:
		}
	})

	s.Require().NoError(s.layoutTick())
}

func (s *CacheRefreshE2ESuite) TearDownTest() {
	if s.app != nil && s.app.Cache != nil {
		_ = s.app.Cache.Shutdown(context.Background())
	}
	if s.g != nil {
		s.g.Close()
	}
	if s.srv != nil {
		s.srv.Close()
	}
}

func (s *CacheRefreshE2ESuite) layoutTick() error { return s.manager(s.g) }

// seedStaleMRListCache writes a disk cache entry for the mr_list namespace
// with createdAt=0 so the in-process cache picks it up as immediately stale
// on the first Do call, triggering the background refresh path.
func (s *CacheRefreshE2ESuite) seedStaleMRListCache(projectID int, state string, mrs []*models.MergeRequest) {
	payload := map[string]any{
		"created_at": 0,
		"data":       mrs,
	}
	body, err := json.Marshal(payload)
	s.Require().NoError(err)

	key := fmt.Sprintf("mr_list_%d_%s", projectID, state)
	path := filepath.Join(s.app.Config.Cache.Directory, "api_"+key+".json")
	s.Require().NoError(s.fs.MkdirAll(s.app.Config.Cache.Directory, 0o700))
	s.Require().NoError(afero.WriteFile(s.fs, path, body, 0o600))
}

func (s *CacheRefreshE2ESuite) mrsBuffer() string {
	pane, err := s.g.View(keymap.ViewMRs)
	s.Require().NoError(err)

	return pane.Buffer()
}

// waitForRefreshEvent blocks for up to 1s waiting for the cache to emit
// the event. The test asserts fine-grained behaviour off the dispatched
// event, not real time.
func (s *CacheRefreshE2ESuite) waitForRefreshEvent() refreshFiredEvent {
	select {
	case ev := <-s.refreshFired:
		return ev
	case <-time.After(time.Second):
		s.T().Fatal("cache refresh event did not fire within 1s")

		return refreshFiredEvent{}
	}
}

func (s *CacheRefreshE2ESuite) TestStaleDiskCache_BackgroundRefreshFiresEvent() {
	staleMRs := []*models.MergeRequest{{
		ID: 1, IID: 1, Title: "Existing MR", State: models.MRStateOpened,
		Author: models.User{ID: 1, Username: "alice", Name: "A", WebURL: "u"},
		SourceBranch: "s", TargetBranch: "main", WebURL: "u",
	}}
	s.seedStaleMRListCache(42, "opened", staleMRs)

	cachePath := filepath.Join(s.app.Config.Cache.Directory, "api_mr_list_42_opened.json")
	exists, statErr := afero.Exists(s.fs, cachePath)
	s.Require().NoError(statErr)
	s.Require().True(exists, "stale disk cache file must exist at %s", cachePath)

	s.mrListBody.Store(`[
		{"id":1,"iid":1,"title":"Existing MR","state":"opened","author":{"id":1,"username":"alice","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"},
		{"id":2,"iid":2,"title":"Brand New MR","state":"opened","author":{"id":2,"username":"bob","name":"B","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}
	]`)

	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	initialBuf := s.mrsBuffer()
	s.Require().Contains(initialBuf, "Existing MR", "initial render serves stale disk entry")
	s.Require().NotContains(initialBuf, "Brand New MR", "new MR not yet visible before background refresh")

	ev := s.waitForRefreshEvent()
	s.Require().Equal("mr_list", ev.Namespace)
	s.Require().Equal(cache.MakeKey("mr_list", project.ID, "opened"), ev.Key)

	s.Require().NoError(s.v.MRs.ReloadFromCacheRefreshSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	updatedBuf := s.mrsBuffer()
	s.Require().Contains(updatedBuf, "Existing MR", "pre-existing MR still rendered post-refresh")
	s.Require().Contains(updatedBuf, "Brand New MR", "new MR visible after refresh-driven reload")
}

func (s *CacheRefreshE2ESuite) TestPipelineEvent_DoesNotDisturbMRsPane() {
	s.mrListBody.Store(`[
		{"id":1,"iid":1,"title":"Auth middleware","state":"opened","author":{"id":1,"username":"alice","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"},
		{"id":2,"iid":2,"title":"Bugfix","state":"opened","author":{"id":2,"username":"bob","name":"B","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}
	]`)

	s.Require().NoError(s.v.Repos.LoadSync(context.Background()))
	project := s.v.Repos.SelectedProject()
	s.Require().NotNil(project)
	s.Require().NoError(s.v.MRs.SetProjectSync(context.Background(), project))
	s.Require().NoError(s.layoutTick())

	// Establish UI state the refresh must preserve: submitted search filter
	// narrows the list to one MR.
	s.Require().NoError(s.dispatch(keymap.ViewMRs, '/'))
	s.Require().NoError(s.layoutTick())
	sv, err := s.g.View(keymap.ViewMRsSearch)
	s.Require().NoError(err)
	sv.Clear()
	_, err = sv.Write([]byte("auth"))
	s.Require().NoError(err)
	s.Require().NoError(s.dispatch(keymap.ViewMRsSearch, gocui.KeyEnter))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal("auth", s.v.MRs.Query())
	before := s.mrListCalls.Load()

	s.v.Dispatch(context.Background(), "mr_pipeline", cache.MakeKey("mr_pipeline", project.ID, 2))
	s.Require().NoError(s.layoutTick())

	s.Require().Equal("auth", s.v.MRs.Query(), "pipeline refresh must not clear MRs search query")
	s.Require().Equal(before, s.mrListCalls.Load(), "pipeline refresh must not cause an extra mr_list fetch")

	buf := s.mrsBuffer()
	s.Require().Contains(buf, "Auth middleware", "filter row still rendered after pipeline event")
	s.Require().NotContains(buf, "Bugfix", "filter still applied after pipeline event")
}

func (s *CacheRefreshE2ESuite) dispatch(view string, key any) error {
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

//nolint:paralleltest // gocui stores tcell simulation screen in a global; parallel runs race.
func TestCacheRefreshE2ESuite(t *testing.T) {
	suite.Run(t, new(CacheRefreshE2ESuite))
}

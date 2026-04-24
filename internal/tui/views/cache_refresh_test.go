package views

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

type CacheRefreshSuite struct {
	suite.Suite
	g       *gocui.Gui
	fs      afero.Fs
	srv     *httptest.Server
	app     *appcontext.AppContext
	cfgPath string

	projectListBody func() string
	mrListBody      func(projectID int, state string) string
}

func (s *CacheRefreshSuite) SetupTest() {
	g, err := gocui.NewGui(gocui.NewGuiOpts{Headless: true, Width: 120, Height: 40})
	s.Require().NoError(err)
	s.g = g

	s.fs = afero.NewMemMapFs()
	s.projectListBody = func() string { return "[]" }
	s.mrListBody = func(int, string) string { return "[]" }

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s.projectListBody()))
	})
	mux.HandleFunc("/api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v4/projects/"), "/")
		if len(parts) >= 2 && parts[1] == "merge_requests" {
			var pid int
			_, _ = fmt.Sscanf(parts[0], "%d", &pid)
			state := r.URL.Query().Get("state")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(s.mrListBody(pid, state)))

			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 1, "username": "me", "name": "Me"}`))
	})
	s.srv = httptest.NewServer(mux)

	cfg := config.Defaults()
	cfg.GitLab.URL = s.srv.URL
	cfg.GitLab.Token = testGitLabToken
	client, err := gitlab.New(cfg.GitLab)
	s.Require().NoError(err)
	c := cache.New(cfg.Cache, s.fs)
	s.cfgPath = "/cfg/config.yaml"
	s.Require().NoError(cfg.Save(s.fs, s.cfgPath))
	s.app = appcontext.New(cfg, client, c, s.fs, s.cfgPath)

	_, err = s.g.SetView(keymap.ViewRepos, 0, 0, 40, 20, 0)
	if err != nil && !strings.Contains(err.Error(), "unknown view") {
		s.T().Fatalf("SetView repos: %v", err)
	}
	_, err = s.g.SetView(keymap.ViewMRs, 0, 0, 40, 20, 0)
	if err != nil && !strings.Contains(err.Error(), "unknown view") {
		s.T().Fatalf("SetView mrs: %v", err)
	}
}

func (s *CacheRefreshSuite) TearDownTest() {
	if s.app != nil && s.app.Cache != nil {
		_ = s.app.Cache.Shutdown(context.Background())
	}
	if s.srv != nil {
		s.srv.Close()
	}
	if s.g != nil {
		s.g.Close()
	}
}

func projectsPayload(ps []*models.Project) string {
	type projectJSON struct {
		ID                int       `json:"id"`
		Name              string    `json:"name"`
		PathWithNamespace string    `json:"path_with_namespace"`
		LastActivityAt    time.Time `json:"last_activity_at"`
	}
	out := make([]projectJSON, 0, len(ps))
	for _, p := range ps {
		out = append(out, projectJSON{p.ID, p.Name, p.PathWithNamespace, p.LastActivityAt})
	}
	b, _ := json.Marshal(out)

	return string(b)
}

func mrsPayload(mrs []*models.MergeRequest) string {
	type mrJSON struct {
		ID     int    `json:"id"`
		IID    int    `json:"iid"`
		Title  string `json:"title"`
		State  string `json:"state"`
		Author struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"author"`
	}
	out := make([]mrJSON, 0, len(mrs))
	for _, m := range mrs {
		j := mrJSON{ID: m.ID, IID: m.IID, Title: m.Title, State: string(m.State)}
		j.Author.ID = m.Author.ID
		j.Author.Username = m.Author.Username
		j.Author.Name = m.Author.Name
		out = append(out, j)
	}
	b, _ := json.Marshal(out)

	return string(b)
}

func (s *CacheRefreshSuite) TestReposOnCacheRefresh_PreservesCursorByID() {
	initial := []*models.Project{
		{ID: 10, PathWithNamespace: "g/a", LastActivityAt: time.Now().Add(-time.Hour)},
		{ID: 20, PathWithNamespace: "g/b", LastActivityAt: time.Now().Add(-2 * time.Hour)},
		{ID: 30, PathWithNamespace: "g/c", LastActivityAt: time.Now().Add(-3 * time.Hour)},
	}
	s.projectListBody = func() string { return projectsPayload(initial) }

	v := NewRepos(s.g, s.app)
	s.Require().NoError(v.LoadSync(context.Background()))

	v.mu.Lock()
	v.cursor = 1 // cursor on ID 20
	v.mu.Unlock()

	expanded := []*models.Project{
		{ID: 10, PathWithNamespace: "g/a", LastActivityAt: time.Now().Add(-time.Hour)},
		{ID: 40, PathWithNamespace: "g/inserted", LastActivityAt: time.Now().Add(-30 * time.Minute)},
		{ID: 20, PathWithNamespace: "g/b", LastActivityAt: time.Now().Add(-2 * time.Hour)},
		{ID: 30, PathWithNamespace: "g/c", LastActivityAt: time.Now().Add(-3 * time.Hour)},
	}
	s.projectListBody = func() string { return projectsPayload(expanded) }

	s.Require().NoError(v.LoadSync(context.Background()))

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Len(v.filtered, 4)
	s.Require().Equal(20, v.filtered[v.cursor].ID, "cursor preserved on project with ID 20")
}

func (s *CacheRefreshSuite) TestReposOnCacheRefresh_SelectedProjectRemovedClampsCursor() {
	initial := []*models.Project{
		{ID: 10, PathWithNamespace: "g/a"},
		{ID: 20, PathWithNamespace: "g/b"},
		{ID: 30, PathWithNamespace: "g/c"},
	}
	s.projectListBody = func() string { return projectsPayload(initial) }

	v := NewRepos(s.g, s.app)
	s.Require().NoError(v.LoadSync(context.Background()))

	v.mu.Lock()
	v.cursor = 1 // cursor on ID 20
	v.mu.Unlock()

	shrunk := []*models.Project{
		{ID: 10, PathWithNamespace: "g/a"},
		{ID: 30, PathWithNamespace: "g/c"},
	}
	s.projectListBody = func() string { return projectsPayload(shrunk) }

	s.Require().NoError(v.LoadSync(context.Background()))

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Len(v.filtered, 2)
	s.Require().GreaterOrEqual(v.cursor, 0)
	s.Require().Less(v.cursor, 2, "cursor clamped into new bounds")
}

func (s *CacheRefreshSuite) TestReposOnCacheRefresh_IgnoresUnrelatedNamespace() {
	initial := []*models.Project{{ID: 10, PathWithNamespace: "g/a"}}
	s.projectListBody = func() string { return projectsPayload(initial) }

	v := NewRepos(s.g, s.app)
	s.Require().NoError(v.LoadSync(context.Background()))

	v.mu.Lock()
	v.query = "keep-me"
	v.cursor = 0
	v.mu.Unlock()

	v.OnCacheRefresh(context.Background(), "mr_list", "mr_list:10:opened")

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Equal("keep-me", v.query, "unrelated namespace must not clear repos search query")
}

func (s *CacheRefreshSuite) TestMRsOnCacheRefresh_PreservesCursorByIID() {
	project := &models.Project{ID: 10, PathWithNamespace: "g/a"}

	initial := []*models.MergeRequest{
		{ID: 100, IID: 1, Title: "first", State: models.MRStateOpened},
		{ID: 200, IID: 2, Title: "second", State: models.MRStateOpened},
	}
	s.mrListBody = func(int, string) string { return mrsPayload(initial) }

	v := NewMRs(s.g, s.app)
	s.Require().NoError(v.SetProjectSync(context.Background(), project))

	v.mu.Lock()
	v.cursor = 1 // cursor on IID 2
	v.mu.Unlock()

	expanded := []*models.MergeRequest{
		{ID: 300, IID: 3, Title: "new-on-top", State: models.MRStateOpened},
		{ID: 100, IID: 1, Title: "first", State: models.MRStateOpened},
		{ID: 200, IID: 2, Title: "second", State: models.MRStateOpened},
	}
	s.mrListBody = func(int, string) string { return mrsPayload(expanded) }

	s.app.Cache.Invalidate(cache.MakeKey("mr_list", project.ID) + ":")
	s.Require().NoError(v.ReloadFromCacheRefreshSync(context.Background(), project))

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Len(v.filtered, 3)
	s.Require().Equal(2, v.filtered[v.cursor].IID, "cursor preserved on MR with IID 2 after insertion")
}

func (s *CacheRefreshSuite) TestMRsOnCacheRefresh_SearchQueryPreserved() {
	project := &models.Project{ID: 10, PathWithNamespace: "g/a"}

	initial := []*models.MergeRequest{
		{ID: 100, IID: 1, Title: "authfix", State: models.MRStateOpened},
		{ID: 200, IID: 2, Title: "other", State: models.MRStateOpened},
	}
	s.mrListBody = func(int, string) string { return mrsPayload(initial) }

	v := NewMRs(s.g, s.app)
	s.Require().NoError(v.SetProjectSync(context.Background(), project))

	v.mu.Lock()
	v.query = "auth"
	v.searchActive = true
	v.applyFilterLocked()
	v.mu.Unlock()

	expanded := []*models.MergeRequest{
		{ID: 100, IID: 1, Title: "authfix", State: models.MRStateOpened},
		{ID: 200, IID: 2, Title: "other", State: models.MRStateOpened},
		{ID: 300, IID: 3, Title: "authcheck", State: models.MRStateOpened},
	}
	s.mrListBody = func(int, string) string { return mrsPayload(expanded) }

	s.app.Cache.Invalidate(cache.MakeKey("mr_list", project.ID) + ":")
	s.Require().NoError(v.ReloadFromCacheRefreshSync(context.Background(), project))

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Equal("auth", v.query, "search query survives refresh")
	s.Require().True(v.searchActive, "searchActive flag untouched")
	titles := make([]string, 0, len(v.filtered))
	for _, m := range v.filtered {
		titles = append(titles, m.Title)
	}
	s.Require().ElementsMatch([]string{"authfix", "authcheck"}, titles, "filter applied to refreshed list")
}

func (s *CacheRefreshSuite) TestMRsOnCacheRefresh_WrongProjectIgnored() {
	project := &models.Project{ID: 10, PathWithNamespace: "g/a"}

	initial := []*models.MergeRequest{{ID: 100, IID: 1, Title: "first", State: models.MRStateOpened}}
	s.mrListBody = func(int, string) string { return mrsPayload(initial) }

	v := NewMRs(s.g, s.app)
	s.Require().NoError(v.SetProjectSync(context.Background(), project))

	beforeSeq := v.loadSeq
	v.OnCacheRefresh(context.Background(), "mr_list", cache.MakeKey("mr_list", 20, "opened"))

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Equal(beforeSeq, v.loadSeq, "event for project 20 must not bump load seq on project-10 pane")
}

func (s *CacheRefreshSuite) TestMRsOnCacheRefresh_UnrelatedNamespaceIgnored() {
	project := &models.Project{ID: 10}

	initial := []*models.MergeRequest{{ID: 100, IID: 1, Title: "first", State: models.MRStateOpened}}
	s.mrListBody = func(int, string) string { return mrsPayload(initial) }

	v := NewMRs(s.g, s.app)
	s.Require().NoError(v.SetProjectSync(context.Background(), project))

	beforeSeq := v.loadSeq
	v.OnCacheRefresh(context.Background(), "mr_pipeline", cache.MakeKey("mr_pipeline", 10, 1))

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Equal(beforeSeq, v.loadSeq, "mr_pipeline event must not trigger MRs reload")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_WrongMRIgnored() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{ID: 100, IID: 1, Title: "first"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	beforeSeq := d.statsSeq

	d.OnCacheRefresh(context.Background(), "mr_discussions", cache.MakeKey("mr_discussions", 10, 99))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().Equal(beforeSeq, d.statsSeq, "event for MR 99 must not bump statsSeq when MR 1 is showing")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_NoProjectIgnored() {
	d := NewDetail(s.g, s.app)
	d.project = nil
	d.mr = nil

	d.OnCacheRefresh(context.Background(), "mr_discussions", cache.MakeKey("mr_discussions", 10, 1))
	// No panic, no state change expected.
}

// Detail sub-panes must NOT wipe to a "Loading…" state on a refresh event
// for the MR they currently show — that was the flicker ADR 009 feared and
// ADR 021's "no content shift" rule forbids. The silent-refetch helpers
// bump the seq and fetch, but do not toggle diffLoading / pipelineLoading
// etc. beforehand.
func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_DiffRefetchDoesNotWipeContent() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{IID: 1, Title: "t"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	d.diffLoading = false

	d.OnCacheRefresh(context.Background(), "mr_changes", cache.MakeKey("mr_changes", 10, 1))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().False(d.diffLoading, "silent diff refresh must not flip diffLoading to true")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_PipelineRefetchDoesNotWipeContent() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{IID: 1, Title: "t"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	d.pipelineLoading = false

	d.OnCacheRefresh(context.Background(), "mr_pipeline", cache.MakeKey("mr_pipeline", 10, 1))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().False(d.pipelineLoading, "silent pipeline refresh must not flip pipelineLoading to true")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_ConversationRefetchBumpsSeq() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{IID: 1, Title: "t"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	beforeSeq := d.conversationSeq

	d.OnCacheRefresh(context.Background(), "mr_conversation", cache.MakeKey("mr_conversation", 10, 1))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().Equal(beforeSeq+1, d.conversationSeq, "conversation refresh must bump conversationSeq")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_OverviewApprovalsBumpsApprovalsSeqOnly() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{IID: 1, Title: "t"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	beforeApprovals := d.approvalsSeq
	beforeStats := d.statsSeq

	d.OnCacheRefresh(context.Background(), "mr_approvals", cache.MakeKey("mr_approvals", 10, 1))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().Equal(beforeApprovals+1, d.approvalsSeq, "mr_approvals must bump approvalsSeq")
	s.Require().Equal(beforeStats, d.statsSeq, "mr_approvals must NOT bump statsSeq")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_JobTraceLogClosedDoesNotRefetch() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{IID: 1, Title: "t"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	d.logOpen = false
	d.logJob = nil
	beforeSeq := d.logSeq

	d.OnCacheRefresh(context.Background(), "job_trace", cache.MakeKey("job_trace", 10, 5))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().Equal(beforeSeq, d.logSeq, "job_trace event with log closed must not bump logSeq")
}

func (s *CacheRefreshSuite) TestDetailOnCacheRefresh_JobTraceWrongJobIgnored() {
	project := &models.Project{ID: 10}
	mr := &models.MergeRequest{IID: 1, Title: "t"}

	d := NewDetail(s.g, s.app)
	d.project = project
	d.mr = mr
	d.logOpen = true
	d.logJob = &models.PipelineJob{ID: 5}
	beforeSeq := d.logSeq

	d.OnCacheRefresh(context.Background(), "job_trace", cache.MakeKey("job_trace", 10, 99))

	d.mu.Lock()
	defer d.mu.Unlock()
	s.Require().Equal(beforeSeq, d.logSeq, "job_trace for unrelated job must not bump logSeq")
}

func (s *CacheRefreshSuite) TestMRsApplySilentReload_LoaderErrorPreservesStaleRows() {
	project := &models.Project{ID: 10, PathWithNamespace: "g/a"}
	initial := []*models.MergeRequest{
		{ID: 100, IID: 1, Title: "keep-me", State: models.MRStateOpened},
	}
	s.mrListBody = func(int, string) string { return mrsPayload(initial) }

	v := NewMRs(s.g, s.app)
	s.Require().NoError(v.SetProjectSync(context.Background(), project))

	s.mrListBody = func(int, string) string { return "not-valid-json" }

	err := v.ReloadFromCacheRefreshSync(context.Background(), project)
	s.Require().Error(err)

	v.mu.Lock()
	defer v.mu.Unlock()
	s.Require().Len(v.all, 1, "stale rows preserved across a failed silent reload")
	s.Require().Equal("keep-me", v.all[0].Title)
	s.Require().NoError(v.loadErr, "transient loader error in silent path must not surface as loadErr")
}

func (s *CacheRefreshSuite) TestViewsDispatch_PipelineEventOnlyRoutesToDetail() {
	v := New(s.g, s.app)
	project := &models.Project{ID: 10}
	v.Repos.mu.Lock()
	v.Repos.query = "keep"
	v.Repos.mu.Unlock()
	v.MRs.mu.Lock()
	v.MRs.current = project
	v.MRs.query = "keep-mrs"
	v.MRs.searchActive = true
	beforeMRsSeq := v.MRs.loadSeq
	v.MRs.mu.Unlock()

	v.Dispatch(context.Background(), "mr_pipeline", cache.MakeKey("mr_pipeline", 10, 1))

	v.Repos.mu.Lock()
	s.Require().Equal("keep", v.Repos.query, "pipeline event must not touch repos query")
	v.Repos.mu.Unlock()
	v.MRs.mu.Lock()
	s.Require().Equal("keep-mrs", v.MRs.query, "pipeline event must not touch MRs query")
	s.Require().Equal(beforeMRsSeq, v.MRs.loadSeq, "pipeline event must not bump MRs load seq")
	v.MRs.mu.Unlock()
}

//nolint:paralleltest // gocui uses a global simulation screen; parallel suites race.
func TestCacheRefreshSuite(t *testing.T) {
	suite.Run(t, new(CacheRefreshSuite))
}

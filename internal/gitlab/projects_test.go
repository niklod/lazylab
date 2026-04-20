package gitlab_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
)

type ProjectsSuite struct {
	suite.Suite
}

func (s *ProjectsSuite) TestListProjects_PaginatesAndMapsDomainFields() {
	page1 := `[
		{"id":1,"name":"Alpha","path_with_namespace":"grp/alpha","default_branch":"main","web_url":"https://gl/grp/alpha","last_activity_at":"2026-03-01T10:00:00Z","archived":false},
		{"id":2,"name":"Bravo","path_with_namespace":"grp/bravo","web_url":"https://gl/grp/bravo","archived":true}
	]`
	page2 := `[
		{"id":3,"name":"Charlie","path_with_namespace":"grp/charlie","web_url":"https://gl/grp/charlie","last_activity_at":"2026-02-14T08:30:00Z"}
	]`

	var (
		gotPaths    []string
		gotQueries  []url.Values
		fetchedPage atomic.Int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		gotQueries = append(gotQueries, r.URL.Query())

		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		fetchedPage.Add(1)

		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			w.Header().Set("X-Next-Page", "2")
			w.Header().Set("X-Page", "1")
			_, _ = w.Write([]byte(page1))
		case "2":
			w.Header().Set("X-Next-Page", "")
			w.Header().Set("X-Page", "2")
			_, _ = w.Write([]byte(page2))
		default:
			s.T().Fatalf("unexpected page %q", page)
		}
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	includeArchived := true
	projects, err := client.ListProjects(context.Background(), gitlab.ListProjectsOptions{Archived: &includeArchived})

	s.Require().NoError(err)
	s.Require().Len(projects, 3, "Archived=true keeps the archived project in the list")

	s.Require().Equal(1, projects[0].ID)
	s.Require().Equal("grp/alpha", projects[0].PathWithNamespace)
	s.Require().Equal("main", projects[0].DefaultBranch)
	s.Require().False(projects[0].Archived)
	s.Require().Equal(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), projects[0].LastActivityAt)

	s.Require().Equal(2, projects[1].ID)
	s.Require().True(projects[1].Archived)
	s.Require().True(projects[1].LastActivityAt.IsZero(), "missing last_activity_at maps to zero time")

	s.Require().Equal(3, projects[2].ID)

	s.Require().GreaterOrEqual(fetchedPage.Load(), int32(2), "both pages fetched")
	for _, p := range gotPaths {
		s.Require().Contains(p, "/projects")
	}
	firstQ := gotQueries[0]
	s.Require().Equal("true", firstQ.Get("membership"))
	s.Require().Equal("true", firstQ.Get("archived"))
	s.Require().Equal("last_activity_at", firstQ.Get("order_by"))
	s.Require().Equal("desc", firstQ.Get("sort"))
	s.Require().Equal(strconv.Itoa(100), firstQ.Get("per_page"))
}

func (s *ProjectsSuite) TestListProjects_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"message":"401 Unauthorized"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.ListProjects(context.Background(), gitlab.ListProjectsOptions{})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: list projects")
}

func (s *ProjectsSuite) TestListProjects_PropagatesContextCancellation() {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(block)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.ListProjects(ctx, gitlab.ListProjectsOptions{})

	s.Require().Error(err)
}

func (s *ProjectsSuite) TestListProjects_CachedClient_ReusesResultOnSecondCall() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":42,"name":"Only","path_with_namespace":"grp/only","web_url":"https://x","last_activity_at":"2026-04-01T10:00:00Z"}]`)
	}))
	s.T().Cleanup(srv.Close)

	fs := afero.NewMemMapFs()
	cacheCfg := config.CacheConfig{Directory: "/cache", TTL: 600}
	c := cache.New(cacheCfg, fs)
	s.T().Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Shutdown(shutdownCtx)
	})

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
		gitlab.WithCache(c),
	)
	s.Require().NoError(err)

	first, err := client.ListProjects(context.Background(), gitlab.ListProjectsOptions{})
	s.Require().NoError(err)
	s.Require().Len(first, 1)
	s.Require().Equal(int32(1), hits.Load())

	second, err := client.ListProjects(context.Background(), gitlab.ListProjectsOptions{})
	s.Require().NoError(err)
	s.Require().Len(second, 1)
	s.Require().Equal(int32(1), hits.Load(), "cached ListProjects skips HTTP on fresh entry")
}

func (s *ProjectsSuite) TestListProjects_DropsArchivedEvenIfUpstreamReturnsThem() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Upstream ignored archived=false and returned a mixed page.
		_, _ = fmt.Fprint(w, `[
			{"id":1,"name":"live","path_with_namespace":"grp/live","archived":false,"web_url":"u","last_activity_at":"2026-04-10T10:00:00Z"},
			{"id":2,"name":"dead","path_with_namespace":"grp/dead","archived":true,"web_url":"u","last_activity_at":"2024-01-01T10:00:00Z"},
			{"id":3,"name":"alsoLive","path_with_namespace":"grp/alsoLive","archived":false,"web_url":"u","last_activity_at":"2026-04-11T10:00:00Z"}
		]`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	projects, err := client.ListProjects(context.Background(), gitlab.ListProjectsOptions{})
	s.Require().NoError(err)
	s.Require().Len(projects, 2, "archived project filtered out")
	for _, p := range projects {
		s.Require().False(p.Archived, "archived project slipped through: %s", p.PathWithNamespace)
	}
}

func (s *ProjectsSuite) TestListProjects_KeepsArchivedWhenExplicitlyRequested() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"id":1,"name":"dead","path_with_namespace":"grp/dead","archived":true,"web_url":"u","last_activity_at":"2024-01-01T10:00:00Z"}
		]`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	archived := true
	projects, err := client.ListProjects(context.Background(), gitlab.ListProjectsOptions{Archived: &archived})
	s.Require().NoError(err)
	s.Require().Len(projects, 1, "Archived=true keeps archived projects")
	s.Require().True(projects[0].Archived)
}

func TestProjectsSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ProjectsSuite))
}

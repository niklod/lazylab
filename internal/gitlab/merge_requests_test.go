package gitlab_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
)

type MergeRequestsSuite struct {
	suite.Suite
}

func (s *MergeRequestsSuite) TestListMergeRequests_PaginatesAndMapsFields() {
	page1 := `[
		{"id":101,"iid":1,"title":"Feature","state":"opened","author":{"id":10,"username":"alice","name":"Alice","web_url":"https://gl/u/alice"},"source_branch":"feat","target_branch":"main","web_url":"https://gl/mr/1","created_at":"2026-04-10T10:00:00Z","updated_at":"2026-04-11T10:00:00Z","user_notes_count":3,"has_conflicts":false,"detailed_merge_status":"mergeable"},
		{"id":102,"iid":2,"title":"Bugfix","state":"merged","author":{"id":11,"username":"bob","name":"Bob","web_url":"https://gl/u/bob"},"source_branch":"bug","target_branch":"main","web_url":"https://gl/mr/2","created_at":"2026-04-01T10:00:00Z","updated_at":"2026-04-05T10:00:00Z","merged_at":"2026-04-05T12:00:00Z","user_notes_count":0}
	]`
	page2 := `[
		{"id":103,"iid":3,"title":"Docs","state":"opened","author":{"id":12,"username":"carol","name":"Carol","web_url":"https://gl/u/carol"},"source_branch":"docs","target_branch":"main","web_url":"https://gl/mr/3","created_at":"2026-03-20T10:00:00Z","updated_at":"2026-03-21T10:00:00Z"}
	]`

	var (
		paths   []string
		queries []url.Values
		fetched atomic.Int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		queries = append(queries, r.URL.Query())
		fetched.Add(1)

		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(page1))
		case "2":
			w.Header().Set("X-Next-Page", "")
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

	mrs, err := client.ListMergeRequests(context.Background(), gitlab.ListMergeRequestsOptions{
		ProjectID:   42,
		ProjectPath: "grp/alpha",
		State:       models.MRStateFilterOpened,
	})

	s.Require().NoError(err)
	s.Require().Len(mrs, 3)

	s.Require().Equal(1, mrs[0].IID)
	s.Require().Equal("Feature", mrs[0].Title)
	s.Require().Equal(models.MRStateOpened, mrs[0].State)
	s.Require().Equal("alice", mrs[0].Author.Username)
	s.Require().Equal("grp/alpha", mrs[0].ProjectPath)
	s.Require().Equal("mergeable", mrs[0].MergeStatus)
	s.Require().Equal(3, mrs[0].UserNotesCount)
	s.Require().Equal(time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC), mrs[0].UpdatedAt)

	s.Require().Equal(models.MRStateMerged, mrs[1].State)
	s.Require().NotNil(mrs[1].MergedAt)
	s.Require().Equal(time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC), *mrs[1].MergedAt)

	s.Require().Nil(mrs[2].MergedAt)
	s.Require().GreaterOrEqual(fetched.Load(), int32(2), "both pages fetched")

	for _, p := range paths {
		s.Require().Contains(p, "/projects/42/merge_requests")
	}

	firstQ := queries[0]
	s.Require().Equal("opened", firstQ.Get("state"))
	s.Require().Equal("updated_at", firstQ.Get("order_by"))
	s.Require().Equal("desc", firstQ.Get("sort"))
}

func (s *MergeRequestsSuite) TestListMergeRequests_AuthorAndReviewerFilters() {
	var (
		gotQueries []url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueries = append(gotQueries, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	authorID := 7
	reviewerID := 9
	_, err = client.ListMergeRequests(context.Background(), gitlab.ListMergeRequestsOptions{
		ProjectID:   1,
		ProjectPath: "grp/alpha",
		State:       models.MRStateFilterAll,
		AuthorID:    &authorID,
		ReviewerID:  &reviewerID,
	})

	s.Require().NoError(err)
	s.Require().NotEmpty(gotQueries)
	q := gotQueries[0]
	s.Require().Equal("all", q.Get("state"))
	s.Require().Equal("7", q.Get("author_id"))
	s.Require().Equal("9", q.Get("reviewer_id"))
}

func (s *MergeRequestsSuite) TestListMergeRequests_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"500"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.ListMergeRequests(context.Background(), gitlab.ListMergeRequestsOptions{
		ProjectID:   1,
		ProjectPath: "grp/alpha",
	})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: list merge requests")
}

func (s *MergeRequestsSuite) TestListMergeRequests_RejectsZeroProjectID() {
	client, err := gitlab.New(config.GitLabConfig{URL: "https://gl", Token: "secret"})
	s.Require().NoError(err)

	_, err = client.ListMergeRequests(context.Background(), gitlab.ListMergeRequestsOptions{})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "project id required")
}

func (s *MergeRequestsSuite) TestListMergeRequests_CachedReusesOnSecondCall() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":1,"iid":1,"title":"X","state":"opened","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}]`)
	}))
	s.T().Cleanup(srv.Close)

	fs := afero.NewMemMapFs()
	c := cache.New(config.CacheConfig{Directory: "/cache", TTL: 600}, fs)
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

	opts := gitlab.ListMergeRequestsOptions{ProjectID: 1, ProjectPath: "grp/x", State: models.MRStateFilterOpened}

	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), hits.Load())

	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), hits.Load(), "second fresh call skips HTTP")
}

// TestListMergeRequests_CachedAuthorVsReviewerDoNotCollide guards against the
// Python-parity nil-skipping bug where AuthorID=7 (ReviewerID=nil) and
// ReviewerID=7 (AuthorID=nil) produced the same cache key.
func (s *MergeRequestsSuite) TestListMergeRequests_CachedAuthorVsReviewerDoNotCollide() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	s.T().Cleanup(srv.Close)

	fs := afero.NewMemMapFs()
	c := cache.New(config.CacheConfig{Directory: "/cache", TTL: 600}, fs)
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

	userID := 77
	base := gitlab.ListMergeRequestsOptions{ProjectID: 1, ProjectPath: "grp/x", State: models.MRStateFilterOpened}

	mineOpts := base
	mineOpts.AuthorID = &userID
	_, err = client.ListMergeRequests(context.Background(), mineOpts)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), hits.Load())

	reviewerOpts := base
	reviewerOpts.ReviewerID = &userID
	_, err = client.ListMergeRequests(context.Background(), reviewerOpts)
	s.Require().NoError(err)
	s.Require().Equal(int32(2), hits.Load(), "author vs reviewer with the same id must have distinct cache keys")
}

func (s *MergeRequestsSuite) TestGetMergeRequest_MapsFields() {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":500,"iid":7,"title":"Full","state":"opened","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u","has_conflicts":true,"user_notes_count":4,"detailed_merge_status":"checking"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	mr, err := client.GetMergeRequest(context.Background(), 42, 7, "grp/alpha")

	s.Require().NoError(err)
	s.Require().NotNil(mr)
	s.Require().Equal(7, mr.IID)
	s.Require().Equal("Full", mr.Title)
	s.Require().True(mr.HasConflicts)
	s.Require().Equal(4, mr.UserNotesCount)
	s.Require().Equal("grp/alpha", mr.ProjectPath)
	s.Require().True(strings.HasSuffix(path, "/projects/42/merge_requests/7"))
}

func (s *MergeRequestsSuite) TestGetMergeRequest_RejectsZeroIIDs() {
	client, err := gitlab.New(config.GitLabConfig{URL: "https://gl", Token: "secret"})
	s.Require().NoError(err)

	_, err = client.GetMergeRequest(context.Background(), 0, 7, "grp/x")
	s.Require().Error(err)

	_, err = client.GetMergeRequest(context.Background(), 1, 0, "grp/x")
	s.Require().Error(err)
}

func (s *MergeRequestsSuite) TestGetMRApprovals_MapsApprovedByUsers() {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"approved":false,"approvals_required":2,"approvals_left":1,"approved_by":[{"user":{"id":10,"username":"alice","name":"Alice","web_url":"u"}}]}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	approval, err := client.GetMRApprovals(context.Background(), 42, 7)

	s.Require().NoError(err)
	s.Require().Contains(gotPath, "/projects/42/merge_requests/7/approvals")
	s.Require().NotNil(approval)
	s.Require().False(approval.Approved)
	s.Require().Equal(2, approval.ApprovalsRequired)
	s.Require().Equal(1, approval.ApprovalsLeft)
	s.Require().Len(approval.ApprovedBy, 1)
	s.Require().Equal("alice", approval.ApprovedBy[0].Username)
}

func (s *MergeRequestsSuite) TestGetMRApprovals_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"message":"forbidden"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.GetMRApprovals(context.Background(), 42, 7)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: get mr approvals")
}

func (s *MergeRequestsSuite) TestGetMRDiscussionStats_AggregatesResolvableAndResolved() {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[
			{"id":"d1","notes":[{"id":1,"resolvable":true,"resolved":true}]},
			{"id":"d2","notes":[{"id":2,"resolvable":true,"resolved":false}]},
			{"id":"d3","notes":[{"id":3,"resolvable":true,"resolved":true},{"id":4,"resolvable":true,"resolved":true}]},
			{"id":"d4","notes":[{"id":5,"resolvable":true,"resolved":true},{"id":6,"resolvable":true,"resolved":false}]},
			{"id":"d5","notes":[{"id":7,"resolvable":false,"resolved":false}]},
			{"id":"d6","notes":[]}
		]`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	stats, err := client.GetMRDiscussionStats(context.Background(), 42, 7)

	s.Require().NoError(err)
	s.Require().Contains(gotPath, "/projects/42/merge_requests/7/discussions")
	s.Require().Equal(4, stats.TotalResolvable)
	s.Require().Equal(2, stats.Resolved, "two discussions are fully resolved; one mixed-partial counts as unresolved")
}

func (s *MergeRequestsSuite) TestGetMRDiscussionStats_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"500"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.GetMRDiscussionStats(context.Background(), 42, 7)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: list mr discussions")
}

func (s *MergeRequestsSuite) TestGetMRDiscussionStats_ValidatesInputs() {
	client, err := gitlab.New(
		config.GitLabConfig{URL: "https://gitlab.example", Token: "secret"},
	)
	s.Require().NoError(err)

	_, err = client.GetMRDiscussionStats(context.Background(), 0, 7)
	s.Require().Error(err)

	_, err = client.GetMRDiscussionStats(context.Background(), 1, 0)
	s.Require().Error(err)
}

func (s *MergeRequestsSuite) TestGetMRChanges_PaginatesAndMapsFiles() {
	page1 := `[
		{"old_path":"src/a.go","new_path":"src/a.go","diff":"@@ -1 +1 @@\n-a\n+b\n","new_file":false,"renamed_file":false,"deleted_file":false},
		{"old_path":"src/b.go","new_path":"src/b_renamed.go","diff":"","new_file":false,"renamed_file":true,"deleted_file":false}
	]`
	page2 := `[
		{"old_path":"","new_path":"src/c.go","diff":"@@ -0,0 +1 @@\n+new\n","new_file":true,"renamed_file":false,"deleted_file":false}
	]`

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(page1))
		case "2":
			w.Header().Set("X-Next-Page", "")
			_, _ = w.Write([]byte(page2))
		default:
			s.T().Fatalf("unexpected page %q", r.URL.Query().Get("page"))
		}
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	data, err := client.GetMRChanges(context.Background(), 42, 7)

	s.Require().NoError(err)
	s.Require().NotNil(data)
	s.Require().Len(data.Files, 3)

	s.Require().Equal("src/a.go", data.Files[0].NewPath)
	s.Require().Contains(data.Files[0].Diff, "+b")

	s.Require().True(data.Files[1].RenamedFile)
	s.Require().Equal("src/b_renamed.go", data.Files[1].NewPath)

	s.Require().True(data.Files[2].NewFile)
	s.Require().Equal("src/c.go", data.Files[2].NewPath)

	for _, p := range paths {
		s.Require().Contains(p, "/projects/42/merge_requests/7/diffs")
	}
}

func (s *MergeRequestsSuite) TestGetMRChanges_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"500"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.GetMRChanges(context.Background(), 42, 7)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: list mr diffs")
}

func (s *MergeRequestsSuite) TestGetMRChanges_ValidatesInputs() {
	client, err := gitlab.New(config.GitLabConfig{URL: "https://gl", Token: "secret"})
	s.Require().NoError(err)

	_, err = client.GetMRChanges(context.Background(), 0, 7)
	s.Require().Error(err)

	_, err = client.GetMRChanges(context.Background(), 1, 0)
	s.Require().Error(err)
}

func (s *MergeRequestsSuite) TestGetMRChanges_CachedReusesOnSecondCall() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"old_path":"a","new_path":"a","diff":""}]`)
	}))
	s.T().Cleanup(srv.Close)

	fs := afero.NewMemMapFs()
	c := cache.New(config.CacheConfig{Directory: "/cache", TTL: 600}, fs)
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

	_, err = client.GetMRChanges(context.Background(), 1, 2)
	s.Require().NoError(err)
	_, err = client.GetMRChanges(context.Background(), 1, 2)
	s.Require().NoError(err)

	s.Require().Equal(int32(1), hits.Load(), "second fresh call skips HTTP")
}

func (s *MergeRequestsSuite) TestGetCurrentUser_MapsFields() {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":99,"username":"me","name":"Me","web_url":"https://gl/u/me","avatar_url":"https://gl/a"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	u, err := client.GetCurrentUser(context.Background())

	s.Require().NoError(err)
	s.Require().Contains(gotPath, "/user")
	s.Require().NotNil(u)
	s.Require().Equal(99, u.ID)
	s.Require().Equal("me", u.Username)
	s.Require().Equal("https://gl/a", u.AvatarURL)
}

func (s *MergeRequestsSuite) TestGetCurrentUser_CachedReusesOnSecondCall() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"username":"me","name":"Me","web_url":"u"}`)
	}))
	s.T().Cleanup(srv.Close)

	fs := afero.NewMemMapFs()
	c := cache.New(config.CacheConfig{Directory: "/cache", TTL: 600}, fs)
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

	_, err = client.GetCurrentUser(context.Background())
	s.Require().NoError(err)
	_, err = client.GetCurrentUser(context.Background())
	s.Require().NoError(err)

	s.Require().Equal(int32(1), hits.Load(), "current user cached")
}

func TestMergeRequestsSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(MergeRequestsSuite))
}

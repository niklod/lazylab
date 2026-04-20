package gitlab_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func decodeJSONBody(t interface{ Fatalf(string, ...any) }, r *http.Request) map[string]any {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal body %q: %v", string(body), err)
	}

	return out
}

type MRActionsSuite struct {
	suite.Suite
}

func (s *MRActionsSuite) TestCloseMergeRequest_SendsStateEventAndMapsResponse() {
	var (
		method string
		path   string
		body   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		body = decodeJSONBody(s.T(), r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":101,"iid":7,"title":"Closed","state":"closed","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"x","target_branch":"main","web_url":"u"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	mr, err := client.CloseMergeRequest(context.Background(), 42, 7, "grp/alpha")

	s.Require().NoError(err)
	s.Require().Equal(http.MethodPut, method)
	s.Require().Contains(path, "/projects/42/merge_requests/7")
	s.Require().Equal("close", body["state_event"])
	s.Require().NotNil(mr)
	s.Require().Equal(models.MRStateClosed, mr.State)
	s.Require().Equal(7, mr.IID)
	s.Require().Equal("grp/alpha", mr.ProjectPath)
}

func (s *MRActionsSuite) TestCloseMergeRequest_WrapsUpstreamError() {
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

	_, err = client.CloseMergeRequest(context.Background(), 1, 2, "grp/x")

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: close merge request 1!2")
}

func (s *MRActionsSuite) TestCloseMergeRequest_RejectsZeroIDs() {
	client, err := gitlab.New(config.GitLabConfig{URL: "https://gl", Token: "secret"})
	s.Require().NoError(err)

	_, err = client.CloseMergeRequest(context.Background(), 0, 1, "grp/x")
	s.Require().Error(err)
	s.Require().ErrorContains(err, "project id and iid required")

	_, err = client.CloseMergeRequest(context.Background(), 1, 0, "grp/x")
	s.Require().Error(err)
	s.Require().ErrorContains(err, "project id and iid required")
}

func (s *MRActionsSuite) TestCloseMergeRequest_InvalidatesCache() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge_requests/7"):
			_, _ = fmt.Fprint(w, `{"id":1,"iid":7,"title":"C","state":"closed","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}`)
		default:
			hits.Add(1)
			_, _ = fmt.Fprint(w, `[{"id":1,"iid":7,"title":"C","state":"opened","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}]`)
		}
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

	opts := gitlab.ListMergeRequestsOptions{ProjectID: 42, ProjectPath: "grp/x", State: models.MRStateFilterOpened}
	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), hits.Load())

	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), hits.Load(), "second fresh call skips HTTP")

	_, err = client.CloseMergeRequest(context.Background(), 42, 7, "grp/x")
	s.Require().NoError(err)

	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(2), hits.Load(), "close must invalidate mr_list cache")
}

func (s *MRActionsSuite) TestAcceptMergeRequest_InvalidatesCache() {
	var listHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge"):
			_, _ = fmt.Fprint(w, `{"id":1,"iid":7,"title":"M","state":"merged","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}`)
		default:
			listHits.Add(1)
			_, _ = fmt.Fprint(w, `[{"id":1,"iid":7,"title":"M","state":"opened","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"s","target_branch":"main","web_url":"u"}]`)
		}
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

	opts := gitlab.ListMergeRequestsOptions{ProjectID: 42, ProjectPath: "grp/x", State: models.MRStateFilterOpened}
	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(1), listHits.Load(), "second list served from cache")

	_, err = client.AcceptMergeRequest(context.Background(), 42, 7, "grp/x", gitlab.AcceptOptions{})
	s.Require().NoError(err)

	_, err = client.ListMergeRequests(context.Background(), opts)
	s.Require().NoError(err)
	s.Require().Equal(int32(2), listHits.Load(), "accept must invalidate mr_list cache")
}

func (s *MRActionsSuite) TestAcceptMergeRequest_SendsSquashAndDeleteBranch() {
	var (
		method string
		path   string
		body   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		body = decodeJSONBody(s.T(), r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":9,"iid":5,"title":"Merged","state":"merged","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"x","target_branch":"main","web_url":"u"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	mr, err := client.AcceptMergeRequest(
		context.Background(),
		42, 5,
		"grp/alpha",
		gitlab.AcceptOptions{Squash: true, ShouldRemoveSourceBranch: true},
	)

	s.Require().NoError(err)
	s.Require().Equal(http.MethodPut, method)
	s.Require().Contains(path, "/projects/42/merge_requests/5/merge")
	s.Require().Equal(true, body["squash"])
	s.Require().Equal(true, body["should_remove_source_branch"])
	_, autoMergeSent := body["auto_merge"]
	s.Require().False(autoMergeSent, "auto_merge must not be sent")
	_, mwpsSent := body["merge_when_pipeline_succeeds"]
	s.Require().False(mwpsSent, "deprecated merge_when_pipeline_succeeds must not be sent")
	s.Require().NotNil(mr)
	s.Require().Equal(models.MRStateMerged, mr.State)
	s.Require().Equal(5, mr.IID)
}

func (s *MRActionsSuite) TestAcceptMergeRequest_DefaultOptionsSendFalse() {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeJSONBody(s.T(), r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":9,"iid":5,"title":"Merged","state":"merged","author":{"id":1,"username":"a","name":"A","web_url":"u"},"source_branch":"x","target_branch":"main","web_url":"u"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.AcceptMergeRequest(context.Background(), 1, 1, "grp/x", gitlab.AcceptOptions{})

	s.Require().NoError(err)
	s.Require().Equal(false, body["squash"])
	s.Require().Equal(false, body["should_remove_source_branch"])
}

func (s *MRActionsSuite) TestAcceptMergeRequest_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = fmt.Fprint(w, `{"message":"Branch cannot be merged"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.AcceptMergeRequest(context.Background(), 1, 2, "grp/x", gitlab.AcceptOptions{})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: accept merge request 1!2")
}

func (s *MRActionsSuite) TestAcceptMergeRequest_RejectsZeroIDs() {
	client, err := gitlab.New(config.GitLabConfig{URL: "https://gl", Token: "secret"})
	s.Require().NoError(err)

	_, err = client.AcceptMergeRequest(context.Background(), 0, 1, "grp/x", gitlab.AcceptOptions{})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "project id and iid required")

	_, err = client.AcceptMergeRequest(context.Background(), 1, 0, "grp/x", gitlab.AcceptOptions{})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "project id and iid required")
}

func TestMRActionsSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(MRActionsSuite))
}

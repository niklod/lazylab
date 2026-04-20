package gitlab_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
)

type DiscussionsSuite struct {
	suite.Suite
}

func (s *DiscussionsSuite) TestListMRDiscussions_PaginatesAndMapsFields() {
	page1 := `[
		{"id":"d1","notes":[
			{"id":1,"body":"nit on line 42","author":{"id":10,"username":"alice","name":"Alice","web_url":"https://gl/u/alice"},"created_at":"2026-04-10T10:00:00Z","resolvable":true,"resolved":false,"system":false,"position":{"new_path":"foo.go","new_line":42}},
			{"id":2,"body":"fixed","author":{"id":11,"username":"bob","name":"Bob"},"created_at":"2026-04-11T10:00:00Z","resolvable":true,"resolved":true,"resolved_by":{"id":10,"username":"alice","name":"Alice"},"system":false}
		]},
		{"id":"d2","notes":[
			{"id":3,"body":"general comment","author":{"id":12,"username":"carol","name":"Carol"},"resolvable":false,"system":false}
		]}
	]`
	page2 := `[
		{"id":"d3","notes":[
			{"id":4,"body":"resolved thread","author":{"id":13,"username":"dev","name":"Dev"},"system":true}
		]}
	]`

	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path+"?"+r.URL.RawQuery)
		page, _ := url.ParseQuery(r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Next-Page", "")
		if page.Get("page") == "" || page.Get("page") == "1" {
			w.Header().Set("X-Next-Page", "2")
			_, _ = fmt.Fprint(w, page1)

			return
		}
		_, _ = fmt.Fprint(w, page2)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	discs, err := client.ListMRDiscussions(context.Background(), 42, 7)

	s.Require().NoError(err)
	s.Require().Len(gotPaths, 2, "two pages fetched")
	s.Require().Contains(gotPaths[0], "/projects/42/merge_requests/7/discussions")

	s.Require().Len(discs, 3)
	s.Require().Equal("d1", discs[0].ID)
	s.Require().Len(discs[0].Notes, 2)
	s.Require().Equal("alice", discs[0].Notes[0].Author.Username)
	s.Require().True(discs[0].Notes[0].Resolvable)
	s.Require().False(discs[0].Notes[0].Resolved)
	s.Require().Equal("foo.go", discs[0].Notes[0].Position.NewPath)
	s.Require().Equal(42, discs[0].Notes[0].Position.NewLine)
	s.Require().NotNil(discs[0].Notes[1].ResolvedBy)
	s.Require().Equal("alice", discs[0].Notes[1].ResolvedBy.Username)

	s.Require().False(discs[1].IsResolvable(), "d2 is a general comment")
	s.Require().True(discs[2].Notes[0].System, "system note flag round-trips")
}

func (s *DiscussionsSuite) TestListMRDiscussions_WrapsUpstreamError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"boom"}`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, err = client.ListMRDiscussions(context.Background(), 42, 7)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: list mr discussions")
}

func (s *DiscussionsSuite) TestListMRDiscussions_EmptyBodyReturnsEmpty() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	discs, err := client.ListMRDiscussions(context.Background(), 1, 1)
	s.Require().NoError(err)
	s.Require().Empty(discs)
}

func (s *DiscussionsSuite) TestListMRDiscussions_ValidatesInputs() {
	client, err := gitlab.New(
		config.GitLabConfig{URL: "https://gitlab.example", Token: "secret"},
	)
	s.Require().NoError(err)

	_, err = client.ListMRDiscussions(context.Background(), 0, 7)
	s.Require().Error(err)

	_, err = client.ListMRDiscussions(context.Background(), 1, 0)
	s.Require().Error(err)
}

func (s *DiscussionsSuite) TestListMRDiscussions_CachedReusesOnSecondCall() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"id":"d1","notes":[{"id":1,"resolvable":false}]}]`)
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

	_, err = client.ListMRDiscussions(context.Background(), 1, 2)
	s.Require().NoError(err)
	_, err = client.ListMRDiscussions(context.Background(), 1, 2)
	s.Require().NoError(err)

	s.Require().Equal(int32(1), hits.Load(), "second fresh call skips HTTP")
}

func TestDiscussionsSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DiscussionsSuite))
}

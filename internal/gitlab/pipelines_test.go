package gitlab_test

import (
	"context"
	"fmt"
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

const (
	pipelinesFixture = `[{"id":77,"iid":1,"project_id":11,"status":"running","ref":"feat/x","web_url":"u","updated_at":"2026-04-10T12:00:00Z"}]`
	pipelineFixture  = `{"id":77,"status":"running","ref":"feat/x","sha":"deadbeef","web_url":"u","updated_at":"2026-04-10T12:00:00Z","created_at":"2026-04-10T11:00:00Z","user":{"id":42,"username":"mira.k","name":"Mira K"}}`

	pipelineJobsPage1 = `[
		{"id":1,"name":"build","stage":"build","status":"success","web_url":"u1","duration":12.5,"allow_failure":false},
		{"id":2,"name":"test:unit","stage":"test","status":"failed","web_url":"u2","duration":90,"allow_failure":true}
	]`
	pipelineJobsPage2 = `[
		{"id":3,"name":"deploy:prod","stage":"deploy","status":"manual","web_url":"u3","duration":0,"allow_failure":false}
	]`
)

type PipelinesSuite struct {
	suite.Suite
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_MapsPipelineAndPaginatedJobs() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/merge_requests/5/pipelines"):
			_, _ = fmt.Fprint(w, pipelinesFixture)
		case strings.HasSuffix(r.URL.Path, "/pipelines/77"):
			_, _ = fmt.Fprint(w, pipelineFixture)
		case strings.Contains(r.URL.Path, "/pipelines/77/jobs"):
			page := r.URL.Query().Get("page")
			if page == "" {
				page = "1"
			}
			switch page {
			case "1":
				w.Header().Set("X-Next-Page", "2")
				_, _ = fmt.Fprint(w, pipelineJobsPage1)
			case "2":
				w.Header().Set("X-Next-Page", "")
				_, _ = fmt.Fprint(w, pipelineJobsPage2)
			default:
				s.T().Fatalf("unexpected page %q", page)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	s.T().Cleanup(srv.Close)

	client := s.newClient(srv)

	detail, err := client.GetMRPipelineDetail(context.Background(), 11, 5)

	s.Require().NoError(err)
	s.Require().NotNil(detail)
	s.Require().Equal(77, detail.Pipeline.ID)
	s.Require().Equal(models.PipelineStatusRunning, detail.Pipeline.Status)
	s.Require().Len(detail.Jobs, 3)
	// Fixture pages: page1=[build, test:unit], page2=[deploy:prod].
	// GitLab returns newest-first; the client reverses to exec order so
	// the first job is the earliest-run stage (build).
	s.Require().Equal("deploy", detail.Jobs[0].Stage)
	s.Require().Equal("deploy:prod", detail.Jobs[0].Name)
	s.Require().Equal("test:unit", detail.Jobs[1].Name)
	s.Require().Equal(models.PipelineStatusFailed, detail.Jobs[1].Status)
	s.Require().Equal("build", detail.Jobs[2].Stage)
	s.Require().NotNil(detail.Jobs[2].Duration)
	s.Require().InDelta(12.5, *detail.Jobs[2].Duration, 0.01)
	s.Require().Nil(detail.Jobs[0].Duration, "zero duration maps to nil")
	s.Require().NotNil(detail.Pipeline.TriggeredBy, "triggered_by must propagate from upstream user")
	s.Require().Equal("mira.k", detail.Pipeline.TriggeredBy.Username)
	s.Require().Equal("Mira K", detail.Pipeline.TriggeredBy.Name)
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_ReversesAPIOrderToPipelineExecOrder() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/merge_requests/5/pipelines"):
			_, _ = fmt.Fprint(w, pipelinesFixture)
		case strings.HasSuffix(r.URL.Path, "/pipelines/77"):
			_, _ = fmt.Fprint(w, pipelineFixture)
		case strings.Contains(r.URL.Path, "/pipelines/77/jobs"):
			_, _ = fmt.Fprint(w, `[
				{"id":30,"name":"deploy:prod","stage":"deploy","status":"manual","duration":0},
				{"id":20,"name":"test:unit","stage":"test","status":"success","duration":5},
				{"id":10,"name":"build:bin","stage":"build","status":"success","duration":12}
			]`)
		default:
			http.NotFound(w, r)
		}
	}))
	s.T().Cleanup(srv.Close)

	detail, err := s.newClient(srv).GetMRPipelineDetail(context.Background(), 11, 5)

	s.Require().NoError(err)
	s.Require().Len(detail.Jobs, 3)
	s.Require().Equal("build", detail.Jobs[0].Stage)
	s.Require().Equal("test", detail.Jobs[1].Stage)
	s.Require().Equal("deploy", detail.Jobs[2].Stage)
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_EmptyPipelinesReturnsNil() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/merge_requests/5/pipelines") {
			_, _ = fmt.Fprint(w, `[]`)

			return
		}
		http.NotFound(w, r)
	}))
	s.T().Cleanup(srv.Close)

	client := s.newClient(srv)

	detail, err := client.GetMRPipelineDetail(context.Background(), 11, 5)

	s.Require().NoError(err)
	s.Require().Nil(detail)
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_PipelineFetchErrorWrapped() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/merge_requests/5/pipelines"):
			_, _ = fmt.Fprint(w, pipelinesFixture)
		case strings.HasSuffix(r.URL.Path, "/pipelines/77"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"message":"pipeline explode"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	s.T().Cleanup(srv.Close)

	client := s.newClient(srv)

	_, err := client.GetMRPipelineDetail(context.Background(), 11, 5)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: get pipeline 77")
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_JobsPageErrorWrapped() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/merge_requests/5/pipelines"):
			_, _ = fmt.Fprint(w, pipelinesFixture)
		case strings.HasSuffix(r.URL.Path, "/pipelines/77"):
			_, _ = fmt.Fprint(w, pipelineFixture)
		case strings.Contains(r.URL.Path, "/pipelines/77/jobs"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"message":"jobs explode"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	s.T().Cleanup(srv.Close)

	client := s.newClient(srv)

	_, err := client.GetMRPipelineDetail(context.Background(), 11, 5)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "fetch mr pipeline detail",
		"jobs error is wrapped at the fetch layer to keep call-site context")
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_UpstreamErrorWrapped() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"boom"}`)
	}))
	s.T().Cleanup(srv.Close)

	client := s.newClient(srv)

	_, err := client.GetMRPipelineDetail(context.Background(), 11, 5)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: list mr pipelines")
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_RejectsInvalidInput() {
	client := s.newClient(nil)

	_, err := client.GetMRPipelineDetail(context.Background(), 0, 5)
	s.Require().ErrorContains(err, "project id and iid required")

	_, err = client.GetMRPipelineDetail(context.Background(), 11, 0)
	s.Require().ErrorContains(err, "project id and iid required")
}

func (s *PipelinesSuite) TestGetJobTrace_ReturnsBody() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/jobs/99/trace") {
			_, _ = fmt.Fprint(w, "line 1\nline 2\n")

			return
		}
		http.NotFound(w, r)
	}))
	s.T().Cleanup(srv.Close)

	client := s.newClient(srv)

	trace, err := client.GetJobTrace(context.Background(), 11, 99)

	s.Require().NoError(err)
	s.Require().Contains(trace, "line 1")
	s.Require().Contains(trace, "line 2")
}

func (s *PipelinesSuite) TestGetJobTrace_RejectsInvalidInput() {
	client := s.newClient(nil)

	_, err := client.GetJobTrace(context.Background(), 0, 99)
	s.Require().ErrorContains(err, "project id and job id required")

	_, err = client.GetJobTrace(context.Background(), 11, 0)
	s.Require().ErrorContains(err, "project id and job id required")
}

func (s *PipelinesSuite) TestGetMRPipelineDetail_CachedClient_Dedups() {
	var pipelineHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/merge_requests/5/pipelines"):
			pipelineHits.Add(1)
			_, _ = fmt.Fprint(w, pipelinesFixture)
		case strings.HasSuffix(r.URL.Path, "/pipelines/77"):
			_, _ = fmt.Fprint(w, pipelineFixture)
		case strings.Contains(r.URL.Path, "/pipelines/77/jobs"):
			_, _ = fmt.Fprint(w, pipelineJobsPage2)
		default:
			http.NotFound(w, r)
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

	_, err = client.GetMRPipelineDetail(context.Background(), 11, 5)
	s.Require().NoError(err)
	_, err = client.GetMRPipelineDetail(context.Background(), 11, 5)
	s.Require().NoError(err)

	s.Require().Equal(int32(1), pipelineHits.Load(), "fresh cache skips upstream")
}

func (s *PipelinesSuite) newClient(srv *httptest.Server) *gitlab.Client {
	url := "https://example.invalid"
	var httpClient = http.DefaultClient
	if srv != nil {
		url = srv.URL
		httpClient = srv.Client()
	}
	client, err := gitlab.New(
		config.GitLabConfig{URL: url, Token: "secret"},
		gitlab.WithHTTPClient(httpClient),
	)
	s.Require().NoError(err)

	return client
}

func TestPipelinesSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(PipelinesSuite))
}

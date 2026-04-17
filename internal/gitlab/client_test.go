package gitlab_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
)

type ClientSuite struct {
	suite.Suite
}

func (s *ClientSuite) TestNew_ReturnsErrorOnEmptyURL() {
	_, err := gitlab.New(config.GitLabConfig{URL: "", Token: "tok"})

	s.Require().ErrorIs(err, gitlab.ErrMissingURL)
}

func (s *ClientSuite) TestNew_ReturnsErrorOnEmptyToken() {
	_, err := gitlab.New(config.GitLabConfig{URL: "https://gitlab.example.com", Token: ""})

	s.Require().ErrorIs(err, gitlab.ErrMissingToken)
}

func (s *ClientSuite) TestNew_SucceedsWithValidConfig() {
	client, err := gitlab.New(config.GitLabConfig{
		URL:   "https://gitlab.example.com",
		Token: "tok",
	})

	s.Require().NoError(err)
	s.Require().NotNil(client)
	s.Require().NotNil(client.API())
}

func (s *ClientSuite) TestNew_WrapsUpstreamError() {
	_, err := gitlab.New(config.GitLabConfig{
		URL:   "://bad-scheme",
		Token: "tok",
	})

	s.Require().Error(err)
	s.Require().ErrorContains(err, "gitlab: new client")
}

func (s *ClientSuite) TestNew_UsesInjectedHTTPClientAgainstTestServer() {
	var (
		gotToken string
		gotPath  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"u","name":"U","web_url":"https://x"}`))
	}))
	s.T().Cleanup(srv.Close)

	client, err := gitlab.New(
		config.GitLabConfig{URL: srv.URL, Token: "secret"},
		gitlab.WithHTTPClient(srv.Client()),
	)
	s.Require().NoError(err)

	_, _, err = client.API().Users.CurrentUser()

	s.Require().NoError(err)
	s.Require().Equal("secret", gotToken)
	s.Require().Contains(gotPath, "/user")
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(ClientSuite))
}

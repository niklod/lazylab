package appcontext_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
)

type AppContextSuite struct {
	suite.Suite
	cfg    *config.Config
	client *gitlab.Client
}

func (s *AppContextSuite) SetupTest() {
	s.cfg = config.Defaults()
	s.cfg.GitLab.URL = "https://gitlab.example.com"
	s.cfg.GitLab.Token = "tok"
	client, err := gitlab.New(s.cfg.GitLab)
	s.Require().NoError(err)
	s.client = client
}

func (s *AppContextSuite) TestNew_WiresConfigAndClient() {
	ctx := appcontext.New(s.cfg, s.client)

	s.Require().Same(s.cfg, ctx.Config)
	s.Require().Same(s.client, ctx.GitLab)
	s.Require().Nil(ctx.CurrentProject)
}

func (s *AppContextSuite) TestWithCurrentProject_ReturnsCopy_DoesNotMutateOriginal() {
	ctx := appcontext.New(s.cfg, s.client)
	project := &models.Project{ID: 42, PathWithNamespace: "group/demo"}

	next := ctx.WithCurrentProject(project)

	s.Require().Nil(ctx.CurrentProject)
	s.Require().Same(project, next.CurrentProject)
	s.Require().Same(s.cfg, next.Config)
	s.Require().Same(s.client, next.GitLab)
	s.Require().NotSame(ctx, next)
}

func (s *AppContextSuite) TestWithCurrentProject_Nil_Clears() {
	ctx := appcontext.New(s.cfg, s.client).WithCurrentProject(&models.Project{ID: 1})

	cleared := ctx.WithCurrentProject(nil)

	s.Require().Nil(cleared.CurrentProject)
	s.Require().NotNil(ctx.CurrentProject)
}

func TestAppContextSuite(t *testing.T) {
	suite.Run(t, new(AppContextSuite))
}

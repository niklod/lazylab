package appcontext_test

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/cache"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
)

const testConfigPath = "/cfg/config.yaml"

type AppContextSuite struct {
	suite.Suite
	cfg    *config.Config
	client *gitlab.Client
	cache  *cache.Cache
	fs     afero.Fs
}

func (s *AppContextSuite) SetupTest() {
	s.cfg = config.Defaults()
	s.cfg.GitLab.URL = "https://gitlab.example.com"
	s.cfg.GitLab.Token = "tok"
	client, err := gitlab.New(s.cfg.GitLab)
	s.Require().NoError(err)
	s.client = client
	s.fs = afero.NewMemMapFs()
	s.cache = cache.New(s.cfg.Cache, s.fs)
}

func (s *AppContextSuite) TestNew_WiresAllFields() {
	ctx := appcontext.New(s.cfg, s.client, s.cache, s.fs, testConfigPath)

	s.Require().Same(s.cfg, ctx.Config)
	s.Require().Same(s.client, ctx.GitLab)
	s.Require().Same(s.cache, ctx.Cache)
	s.Require().Equal(s.fs, ctx.FS)
	s.Require().Equal(testConfigPath, ctx.ConfigPath)
	s.Require().Nil(ctx.CurrentProject)
}

func (s *AppContextSuite) TestWithCurrentProject_ReturnsCopy_DoesNotMutateOriginal() {
	ctx := appcontext.New(s.cfg, s.client, s.cache, s.fs, testConfigPath)
	project := &models.Project{ID: 42, PathWithNamespace: "group/demo"}

	next := ctx.WithCurrentProject(project)

	s.Require().Nil(ctx.CurrentProject)
	s.Require().Same(project, next.CurrentProject)
	s.Require().Same(s.cfg, next.Config)
	s.Require().Same(s.client, next.GitLab)
	s.Require().Same(s.cache, next.Cache)
	s.Require().Equal(s.fs, next.FS)
	s.Require().Equal(testConfigPath, next.ConfigPath)
	s.Require().NotSame(ctx, next)
}

func (s *AppContextSuite) TestWithCurrentProject_Nil_Clears() {
	ctx := appcontext.New(s.cfg, s.client, s.cache, s.fs, testConfigPath).WithCurrentProject(&models.Project{ID: 1})

	cleared := ctx.WithCurrentProject(nil)

	s.Require().Nil(cleared.CurrentProject)
	s.Require().NotNil(ctx.CurrentProject)
}

func TestAppContextSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(AppContextSuite))
}

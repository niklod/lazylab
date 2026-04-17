package appcontext_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/niklod/lazylab/internal/appcontext"
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/models"
)

type AppContextSuite struct {
	suite.Suite
	cfg *config.Config
}

func (s *AppContextSuite) SetupTest() {
	s.cfg = config.Defaults()
}

func (s *AppContextSuite) TestNew_WiresConfig() {
	ctx := appcontext.New(s.cfg)

	s.Require().Same(s.cfg, ctx.Config)
	s.Require().Nil(ctx.CurrentProject)
}

func (s *AppContextSuite) TestWithCurrentProject_ReturnsCopy_DoesNotMutateOriginal() {
	ctx := appcontext.New(s.cfg)
	project := &models.Project{ID: 42, PathWithNamespace: "group/demo"}

	next := ctx.WithCurrentProject(project)

	s.Require().Nil(ctx.CurrentProject)
	s.Require().Same(project, next.CurrentProject)
	s.Require().Same(s.cfg, next.Config)
	s.Require().NotSame(ctx, next)
}

func (s *AppContextSuite) TestWithCurrentProject_Nil_Clears() {
	ctx := appcontext.New(s.cfg).WithCurrentProject(&models.Project{ID: 1})

	cleared := ctx.WithCurrentProject(nil)

	s.Require().Nil(cleared.CurrentProject)
	s.Require().NotNil(ctx.CurrentProject)
}

func TestAppContextSuite(t *testing.T) {
	suite.Run(t, new(AppContextSuite))
}

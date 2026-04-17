package appcontext

import (
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/models"
)

type AppContext struct {
	Config         *config.Config
	CurrentProject *models.Project
}

func New(cfg *config.Config) *AppContext {
	return &AppContext{Config: cfg}
}

func (c *AppContext) WithCurrentProject(p *models.Project) *AppContext {
	next := *c
	next.CurrentProject = p
	return &next
}

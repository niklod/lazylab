package appcontext

import (
	"github.com/niklod/lazylab/internal/config"
	"github.com/niklod/lazylab/internal/gitlab"
	"github.com/niklod/lazylab/internal/models"
)

type AppContext struct {
	Config         *config.Config
	GitLab         *gitlab.Client
	CurrentProject *models.Project
}

func New(cfg *config.Config, gl *gitlab.Client) *AppContext {
	return &AppContext{Config: cfg, GitLab: gl}
}

// WithCurrentProject returns a shallow copy with CurrentProject set.
// Pointer fields (Config, GitLab) are shared with the receiver — treat them as read-only.
func (c *AppContext) WithCurrentProject(p *models.Project) *AppContext {
	next := *c
	next.CurrentProject = p
	return &next
}

package gitlab

import (
	"context"
	"fmt"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/models"
)

const projectsCacheNamespace = "projects"

const defaultProjectsPerPage = 100

// ListProjectsOptions mirrors the Python `list_projects` arguments.
// All fields have sensible defaults; the zero value means "use Python-parity default".
type ListProjectsOptions struct {
	Membership *bool
	Archived   *bool
	OrderBy    string
	Sort       string
	PerPage    int
}

func (o ListProjectsOptions) resolved() ListProjectsOptions {
	r := o
	if r.Membership == nil {
		r.Membership = ptr(true)
	}
	if r.Archived == nil {
		r.Archived = ptr(false)
	}
	if r.OrderBy == "" {
		r.OrderBy = "last_activity_at"
	}
	if r.Sort == "" {
		r.Sort = "desc"
	}
	if r.PerPage <= 0 {
		r.PerPage = defaultProjectsPerPage
	}

	return r
}

func ptr[T any](v T) *T { return &v }

// ListProjects fetches every page of projects matching opts and returns them as
// domain models. Mirrors Python `GitLabClient.list_projects` with `get_all=True`.
//
// When the client was built with WithCache, the call is routed through
// cache.Do[T]: a disk/memory hit returns immediately while a stale entry
// triggers a silent background refresh (ADR 009). The cache key includes the
// resolved option fields so divergent call sites don't share state.
func (c *Client) ListProjects(ctx context.Context, opts ListProjectsOptions) ([]*models.Project, error) {
	r := opts.resolved()
	loader := func(ctx context.Context) ([]*models.Project, error) {
		return c.listProjectsRaw(ctx, r)
	}

	return doCached(ctx, c, projectsCacheNamespace, "list projects", loader,
		*r.Membership, *r.Archived, r.OrderBy, r.Sort,
	)
}

func (c *Client) listProjectsRaw(ctx context.Context, r ListProjectsOptions) ([]*models.Project, error) {
	listOpts := &gogitlab.ListProjectsOptions{
		Membership: r.Membership,
		Archived:   r.Archived,
		OrderBy:    &r.OrderBy,
		Sort:       &r.Sort,
		ListOptions: gogitlab.ListOptions{
			Page:    1,
			PerPage: int64(r.PerPage),
		},
	}

	var out []*models.Project
	for {
		page, resp, err := c.api.Projects.ListProjects(listOpts, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list projects page %d: %w", listOpts.Page, err)
		}
		for _, p := range page {
			out = append(out, toDomainProject(p))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return out, nil
}

func toDomainProject(p *gogitlab.Project) *models.Project {
	if p == nil {
		return nil
	}
	m := &models.Project{
		ID:                int(p.ID),
		Name:              p.Name,
		PathWithNamespace: p.PathWithNamespace,
		DefaultBranch:     p.DefaultBranch,
		WebURL:            p.WebURL,
		Archived:          p.Archived,
	}
	if p.LastActivityAt != nil {
		m.LastActivityAt = *p.LastActivityAt
	}

	return m
}

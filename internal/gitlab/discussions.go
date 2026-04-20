package gitlab

import (
	"context"
	"fmt"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/models"
)

const mrConversationCacheNamespace = "mr_conversation"

// ListMRDiscussions fetches every discussion (and their notes) on an MR and
// maps them to domain models. Separate cache namespace from
// mrDiscussionsCacheNamespace — that stored *DiscussionStats, this stores
// the full []*models.Discussion; cache.Do is generic over value type, so
// sharing a namespace would fight the type system.
func (c *Client) ListMRDiscussions(ctx context.Context, projectID, iid int) ([]*models.Discussion, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: list mr discussions: project id and iid required")
	}

	loader := func(ctx context.Context) ([]*models.Discussion, error) {
		return c.fetchMRDiscussions(ctx, projectID, iid)
	}

	return doCached(ctx, c, mrConversationCacheNamespace, "list mr discussions", loader, projectID, iid)
}

func (c *Client) fetchMRDiscussions(ctx context.Context, projectID, iid int) ([]*models.Discussion, error) {
	listOpts := &gogitlab.ListMergeRequestDiscussionsOptions{
		ListOptions: gogitlab.ListOptions{
			Page:    1,
			PerPage: int64(defaultDiscussionsPerPage),
		},
	}

	var out []*models.Discussion
	for {
		page, resp, err := c.api.Discussions.ListMergeRequestDiscussions(projectID, int64(iid), listOpts, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list mr discussions page %d: %w", listOpts.Page, err)
		}
		for _, d := range page {
			if d == nil {
				continue
			}
			out = append(out, toDomainDiscussion(d))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return out, nil
}

func toDomainDiscussion(d *gogitlab.Discussion) *models.Discussion {
	if d == nil {
		return nil
	}
	notes := make([]models.Note, 0, len(d.Notes))
	for _, n := range d.Notes {
		if n == nil {
			continue
		}
		notes = append(notes, toDomainNote(n))
	}

	return &models.Discussion{ID: d.ID, Notes: notes}
}

func toDomainNote(n *gogitlab.Note) models.Note {
	note := models.Note{
		ID:         int(n.ID),
		Body:       n.Body,
		Author:     domainUserFromNoteAuthor(n.Author),
		Resolvable: n.Resolvable,
		Resolved:   n.Resolved,
		System:     n.System,
		Position:   toDomainNotePosition(n.Position),
	}
	if n.CreatedAt != nil {
		note.CreatedAt = *n.CreatedAt
	}
	if n.Resolved && n.ResolvedBy.ID != 0 {
		resolver := domainUserFromNoteResolvedBy(n.ResolvedBy)
		note.ResolvedBy = &resolver
	}

	return note
}

func toDomainNotePosition(p *gogitlab.NotePosition) *models.NotePosition {
	if p == nil {
		return nil
	}

	return &models.NotePosition{
		NewPath: p.NewPath,
		OldPath: p.OldPath,
		NewLine: int(p.NewLine),
		OldLine: int(p.OldLine),
	}
}

func domainUserFromNoteAuthor(a gogitlab.NoteAuthor) models.User {
	return models.User{
		ID:        int(a.ID),
		Username:  a.Username,
		Name:      a.Name,
		WebURL:    a.WebURL,
		AvatarURL: a.AvatarURL,
	}
}

func domainUserFromNoteResolvedBy(r gogitlab.NoteResolvedBy) models.User {
	return models.User{
		ID:        int(r.ID),
		Username:  r.Username,
		Name:      r.Name,
		WebURL:    r.WebURL,
		AvatarURL: r.AvatarURL,
	}
}

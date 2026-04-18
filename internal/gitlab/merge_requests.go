package gitlab

import (
	"context"
	"fmt"

	gogitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/niklod/lazylab/internal/models"
)

const (
	mrListCacheNamespace        = "mr_list"
	mrCacheNamespace            = "mr"
	mrApprovalsCacheNamespace   = "mr_approvals"
	mrDiscussionsCacheNamespace = "mr_discussions"
	mrChangesCacheNamespace     = "mr_changes"

	defaultMRsPerPage         = 100
	defaultDiscussionsPerPage = 100
	defaultMRDiffsPerPage     = 100
)

// ListMergeRequestsOptions mirrors the Python `list_merge_requests` arguments.
// AuthorID / ReviewerID are optional — nil means "no filter".
type ListMergeRequestsOptions struct {
	ProjectID   int
	ProjectPath string
	State       models.MRStateFilter
	AuthorID    *int
	ReviewerID  *int
}

// ListMergeRequests fetches every page of MRs for a project and maps them to
// domain models. Routed through cache.Do when the client was built with
// WithCache (ADR 009). Mirrors Python `list_merge_requests` — order_by=updated_at,
// sort=desc.
func (c *Client) ListMergeRequests(ctx context.Context, opts ListMergeRequestsOptions) ([]*models.MergeRequest, error) {
	if opts.ProjectID <= 0 {
		return nil, fmt.Errorf("gitlab: list merge requests: project id required")
	}
	state := opts.State
	if state == "" {
		state = models.MRStateFilterOpened
	}

	loader := func(ctx context.Context) ([]*models.MergeRequest, error) {
		return c.listMergeRequestsRaw(ctx, opts.ProjectID, opts.ProjectPath, state, opts.AuthorID, opts.ReviewerID)
	}

	return doCached(ctx, c, mrListCacheNamespace, "list merge requests", loader,
		opts.ProjectID, string(state),
		intPtrArg("author", opts.AuthorID),
		intPtrArg("reviewer", opts.ReviewerID),
	)
}

func (c *Client) listMergeRequestsRaw(
	ctx context.Context,
	projectID int,
	projectPath string,
	state models.MRStateFilter,
	authorID, reviewerID *int,
) ([]*models.MergeRequest, error) {
	orderBy := "updated_at"
	sort := "desc"
	stateStr := string(state)

	listOpts := &gogitlab.ListProjectMergeRequestsOptions{
		State:   &stateStr,
		OrderBy: &orderBy,
		Sort:    &sort,
		ListOptions: gogitlab.ListOptions{
			Page:    1,
			PerPage: int64(defaultMRsPerPage),
		},
	}
	if authorID != nil {
		id := int64(*authorID)
		listOpts.AuthorID = &id
	}
	if reviewerID != nil {
		listOpts.ReviewerID = gogitlab.ReviewerID(int64(*reviewerID))
	}

	var out []*models.MergeRequest
	for {
		page, resp, err := c.api.MergeRequests.ListProjectMergeRequests(projectID, listOpts, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list merge requests page %d: %w", listOpts.Page, err)
		}
		for _, mr := range page {
			out = append(out, toDomainMergeRequestFromBasic(mr, projectPath))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return out, nil
}

// GetMergeRequest fetches a single MR by IID. Cached per (project_id, iid).
func (c *Client) GetMergeRequest(ctx context.Context, projectID, iid int, projectPath string) (*models.MergeRequest, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: get merge request: project id and iid required")
	}

	loader := func(ctx context.Context) (*models.MergeRequest, error) {
		mr, _, err := c.api.MergeRequests.GetMergeRequest(projectID, int64(iid), nil, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: get merge request %d!%d: %w", projectID, iid, err)
		}

		return toDomainMergeRequestFromFull(mr, projectPath), nil
	}

	return doCached(ctx, c, mrCacheNamespace, "get merge request", loader, projectID, iid)
}

// GetMRDiscussionStats aggregates resolvable/resolved counts across all
// discussions on an MR. Mirrors Python `get_mr_discussion_stats`: a
// discussion counts as resolvable iff its first note is resolvable; it
// counts as resolved iff every resolvable note in that discussion is
// resolved. Cached per (project_id, iid).
func (c *Client) GetMRDiscussionStats(ctx context.Context, projectID, iid int) (*models.DiscussionStats, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: get mr discussion stats: project id and iid required")
	}

	loader := func(ctx context.Context) (*models.DiscussionStats, error) {
		return c.listMRDiscussionsRaw(ctx, projectID, iid)
	}

	return doCached(ctx, c, mrDiscussionsCacheNamespace, "get mr discussion stats", loader, projectID, iid)
}

func (c *Client) listMRDiscussionsRaw(ctx context.Context, projectID, iid int) (*models.DiscussionStats, error) {
	listOpts := &gogitlab.ListMergeRequestDiscussionsOptions{
		ListOptions: gogitlab.ListOptions{
			Page:    1,
			PerPage: int64(defaultDiscussionsPerPage),
		},
	}

	stats := &models.DiscussionStats{}
	for {
		page, resp, err := c.api.Discussions.ListMergeRequestDiscussions(projectID, int64(iid), listOpts, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list mr discussions page %d: %w", listOpts.Page, err)
		}
		for _, d := range page {
			if d == nil || len(d.Notes) == 0 {
				continue
			}
			if !d.Notes[0].Resolvable {
				continue
			}
			stats.TotalResolvable++
			if allResolvableResolved(d.Notes) {
				stats.Resolved++
			}
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return stats, nil
}

func allResolvableResolved(notes []*gogitlab.Note) bool {
	for _, n := range notes {
		if n == nil || !n.Resolvable {
			continue
		}
		if !n.Resolved {
			return false
		}
	}

	return true
}

// GetMRChanges fetches the list of file-level diffs for an MR. Mirrors Python
// `get_mr_changes`; uses client-go's ListMergeRequestDiffs (the non-deprecated
// replacement for /changes). The MergeRequestDiff shape — OldPath, NewPath,
// Diff, NewFile, RenamedFile, DeletedFile — already matches models.MRDiffFile
// 1:1, so the mapping is a direct copy. Paginated until NextPage == 0.
// Cached per (project_id, iid).
func (c *Client) GetMRChanges(ctx context.Context, projectID, iid int) (*models.MRDiffData, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: get mr changes: project id and iid required")
	}

	loader := func(ctx context.Context) (*models.MRDiffData, error) {
		return c.listMRDiffsRaw(ctx, projectID, iid)
	}

	return doCached(ctx, c, mrChangesCacheNamespace, "get mr changes", loader, projectID, iid)
}

func (c *Client) listMRDiffsRaw(ctx context.Context, projectID, iid int) (*models.MRDiffData, error) {
	listOpts := &gogitlab.ListMergeRequestDiffsOptions{
		ListOptions: gogitlab.ListOptions{
			Page:    1,
			PerPage: int64(defaultMRDiffsPerPage),
		},
	}

	out := &models.MRDiffData{}
	for {
		page, resp, err := c.api.MergeRequests.ListMergeRequestDiffs(projectID, int64(iid), listOpts, gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list mr diffs page %d: %w", listOpts.Page, err)
		}
		for _, d := range page {
			if d == nil {
				continue
			}
			out.Files = append(out.Files, models.MRDiffFile{
				OldPath:     d.OldPath,
				NewPath:     d.NewPath,
				Diff:        d.Diff,
				NewFile:     d.NewFile,
				RenamedFile: d.RenamedFile,
				DeletedFile: d.DeletedFile,
			})
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return out, nil
}

// GetMRApprovals fetches approval status for an MR. Cached per (project_id, iid).
func (c *Client) GetMRApprovals(ctx context.Context, projectID, iid int) (*models.ApprovalStatus, error) {
	if projectID <= 0 || iid <= 0 {
		return nil, fmt.Errorf("gitlab: get mr approvals: project id and iid required")
	}

	loader := func(ctx context.Context) (*models.ApprovalStatus, error) {
		a, _, err := c.api.MergeRequestApprovals.GetConfiguration(projectID, int64(iid), gogitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: get mr approvals %d!%d: %w", projectID, iid, err)
		}

		return toDomainApproval(a), nil
	}

	return doCached(ctx, c, mrApprovalsCacheNamespace, "get mr approvals", loader, projectID, iid)
}

func toDomainMergeRequestFromBasic(mr *gogitlab.BasicMergeRequest, projectPath string) *models.MergeRequest {
	if mr == nil {
		return nil
	}
	m := &models.MergeRequest{
		ID:             int(mr.ID),
		IID:            int(mr.IID),
		Title:          mr.Title,
		Description:    mr.Description,
		State:          models.MRState(mr.State),
		Author:         domainUserFromBasic(mr.Author),
		Reviewers:      domainUsersFromBasic(mr.Reviewers),
		SourceBranch:   mr.SourceBranch,
		TargetBranch:   mr.TargetBranch,
		WebURL:         mr.WebURL,
		HasConflicts:   mr.HasConflicts,
		MergeStatus:    mr.DetailedMergeStatus,
		UserNotesCount: int(mr.UserNotesCount),
		ProjectPath:    projectPath,
	}
	if mr.CreatedAt != nil {
		m.CreatedAt = *mr.CreatedAt
	}
	if mr.UpdatedAt != nil {
		m.UpdatedAt = *mr.UpdatedAt
	}
	if mr.MergedAt != nil {
		merged := *mr.MergedAt
		m.MergedAt = &merged
	}

	return m
}

func toDomainMergeRequestFromFull(mr *gogitlab.MergeRequest, projectPath string) *models.MergeRequest {
	if mr == nil {
		return nil
	}

	return toDomainMergeRequestFromBasic(&mr.BasicMergeRequest, projectPath)
}

func toDomainApproval(a *gogitlab.MergeRequestApprovals) *models.ApprovalStatus {
	if a == nil {
		return &models.ApprovalStatus{}
	}
	approvedBy := make([]models.User, 0, len(a.ApprovedBy))
	for _, ap := range a.ApprovedBy {
		if ap == nil || ap.User == nil {
			continue
		}
		approvedBy = append(approvedBy, domainUserFromBasic(ap.User))
	}

	return &models.ApprovalStatus{
		Approved:          a.Approved,
		ApprovalsRequired: int(a.ApprovalsRequired),
		ApprovalsLeft:     int(a.ApprovalsLeft),
		ApprovedBy:        approvedBy,
	}
}

func domainUsersFromBasic(users []*gogitlab.BasicUser) []models.User {
	if len(users) == 0 {
		return nil
	}
	out := make([]models.User, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		out = append(out, domainUserFromBasic(u))
	}

	return out
}

func domainUserFromBasic(u *gogitlab.BasicUser) models.User {
	if u == nil {
		return models.User{}
	}

	return models.User{
		ID:        int(u.ID),
		Username:  u.Username,
		Name:      u.Name,
		WebURL:    u.WebURL,
		AvatarURL: u.AvatarURL,
	}
}

// intPtrArg tags a positional *int for MakeKey so slot identity is preserved
// when another slot is nil. Without the tag, `author=nil, reviewer=7` and
// `author=7, reviewer=nil` would collapse to the same key because MakeKey
// skips nil arguments.
func intPtrArg(tag string, p *int) any {
	if p == nil {
		return nil
	}

	return fmt.Sprintf("%s=%d", tag, *p)
}
